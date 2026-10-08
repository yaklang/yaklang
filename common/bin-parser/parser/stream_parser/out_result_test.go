package stream_parser_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const outResultRule = `
Package:
  Record:
    Flag: uint8
    Items:
      list: true
      list-length: 2
      Item:
        Count: uint8
        Value: raw,2
        out: |
          literal = b"abc"
          literal[0] = data.Child("Count").Value
          return newStructValue(node, newValue("count", data.Child("Count").Value), data.Child("Value"), newValue("literal", literal), newValue("alias", name))
      out: |
        return newListValue(node, data.Value...)
    out: |
      return newStructValue(node, data.Child("Flag"), data.Child("Items"))
`

type outResultSnapshot struct {
	Name, Origin string
	List, Struct bool
	Value        any
	Children     []outResultSnapshot
}

type outResultNodeSnapshot struct {
	Name       string
	ChildCount int
	HasResult  bool
	Span       [2]uint64
	Value      any
}

func snapshotOutResult(value *base.NodeValue) outResultSnapshot {
	s := outResultSnapshot{Name: value.Name, List: value.IsList(), Struct: value.IsStruct()}
	if value.Origin != nil {
		s.Origin = value.Origin.Name
	}
	if value.IsValue() {
		s.Value = value.Value
	} else {
		for _, child := range value.Children() {
			s.Children = append(s.Children, snapshotOutResult(child))
		}
	}
	return s
}

func snapshotOutResultNodes(node *base.Node) []outResultNodeSnapshot {
	var snapshots []outResultNodeSnapshot
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		snapshot := outResultNodeSnapshot{Name: current.Name, ChildCount: len(current.Children)}
		if stream_parser.NodeHasResult(current) {
			snapshot.HasResult = true
			snapshot.Span = stream_parser.GetNodeResultPos(current)
			snapshot.Value = stream_parser.GetNodeResult(current)
			if raw, ok := snapshot.Value.([]byte); ok {
				snapshot.Value = bytes.Clone(raw)
			}
		}
		snapshots = append(snapshots, snapshot)
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return snapshots
}

