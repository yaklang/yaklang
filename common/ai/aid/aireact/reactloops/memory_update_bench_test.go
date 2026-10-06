package reactloops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// A recalled batch can overflow the budget after an existing entry grows.
// Measure the whole eviction operation, including map setup, without any AI/DB.
func BenchmarkPushMemoryEviction(b *testing.B) {
	content := strings.Repeat("remember this verified observation ", 32)
	entries := make([]*aicommon.MemoryEntity, 128)
	for i := range entries {
		entries[i] = &aicommon.MemoryEntity{Id: fmt.Sprint(i), Content: content}
	}
	result := &aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: "latest", Content: content}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop := &ReActLoop{currentMemories: omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](), memorySizeLimit: aicommon.MeasureTokens(content)}
		for _, entry := range entries {
			loop.currentMemories.Set(entry.Id, entry)
		}
		loop.PushMemory(result)
	}
}
