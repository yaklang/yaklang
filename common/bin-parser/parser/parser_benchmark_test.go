package parser

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type benchmarkBoundedReader struct {
	*bytes.Reader
	bitLength uint64
}

func (r *benchmarkBoundedReader) InputBitLength() uint64 {
	return r.bitLength
}

func BenchmarkParseRuleNTP(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := base.ParseRule("application-layer/ntp.yaml"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseBinaryNTP(b *testing.B) {
	wire := make([]byte, 48)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader := &benchmarkBoundedReader{
			Reader:    bytes.NewReader(wire),
			bitLength: uint64(len(wire) * 8),
		}
		if _, err := ParseBinary(reader, "application-layer.ntp", "NTP"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseBinaryNTPParallel(b *testing.B) {
	wire := make([]byte, 48)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			reader := &benchmarkBoundedReader{
				Reader: bytes.NewReader(wire), bitLength: uint64(len(wire) * 8),
			}
			if _, err := ParseBinary(reader, "application-layer.ntp", "NTP"); err != nil {
				b.Error(err)
				return
			}
			if reader.Len() != 0 {
				b.Errorf("NTP left %d input bytes unread", reader.Len())
				return
			}
		}
	})
}
