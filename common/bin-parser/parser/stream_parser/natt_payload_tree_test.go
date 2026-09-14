package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func nattNativeTestNode(t testing.TB, size, major, typ int, unit ...string) (*DefParser, *base.Node) {
	t.Helper()
	root, err := base.ParseRule("nat_t.yaml")
	require.NoError(t, err)
	p := &DefParser{}
	require.NoError(t, p.OnRoot(root))
	anchor := root.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["Package"]
	if len(unit) != 0 {
		anchor.Cfg.SetItem(CfgUnit, unit[0])
	}
	n, err := NewNodeByType(anchor, "NATTIKEPayloads")
	require.NoError(t, err)
	n.Name = "Payloads"
	n.Cfg.SetItem(CfgLength, uint64(size)*8)
	n.Ctx.SetItem("natt_ike_major", major)
	n.Ctx.SetItem("natt_payload_type", typ)
	n.Ctx.SetItem("natt_payload_index", 777)
	return p, n
}

func nattNativeTestParse(t testing.TB, p *DefParser, n *base.Node, body []byte) {
	t.Helper()
	r := base.NewBitReader(bytes.NewReader(body))
	handled, err := parseNATTPayloads(n, func(raw *base.Node) (func(bool), error) {
		require.NoError(t, r.Backup())
		return func(rollback bool) {
			require.False(t, rollback)
			require.NoError(t, r.PopBackup())
		}, p.Parse(r, raw)
	})
	require.NoError(t, err)
	require.True(t, handled, "must exercise the native path")
	require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(n))
	require.ErrorContains(t, r.Recovery(), "no backup")
}

func TestNATTPayloadBridgeFields(t *testing.T) {
	for _, unit := range [][]string{nil, {"byte"}} {
		for _, tc := range nattNativeCases() {
			t.Run(fmt.Sprintf("%v/%s", unit, tc.name), func(t *testing.T) {
				p, n := nattNativeTestNode(t, len(tc.body), tc.major, tc.typ, unit...)
				nattNativeTestParse(t, p, n, tc.body)
				require.Len(t, n.Children, 1)
				require.Equal(t, map[string]any{"Payload Count": 1}, n.Cfg.GetItem("additionInfo"))
				require.Equal(t, 0, n.Ctx.GetItem("natt_payload_type"))
				require.Equal(t, 0, n.Ctx.GetItem("natt_payload_index"))
				element := n.Children[0]
				require.Same(t, n, element.Cfg.GetItem(CfgParent))
				require.False(t, element.Cfg.Has(CfgElementIndex))
				require.Equal(t, uint64(len(tc.body))*8, element.Cfg.GetItem(CfgLength))
				reserved := int(tc.body[1] & 127)
				if tc.major == 1 {
					reserved = int(tc.body[1])
				}
				info := map[string]any{"Payload Type": tc.typ, "Critical": tc.major == 2 && tc.body[1]&128 != 0,
					"Reserved Flags": reserved, "Body Layout": tc.layout, "Body Semantics Decoded": false}
				if tc.inner >= 0 {
					info["Inner Next Payload"] = tc.inner
					info["Inner Payloads Decoded"] = false
				}
				require.Equal(t, info, element.Cfg.GetItem("additionInfo"))
				spans := map[int][2]int{nattNextPayload: {0, 1}, nattPayloadFlags: {1, 2}, nattPayloadLength: {2, 4}}
				for index, span := range tc.spans {
					spans[index] = span
				}
				require.Len(t, element.Children, len(nattPayloadFieldNames))
				covered := make([]bool, len(tc.body))
				for index, field := range element.Children {
					require.Equal(t, nattPayloadFieldNames[index], field.Name)
					require.Same(t, element, field.Cfg.GetItem(CfgParent))
					require.Same(t, n.Ctx, field.Ctx)
					require.Equal(t, index == len(element.Children)-1, field.Cfg.GetBool(CfgLastNode))
					if len(unit) > 0 {
						require.Equal(t, unit[0], field.Cfg.GetItem(CfgUnit))
					}
					span, processed := spans[index]
					require.Equal(t, processed, field.Cfg.Has(CfgNodeResult), field.Name)
					if !processed {
						if field.Cfg.GetString(CfgType) == "raw" {
							require.False(t, field.Cfg.Has(CfgLength), field.Name)
						}
						continue
					}
					require.Equal(t, [2]uint64{uint64(span[0]) * 8, uint64(span[1]) * 8}, GetNodeResultPos(field), field.Name)
					got, err := getNodeResult(field, true)
					require.NoError(t, err)
					require.Equal(t, tc.body[span[0]:span[1]], got, field.Name)
					value, err := p.Result(field)
					require.NoError(t, err)
					require.Same(t, field, value.Origin)
					switch field.Cfg.GetString(CfgType) {
					case "uint8":
						require.IsType(t, uint8(0), value.Value)
					case "uint16":
						require.IsType(t, uint16(0), value.Value)
					case "uint32":
						require.IsType(t, uint32(0), value.Value)
					case "raw":
						require.IsType(t, []byte{}, value.Value)
					}
					for pos := span[0]; pos < span[1]; pos++ {
						require.False(t, covered[pos], "overlapping field at %d", pos)
						covered[pos] = true
					}
				}
				for pos, yes := range covered {
					require.True(t, yes, "unrepresented byte %d", pos)
				}
				template := n.Cfg.GetItem("template").(*base.Node)
				require.Same(t, n, template.Cfg.GetItem(CfgParent))
				require.Equal(t, template.Origin, element.Origin)
				for _, field := range template.Children {
					require.False(t, field.Cfg.Has(CfgNodeResult))
				}
			})
		}
	}
}

