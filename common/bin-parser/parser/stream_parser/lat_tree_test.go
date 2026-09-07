package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestLATBridgeTransactions(t *testing.T) {
	// One Data_a slot with an odd body and deliberately nonzero alignment byte.
	valid := []byte{1, 1, 0x34, 0x12, 0x78, 0x56, 9, 8, 1, 2, 1, 0, 0xab, 0xcd}
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "invalid header", "late invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("offset-%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				switch outcome {
				case "invalid header":
					input[0] = 40
				case "late invalid":
					input[11] = 0xff
				case "short":
					input = input[:len(input)-1]
				}
				root := giopBridgeInlineRoot(t, "endian: big\nunit: byte\nPackage:\n  Message: {}\n")
				root.Children[0].Children[0].Cfg.SetItem(CfgIsTerminal, false)
				require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
				root.Cfg.SetItem(CfgLength, pending+uint64(len(valid))*8)
				p := &DefParser{}
				require.NoError(t, p.OnRoot(root))
				n := root.Children[0].Children[0]
				n.Cfg.SetItem(CfgLength, uint64(len(valid))*8)
				n.Cfg.SetItem("caller-marker", "unchanged")
				alias := &base.Node{Name: "existing alias", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
				alias.Cfg.SetItem(CfgParent, n)
				n.Children = []*base.Node{alias}
				cfg, ctx := n.Cfg, n.Ctx
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
				reader := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if pending > 0 {
					_, err := reader.ReadBits(pending)
					require.NoError(t, err)
				}
				writer := n.Ctx.GetItem("writer").(*base.BitWriter)
				buffer := n.Ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				beforeBuffer := bytes.Clone(buffer.Bytes())
				calls, finishes := 0, 0
				err := parseLATMessage(n, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, n.Children, raw)
					require.Same(t, n, raw.Cfg.GetItem(CfgParent))
					require.NoError(t, reader.Backup())
					position, state := ctx.GetUint64("pointer"), writer.Snapshot()
					e := p.Parse(reader, raw)
					if outcome == "callback" {
						e = errors.New("injected callback failure")
					}
					return func(rollback bool) {
						finishes++
						if rollback {
							ctx.SetItem("pointer", position)
							buffer.Truncate(int(position / 8))
							require.NoError(t, writer.Restore(state))
							require.NoError(t, reader.Recovery())
						} else {
							require.NoError(t, reader.PopBackup())
						}
					}, e
				})
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finishes)
				require.Same(t, cfg, n.Cfg)
				require.Same(t, ctx, n.Ctx)
				require.Equal(t, "unchanged", n.Cfg.GetItem("caller-marker"))
				if outcome == "commit" {
					require.NoError(t, err)
					giopBridgeAssertTree(t, n, pending, pending+uint64(len(valid))*8)
					value, e := giopBridgeFind(t, n, "Destination Circuit ID").Result()
					require.NoError(t, e)
					require.Equal(t, uint16(0x1234), value.Value)
					require.Equal(t, pending+uint64(len(valid))*8, ctx.GetUint64("pointer"))
				} else {
					require.Error(t, err)
					require.Len(t, n.Children, 1)
					require.Same(t, alias, n.Children[0])
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, pending, ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					require.True(t, bytes.Equal(beforeBuffer, buffer.Bytes()))
					got, e := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, e)
					require.Equal(t, input, got)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
			})
		}
	}
}

func TestLATBridgeBoundaryBeforeRead(t *testing.T) {
	for _, bits := range []uint64{0, 1, 63, 65, 1500*8 + 1, 1501 * 8} {
		root := giopBridgeInlineRoot(t, "endian: little\nunit: byte\nPackage:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseLATMessage(n, func(*base.Node) (func(bool), error) {
			t.Fatal("invalid boundary caused a read")
			return nil, nil
		})
		require.Error(t, err)
	}
}

func TestLATDecoderDeterministicBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(5023))
	var check func([]latField, int, int)
	check = func(fields []latField, start, end int) {
		position := start
		for _, field := range fields {
			require.Equal(t, position, field.Start, field.Name)
			require.GreaterOrEqual(t, field.End, field.Start, field.Name)
			require.LessOrEqual(t, field.End, end, field.Name)
			if field.Type == "" {
				check(field.Children, field.Start, field.End)
			}
			position = field.End
		}
		require.Equal(t, end, position)
	}
	for size := 0; size <= 1501; size++ {
		wire := make([]byte, size)
		_, err := rng.Read(wire)
		require.NoError(t, err)
		if size >= 8 {
			copy(wire[:8], []byte{1, 0, 0x34, 0x12, 0x78, 0x56, 0, 0})
			wire[0] = byte(size % 3 * 4)
			wire[1] = byte(size % 256)
		}
		fields, err := decodeLATMessage(wire)
		if err != nil {
			require.Nil(t, fields)
		} else {
			check(fields, 0, size*8)
		}
	}
}
