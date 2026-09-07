package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// The old YAML operator remains an explicit caller-selected oracle. These
// small fixtures compare externally observable tree/value shape, not a second
// implementation of the native bridge. The 4096/4097 limit tests stay in the
// existing resource suite; repeating them here would obscure useful latency.
type dicomNativeTreeSnapshot struct {
	Path, Name, ParentPath, ParentName string
	ContextGroup, ParentContextGroup   int
	ContextRootName                    string
	SharesRootBuffer, SharesRootWriter bool
	HasType, HasLength, HasUnit        bool
	Type, Length, Unit                 any
	Terminal, HasList, List            bool
	HasConfigInList, HasContextInList  bool
	ConfigInList, ContextInList        any
	HasResult, HasConsumed             bool
	Span                               [2]uint64
	Consumed                           any
}

type dicomNativeValueSnapshot struct {
	Name, OriginPath string
	List, Struct     bool
	Value            any
	Children         []dicomNativeValueSnapshot
}

func dicomNativeSnapshot(t *testing.T, root *base.Node, wire []byte) ([]dicomNativeTreeSnapshot, dicomNativeValueSnapshot) {
	t.Helper()
	paths := map[*base.Node]string{}
	contexts := map[*base.NodeContext]int{}
	var ordered []*base.Node
	contextID := func(ctx *base.NodeContext) int {
		if ctx == nil {
			return -1
		}
		if id, ok := contexts[ctx]; ok {
			return id
		}
		id := len(contexts)
		contexts[ctx] = id
		return id
	}
	var index func(*base.Node, *base.Node, string)
	index = func(node, parent *base.Node, path string) {
		require.NotNil(t, node.Cfg, path)
		require.NotNil(t, node.Ctx, path)
		if parent != nil {
			require.Same(t, parent, node.Cfg.GetItem(base.CfgParent), "actual parent at %s", path)
		}
		require.NotContains(t, paths, node, "node cannot occur twice in the tree")
		paths[node] = path
		ordered = append(ordered, node)
		contextID(node.Ctx)
		for i, child := range node.Children {
			index(child, node, fmt.Sprintf("%s/%d:%s", path, i, child.Name))
		}
	}
	index(root, nil, root.Name)
	var tree []dicomNativeTreeSnapshot
	for _, node := range ordered {
		s := dicomNativeTreeSnapshot{Path: paths[node], Name: node.Name, ParentContextGroup: -1, ContextGroup: contextID(node.Ctx),
			SharesRootBuffer: node.Ctx.GetItem("buffer") == root.Ctx.GetItem("buffer"), SharesRootWriter: node.Ctx.GetItem("writer") == root.Ctx.GetItem("writer"),
			HasType: node.Cfg.Has(base.CfgType), HasLength: node.Cfg.Has(base.CfgLength), HasUnit: node.Cfg.Has("unit"), Type: node.Cfg.GetItem(base.CfgType), Length: node.Cfg.GetItem(base.CfgLength), Unit: node.Cfg.GetItem("unit"),
			Terminal: stream_parser.NodeIsTerminal(node), HasList: node.Cfg.Has(stream_parser.CfgIsList), List: node.Cfg.GetBool(stream_parser.CfgIsList),
			HasConfigInList: node.Cfg.Has(stream_parser.CfgInList), ConfigInList: node.Cfg.GetItem(stream_parser.CfgInList), HasContextInList: node.Ctx.Has(stream_parser.CfgInList), ContextInList: node.Ctx.GetItem(stream_parser.CfgInList),
			HasResult: stream_parser.NodeHasResult(node), HasConsumed: node.Cfg.Has(stream_parser.CfgConsumedBits), Consumed: node.Cfg.GetItem(stream_parser.CfgConsumedBits)}
		if parent, ok := node.Cfg.GetItem(base.CfgParent).(*base.Node); ok && parent != nil {
			s.ParentName = parent.Name
			s.ParentContextGroup = contextID(parent.Ctx)
			s.ParentPath = paths[parent]
			if s.ParentPath == "" {
				s.ParentPath = "outside-subtree:" + parent.Name
			}
		}
		if ctxRoot, ok := node.Ctx.GetItem("root").(*base.Node); ok && ctxRoot != nil {
			s.ContextRootName = ctxRoot.Name
		}
		if s.HasResult {
			s.Span = stream_parser.GetNodeResultPos(node)
			require.LessOrEqual(t, s.Span[0], s.Span[1], s.Path)
			require.LessOrEqual(t, s.Span[1], uint64(len(wire))*8, s.Path)
		}
		tree = append(tree, s)
	}
	value, err := root.Result()
	require.NoError(t, err)
	var snapshotValue func(*base.NodeValue) dicomNativeValueSnapshot
	snapshotValue = func(value *base.NodeValue) dicomNativeValueSnapshot {
		require.Contains(t, paths, value.Origin, "result origin %s", value.Name)
		s := dicomNativeValueSnapshot{Name: value.Name, OriginPath: paths[value.Origin], List: value.IsList(), Struct: value.IsStruct()}
		if value.IsValue() {
			s.Value = value.Value
		} else {
			for _, child := range value.Children() {
				s.Children = append(s.Children, snapshotValue(child))
			}
		}
		return s
	}
	require.Equal(t, wire, NodeToBytes(root), "result buffer must preserve exact global bytes")
	return tree, snapshotValue(value)
}