func TestNATTPayloadBridgeTransaction(t *testing.T) {
	body := append(nattNativePayload(53, 0xff, []byte{0xde, 0xad}), nattNativePayload(40, 0, []byte{0, 1, 0, 2, 0xee})...)
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid tail", "short read", "callback error"} {
			t.Run(fmt.Sprintf("bits-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(body)
				if outcome == "invalid tail" {
					input[11] = 3 // fragment 3 exceeds total 2, after a valid first record
				}
				if outcome == "short read" {
					input = input[:len(input)-1]
				}
				p, n := nattNativeTestNode(t, len(body), 2, 250)
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
				handled, err := parseNATTPayloads(n, func(raw *base.Node) (func(bool), error) {
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
				require.True(t, handled)
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finalizers)
				require.Same(t, cfg, n.Cfg)
				require.Equal(t, 2, n.Ctx.GetItem("natt_ike_major"))
				if outcome != "commit" {
					require.Error(t, err)
					require.Len(t, n.Children, 1)
					require.Same(t, alias, n.Children[0])
					require.False(t, n.Cfg.Has("template"))
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, 250, n.Ctx.GetItem("natt_payload_type"))
					require.Equal(t, 777, n.Ctx.GetItem("natt_payload_index"))
					require.Equal(t, pending, n.Ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					got, e := r.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, e)
					require.Equal(t, input, got)
				} else {
					require.NoError(t, err)
					require.Len(t, n.Children, 2)
					require.Equal(t, map[string]any{"Payload Count": 2}, n.Cfg.GetItem("additionInfo"))
					require.Equal(t, uint64(len(body))*8, CalcNodeConsumedLength(n))
					require.Equal(t, 0, n.Ctx.GetItem("natt_payload_type"))
					require.Equal(t, 1, n.Ctx.GetItem("natt_payload_index"))
					require.Equal(t, [2]uint64{pending, pending + 8}, GetNodeResultPos(n.Children[0].Children[0]))
					require.Equal(t, [2]uint64{pending + 48, pending + 56}, GetNodeResultPos(n.Children[1].Children[0]))
					require.Equal(t, [2]uint64{pending + 112, pending + 120}, GetNodeResultPos(n.Children[1].Children[nattEncryptedPayloadData]))
					// Check the original terminal Result ranges at every pending
					// bit position, not the generic struct raw-byte convenience
					// view (which rounds its start down to a byte).
					var got []byte
					for _, element := range n.Children {
						for _, field := range element.Children {
							if !NodeHasResult(field) {
								continue
							}
							value, e := getNodeResult(field, true)
							require.NoError(t, e)
							got = append(got, value.([]byte)...)
						}
					}
					require.Equal(t, body, got)
				}
				require.ErrorContains(t, r.Recovery(), "no backup")
				require.ErrorContains(t, r.PopBackup(), "no backup")
			})
		}
	}
}

