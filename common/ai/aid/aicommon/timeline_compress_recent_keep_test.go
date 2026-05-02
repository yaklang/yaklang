package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// makeBigToolResultPayload 构造较长的 ToolResult Data，便于稳定地触发 token 累加
// 关键词: timeline 压缩测试 工具构造
func makeBigToolResultPayload(seed int, repeat int) string {
	var b strings.Builder
	for i := 0; i < repeat; i++ {
		b.WriteString("data-block-")
		b.WriteString(strings.Repeat("X", 32))
		b.WriteString("-seed-")
		b.WriteString(strings.Repeat("Y", seed%7+1))
		b.WriteString("\n")
	}
	return b.String()
}

// TestFindCompressSplit_ByRecentKeepTokens_Even 100 个均匀 item，
// keepTokens = currentSize/4，断言 splitIdx 落在 item 总数 75% 附近（容差 ±10%）。
// 关键词: findCompressSplitByRecentKeepTokens 均匀
func TestFindCompressSplit_ByRecentKeepTokens_Even(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	const N = 100
	for i := int64(1); i <= N; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, makeBigToolResultPayload(int(i), 4)))
	}

	currentSize := tl.calculateActualContentSize()
	require.Greater(t, currentSize, int64(0))

	keepTokens := currentSize / 4
	splitIdx := tl.findCompressSplitByRecentKeepTokens(keepTokens)
	require.Greater(t, splitIdx, 0, "split must be > 0 for sufficiently large timeline")
	require.Less(t, splitIdx, N, "split must be < N (something must remain in recent keep)")

	// 期望切点在 75% 位置 ± 15% 容差（BPE 估算近似不严格线性）
	expected := N * 3 / 4
	tolerance := N * 15 / 100
	require.InDelta(t, expected, splitIdx, float64(tolerance),
		"splitIdx %d should be ~75%% of N=%d (within +-%d), currentSize=%d, keepTokens=%d",
		splitIdx, N, tolerance, currentSize, keepTokens)
}

// TestFindCompressSplit_LargeNewest 最新 1 条 item 占大头，splitIdx 应接近 N-1（其余全部进入待压缩）
// 关键词: findCompressSplitByRecentKeepTokens 大尾巴
func TestFindCompressSplit_LargeNewest(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	const N = 20
	// 前 N-1 条小 item
	for i := int64(1); i <= N-1; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "tiny"))
	}
	// 最新 1 条非常大
	injectTimelineItem(tl, int64(N), baseTs.Add(time.Duration(N)*time.Second),
		makeToolResult(int64(N), "tool", true, makeBigToolResultPayload(N, 200)))

	currentSize := tl.calculateActualContentSize()
	require.Greater(t, currentSize, int64(0))

	keepTokens := currentSize / 4
	splitIdx := tl.findCompressSplitByRecentKeepTokens(keepTokens)

	// 最新一条已大于 keepTokens，splitIdx 必须 == N-1（即只保留最后一条）
	require.Equal(t, N-1, splitIdx,
		"splitIdx should be N-1=%d (only the newest huge item kept), got %d", N-1, splitIdx)
}

// TestFindCompressSplit_SingleItem 1 条不压缩
// 关键词: findCompressSplitByRecentKeepTokens 边界 单 item
func TestFindCompressSplit_SingleItem(t *testing.T) {
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, time.Now(), makeToolResult(1, "tool", true, "data"))
	splitIdx := tl.findCompressSplitByRecentKeepTokens(int64(1))
	require.Equal(t, 0, splitIdx, "single item must not be compressed")
}

// TestFindCompressSplit_KeepTokensZero keepTokens=0 至少保留最新 1 条
// 关键词: findCompressSplitByRecentKeepTokens 边界 keepTokens=0
func TestFindCompressSplit_KeepTokensZero(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "x"))
	}
	splitIdx := tl.findCompressSplitByRecentKeepTokens(0)
	// keepTokens<=0 走 fallback：至少保留最新 1 个 item，splitIdx = total - 1 = 4
	require.Equal(t, 4, splitIdx)
}