func dicomNativeFixture(count, variant int) ([]byte, [][]byte, []byte) {
	var body []byte
	var fragments [][]byte
	var controls []byte
	for i := 0; i < count; i++ {
		control := byte(0xa4 | ((i + variant) & 3))
		var fragment []byte
		if variant != 0 || i%3 != 0 {
			fragment = make([]byte, 2*(i%3+1))
			for j := range fragment {
				fragment[j] = byte(1 + (i*11+j)%255)
			}
		}
		body = append(body, dicomTestPDV(253, control, fragment)...)
		fragments = append(fragments, fragment)
		controls = append(controls, control)
	}
	return dicomTestPDU(4, body), fragments, controls
}

func dicomNativeRequireFieldRanges(t *testing.T, root *base.Node, pduOffset int, fragments [][]byte, controls []byte) {
	t.Helper()
	dicom := protocolCorpusFindNode(root, "DICOM")
	require.NotNil(t, dicom)
	list := protocolCorpusFindNode(dicom, "Presentation Data Values")
	require.NotNil(t, list)
	require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
	require.Len(t, list.Children, len(fragments))
	listValue, err := list.Result()
	require.NoError(t, err)
	require.True(t, listValue.IsList())
	require.Len(t, listValue.Children(), len(fragments))
	position := pduOffset + 6
	for i, pdv := range list.Children {
		require.Equal(t, "PDV", pdv.Name)
		names := make([]string, len(pdv.Children))
		for j, child := range pdv.Children {
			names[j] = child.Name
		}
		require.Equal(t, []string{"PDV Length", "Context ID", "Control Reserved", "Last Fragment", "Command Fragment", "Message Fragment"}, names)
		start := uint64(position) * 8
		for j, span := range [][2]uint64{{start, start + 32}, {start + 32, start + 40}, {start + 40, start + 46}, {start + 46, start + 47}, {start + 47, start + 48}} {
			require.Equal(t, span, stream_parser.GetNodeResultPos(pdv.Children[j]), "PDV %d field %s", i, pdv.Children[j].Name)
		}
		for field, value := range map[string]uint64{"PDV Length": uint64(len(fragments[i]) + 2), "Context ID": 253, "Control Reserved": uint64(controls[i] >> 2), "Last Fragment": uint64((controls[i] >> 1) & 1), "Command Fragment": uint64(controls[i] & 1)} {
			protocolCorpusRequireValue(t, pdv, field, value)
		}
		fragmentNode := pdv.Children[5]
		fragmentValue := listValue.Children()[i].Child("Message Fragment")
		if len(fragments[i]) == 0 {
			// The legacy tree keeps an unprocessed raw template child; it is
			// deliberately absent from Result(), not a synthesized empty field.
			require.False(t, stream_parser.NodeHasResult(fragmentNode))
			require.False(t, fragmentNode.Cfg.Has(base.CfgLength))
			require.Nil(t, fragmentValue)
		} else {
			require.Equal(t, [2]uint64{start + 48, start + 48 + uint64(len(fragments[i]))*8}, stream_parser.GetNodeResultPos(fragmentNode))
			require.NotNil(t, fragmentValue)
			require.Equal(t, fragments[i], fragmentValue.Value)
		}
		position += 6 + len(fragments[i])
	}
}

