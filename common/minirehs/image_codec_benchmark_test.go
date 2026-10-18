package minirehs

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// Keep codec tradeoffs reproducible on the same real-world rules and hardware.
func BenchmarkImageCodecs(b *testing.B) {
	_, raw := compileRawImageTest(b, getRequestImagePatterns(b), false)
	for _, name := range []string{"gzip-fast", "zstd-fast", "zstd-default"} {
		b.Run(name, func(b *testing.B) {
			encode := func() []byte {
				if name == "gzip-fast" {
					var out bytes.Buffer
					w, _ := gzip.NewWriterLevel(&out, gzip.BestSpeed)
					_, _ = w.Write(raw)
					_ = w.Close()
					return out.Bytes()
				}
				level := zstd.SpeedFastest
				if name == "zstd-default" {
					level = zstd.SpeedDefault
				}
				w, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(level), zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(1<<20))
				defer w.Close()
				return w.EncodeAll(raw, nil)
			}
			encoded := encode()
			b.Logf("raw=%d encoded=%d", len(raw), len(encoded))
			b.Run("encode", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(encoded)), "encoded-B")
				for i := 0; i < b.N; i++ {
					_ = encode()
				}
			})
			b.Run("decode", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(len(encoded)), "encoded-B")
				for i := 0; i < b.N; i++ {
					var r io.ReadCloser
					if name == "gzip-fast" {
						r, _ = gzip.NewReader(bytes.NewReader(encoded))
					} else {
						d, _ := zstd.NewReader(bytes.NewReader(encoded), zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(1<<20))
						r = d.IOReadCloser()
					}
					out := bytes.NewBuffer(make([]byte, 0, len(raw)+bytes.MinRead))
					_, err := out.ReadFrom(r)
					_ = r.Close()
					if err != nil || out.Len() != len(raw) {
						b.Fatal(err, out.Len())
					}
				}
			})
		})
	}
}
