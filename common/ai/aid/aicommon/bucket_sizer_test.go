package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 关键词: bucket_sizer_test, 动态桶大小, 单测

// TestFixedBucketSizer_ReturnsConstant 固定 sizer 在任何上下文下返回同一值。
// 关键词: FixedBucketSizer, A_Fixed 策略
func TestFixedBucketSizer_ReturnsConstant(t *testing.T) {
	sizer := FixedBucketSizer(48 * 1024)
	for _, items := range []int{0, 1, 10, 100} {
		v := sizer.NextBudget(BucketSizerContext{CurrentBucketItems: items})
		require.Equal(t, int64(48*1024), v)
	}
}

// TestTimeRemainingBucketSizer_DecayBetweenBaseAndMin 时间剩余 100% 时 = base,
// 0% 时 = minBudget, 中间线性。
// 关键词: TimeRemainingBucketSizer, B_TimeRemaining 策略
func TestTimeRemainingBucketSizer_DecayBetweenBaseAndMin(t *testing.T) {
	base := int64(64 * 1024)
	min := int64(8 * 1024)
	sizer := TimeRemainingBucketSizer(base, min)

	bs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	be := bs.Add(3 * time.Minute)

	cases := []struct {
		offsetSec int
		wantMin   int64
		wantMax   int64
	}{
		{0, base - 1, base + 1},    // 0% offset -> base
		{90, 32 * 1024, 33 * 1024}, // 50% -> base/2
		{179, min, min},            // ≈99% offset, 因 min 兜底等于 min
		{200, min, min},            // 桶结束之后 -> min
	}
	for _, c := range cases {
		ctx := BucketSizerContext{
			BucketStart:       bs,
			BucketEnd:         be,
			NextItemCreatedAt: bs.Add(time.Duration(c.offsetSec) * time.Second),
		}
		v := sizer.NextBudget(ctx)
		require.GreaterOrEqual(t, v, c.wantMin, "offset=%ds", c.offsetSec)
		if c.wantMax > 0 {
			require.LessOrEqual(t, v, c.wantMax, "offset=%ds", c.offsetSec)
		}
	}
}

// TestTimeRemainingBucketSizer_RespectsMin 时间到达桶末仍不应低于 minBudget。
// 关键词: TimeRemainingBucketSizer 边界
func TestTimeRemainingBucketSizer_RespectsMin(t *testing.T) {
	sizer := TimeRemainingBucketSizer(64*1024, 4*1024)
	bs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	be := bs.Add(3 * time.Minute)
	ctx := BucketSizerContext{
		BucketStart:       bs,
		BucketEnd:         be,
		NextItemCreatedAt: be.Add(10 * time.Second), // 超出桶末
	}
	require.Equal(t, int64(4*1024), sizer.NextBudget(ctx))
}

// TestEntryAdaptiveBucketSizer_ScalesWithMean 适应平均 entry 字节。
// 关键词: EntryAdaptiveBucketSizer, C_EntryAdaptive 策略
func TestEntryAdaptiveBucketSizer_ScalesWithMean(t *testing.T) {
	sizer := EntryAdaptiveBucketSizer(8, 32*1024, 256*1024)

	cases := []struct {
		mean int
		want int64
	}{
		{0, 32 * 1024},          // 零 mean 走 min
		{1024, 32 * 1024},       // 8x 1024 = 8K < min, 走 min
		{4096, 32 * 1024},       // 8x 4K = 32K = min
		{8192, 8 * 8192},        // 8x 8K = 64K, 中段
		{16384, 8 * 16384},      // 8x 16K = 128K
		{40 * 1024, 256 * 1024}, // 8x 40K = 320K > max, 走 max
	}
	for _, c := range cases {
		v := sizer.NextBudget(BucketSizerContext{RecentEntryMeanBytes: c.mean})
		require.Equal(t, c.want, v, "mean=%d", c.mean)
	}
}

// TestEntryAdaptiveBucketSizer_DefaultsWhenZero 0 输入用默认参数。
// 关键词: EntryAdaptiveBucketSizer 默认值
func TestEntryAdaptiveBucketSizer_DefaultsWhenZero(t *testing.T) {
	sizer := EntryAdaptiveBucketSizer(0, 0, 0) // 全默认: 8 / 32K / 256K
	v := sizer.NextBudget(BucketSizerContext{RecentEntryMeanBytes: 10 * 1024})
	require.Equal(t, int64(80*1024), v)
}

