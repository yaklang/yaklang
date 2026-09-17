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
	"golang.org/x/net/http2/hpack"
)

func http2TestFrame(typ, flags byte, stream uint32, body ...byte) []byte {
	w := []byte{byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)), typ, flags, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(w[5:], stream)
	return append(w, body...)
}

func http2TestStream(mode string, frames ...[]byte) []byte {
	var w []byte
	if mode == "client" {
		w = append(w, http2FieldsPreface...)
	}
	if mode != "frame" {
		w = append(w, http2TestFrame(4, 0, 0)...)
	}
	for _, f := range frames {
		w = append(w, f...)
	}
	return w
}

func http2TestCoverage(t *testing.T, fields []http2WireField, start, end int) {
	t.Helper()
	at := start
	for _, f := range fields {
		require.Equal(t, at, f.Start, f.Name)
		require.GreaterOrEqual(t, f.End, f.Start)
		if f.Type == "" {
			http2TestCoverage(t, f.Children, f.Start, f.End)
		}
		at = f.End
	}
	require.Equal(t, end, at)
}

func TestHTTP2FieldsFrameLayouts(t *testing.T) {
	frames := map[string][]byte{
		"data":                    http2TestFrame(0, 1, 3, 'a'),
		"empty data":              http2TestFrame(0, 0, 1),
		"padded data":             http2TestFrame(0, 8, 1, 2, 'a', 0, 0),
		"zero pad":                http2TestFrame(0, 8, 1, 0),
		"preserved nonzero pad":   http2TestFrame(0, 8, 1, 1, 99),
		"headers":                 http2TestFrame(1, 4, 3, 0x82),
		"priority padded headers": http2TestFrame(1, 44, 3, 2, 0x80, 0, 0, 1, 255, 0x82, 0, 0),
		"priority":                http2TestFrame(2, 0, 3, 0x80, 0, 0, 1, 255),
		"reset":                   http2TestFrame(3, 0, 3, 0, 0, 0, 8),
		"settings":                http2TestFrame(4, 0, 0, 0, 2, 0, 0, 0, 1, 0, 5, 0, 0, 64, 0),
		"settings ack":            http2TestFrame(4, 1, 0),
		"unknown settings":        http2TestFrame(4, 0, 0, 255, 255, 255, 255, 255, 255),
		"push":                    http2TestFrame(5, 4, 3, 0x80, 0, 0, 2, 0x82),
		"padded push":             http2TestFrame(5, 12, 3, 1, 0, 0, 0, 2, 0x82, 0),
		"ping":                    http2TestFrame(6, 1, 0, 1, 2, 3, 4, 5, 6, 7, 8),
		"goaway":                  http2TestFrame(7, 0, 0, 0x80, 0, 0, 3, 0, 0, 0, 0, 'x'),
		"window":                  http2TestFrame(8, 0, 0x80000003, 0xff, 255, 255, 255),
		"continuation":            http2TestFrame(9, 4, 3, 0x82),
		"unknown":                 http2TestFrame(255, 255, 0, 1, 2, 3),
	}
	for name, w := range frames {
		t.Run(name, func(t *testing.T) {
			fields, info, err := decodeHTTP2Fields(w, "frame")
			require.NoError(t, err)
			http2TestCoverage(t, fields, 0, len(w)*8)
			require.Equal(t, 1, info["Frame Count"])
			require.Equal(t, false, info["HPACK Decoded"])
			for cut := 0; cut < len(w); cut++ {
				_, _, err := decodeHTTP2Fields(w[:cut], "frame")
				require.Error(t, err, "prefix %d", cut)
			}
			_, _, err = decodeHTTP2Fields(append(bytes.Clone(w), 0), "frame")
			require.Error(t, err)
		})
	}
	for name, w := range map[string][]byte{
		"data stream zero":        http2TestFrame(0, 0, 0),
		"settings stream nonzero": http2TestFrame(4, 0, 1),
		"missing pad":             http2TestFrame(0, 8, 1),
		"pad too long":            http2TestFrame(0, 8, 1, 1),
		"short priority":          http2TestFrame(1, 32, 1, 0, 0, 0, 0),
		"priority overlaps pad":   http2TestFrame(1, 40, 1, 1, 0, 0, 0, 0, 0),
		"self dependency":         http2TestFrame(2, 0, 3, 0x80, 0, 0, 3, 0),
		"priority extra":          http2TestFrame(2, 0, 3, 0, 0, 0, 0, 0, 0),
		"reset short":             http2TestFrame(3, 0, 3, 0),
		"settings remainder":      http2TestFrame(4, 0, 0, 0),
		"ack payload":             http2TestFrame(4, 1, 0, 0, 1, 0, 0, 0, 0),
		"push setting":            http2TestFrame(4, 0, 0, 0, 2, 0, 0, 0, 2),
		"initial window":          http2TestFrame(4, 0, 0, 0, 4, 128, 0, 0, 0),
		"small frame setting":     http2TestFrame(4, 0, 0, 0, 5, 0, 0, 63, 255),
		"large frame setting":     http2TestFrame(4, 0, 0, 0, 5, 1, 0, 0, 0),
		"short push":              http2TestFrame(5, 4, 1, 0, 0, 0),
		"zero promise":            http2TestFrame(5, 4, 1, 128, 0, 0, 0),
		"short ping":              http2TestFrame(6, 0, 0, 0),
		"short goaway":            http2TestFrame(7, 0, 0, 0),
		"zero increment":          http2TestFrame(8, 0, 0, 128, 0, 0, 0),
		"long increment":          http2TestFrame(8, 0, 0, 0, 0, 0, 1, 0),
	} {
		t.Run(name, func(t *testing.T) { _, _, err := decodeHTTP2Fields(w, "frame"); require.Error(t, err) })
	}
}

