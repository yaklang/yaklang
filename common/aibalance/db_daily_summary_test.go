package aibalance

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: db_daily_summary_test, ai_daily_summary 单元测试

func cleanupDailySummaryForDate(t *testing.T, date string) {
	require.NoError(t, GetDB().Unscoped().Where("date = ?", date).Delete(&schema.AiDailySummary{}).Error)
}

func TestEnsureSummaryTable(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())
}

func TestRecordDailySummaryDelta_FlushUpsert(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())
	resetDailySummaryAccumulator()

	// 用一个唯一 nonce date 隔离
	date := time.Now().AddDate(0, 0, 200).Format("2006-01-02")
	defer cleanupDailySummaryForDate(t, date)
	defer resetDailySummaryAccumulator()

	origNow := nowDateString
	nowDateString = func() string { return date }
	defer func() { nowDateString = origNow }()

	usage := &aispec.ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: &aispec.PromptTokensDetails{CachedTokens: 30},
	}
	for i := 0; i < 5; i++ {
		RecordDailySummaryDelta(usage)
	}

	// 还没 flush 时数据库应没有这一天的行
	var count int64
	require.NoError(t, GetDB().Model(&schema.AiDailySummary{}).Where("date = ?", date).Count(&count).Error)
	assert.Equal(t, int64(0), count)

	require.NoError(t, flushSummaryAccumulator())

	var row schema.AiDailySummary
	require.NoError(t, GetDB().Where("date = ?", date).First(&row).Error)
	assert.Equal(t, int64(5), row.TotalRequests)
	assert.Equal(t, int64(500), row.PromptTokens)
	assert.Equal(t, int64(250), row.CompletionTokens)
	assert.Equal(t, int64(150), row.CachedTokens)

	// 第二次 delta + flush 应该 UPSERT 累加
	RecordDailySummaryDelta(&aispec.ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	})
	require.NoError(t, flushSummaryAccumulator())

	require.NoError(t, GetDB().Where("date = ?", date).First(&row).Error)
	assert.Equal(t, int64(6), row.TotalRequests)
	assert.Equal(t, int64(510), row.PromptTokens)
	assert.Equal(t, int64(255), row.CompletionTokens)
	assert.Equal(t, int64(150), row.CachedTokens, "second delta has no cached_tokens; total stays at 150")
}

func TestDailySummaryAccumulator_DayRollover(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())
	resetDailySummaryAccumulator()

	dayA := time.Now().AddDate(0, 0, 210).Format("2006-01-02")
	dayB := time.Now().AddDate(0, 0, 211).Format("2006-01-02")
	defer cleanupDailySummaryForDate(t, dayA)
	defer cleanupDailySummaryForDate(t, dayB)
	defer resetDailySummaryAccumulator()

	current := dayA
	origNow := nowDateString
	nowDateString = func() string { return current }
	defer func() { nowDateString = origNow }()

	RecordDailySummaryDelta(&aispec.ChatUsage{PromptTokens: 100, TotalTokens: 100})
	RecordDailySummaryDelta(&aispec.ChatUsage{PromptTokens: 200, TotalTokens: 200})

	// 切到 dayB：getOrSwapAccumulator 应在 swap 前自动 flush dayA
	current = dayB
	RecordDailySummaryDelta(&aispec.ChatUsage{PromptTokens: 50, TotalTokens: 50})

	// dayA 的数据应已经被 flush
	var rowA schema.AiDailySummary
	require.NoError(t, GetDB().Where("date = ?", dayA).First(&rowA).Error)
	assert.Equal(t, int64(2), rowA.TotalRequests)
	assert.Equal(t, int64(300), rowA.PromptTokens)

	// dayB 还在内存，需要手动 flush
	require.NoError(t, flushSummaryAccumulator())
	var rowB schema.AiDailySummary
	require.NoError(t, GetDB().Where("date = ?", dayB).First(&rowB).Error)
	assert.Equal(t, int64(1), rowB.TotalRequests)
	assert.Equal(t, int64(50), rowB.PromptTokens)
}

func TestQuerySummaryDays_FillZero(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())

	end := time.Now().AddDate(0, 0, 220)
	dateA := end.AddDate(0, 0, -3).Format("2006-01-02")
	dateB := end.Format("2006-01-02")
	defer cleanupDailySummaryForDate(t, dateA)
	defer cleanupDailySummaryForDate(t, dateB)

	require.NoError(t, GetDB().Create(&schema.AiDailySummary{
		Date: dateA, TotalRequests: 9, PromptTokens: 90,
	}).Error)
	require.NoError(t, GetDB().Create(&schema.AiDailySummary{
		Date: dateB, TotalRequests: 7, PromptTokens: 70,
	}).Error)

	out, err := QuerySummaryDays(5, end)
	require.NoError(t, err)
	require.Len(t, out, 5, "must return exactly 5 days")
	assert.Equal(t, end.AddDate(0, 0, -4).Format("2006-01-02"), out[0].Date, "first day = end-4")
	assert.Equal(t, end.Format("2006-01-02"), out[4].Date, "last day = end")

	// 中间没数据的日期应是 zero row
	gapIdx := 1 // end-3 = dateA, end-2 should be zero
	if out[2].Date != end.AddDate(0, 0, -2).Format("2006-01-02") {
		t.Fatalf("unexpected day-2 date: %s", out[2].Date)
	}
	_ = gapIdx
	zeroRow := out[2]
	assert.Equal(t, int64(0), zeroRow.TotalRequests, "gap day should be zero")
	assert.Equal(t, int64(0), zeroRow.PromptTokens)
}

func TestRecordDailySummaryDelta_NilUsageStillCountsRequest(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())
	resetDailySummaryAccumulator()

	date := time.Now().AddDate(0, 0, 230).Format("2006-01-02")
	defer cleanupDailySummaryForDate(t, date)
	defer resetDailySummaryAccumulator()

	origNow := nowDateString
	nowDateString = func() string { return date }
	defer func() { nowDateString = origNow }()

	for i := 0; i < 3; i++ {
		RecordDailySummaryDelta(nil)
	}
	require.NoError(t, flushSummaryAccumulator())

	var row schema.AiDailySummary
	require.NoError(t, GetDB().Where("date = ?", date).First(&row).Error)
	assert.Equal(t, int64(3), row.TotalRequests)
	assert.Equal(t, int64(0), row.PromptTokens)
	assert.Equal(t, int64(0), row.CompletionTokens)
	assert.Equal(t, int64(0), row.CachedTokens)
}

func TestFlushAccumulator_NoOpOnZero(t *testing.T) {
	require.NoError(t, EnsureSummaryTable())
	resetDailySummaryAccumulator()
	defer resetDailySummaryAccumulator()

	date := time.Now().AddDate(0, 0, 240).Format("2006-01-02")
	defer cleanupDailySummaryForDate(t, date)

	acc := &dailySummaryAccumulator{date: date}
	require.NoError(t, flushAccumulator(acc))

	var count int64
	require.NoError(t, GetDB().Model(&schema.AiDailySummary{}).Where("date = ?", date).Count(&count).Error)
	assert.Equal(t, int64(0), count, "all-zero accumulator should not insert any row")

	// 让 atomic 计数器加上一些值，再 flush 一次（消耗它）
	atomic.AddInt64(&acc.totalRequests, 2)
	require.NoError(t, flushAccumulator(acc))
	require.NoError(t, GetDB().Model(&schema.AiDailySummary{}).Where("date = ?", date).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	_ = utils.RandStringBytes
}