// TestTokenAwareBucketSizer_FixedTokenBudget 按 token 数 × 3.5 字节估算。
// 关键词: TokenAwareBucketSizer, D_TokenAware 策略
func TestTokenAwareBucketSizer_FixedTokenBudget(t *testing.T) {
	sizer := TokenAwareBucketSizer(5000)
	want := int64(float64(5000) * 3.5)
	require.Equal(t, want, sizer.NextBudget(BucketSizerContext{}))
}

// TestDefaultBucketSizer_IsEntryAdaptive 默认 sizer 即 EntryAdaptive(8, 32K, 256K)。
// 关键词: DefaultBucketSizer, 默认推荐
func TestDefaultBucketSizer_IsEntryAdaptive(t *testing.T) {
	sizer := DefaultBucketSizer()
	v := sizer.NextBudget(BucketSizerContext{RecentEntryMeanBytes: 8 * 1024})
	require.Equal(t, int64(8*8*1024), v)
}

// TestTimeline_SetTimelineBucketSizer_TakesPriority 设置 sizer 后 GroupByMinutes
// 走动态路径, 不再用 SetTimelineBucketByteSize 的固定值。
// 关键词: SetTimelineBucketSizer 优先级
func TestTimeline_SetTimelineBucketSizer_TakesPriority(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		injectTimelineItem(tl, int64(i+1), base.Add(time.Duration(i*10)*time.Second),
			makeToolResult(int64(i+1), fmt.Sprintf("t%d", i), true, strings.Repeat("X", 4000)))
	}

	// 不设 sizer: 用很小固定 budget 应该切多个子桶
	tl.SetTimelineBucketByteSize(2 * 1024)
	blocksFixed := tl.GroupByMinutes(3).GetBlocks()
	require.GreaterOrEqual(t, len(blocksFixed), 2, "small fixed budget should produce sub-blocks")

	// 设 sizer 返回 1M (实际上不会切): sizer 优先级更高
	tl.SetTimelineBucketSizer(FixedBucketSizer(1024 * 1024))
	blocksSized := tl.GroupByMinutes(3).GetBlocks()
	require.Equal(t, 1, len(blocksSized), "sizer big budget should produce single block, sizer must override byteSize")

	// 清掉 sizer, 回到 byteSize
	tl.SetTimelineBucketSizer(nil)
	blocksFixed2 := tl.GroupByMinutes(3).GetBlocks()
	require.Equal(t, len(blocksFixed), len(blocksFixed2), "nil sizer must restore byteSize-driven splitting")
}

// TestTimeline_GroupByMinutesAndBytes_BypassSizer 显式调用 GroupByMinutesAndBytes
// 不被 sizer 拦截 (向后兼容)。
// 关键词: GroupByMinutesAndBytes 旁路 sizer
func TestTimeline_GroupByMinutesAndBytes_BypassSizer(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		injectTimelineItem(tl, int64(i+1), base.Add(time.Duration(i*10)*time.Second),
			makeToolResult(int64(i+1), fmt.Sprintf("t%d", i), true, strings.Repeat("X", 4000)))
	}

	// 设大 sizer
	tl.SetTimelineBucketSizer(FixedBucketSizer(1024 * 1024))

	// 显式传 2K, 应该走固定路径切桶
	blocks := tl.GroupByMinutesAndBytes(3, 2*1024).GetBlocks()
	require.GreaterOrEqual(t, len(blocks), 2, "GroupByMinutesAndBytes must bypass sizer")
}

