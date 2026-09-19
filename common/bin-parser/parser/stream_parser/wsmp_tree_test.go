package stream_parser

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func wsmpNativeWires(t *testing.T) [][]byte {
	t.Helper()
	var result [][]byte
	for _, encoded := range []string{
		"0220800003616263", "0280020f01ac10010c0401148100058102616263",
		"02e00000018200020102", "03002003616263",
		"0b050f01ac10010c040114170155fe02aabb00c0000103616263",
		"0301800201fd02beef02cafe", "03021234567803010203", "0b0003123456780000",
		"030120010402aabb00", "0300800000", "0300efffffff00", "0220800000",
	} {
		wire, err := hex.DecodeString(encoded)
		require.NoError(t, err)
		result = append(result, wire)
	}
	return result
}

func wsmpBridgeTestNode(t *testing.T, size int) (*DefParser, *base.Node) {
	t.Helper()
	root, err := base.ParseRule("wsmp.yaml")
	require.NoError(t, err)
	p := &DefParser{}
	require.NoError(t, p.OnRoot(root))
	anchor := root.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["Package"]
	n, err := NewNodeByType(anchor, "WSMPMessage")
	require.NoError(t, err)
	n.Name = "WSMP"
	n.Cfg.SetItem(CfgLength, uint64(size)*8)
	return p, n
}

func TestWSMPNativeDecodeBoundaries(t *testing.T) {
	for index, wire := range wsmpNativeWires(t) {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			before := bytes.Clone(wire)
			plan, err := decodeWSMP(wire)
			require.NoError(t, err)
			require.NotEmpty(t, plan.children)
			require.Equal(t, len(wire), plan.length)
			var end int
			var walk func(wsmpField)
			walk = func(f wsmpField) {
				if f.terminal {
					require.Equal(t, end, f.start)
					require.Greater(t, f.end, f.start)
					end = f.end
				}
				for _, child := range f.children {
					walk(child)
				}
			}
			walk(plan)
			require.Equal(t, len(wire)*8, end)
			for cut := 0; cut < len(wire); cut++ {
				plan, err := decodeWSMP(wire[:cut])
				require.Error(t, err, "cut %d", cut)
				require.Empty(t, plan, "invalid input must not publish a prefix plan")
			}
			require.Equal(t, before, wire)
		})
	}
}

// Include skipped nodes and list templates, not just terminal field values.
// Runtime closure identity is intentionally not compared; all semantic config
// values, parent/Origin relationships and context buffer identity are checked.
func wsmpCompareTrees(t *testing.T, fast, legacy *base.Node) {
	t.Helper()
	require.Equal(t, legacy.Name, fast.Name)
	require.Equal(t, legacy.Origin, fast.Origin, fast.Name)
	for _, key := range append(append([]string{}, wsmpSchemaKeys...), CfgIsTerminal, CfgEndian, "parser", CfgUnit, CfgLastNode, CfgIsTempRoot, "package-child") {
		if key == "template" {
			continue
		}
		require.Equal(t, legacy.Cfg.Has(key), fast.Cfg.Has(key), "%s config %s presence", fast.Name, key)
		require.Equal(t, legacy.Cfg.GetItem(key), fast.Cfg.GetItem(key), "%s config %s", fast.Name, key)
	}
	require.Equal(t, legacy.Cfg.Has("template"), fast.Cfg.Has("template"))
	if fast.Cfg.Has("template") {
		ft, lt := fast.Cfg.GetItem("template").(*base.Node), legacy.Cfg.GetItem("template").(*base.Node)
		require.Same(t, fast, ft.Cfg.GetItem(CfgParent))
		require.Same(t, legacy, lt.Cfg.GetItem(CfgParent))
		wsmpCompareTrees(t, ft, lt)
	}
	require.Len(t, fast.Children, len(legacy.Children), fast.Name)
	for index := range fast.Children {
		fc, lc := fast.Children[index], legacy.Children[index]
		require.Same(t, fast, fc.Cfg.GetItem(CfgParent), fc.Name)
		require.Same(t, legacy, lc.Cfg.GetItem(CfgParent), lc.Name)
		require.Same(t, fast.Ctx, fc.Ctx)
		wsmpCompareTrees(t, fc, lc)
	}
	if NodeHasResult(fast) || (!NodeIsTerminal(fast) && CalcNodeConsumedLength(fast) > 0) {
		fv, fe := fast.Result()
		lv, le := legacy.Result()
		require.NoError(t, fe)
		require.NoError(t, le)
		require.Same(t, fast, fv.Origin)
		require.Same(t, legacy, lv.Origin)
		require.Equal(t, lv.IsList(), fv.IsList())
		if _, structValue := fv.Value.([]*base.NodeValue); !structValue {
			require.IsType(t, lv.Value, fv.Value)
			require.Equal(t, lv.Value, fv.Value)
		}
	}
}

