package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Frozen pre-extraction IPX block. Unlike comparing two current imports, this
// detects a change to the actual field operator as well as alias resolution.
const ipxStandaloneOriginalRule = `
endian: big
Package:
  IPX:
    operator: |
      this.ProcessSubNode("Checksum")
      this.ProcessSubNode("Length")
      length = getNodeResult("Length").Value
      if length != this.GetMaxLength() || length < 30 {
        panic("ipx: packet length does not match input")
      }
      this.ProcessSubNode("Transport Control")
      this.ProcessSubNode("Packet Type")
      this.ProcessSubNode("Destination Network")
      this.ProcessSubNode("Destination Node")
      this.ProcessSubNode("Destination Socket")
      this.ProcessSubNode("Source Network")
      this.ProcessSubNode("Source Node")
      this.ProcessSubNode("Source Socket")
      remaining = this.GetMaxLength() - this.Length()
      if remaining > 0 {
        this.GetSubNode("Payload").SetMaxLength(remaining)
        this.ProcessSubNode("Payload")
      }
    Checksum: uint16
    Length: uint16
    Transport Control: uint8
    Packet Type: uint8
    Destination Network: uint32
    Destination Node: raw,6
    Destination Socket: uint16
    Source Network: uint32
    Source Node: raw,6
    Source Socket: uint16
    Payload: raw
`

func ipxStandaloneInline(t *testing.T, source string) *base.Node {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	return root
}

func ipxStandaloneFixture(size, variant int) []byte {
	wire := make([]byte, size)
	for i := range wire {
		wire[i] = byte(3 + i*7 + variant*13)
	}
	binary.BigEndian.PutUint16(wire[2:], uint16(size))
	return wire
}

func TestProtocolCorpusIPXStandaloneDefinitionsStayEqual(t *testing.T) {
	standalone, err := base.ParseRule("ipx.yaml")
	require.NoError(t, err)
	legacy, err := base.ParseRule("application-layer/extended_protocols.yaml")
	require.NoError(t, err)
	var compare func(*base.Node, *base.Node)
	compare = func(a, b *base.Node) {
		require.NotNil(t, a)
		require.NotNil(t, b)
		require.Equal(t, a.Name, b.Name)
		// Ordered Origin includes the complete operator text, guards,
		// annotations, field declarations and any future nested definition.
		// Nothing inside either IPX definition is normalized away.
		require.Equal(t, a.Origin, b.Origin)
		for _, key := range []string{base.CfgType, base.CfgLength, "unit", "endian", "operator", "out", "input", "import", "ref-type", "list"} {
			require.Equal(t, a.Cfg.Has(key), b.Cfg.Has(key), "%s %s presence", a.Name, key)
			require.Equal(t, a.Cfg.GetItem(key), b.Cfg.GetItem(key), "%s %s value", a.Name, key)
		}
		require.Len(t, b.Children, len(a.Children))
		for i := range a.Children {
			compare(a.Children[i], b.Children[i])
		}
	}
	compare(base.GetNodeByPath(standalone, "@IPX"), base.GetNodeByPath(legacy, "@IPX"))
}

type ipxStandaloneField struct {
	Name, Type, Endian string
	Length             any
	Processed          bool
	Span               [2]uint64
	Consumed           any
	Value              any
}

