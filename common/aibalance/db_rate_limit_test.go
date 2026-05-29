package aibalance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureRateLimitConfigTable(t *testing.T) {
	err := EnsureRateLimitConfigTable()
	require.NoError(t, err, "EnsureRateLimitConfigTable should not fail")
}

func TestGetRateLimitConfig_DefaultValues(t *testing.T) {
	EnsureRateLimitConfigTable()

	GetDB().Exec("DELETE FROM ai_balance_rate_limit_configs WHERE id = 1")

	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, int64(600), cfg.DefaultRPM, "default RPM should be 600")
	assert.Equal(t, int64(3), cfg.FreeUserDelaySec, "default free user delay should be 3")
	assert.Equal(t, "{}", cfg.ModelRPMOverrides, "default model RPM overrides should be empty JSON object")
	assert.Equal(t, "{}", cfg.ModelDelayOverrides, "default model delay overrides should be empty JSON object")

	// 新增的 5 个字段默认值
	// 关键词: AiBalanceRateLimitConfig 默认值 软限额 TPS
	assert.Equal(t, int64(0), cfg.FreeUserDelayMaxSec, "default delay max should be 0")
	assert.Equal(t, int64(0), cfg.FreeUserOutputTPS, "default output TPS should be 0")
	assert.Equal(t, "{}", cfg.ModelOutputTPSOverrides, "default model TPS overrides should be empty JSON object")
	assert.Equal(t, int64(0), cfg.FreeUserTokenSoftLimitM, "default soft limit M should be 0")
	assert.Equal(t, int64(0), cfg.FreeUserSoftLimitTPS, "default soft limit TPS should be 0")
}

// TestSaveRateLimitConfig_RoundtripNewFields 验证 5 个新字段的 roundtrip。
// 关键词: SaveRateLimitConfig roundtrip 新字段
func TestSaveRateLimitConfig_RoundtripNewFields(t *testing.T) {
	EnsureRateLimitConfigTable()

	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)

	cfg.FreeUserDelayMaxSec = 5
	cfg.FreeUserOutputTPS = 25
	cfg.ModelOutputTPSOverrides = `{"slow-free":10,"fast-free":100}`
	cfg.FreeUserTokenSoftLimitM = 600
	cfg.FreeUserSoftLimitTPS = 15

	require.NoError(t, SaveRateLimitConfig(cfg))

	cfg2, err := GetRateLimitConfig()
	require.NoError(t, err)

	assert.Equal(t, int64(5), cfg2.FreeUserDelayMaxSec)
	assert.Equal(t, int64(25), cfg2.FreeUserOutputTPS)
	assert.Equal(t, `{"slow-free":10,"fast-free":100}`, cfg2.ModelOutputTPSOverrides)
	assert.Equal(t, int64(600), cfg2.FreeUserTokenSoftLimitM)
	assert.Equal(t, int64(15), cfg2.FreeUserSoftLimitTPS)
}

func TestSaveRateLimitConfig_Roundtrip(t *testing.T) {
	EnsureRateLimitConfigTable()

	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)

	cfg.DefaultRPM = 1200
	cfg.FreeUserDelaySec = 5
	overrides := map[string]int64{"gpt-4": 100, "claude-3": 200}
	overridesJSON, _ := json.Marshal(overrides)
	cfg.ModelRPMOverrides = string(overridesJSON)
	delayOverrides := map[string]int64{"slow-free": 30, "fast-free": 0}
	delayJSON, _ := json.Marshal(delayOverrides)
	cfg.ModelDelayOverrides = string(delayJSON)

	err = SaveRateLimitConfig(cfg)
	require.NoError(t, err)

	cfg2, err := GetRateLimitConfig()
	require.NoError(t, err)

	assert.Equal(t, int64(1200), cfg2.DefaultRPM)
	assert.Equal(t, int64(5), cfg2.FreeUserDelaySec)

	var parsed map[string]int64
	require.NoError(t, json.Unmarshal([]byte(cfg2.ModelRPMOverrides), &parsed))
	assert.Equal(t, int64(100), parsed["gpt-4"])
	assert.Equal(t, int64(200), parsed["claude-3"])

	var parsedDelay map[string]int64
	require.NoError(t, json.Unmarshal([]byte(cfg2.ModelDelayOverrides), &parsedDelay))
	assert.Equal(t, int64(30), parsedDelay["slow-free"])
	assert.Equal(t, int64(0), parsedDelay["fast-free"])
}

