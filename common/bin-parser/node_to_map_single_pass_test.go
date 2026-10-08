package bin_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Frozen pre-optimization traversal, deliberately retaining the repeated
// recursive call. It is a test-only oracle, never a runtime compatibility flag.
func nodeToMapRepeatedReference(node *base.Node) any {
	if node.Cfg.Has(stream_parser.CfgNodeResult) {
		if node.Cfg.GetBool(stream_parser.CfgIsList) && !stream_parser.NodeIsTerminal(node) && len(node.Children) == 0 {
			span := stream_parser.GetNodeResultPos(node)
			if span[0] == span[1] {
				value, err := stream_parser.ToMap(node)
				if err == nil && value != nil && value.IsList() {
					return []any{}
				}
			}
		}
		return stream_parser.GetResultByNode(node)
	}
	if node.Cfg.GetBool(stream_parser.CfgIsList) {
		res := []any{}
		for _, sub := range node.Children {
			d := nodeToMapRepeatedReference(sub)
			if d != nil {
				res = append(res, d)
			}
		}
		if len(res) == 0 {
			return nil
		}
		return res
	}
	res := map[string]any{}
	for _, sub := range node.Children {
		d := nodeToMapRepeatedReference(sub)
		if d != nil {
			res[sub.Name] = nodeToMapRepeatedReference(sub)
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res
}

func nodeMapRequireEquivalent(t *testing.T, n *base.Node) {
	t.Helper()
	want := nodeToMapRepeatedReference(n)
	got := NodeToMap(n)
	require.Equal(t, want, got) // Includes concrete numeric and byte-slice types.
	a, err := json.Marshal(map[string]any{"fields": want, "metadata": n.Cfg.GetItem("additionInfo")})
	require.NoError(t, err)
	b, err := currentCorpusJSON(n)
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestNodeToMapSinglePassSemantics(t *testing.T) {
	wire := []byte("stats\r\n")
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.memcached_fields", "MemcachedStatsRequestFields")
	container := func(name string, list bool, children ...*base.Node) *base.Node {
		v := base.NewEmptyNode(name, nil, base.NewEmptyConfig(), n.Ctx)
		v.Cfg.SetItem(stream_parser.CfgIsList, list)
		v.Cfg.SetItem("out", "panic(\"NodeToMap must not execute custom out\")")
		v.Children = children
		return v
	}
	leaf := func(name, typ string, from, to uint64) *base.Node {
		v := container(name, false)
		v.Cfg.SetItem(base.CfgIsTerminal, true)
		v.Cfg.SetItem(base.CfgType, typ)
		v.Cfg.SetItem(base.CfgNodeResult, [2]uint64{from, to})
		return v
	}
	empty := container("empty", true)
	empty.Cfg.SetItem(base.CfgNodeResult, [2]uint64{56, 56})
	dormant := container("dormant", true)
	raw := leaf("raw", "raw", 0, 16)
	list := container("list", true, leaf("item", "uint8", 0, 8), dormant, leaf("item", "uint8", 8, 16))
	root := container("root", false, leaf("duplicate", "uint8", 0, 8), leaf("duplicate", "uint8", 8, 16),
		raw, list, empty, dormant, leaf("bits", "uint8", 1, 6))
	want := map[string]any{"duplicate": uint8('t'), "raw": []byte("st"), "list": []any{uint8('s'), uint8('t')}, "empty": []any{}, "bits": uint8(28)}
	require.Equal(t, want, NodeToMap(root))
	nodeMapRequireEquivalent(t, root)
	require.Nil(t, NodeToMap(dormant))
	require.Nil(t, NodeToMap(container("empty-struct", false)))
	// Deep composite structure made the old traversal revisit a subtree
	// twice per struct level. This is an in-memory vector, not a corpus claim.
	for i := 0; i < 8; i++ {
		root = container(fmt.Sprintf("level-%d", i), false, root)
	}
	nodeMapRequireEquivalent(t, root)
	// A retained projection cannot be overwritten by a later projection.
	a := NodeToMap(list).([]any)
	b := NodeToMap(list).([]any)
	b[0] = uint8(0)
	require.Equal(t, uint8('s'), a[0])
	require.Equal(t, wire, NodeToBytes(n))
}

func TestProtocolCorpusNodeToMapSinglePassEnvelopes(t *testing.T) {
	works := currentCorpusWorks(t, false)
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
			t.Parallel()
			count := 0
			for i := worker; i < len(works); i += 4 {
				w := works[i]
				reader := newProtocolCorpusBoundedReader(w.wire)
				n, err := parser.ParseBinary(reader, w.rule, w.entry)
				if w.rejection != "" {
					require.ErrorContains(t, err, w.rejection, w.id)
					require.Nil(t, n, w.id)
				} else {
					require.NoError(t, err, w.id)
					require.Zero(t, reader.Len(), w.id)
					require.True(t, bytes.Equal(w.wire, NodeToBytes(n)), w.id)
					nodeMapRequireEquivalent(t, n)
				}
				count++
			}
			require.Equal(t, (len(works)+3-worker)/4, count)
			t.Logf("checked %d retained records", count)
		})
	}
}

func TestProtocolCorpusNodeToMapSinglePassApplication(t *testing.T) {
	for _, w := range currentCorpusWorks(t, true) {
		t.Run(w.id, func(t *testing.T) {
			n := protocolCorpusRequireBoundedRuleParse(t, w.wire, w.rule, w.entry)
			nodeMapRequireEquivalent(t, n)
			require.Equal(t, w.wire, NodeToBytes(n))
		})
	}
}

func BenchmarkCurrentCorpusApplicationJSONRepeatedReference(b *testing.B) {
	benchmarkCurrentCorpusWithProjection(b, true, true, nodeToMapRepeatedReference)
}
