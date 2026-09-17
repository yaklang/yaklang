package parser

import (
	"bytes"
	"encoding/hex"
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

func captureDNSSamples(t testing.TB) [][]byte {
	t.Helper()
	var samples [][]byte
	for _, s := range []string{
		"1234818000010001000000000377777701780000010001c00c000100010000003c00047f000001",
		"123400000000000000000000",
		"123481800001000100010001017800000c0001017900000c00010000003c0003017a00c00c001c00010000003c00020102c00c000100010000003c0000",
	} {
		w, err := hex.DecodeString(s)
		require.NoError(t, err)
		samples = append(samples, w)
	}
	// Existing independent capture fixtures exercise real query/response data.
	for _, name := range []string{"ndpi-dns.pcap", "ndpi-http-connect.pcap"} {
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
			wire, _, err := r.ReadPacketData()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			p := gopacket.NewPacket(wire, r.LinkType(), gopacket.Default)
			if layer := p.Layer(layers.LayerTypeUDP); layer != nil {
				u := layer.(*layers.UDP)
				if u.SrcPort == 53 || u.DstPort == 53 {
					samples = append(samples, append([]byte(nil), u.Payload...))
				}
			}
		}
	}
	return samples
}

func TestCaptureDNSParity(t *testing.T) {
	require.True(t, captureStructuredRules()["dns"], "audit adapter if embedded rule changes")
	for i, wire := range captureDNSSamples(t) {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			want, err := captureLegacy(t, wire, "application-layer.dns", nil, "DNS")
			require.NoError(t, err)
			got, ok := captureDNSStructured(wire)
			require.True(t, ok)
			require.Equal(t, want, got)
			actual, err := ParseStructured(wire, "application-layer.dns", "DNS")
			require.NoError(t, err)
			require.Equal(t, want, actual)
			for n := 0; n < len(wire); n++ {
				_, ok := captureDNSStructured(wire[:n])
				require.False(t, ok, "truncated at %d", n)
			}
			for j := range wire {
				wire[j] = '!'
			}
			require.Equal(t, want, got, "borrowed input")
		})
	}
	// Impossible counts and trailing bytes must not be admitted by the adapter.
	for _, w := range [][]byte{append(bytes.Repeat([]byte{0xff}, 12), 0), append(make([]byte, 12), 1)} {
		_, ok := captureDNSStructured(w)
		require.False(t, ok)
	}
}

func TestCaptureDNSRespectsRegistration(t *testing.T) {
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	_, err := ParseStructured(make([]byte, 12), "application-layer.dns", "DNS")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
}

func FuzzCaptureDNSStructured(f *testing.F) {
	for _, w := range captureDNSSamples(f) {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 4096 {
			t.Skip()
		}
		got, ok := captureDNSStructured(w)
		if ok {
			want, err := captureLegacy(t, w, "application-layer.dns", nil, "DNS")
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	})
}

func BenchmarkCaptureDNSStructured(b *testing.B) {
	samples := captureDNSSamples(b)
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
							var value map[string]any
							var err error
							if mode == "legacy" {
								value, err = captureLegacy(b, w, "application-layer.dns", nil, "DNS")
							} else {
								value, err = ParseStructured(w, "application-layer.dns", "DNS")
							}
							if err != nil || value == nil {
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
