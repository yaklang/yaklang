package reactloops

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func memoryUpdateTestLoop(limit int) *ReActLoop {
	return &ReActLoop{currentMemories: omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](), memorySizeLimit: limit}
}

func TestCurrentMemorySizeAndContent(t *testing.T) {
	for _, test := range []struct {
		name     string
		contents []string
	}{
		{"empty", nil},
		{"single", []string{"Test content"}},
		{"multiple", []string{"First memory", "Second memory", "Third memory"}},
		{"multilingual", []string{"", "A", "12345", "你好世界", "Hello世界"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := memoryUpdateTestLoop(4096)
			var expectedSize, visible int
			for i, content := range test.contents {
				loop.currentMemories.Set(fmt.Sprint(i), &aicommon.MemoryEntity{Id: fmt.Sprint(i), Content: content})
				expectedSize += aicommon.MeasureTokens(content)
				if content != "" {
					visible++
				}
			}
			require.Equal(t, expectedSize, loop.currentMemorySize())
			content := loop.GetCurrentMemoriesContent()
			for _, expected := range test.contents {
				require.Contains(t, content, expected)
			}
			var bullets int
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(line, "- ") {
					bullets++
				}
			}
			require.Equal(t, visible, bullets)
			if visible == 0 {
				require.Empty(t, content)
			}
		})
	}
}

func TestPushMemoryBudgetAndOrder(t *testing.T) {
	a, b, c := "First memory", "Second memory", "Third memory"
	for _, test := range []struct {
		name     string
		limit    int
		memories []*aicommon.MemoryEntity
		ids      []string
	}{
		{"empty", 100, nil, nil},
		{"nil_entry", 100, []*aicommon.MemoryEntity{nil, {Id: "a", Content: a}}, []string{"a"}},
		{"exact_budget", aicommon.MeasureTokens(a) + aicommon.MeasureTokens(b), []*aicommon.MemoryEntity{{Id: "a", Content: a}, {Id: "b", Content: b}}, []string{"a", "b"}},
		{"oldest_first", aicommon.MeasureTokens(b) + aicommon.MeasureTokens(c), []*aicommon.MemoryEntity{{Id: "a", Content: a}, {Id: "b", Content: b}, {Id: "c", Content: c}}, []string{"b", "c"}},
		{"oversized", 1, []*aicommon.MemoryEntity{{Id: "a", Content: strings.Repeat(a, 100)}}, nil},
		{"zero_budget", 0, []*aicommon.MemoryEntity{{Id: "a", Content: a}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := memoryUpdateTestLoop(test.limit)
			loop.PushMemory(nil)
			loop.PushMemory(&aicommon.SearchMemoryResult{Memories: test.memories})
			require.Equal(t, len(test.ids), loop.currentMemories.Len())
			if len(test.ids) > 0 {
				require.Equal(t, test.ids, loop.currentMemories.Keys())
			}
			require.LessOrEqual(t, loop.currentMemorySize(), test.limit)
		})
	}
	// Updating an existing ID refreshes its recency without a new insertion.
	loop := memoryUpdateTestLoop(aicommon.MeasureTokens(a) + aicommon.MeasureTokens(b))
	loop.PushMemory(&aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: "a", Content: a}, {Id: "b", Content: b}}})
	loop.PushMemory(&aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: "a", Content: a}, {Id: "c", Content: c}}})
	require.Equal(t, []string{"a", "c"}, loop.currentMemories.Keys())
	// A local eviction tally must not become a persistent cache: existing
	// pointers can grow between pushes, requiring more than one eviction.
	entry, _ := loop.currentMemories.Get("a")
	entry.Content = strings.Repeat(a, 100)
	loop.memorySizeLimit = aicommon.MeasureTokens(c)
	loop.PushMemory(&aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: "d", Content: c}}})
	require.Equal(t, []string{"d"}, loop.currentMemories.Keys())
}

func TestPushMemoryConcurrentReaders(t *testing.T) {
	loop := memoryUpdateTestLoop(4096)
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			loop.PushMemory(&aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{{Id: fmt.Sprint(id), Content: fmt.Sprintf("Concurrent memory %d", id)}}})
			_ = loop.GetCurrentMemoriesContent()
		}(i)
	}
	workers.Wait()
	require.Equal(t, 10, loop.currentMemories.Len())
	content := loop.GetCurrentMemoriesContent()
	for i := 0; i < 10; i++ {
		entry, found := loop.currentMemories.Get(fmt.Sprint(i))
		require.True(t, found)
		require.Equal(t, fmt.Sprintf("Concurrent memory %d", i), entry.Content)
	}
	// Prompt projection deliberately selects a bounded subset of stored entries.
	require.Contains(t, content, "Concurrent memory")
}