// TestPackTimelineSubBlocksWithSizer_Idempotent 相同输入 sizer 必出相同切桶。
// 关键词: 动态桶幂等性, 主动缓存稳定性前提
func TestPackTimelineSubBlocksWithSizer_Idempotent(t *testing.T) {
	tl1 := NewTimeline(nil, nil)
	tl2 := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		size := 1024 * (1 + (i % 4))
		tr := makeToolResult(int64(i+1), fmt.Sprintf("t%d", i), true, strings.Repeat("Y", size))
		injectTimelineItem(tl1, int64(i+1), base.Add(time.Duration(i*5)*time.Second), tr)
		injectTimelineItem(tl2, int64(i+1), base.Add(time.Duration(i*5)*time.Second), tr)
	}
	sizer := DefaultBucketSizer()
	tl1.SetTimelineBucketSizer(sizer)
	tl2.SetTimelineBucketSizer(sizer)

	r1 := tl1.GroupByMinutes(3).GetAllRenderable().Render("TL")
	r2 := tl2.GroupByMinutes(3).GetAllRenderable().Render("TL")
	require.Equal(t, r1, r2, "sizer-driven group must be byte-equal across identical timelines")

	r1Again := tl1.GroupByMinutes(3).GetAllRenderable().Render("TL")
	require.Equal(t, r1, r1Again, "sizer-driven group must be idempotent across calls")
}

// TestPackTimelineSubBlocksWithSizer_OversizedEntryStillIsolated 单条 entry
// 超过当次 budget 时仍独占一个子桶, 不在 entry 内部切。
//
// 这里用多行 huge payload (而非单行 200KB), 避免触发 ParseStringToRawLines 走
// bufio.Scanner 默认 64KB token 上限的"静默丢弃"边界 (那不是本测试关心的边界,
// 是上游 bufio 限制)。
//
// 关键词: 动态桶大小, 超大 entry 边界, bufio 边界规避
func TestPackTimelineSubBlocksWithSizer_OversizedEntryStillIsolated(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	// 多行 50KB payload (每行 50 char + \n, 共 1000 行)
	hugeLines := strings.TrimRight(strings.Repeat(strings.Repeat("Z", 50)+"\n", 1000), "\n")
	injectTimelineItem(tl, 1, base.Add(5*time.Second), makeToolResult(1, "small", true, strings.Repeat("a", 1024)))
	injectTimelineItem(tl, 2, base.Add(10*time.Second), makeToolResult(2, "huge", true, hugeLines))
	injectTimelineItem(tl, 3, base.Add(15*time.Second), makeToolResult(3, "tail", true, strings.Repeat("b", 1024)))

	tl.SetTimelineBucketSizer(FixedBucketSizer(8 * 1024)) // huge 必然超

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.GreaterOrEqual(t, len(blocks), 2, "oversized entry must trigger sub-block split")
	// 找到包含 huge 那一条的子桶, 应该只有它自己
	var found bool
	for _, b := range blocks {
		if len(b.Items) == 1 && b.Items[0].GetID() == 2 {
			found = true
			break
		}
	}
	require.True(t, found, "huge entry must occupy its own sub-block")
}

// TestPackTimelineSubBlocksWithSizer_FlushCountDecreaseWithBiggerBudget
// budget 越大, flush 次数越少。
// 关键词: 动态桶大小, 单调性
func TestPackTimelineSubBlocksWithSizer_FlushCountDecreaseWithBiggerBudget(t *testing.T) {
	makeTL := func() *Timeline {
		tl := NewTimeline(nil, nil)
		base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 16; i++ {
			injectTimelineItem(tl, int64(i+1), base.Add(time.Duration(i*8)*time.Second),
				makeToolResult(int64(i+1), fmt.Sprintf("t%d", i), true, strings.Repeat("p", 3*1024)))
		}
		return tl
	}
	tlSmall := makeTL()
	tlBig := makeTL()
	tlSmall.SetTimelineBucketSizer(FixedBucketSizer(4 * 1024))
	tlBig.SetTimelineBucketSizer(FixedBucketSizer(128 * 1024))
	require.Greater(t, len(tlSmall.GroupByMinutes(3).GetBlocks()),
		len(tlBig.GroupByMinutes(3).GetBlocks()))
}

// TestDefault64K_NoMoreFrequentSplitThan16K 默认 64K 在合成场景上,
// flush 次数不比旧 16K 多 (兼容性回归保护)。
// 关键词: 64K 默认值, 回归
func TestDefault64K_NoMoreFrequentSplitThan16K(t *testing.T) {
	baseTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"short_query", "dense_tools", "single_huge", "mixed"} {
		sc := BuildSyntheticScenario(name, baseTs)
		old := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: 16 * 1024})
		now := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: 64 * 1024})
		require.LessOrEqual(t, now.FlushCount, old.FlushCount,
			"scenario=%s 64K flush=%d must <= 16K flush=%d", name, now.FlushCount, old.FlushCount)
	}
}
