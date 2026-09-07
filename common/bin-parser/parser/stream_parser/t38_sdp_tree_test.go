package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestT38SDPBridgeTransactions(t *testing.T) {
	valid := []byte("v=0\r\no=- 1 1 IN IP4 192.0.2.1\r\ns=-\r\nc=IN IP4 192.0.2.1\r\nt=0 0\r\nm=image 4000 udptl t38\r\n")
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					input[0] = '!'
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
				err := parseT38SDPAdvertisement(n, func(raw *base.Node) (func(bool), error) {
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
	for _, bits := range []uint64{0, 1, 7, 9, 31, t38SDPMaxBytes*8 + 1, (t38SDPMaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseT38SDPAdvertisement(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, false)
		require.Error(t, err)
	}
}

func TestT38SDPValueResourceBounds(t *testing.T) {
	const full = "v=0\r\no=- 1 1 IN IP4 192.0.2.1\r\ns=-\r\nc=IN IP4 192.0.2.1\r\nt=0 0\r\nm=image 4000 udptl t38\r\n"
	// Exact maximum message with each extension line independently bounded.
	wire := full
	for len(wire) < 65536 {
		size := 65536 - len(wire)
		if size > 4096 {
			size = 4096
		}
		if 65536-len(wire)-size > 0 && 65536-len(wire)-size < 6 {
			size -= 6
		}
		wire += "a=x:" + strings.Repeat("x", size-6) + "\r\n"
	}
	fields, info, err := decodeT38SDPAdvertisement([]byte(wire), false)
	require.NoError(t, err)
	require.NotEmpty(t, fields)
	require.NotNil(t, info)
	fields, info, err = decodeT38SDPAdvertisement([]byte(wire+"x"), false)
	require.Error(t, err)
	require.Nil(t, fields)
	require.Nil(t, info)
	for _, n := range []int{32, 33} {
		wire := strings.Repeat("v=0\r\nc=IN IP4 $\r\nm=image $ udptl t38\r\n", n)
		_, _, err := decodeT38SDPAdvertisement([]byte(wire), true)
		if n == 32 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "32-alternative")
		}
	}
	for _, n := range []int{64, 65} {
		wire := full + strings.Repeat("m=image 4000 udptl t38\r\n", n-1)
		_, _, err := decodeT38SDPAdvertisement([]byte(wire), false)
		if n == 64 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "64-media")
		}
	}
	for i := range []byte(full) {
		wire := []byte(full)
		wire[i] = 0
		fields, info, err := decodeT38SDPAdvertisement(wire, false)
		require.Error(t, err)
		require.Nil(t, fields)
		require.Nil(t, info)
	}
}
