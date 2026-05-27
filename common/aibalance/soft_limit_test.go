package aibalance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: soft_limit_test, 软限额触发与 TPS 综合判定

// resetSoftLimitTestEnv 把 rate-limit config 与全局桶清空，避免相邻测试相互污染。
// 关键词: resetSoftLimitTestEnv
func resetSoftLimitTestEnv(t *testing.T) {
	t.Helper()
	require.NoError(t, EnsureRateLimitConfigTable())
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())
	GetDB().Exec("DELETE FROM ai_balance_rate_limit_configs WHERE id = 1")
	GetDB().Unscoped().Exec("DELETE FROM free_user_daily_token_usage")
}

// TestEvaluateFreeUserSoftLimit_Disabled 验证：软限额阈值/TPS 任一为 0 时不触发。
// 关键词: EvaluateFreeUserSoftLimit disabled
func TestEvaluateFreeUserSoftLimit_Disabled(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 0 // disabled by threshold=0
	cfg.FreeUserSoftLimitTPS = 50
	require.NoError(t, SaveRateLimitConfig(cfg))

	triggered, tps := EvaluateFreeUserSoftLimit("any-free")
	assert.False(t, triggered)
	assert.Equal(t, int64(0), tps)

	cfg.FreeUserTokenSoftLimitM = 10
	cfg.FreeUserSoftLimitTPS = 0 // disabled by TPS=0
	require.NoError(t, SaveRateLimitConfig(cfg))

	triggered, tps = EvaluateFreeUserSoftLimit("any-free")
	assert.False(t, triggered)
	assert.Equal(t, int64(0), tps)
}

// TestEvaluateFreeUserSoftLimit_BelowThreshold 验证：未达到阈值不触发。
// 关键词: EvaluateFreeUserSoftLimit below threshold
func TestEvaluateFreeUserSoftLimit_BelowThreshold(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 100 // 100 M
	cfg.FreeUserSoftLimitTPS = 30
	require.NoError(t, SaveRateLimitConfig(cfg))

	// 全局桶累计 50M token < 100M -> 未触发
	require.NoError(t, AddFreeUserDailyTokenUsage("", 50*FreeUserTokenMUnit, false))

	triggered, tps := EvaluateFreeUserSoftLimit("any-free")
	assert.False(t, triggered)
	assert.Equal(t, int64(0), tps)
}

// TestEvaluateFreeUserSoftLimit_AtThreshold 验证：到达阈值即触发，返回配置 TPS。
// 关键词: EvaluateFreeUserSoftLimit triggered
func TestEvaluateFreeUserSoftLimit_AtThreshold(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 100
	cfg.FreeUserSoftLimitTPS = 30
	require.NoError(t, SaveRateLimitConfig(cfg))

	// 累加到 100M token -> 命中
	require.NoError(t, AddFreeUserDailyTokenUsage("", 100*FreeUserTokenMUnit, false))

	triggered, tps := EvaluateFreeUserSoftLimit("any-free")
	assert.True(t, triggered)
	assert.Equal(t, int64(30), tps)
}

// TestEvaluateFreeUserSoftLimit_ModelExempt 验证：模型豁免时不触发软限额。
// 关键词: EvaluateFreeUserSoftLimit 模型豁免
func TestEvaluateFreeUserSoftLimit_ModelExempt(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 100
	cfg.FreeUserSoftLimitTPS = 30
	overrides := map[string]FreeUserTokenModelOverride{
		"exempt-free": {Exempt: true},
	}
	ovBytes, _ := json.Marshal(overrides)
	cfg.FreeUserTokenModelOverrides = string(ovBytes)
	require.NoError(t, SaveRateLimitConfig(cfg))

	require.NoError(t, AddFreeUserDailyTokenUsage("", 200*FreeUserTokenMUnit, false))

	triggered, _ := EvaluateFreeUserSoftLimit("exempt-free")
	assert.False(t, triggered, "exempt model should not be throttled by soft limit")

	// 同条件下没配 override 的模型仍然被触发
	triggered2, tps2 := EvaluateFreeUserSoftLimit("normal-free")
	assert.True(t, triggered2)
	assert.Equal(t, int64(30), tps2)
}

