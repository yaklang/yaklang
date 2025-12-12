package yakvm

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var (
	benchmarkString = strings.Repeat("你好yaklang", 64)
	benchRuneSink   rune
	benchValueSink  *Value
)

func BenchmarkStringIndexFullConvert(b *testing.B) {
	s := benchmarkString
	idx := 0
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRuneSink = []rune(s)[idx]
	}
}

func BenchmarkStringIndexDecodeRune(b *testing.B) {
	s := benchmarkString
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r, _ := utf8.DecodeRuneInString(s)
		benchRuneSink = r
	}
}

func BenchmarkStringIndexCachedSlice(b *testing.B) {
	s := benchmarkString
	runes := []rune(s)
	idx := 0
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRuneSink = runes[idx]
	}
}

func BenchmarkYakVMStringIndex(b *testing.B) {
	frame := &Frame{}
	s := benchmarkString
	args := []*Value{NewStringValue(s), NewAutoValue(0)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchValueSink = frame.getValueForLeftIterableCall(args)
	}
}
