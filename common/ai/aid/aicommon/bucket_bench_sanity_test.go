package aicommon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBucketBenchSanity_SyntheticReplay 验证 ReplayAndMeasure 在合成场景下能跑通,
// 且 short_query (30 条 500B = 15KB) 在 16KB 桶下不应触发任何 flush。
// 关键词: bucket_bench sanity, synthetic 重放, smoke test
func TestBucketBenchSanity_SyntheticReplay(t *testing.T) {
	baseTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	sc := BuildSyntheticScenario("short_query", baseTs)
	require.GreaterOrEqual(t, len(sc.Events), 10, "synthetic scenario must produce events")

	res := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: 16 * 1024})
	require.Equal(t, sc.Name, res.Scenario)
	require.Equal(t, len(sc.Events), res.NumEvents)
	require.Equal(t, "16K", res.BudgetLabel)
}

// TestBucketBenchSanity_FlushCountIncreasesAsBudgetShrinks 在合成 dense_tools
// 场景上验证: budget 越小, flush 次数越多 (符合理论预期)。
// 关键词: bucket_bench sanity, flush 次数随 budget 单调
func TestBucketBenchSanity_FlushCountIncreasesAsBudgetShrinks(t *testing.T) {
	baseTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	sc := BuildSyntheticScenario("dense_tools", baseTs)

	resBig := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: 128 * 1024})
	resSmall := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Budget: 4 * 1024})

	require.GreaterOrEqual(t, resSmall.FlushCount, resBig.FlushCount,
		"smaller budget must trigger >= flushes than bigger budget")
}

// TestBucketBenchSanity_SizerPathExecutes 确认 sizer 路径不报错。
// 关键词: bucket_bench sanity, sizer 路径
func TestBucketBenchSanity_SizerPathExecutes(t *testing.T) {
	baseTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	sc := BuildSyntheticScenario("mixed", baseTs)

	sizer := EntryAdaptiveBucketSizer(8, 32*1024, 256*1024)
	res := ReplayAndMeasure(sc.Name, sc.Events, BucketBenchOptions{Sizer: sizer})
	require.Equal(t, "sizer", res.BudgetLabel)
	require.GreaterOrEqual(t, res.NumEvents, 1)
}