// RFC 7541 C.4.1..3: independent published Huffman and dynamic-table vectors.
func TestHTTP2FieldsHPACKContextAndContinuation(t *testing.T) {
	literals := []string{"828684418cf1e3c2e5f23a6ba0ab90f4ff", "828684be5886a8eb10649cbf", "828785bf408825a849e95ba97d7f8925a849e95bb8e8b4bf"}
	expected := [][][2]string{
		{{":method", "GET"}, {":scheme", "http"}, {":path", "/"}, {":authority", "www.example.com"}},
		{{":method", "GET"}, {":scheme", "http"}, {":path", "/"}, {":authority", "www.example.com"}, {"cache-control", "no-cache"}},
		{{":method", "GET"}, {":scheme", "https"}, {":path", "/index.html"}, {":authority", "www.example.com"}, {"custom-key", "custom-value"}},
	}
	var blocks [][]byte
	for _, literal := range literals {
		b, err := hex.DecodeString(literal)
		require.NoError(t, err)
		blocks = append(blocks, b)
	}
	for _, mode := range []string{"client", "server"} {
		for split := 0; split <= len(blocks[0]); split++ {
			w := http2TestStream(mode, http2TestFrame(1, 0, 1, blocks[0][:split]...), http2TestFrame(9, 4, 1, blocks[0][split:]...), http2TestFrame(1, 4, 3, blocks[1]...), http2TestFrame(1, 4, 5, blocks[2]...))
			f, info, err := decodeHTTP2Fields(w, mode)
			require.NoError(t, err, "%s split %d", mode, split)
			http2TestCoverage(t, f, 0, len(w)*8)
			require.Equal(t, 14, info["Header Count"])
			decoded := info["Header Blocks"].([]map[string]any)
			require.Len(t, decoded, 3)
			for i, block := range decoded {
				require.Equal(t, i, block["Prior Header Block Count"])
				require.Equal(t, uint64(1+i*2), block["Stream Identifier"])
				h := block["Headers"].([]map[string]any)
				require.Len(t, h, len(expected[i]))
				for j, pair := range expected[i] {
					require.Equal(t, pair[0], h[j]["Name"])
					require.Equal(t, pair[1], h[j]["Value"])
				}
				var compressed []byte
				for _, span := range block["Relative Byte Ranges"].([][2]int) {
					compressed = append(compressed, w[span[0]:span[1]]...)
				}
				require.Equal(t, blocks[i], compressed)
			}
		}
		_, _, err := decodeHTTP2Fields(http2TestStream(mode, http2TestFrame(1, 4, 3, blocks[1]...)), mode)
		require.Error(t, err, "dynamic reference without prior block must fail")
	}
	// Priority and padding bytes must never be mistaken for compressed fields.
	for _, mode := range []string{"client", "server"} {
		w := http2TestStream(mode, http2TestFrame(1, 44, 3, 2, 0x80, 0, 0, 1, 255, 0x82, 0, 0))
		f, info, err := decodeHTTP2Fields(w, mode)
		require.NoError(t, err)
		http2TestCoverage(t, f, 0, len(w)*8)
		b := info["Header Blocks"].([]map[string]any)[0]
		require.Equal(t, 1, info["Header Count"])
		require.Equal(t, "GET", b["Headers"].([]map[string]any)[0]["Value"])
		span := b["Relative Byte Ranges"].([][2]int)[0]
		require.Equal(t, []byte{0x82}, w[span[0]:span[1]])
		require.Equal(t, uint64(256), f[len(f)-1].Info["Effective Weight"])
	}
	for _, frames := range [][][]byte{
		{http2TestFrame(9, 4, 1, 0x82)},
		{http2TestFrame(1, 0, 1, 0x82)},
		{http2TestFrame(1, 0, 1, 0x82), http2TestFrame(9, 4, 3)},
		{http2TestFrame(1, 0, 1, 0x82), http2TestFrame(255, 0, 0), http2TestFrame(9, 4, 1)},
		{http2TestFrame(1, 4, 1, 0x40)},                   // truncated literal, despite END_HEADERS
		{http2TestFrame(1, 4, 1, 0x80)},                   // indexed zero
		{http2TestFrame(1, 4, 1, 0x3f, 0xe2, 0x1f)},       // table 4097
		{http2TestFrame(1, 4, 1, 0x82, 0x20)},             // update after a field
		{http2TestFrame(1, 4, 1, 0x00, 0x81, 0xff, 0x00)}, // invalid Huffman padding
	} {
		_, _, err := decodeHTTP2Fields(http2TestStream("server", frames...), "server")
		require.Error(t, err, "invalid frames: %x", frames)
	}
	// A direction's own advertised table size is for its peer, not itself.
	w := append(http2TestFrame(4, 0, 0, 0, 1, 0, 0, 0, 0), http2TestFrame(1, 4, 1, blocks[0]...)...)
	w = append(w, http2TestFrame(1, 4, 3, blocks[1]...)...)
	_, _, err := decodeHTTP2Fields(w, "server")
	require.NoError(t, err)
	// Explicit table reset evicts the earlier authority; literal never-indexed
	// fields retain their HPACK flag without changing the dictionary.
	w = http2TestStream("server", http2TestFrame(1, 4, 1, blocks[0]...), http2TestFrame(1, 4, 3, 0x20, 0x10, 1, 'x', 1, 'y'))
	_, info, err := decodeHTTP2Fields(w, "server")
	require.NoError(t, err)
	require.Equal(t, true, info["Header Blocks"].([]map[string]any)[1]["Headers"].([]map[string]any)[0]["Sensitive"])
	_, _, err = decodeHTTP2Fields(append(w, http2TestFrame(1, 4, 5, 0xbe)...), "server")
	require.Error(t, err)
	// Two leading updates (256 then 512) preserve the 57-byte authority entry.
	// They must work even when the decoder's dynamic table is not empty.
	w = http2TestStream("server", http2TestFrame(1, 4, 1, blocks[0]...), http2TestFrame(1, 4, 3, 0x3f, 0xe1, 0x01, 0x3f, 0xe1, 0x03, 0xbe))
	_, info, err = decodeHTTP2Fields(w, "server")
	require.NoError(t, err)
	require.Equal(t, "www.example.com", info["Header Blocks"].([]map[string]any)[1]["Headers"].([]map[string]any)[0]["Value"])
	// A zero-sized reduction really evicts, even if the limit is raised again.
	w = http2TestStream("server", http2TestFrame(1, 4, 1, blocks[0]...), http2TestFrame(1, 4, 3, 0x20, 0x3f, 0xe1, 0x03, 0xbe))
	_, _, err = decodeHTTP2Fields(w, "server")
	require.Error(t, err)
	for _, block := range [][]byte{{0xff, 0xff}, append([]byte{0xff}, bytes.Repeat([]byte{0xff}, 12)...), {0x00, 0x7f, 0xff}, {0x00, 1, 'x', 0x7f, 0x00}} {
		_, _, err = decodeHTTP2Fields(http2TestStream("server", http2TestFrame(1, 4, 1, block...)), "server")
		require.Error(t, err)
	}
	// PUSH_PROMISE header provenance refers to the initiating stream and promise.
	w = http2TestStream("server", http2TestFrame(5, 0, 1, 0, 0, 0, 2, 0x82), http2TestFrame(9, 0, 1, 0x86), http2TestFrame(9, 4, 1, 0x84))
	_, info, err = decodeHTTP2Fields(w, "server")
	require.NoError(t, err)
	b := info["Header Blocks"].([]map[string]any)[0]
	require.Equal(t, uint64(5), b["Frame Type"])
	require.Equal(t, uint64(2), b["Promised Stream Identifier"])
	_, _, err = decodeHTTP2Fields(append([]byte(http2FieldsPreface), w...), "client")
	require.Error(t, err)
	_, _, err = decodeHTTP2Fields(http2TestFrame(4, 0, 0, 0, 2, 0, 0, 0, 1), "server")
	require.Error(t, err)
	for _, mode := range []string{"client", "server"} {
		for _, bad := range [][]byte{http2TestFrame(4, 1, 0), http2TestFrame(1, 4, 1, 0x82)} {
			if mode == "client" {
				bad = append([]byte(http2FieldsPreface), bad...)
			}
			_, _, err = decodeHTTP2Fields(bad, mode)
			require.Error(t, err)
		}
	}
}

