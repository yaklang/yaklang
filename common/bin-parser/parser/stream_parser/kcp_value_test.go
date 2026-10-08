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

func TestKCPDecodeExactBoundaries(t *testing.T) {
	valid, err := hex.DecodeString("010203045102341244332211f0ffffffd0c0b0a003000000616263")
	require.NoError(t, err)
	for cut := 0; cut < len(valid); cut++ {
		fields, info, err := decodeKCP(valid[:cut], false)
		require.Error(t, err, "prefix %d", cut)
		require.Nil(t, fields)
		require.Nil(t, info)
	}
	for cmd := 0; cmd < 256; cmd++ {
		wire := bytes.Clone(valid)
		wire[4] = byte(cmd)
		wire[5] = 255 // fragment values are not a grammar discriminator
		fields, info, err := decodeKCP(wire, true)
		if cmd >= 81 && cmd <= 84 {
			require.NoError(t, err)
			require.Len(t, fields, 1)
			require.Len(t, fields[0].Children, 9)
			require.Equal(t, 1, info["Segment Count"])
			require.Equal(t, cmd != 81, fields[0].Info["Receiver Ignores Segment Data"])
		} else {
			require.ErrorContains(t, err, "command")
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	// Maximum-size single payload, maximum possible segment count, and a final
	// truncated segment after thousands of successful headers. Failed plans must
	// never escape, even when a long prefix was already structurally decoded.
	for _, size := range []int{kcpMaxBytes, kcpMaxBytes + 1} {
		wire := append(bytes.Clone(valid[:24]), make([]byte, size-24)...)
		binary.LittleEndian.PutUint32(wire[20:], uint32(size-24))
		fields, info, err := decodeKCP(wire, true)
		if size == kcpMaxBytes {
			require.NoError(t, err)
			require.Equal(t, size, fields[0].End)
		} else {
			require.ErrorContains(t, err, "boundary")
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	empty := bytes.Clone(valid[:24])
	binary.LittleEndian.PutUint32(empty[20:], 0)
	count := kcpMaxBytes / 24
	aggregate := bytes.Repeat(empty, count)
	fields, info, err := decodeKCP(aggregate, false)
	require.NoError(t, err)
	require.Len(t, fields, count)
	require.Equal(t, count, info["Segment Count"])
	for i, f := range fields {
		require.Equal(t, i*24, f.Start)
		require.Equal(t, (i+1)*24, f.End)
		require.Equal(t, f.End, f.Children[8].Start)
		require.Equal(t, f.End, f.Children[8].End)
	}
	for _, wire := range [][]byte{
		append(bytes.Clone(aggregate), 0),
		append(bytes.Clone(valid), valid[:23]...),
		append(bytes.Clone(valid), append([]byte{0}, valid[1:]...)...),
	} {
		fields, info, err := decodeKCP(wire, false)
		require.Error(t, err)
		require.Nil(t, fields)
		require.Nil(t, info)
	}
}

func TestKCPBridgeTransactions(t *testing.T) {
	valid, err := hex.DecodeString("010203045102341244332211f0ffffffd0c0b0a003000000616263")
	require.NoError(t, err)
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					input[4] = 0
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
				err := parseKCP(n, func(raw *base.Node) (func(bool), error) {
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
				}, false)
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
	for _, bits := range []uint64{0, 1, 7, 191, 193, kcpMaxBytes*8 + 1, (kcpMaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseKCP(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, false)
		require.Error(t, err)
	}
}
