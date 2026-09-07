package stream_parser

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestGSSAPIBridgeTransactions(t *testing.T) {
	valid, err := hex.DecodeString("601b06062b0601050502a011300fa00d300b06092a864886f712010202")
	require.NoError(t, err)
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					input[10] = 0xa1
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
				err := parseGSSAPIToken(n, func(raw *base.Node) (func(bool), error) {
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
	for _, bits := range []uint64{0, 1, 7, 9, 31, 65535*8 + 1, 65536 * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseGSSAPIToken(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil })
		require.Error(t, err)
	}
}

func TestGSSAPIOIDAndBase64Boundaries(t *testing.T) {
	for _, pair := range []struct{ hex, oid string }{
		{"2a864886f712010202", "1.2.840.113554.1.2.2"},
		{"883703", "2.999.3"}, {"00", "0.0"}, {"4f", "1.39"}, {"50", "2.0"},
		{"2a81ffffffffffffffff7f", "1.2.18446744073709551615"},
	} {
		value, err := hex.DecodeString(pair.hex)
		require.NoError(t, err)
		oid, err := gssapiOID(value)
		require.NoError(t, err)
		require.Equal(t, pair.oid, oid)
	}
	for _, value := range [][]byte{nil, {0x80, 1}, {0x2a, 0x80, 1}, {0x2a, 0x81}, bytes.Repeat([]byte{1}, 129), bytes.Repeat([]byte{1}, 64), {0x2a, 0x82, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}} {
		_, err := gssapiOID(value)
		require.Error(t, err, "%x", value)
	}
	wire := []byte{0xa1, 2, 0x30, 0}
	encoded := base64.StdEncoding.EncodeToString(wire)
	first, err := inspectGSSAPIBase64(encoded)
	require.NoError(t, err)
	first["Decoded Bytes"].([]byte)[0] = 0
	second, err := inspectGSSAPIBase64(encoded)
	require.NoError(t, err)
	require.Equal(t, wire, second["Decoded Bytes"])
	for _, value := range []string{"", "oQIwAA==\r\n", "oQIwAA== ", "oQIwAA", "oQIwAB==", string(bytes.Repeat([]byte{'A'}, base64.StdEncoding.EncodedLen(gssapiMaxBytes)+1))} {
		info, err := inspectGSSAPIBase64(value)
		require.Error(t, err)
		require.Nil(t, info)
	}
}

func TestGSSAPIValueDeterministicMutations(t *testing.T) {
	valid, err := hex.DecodeString("602e06062b0601050502a0243022a00d300b06092a864886f712010202a10403020640a2050403616263a3040402dead")
	require.NoError(t, err)
	for at := range valid {
		for _, mask := range []byte{1, 0x80, 0xff} {
			wire := bytes.Clone(valid)
			wire[at] ^= mask
			one, info, err := decodeGSSAPIToken(wire)
			two, again, repeat := decodeGSSAPIToken(wire)
			require.Equal(t, err == nil, repeat == nil)
			require.Equal(t, one, two)
			require.Equal(t, info, again)
			if err != nil {
				require.Nil(t, one)
				require.Nil(t, info)
			}
		}
	}
}
