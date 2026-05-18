package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: db_free_token_test, FreeUserDailyTokenUsage / 免费用户限额单元测试

func cleanupFreeUserTokenForDate(t *testing.T, date string) {
	require.NoError(t, freeTokenDB().Where("date = ?", date).Delete(&schema.FreeUserDailyTokenUsage{}).Error)
}

func setFreeTokenNowDate(date string) func() {
	orig := freeTokenNowDate
	freeTokenNowDate = func() string { return date }
	return func() { freeTokenNowDate = orig }
}

func setRateLimitConfigForFreeTokenTest(t *testing.T, limitM int64, overridesJSON string) {
	require.NoError(t, EnsureRateLimitConfigTable())
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.FreeUserTokenLimitM = limitM
	cfg.FreeUserTokenModelOverrides = overridesJSON
	require.NoError(t, SaveRateLimitConfig(cfg))
}

func TestEnsureFreeUserDailyTokenUsageTable(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())
}

func TestAddFreeUserDailyTokenUsage_GlobalBucket(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 300).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// modelHasOwnBucket=false -> 写入全局桶
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 1000, false))
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-standard-free", 500, false))

	// 全局桶累加
	used, err := GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), used)

	// 各自模型桶应为 0（没有写入模型桶）
	for _, model := range []string{"memfit-light-free", "memfit-standard-free"} {
		used, err := GetFreeUserDailyTokenUsage(date, model)
		require.NoError(t, err)
		assert.Equal(t, int64(0), used, "model bucket should remain 0 when modelHasOwnBucket=false")
	}
}

func TestAddFreeUserDailyTokenUsage_ModelBucket(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 301).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// modelHasOwnBucket=true -> 只写模型桶，不写全局桶
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 2000, true))
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 1000, true))

	used, err := GetFreeUserDailyTokenUsage(date, "memfit-light-free")
	require.NoError(t, err)
	assert.Equal(t, int64(3000), used)

	used, err = GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), used, "global bucket should be untouched when modelHasOwnBucket=true")
}

func TestAddFreeUserDailyTokenUsage_NonPositiveDelta(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 302).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	require.NoError(t, AddFreeUserDailyTokenUsage("foo-free", 0, false))
	require.NoError(t, AddFreeUserDailyTokenUsage("foo-free", -10, false))

	used, err := GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), used)
}

func TestAddFreeUserDailyTokenUsage_DayRollover(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	day1 := time.Now().AddDate(0, 0, 303).Format("2006-01-02")
	day2 := time.Now().AddDate(0, 0, 304).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, day1)
	defer cleanupFreeUserTokenForDate(t, day2)

	restore := setFreeTokenNowDate(day1)
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 999, false))
	restore()

	restore2 := setFreeTokenNowDate(day2)
	defer restore2()
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 100, false))

	usedDay1, err := GetFreeUserDailyTokenUsage(day1, "")
	require.NoError(t, err)
	assert.Equal(t, int64(999), usedDay1)

	usedDay2, err := GetFreeUserDailyTokenUsage(day2, "")
	require.NoError(t, err)
	assert.Equal(t, int64(100), usedDay2, "day 2 bucket starts fresh from 0")
}

func TestCheckFreeUserDailyTokenLimit_GlobalBucket_AllowedAndExceeded(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 305).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 设置全局限额=2M，模型覆盖为空
	setRateLimitConfigForFreeTokenTest(t, 2, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	d, err := CheckFreeUserDailyTokenLimit("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.False(t, d.Exempt)
	assert.Equal(t, "global", d.Bucket)
	assert.Equal(t, int64(0), d.TokensUsed)
	assert.Equal(t, int64(2*FreeUserTokenMUnit), d.TokensLimit)

	// 累加到刚好超出
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 2*FreeUserTokenMUnit, false))
	d2, err := CheckFreeUserDailyTokenLimit("memfit-light-free")
	require.NoError(t, err)
	assert.False(t, d2.Allowed, "should reject when used >= limit")
	assert.Equal(t, int64(2*FreeUserTokenMUnit), d2.TokensUsed)
}

