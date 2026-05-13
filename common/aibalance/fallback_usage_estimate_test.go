package aibalance

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: fallback_usage_estimate_test, ytoken 兜底扣费单元测试

// setupFallbackTestServerConfig 构造一个最小可用的 ServerConfig，仅供 fallback
// 扣费方法单测使用 (不启动 TCP listener、不依赖 chatRateLimiter)。
// 关键词: setupFallbackTestServerConfig fallback 单测
func setupFallbackTestServerConfig() *ServerConfig {
	return NewServerConfig()
}

// fallbackTestEnsureTables 一次性确保 fallback 测试相关的所有持久化表存在。
// 关键词: fallbackTestEnsureTables
func fallbackTestEnsureTables(t *testing.T) {
	t.Helper()
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())
	require.NoError(t, EnsureRateLimitConfigTable())
	require.NoError(t, GetDB().AutoMigrate(&schema.AiApiKeys{}).Error)
}

// TestApplyUsageFallbackEstimate_EmptyInput_NoBilling 空输入下应不扣费、不抛错。
// 关键词: fallback 估算 空输入 不扣费
func TestApplyUsageFallbackEstimate_EmptyInput_NoBilling(t *testing.T) {
	fallbackTestEnsureTables(t)

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"empty-free", true, nil, "mock-provider",
		"", 0, "", "",
	)
	assert.Equal(t, int64(0), res.EstPromptTokens)
	assert.Equal(t, int64(0), res.EstCompletionTokens)
	assert.Equal(t, int64(0), res.Weighted)
	assert.False(t, res.Billed)
	assert.Equal(t, "", res.Bucket)
}

// TestApplyUsageFallbackEstimate_FreeUser_GlobalBucket 免费模型默认走全局共享池。
// 关键词: fallback 免费模型 全局桶
func TestApplyUsageFallbackEstimate_FreeUser_GlobalBucket(t *testing.T) {
	fallbackTestEnsureTables(t)

	date := time.Now().AddDate(0, 0, 410).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()
	setRateLimitConfigForFreeTokenTest(t, 1200, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	promptText := strings.Repeat("hello world ", 100)
	outputText := strings.Repeat("response text ", 200)
	reasonText := "short reason"

	res := cfg.applyUsageFallbackEstimate(
		"fallback-test-free", true, nil, "mock-provider",
		promptText, 0, outputText, reasonText,
	)
	require.True(t, res.Billed, "free model without override should bill via fallback")
	assert.Equal(t, "global", res.Bucket)
	assert.Greater(t, res.Weighted, int64(0))

	// 估算口径自检：text 部分必须等于 ytoken 计数
	expectedPrompt := int64(ytoken.CalcTokenCount(promptText))
	expectedCompletion := int64(ytoken.CalcTokenCount(outputText) + ytoken.CalcTokenCount(reasonText))
	assert.Equal(t, expectedPrompt, res.EstPromptTokens)
	assert.Equal(t, expectedCompletion, res.EstCompletionTokens)

	used, err := GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, res.Weighted, used, "global bucket should be incremented by exactly weighted")

	usedModel, err := GetFreeUserDailyTokenUsage(date, "fallback-test-free")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usedModel, "model bucket should remain 0 when no override")
}

