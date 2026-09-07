package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestTLS12ControlHandshakeValues(t *testing.T) {
	for _, typ := range []byte{4, 14} {
		wire := []byte{14, 0, 0, 0}
		if typ == 4 {
			wire = []byte{4, 0, 0, 6, 0, 0, 0, 0, 0, 0}
		}
		fields, info, err := decodeTLS12ControlHandshake(wire, typ)
		require.NoError(t, err)
		require.Equal(t, tls12ControlField{"Handshake Length", "uint32", 1, 4}, fields[1])
		require.Equal(t, false, info["Handshake Completion Validated"])
		if typ == 4 {
			require.Equal(t, tls12ControlField{"Ticket", "raw", 10, 10}, fields[4])
			require.Equal(t, true, info["Lifetime Hint Unspecified"])
		}
		for cut := 0; cut < len(wire); cut++ {
			fields, info, err := decodeTLS12ControlHandshake(wire[:cut], typ)
			require.Error(t, err)
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	fields, info, err := decodeTLS12ControlHandshake([]byte{14, 0, 0, 0}, 2)
	require.Error(t, err)
	require.Nil(t, fields)
	require.Nil(t, info)
}

func TestTLS12ControlHandshakeBridgeTransactions(t *testing.T) {
	for _, typ := range []byte{4, 14} {
		valid := []byte{14, 0, 0, 0}
		if typ == 4 {
			valid = []byte{4, 0, 0, 8, 1, 2, 3, 4, 0, 2, 0xaa, 0xbb}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
				t.Run(fmt.Sprintf("%d/%d/%s", typ, offset, outcome), func(t *testing.T) {
					input := bytes.Clone(valid)
					if outcome == "invalid" {
						input[3]++
					} else if outcome == "short" {
						input = input[:len(input)-1]
					}
					root := giopBridgeInlineRoot(t, "endian: big\nunit: byte\nPackage:\n  Message: {}\n")
					n := root.Children[0].Children[0]
					n.Cfg.SetItem(CfgIsTerminal, false)
					require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
					root.Cfg.SetItem(CfgLength, offset+uint64(len(valid))*8)
					p := &DefParser{}
					require.NoError(t, p.OnRoot(root))
					n.Cfg.SetItem(CfgLength, uint64(len(valid))*8)
					n.Cfg.SetItem("additionInfo", map[string]any{"caller": true})
					alias := &base.Node{Name: "existing", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
					alias.Cfg.SetItem(CfgParent, n)
					n.Children = []*base.Node{alias}
					cfg, ctx := n.Cfg, n.Ctx
					var wire bytes.Buffer
					w := base.NewBitWriter(&wire)
					if offset != 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
						_, err := p.write([]byte{0x55}, offset)
						require.NoError(t, err)
					}
					require.NoError(t, w.WriteBits(input, uint64(len(input))*8))
					if offset != 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					r := base.NewBitReader(bytes.NewReader(wire.Bytes()))
					if offset != 0 {
						_, err := r.ReadBits(offset)
						require.NoError(t, err)
					}
					writer, buffer := ctx.GetItem("writer").(*base.BitWriter), ctx.GetItem("buffer").(*bytes.Buffer)
					before, beforeBuffer := writer.Snapshot(), bytes.Clone(buffer.Bytes())
					calls, finishes := 0, 0
					err := parseTLS12ControlHandshake(n, func(raw *base.Node) (func(bool), error) {
						calls++
						require.NotContains(t, n.Children, raw)
						require.Same(t, n, raw.Cfg.GetItem(CfgParent))
						require.NoError(t, r.Backup())
						position, state := ctx.GetUint64("pointer"), writer.Snapshot()
						err := p.Parse(r, raw)
						if outcome == "callback" {
							err = errors.New("injected callback failure")
						}
						return func(rollback bool) {
							finishes++
							if rollback {
								ctx.SetItem("pointer", position)
								buffer.Truncate(int(position / 8))
								require.NoError(t, writer.Restore(state))
								require.NoError(t, r.Recovery())
							} else {
								require.NoError(t, r.PopBackup())
							}
						}, err
					}, typ)
					require.Equal(t, 1, calls)
					require.Equal(t, 1, finishes)
					require.Same(t, cfg, n.Cfg)
					require.Same(t, ctx, n.Ctx)
					if outcome == "commit" {
						require.NoError(t, err)
						giopBridgeAssertTree(t, n, offset, offset+uint64(len(valid))*8)
						require.Equal(t, offset+uint64(len(valid))*8, ctx.GetUint64("pointer"))
					} else {
						require.Error(t, err)
						require.Len(t, n.Children, 1)
						require.Same(t, alias, n.Children[0])
						require.Equal(t, map[string]any{"caller": true}, n.Cfg.GetItem("additionInfo"))
						require.Equal(t, offset, ctx.GetUint64("pointer"))
						require.Equal(t, before, writer.Snapshot())
						require.True(t, bytes.Equal(beforeBuffer, buffer.Bytes()))
						got, err := r.ReadBits(uint64(len(input)) * 8)
						require.NoError(t, err)
						require.Equal(t, input, got)
					}
					require.ErrorContains(t, r.PopBackup(), "no backup")
				})
			}
		}
	}
	for _, bits := range []uint64{0, 1, 7, 9, 16, 31, tls12ControlMaxBytes*8 + 1, (tls12ControlMaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseTLS12ControlHandshake(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, 4)
		require.Error(t, err)
	}
}