func TestCheckFreeUserDailyTokenLimit_ModelBucketOverride(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 306).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 全局 1M，但 memfit-light-free 模型覆盖 5M
	setRateLimitConfigForFreeTokenTest(t, 1, `{"memfit-light-free":{"limit_m":5,"exempt":false}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	d, err := CheckFreeUserDailyTokenLimit("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.True(t, d.ModelHasOwn)
	assert.Equal(t, "model", d.Bucket)
	assert.Equal(t, int64(5*FreeUserTokenMUnit), d.TokensLimit)

	// 累加到模型桶
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 5*FreeUserTokenMUnit, true))
	d2, err := CheckFreeUserDailyTokenLimit("memfit-light-free")
	require.NoError(t, err)
	assert.False(t, d2.Allowed)

	// 没有覆盖的模型仍走全局桶（不受影响）
	d3, err := CheckFreeUserDailyTokenLimit("memfit-standard-free")
	require.NoError(t, err)
	assert.True(t, d3.Allowed)
	assert.Equal(t, "global", d3.Bucket)
	assert.Equal(t, int64(1*FreeUserTokenMUnit), d3.TokensLimit)
}

func TestCheckFreeUserDailyTokenLimit_ModelExempt(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 307).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 全局已用满 + 模型豁免 -> 仍放行
	setRateLimitConfigForFreeTokenTest(t, 1, `{"memfit-test-free":{"limit_m":0,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	require.NoError(t, AddFreeUserDailyTokenUsage("foo-free", 1*FreeUserTokenMUnit, false))

	d, err := CheckFreeUserDailyTokenLimit("memfit-test-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.True(t, d.Exempt)

	// 非豁免模型被全局桶拒绝
	d2, err := CheckFreeUserDailyTokenLimit("foo-free")
	require.NoError(t, err)
	assert.False(t, d2.Allowed)
}

func TestQueryFreeUserTokenUsageSnapshot(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 308).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	setRateLimitConfigForFreeTokenTest(t, 100, `{"memfit-light-free":{"limit_m":50,"exempt":false},"foo-free":{"limit_m":0,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 3*FreeUserTokenMUnit, true))
	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-standard-free", 2*FreeUserTokenMUnit, false))

	global, perModel, gotDate, err := QueryFreeUserTokenUsageSnapshot()
	require.NoError(t, err)
	assert.Equal(t, date, gotDate)
	assert.Equal(t, int64(2*FreeUserTokenMUnit), global.TokensUsed)
	assert.Equal(t, int64(100), global.LimitM)

	// per-model 必须包含 memfit-light-free（有数据 + 配置）和 foo-free（仅配置 exempt）
	modelMap := make(map[string]FreeUserTokenBucketSnapshot)
	for _, m := range perModel {
		modelMap[m.Model] = m
	}
	assert.Equal(t, int64(50), modelMap["memfit-light-free"].LimitM)
	assert.Equal(t, int64(3*FreeUserTokenMUnit), modelMap["memfit-light-free"].TokensUsed)
	assert.True(t, modelMap["foo-free"].Exempt)
}

// setFreeTokenWallClock 临时替换 freeTokenWallClock，便于断言切日边界。
// 返回 restore 函数恢复原值，与 setFreeTokenNowDate 风格一致。
// 关键词: setFreeTokenWallClock, 单元测试 wall clock mock
func setFreeTokenWallClock(now time.Time) func() {
	orig := freeTokenWallClock
	freeTokenWallClock = func() time.Time { return now }
	return func() { freeTokenWallClock = orig }
}

// TestFreeTokenNowDate_BeijingSixAMBoundary 验证免费用户日 Token 限额的切日时点
// 落在「北京时间每日 06:00」：
//   - 北京时间 05:59:59 仍属于昨日桶
//   - 北京时间 06:00:00 切换到今日桶
//   - 跨时区 wall clock（UTC 22:00 等价于北京时间次日 06:00）也应当切日
//
// 关键词: TestFreeTokenNowDate_BeijingSixAMBoundary, 北京时间 6 点切日边界
func TestFreeTokenNowDate_BeijingSixAMBoundary(t *testing.T) {
	// 北京时间 2026-05-19 05:59:59 -> 应归入 2026-05-18
	beijing0559 := time.Date(2026, 5, 19, 5, 59, 59, 0, beijingTZ)
	restore1 := setFreeTokenWallClock(beijing0559)
	got1 := freeTokenNowDate()
	restore1()
	assert.Equal(t, "2026-05-18", got1,
		"Beijing 05:59:59 must still belong to yesterday's bucket")

	// 北京时间 2026-05-19 06:00:00 -> 应切换到 2026-05-19
	beijing0600 := time.Date(2026, 5, 19, 6, 0, 0, 0, beijingTZ)
	restore2 := setFreeTokenWallClock(beijing0600)
	got2 := freeTokenNowDate()
	restore2()
	assert.Equal(t, "2026-05-19", got2,
		"Beijing 06:00:00 must roll over to a new bucket")

	// UTC 2026-05-18 22:00:00 == 北京时间 2026-05-19 06:00 -> 切到 2026-05-19
	utc2200 := time.Date(2026, 5, 18, 22, 0, 0, 0, time.UTC)
	restore3 := setFreeTokenWallClock(utc2200)
	got3 := freeTokenNowDate()
	restore3()
	assert.Equal(t, "2026-05-19", got3,
		"cross-timezone wall clock should respect Beijing 06:00 cutover")

	// 北京时间正午 -> 当天日期
	beijingNoon := time.Date(2026, 5, 19, 12, 0, 0, 0, beijingTZ)
	restore4 := setFreeTokenWallClock(beijingNoon)
	got4 := freeTokenNowDate()
	restore4()
	assert.Equal(t, "2026-05-19", got4, "Beijing noon should be today's bucket")

	// 北京时间凌晨 03:00 -> 应归入前一天
	beijing0300 := time.Date(2026, 5, 19, 3, 0, 0, 0, beijingTZ)
	restore5 := setFreeTokenWallClock(beijing0300)
	got5 := freeTokenNowDate()
	restore5()
	assert.Equal(t, "2026-05-18", got5,
		"Beijing 03:00 still belongs to previous day's bucket")
}

func TestUpdateAiApiKeyTokenUsedAndCheck(t *testing.T) {
	require.NoError(t, GetDB().AutoMigrate(&schema.AiApiKeys{}).Error)

	apiKey := "test-token-key-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{})

	require.NoError(t, GetDB().Create(&schema.AiApiKeys{
		APIKey:           apiKey,
		Active:           true,
		TokenLimit:       1000,
		TokenLimitEnable: true,
	}).Error)

	allowed, err := CheckAiApiKeyTokenLimit(apiKey)
	require.NoError(t, err)
	assert.True(t, allowed)

	require.NoError(t, UpdateAiApiKeyTokenUsed(apiKey, 600))
	require.NoError(t, UpdateAiApiKeyTokenUsed(apiKey, 400))

	allowed2, err := CheckAiApiKeyTokenLimit(apiKey)
	require.NoError(t, err)
	assert.False(t, allowed2, "1000/1000 should be rejected")

	var key schema.AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", apiKey).First(&key).Error)
	assert.Equal(t, int64(1000), key.TokenUsed)
}
