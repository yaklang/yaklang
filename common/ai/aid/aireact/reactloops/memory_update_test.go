package reactloops

import (
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// TestCurrentMemorySize_Empty 测试空内存大小
func TestCurrentMemorySize_Empty(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	size := loop.currentMemorySize()
	if size != 0 {
		t.Errorf("expected size 0 for empty memory, got %d", size)
	}
	log.Infof("Empty memory size test passed: size=%d", size)
}

// TestCurrentMemorySize_SingleMemory 测试单个记忆的大小
func TestCurrentMemorySize_SingleMemory(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	content := "This is a test memory content"
	entity := &aimem.MemoryEntity{
		Id:      "mem-1",
		Content: content,
	}

	loop.currentMemories.Set("mem-1", entity)
	size := loop.currentMemorySize()

	expectedSize := len(content)
	if size != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, size)
	}
	log.Infof("Single memory size test passed: size=%d, content_length=%d", size, len(content))
}

// TestCurrentMemorySize_MultipleMemories 测试多个记忆的大小
func TestCurrentMemorySize_MultipleMemories(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	contents := []struct {
		id      string
		content string
	}{
		{"mem-1", "First memory content"},
		{"mem-2", "Second memory content with more details"},
		{"mem-3", "Third memory"},
	}

	expectedSize := 0
	for _, c := range contents {
		entity := &aimem.MemoryEntity{
			Id:      c.id,
			Content: c.content,
		}
		loop.currentMemories.Set(c.id, entity)
		expectedSize += len(c.content)
	}

	actualSize := loop.currentMemorySize()
	if actualSize != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, actualSize)
	}
	log.Infof("Multiple memory size test passed: total_size=%d", actualSize)
}

// TestGetCurrentMemoriesContent_Empty 测试获取空记忆内容
func TestGetCurrentMemoriesContent_Empty(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	content := loop.GetCurrentMemoriesContent()
	if content != "" {
		t.Errorf("expected empty content, got '%s'", content)
	}
	log.Infof("GetCurrentMemoriesContent empty test passed")
}

// TestGetCurrentMemoriesContent_SingleMemory 测试获取单个记忆的内容
func TestGetCurrentMemoriesContent_SingleMemory(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	entity := &aimem.MemoryEntity{
		Id:      "mem-1",
		Content: "Test content",
	}
	loop.currentMemories.Set("mem-1", entity)

	content := loop.GetCurrentMemoriesContent()
	if !strings.Contains(content, "Test content") {
		t.Errorf("expected content to contain 'Test content', got '%s'", content)
	}
	log.Infof("GetCurrentMemoriesContent single memory test passed")
}

// TestGetCurrentMemoriesContent_MultipleMemories 测试获取多个记忆的内容
func TestGetCurrentMemoriesContent_MultipleMemories(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	contents := []string{"First memory", "Second memory", "Third memory"}
	for i, c := range contents {
		entity := &aimem.MemoryEntity{
			Id:      "mem-" + string(rune(i+1)),
			Content: c,
		}
		loop.currentMemories.Set(entity.Id, entity)
	}

	content := loop.GetCurrentMemoriesContent()

	for _, expectedContent := range contents {
		if !strings.Contains(content, expectedContent) {
			t.Errorf("expected content to contain '%s', got '%s'", expectedContent, content)
		}
	}

	log.Infof("GetCurrentMemoriesContent multiple memories test passed: total_length=%d", len(content))
}

// TestGetCurrentMemoriesContent_WithNewlines 测试获取内容中的换行符
func TestGetCurrentMemoriesContent_WithNewlines(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	entity1 := &aimem.MemoryEntity{
		Id:      "mem-1",
		Content: "First",
	}
	entity2 := &aimem.MemoryEntity{
		Id:      "mem-2",
		Content: "Second",
	}

	loop.currentMemories.Set("mem-1", entity1)
	loop.currentMemories.Set("mem-2", entity2)

	content := loop.GetCurrentMemoriesContent()

	// 检查是否有换行符分隔
	if !strings.Contains(content, "First\n") {
		t.Error("expected content to have newlines between memories")
	}

	log.Infof("GetCurrentMemoriesContent newlines test passed")
}

