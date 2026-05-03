package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: cleanup_scheduler_test, 100 天保留窗 + ai_daily_summary 不清理

func TestRunCleanupOnce_RemovesOldDoesNotTouchSummary(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	require.NoError(t, EnsureUserSeenTable())
	require.NoError(t, EnsureSummaryTable())

	wrapper := "wrap-clean-" + utils.RandStringBytes(6)
	defer func() {
		GetDB().Where("wrapper_name = ?", wrapper).Delete(&schema.AiDailyCacheStat{})
	}()

	tagDate := time.Now().AddDate(0, 0, -200).Format("2006-01-02")
	tagDateRecent := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	cleanupUserSeenForDate(t, tagDate)
	cleanupUserSeenForDate(t, tagDateRecent)
	cleanupDailySummaryForDate(t, tagDate)

	// cache stats 200 天前的应被删
	require.NoError(t, GetDB().Create(&schema.AiDailyCacheStat{
		Date:             tagDate,
		WrapperName:      wrapper,
		ModelName:        "old-m",
		ProviderTypeName: "old-t",
		ProviderDomain:   "old-d",
		APIKeyHash:       hashAPIKeyForCache("kold"),
		APIKeyShrink:     "k...",
		RequestCount:     1,
	}).Error)
	// 1 天前的应保留
	require.NoError(t, GetDB().Create(&schema.AiDailyCacheStat{
		Date:             tagDateRecent,
		WrapperName:      wrapper,
		ModelName:        "fresh-m",
		ProviderTypeName: "fresh-t",
		ProviderDomain:   "fresh-d",
		APIKeyHash:       hashAPIKeyForCache("knew"),
		APIKeyShrink:     "k...",
		RequestCount:     5,
	}).Error)

	// user_seen 200 天前的应被删
	require.NoError(t, GetDB().Create(&schema.AiDailyUserSeen{
		Date: tagDate, SourceKind: SourceKindAPIKey, UserHash: "u-old", LastSeenAt: time.Now(),
	}).Error)
	require.NoError(t, GetDB().Create(&schema.AiDailyUserSeen{
		Date: tagDateRecent, SourceKind: SourceKindAPIKey, UserHash: "u-recent", LastSeenAt: time.Now(),
	}).Error)

	// daily_summary 200 天前的应被保留（不清理）
	require.NoError(t, GetDB().Create(&schema.AiDailySummary{
		Date:          tagDate,
		TotalRequests: 99,
	}).Error)

	cacheRows, userRows := runCleanupOnce(StatsRetentionDays)
	assert.GreaterOrEqual(t, cacheRows, int64(1))
	assert.GreaterOrEqual(t, userRows, int64(1))

	var oldCache, oldUser, oldSummary, freshCache, freshUser int64
	require.NoError(t, GetDB().Model(&schema.AiDailyCacheStat{}).
		Where("wrapper_name = ? AND date = ?", wrapper, tagDate).Count(&oldCache).Error)
	require.NoError(t, GetDB().Model(&schema.AiDailyCacheStat{}).
		Where("wrapper_name = ? AND date = ?", wrapper, tagDateRecent).Count(&freshCache).Error)
	require.NoError(t, GetDB().Model(&schema.AiDailyUserSeen{}).
		Where("date = ?", tagDate).Count(&oldUser).Error)
	require.NoError(t, GetDB().Model(&schema.AiDailyUserSeen{}).
		Where("date = ?", tagDateRecent).Count(&freshUser).Error)
	require.NoError(t, GetDB().Model(&schema.AiDailySummary{}).
		Where("date = ?", tagDate).Count(&oldSummary).Error)

	assert.Equal(t, int64(0), oldCache, "old cache row should be removed")
	assert.Equal(t, int64(1), freshCache, "fresh cache row should be kept")
	assert.Equal(t, int64(0), oldUser, "old user_seen row should be removed")
	assert.Equal(t, int64(1), freshUser, "fresh user_seen row should be kept")
	assert.Equal(t, int64(1), oldSummary, "ai_daily_summary should NOT be touched by cleanup")

	// cleanup
	cleanupDailySummaryForDate(t, tagDate)
	cleanupUserSeenForDate(t, tagDate)
	cleanupUserSeenForDate(t, tagDateRecent)
}

func TestNextCleanupAt(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, loc)
	next := nextCleanupAt(now)
	assert.True(t, next.After(now))
	assert.Equal(t, 0, next.Hour())
	assert.Equal(t, 1, next.Minute())
	assert.Equal(t, 0, next.Second())
	assert.Equal(t, 4, next.Day(), "should be next day at 0:01")

	beforeMidnight := time.Date(2026, 5, 3, 0, 0, 30, 0, loc)
	next2 := nextCleanupAt(beforeMidnight)
	assert.Equal(t, 3, next2.Day(), "0:00:30 should target the same day's 0:01")
	assert.Equal(t, 1, next2.Minute())
}
