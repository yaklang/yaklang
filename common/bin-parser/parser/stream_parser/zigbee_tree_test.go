package stream_parser

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestZigbeeNativeTransactionFailures(t *testing.T) {
	valid, e := hex.DecodeString("4188073412785600000800785600001e09410f0907")
	require.NoError(t, e)
	for pending := uint64(0); pending < 8; pending++ {
		for _, outcome := range []string{"commit", "late", "physical", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", pending, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "late" {
					input[len(input)-2] = 8
				} // request-key now requires a valid key type/partner
				if outcome == "physical" {
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
				n.Cfg.SetItem("caller-marker", "held")
				alias := &base.Node{Name: "existing alias", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
				alias.Cfg.SetItem(CfgParent, n)
				n.Children = []*base.Node{alias}
				cfg, ctx := n.Cfg, n.Ctx
				var wire bytes.Buffer
				ww := base.NewBitWriter(&wire)
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0x55}, pending))
					_, e = p.write([]byte{0x55}, pending)
					require.NoError(t, e)
				}
				require.NoError(t, ww.WriteBits(input, uint64(len(input))*8))
				if pending > 0 {
					require.NoError(t, ww.WriteBits([]byte{0}, 8-pending))
				}
				reader := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if pending > 0 {
					_, e = reader.ReadBits(pending)
					require.NoError(t, e)
				}
				writer := ctx.GetItem("writer").(*base.BitWriter)
				buffer := ctx.GetItem("buffer").(*bytes.Buffer)
				before := writer.Snapshot()
				beforeBuffer := bytes.Clone(buffer.Bytes())
				calls, finishes := 0, 0
				e = parseZigbeeFrame(n, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, n.Children, raw)
					require.Same(t, n, raw.Cfg.GetItem(CfgParent))
					require.NoError(t, reader.Backup())
					position, state := ctx.GetUint64("pointer"), writer.Snapshot()
					err := p.Parse(reader, raw)
					if outcome == "callback" {
						err = errors.New("injected")
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
					}, err
				}, false)
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finishes)
				require.Same(t, cfg, n.Cfg)
				require.Same(t, ctx, n.Ctx)
				require.Equal(t, "held", n.Cfg.GetItem("caller-marker"))
				if outcome == "commit" {
					require.NoError(t, e)
					require.Equal(t, pending+uint64(len(valid))*8, ctx.GetUint64("pointer"))
					giopBridgeAssertTree(t, n, pending, pending+uint64(len(valid))*8)
				} else {
					require.Error(t, e)
					require.Len(t, n.Children, 1)
					require.Same(t, alias, n.Children[0])
					require.False(t, n.Cfg.Has("additionInfo"))
					require.Equal(t, pending, ctx.GetUint64("pointer"))
					require.Equal(t, before, writer.Snapshot())
					require.True(t, bytes.Equal(beforeBuffer, buffer.Bytes()))
					got, err := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, err)
					require.Equal(t, input, got)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
			})
		}
	}
}

func TestZigbeeNativeBoundaryBeforeRead(t *testing.T) {
	for _, fcs := range []bool{false, true} {
		for _, bits := range []uint64{0, 1, 23, 25, 39, 41, 1001, 1017, 1024} {
			if bits%8 == 0 && ((!fcs && bits >= 24 && bits <= 1000) || (fcs && bits >= 40 && bits <= 1016)) {
				continue
			}
			root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
			n := root.Children[0].Children[0]
			n.Cfg.SetItem(CfgLength, bits)
			e := parseZigbeeFrame(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary caused IO"); return nil, nil }, fcs)
			require.Error(t, e)
		}
	}
	require.Equal(t, uint16(0x2189), zigbeeCRC([]byte("123456789")), "independent CRC-16/KERMIT check value")
}

func TestZigbeeDecoderDeterministicBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(5023))
	var check func([]zigbeeField, int, int)
	check = func(fields []zigbeeField, start, end int) {
		pos := start
		for _, f := range fields {
			require.Equal(t, pos, f.Start, f.Name)
			require.GreaterOrEqual(t, f.End, f.Start)
			require.LessOrEqual(t, f.End, end)
			if f.Type == "" {
				check(f.Children, f.Start, f.End)
			}
			pos = f.End
		}
		require.Equal(t, end, pos)
	}
	for sample := 0; sample < 4096; sample++ {
		wire := make([]byte, sample%129)
		_, e := rng.Read(wire)
		require.NoError(t, e)
		// Ensure the corpus exercises deeper paths as well as header rejection.
		if len(wire) >= 3 && sample%4 == 0 {
			wire[0] = 2
			wire[1] = 0
		}
		if len(wire) >= 17 && sample%4 == 1 {
			copy(wire, []byte{0x41, 0x88, 7, 0x34, 0x12, 0x78, 0x56, 0, 0, 8, 0, 0x78, 0x56, 0, 0, 0x1e, 9})
		}
		fields, info, e := decodeZigbeeFrame(wire, false)
		if e != nil {
			require.Nil(t, fields)
			require.Nil(t, info)
		} else {
			check(fields, 0, len(wire))
			require.Equal(t, false, info["Payload Decrypted"])
		}
	}
}
