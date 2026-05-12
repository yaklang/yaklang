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
