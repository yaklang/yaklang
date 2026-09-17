package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestExactByteBatchTreeStagingAndEmptyShapes(t *testing.T) {
	for _, outcome := range []string{"commit", "bad-span", "bad-endian", "bad-description", "empty"} {
		t.Run(outcome, func(t *testing.T) {
			root := giopBridgeInlineRoot(t, "unit: byte\nPackage:\n  Message: {}\n")
			n := root.Children[0].Children[0]
			n.Cfg.SetItem(CfgIsTerminal, false)
			require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
			root.Cfg.SetItem(CfgLength, uint64(16))
			p := &DefParser{}
			require.NoError(t, p.OnRoot(root))
			n.Cfg.SetItem(CfgLength, uint64(16))
			n.Cfg.SetItem("additionInfo", "retained metadata")
			previous := &base.Node{Name: "retained child", Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
			previous.Cfg.SetItem(CfgParent, n)
			n.Children = []*base.Node{previous}
			fields := []tlsCertificateField{
				{Name: "Empty", Start: 0, End: 0, List: true},
				{Name: "Values", Start: 0, End: 2, List: true, Children: []tlsCertificateField{
					{Name: "First", Type: "uint8", Start: 0, End: 1},
					{Name: "Second", Type: "raw,1", Start: 1, End: 2, Endian: "little"},
				}},
			}
			switch outcome {
			case "bad-span":
				fields[1].Children[1].End = 3
			case "bad-endian":
				fields[1].Children[1].Endian = "invalid"
			case "bad-description":
				fields[1].Children[1].Type = "raw,invalid"
			case "empty":
				fields = nil
			}
			reader := base.NewBitReader(bytes.NewReader([]byte{1, 2}))
			writer := n.Ctx.GetItem("writer").(*base.BitWriter)
			buffer := n.Ctx.GetItem("buffer").(*bytes.Buffer)
			before := writer.Snapshot()
			finishes, rolledBack := 0, false
			err := parseExactByteFieldTreeWithEndian(n, func(raw *base.Node) (func(bool), error) {
				require.NoError(t, reader.Backup())
				err := p.Parse(reader, raw)
				return func(rollback bool) {
					finishes++
					rolledBack = rollback
					if rollback {
						n.Ctx.SetItem("pointer", uint64(0))
						buffer.Reset()
						require.NoError(t, writer.Restore(before))
						require.NoError(t, reader.Recovery())
					} else {
						require.NoError(t, reader.PopBackup())
					}
				}, err
			}, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
				require.Equal(t, []byte{1, 2}, wire)
				return fields, map[string]any{"complete": true}, nil
			}, "batch-test", "big")
			require.Equal(t, 1, finishes)
			if outcome != "commit" && outcome != "empty" {
				require.Error(t, err)
				require.True(t, rolledBack)
				require.Same(t, previous, n.Children[0])
				require.Len(t, n.Children, 1)
				require.Equal(t, "retained metadata", n.Cfg.GetItem("additionInfo"))
				require.Equal(t, before, writer.Snapshot())
				wire, err := reader.ReadBits(16)
				require.NoError(t, err)
				require.Equal(t, []byte{1, 2}, wire)
				return
			}
			require.NoError(t, err)
			require.False(t, rolledBack)
			require.Equal(t, map[string]any{"complete": true}, n.Cfg.GetItem("additionInfo"))
			if outcome == "empty" {
				require.Nil(t, n.Children)
				return
			}
			require.Len(t, n.Children, 2)
			empty, list := n.Children[0], n.Children[1]
			require.Nil(t, empty.Children)
			require.True(t, empty.Cfg.GetBool(CfgIsList))
			require.Equal(t, [2]uint64{0, 0}, GetNodeResultPos(empty))
			require.Same(t, n, list.Cfg.GetItem(CfgParent))
			require.True(t, list.Cfg.GetBool(CfgLastNode))
			for i, child := range list.Children {
				require.Nil(t, child.Children)
				require.Same(t, list, child.Cfg.GetItem(CfgParent))
				require.Equal(t, i, child.Cfg.GetItem(CfgElementIndex))
				require.Equal(t, [2]uint64{uint64(i) * 8, uint64(i+1) * 8}, GetNodeResultPos(child))
				require.Equal(t, i == 1, child.Cfg.GetBool(CfgLastNode))
				replayed := base.AppendConfig(base.NewEmptyConfig(), child.Cfg)
				require.Equal(t, child.Cfg.GetItem(CfgLength), replayed.GetItem(CfgLength))
				require.Equal(t, child.Cfg.GetString(CfgEndian), replayed.GetString(CfgEndian))
				require.Same(t, list, replayed.GetItem(CfgParent))
			}
			require.Equal(t, "little", list.Children[1].Cfg.GetString(CfgEndian))
			require.Equal(t, "big", list.Children[0].Cfg.GetString(CfgEndian))
			require.Equal(t, "raw,1", list.Children[1].Origin)
		})
	}
}

