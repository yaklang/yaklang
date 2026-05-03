package aibalance

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: db_user_seen_test, ai_daily_user_seen 单元测试

// uniqueTestDate 生成一个绝对不会与生产 / 历史数据冲突的"未来"日期串。
// 避免对 GROUP BY 类聚合查询造成难以复现的脏数据干扰。
func uniqueTestDate(offsetDays int) string {
	return time.Now().AddDate(0, 0, 100+offsetDays).Format("2006-01-02")
}

// cleanupUserSeenForDate 清空指定 date 的所有 user_seen 行（硬删除）。
func cleanupUserSeenForDate(t *testing.T, date string) {
	require.NoError(t, GetDB().Unscoped().Where("date = ?", date).Delete(&schema.AiDailyUserSeen{}).Error)
}

func TestEnsureUserSeenTable(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
}

func TestRecordDailyUserSeen_InsertIgnore(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	date := uniqueTestDate(1)
	defer cleanupUserSeenForDate(t, date)

	hash := "hash-" + utils.RandStringBytes(8)
	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, hash))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, hash))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, hash))

	var rows []*schema.AiDailyUserSeen
	require.NoError(t, GetDB().Where("date = ? AND source_kind = ? AND user_hash = ?", date, SourceKindAPIKey, hash).Find(&rows).Error)
	require.Len(t, rows, 1, "duplicate (date,kind,hash) inserts should not create extra rows")
}

func TestRecordDailyUserSeen_DifferentSourceKindIsolated(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	date := uniqueTestDate(2)
	defer cleanupUserSeenForDate(t, date)

	hash := "samehash-" + utils.RandStringBytes(8)
	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, hash))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeTrace, hash))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeIP, hash))

	var rows []*schema.AiDailyUserSeen
	require.NoError(t, GetDB().Where("date = ?", date).Find(&rows).Error)
	require.Len(t, rows, 3, "same hash under different source_kind should produce 3 rows")
}

func TestRecordDailyUserSeen_CapDropsAfterLimit(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	date := uniqueTestDate(3)
	defer cleanupUserSeenForDate(t, date)

	// 直接把 counter 提到上限，模拟"已经写满 1M 行"的状态
	counter := loadOrCreateUserSeenCounter(date, SourceKindFreeIP)
	atomic.StoreInt64(counter, userSeenDailyCap)

	for i := 0; i < 5; i++ {
		require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeIP, "after-cap-"+utils.RandStringBytes(6)))
	}
	var count int64
	require.NoError(t, GetDB().Model(&schema.AiDailyUserSeen{}).
		Where("date = ? AND source_kind = ?", date, SourceKindFreeIP).Count(&count).Error)
	assert.Equal(t, int64(0), count, "after cap reached, RecordDailyUserSeen should silently drop new rows")

	// 但如果已经写过同样指纹（cap 之前），刷 last_seen_at 仍然可达（因为先 First 命中走 Update 路径）。
	resetUserSeenCounters()
	hash := "before-cap-" + utils.RandStringBytes(6)
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeIP, hash))
	counter = loadOrCreateUserSeenCounter(date, SourceKindFreeIP)
	atomic.StoreInt64(counter, userSeenDailyCap)
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeIP, hash), "existing fingerprint should still update last_seen_at even after cap")
}