// TestFindCompressSplit_AllFitInKeep 全部 item 累加 token 仍 < keepTokens 时，不压缩
// 关键词: findCompressSplitByRecentKeepTokens 全部纳入 recent
func TestFindCompressSplit_AllFitInKeep(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "x"))
	}
	// keepTokens 设为远超活跃区总 token 的值
	splitIdx := tl.findCompressSplitByRecentKeepTokens(int64(1 << 30))
	require.Equal(t, 0, splitIdx, "when all items fit in keep budget, split must be 0 (no compress)")
}

// TestRenderBatchCompressPrompt_ContainsBothSections 双段都应出现在 prompt 中
// 关键词: renderBatchCompressPrompt RECENT_KEEP ITEMS_TO_COMPRESS
func TestRenderBatchCompressPrompt_ContainsBothSections(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	var toCompress []*TimelineItem
	var recentKeep []*TimelineItem
	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "old-payload"))
		item, _ := tl.idToTimelineItem.Get(i)
		toCompress = append(toCompress, item)
	}
	for i := int64(4); i <= 6; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "recent-payload"))
		item, _ := tl.idToTimelineItem.Get(i)
		recentKeep = append(recentKeep, item)
	}

	out := tl.renderBatchCompressPrompt(toCompress, recentKeep, "TESTNONCE")
	require.NotEmpty(t, out)
	require.Contains(t, out, "<|RECENT_KEEP_TESTNONCE|>", "RECENT_KEEP open tag missing")
	require.Contains(t, out, "<|RECENT_KEEP_END_TESTNONCE|>", "RECENT_KEEP close tag missing")
	require.Contains(t, out, "<|ITEMS_TO_COMPRESS_TESTNONCE|>", "ITEMS_TO_COMPRESS open tag missing")
	require.Contains(t, out, "<|ITEMS_TO_COMPRESS_END_TESTNONCE|>", "ITEMS_TO_COMPRESS close tag missing")
	// 老/新 payload 都应出现在各自的段
	require.Contains(t, out, "old-payload")
	require.Contains(t, out, "recent-payload")
}

// TestRenderBatchCompressPrompt_NoRecentKept 无 recentKeep 时不应渲染 RECENT_KEEP 段
// 关键词: renderBatchCompressPrompt 无 recent
func TestRenderBatchCompressPrompt_NoRecentKept(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	var toCompress []*TimelineItem
	for i := int64(1); i <= 2; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, "x"))
		item, _ := tl.idToTimelineItem.Get(i)
		toCompress = append(toCompress, item)
	}
	out := tl.renderBatchCompressPrompt(toCompress, nil, "NOREC")
	require.NotEmpty(t, out)
	require.Contains(t, out, "<|ITEMS_TO_COMPRESS_NOREC|>")
	require.NotContains(t, out, "<|RECENT_KEEP_NOREC|>", "RECENT_KEEP block must be omitted when recent is empty")
}

// TestBuildRecentKeptString_Truncation recentKeep 超过 budget 时应截断且加 truncate notice
// 关键词: buildRecentKeptString budget 截断
func TestBuildRecentKeptString_Truncation(t *testing.T) {
	// 构造 5 个 ~1KB item，budget 设为 2KB
	var items []*TimelineItem
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	for i := int64(1); i <= 5; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, strings.Repeat("A", 800)))
		item, _ := tl.idToTimelineItem.Get(i)
		items = append(items, item)
	}

	out, count, truncated := buildRecentKeptString(items, 2048)
	require.True(t, truncated, "should be truncated when total > budget")
	require.Less(t, count, 5, "fewer than all 5 items should fit in 2KB budget")
	require.Greater(t, count, 0)
	require.Contains(t, out, "earlier recent items truncated due to size budget")
	// 必须保留最新若干条（id=5 一定在结果里）
	require.Contains(t, out, "[5]")
}

