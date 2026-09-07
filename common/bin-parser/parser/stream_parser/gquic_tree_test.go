package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestGQUIC35HashAndResourceBounds(t *testing.T) {
	for _, tc := range []struct{ header, body, hash string }{
		{"", "", "8dc595627521b8624201bb07"},
		{"he", "llo", "b3319d594b3181704fd98342"},
		{"header", "body", "8c1f480798c0b9ebcfc7497b"},
	} {
		hash := gquic35NullHash([]byte(tc.header), []byte(tc.body))
		require.Equal(t, tc.hash, hex.EncodeToString(hash[:]))
	}
	for _, count := range []int{128, 129} {
		chlo := make([]byte, 8+count*8)
		copy(chlo, "CHLO")
		binary.LittleEndian.PutUint16(chlo[4:], uint16(count))
		for i := 0; i < count; i++ {
			// Unknown tags, including zero-length values, are preserved. They
			// cannot be declared understood just because their framing is valid.
			binary.LittleEndian.PutUint32(chlo[8+i*8:], uint32(i))
		}
		header := []byte{0, 0} // wire 0 is not an inferred absolute packet number
		body := append([]byte{0x80, 1}, chlo...)
		hash := gquic35NullHash(header, body)
		wire := append(append(bytes.Clone(header), hash[:]...), body...)
		fields, info, err := decodeGQUIC35(wire, false, true)
		if count == 128 {
			require.NoError(t, err)
			require.Equal(t, count, info["CHLO Entry Count"])
			values := fields[2].Children[2].Children[4].Children
			require.Len(t, values, count)
			for _, value := range values {
				require.Equal(t, "raw", value.Type)
				require.Equal(t, value.Start, value.End)
				require.Equal(t, false, value.Info["Value Layout Decoded"])
			}
		} else {
			require.ErrorContains(t, err, "128-entry")
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
}

func TestGQUIC35BridgeTransactions(t *testing.T) {
	gquic35TestBridgeTransactions(t, false)
}

func TestGQUIC35OptionsBridgeTransactions(t *testing.T) {
	gquic35TestBridgeTransactions(t, true)
}

func gquic35TestBridgeTransactions(t *testing.T, options bool) {
	valid := []byte{0, 1, 0x42}
	if options {
		valid = gquic35TestTypedBridgePacket(false)
	}
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					if options {
						input = gquic35TestTypedBridgePacket(true)
					} else {
						input[0] = 0x80
					}
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
				err := parseGQUIC35(n, func(raw *base.Node) (func(bool), error) {
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
				}, false, options)
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
	for _, bits := range []uint64{0, 1, 7, 9, 16, gquic35MaxBytes*8 + 1, (gquic35MaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseGQUIC35(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, false, options)
		require.Error(t, err)
	}
}