func TestProtocolCorpusDICOMNativeLegacyTreeDifferential(t *testing.T) {
	for _, count := range []int{1, 2, 8, 128} {
		for _, variant := range []int{0, 1} {
			pdu, fragments, controls := dicomNativeFixture(count, variant)
			for _, imported := range []bool{false, true} {
				t.Run(fmt.Sprintf("pdvs-%d/variant-%d/import-%t", count, variant, imported), func(t *testing.T) {
					wire, rule, entry, offset := pdu, dicomRule, "DICOM", 0
					if imported {
						wire = ipv4TCPFrame(t, 41000, 104, pdu)
						rule, entry, offset = "ethernet", "Ethernet", 54
						require.Equal(t, pdu, wire[offset:])
					}
					native := protocolCorpusRequireBoundedRuleParse(t, wire, rule, entry)
					legacy := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, map[string]any{"dicomPDVLegacy": true})
					for _, root := range []*base.Node{native, legacy} {
						dicomNativeRequireFieldRanges(t, root, offset, fragments, controls)
					}
					nativeTree, nativeValue := dicomNativeSnapshot(t, native, wire)
					legacyTree, legacyValue := dicomNativeSnapshot(t, legacy, wire)
					require.Equal(t, legacyTree, nativeTree, "every node's order, config, span, consumption and relationship")
					require.Equal(t, legacyValue, nativeValue, "complete Result tree with originating node paths")
				})
			}
		}
	}
}

func TestProtocolCorpusDICOMNativeLegacyRejectedBodies(t *testing.T) {
	valid := dicomTestPDV(1, 0xff, []byte{0xaa, 0xbb})
	badLength := func(length uint32) []byte {
		v := append([]byte(nil), valid...)
		binary.BigEndian.PutUint32(v, length)
		return v
	}
	bodies := [][]byte{nil, {0}, {0, 0, 0, 2}, {0, 0, 0, 2, 1}, badLength(0), badLength(1), badLength(3), badLength(5), badLength(0xffffffff), dicomTestPDV(0, 3, nil), dicomTestPDV(2, 3, nil), dicomTestPDV(1, 3, []byte{1}), append(append([]byte(nil), valid...), 0), append(append([]byte(nil), valid...), dicomTestPDV(3, 0, nil)...)}
	for i, body := range bodies {
		wire := dicomTestPDU(4, body)
		for _, legacy := range []bool{false, true} {
			_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), dicomRule, map[string]any{"dicomPDVLegacy": legacy}, "DICOM")
			require.Error(t, err, "body %d legacy %t", i, legacy)
		}
	}
	pdu, _, _ := dicomNativeFixture(2, 0)
	for cut := 0; cut < len(pdu); cut++ {
		for _, legacy := range []bool{false, true} {
			_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(pdu[:cut]), dicomRule, map[string]any{"dicomPDVLegacy": legacy}, "DICOM")
			require.Error(t, err, "prefix %d legacy %t", cut, legacy)
		}
	}
}

func TestProtocolCorpusDICOMNativeLegacyStreamBoundary(t *testing.T) {
	pdu, fragments, controls := dicomNativeFixture(8, 0)
	var snapshots [][]dicomNativeTreeSnapshot
	var values []dicomNativeValueSnapshot
	for _, legacy := range []bool{false, true} {
		reader := bytes.NewReader(append(append([]byte(nil), pdu...), 0xde, 0xad, 0xbe, 0xef))
		root, err := parser.ParseBinaryWithConfig(reader, dicomRule, map[string]any{"dicomPDVLegacy": legacy}, "DICOM")
		require.NoError(t, err)
		require.Equal(t, 4, reader.Len())
		dicomNativeRequireFieldRanges(t, root, 0, fragments, controls)
		tree, value := dicomNativeSnapshot(t, root, pdu)
		snapshots = append(snapshots, tree)
		values = append(values, value)
	}
	require.Equal(t, snapshots[1], snapshots[0])
	require.Equal(t, values[1], values[0])
}
