package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func captureHelloWire(body []byte) []byte {
	n := len(body) + 4
	w := append([]byte{22, 3, 3, byte(n >> 8), byte(n), 1, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
	return w
}

func captureHelloSamples(t testing.TB) [][]byte {
	t.Helper()
	body := append([]byte{3, 3}, make([]byte, 32)...)
	body = append(body, 0, 0, 2, 0x13, 1, 1, 0)
	samples := [][]byte{captureHelloWire(body)}
	// Unknown extension and a complete SNI entry; raw extension bytes stay strings.
	sni := []byte{0, 6, 0, 0, 3, 'a', '.', 'b'}
	ext := []byte{0x12, 0x34, 0, 2, 0, 0xff, 0, 0, 0, byte(len(sni))}
	ext = append(ext, sni...)
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)
	samples = append(samples, captureHelloWire(body))
	for _, name := range []string{"ndpi-tls.pcap", "ndpi-http-connect.pcap"} {
		w, err := os.ReadFile(filepath.Join("..", "testdata", "protocol-corpus", "captures", "ndpi", name))
		require.NoError(t, err)
		var r interface {
			ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
			LinkType() layers.LinkType
		}
		if bytes.HasPrefix(w, []byte{10, 13, 13, 10}) {
			r, err = pcapgo.NewNgReader(bytes.NewReader(w), pcapgo.DefaultNgReaderOptions)
		} else {
			r, err = pcapgo.NewReader(bytes.NewReader(w))
		}
		require.NoError(t, err)
		for {
			raw, _, err := r.ReadPacketData()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			p := gopacket.NewPacket(raw, r.LinkType(), gopacket.Default)
			if layer := p.Layer(layers.LayerTypeTCP); layer != nil {
				payload := layer.(*layers.TCP).Payload
				if len(payload) > 9 && payload[0] == 22 && payload[5] == 1 {
					n := int(binary.BigEndian.Uint16(payload[3:])) + 5
					if n <= len(payload) {
						samples = append(samples, append([]byte(nil), payload[:n]...))
					}
				}
			}
		}
	}
	return samples
}

func TestCaptureHelloParity(t *testing.T) {
	require.True(t, captureStructuredRules()["tls"])
	require.True(t, captureStructuredRules()["tls_hello"])
	for i, w := range captureHelloSamples(t) {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			want, err := captureLegacy(t, w, "application-layer.tls", nil)
			require.NoError(t, err)
			got, ok := captureTLSStructured(w)
			require.True(t, ok)
			require.Equal(t, want, got)
			actual, err := ParseStructured(w, "application-layer.tls")
			require.NoError(t, err)
			require.Equal(t, want, actual)
			hello, err := captureLegacy(t, w[5:], "application-layer.tls_hello", nil, "TLSClientHello")
			require.NoError(t, err)
			actual, err = ParseStructured(w[5:], "application-layer.tls_hello", "TLSClientHello")
			require.NoError(t, err)
			require.Equal(t, hello, actual)
			for n := 0; n < len(w); n++ {
				_, ok := captureTLSStructured(w[:n])
				require.False(t, ok, "truncated at %d", n)
			}
			for j := range w {
				w[j] = 0
			}
			require.Equal(t, want, got, "input escaped into output")
		})
	}
	// Multiple records and a following application PDU keep their full shape.
	w := captureHelloSamples(t)[1]
	w = append(w, 23, 3, 3, 0, 2, 1, 2)
	want, err := captureLegacy(t, w, "application-layer.tls", nil)
	require.NoError(t, err)
	got, ok := captureTLSStructured(w)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestCaptureHelloFallback(t *testing.T) {
	valid := captureHelloSamples(t)[1]
	var cases [][]byte
	// These cases deliberately keep the outer TLS length complete. The legacy
	// rule may recover a failed ClientHello as raw Payload, not necessarily error.
	for _, offset := range []int{8, 43, 44, 45, 47, 50, 53, 54, 58, 62, 63, 65, 66} {
		w := bytes.Clone(valid)
		w[offset] = 0xff
		cases = append(cases, w)
	}
	for _, w := range cases {
		want, before := captureLegacy(t, w, "application-layer.tls", nil)
		got, after := ParseStructured(w, "application-layer.tls")
		if before != nil {
			require.Error(t, after)
		} else {
			require.NoError(t, after)
			require.Equal(t, want, got)
		}
	}
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	_, err := ParseStructured(valid, "application-layer.tls")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
}

func TestCaptureHelloBoundaryFields(t *testing.T) {
	for _, sid := range []int{0, 32} {
		for _, suites := range []int{0, 2, 512} {
			for _, compression := range []int{0, 1, 16} {
				// Explicit zero-length extension list, empty SNI name, arbitrary
				// SNI list length/name type, and unknown zero-length extension.
				for _, ext := range [][]byte{nil, {0, 0, 0, 5, 0, 99, 7, 0, 0}, {0x12, 0x34, 0, 0}} {
					body := append([]byte{3, 3}, make([]byte, 32)...)
					body = append(body, byte(sid))
					body = append(body, bytes.Repeat([]byte{'s'}, sid)...)
					body = append(body, byte(suites>>8), byte(suites))
					body = append(body, make([]byte, suites)...)
					body = append(body, byte(compression))
					body = append(body, make([]byte, compression)...)
					body = append(body, byte(len(ext)>>8), byte(len(ext)))
					body = append(body, ext...)
					w := captureHelloWire(body)
					want, err := captureLegacy(t, w, "application-layer.tls", nil)
					require.NoError(t, err)
					got, ok := captureTLSStructured(w)
					require.True(t, ok)
					require.Equal(t, want, got, "sid=%d suites=%d compression=%d extension=%x", sid, suites, compression, ext)
				}
			}
		}
	}
}

func FuzzCaptureTLSClientHello(f *testing.F) {
	for _, w := range captureHelloSamples(f) {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			t.Skip()
		}
		if got, ok := captureTLSStructured(w); ok {
			want, err := captureLegacy(t, w, "application-layer.tls", nil)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	})
}

func BenchmarkCaptureTLSClientHello(b *testing.B) {
	samples := captureHelloSamples(b)
	var total int64
	for _, w := range samples {
		total += int64(len(w))
	}
	for _, mode := range []string{"legacy", "structured"} {
		for _, workers := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("%s/workers=%d", mode, workers), func(b *testing.B) {
				old := runtime.GOMAXPROCS(workers)
				defer runtime.GOMAXPROCS(old)
				b.SetBytes(total)
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						for _, w := range samples {
							var result map[string]any
							var err error
							if mode == "legacy" {
								result, err = captureLegacy(b, w, "application-layer.tls", nil)
							} else {
								result, err = ParseStructured(w, "application-layer.tls")
							}
							if err != nil || result == nil {
								b.Errorf("decode failed: %v", err)
								return
							}
						}
					}
				})
			})
		}
	}
}