func TestQueryDAUDays_GroupBy(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	end := time.Now().AddDate(0, 0, 110)
	dateA := end.AddDate(0, 0, -2).Format("2006-01-02")
	dateB := end.AddDate(0, 0, -1).Format("2006-01-02")
	dateC := end.Format("2006-01-02")
	defer cleanupUserSeenForDate(t, dateA)
	defer cleanupUserSeenForDate(t, dateB)
	defer cleanupUserSeenForDate(t, dateC)

	// dateA: 2 个 api_key, 1 个 free_trace
	require.NoError(t, RecordDailyUserSeen(dateA, SourceKindAPIKey, "ka1"))
	require.NoError(t, RecordDailyUserSeen(dateA, SourceKindAPIKey, "ka2"))
	require.NoError(t, RecordDailyUserSeen(dateA, SourceKindFreeTrace, "kt1"))

	// dateB: 1 个 free_ip
	require.NoError(t, RecordDailyUserSeen(dateB, SourceKindFreeIP, "kip1"))

	// dateC: 3 个 api_key + 2 个 free_ip
	require.NoError(t, RecordDailyUserSeen(dateC, SourceKindAPIKey, "kc1"))
	require.NoError(t, RecordDailyUserSeen(dateC, SourceKindAPIKey, "kc2"))
	require.NoError(t, RecordDailyUserSeen(dateC, SourceKindAPIKey, "kc3"))
	require.NoError(t, RecordDailyUserSeen(dateC, SourceKindFreeIP, "kcip1"))
	require.NoError(t, RecordDailyUserSeen(dateC, SourceKindFreeIP, "kcip2"))

	days, err := QueryDAUDays(3, end)
	require.NoError(t, err)
	require.Len(t, days, 3)

	idx := map[string]*DAUDay{}
	for _, d := range days {
		idx[d.Date] = d
	}
	require.NotNil(t, idx[dateA])
	require.NotNil(t, idx[dateB])
	require.NotNil(t, idx[dateC])

	assert.Equal(t, int64(2), idx[dateA].APIKey)
	assert.Equal(t, int64(1), idx[dateA].FreeTrace)
	assert.Equal(t, int64(0), idx[dateA].FreeIP)
	assert.Equal(t, int64(3), idx[dateA].Total)

	assert.Equal(t, int64(0), idx[dateB].APIKey)
	assert.Equal(t, int64(0), idx[dateB].FreeTrace)
	assert.Equal(t, int64(1), idx[dateB].FreeIP)
	assert.Equal(t, int64(1), idx[dateB].Total)

	assert.Equal(t, int64(3), idx[dateC].APIKey)
	assert.Equal(t, int64(0), idx[dateC].FreeTrace)
	assert.Equal(t, int64(2), idx[dateC].FreeIP)
	assert.Equal(t, int64(5), idx[dateC].Total)
}

func TestQueryDAUTotalByDate(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	date := uniqueTestDate(7)
	defer cleanupUserSeenForDate(t, date)

	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, "u1"))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindAPIKey, "u2"))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeTrace, "u1"))
	require.NoError(t, RecordDailyUserSeen(date, SourceKindFreeIP, "u3"))

	total, err := QueryDAUTotalByDate(date)
	require.NoError(t, err)
	// (api_key|u1), (api_key|u2), (free_trace|u1), (free_ip|u3) => 4 unique buckets
	assert.Equal(t, int64(4), total)
}

func TestCleanupOldUserSeen(t *testing.T) {
	require.NoError(t, EnsureUserSeenTable())
	resetUserSeenCounters()

	date := time.Now().AddDate(0, 0, -150).Format("2006-01-02")
	freshDate := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	defer cleanupUserSeenForDate(t, date)
	defer cleanupUserSeenForDate(t, freshDate)

	require.NoError(t, GetDB().Create(&schema.AiDailyUserSeen{
		Date: date, SourceKind: SourceKindAPIKey, UserHash: "u-old", LastSeenAt: time.Now(),
	}).Error)
	require.NoError(t, GetDB().Create(&schema.AiDailyUserSeen{
		Date: freshDate, SourceKind: SourceKindAPIKey, UserHash: "u-fresh", LastSeenAt: time.Now(),
	}).Error)

	removed, err := CleanupOldUserSeen(100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, removed, int64(1))

	var count int64
	require.NoError(t, GetDB().Model(&schema.AiDailyUserSeen{}).Where("date = ?", date).Count(&count).Error)
	assert.Equal(t, int64(0), count, "old row should have been deleted")
	require.NoError(t, GetDB().Model(&schema.AiDailyUserSeen{}).Where("date = ?", freshDate).Count(&count).Error)
	assert.Equal(t, int64(1), count, "fresh row should be kept")
}