func TestNATTPayloadBridgePadding(t *testing.T) {
	for count := 1; count <= 3; count++ {
		body := append(nattNativePayload(0, 0, bytes.Repeat([]byte{0x34}, 4-count)), bytes.Repeat([]byte{0xab}, count)...)
		p, n := nattNativeTestNode(t, len(body), 1, 250, "byte")
		nattNativeTestParse(t, p, n, body)
		require.Len(t, n.Children, 2)
		require.Equal(t, map[string]any{"Payload Count": 1}, n.Cfg.GetItem("additionInfo"))
		require.Equal(t, 0, n.Ctx.GetItem("natt_payload_index"))
		padding := n.Children[1]
		require.Equal(t, "ISAKMP Padding", padding.Name)
		require.Equal(t, "raw", padding.Origin)
		require.Same(t, n, padding.Cfg.GetItem(CfgParent))
		require.Same(t, n.Ctx, padding.Ctx)
		require.Equal(t, "byte", padding.Cfg.GetItem(CfgUnit))
		require.False(t, padding.Cfg.Has(CfgElementIndex))
		require.Equal(t, [2]uint64{uint64(len(body)-count) * 8, uint64(len(body)) * 8}, GetNodeResultPos(padding))
		value, err := p.Result(padding)
		require.NoError(t, err)
		require.Same(t, padding, value.Origin)
		require.Equal(t, bytes.Repeat([]byte{0xab}, count), value.Value)
	}
}

func TestNATTPayloadBridgeCustomGuards(t *testing.T) {
	for name, modify := range map[string]func(*base.Node){
		"terminal list":       func(n *base.Node) { n.Cfg.SetItem(CfgIsTerminal, true) },
		"alias type":          func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgType, "raw") },
		"alias child":         func(n *base.Node) { n.Children[0].Children = []*base.Node{{Name: "custom"}} },
		"alias context":       func(n *base.Node) { n.Children[0].Ctx = &base.NodeContext{} },
		"alias parent":        func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgParent, nil) },
		"list unit":           func(n *base.Node) { n.Cfg.SetItem(CfgUnit, "bit") },
		"list operator":       func(n *base.Node) { n.Cfg.SetItem(CfgOperator, "custom()") },
		"list existing info":  func(n *base.Node) { n.Cfg.SetItem("additionInfo", map[string]any{"x": 1}) },
		"existing template":   func(n *base.Node) { n.Cfg.SetItem("template", n.Children[0]) },
		"list cache":          func(n *base.Node) { n.Cfg.SetItem(CfgLengthCacheMap, map[string]any{}) },
		"list delimiter":      func(n *base.Node) { n.Cfg.SetItem(CfgDelimiter, "custom") },
		"alias operator":      func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgOperator, "custom()") },
		"alias out":           func(n *base.Node) { n.Children[0].Cfg.SetItem("out", "custom()") },
		"list out":            func(n *base.Node) { n.Cfg.SetItem("out", "custom()") },
		"alias endian":        func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgEndian, "little") },
		"alias length":        func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgLength, uint64(32)) },
		"alias info":          func(n *base.Node) { n.Children[0].Cfg.SetItem("additionInfo", map[string]any{"x": 1}) },
		"alias unit":          func(n *base.Node) { n.Children[0].Cfg.SetItem(CfgUnit, "byte") },
		"definition":          func(n *base.Node) { n.Ctx.GetItem(CfgRootMap).(map[string]*base.Node)["NATTIKEPayload"].Origin = "raw" },
		"zero":                func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(0)) },
		"bit-bound":           func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(31)) },
		"oversize":            func(n *base.Node) { n.Cfg.SetItem(CfgLength, uint64(65528*8)) },
		"missing-major":       func(n *base.Node) { n.Ctx.DeleteItem("natt_ike_major") },
		"custom-major":        func(n *base.Node) { n.Ctx.SetItem("natt_ike_major", 3) },
		"negative-type":       func(n *base.Node) { n.Ctx.SetItem("natt_payload_type", -1) },
		"oversize-type":       func(n *base.Node) { n.Ctx.SetItem("natt_payload_type", 256) },
		"custom-type-context": func(n *base.Node) { n.Ctx.SetItem("natt_payload_type", "250") },
	} {
		t.Run(name, func(t *testing.T) {
			_, n := nattNativeTestNode(t, 4, 2, 250)
			modify(n)
			alias := n.Children[0]
			handled, err := parseNATTPayloads(n, func(*base.Node) (func(bool), error) { t.Fatal("custom schema must not read"); return nil, nil })
			require.NoError(t, err)
			require.False(t, handled)
			require.Same(t, alias, n.Children[0])
		})
	}
}

