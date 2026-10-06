package aicommon

import (
	"strings"
	"testing"
)

// Isolate the copy/serialization phase that holds Timeline's read lock.
func BenchmarkTimelineCompressionSnapshot(b *testing.B) {
	tl := NewTimeline(nil, nil)
	for id := int64(1); id <= 128; id++ {
		tl.PushText(id, strings.Repeat("verified historical observation ", 64))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tl.captureCompressionSnapshot(); err != nil {
			b.Fatal(err)
		}
	}
}
