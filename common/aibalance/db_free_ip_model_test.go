package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: db_free_ip_model_test, 单 IP 按模型每日用量 + TOP3 聚合单元测试

func cleanupFreeUserIPModelForDate(t *testing.T, date string) {
	require.NoError(t, freeIPModelDB().Where("date = ?", date).Delete(&FreeUserIPModelDailyUsage{}).Error)
}

func TestEnsureFreeUserIPModelDailyUsageTable(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())
}

func TestAddFreeUserIPModelDailyUsage_Accumulate(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())

	date := time.Now().AddDate(0, 0, 410).Format("2006-01-02")
	defer cleanupFreeUserIPModelForDate(t, date)
	defer setFreeTokenNowDate(date)()

	ip := "198.51.100.21"
	require.NoError(t, AddFreeUserIPModelDailyRequest(ip, "model-a"))
	require.NoError(t, AddFreeUserIPModelDailyRequest(ip, "model-a"))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "model-a", 3*FreeUserTokenMUnit))

	res, err := QueryFreeIPTopModelsBatch([]string{ip}, 3)
	require.NoError(t, err)
	rows := res[ip]
	require.Len(t, rows, 1)
	assert.Equal(t, "model-a", rows[0].Model)
	assert.Equal(t, int64(2), rows[0].RequestCount)
	assert.Equal(t, int64(3*FreeUserTokenMUnit), rows[0].TokensUsed)
	assert.InDelta(t, 3.0, rows[0].UsedM, 0.0001)
}

func TestQueryFreeIPTopModelsBatch_Top3Sorted(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())

	date := time.Now().AddDate(0, 0, 411).Format("2006-01-02")
	defer cleanupFreeUserIPModelForDate(t, date)
	defer setFreeTokenNowDate(date)()

	ip := "198.51.100.22"
	// 5 个模型, 加权 token 各不同; 期望按 token 降序取前 3。
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "m1", 1*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "m2", 5*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "m3", 3*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "m4", 9*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "m5", 2*FreeUserTokenMUnit))

	res, err := QueryFreeIPTopModelsBatch([]string{ip}, 3)
	require.NoError(t, err)
	rows := res[ip]
	require.Len(t, rows, 3)
	assert.Equal(t, "m4", rows[0].Model)
	assert.Equal(t, "m2", rows[1].Model)
	assert.Equal(t, "m3", rows[2].Model)
}

func TestQueryFreeIPTopModelsBatch_MultiIPIsolation(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())

	date := time.Now().AddDate(0, 0, 412).Format("2006-01-02")
	defer cleanupFreeUserIPModelForDate(t, date)
	defer setFreeTokenNowDate(date)()

	ipA := "198.51.100.31"
	ipB := "198.51.100.32"
	require.NoError(t, AddFreeUserIPModelDailyTokens(ipA, "alpha", 4*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ipB, "beta", 7*FreeUserTokenMUnit))

	res, err := QueryFreeIPTopModelsBatch([]string{ipA, ipB}, 3)
	require.NoError(t, err)
	require.Len(t, res[ipA], 1)
	require.Len(t, res[ipB], 1)
	assert.Equal(t, "alpha", res[ipA][0].Model)
	assert.Equal(t, "beta", res[ipB][0].Model)
}

func TestQueryFreeIPTopModelsBatch_EmptyInput(t *testing.T) {
	res, err := QueryFreeIPTopModelsBatch(nil, 3)
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestAddFreeUserIPModelDaily_IgnoreEmptyModel(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())

	date := time.Now().AddDate(0, 0, 413).Format("2006-01-02")
	defer cleanupFreeUserIPModelForDate(t, date)
	defer setFreeTokenNowDate(date)()

	ip := "198.51.100.41"
	// 空 model 应被静默跳过, 不报错也不入库。
	require.NoError(t, AddFreeUserIPModelDailyRequest(ip, ""))
	require.NoError(t, AddFreeUserIPModelDailyTokens(ip, "", 5*FreeUserTokenMUnit))

	res, err := QueryFreeIPTopModelsBatch([]string{ip}, 3)
	require.NoError(t, err)
	assert.Empty(t, res[ip])
}

func TestCleanupOldFreeUserIPModelUsage(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPModelDailyUsageTable())

	oldDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	defer cleanupFreeUserIPModelForDate(t, oldDate)
	func() {
		defer setFreeTokenNowDate(oldDate)()
		require.NoError(t, AddFreeUserIPModelDailyTokens("198.51.100.51", "stale", 1*FreeUserTokenMUnit))
	}()

	removed, err := CleanupOldFreeUserIPModelUsage(2)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, removed, int64(1))
}