func TestSaveRateLimitConfig_Singleton(t *testing.T) {
	EnsureRateLimitConfigTable()

	cfg, _ := GetRateLimitConfig()
	cfg.DefaultRPM = 999
	SaveRateLimitConfig(cfg)

	cfg2, _ := GetRateLimitConfig()
	cfg2.DefaultRPM = 888
	SaveRateLimitConfig(cfg2)

	cfg3, err := GetRateLimitConfig()
	require.NoError(t, err)
	assert.Equal(t, uint(1), cfg3.ID, "should always use ID=1")
	assert.Equal(t, int64(888), cfg3.DefaultRPM, "should reflect latest save")
}

func TestApplyRateLimitConfig_Integration(t *testing.T) {
	EnsureRateLimitConfigTable()

	cfg := NewServerConfig()
	defer cfg.Close()

	overrides := map[string]int64{"special-model": 42}
	overridesJSON, _ := json.Marshal(overrides)
	delayOverrides := map[string]int64{"slow-free": 30, "fast-free": 0}
	delayJSON, _ := json.Marshal(delayOverrides)

	rlCfg := &AiBalanceRateLimitConfig{
		DefaultRPM:          250,
		FreeUserDelaySec:    10,
		ModelRPMOverrides:   string(overridesJSON),
		ModelDelayOverrides: string(delayJSON),
	}
	rlCfg.ID = 1
	require.NoError(t, SaveRateLimitConfig(rlCfg))

	rlCfg2, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.applyRateLimitConfig(rlCfg2)

	assert.Equal(t, int64(250), cfg.chatRateLimiter.defaultRPM.Load())
	assert.Equal(t, int64(10), cfg.freeUserDelayMinSec)
	assert.Equal(t, int64(42), cfg.chatRateLimiter.getEffectiveRPM("special-model"))
	assert.Equal(t, int64(250), cfg.chatRateLimiter.getEffectiveRPM("generic-model"))

	// per-model delay override should win over the global free-user delay
	// 关键词: GetEffectiveDelay DelayRange 覆盖优先
	slowMin, slowMax := cfg.chatRateLimiter.GetEffectiveDelay("slow-free",
		cfg.freeUserDelayMinSec, cfg.freeUserDelayMaxSec)
	assert.Equal(t, int64(30), slowMin, "per-model delay override min should win")
	assert.Equal(t, int64(0), slowMax, "legacy numeric override stores Max=0")

	fastMin, fastMax := cfg.chatRateLimiter.GetEffectiveDelay("fast-free",
		cfg.freeUserDelayMinSec, cfg.freeUserDelayMaxSec)
	assert.Equal(t, int64(0), fastMin, "explicit zero delay override should win over the global free-user delay")
	assert.Equal(t, int64(0), fastMax)

	genericMin, genericMax := cfg.chatRateLimiter.GetEffectiveDelay("generic-free",
		cfg.freeUserDelayMinSec, cfg.freeUserDelayMaxSec)
	assert.Equal(t, cfg.freeUserDelayMinSec, genericMin,
		"models without override should fall back to global free-user delay min")
	assert.Equal(t, cfg.freeUserDelayMaxSec, genericMax,
		"models without override should fall back to global free-user delay max")
}

func TestApplyRateLimitConfig_NilSafe(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.applyRateLimitConfig(nil)
	assert.Equal(t, int64(defaultRPMValue), cfg.chatRateLimiter.defaultRPM.Load(), "nil config should not crash or change values")
}

func TestApplyRateLimitConfig_EmptyOverrides(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.chatRateLimiter.SetModelRPM("leftover-model", 99)
	cfg.chatRateLimiter.SetModelDelay("leftover-free", 77, 88)
	assert.Equal(t, int64(99), cfg.chatRateLimiter.getEffectiveRPM("leftover-model"))
	leftoverMin, leftoverMax := cfg.chatRateLimiter.GetEffectiveDelay("leftover-free", 1, 2)
	assert.Equal(t, int64(77), leftoverMin)
	assert.Equal(t, int64(88), leftoverMax)

	rlCfg := &AiBalanceRateLimitConfig{
		DefaultRPM:          300,
		FreeUserDelaySec:    1,
		ModelRPMOverrides:   "{}",
		ModelDelayOverrides: "{}",
	}
	cfg.applyRateLimitConfig(rlCfg)

	assert.Equal(t, int64(300), cfg.chatRateLimiter.getEffectiveRPM("leftover-model"),
		"after applying empty overrides, old model RPM should be cleared")
	clearedMin, clearedMax := cfg.chatRateLimiter.GetEffectiveDelay("leftover-free", 1, 2)
	assert.Equal(t, int64(1), clearedMin,
		"after applying empty delay overrides, old model delay should be cleared (fallback Min)")
	assert.Equal(t, int64(2), clearedMax,
		"after applying empty delay overrides, old model delay should be cleared (fallback Max)")
}