func TestWSMPNativeFullTreeDifferential(t *testing.T) {
	for index, wire := range wsmpNativeWires(t) {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			parse := func(legacy bool) *base.Node {
				root, err := base.ParseRule("wsmp.yaml")
				require.NoError(t, err)
				root.Cfg.SetItem(CfgLength, uint64(len(wire))*8)
				root.Ctx.SetItem("wsmpLegacy", legacy)
				root.Ctx.SetItem("outScalarLegacy", legacy)
				require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), "WSMP"))
				return base.GetNodeByPath(root, "@WSMP")
			}
			fast, legacy := parse(false), parse(true)
			wsmpCompareTrees(t, fast, legacy)
			require.Equal(t, wire, fast.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
			require.Equal(t, wire, legacy.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
			require.Equal(t, uint64(len(wire))*8, fast.Ctx.GetUint64("pointer"))
		})
	}
}

func TestWSMPNativeBridgeTransaction(t *testing.T) {
	body := wsmpNativeWires(t)[4]
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid tail", "short read", "callback error"} {
			t.Run(fmt.Sprintf("bits-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(body)
				if outcome == "invalid tail" {
					input[len(input)-4] = 4
				}
				if outcome == "short read" {
					input = input[:len(input)-1]
				}
				p, n := wsmpBridgeTestNode(t, len(body))
				cfg, first := n.Cfg, n.Children[0]
				var wire bytes.Buffer
				ww := base.NewBitWriter(&wire)
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0x55}, pending))
					_, err := p.write([]byte{0x55}, pending)
					require.NoError(t, err)
				}
				require.NoError(t, ww.WriteBits(input, uint64(len(input))*8))
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0}, 8-pending))
				}
				r := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if pending > 0 {
					_, err := r.ReadBits(pending)
					require.NoError(t, err)
				}
				writer, buffer := n.Ctx.GetItem("writer").(*base.BitWriter), n.Ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				calls, finalizers := 0, 0
				handled, err := parseWSMP(n, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, n.Children, raw)
					require.NoError(t, r.Backup())
					e := p.Parse(r, raw)
					if outcome == "callback error" {
						e = errors.New("injected callback failure")
					}
					return func(rollback bool) {
						finalizers++
						if rollback {
							n.Ctx.SetItem("pointer", pending)
							buffer.Truncate(0)
							require.NoError(t, writer.Restore(before))
							require.NoError(t, r.Recovery())
						} else {
							require.NoError(t, r.PopBackup())
						}
					}, e
				})
				require.True(t, handled, "test must exercise native bridge")
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finalizers)
				require.Same(t, cfg, n.Cfg)
				if outcome != "commit" {
					require.Error(t, err)
					require.Len(t, n.Children, 2)
					require.Same(t, first, n.Children[0])
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, before, writer.Snapshot())
					require.Equal(t, pending, n.Ctx.GetUint64("pointer"))
					got, e := r.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, e)
					require.Equal(t, input, got)
				} else {
					require.NoError(t, err)
					require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(n))
					require.Equal(t, [2]uint64{pending, pending + 4}, GetNodeResultPos(n.Children[1].Children[0]))
				}
				require.ErrorContains(t, r.Recovery(), "no backup")
				require.ErrorContains(t, r.PopBackup(), "no backup")
			})
		}
	}
}

func TestWSMPNativeModifiedRuleGuard(t *testing.T) {
	for name, modify := range map[string]func(*base.Node){
		"operator":          func(n *base.Node) { n.Cfg.SetItem(CfgOperator, "custom()") },
		"out":               func(n *base.Node) { n.Cfg.SetItem("out", "custom()") },
		"unit":              func(n *base.Node) { n.Cfg.SetItem(CfgUnit, "bit") },
		"endian":            func(n *base.Node) { n.Cfg.SetItem(CfgEndian, "little") },
		"metadata":          func(n *base.Node) { n.Cfg.SetItem("additionInfo", map[string]any{"x": 1}) },
		"alias operator":    func(n *base.Node) { n.Children[1].Cfg.SetItem(CfgOperator, "custom()") },
		"alias length":      func(n *base.Node) { n.Children[1].Cfg.SetItem(CfgLength, uint64(8)) },
		"alias origin":      func(n *base.Node) { n.Children[1].Origin = "raw" },
		"alias context":     func(n *base.Node) { _, other := wsmpBridgeTestNode(t, 8); n.Children[1].Ctx = other.Ctx },
		"definition origin": func(n *base.Node) { n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["WSMPPSID"].Origin = "raw" },
		"definition scalar": func(n *base.Node) {
			n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["WSMPVersion3"].Children[0].Cfg.SetItem(CfgLength, uint64(5))
		},
		"definition out": func(n *base.Node) {
			n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["WSMPLength"].Cfg.SetItem("out", "return 7")
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, n := wsmpBridgeTestNode(t, 8)
			modify(n)
			handled, err := parseWSMP(n, func(*base.Node) (func(bool), error) { t.Fatal("modified schema must not read"); return nil, nil })
			require.NoError(t, err)
			require.False(t, handled)
		})
	}
	for _, mode := range []string{"", GeneratorMode, ParserMode} {
		t.Run("mode-"+mode, func(t *testing.T) {
			_, node := wsmpBridgeTestNode(t, 8)
			if mode == ParserMode {
				node.Ctx.SetItem("wsmpLegacy", true)
			}
			err := ExecOperator(node, `handled, err = tryParseWSMP(); if handled || err != nil { panic("unexpected native parse") }`, func(*base.Node) (func(bool), error) { t.Fatal("mode gate must not read"); return nil, nil }, mode)
			require.NoError(t, err)
		})
	}
}
