package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"golang.org/x/net/http2/hpack"
)

// Synthetic bounded frames; one operation is one complete layout or HPACK block.
func BenchmarkHTTP2Review(b *testing.B) {
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"headers", http2TestFrame(1, 4, 1, 0x82, 0x84)},
		{"settings", http2TestFrame(4, 0, 0, bytes.Repeat([]byte{0, 1, 0, 0, 16, 0}, 32)...)},
		{"padded-priority", http2TestFrame(1, 44, 3, 2, 0x80, 0, 0, 1, 255, 0x82, 0, 0)},
	} {
		b.Run("layout/"+tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.wire)))
			for i := 0; i < b.N; i++ {
				if _, err := InspectHTTP2Frame(tc.wire); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	for _, n := range []int{4, 128} {
		var wire bytes.Buffer
		enc := hpack.NewEncoder(&wire)
		for i := 0; i < n; i++ {
			if err := enc.WriteField(hpack.HeaderField{Name: fmt.Sprintf("x-header-%d", i), Value: "some repeated header value for Huffman coding", Sensitive: true}); err != nil {
				b.Fatal(err)
			}
		}
		block := bytes.Clone(wire.Bytes())
		b.Run(fmt.Sprintf("hpack/headers=%d", n), func(b *testing.B) {
			d := NewHTTP2HeaderDecoder()
			b.ReportAllocs()
			b.SetBytes(int64(len(block)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h, err := d.Decode(block)
				if err != nil || len(h) != n {
					b.Fatalf("headers=%d error=%v", len(h), err)
				}
			}
		})
	}
}