func TestExactByteBatchWideCertificateList(t *testing.T) {
	// The existing TLS decoder validates the exact upper-bound vector. Ensure
	// its field description remains complete before it enters the shared builder.
	certificates := make([][]byte, tlsCertificateMaxEntries)
	for i := range certificates {
		certificates[i] = []byte{byte(i)}
	}
	wire := tlsCertificateTestMessage(certificates...)
	fields, info, err := decodeTLSCertificateHandshake(wire)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(wire))
	require.Equal(t, tlsCertificateMaxEntries, info["Certificate Count"])
	root := giopBridgeInlineRoot(t, "unit: byte\nPackage:\n  Message: {}\n")
	n := root.Children[0].Children[0]
	n.Cfg.SetItem(CfgIsTerminal, false)
	require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
	root.Cfg.SetItem(CfgLength, uint64(len(wire)*8))
	n.Cfg.SetItem(CfgLength, uint64(len(wire)*8))
	p := &DefParser{}
	require.NoError(t, p.OnRoot(root))
	reader := base.NewBitReader(bytes.NewReader(wire))
	require.NoError(t, parseTLSCertificateHandshake(n, func(raw *base.Node) (func(bool), error) {
		return nil, p.Parse(reader, raw)
	}))
	var walk func(*base.Node)
	walk = func(parent *base.Node) {
		for i, child := range parent.Children {
			require.Same(t, parent, child.Cfg.GetItem(CfgParent))
			if parent.Cfg.GetBool(CfgIsList) {
				require.Equal(t, i, child.Cfg.GetItem(CfgElementIndex))
			}
			require.Equal(t, i == len(parent.Children)-1, child.Cfg.GetBool(CfgLastNode))
			walk(child)
		}
	}
	walk(n)
	require.Equal(t, uint64(len(wire)*8), n.Ctx.GetUint64("pointer"))
}

func TestExactByteBatchInitializationHistory(t *testing.T) {
	parent := base.NewEmptyConfig()
	parent.SetItems(base.ConfigItem{Key: CfgEndian, Value: "big"}, base.ConfigItem{Key: "parser", Value: "default"}, base.ConfigItem{Key: "unit", Value: "byte"})
	n := &base.Node{Name: "Message", Cfg: base.NewConfig(parent)}
	fields := []tlsCertificateField{
		{Name: "Values", Start: 0, End: 2, List: true, Children: []tlsCertificateField{
			{Name: "First", Type: "uint8", Start: 0, End: 1},
			{Name: "Second", Type: "uint8", Start: 1, End: 2, Endian: "little"},
		}},
		{Name: "Tail", Type: "raw", Start: 2, End: 3},
	}
	require.NoError(t, buildExactByteFieldTree(n, fields, nil, 0, 24, "history", "big"))
	defaults := []base.ConfigItem{{Key: CfgEndian, Value: "big"}, {Key: "parser", Value: "default"}, {Key: "unit", Value: "byte"}}
	check := func(cfg *base.Config, want []base.ConfigItem) {
		callbacks := cfg.GetItem(base.CfgOptionFuns).([]base.NodeConfigFun)
		require.Len(t, callbacks, len(want))
		for i, callback := range callbacks {
			target := base.NewEmptyConfig()
			callback(target)
			require.True(t, target.Has(want[i].Key), "history entry %d", i)
			if node, ok := want[i].Value.(*base.Node); ok {
				require.Same(t, node, target.GetItem(want[i].Key))
			} else {
				require.Equal(t, want[i].Value, target.GetItem(want[i].Key))
			}
		}
	}
	list := n.Children[0]
	// The original staging parent remains in the early replay entries; only
	// the final write publishes the root's real parent. Do not deduplicate it.
	firstParent := base.NewEmptyConfig()
	list.Cfg.GetItem(base.CfgOptionFuns).([]base.NodeConfigFun)[3](firstParent)
	staged := firstParent.GetItem(CfgParent).(*base.Node)
	require.NotSame(t, n, staged)
	check(list.Cfg, append(append([]base.ConfigItem(nil), defaults...),
		base.ConfigItem{Key: CfgParent, Value: staged}, base.ConfigItem{Key: CfgLength, Value: uint64(16)},
		base.ConfigItem{Key: CfgIsList, Value: true}, base.ConfigItem{Key: CfgParent, Value: staged}, base.ConfigItem{Key: CfgParent, Value: n}))
	for i, child := range list.Children {
		want := append(append([]base.ConfigItem(nil), defaults...), base.ConfigItem{Key: CfgIsTerminal, Value: true}, base.ConfigItem{Key: CfgType, Value: "uint8"},
			base.ConfigItem{Key: CfgNodeResult, Value: [2]uint64{uint64(i * 8), uint64((i + 1) * 8)}})
		if i == 1 {
			want = append(want, base.ConfigItem{Key: CfgEndian, Value: "little"})
		}
		want = append(want, base.ConfigItem{Key: CfgParent, Value: list}, base.ConfigItem{Key: CfgLength, Value: uint64(8)},
			base.ConfigItem{Key: CfgIsList, Value: false}, base.ConfigItem{Key: CfgElementIndex, Value: i}, base.ConfigItem{Key: CfgParent, Value: list})
		if i == 1 {
			want = append(want, base.ConfigItem{Key: CfgLastNode, Value: true})
		}
		check(child.Cfg, want)
	}
}