func outResultNodesWithExpression(node *base.Node) []*base.Node {
	var nodes []*base.Node
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if current.Cfg.Has("out") {
			nodes = append(nodes, current)
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return nodes
}

func outResultOptionCount(t *testing.T, node *base.Node) int {
	t.Helper()
	options, ok := node.Cfg.GetItem(base.CfgOptionFuns).([]base.NodeConfigFun)
	require.True(t, ok, "%s has no config replay history", node.Name)
	return len(options)
}

func newOutResultTree(t *testing.T, legacy bool) *base.Node {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(outResultRule), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	root.Ctx.SetItem("outProgramLegacy", legacy)
	return root
}

func TestOutResultNestedHelpersAndGeneration(t *testing.T) {
	wire := []byte{42, 17, 1, 2, 34, 3, 4}
	var want *outResultSnapshot
	for _, generated := range []bool{false, true} {
		for _, legacy := range []bool{true, false} {
			t.Run(fmt.Sprintf("generate-%t/legacy-%t", generated, legacy), func(t *testing.T) {
				root := newOutResultTree(t, legacy)
				if generated {
					require.NoError(t, root.GenerateSubNode(map[string]any{
						"Flag": 42, "Items": []any{
							map[string]any{"Count": 17, "Value": []byte{1, 2}},
							map[string]any{"Count": 34, "Value": []byte{3, 4}},
						},
					}, "Record"))
				} else {
					root.Cfg.SetItem(base.CfgLength, uint64(len(wire)*8))
					reader := bytes.NewReader(wire)
					require.NoError(t, root.ParseSubNode(base.NewBitReader(reader), "Record"))
					require.Zero(t, reader.Len())
				}
				record := base.GetNodeByPath(root, "@Record")
				require.NotNil(t, record)
				require.Equal(t, wire, record.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
				record.Cfg.SetItem("result-replay-marker", "kept")
				outNodes := outResultNodesWithExpression(record)
				require.Len(t, outNodes, 4)
				historyLengths := make(map[*base.Node]int, len(outNodes))
				for _, node := range outNodes {
					historyLengths[node] = outResultOptionCount(t, node)
				}
				nodeState := snapshotOutResultNodes(record)
				for repeat := 0; repeat < 3; repeat++ {
					value, err := record.Result()
					require.NoError(t, err)
					for _, node := range outNodes {
						require.Equal(t, historyLengths[node], outResultOptionCount(t, node), "%s replay history changed on Result call %d", node.Name, repeat)
					}
					require.Equal(t, nodeState, snapshotOutResultNodes(record), "parsed values, spans, or child order changed on Result call %d", repeat)
					require.Same(t, record, value.Origin)
					require.Equal(t, uint8(42), value.Child("Flag").Value)
					items := value.Child("Items")
					require.True(t, items.IsList())
					require.Len(t, items.Children(), 2)
					for index, count := range []int{17, 34} {
						item := items.Children()[index]
						require.Equal(t, count, item.Child("count").Value)
						require.Nil(t, item.Child("count").Origin, "named helper value has no node origin")
						require.Equal(t, "Item", item.Child("alias").Value)
						require.Equal(t, []byte{byte(count), 'b', 'c'}, item.Child("literal").Value)
						leaf := item.Child("Value")
						require.Equal(t, wire[2+index*3:4+index*3], leaf.Value)
						require.Equal(t, [2]uint64{uint64(2+index*3) * 8, uint64(4+index*3) * 8}, stream_parser.GetNodeResultPos(leaf.Origin))
					}
					snapshot := snapshotOutResult(value)
					if want == nil {
						want = &snapshot
					} else {
						require.Equal(t, *want, snapshot)
					}
					// Neither the compiled literal nor a previous NodeValue may be
					// reused by the next Result call on this same serially used tree.
					if repeat > 0 {
						items.Children()[0].Child("literal").Value.([]byte)[0] = 255
					}
				}
				merged := base.AppendConfig(base.NewEmptyConfig(), record.Cfg)
				require.Equal(t, "kept", merged.GetString("result-replay-marker"))
				require.Equal(t, record.Cfg.GetString("out"), merged.GetString("out"))
			})
		}
	}
}

func TestOutResultRestoresExpressionAndFormatterOnFailure(t *testing.T) {
	wire := []byte{42, 17, 1, 2, 34, 3, 4}
	for _, generated := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("generate-%t/legacy-%t", generated, legacy), func(t *testing.T) {
				root := newOutResultTree(t, legacy)
				if generated {
					require.NoError(t, root.GenerateSubNode(map[string]any{
						"Flag": 42, "Items": []any{
							map[string]any{"Count": 17, "Value": []byte{1, 2}},
							map[string]any{"Count": 34, "Value": []byte{3, 4}},
						},
					}, "Record"))
				} else {
					require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), "Record"))
				}
				record := base.GetNodeByPath(root, "@Record")
				nodeState := snapshotOutResultNodes(record)
				original := record.Cfg.GetString("out")
				for _, source := range []string{"return (", `panic("expected out rejection")`} {
					record.Cfg.SetItem("out", source)
					historyLength := outResultOptionCount(t, record)
					_, err := record.Result()
					require.Error(t, err)
					require.Equal(t, source, record.Cfg.GetString("out"))
					require.Equal(t, historyLength, outResultOptionCount(t, record))
					require.Equal(t, nodeState, snapshotOutResultNodes(record))
				}
				record.Cfg.SetItem("out", original)
				record.Ctx.SetItem("formatter", "missing-out-test-formatter")
				historyLength := outResultOptionCount(t, record)
				_, err := record.Result()
				require.ErrorContains(t, err, "formatter missing-out-test-formatter not found")
				require.Equal(t, original, record.Cfg.GetString("out"))
				require.Equal(t, "missing-out-test-formatter", record.Ctx.GetString("formatter"))
				require.Equal(t, historyLength, outResultOptionCount(t, record))
				require.Equal(t, nodeState, snapshotOutResultNodes(record))
				record.Ctx.DeleteItem("formatter")
				historyLength = outResultOptionCount(t, record)
				value, err := record.Result()
				require.NoError(t, err)
				require.Equal(t, uint8(42), value.Child("Flag").Value)
				require.Equal(t, historyLength, outResultOptionCount(t, record))
			})
		}
	}
}

func TestOutResultCallerSettingPropagatesThroughImports(t *testing.T) {
	// LDAP's Message alias imports BER, whose out expressions are used while
	// parsing. This complete bind message exercises explicit config propagation.
	wire := []byte{0x30, 0x0c, 0x02, 0x01, 0x01, 0x60, 0x07, 0x02, 0x01, 0x03, 0x04, 0x00, 0x80, 0x00}
	var want *outResultSnapshot
	for _, legacy := range []bool{true, false} {
		root, err := parser.ParseBinaryWithConfig(bytes.NewReader(wire), "application-layer.ldap", map[string]any{"outProgramLegacy": legacy}, "Message")
		require.NoError(t, err)
		count := 0
		var walk func(*base.Node)
		walk = func(node *base.Node) {
			if node.Cfg.Has("out") {
				count++
				require.Equal(t, legacy, node.Ctx.GetBool("outProgramLegacy"), node.Name)
			}
			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(root)
		require.Greater(t, count, 1)
		value, err := root.Result()
		require.NoError(t, err)
		snapshot := snapshotOutResult(value)
		if want == nil {
			want = &snapshot
		} else {
			require.Equal(t, *want, snapshot)
		}
	}
}