func TestHTTP2FieldsResourceLimits(t *testing.T) {
	for _, count := range []int{1024, 1025} {
		_, _, err := decodeHTTP2Fields(bytes.Repeat(http2TestFrame(4, 0, 0), count), "server")
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "frame count")
		}
		settings := bytes.Repeat([]byte{0, 1, 0, 0, 0, 0}, count)
		_, _, err = decodeHTTP2Fields(http2TestFrame(4, 0, 0, settings...), "server")
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	w := append(http2TestFrame(4, 0, 0, bytes.Repeat([]byte{0, 1, 0, 0, 0, 0}, 1024)...), http2TestFrame(4, 0, 0, 0, 1, 0, 0, 0, 0)...)
	_, _, err := decodeHTTP2Fields(w, "server")
	require.ErrorContains(t, err, "cumulative SETTINGS")
	for _, n := range []int{4096, 4097} {
		_, _, err := decodeHTTP2Fields(http2TestStream("server", http2TestFrame(1, 4, 1, bytes.Repeat([]byte{0x82}, n)...)), "server")
		if n == 4096 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, n := range []int{65536, 65537} {
		var b bytes.Buffer
		e := hpack.NewEncoder(&b)
		require.NoError(t, e.WriteField(hpack.HeaderField{Name: "x", Value: string(bytes.Repeat([]byte{'a'}, n))}))
		_, _, err := decodeHTTP2Fields(http2TestStream("server", http2TestFrame(1, 4, 1, b.Bytes()...)), "server")
		if n == 65536 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	// Expansion limit counts repeated dynamic references, not only wire bytes.
	var b bytes.Buffer
	e := hpack.NewEncoder(&b)
	for i := 0; i < 1100; i++ {
		require.NoError(t, e.WriteField(hpack.HeaderField{Name: "x", Value: string(bytes.Repeat([]byte{'a'}, 1024))}))
	}
	_, _, err = decodeHTTP2Fields(http2TestStream("server", http2TestFrame(1, 4, 1, b.Bytes()...)), "server")
	require.ErrorContains(t, err, "resource limit")
	for _, n := range []int{http2FieldsMaxBytes, http2FieldsMaxBytes + 1} {
		w := http2TestFrame(255, 0, 0, make([]byte, n-9)...)
		_, _, err := decodeHTTP2Fields(w, "frame")
		if n == http2FieldsMaxBytes {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestHTTP2FieldsBridgeTransactions(t *testing.T) {
	for _, mode := range []string{"frame", "client", "server"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
				t.Run(fmt.Sprintf("%s/%d/%s", mode, offset, outcome), func(t *testing.T) {
					input := http2TestStream(mode, http2TestFrame(8, 0, 0, 0x80, 0, 0, 1))
					inputBits := uint64(len(input)) * 8
					if outcome == "invalid" {
						input[len(input)-1] = 0
					} else if outcome == "short" {
						input = input[:len(input)-1]
					}
					root := giopBridgeInlineRoot(t, "endian: little\nunit: byte\nPackage:\n  Message: {}\n")
					n := root.Children[0].Children[0]
					n.Cfg.SetItem(CfgIsTerminal, false)
					require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
					root.Cfg.SetItem(CfgLength, offset+inputBits)
					p := &DefParser{}
					require.NoError(t, p.OnRoot(root))
					n.Cfg.SetItem(CfgLength, inputBits)
					n.Cfg.SetItem("additionInfo", map[string]any{"caller": true})
					alias := &base.Node{Name: "existing", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
					alias.Cfg.SetItem(CfgParent, n)
					n.Children = []*base.Node{alias}
					cfg, ctx := n.Cfg, n.Ctx
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
						_, err := p.write([]byte{0x55}, offset)
						require.NoError(t, err)
					}
					require.NoError(t, w.WriteBits(input, uint64(len(input))*8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					if offset > 0 {
						_, err := r.ReadBits(offset)
						require.NoError(t, err)
					}
					writer, buffer := ctx.GetItem("writer").(*base.BitWriter), ctx.GetItem("buffer").(*bytes.Buffer)
					before, beforeBuffer := writer.Snapshot(), bytes.Clone(buffer.Bytes())
					calls, finishes := 0, 0
					err := parseHTTP2Fields(n, func(raw *base.Node) (func(bool), error) {
						calls++
						require.NotContains(t, n.Children, raw)
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
					}, mode)
					require.Equal(t, 1, calls)
					require.Equal(t, 1, finishes)
					require.Same(t, cfg, n.Cfg)
					require.Same(t, ctx, n.Ctx)
					if outcome == "commit" {
						require.NoError(t, err)
						giopBridgeAssertTree(t, n, offset, offset+inputBits)
						require.Equal(t, offset+inputBits, ctx.GetUint64("pointer"))
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
	for _, bits := range []uint64{0, 1, 7, 8, 71, 73, (http2FieldsMaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseHTTP2Fields(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, "frame")
		require.Error(t, err)
	}
}