// TestApplyUsageFallbackEstimate_FreeUser_ModelBucket 免费模型有独立桶时不走全局。
// 关键词: fallback 免费模型 独立桶
func TestApplyUsageFallbackEstimate_FreeUser_ModelBucket(t *testing.T) {
	fallbackTestEnsureTables(t)

	date := time.Now().AddDate(0, 0, 411).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()
	setRateLimitConfigForFreeTokenTest(t, 1200,
		`{"fallback-iso-free":{"limit_m":50,"exempt":false}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"fallback-iso-free", true, nil, "mock-provider",
		strings.Repeat("prompt ", 50), 0,
		strings.Repeat("output ", 80), "",
	)
	require.True(t, res.Billed)
	assert.Equal(t, "model", res.Bucket)

	usedGlobal, err := GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usedGlobal, "global bucket should NOT be touched when model has own bucket")

	usedModel, err := GetFreeUserDailyTokenUsage(date, "fallback-iso-free")
	require.NoError(t, err)
	assert.Equal(t, res.Weighted, usedModel)
}

// TestApplyUsageFallbackEstimate_FreeUser_Exempt 模型 exempt 时 fallback 也必须 skip 扣费。
// 关键词: fallback 免费模型 exempt 跳过扣费
func TestApplyUsageFallbackEstimate_FreeUser_Exempt(t *testing.T) {
	fallbackTestEnsureTables(t)

	date := time.Now().AddDate(0, 0, 412).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()
	setRateLimitConfigForFreeTokenTest(t, 1200,
		`{"fallback-exempt-free":{"limit_m":0,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"fallback-exempt-free", true, nil, "mock-provider",
		strings.Repeat("prompt ", 100), 0,
		strings.Repeat("output ", 100), "",
	)
	assert.Greater(t, res.Weighted, int64(0),
		"estimate should still be computed even for exempt models (for logging)")
	assert.False(t, res.Billed, "exempt model should NEVER be billed")
	assert.Equal(t, "", res.Bucket)

	usedGlobal, err := GetFreeUserDailyTokenUsage(date, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usedGlobal)

	usedModel, err := GetFreeUserDailyTokenUsage(date, "fallback-exempt-free")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usedModel)
}

// TestApplyUsageFallbackEstimate_APIKey_Billed 非免费模型 + 有 key 时落到 APIKey TokenUsed。
// 关键词: fallback APIKey 计费
func TestApplyUsageFallbackEstimate_APIKey_Billed(t *testing.T) {
	fallbackTestEnsureTables(t)

	apiKey := fmt.Sprintf("test-fallback-key-%d", time.Now().UnixNano())
	defer GetDB().Unscoped().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{})

	require.NoError(t, GetDB().Create(&schema.AiApiKeys{
		APIKey:           apiKey,
		Active:           true,
		TokenLimit:       0,
		TokenLimitEnable: false,
	}).Error)

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"paid-model-x", false, &Key{Key: apiKey}, "mock-provider",
		strings.Repeat("prompt ", 100), 0,
		strings.Repeat("output ", 100), "",
	)
	require.True(t, res.Billed)
	assert.Equal(t, "apikey", res.Bucket)
	assert.Greater(t, res.Weighted, int64(0))

	var got schema.AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", apiKey).First(&got).Error)
	assert.Equal(t, res.Weighted, got.TokenUsed,
		"APIKey TokenUsed should be incremented by exactly weighted")
}

// TestApplyUsageFallbackEstimate_APIKey_Missing 非免费模型但 key=nil 时 fallback 不扣费。
// 关键词: fallback APIKey 缺失 不扣费
func TestApplyUsageFallbackEstimate_APIKey_Missing(t *testing.T) {
	fallbackTestEnsureTables(t)

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"paid-model-y", false, nil, "mock-provider",
		strings.Repeat("prompt ", 50), 0,
		strings.Repeat("output ", 50), "",
	)
	assert.Greater(t, res.Weighted, int64(0),
		"estimate should still be computed even when key=nil (for logging)")
	assert.False(t, res.Billed, "no api key -> cannot bill anyone")
	assert.Equal(t, "", res.Bucket)
}

// TestApplyUsageFallbackEstimate_ImagePreCharge image_url 必须按 4K token 预扣。
// 关键词: fallback image 4K 预扣
func TestApplyUsageFallbackEstimate_ImagePreCharge(t *testing.T) {
	fallbackTestEnsureTables(t)

	date := time.Now().AddDate(0, 0, 413).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()
	setRateLimitConfigForFreeTokenTest(t, 1200, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	const imageCount = 3
	promptText := "describe these images"

	res := cfg.applyUsageFallbackEstimate(
		"vision-fallback-free", true, nil, "mock-provider",
		promptText, imageCount, "the photos show ...", "",
	)
	require.True(t, res.Billed)

	// 估算的 prompt = ytoken(promptText) + 3 * 4096
	textTokens := int64(ytoken.CalcTokenCount(promptText))
	expected := textTokens + int64(imageCount)*fallbackImageTokenEstimate
	assert.Equal(t, expected, res.EstPromptTokens,
		"image_url must be pre-charged at %d tokens each", fallbackImageTokenEstimate)
	assert.GreaterOrEqual(t, res.EstPromptTokens,
		int64(imageCount)*fallbackImageTokenEstimate,
		"prompt token estimate should at least cover image pre-charge")
}

// TestApplyUsageFallbackEstimate_PureImageOnly_StillBills 没有 prompt 文本但有 image
// 时也必须能扣费 (vision-only 请求场景)。
// 关键词: fallback 纯 image 请求 仍扣费
func TestApplyUsageFallbackEstimate_PureImageOnly_StillBills(t *testing.T) {
	fallbackTestEnsureTables(t)

	date := time.Now().AddDate(0, 0, 414).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()
	setRateLimitConfigForFreeTokenTest(t, 1200, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	cfg := setupFallbackTestServerConfig()
	defer cfg.Close()

	res := cfg.applyUsageFallbackEstimate(
		"vision-only-free", true, nil, "mock-provider",
		"", 2, "", "",
	)
	require.True(t, res.Billed,
		"pure image (no text) request should still be billed via image pre-charge")
	assert.Equal(t, int64(2*fallbackImageTokenEstimate), res.EstPromptTokens)
	assert.Equal(t, int64(0), res.EstCompletionTokens)
	assert.Greater(t, res.Weighted, int64(0))
}