// Import boundaries intentionally have their own rule context/root map. The
// compatible relationship is same-context descendants plus the real external
// parent and shared writer/buffer, not pointer equality with the caller's Ctx.
func ipxStandaloneSnapshot(t *testing.T, node *base.Node, wire []byte, offset int) []ipxStandaloneField {
	t.Helper()
	require.NotNil(t, node)
	require.Equal(t, "IPX", node.Name)
	parent, ok := node.Cfg.GetItem(base.CfgParent).(*base.Node)
	require.True(t, ok)
	found := false
	for _, child := range parent.Children {
		found = found || child == node
	}
	require.True(t, found, "IPX parent is the actual tree owner")
	require.Same(t, parent.Ctx.GetItem("writer"), node.Ctx.GetItem("writer"))
	require.Same(t, parent.Ctx.GetItem("buffer"), node.Ctx.GetItem("buffer"))
	require.IsType(t, &base.Node{}, node.Ctx.GetItem("root"))
	require.IsType(t, map[string]*base.Node{}, node.Ctx.GetItem("rootNodeMap"))
	// Structs aggregate their field spans; unlike terminal fields they need
	// not own a synthetic NodeResult or ConsumedBits entry.
	require.EqualValues(t, len(wire)*8, stream_parser.CalcNodeConsumedLength(node))
	result, err := node.Result()
	require.NoError(t, err)
	require.True(t, result.IsStruct())
	names := []string{"Checksum", "Length", "Transport Control", "Packet Type", "Destination Network", "Destination Node", "Destination Socket", "Source Network", "Source Node", "Source Socket", "Payload"}
	widths := []int{2, 2, 1, 1, 4, 6, 2, 4, 6, 2, len(wire) - 30}
	require.Len(t, node.Children, len(names))
	position, valueIndex := offset, 0
	var fields []ipxStandaloneField
	for i, child := range node.Children {
		require.Equal(t, names[i], child.Name)
		require.Empty(t, child.Children)
		require.Same(t, node, child.Cfg.GetItem(base.CfgParent))
		require.Same(t, node.Ctx, child.Ctx)
		field := ipxStandaloneField{Name: child.Name, Type: child.Cfg.GetString(base.CfgType), Endian: child.Cfg.GetString("endian"), Length: child.Cfg.GetItem(base.CfgLength), Processed: stream_parser.NodeHasResult(child), Consumed: child.Cfg.GetItem(stream_parser.CfgConsumedBits)}
		if widths[i] == 0 {
			require.False(t, field.Processed)
			require.Nil(t, result.Child(child.Name), "zero payload remains an unprocessed template")
		} else {
			require.True(t, field.Processed)
			field.Span = stream_parser.GetNodeResultPos(child)
			require.Equal(t, [2]uint64{uint64(position * 8), uint64((position + widths[i]) * 8)}, field.Span)
			value := result.Children()[valueIndex]
			valueIndex++
			require.Equal(t, child.Name, value.Name)
			require.Same(t, child, value.Origin)
			require.True(t, value.IsValue())
			field.Value = value.Value
		}
		position += widths[i]
		fields = append(fields, field)
	}
	require.Len(t, result.Children(), valueIndex)
	return fields
}

func TestProtocolCorpusIPXStandaloneTreeCompatibility(t *testing.T) {
	for _, size := range []int{30, 33, 257} {
		wire := ipxStandaloneFixture(size, size)
		original := ipxStandaloneInline(t, ipxStandaloneOriginalRule)
		original.Cfg.SetItem(base.CfgLength, uint64(len(wire)*8))
		require.NoError(t, original.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), "IPX"))
		want := ipxStandaloneSnapshot(t, base.GetNodeByPath(original, "@IPX"), wire, 0)
		for _, rule := range []string{"ipx", "application-layer.extended_protocols"} {
			t.Run(fmt.Sprintf("%s/%d", rule, size), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, wire, rule, "IPX")
				require.Equal(t, want, ipxStandaloneSnapshot(t, node, wire, 0))
				protocolCorpusIPXFields(t, node, wire)
			})
		}
	}
}

