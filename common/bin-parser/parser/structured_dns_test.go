package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"io"

	"path/filepath"
	"runtime"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func captureDNSSamples(t testing.TB) [][]byte {
	t.Helper()
	var samples [][]byte
	for _, s := range []string{
		"1234818000010001000000000377777701780000010001c00c000100010000003c00047f000001",
		"123400000000000000000000",
		"123481800001000100010001017800000c0001017900000c00010000003c0003017a00c00cfde800010000003c00020102c00c000100010000003c0000",
	} {
		w, err := hex.DecodeString(s)
		require.NoError(t, err)
		samples = append(samples, w)
	}
	// Existing independent capture fixtures exercise real query/response data.
	for _, name := range []string{"ndpi-dns.pcap", "ndpi-http-connect.pcap"} {
		w, err := trafficfixture.ReadFile(filepath.Join("..", "testdata", "protocol-corpus", "captures", "ndpi", name))
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

// Keep the original malformed-AAAA control as explicit rejection evidence.
// The opaque positive control above uses private TYPE65000 instead.
func TestCaptureDNSOriginalShortAAAARejected(t *testing.T) {
	w, err := hex.DecodeString("123481800001000100010001017800000c0001017900000c00010000003c0003017a00c00c001c00010000003c00020102c00c000100010000003c0000")
	require.NoError(t, err)
	_, ok := captureDNSStructured(w)
	require.False(t, ok)
	_, err = captureLegacy(t, w, "application-layer.dns", nil, "DNS")
	require.Error(t, err)
	_, err = ParseStructured(w, "application-layer.dns", "DNS")
	require.Error(t, err)
}

func TestCaptureDNSMixedCompressionParity(t *testing.T) {
	for section := 1; section <= 3; section++ {
		sectionName := []string{"", "Answers", "Authority", "Additional"}[section]
		for _, pointerOnlyQuestion := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/pointer-question-%v", sectionName, pointerOnlyQuestion), func(t *testing.T) {
				wire := make([]byte, 12)
				binary.BigEndian.PutUint16(wire, 0x5013)
				binary.BigEndian.PutUint16(wire[2:], 0x8180)
				binary.BigEndian.PutUint16(wire[4:], 2)
				binary.BigEndian.PutUint16(wire[4+2*section:], 3)
				wire = append(wire, 1, 'b', 3, 'm', 'v', 'p', 7, 'i', 'n', 'v', 'a', 'l', 'i', 'd', 0, 0, 255, 0, 1)
				secondName := []any{map[string]any{"Count": uint8(192), "PointerLow": uint8(14)}}
				if !pointerOnlyQuestion {
					wire = append(wire, 1, 'a')
					secondName = append([]any{map[string]any{"Count": uint8(1), "Text": "a"}}, secondName...)
				}
				wire = append(wire, 0xc0, 14, 0, 1, 0, 1)
				appendRecord := func(name []byte, typ uint16, data []byte) {
					wire = append(wire, name...)
					wire = binary.BigEndian.AppendUint16(wire, typ)
					wire = binary.BigEndian.AppendUint16(wire, 1)
					wire = binary.BigEndian.AppendUint32(wire, 60)
					wire = binary.BigEndian.AppendUint16(wire, uint16(len(data)))
					wire = append(wire, data...)
				}
				address := []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}
				appendRecord([]byte{1, 'c', 0xc0, 14}, 12, []byte{1, 'p', 0xc0, 14})
				appendRecord([]byte{1, 'd', 0xc0, 14}, 28, address)
				appendRecord([]byte{0xc0, 12}, 16, []byte{3, 'm', 'v', 'p', 0})
				labels := func(text string) []any {
					return []any{map[string]any{"Count": uint8(1), "Text": text}, map[string]any{"Count": uint8(192), "PointerLow": uint8(14)}}
				}
				header := map[string]any{"ID": uint16(0x5013), "Flags": uint16(0x8180), "Questions": uint16(2), "Answer RRs": uint16(0), "Authority RRs": uint16(0), "Additional RRs": uint16(0)}
				header[[]string{"", "Answer RRs", "Authority RRs", "Additional RRs"}[section]] = uint16(3)
				want := map[string]any{"fields": map[string]any{
					"Header": header,
					"Questions": []any{
						map[string]any{"Name": []any{map[string]any{"Count": uint8(1), "Text": "b"}, map[string]any{"Count": uint8(3), "Text": "mvp"}, map[string]any{"Count": uint8(7), "Text": "invalid"}, map[string]any{"Count": uint8(0)}}, "Type": uint16(255), "Class": uint16(1)},
						map[string]any{"Name": secondName, "Type": uint16(1), "Class": uint16(1)},
					},
					sectionName: []any{
						map[string]any{"Name": map[string]any{"Labels": labels("c")}, "Type": uint16(12), "Class": uint16(1), "TTL": uint32(60), "RDLength": uint16(4), "DNSPTR": map[string]any{"Name": labels("p")}},
						map[string]any{"Name": map[string]any{"Labels": labels("d")}, "Type": uint16(28), "Class": uint16(1), "TTL": uint32(60), "RDLength": uint16(16), "DNSAAAA": map[string]any{"Address": bytes.Clone(address)}},
						map[string]any{"Name": map[string]any{"PointerFlag": uint8(3), "Pointer": uint16(12)}, "Type": uint16(16), "Class": uint16(1), "TTL": uint32(60), "RDLength": uint16(5), "DNSTXT": map[string]any{"Strings": []any{map[string]any{"Length": uint8(3), "Text": "mvp"}, map[string]any{"Length": uint8(0)}}}},
					},
				}, "metadata": nil}
				legacy, err := captureLegacy(t, wire, "application-layer.dns", nil, "DNS")
				require.NoError(t, err)
				require.Equal(t, want, legacy, "independent wire projection includes terminal pointer octets")
				got, ok := captureDNSStructured(wire)
				require.True(t, ok)
				require.Equal(t, want, got)
				actual, err := ParseStructured(wire, "application-layer.dns", "DNS")
				require.NoError(t, err)
				require.Equal(t, want, actual)
				for n := range wire {
					_, ok := captureDNSStructured(wire[:n])
					require.False(t, ok, "truncated pointer/RR window at %d", n)
				}
				for i := range wire {
					wire[i] = 0xff
				}
				require.Equal(t, want, got, "owned projection")
			})
		}
	}
}

func TestCaptureDNSCompressionRejectsReservedAndTruncatedNames(t *testing.T) {
	for _, label := range [][]byte{{0xc0}, append(append([]byte{64}, bytes.Repeat([]byte{'x'}, 64)...), 0), append(append([]byte{128}, bytes.Repeat([]byte{'x'}, 128)...), 0)} {
		wire := make([]byte, 12)
		binary.BigEndian.PutUint16(wire[4:], 1)
		wire = append(wire, label...)
		if label[0] != 0xc0 {
			wire = append(wire, 0, 1, 0, 1)
		}
		_, ok := captureDNSStructured(wire)
		require.False(t, ok)
		_, err := captureLegacy(t, wire, "application-layer.dns", nil, "DNS")
		require.Error(t, err)
		_, err = ParseStructured(wire, "application-layer.dns", "DNS")
		require.Error(t, err)
	}
}