// TestBuildItemsToCompressString_Truncation 待压缩段超 budget 时应截断
// 关键词: buildItemsToCompressString budget 截断
func TestBuildItemsToCompressString_Truncation(t *testing.T) {
	var items []*TimelineItem
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	for i := int64(1); i <= 10; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "tool", true, strings.Repeat("B", 1024)))
		item, _ := tl.idToTimelineItem.Get(i)
		items = append(items, item)
	}

	out, count, truncated := buildItemsToCompressString(items, 4096)
	require.True(t, truncated)
	require.Less(t, count, 10)
	require.Greater(t, count, 0)
	require.Contains(t, out, "more items truncated due to size limit")
	// 最旧的几条必须保留（id=1 在结果里）
	require.Contains(t, out, "[1]")
}

// TestRenderBatchCompressPrompt_RecentBudgetRespectsToCompress recent 段总量超 budget 时
// 应被部分截断，但 toCompress 段必须不被 recent 抢走预算导致失败
// 关键词: renderBatchCompressPrompt 预算分配 toCompress 优先级
func TestRenderBatchCompressPrompt_RecentBudgetRespectsToCompress(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	var toCompress []*TimelineItem
	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "old", true, "old-keyword-XYZ"))
		item, _ := tl.idToTimelineItem.Get(i)
		toCompress = append(toCompress, item)
	}
	// 构造 5 条中等大小的 recentKeep item，每条 ~6KB，合计 ~30KB > 16KB budget
	var recentKeep []*TimelineItem
	for i := int64(100); i <= 104; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "rec", true, strings.Repeat("R", 6*1024)))
		item, _ := tl.idToTimelineItem.Get(i)
		recentKeep = append(recentKeep, item)
	}

	out := tl.renderBatchCompressPrompt(toCompress, recentKeep, "BUDGET")
	require.NotEmpty(t, out)
	// toCompress 段必须可见
	require.Contains(t, out, "old-keyword-XYZ", "toCompress must remain visible even when recent contends for budget")
	// recent 段被截断渲染（保留最新若干 + truncate notice）
	require.Contains(t, out, "<|RECENT_KEEP_BUDGET|>", "RECENT_KEEP segment must still appear with at least one item")
	require.Contains(t, out, "earlier recent items truncated", "RECENT_KEEP truncate notice expected")
	// id=104（最新）必须出现在 RECENT_KEEP 段
	require.Contains(t, out, "[5]", "newest recent item (with seq=5) must be kept after truncation")
}

// TestRenderBatchCompressPrompt_SingleHugeRecentDropped 单条 recent item 大过 budget 时整段被丢弃
// 但 toCompress 仍应正常渲染（这是预期且合理的边界）
// 关键词: renderBatchCompressPrompt 单条超大 recent 边界
func TestRenderBatchCompressPrompt_SingleHugeRecentDropped(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	var toCompress []*TimelineItem
	for i := int64(1); i <= 2; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second),
			makeToolResult(i, "old", true, "stay-visible"))
		item, _ := tl.idToTimelineItem.Get(i)
		toCompress = append(toCompress, item)
	}
	hugeData := strings.Repeat("H", MaxBatchCompressRecentSize+5000)
	injectTimelineItem(tl, 999, baseTs.Add(999*time.Second), makeToolResult(999, "rec", true, hugeData))
	bigItem, _ := tl.idToTimelineItem.Get(int64(999))
	recentKeep := []*TimelineItem{bigItem}

	out := tl.renderBatchCompressPrompt(toCompress, recentKeep, "HUGE")
	require.NotEmpty(t, out)
	require.Contains(t, out, "stay-visible", "toCompress must still render even if single recent item exceeds budget")
	require.NotContains(t, out, "<|RECENT_KEEP_HUGE|>", "RECENT_KEEP must be dropped when no item fits in its budget")
}

// TestRenderBatchCompressPrompt_EmptyToCompress 待压缩段为空，应返回空 prompt
// 关键词: renderBatchCompressPrompt 空 toCompress
func TestRenderBatchCompressPrompt_EmptyToCompress(t *testing.T) {
	tl := NewTimeline(nil, nil)
	out := tl.renderBatchCompressPrompt(nil, nil, "EMPTY")
	require.Empty(t, out, "empty toCompress should produce empty prompt")
}
