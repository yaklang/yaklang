package stream_parser

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestXTPBridgeTransactions(t *testing.T) {
	valid, err := hex.DecodeString("8001020304050607000000200000000523b512340102030401020304050607086162636465")
	require.NoError(t, err)
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					input[len(input)-1] = '!'
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
				writer := ctx.GetItem("writer").(*base.BitWriter)
				buffer := ctx.GetItem("buffer").(*bytes.Buffer)
				before, beforeBuffer := writer.Snapshot(), bytes.Clone(buffer.Bytes())
				calls, finishes := 0, 0
				err := parseXTPMessage(n, func(raw *base.Node) (func(bool), error) {
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
				})
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

func TestXTPBridgeBoundaries(t *testing.T) {
	for _, bits := range []uint64{0, 1, 7, 9, 255, 65535*8 + 1, 65536 * 8} {
		root := giopBridgeInlineRoot(t, "endian: big\nunit: byte\nPackage:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseXTPMessage(n, func(*base.Node) (func(bool), error) {
			t.Fatal("invalid boundary read input")
			return nil, nil
		})
		require.Error(t, err)
	}
}

func TestXTPChecksumIndependentVectors(t *testing.T) {
	// RFC 1071 section 3 numerical example, not produced by this decoder.
	wire := []byte{0, 1, 0xf2, 3, 0xf4, 0xf5, 0xf6, 0xf7}
	require.Equal(t, uint16(0x220d), xtpChecksum(wire))
	require.Equal(t, uint16(0xfffe), xtpChecksum([]byte{0, 1}))
	require.Equal(t, uint16(0xfeff), xtpChecksum([]byte{1}))
	require.Equal(t, uint16(0xffff), xtpChecksum(nil))
	fields, err := decodeXTPMessage(make([]byte, 32))
	require.ErrorContains(t, err, "unsupported version")
	require.Nil(t, fields)
}