// TestEvaluateFreeUserSoftLimit_ModelOwnBucket 验证：模型有独立桶时不受全局软限额影响。
// 关键词: EvaluateFreeUserSoftLimit 模型独立桶
func TestEvaluateFreeUserSoftLimit_ModelOwnBucket(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 100
	cfg.FreeUserSoftLimitTPS = 30
	overrides := map[string]FreeUserTokenModelOverride{
		"own-bucket-free": {LimitM: 50},
	}
	ovBytes, _ := json.Marshal(overrides)
	cfg.FreeUserTokenModelOverrides = string(ovBytes)
	require.NoError(t, SaveRateLimitConfig(cfg))

	require.NoError(t, AddFreeUserDailyTokenUsage("", 200*FreeUserTokenMUnit, false))

	triggered, _ := EvaluateFreeUserSoftLimit("own-bucket-free")
	assert.False(t, triggered, "model with own bucket should not be throttled by global soft limit")
}

// TestResolveEffectiveOutputTPS_Strictest 验证：模型 TPS / 全局 TPS / 软限额 TPS 三档取最严。
// 关键词: ResolveEffectiveOutputTPS 取最严
func TestResolveEffectiveOutputTPS_Strictest(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	// 准备：软限额已触发（全局桶 200M >= 100M 阈值）
	cfg, _ := GetRateLimitConfig()
	cfg.FreeUserTokenSoftLimitM = 100
	cfg.FreeUserSoftLimitTPS = 20
	cfg.FreeUserOutputTPS = 40
	require.NoError(t, SaveRateLimitConfig(cfg))
	require.NoError(t, AddFreeUserDailyTokenUsage("", 200*FreeUserTokenMUnit, false))

	sc := NewServerConfig()
	defer sc.Close()
	sc.freeUserOutputTPS = 40
	sc.chatRateLimiter.SetModelOutputTPS("strict-free", 10) // 最严

	// 模型 10、全局 40、软限额 20 -> 取最严 10
	got := sc.ResolveEffectiveOutputTPS("strict-free", true)
	assert.Equal(t, int64(10), got)

	// 没模型 TPS 时，全局 40 vs 软限额 20 -> 20
	got2 := sc.ResolveEffectiveOutputTPS("other-free", true)
	assert.Equal(t, int64(20), got2)

	// 非免费模型不限速
	got3 := sc.ResolveEffectiveOutputTPS("paid-model", false)
	assert.Equal(t, int64(0), got3)
}

// TestResolveEffectiveOutputTPS_NoSoftLimit 验证：未触发软限额时仅看模型/全局 TPS。
// 关键词: ResolveEffectiveOutputTPS no soft limit
func TestResolveEffectiveOutputTPS_NoSoftLimit(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	sc := NewServerConfig()
	defer sc.Close()
	sc.freeUserOutputTPS = 30
	sc.chatRateLimiter.SetModelOutputTPS("with-model-tps-free", 50)

	// 模型 50 vs 全局 30 -> 30
	got := sc.ResolveEffectiveOutputTPS("with-model-tps-free", true)
	assert.Equal(t, int64(30), got)

	// 没模型 TPS、没软限额 -> 30
	got2 := sc.ResolveEffectiveOutputTPS("only-global-free", true)
	assert.Equal(t, int64(30), got2)

	// 全局 0 + 模型 0 -> 0
	sc.freeUserOutputTPS = 0
	sc.chatRateLimiter.ClearModelOutputTPS()
	got3 := sc.ResolveEffectiveOutputTPS("any-free", true)
	assert.Equal(t, int64(0), got3)
}

// TestPickStricterTPS 验证 0 视作 "无限制" 的取最严语义。
// 关键词: pickStricterTPS 单元测试
func TestPickStricterTPS(t *testing.T) {
	assert.Equal(t, int64(10), pickStricterTPS(10, 0))
	assert.Equal(t, int64(10), pickStricterTPS(0, 10))
	assert.Equal(t, int64(0), pickStricterTPS(0, 0))
	assert.Equal(t, int64(5), pickStricterTPS(5, 10))
	assert.Equal(t, int64(5), pickStricterTPS(10, 5))
	assert.Equal(t, int64(7), pickStricterTPS(7, 7))
}

// TestSchema_AiBalanceRateLimitConfig_Defaults 验证 schema 默认值。
// 关键词: schema defaults soft-limit fields
func TestSchema_AiBalanceRateLimitConfig_Defaults(t *testing.T) {
	resetSoftLimitTestEnv(t)
	defer resetSoftLimitTestEnv(t)

	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	assert.Equal(t, int64(0), cfg.FreeUserDelayMaxSec)
	assert.Equal(t, int64(0), cfg.FreeUserOutputTPS)
	assert.Equal(t, "{}", cfg.ModelOutputTPSOverrides)
	assert.Equal(t, int64(0), cfg.FreeUserTokenSoftLimitM)
	assert.Equal(t, int64(0), cfg.FreeUserSoftLimitTPS)
	// 确保旧字段没坏
	_ = schema.AiBalanceRateLimitConfig{}
}
