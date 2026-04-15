package aibalance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
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
	assert.Equal(t, "{}", cfg.ModelRPMOverrides, "default model overrides should be empty JSON object")
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

	rlCfg := &schema.AiBalanceRateLimitConfig{
		DefaultRPM:        250,
		FreeUserDelaySec:  10,
		ModelRPMOverrides: string(overridesJSON),
	}
	rlCfg.ID = 1
	require.NoError(t, SaveRateLimitConfig(rlCfg))

	rlCfg2, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.applyRateLimitConfig(rlCfg2)

	assert.Equal(t, int64(250), cfg.chatRateLimiter.defaultRPM.Load())
	assert.Equal(t, int64(10), cfg.freeUserDelaySec)
	assert.Equal(t, int64(42), cfg.chatRateLimiter.getEffectiveRPM("special-model"))
	assert.Equal(t, int64(250), cfg.chatRateLimiter.getEffectiveRPM("generic-model"))
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
	assert.Equal(t, int64(99), cfg.chatRateLimiter.getEffectiveRPM("leftover-model"))

	rlCfg := &schema.AiBalanceRateLimitConfig{
		DefaultRPM:        300,
		FreeUserDelaySec:  1,
		ModelRPMOverrides: "{}",
	}
	cfg.applyRateLimitConfig(rlCfg)

	assert.Equal(t, int64(300), cfg.chatRateLimiter.getEffectiveRPM("leftover-model"),
		"after applying empty overrides, old model RPM should be cleared")
}