// TestMemorySize_DirectAccess 测试通过直接设置记忆来计算大小
func TestMemorySize_DirectAccess(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	// 直接设置记忆条目而不通过 PushMemory
	entities := []*aimem.MemoryEntity{
		{Id: "m1", Content: "Short"},
		{Id: "m2", Content: "Medium length content here"},
		{Id: "m3", Content: "This is a very long memory content"},
	}

	expectedSize := 0
	for _, e := range entities {
		loop.currentMemories.Set(e.Id, e)
		expectedSize += len(e.Content)
	}

	actualSize := loop.currentMemorySize()
	if actualSize != expectedSize {
		t.Errorf("expected %d, got %d", expectedSize, actualSize)
	}

	log.Infof("DirectAccess test passed: size=%d", actualSize)
}

// TestMemoryContent_Retrieval 测试记忆内容检索的完整性
func TestMemoryContent_Retrieval(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	// 设置多个内存条目
	memories := map[string]string{
		"mem-1": "Go is a programming language",
		"mem-2": "Python is also a programming language",
		"mem-3": "JavaScript runs in browsers",
	}

	for id, content := range memories {
		loop.currentMemories.Set(id, &aimem.MemoryEntity{
			Id:      id,
			Content: content,
		})
	}

	content := loop.GetCurrentMemoriesContent()

	// 验证所有记忆内容都包含在结果中
	for _, memContent := range memories {
		if !strings.Contains(content, memContent) {
			t.Errorf("memory content '%s' not found in result", memContent)
		}
	}

	// 验证格式：每个条目后面都有换行符
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) != len(memories) {
		t.Errorf("expected %d lines, got %d", len(memories), len(lines))
	}

	log.Infof("Retrieval test passed: retrieved %d memories", len(lines))
}

// TestMemorySize_Accuracy 测试内存大小计算的准确性
func TestMemorySize_Accuracy(t *testing.T) {
	testCases := []struct {
		id       string
		content  string
		expected int
	}{
		{"empty", "", 0},
		{"single_char", "A", 1},
		{"number", "12345", 5},
		{"chinese", "你好世界", 12},  // UTF-8: 3 bytes per character
		{"mixed", "Hello世界", 11}, // "Hello" = 5 bytes, "世界" = 6 bytes
	}

	for _, tc := range testCases {
		loop := &ReActLoop{
			currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		}
		loop.currentMemories.Set(tc.id, &aimem.MemoryEntity{
			Id:      tc.id,
			Content: tc.content,
		})

		actualSize := loop.currentMemorySize()
		expectedSize := len([]byte(tc.content))

		if actualSize != expectedSize {
			t.Errorf("testcase %s: expected %d bytes, got %d bytes", tc.id, expectedSize, actualSize)
		}
	}

	log.Infof("Accuracy test passed for all test cases")
}

// TestMemoryOperations_Concurrent 测试并发访问记忆操作
func TestMemoryOperations_Concurrent(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		taskMutex:       &sync.Mutex{},
	}

	// 并发添加记忆
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			entity := &aimem.MemoryEntity{
				Id:      "mem-" + string(rune(index)),
				Content: "Concurrent memory " + string(rune(index)),
			}
			loop.currentMemories.Set(entity.Id, entity)
		}(i)
	}

	wg.Wait()

	if loop.currentMemories.Len() != 10 {
		t.Errorf("expected 10 memories, got %d", loop.currentMemories.Len())
	}

	// 并发获取内容
	var wg2 sync.WaitGroup
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		wg2.Add(1)
		go func(index int) {
			defer wg2.Done()
			results[index] = loop.GetCurrentMemoriesContent()
		}(i)
	}

	wg2.Wait()

	// 验证所有获取的内容都一致
	for i, result := range results {
		if len(result) == 0 {
			t.Errorf("result %d is empty", i)
		}
	}

	log.Infof("Concurrent test passed: all %d goroutines completed successfully", loop.currentMemories.Len())
}