func TestProtocolCorpusIPXStandaloneImportsAndContext(t *testing.T) {
	wire := ipxStandaloneFixture(37, 2)
	var reference []ipxStandaloneField
	for _, imported := range []string{"ipx.yaml", "application-layer/extended_protocols.yaml"} {
		root := ipxStandaloneInline(t, fmt.Sprintf(`
endian: big
Package:
  Envelope:
    Prefix: raw,5
    IPX: "import:%s;node:IPX"
`, imported))
		input := append([]byte{0x71, 0xa5, 0xe3, 0x17, 0xff}, wire...)
		root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))
		shared := map[string]any{"counter": 19}
		root.Ctx.SetItem(base.CtxInputConfig, map[string]any{"ipxCallerValue": 731, "ipxExplicitFalse": false, "ipxSharedState": shared, "root": "do not forward", "writer": "do not forward"})
		reader := base.NewBitReader(bytes.NewReader(input))
		require.NoError(t, root.ParseSubNode(reader, "Envelope"))
		node := base.GetNodeByPath(root, "@Envelope.IPX")
		fields := ipxStandaloneSnapshot(t, node, wire, 5)
		if reference == nil {
			reference = fields
		} else {
			require.Equal(t, reference, fields)
		}
		require.Equal(t, input, NodeToBytes(node))
		require.NotSame(t, root.Ctx, node.Ctx)
		require.Equal(t, 731, node.Ctx.GetItem("ipxCallerValue"))
		require.True(t, node.Ctx.Has("ipxExplicitFalse"))
		require.Equal(t, false, node.Ctx.GetItem("ipxExplicitFalse"))
		node.Ctx.GetItem("ipxSharedState").(map[string]any)["counter"] = 23
		require.Equal(t, 23, shared["counter"])
		ipxStandaloneRequireReaderClean(t, reader, nil)
	}
	for _, sll2 := range []bool{false, true} {
		header, rule, entry := []byte{0xe0, 0xe0, 3}, "llc", "LLC"
		if sll2 {
			header = []byte{0, 1, 0, 0, 0, 0, 0, 17, 0, 1, 0, 6, 1, 3, 5, 7, 9, 11, 0, 0}
			rule, entry = "linux_sll2", "LinuxSLL2"
		}
		input := append(append([]byte(nil), header...), wire...)
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, input, rule, entry, map[string]any{"ipxCallerValue": 731})
		ipx := protocolCorpusFindNode(node, "IPX")
		ipxStandaloneSnapshot(t, ipx, wire, len(header))
		protocolCorpusIPXFields(t, ipx, wire)
		require.Equal(t, 731, ipx.Ctx.GetItem("ipxCallerValue"))
	}
	for _, trailer := range [][]byte{nil, {0x7f, 0x83, 0x29}} {
		input := append(append([]byte{0xe0, 0xe0, 3}, wire...), trailer...)
		root, err := base.ParseRule("llc.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))
		reader := base.NewBitReader(bytes.NewReader(input))
		require.NoError(t, root.ParseSubNode(reader, "LLC"))
		ipxStandaloneSnapshot(t, protocolCorpusFindNode(root, "IPX"), wire, 3)
		require.Equal(t, input, NodeToBytes(root))
		if len(trailer) > 0 {
			protocolCorpusRequireValue(t, root, "IPX Link Trailer", trailer)
		}
		ipxStandaloneRequireReaderClean(t, reader, nil)
	}
}

func ipxStandaloneRequireReaderClean(t *testing.T, reader *base.BitReader, remainder []byte) {
	t.Helper()
	require.ErrorContains(t, reader.Recovery(), "no backup")
	require.ErrorContains(t, reader.PopBackup(), "no backup")
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, remainder, append([]byte(nil), got...))
	_, err = reader.ReadBits(8)
	require.ErrorIs(t, err, io.EOF)
}

func TestProtocolCorpusIPXStandaloneFailureAndReaderCompatibility(t *testing.T) {
	wire := ipxStandaloneFixture(33, 5)
	for cut := 0; cut < len(wire); cut++ {
		for _, rule := range []string{"ipx", "application-layer.extended_protocols"} {
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), rule, "IPX")
			require.Error(t, err, "%s prefix %d", rule, cut)
			require.Nil(t, node)
		}
	}
	for _, valid := range []bool{false, true} {
		for _, source := range []string{ipxStandaloneOriginalRule, "ipx.yaml", "application-layer/extended_protocols.yaml"} {
			var root *base.Node
			if strings.Contains(source, "\n") {
				root = ipxStandaloneInline(t, source)
			} else {
				var err error
				root, err = base.ParseRule(source)
				require.NoError(t, err)
			}
			input := append([]byte(nil), wire...)
			if !valid {
				binary.BigEndian.PutUint16(input[2:], 32)
			}
			root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))
			reader := base.NewBitReader(bytes.NewReader(input))
			err := root.ParseSubNode(reader, "IPX")
			if valid {
				require.NoError(t, err)
				ipxStandaloneRequireReaderClean(t, reader, nil)
			} else {
				require.ErrorContains(t, err, "ipx: packet length does not match input")
				// A direct entry is not a transactional carrier: its first two
				// processed fields stay consumed on this pre-existing failure.
				ipxStandaloneRequireReaderClean(t, reader, input[4:])
			}
		}
	}
}