func TestNATTPayloadBridgeModes(t *testing.T) {
	for _, mode := range []string{"", GeneratorMode, ParserMode} {
		t.Run(mode, func(t *testing.T) {
			_, n := nattNativeTestNode(t, 4, 2, 250)
			if mode == ParserMode {
				n.Ctx.SetItem("nattPayloadsLegacy", true)
			}
			var modes []string
			if mode != "" {
				modes = append(modes, mode)
			}
			require.NoError(t, ExecOperator(n, `handled, err = tryParseNATTPayloads(); if err != nil { panic(err) }; if handled { panic("unexpected native") }`, func(*base.Node) (func(bool), error) {
				t.Fatal("non-parser or explicit legacy mode must not read")
				return nil, nil
			}, modes...))
		})
	}
}

func TestNATTPayloadBridgeConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tc := nattNativeCases()[i%len(nattNativeCases())]
			for j := 0; j < 4; j++ {
				p, n := nattNativeTestNode(t, len(tc.body), tc.major, tc.typ)
				nattNativeTestParse(t, p, n, tc.body)
				require.Equal(t, tc.typ, n.Children[0].Cfg.GetItem("additionInfo").(map[string]any)["Payload Type"])
				n.Children[0].Children[0].Name = fmt.Sprintf("private-%d-%d", i, j)
				require.Equal(t, "Next Payload", n.Cfg.GetItem("template").(*base.Node).Children[0].Name)
			}
		}(i)
	}
	wg.Wait()
}

func TestNATTPayloadBridgeResourceBound(t *testing.T) {
	body := nattNativeChain(4096)
	p, n := nattNativeTestNode(t, len(body), 2, 250)
	nattNativeTestParse(t, p, n, body)
	require.Len(t, n.Children, 4096)
	require.Equal(t, 4095, n.Ctx.GetItem("natt_payload_index"))
	require.Equal(t, map[string]any{"Payload Count": 4096}, n.Cfg.GetItem("additionInfo"))
	require.Equal(t, [2]uint64{4095 * 32, 4095*32 + 8}, GetNodeResultPos(n.Children[4095].Children[0]))
}

func BenchmarkNATTPayloadBridge(b *testing.B) {
	for _, count := range []int{16, 256, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			body := nattNativeChain(count)
			// Measure staged wire validation and complete result-tree creation;
			// exclude the caller's rule-loading and initial root setup.
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				p, n := nattNativeTestNode(b, len(body), 2, 250)
				r := base.NewBitReader(bytes.NewReader(body))
				b.StartTimer()
				handled, err := parseNATTPayloads(n, func(raw *base.Node) (func(bool), error) { return nil, p.Parse(r, raw) })
				if !handled || err != nil {
					b.Fatalf("handled=%v err=%v", handled, err)
				}
			}
		})
	}
}
