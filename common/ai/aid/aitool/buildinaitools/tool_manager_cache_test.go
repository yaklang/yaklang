package buildinaitools

import (
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"gotest.tools/v3/assert"
)

func makeTool(name, desc string, params ...aitool.ToolOption) *aitool.Tool {
	opts := []aitool.ToolOption{aitool.WithDescription(desc)}
	opts = append(opts, params...)
	return aitool.NewWithoutCallback(name, opts...)
}

func newManagerWithCache(maxBytes int) *AiToolManager {
	m := &AiToolManager{
		toolEnabled: make(map[string]bool),
	}
	if maxBytes > 0 {
		m.maxCacheBytes = maxBytes
	}
	return m
}

func TestRecentToolCache_AddAndGet(t *testing.T) {
	mgr := newManagerWithCache(0)

	tool := makeTool("sleep_test", "Sleep for N seconds",
		aitool.WithNumberParam("seconds", aitool.WithParam_Description("seconds to sleep")),
	)
	mgr.AddRecentlyUsedTool(tool)

	assert.Check(t, mgr.HasRecentlyUsedTools(), "cache should not be empty after add")
	assert.Check(t, mgr.IsRecentlyUsedTool("sleep_test"), "sleep_test should be in cache")
	assert.Check(t, !mgr.IsRecentlyUsedTool("unknown_tool"), "unknown_tool should not be in cache")

	names := mgr.GetRecentToolNames()
	assert.Equal(t, len(names), 1)
	assert.Equal(t, names[0], "sleep_test")
}

func TestRecentToolCache_Dedup(t *testing.T) {
	mgr := newManagerWithCache(0)

	tool := makeTool("read_file", "Read file content")
	mgr.AddRecentlyUsedTool(tool)
	mgr.AddRecentlyUsedTool(tool)
	mgr.AddRecentlyUsedTool(tool)

	names := mgr.GetRecentToolNames()
	assert.Equal(t, len(names), 1, "duplicate adds should not create multiple entries")
}

func TestRecentToolCache_SizeLimit(t *testing.T) {
	// measure a single entry size first
	probe := makeTool("probe", "probe description",
		aitool.WithStringParam("x", aitool.WithParam_Description("dummy")),
	)
	probeMgr := newManagerWithCache(0)
	probeMgr.AddRecentlyUsedTool(probe)
	singleSize := probeMgr.totalCacheSize()
	t.Logf("single entry size: %d bytes", singleSize)

	// set limit to hold at most 2 entries
	maxBytes := singleSize*2 + 10
	mgr := newManagerWithCache(maxBytes)

	t1 := makeTool("tool_aaa", "probe description",
		aitool.WithStringParam("x", aitool.WithParam_Description("dummy")),
	)
	t2 := makeTool("tool_bbb", "probe description",
		aitool.WithStringParam("x", aitool.WithParam_Description("dummy")),
	)
	t3 := makeTool("tool_ccc", "probe description",
		aitool.WithStringParam("x", aitool.WithParam_Description("dummy")),
	)

	mgr.AddRecentlyUsedTool(t1)
	mgr.AddRecentlyUsedTool(t2)
	mgr.AddRecentlyUsedTool(t3)

	assert.Check(t, mgr.IsRecentlyUsedTool("tool_ccc"), "most recent tool must survive eviction")
	assert.Check(t, !mgr.IsRecentlyUsedTool("tool_aaa"), "oldest tool should have been evicted")

	mgr.recentToolsMu.Lock()
	total := mgr.totalCacheSize()
	mgr.recentToolsMu.Unlock()
	assert.Check(t, total <= mgr.getMaxCacheBytes(),
		"total cache size %d should not exceed max %d", total, mgr.getMaxCacheBytes())
}

func TestRecentToolCache_Summary(t *testing.T) {
	mgr := newManagerWithCache(0)

	t1 := makeTool("sleep_test", "Sleep for N seconds",
		aitool.WithNumberParam("seconds"),
		aitool.WithUsage("pass seconds as a float, e.g. 0.5 for 500ms"),
	)
	t2 := makeTool("read_file", "Read file content",
		aitool.WithStringParam("path"),
	)
	mgr.AddRecentlyUsedTool(t1)
	mgr.AddRecentlyUsedTool(t2)

	summary := mgr.GetRecentToolsSummary(10240, "testnonce")
	assert.Check(t, summary != "", "summary should not be empty")
	assert.Check(t, strings.Contains(summary, "sleep_test"), "summary should contain sleep_test")
	assert.Check(t, strings.Contains(summary, "read_file"), "summary should contain read_file")
	assert.Check(t, strings.Contains(summary, "Params Schema:"), "summary should contain schema section")

	// AITAG boundaries
	assert.Check(t, strings.Contains(summary, "<|TOOL_sleep_test_testnonce|>"), "summary should have AITAG open boundary")
	assert.Check(t, strings.Contains(summary, "<|TOOL_sleep_test_END_testnonce|>"), "summary should have AITAG close boundary")
	assert.Check(t, strings.Contains(summary, "<|TOOL_read_file_testnonce|>"), "read_file should have AITAG open boundary")
	assert.Check(t, strings.Contains(summary, "<|TOOL_read_file_END_testnonce|>"), "read_file should have AITAG close boundary")

	// __USAGE__ only appears for tools that have it
	assert.Check(t, strings.Contains(summary, "__USAGE__: pass seconds as a float"), "sleep_test should show __USAGE__")

	// footer with directly_call_tool instruction including new fields
	assert.Check(t, strings.Contains(summary, "directly_call_tool"), "footer should reference directly_call_tool")
	assert.Check(t, strings.Contains(summary, "directly_call_tool_params"), "footer should reference directly_call_tool_params")
	assert.Check(t, strings.Contains(summary, "directly_call_identifier"), "footer should reference directly_call_identifier")
	assert.Check(t, strings.Contains(summary, "directly_call_expectations"), "footer should reference directly_call_expectations")
}

