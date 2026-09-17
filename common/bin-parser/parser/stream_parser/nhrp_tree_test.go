package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func nhrpBridgeTestNode(t *testing.T, size int, unit ...string) (*DefParser, *base.Node) {
	t.Helper()
	root, err := base.ParseRule("nhrp.yaml")
	require.NoError(t, err)
	p := &DefParser{}
	require.NoError(t, p.OnRoot(root))
	anchor := root.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["Package"]
	if len(unit) > 0 {
		anchor.Cfg.SetItem(CfgUnit, unit[0])
	}
	n, err := NewNodeByType(anchor, "NHRPClients")
	require.NoError(t, err)
	n.Name = "Clients"
	n.Cfg.SetItem(CfgLength, uint64(size)*8)
	return p, n
}

func TestNHRPClientBridgeTransaction(t *testing.T) {
	body := append(nhrpValueTestClient(0x40, 0x40, 0), nhrpValueTestClient(4, 3, 5)...)
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid tail", "short read", "callback error"} {
			t.Run(fmt.Sprintf("bits-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(body)
				if outcome == "invalid tail" {
					input[20] |= 128
				}
				if outcome == "short read" {
					input = input[:len(input)-1]
				}
				p, n := nhrpBridgeTestNode(t, len(body))
				cfg, alias := n.Cfg, n.Children[0]
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
				writer := n.Ctx.GetItem("writer").(*base.BitWriter)
				buffer := n.Ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				calls, finalizers := 0, 0
				handled, err := parseNHRPClients(n, func(raw *base.Node) (func(bool), error) {
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
				require.True(t, handled, "must actually take the native path")
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finalizers)
				require.Same(t, cfg, n.Cfg)
				if outcome != "commit" {
					require.Error(t, err)
					require.Len(t, n.Children, 1)
					require.Same(t, alias, n.Children[0])
					require.False(t, n.Cfg.Has("template"))
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, pending, n.Ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					got, e := r.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, e)
					require.Equal(t, input, got)
				} else {
					require.NoError(t, err)
					require.Len(t, n.Children, 2)
					require.Equal(t, map[string]any{"Client Count": 2, "First Prefix Length": 29}, n.Cfg.GetItem("additionInfo"))
					require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(n))
					for i, element := range n.Children {
						require.Same(t, n, element.Cfg.GetItem(CfgParent))
						require.False(t, element.Cfg.Has(CfgLength))
						require.Equal(t, [2]uint64{pending + uint64(i)*96, pending + uint64(i)*96 + 8}, GetNodeResultPos(element.Children[0]))
						for _, field := range element.Children {
							require.Same(t, element, field.Cfg.GetItem(CfgParent))
							require.Same(t, n.Ctx, field.Ctx)
						}
					}
					for _, field := range n.Children[0].Children[9:] {
						require.False(t, field.Cfg.Has(CfgLength))
						require.False(t, field.Cfg.Has(CfgNodeResult))
					}
					require.Same(t, n, n.Cfg.GetItem("template").(*base.Node).Cfg.GetItem(CfgParent))
				}
				require.ErrorContains(t, r.Recovery(), "no backup")
				require.ErrorContains(t, r.PopBackup(), "no backup")
			})
		}
	}
}

func TestNHRPClientBridgeCustomDefinitionsAndModes(t *testing.T) {
	for name, modify := range map[string]func(*base.Node){
		"list unit":          func(n *base.Node) { n.Cfg.SetItem(CfgUnit, "bit") },
		"list operator":      func(n *base.Node) { n.Cfg.SetItem(CfgOperator, "custom()") },
		"list existing info": func(n *base.Node) { n.Cfg.SetItem("additionInfo", map[string]any{"x": 1}) },
		"existing template":  func(n *base.Node) { n.Cfg.SetItem("template", n.Children[0]) },
		"alias operator":     func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgOperator, "custom()") },
		"alias out":          func(n *base.Node) { n.Children[0].Cfg.SetItem("out", "custom()") },
		"list out":           func(n *base.Node) { n.Cfg.SetItem("out", "custom()") },
		"alias endian":       func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgEndian, "little") },
		"alias length":       func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgLength, uint64(96)) },
		"definition":         func(n *base.Node) { n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["NHRPClient"].Origin = "raw" },
		"zero":               func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(0)) },
		"oversize":           func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(65536*8)) },
	} {
		t.Run(name, func(t *testing.T) {
			_, n := nhrpBridgeTestNode(t, 12)
			modify(n)
			alias := n.Children[0]
			handled, err := parseNHRPClients(n, func(*base.Node) (func(bool), error) { t.Fatal("custom schema must not read"); return nil, nil })
			require.NoError(t, err)
			require.False(t, handled)
			require.Same(t, alias, n.Children[0])
		})
	}
	for _, mode := range []string{"", GeneratorMode, ParserMode} {
		t.Run("mode-"+mode, func(t *testing.T) {
			_, n := nhrpBridgeTestNode(t, 12)
			if mode == ParserMode {
				n.Ctx.SetItem("nhrpClientsLegacy", true)
			}
			var modes []string
			if mode != "" {
				modes = append(modes, mode)
			}
			require.NoError(t, ExecOperator(n, `handled, err = tryParseNHRPClients(); if err != nil { panic(err) }; if handled { panic("unexpected native") }`, func(*base.Node) (func(bool), error) { t.Fatal("mode must not read"); return nil, nil }, modes...))
		})
	}
}

func TestNHRPClientBridgeImportedByteUnit(t *testing.T) {
	body := nhrpValueTestClient(3, 2, 5)
	p, n := nhrpBridgeTestNode(t, len(body), "byte")
	r := base.NewBitReader(bytes.NewReader(body))
	handled, err := parseNHRPClients(n, func(raw *base.Node) (func(bool), error) {
		require.NoError(t, r.Backup())
		return func(rollback bool) { require.False(t, rollback); require.NoError(t, r.PopBackup()) }, p.Parse(r, raw)
	})
	require.NoError(t, err)
	require.True(t, handled, "the explicit byte-unit shape inherited by transport imports must use native parsing")
	for _, field := range n.Children[0].Children {
		require.Equal(t, "byte", field.Cfg.GetItem(CfgUnit))
	}
	require.ErrorContains(t, r.Recovery(), "no backup")
}