func TestProtocolCorpusIPXStandaloneStructuredGeneration(t *testing.T) {
	for _, size := range []int{30, 37} {
		wire := ipxStandaloneFixture(size, 3)
		input := map[string]any{"Checksum": binary.BigEndian.Uint16(wire), "Length": size, "Transport Control": wire[4], "Packet Type": wire[5], "Destination Network": binary.BigEndian.Uint32(wire[6:]), "Destination Node": wire[10:16], "Destination Socket": binary.BigEndian.Uint16(wire[16:]), "Source Network": binary.BigEndian.Uint32(wire[18:]), "Source Node": wire[22:28], "Source Socket": binary.BigEndian.Uint16(wire[28:]), "Payload": wire[30:]}
		for _, source := range []string{ipxStandaloneOriginalRule, "ipx.yaml", "application-layer/extended_protocols.yaml"} {
			makeRoot := func() *base.Node {
				if strings.Contains(source, "\n") {
					return ipxStandaloneInline(t, source)
				}
				root, err := base.ParseRule(source)
				require.NoError(t, err)
				return root
			}
			root := makeRoot()
			root.Cfg.SetItem(base.CfgLength, uint64(size*8))
			require.NoError(t, root.GenerateSubNode(input, "IPX"))
			node := base.GetNodeByPath(root, "@IPX")
			require.Equal(t, wire, NodeToBytes(node))
			ipxStandaloneSnapshot(t, node, wire, 0)
			root = makeRoot()
			root.Cfg.SetItem(base.CfgLength, uint64((size+1)*8))
			require.ErrorContains(t, root.GenerateSubNode(input, "IPX"), "ipx: packet length does not match input")
			// There is no public GenerateBinary length/config option. Both
			// original and extracted operators require a caller-supplied bound.
			require.Error(t, makeRoot().GenerateSubNode(input, "IPX"))
		}
		for _, rule := range []string{"ipx", "application-layer.extended_protocols"} {
			node, err := parser.GenerateBinary(input, rule, "IPX")
			require.Nil(t, node)
			require.ErrorContains(t, err, "ipx: packet boundary is required")
		}
		for _, imported := range []string{"ipx.yaml", "application-layer/extended_protocols.yaml"} {
			root := ipxStandaloneInline(t, fmt.Sprintf(`
endian: big
Package:
  Envelope:
    Prefix: raw,5
    IPX: "import:%s;node:IPX"
`, imported))
			prefix := []byte{0x53, 0x71, 0xa9, 0x19, 0xfd}
			root.Cfg.SetItem(base.CfgLength, uint64((len(prefix)+size)*8))
			require.NoError(t, root.GenerateSubNode(map[string]any{"Prefix": prefix, "IPX": input}, "Envelope"))
			node := base.GetNodeByPath(root, "@Envelope.IPX")
			require.Equal(t, append(prefix, wire...), NodeToBytes(node))
			ipxStandaloneSnapshot(t, node, wire, len(prefix))
		}
	}
}

func TestProtocolCorpusIPXStandaloneImportedCandidateTransactions(t *testing.T) {
	for _, imported := range []string{"ipx.yaml", "application-layer/extended_protocols.yaml"} {
		for _, valid := range []bool{false, true} {
			root := ipxStandaloneInline(t, fmt.Sprintf(`
endian: big
Package:
  Envelope:
    operator: |
      this.ProcessSubNode("Prefix")
      result, operation = this.TryProcessByType("Candidate")
      if operation.OK { operation.Save(); return }
      operation.Recovery()
      this.ProcessSubNode("Raw Payload")
    Prefix: raw,5
    Raw Payload: raw
Candidate: "import:%s;node:IPX"
`, imported))
			body := ipxStandaloneFixture(37, 7)
			if !valid {
				binary.BigEndian.PutUint16(body[2:], 38)
			}
			input := append([]byte{0x59, 0x91, 0xed, 0x53, 0x31}, body...)
			root.Cfg.SetItem(base.CfgLength, uint64(len(input)*8))
			reader := base.NewBitReader(bytes.NewReader(input))
			require.NoError(t, root.ParseSubNode(reader, "Envelope"))
			require.Equal(t, input, NodeToBytes(root), "successful candidate or failed candidate must consume and preserve every byte")
			if valid {
				require.NotNil(t, protocolCorpusFindNode(root, "Source Socket"))
				require.Nil(t, protocolCorpusFindNode(root, "Raw Payload"))
			} else {
				require.Nil(t, protocolCorpusFindNode(root, "Source Socket"))
				protocolCorpusRequireValue(t, root, "Raw Payload", body)
				require.Equal(t, [2]uint64{40, uint64(len(input) * 8)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(root, "Raw Payload")))
			}
			ipxStandaloneRequireReaderClean(t, reader, nil)
		}
	}
}

func TestProtocolCorpusIPXStandaloneParallelIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for _, rule := range []string{"ipx", "application-layer.extended_protocols"} {
				wire := ipxStandaloneFixture(30+i, i)
				node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, "IPX", map[string]any{"ipxCallerValue": i})
				protocolCorpusIPXFields(t, node, wire)
				ipxStandaloneSnapshot(t, node, wire, 0)
				require.Equal(t, i, node.Ctx.GetItem("ipxCallerValue"))
			}
		})
	}
}