func TestRecentToolCache_SummaryMaxBytes(t *testing.T) {
	mgr := newManagerWithCache(0)

	for i := 0; i < 20; i++ {
		name := strings.Repeat("x", 50) + string(rune('a'+i))
		tool := makeTool(name, strings.Repeat("description ", 20),
			aitool.WithStringParam("param1"),
		)
		mgr.AddRecentlyUsedTool(tool)
	}

	// first entry is always included even if it exceeds maxBytes budget
	summary := mgr.GetRecentToolsSummary(500, "n")
	assert.Check(t, summary != "", "summary should not be empty when cache has entries")
	// should contain exactly 1 tool entry (the first surviving one)
	toolCount := strings.Count(summary, "<|TOOL_")
	// each tool has open + close tag, so 2 matches per tool
	assert.Check(t, toolCount == 2, "expected exactly 1 tool entry (2 AITAG matches), got %d", toolCount)

	// with large budget, all remaining entries should be included
	largeSummary := mgr.GetRecentToolsSummary(0, "n")
	largeToolCount := strings.Count(largeSummary, "<|TOOL_")
	assert.Check(t, largeToolCount > 2, "with unlimited budget, should have multiple tools, got %d tags", largeToolCount)
}

func TestRecentToolCache_EmptyManager(t *testing.T) {
	mgr := newManagerWithCache(0)

	assert.Check(t, !mgr.HasRecentlyUsedTools(), "new manager should have no cached tools")
	assert.Check(t, !mgr.IsRecentlyUsedTool("anything"), "nothing should be found in empty cache")
	assert.Equal(t, len(mgr.GetRecentToolNames()), 0)
	assert.Equal(t, mgr.GetRecentToolsSummary(10240, "x"), "")
}

func TestRecentToolCache_NilTool(t *testing.T) {
	mgr := newManagerWithCache(0)
	mgr.AddRecentlyUsedTool(nil)
	assert.Check(t, !mgr.HasRecentlyUsedTools(), "nil tool should not be cached")
}

func TestRecentToolCache_Concurrent(t *testing.T) {
	mgr := newManagerWithCache(0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := strings.Repeat("t", 3) + string(rune('a'+idx%26))
			tool := makeTool(name, "concurrent test tool")
			mgr.AddRecentlyUsedTool(tool)
			mgr.IsRecentlyUsedTool(name)
			mgr.GetRecentToolNames()
			mgr.HasRecentlyUsedTools()
			mgr.GetRecentToolsSummary(10240, "concurrent")
		}(i)
	}
	wg.Wait()

	assert.Check(t, mgr.HasRecentlyUsedTools(), "cache should not be empty after concurrent writes")
}

func TestRecentToolCache_OrderPreservation(t *testing.T) {
	mgr := newManagerWithCache(0)

	mgr.AddRecentlyUsedTool(makeTool("first", "first tool"))
	mgr.AddRecentlyUsedTool(makeTool("second", "second tool"))
	mgr.AddRecentlyUsedTool(makeTool("third", "third tool"))

	names := mgr.GetRecentToolNames()
	assert.Equal(t, len(names), 3)
	assert.Equal(t, names[0], "first")
	assert.Equal(t, names[1], "second")
	assert.Equal(t, names[2], "third")
}

func TestRecentToolCache_ReaddMovesToTail(t *testing.T) {
	mgr := newManagerWithCache(0)

	mgr.AddRecentlyUsedTool(makeTool("alpha", "first"))
	mgr.AddRecentlyUsedTool(makeTool("beta", "second"))
	mgr.AddRecentlyUsedTool(makeTool("alpha", "first updated"))

	names := mgr.GetRecentToolNames()
	assert.Equal(t, len(names), 2)
	assert.Equal(t, names[0], "beta")
	assert.Equal(t, names[1], "alpha")
}
