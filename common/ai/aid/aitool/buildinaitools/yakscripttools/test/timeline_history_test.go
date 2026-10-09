package test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	_ "github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestTimelineHistoryRecallBeforeCompression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer db.Close()
	// AI checkpoints and item archives share this SQLite fixture.
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}, &schema.AiCheckpoint{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "history-runtime", PersistentSession: "session-a"}).Error)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithSpeedPriorityAICallback(func(_ aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			response := aicommon.NewUnboundAIResponse()
			response.EmitOutputStream(strings.NewReader(`{"@action":"timeline-summary","summary":"已完成认证调查，继续验证。","ratain_timeline_item_range":"","memory_entities":[]}`))
			response.Close()
			return response, nil
		}))
	cfg.DisableCreateDBRuntime = false
	cfg.PersistentSessionId = "session-a"
	cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB("history-runtime", db)
	tl := aicommon.NewTimeline(cfg, nil)
	tl.SoftBindConfig(cfg, nil)
	runtime := &aitool.ToolRuntimeConfig{ProjectDatabase: db,
		PersistentSessionID: "session-a", MemoryNamespace: "memory-a", TimelineHistorySnapshot: tl.HistorySnapshot}
	testTools := map[string]*aitool.Tool{}
	for name, path := range map[string]string{"grep_timeline_history": "history", "read_memory": "memory", "amend_memory": "memory"} {
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/" + path + "/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, tools, 1)
		testTools[name] = tools[0]
	}
	invokeTool := func(name string, params map[string]any) string {
		t.Helper()
		config := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(runtime)(config)
		result, err := testTools[name].ExecuteToolWithCapture(ctx, params, config)
		require.NoError(t, err)
		require.Nil(t, result.Result, "history/memory items must be carried by stdout, never RESULT")
		return result.Stdout
	}
	invoke := func(params map[string]any) string { t.Helper(); return invokeTool("grep_timeline_history", params) }
	queryRows := func(session, mode, query string) []*schema.AITimelineHistory {
		t.Helper()
		rows, err := yakit.GrepAITimelineHistory(ctx, db, query, yakit.AITimelineHistoryQuery{SessionID: session, Mode: mode, Limit: 50})
		require.NoError(t, err)
		return rows
	}

	// 1. Write every supported kind, including a large original and more than one
	// regexp scan page. Push must persist each receipt before any Save/compression.
	original := "历史 sentinel quoted \"认证失败\"\n第二行保留换行，路径 C:\\scan\n" + strings.Repeat("大报文原文；", 3000) + "正文结束"
	tl.PushTextWithPromptProjection(100, original, "tiny prompt projection")
	tl.PushToolResult(&aitool.ToolResult{ID: 101, Name: "do_http_request", Success: true, Data: &aitool.ToolExecutionResult{Stdout: "tool sentinel \"401\"\nHTTP evidence", CombinedOutput: "tool sentinel \"401\"\nHTTP evidence"}, ShrinkResult: "tiny tool projection", ShrinkSimilarResult: "tiny similar projection"})
	tl.PushUserInteraction(aicommon.UserInteractionStage_Review, 102, "user sentinel question", "用户原文 \"拒绝匿名\"\n继续调查")
	require.True(t, tl.PushPromotable(103, aicommon.TimelinePromotedKindRecentTool, aicommon.TimelinePromotedTargetSemiDynamic1,
		"probe", aicommon.TimelinePromotedOperationUpsert, "promotion sentinel exact schema"))
	for id := int64(104); id < 224; id++ {
		tl.PushText(id, fmt.Sprintf("filler-%d", id))
	}
	var count int
	require.NoError(t, db.Model(&schema.AITimelineHistory{}).Where("session_id = ?", "session-a").Count(&count).Error)
	require.Equal(t, 124, count)
	rows := queryRows("session-a", "literal", "正文结束")
	require.Len(t, rows, 1)
	require.Equal(t, original, rows[0].Content)
	archiveID := rows[0].ID
	foreignConfig := *cfg
	foreignConfig.PersistentSessionId = "session-b"
	foreign := aicommon.NewTimeline(&foreignConfig, nil)
	foreign.SoftBindConfig(&foreignConfig, nil)
	foreign.PushText(100, "foreign sentinel must never leak across sessions")
	for _, mode := range []string{"bm25", "regexp", "literal"} {
		out := invoke(map[string]any{"query": "sentinel", "search_mode": mode, "limit": 1})
		require.Contains(t, out, "next_offset=0", "all live matches must be excluded before pagination")
		require.NotContains(t, out, "<|TIMELINE_")
	}

	// 2. Really compress the Timeline using a deterministic model response. The
	// resume checkpoint must contain only the summary/live tail, not old packets;
	// all originals must remain separate database rows after retirement.
	compression, err := tl.CompressOnce(aicommon.TimelineCompressionOptions{MaxInputTokens: 100000, MaxSummaryTokens: 4096})
	require.NoError(t, err)
	require.NotEmpty(t, compression.RetiredIDs)
	require.NotContains(t, tl.Dump(), "正文结束")
	// Exact user/promotion entries remain live across compression and must stay
	// excluded. Retire them explicitly to exercise history rendering of all kinds.
	out := invoke(map[string]any{"query": "sentinel", "search_mode": "literal"})
	require.Contains(t, out, "next_offset=2")
	tl.SoftDelete(102, 103)
	tl.Save(db, "session-a")
	checkpoint, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(db, "session-a")
	require.NoError(t, err)
	require.NotContains(t, checkpoint.GetTimeline(), "正文结束")
	require.Less(t, len(checkpoint.GetTimeline()), len(original))
	require.Len(t, queryRows("session-a", "literal", "sentinel"), 4)
	require.Len(t, queryRows("session-b", "literal", "sentinel"), 1)
	require.Len(t, queryRows("session-a", "regexp", "\"401\"\nHTTP evidence"), 1, "regexp searches raw multiline tool output")

	// 3. Restore, reassign local IDs, and reuse an old local ID for a new live
	// match. Stable history IDs exclude the live copy without hiding old originals.
	tl.PushText(500, "current sentinel must remain invisible in history search")
	tl.Save(db, "session-a")
	raw, err := aicommon.MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := aicommon.UnmarshalTimeline(raw)
	require.NoError(t, err)
	var seq int64
	restored.ReassignIDs(func() int64 { seq++; return seq })
	restored.SoftBindConfig(cfg, nil)
	restored.PushText(100, "reused local ID sentinel must also be excluded")
	restored.Save(db, "session-a")
	runtime.TimelineHistorySnapshot = restored.HistorySnapshot
	require.NoError(t, db.Model(&schema.AITimelineHistory{}).Where("session_id = ?", "session-a").Count(&count).Error)
	require.Equal(t, 126, count, "restore/reassignment must not duplicate history rows")
	for _, mode := range []string{"bm25", "regexp", "literal"} {
		out := invoke(map[string]any{"query": "sentinel", "search_mode": mode, "limit": 10})
		require.Contains(t, out, "next_offset=4")
		require.Contains(t, out, "<|TIMELINE_")
		require.Contains(t, out, "<|TIMELINE_END_")
		require.Contains(t, out, `quoted "认证失败"`)
		require.Contains(t, out, `路径 C:\scan`)
		require.Contains(t, out, `tool sentinel "401"`)
		require.Contains(t, out, `用户原文 "拒绝匿名"`)
		require.Contains(t, out, "promotion sentinel exact schema")
		require.NotContains(t, out, `\"认证失败\"`)
		require.NotContains(t, out, `\n第二行`)
		require.NotContains(t, out, "tiny prompt projection")
		require.NotContains(t, out, "tiny tool projection")
		require.NotContains(t, out, "tiny similar projection")
		require.NotContains(t, out, "foreign sentinel")
		require.NotContains(t, out, "current sentinel")
		require.NotContains(t, out, "reused local ID sentinel")
		page := invoke(map[string]any{"query": "sentinel", "search_mode": mode, "limit": 1})
		require.Contains(t, page, "next_offset=1", "a newest live match must not consume the result page")
	}

	// 4. Read the archived large original by stable DB item ID, with character
	// chunks, and check regexp keyset paging, short Chinese BM25 fallback, offsets
	// and exact-ID exclusion. These paths must use the same full archive corpus.
	out = invoke(map[string]any{"query": "", "item_id": archiveID, "limit": 1, "content_limit": 30})
	require.Contains(t, out, "truncated=true")
	require.Contains(t, out, "next_content_offset=30")
	out = invoke(map[string]any{"query": "", "item_id": archiveID, "limit": 1, "content_offset": len([]rune(original)) - 4, "content_limit": 30})
	require.Contains(t, out, "正文结束")
	require.Contains(t, out, "truncated=false")
	out = invoke(map[string]any{"query": "^历史 sentinel", "search_mode": "regexp", "limit": 1})
	require.Contains(t, out, "timeline_item_id=100", "regexp must scan past >100 newer nonmatching items")
	out = invoke(map[string]any{"query": "历史", "search_mode": "bm25"})
	require.Contains(t, out, `quoted "认证失败"`)
	out = invoke(map[string]any{"query": "sentinel", "search_mode": "literal", "limit": 1, "offset": 3})
	require.Contains(t, out, "next_offset=4")
	require.Contains(t, out, "timeline_item_id=100")
	liveRows := queryRows("session-a", "literal", "current sentinel")
	require.Len(t, liveRows, 1)
	out = invoke(map[string]any{"query": "", "item_id": liveRows[0].ID})
	require.NotContains(t, out, "<|TIMELINE_")
	_, err = yakit.GrepAITimelineHistory(ctx, db, "[invalid", yakit.AITimelineHistoryQuery{SessionID: "session-a", Mode: "regexp", Limit: 10})
	require.ErrorContains(t, err, "invalid timeline regexp")

	// 5. A failed pre-compression checkpoint must leave the original visible.
	// Already-persisted independent receipts survive the failed resume update.
	cfg.PersistentSessionId = "missing-runtime"
	fail := aicommon.NewTimeline(cfg, nil)
	fail.SoftBindConfig(cfg, nil)
	fail.PushText(1, "must remain after checkpoint failure")
	_, err = fail.CompressOnce(aicommon.TimelineCompressionOptions{MaxInputTokens: 100000, MaxSummaryTokens: 4096})
	require.ErrorContains(t, err, "no runtime")
	require.Contains(t, fail.Dump(), "must remain after checkpoint failure")
	require.Len(t, queryRows("missing-runtime", "literal", "checkpoint failure"), 1)
	// 6. Exercise the same item-oriented API through the actual memory scripts:
	// add/query/change/delete, array tags, raw quotes/newlines, runtime
	// namespace isolation, and the new replacement ID. Embedding is deterministic.
	previousMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	defer func() { vectorstore.IsMockMode = previousMock }()
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{}).Error)
	memoryText := "memory-sentinel report \"markdown\"\n保留来源和原始换行"
	added := invokeTool("amend_memory", map[string]any{"operator": "add", "content": memoryText, "tags": []string{"报告", "line\nbreak"}})
	idPattern := regexp.MustCompile(`memory_id=([a-z0-9-]+)`)
	match := idPattern.FindStringSubmatch(added)
	require.Len(t, match, 2)
	memoryID := match[1]
	require.Contains(t, added, memoryText)
	var memory schema.AIMemoryEntity
	require.NoError(t, db.Where("memory_id = ?", memoryID).First(&memory).Error)
	require.Equal(t, "memory-a", memory.SessionID)
	require.Equal(t, []string{"报告", "line\nbreak"}, []string(memory.Tags))
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: "foreign-memory", SessionID: "memory-b", Content: "memory-sentinel foreign namespace"}).Error)
	read := invokeTool("read_memory", map[string]any{"query": "memory-sentinel", "search_mode": "bm25"})
	require.Contains(t, read, memoryText)
	require.NotContains(t, read, "foreign namespace")
	changed := invokeTool("amend_memory", map[string]any{"operator": "change", "memory_id": memoryID, "content": "memory-revised report JSON"})
	match = idPattern.FindStringSubmatch(changed)
	require.Len(t, match, 2)
	require.NotEqual(t, memoryID, match[1])
	read = invokeTool("read_memory", map[string]any{"query": "memory-sentinel", "search_mode": "bm25"})
	require.Contains(t, read, "hits=0")
	read = invokeTool("read_memory", map[string]any{"query": "memory-revised", "search_mode": "bm25"})
	require.Contains(t, read, "memory-revised report JSON")
	invokeTool("amend_memory", map[string]any{"operator": "delete", "memory_id": match[1]})
	read = invokeTool("read_memory", map[string]any{"query": "memory-revised", "search_mode": "bm25"})
	require.Contains(t, read, "hits=0")

}
