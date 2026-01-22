package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestModelTierConstants(t *testing.T) {
	assert.Equal(t, ModelTier("intelligent"), TierIntelligent)
	assert.Equal(t, ModelTier("lightweight"), TierLightweight)
	assert.Equal(t, ModelTier("vision"), TierVision)
}

func TestRoutingPolicyConstants(t *testing.T) {
	assert.Equal(t, consts.PolicyAuto, PolicyAuto)
	assert.Equal(t, consts.PolicyPerformance, PolicyPerformance)
	assert.Equal(t, consts.PolicyCost, PolicyCost)
	assert.Equal(t, consts.PolicyBalance, PolicyBalance)
}

func TestIsTieredAIConfig(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	originalLoaded := IsConfigLoaded()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		if !originalLoaded {
			ResetConfigLoaded()
		}
	}()

	// Reset to prevent EnsureConfigLoaded from interfering
	ResetConfigLoaded()

	// Test when disabled - set configLoaded to true to prevent auto-loading
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: false,
	})
	// Directly test consts function to avoid EnsureConfigLoaded interference
	assert.False(t, consts.IsTieredAIModelConfigEnabled())

	// Test when enabled
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
	})
	assert.True(t, consts.IsTieredAIModelConfigEnabled())

	// Test when nil
	consts.SetTieredAIConfig(nil)
	assert.False(t, consts.IsTieredAIModelConfigEnabled())
}

func TestGetCurrentPolicy(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	// Test default policy
	consts.SetTieredAIConfig(nil)
	assert.Equal(t, PolicyBalance, GetCurrentPolicy())

	// Test with explicit policy
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyPerformance,
	})
	assert.Equal(t, PolicyPerformance, GetCurrentPolicy())
}

func TestAIConfigManager(t *testing.T) {
	mgr := GetGlobalManager()
	assert.NotNil(t, mgr)

	// Test singleton
	mgr2 := GetGlobalManager()
	assert.Same(t, mgr, mgr2)
}

func TestGetConfigsByTier(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	intelligentConfig := &ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "test-key",
	}
	lightweightConfig := &ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "light-key",
	}
	visionConfig := &ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "vision-key",
	}

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{intelligentConfig},
		LightweightConfigs: []*ypb.ThirdPartyApplicationConfig{lightweightConfig},
		VisionConfigs:      []*ypb.ThirdPartyApplicationConfig{visionConfig},
	})

	mgr := GetGlobalManager()

	// Test getting configs by tier
	intelligentConfigs := mgr.GetConfigsByTier(TierIntelligent)
	assert.Len(t, intelligentConfigs, 1)
	assert.Equal(t, "test-key", intelligentConfigs[0].APIKey)

	lightweightConfigs := mgr.GetConfigsByTier(TierLightweight)
	assert.Len(t, lightweightConfigs, 1)
	assert.Equal(t, "light-key", lightweightConfigs[0].APIKey)

	visionConfigs := mgr.GetConfigsByTier(TierVision)
	assert.Len(t, visionConfigs, 1)
	assert.Equal(t, "vision-key", visionConfigs[0].APIKey)
}

func TestGetFirstConfig(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "first"},
			{Type: "aibalance", APIKey: "second"},
		},
	})

	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(TierIntelligent)
	assert.NotNil(t, config)
	assert.Equal(t, "first", config.APIKey)

	// Test with empty configs
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: nil,
	})
	config = mgr.GetFirstConfig(TierIntelligent)
	assert.Nil(t, config)
}

func TestGetModelByPolicy(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	intelligentConfig := &ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "intelligent-key",
	}
	lightweightConfig := &ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "lightweight-key",
	}

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{intelligentConfig},
		LightweightConfigs: []*ypb.ThirdPartyApplicationConfig{lightweightConfig},
	})

	// Test performance policy
	config, err := GetModelByPolicy(PolicyPerformance)
	assert.NoError(t, err)
	assert.Equal(t, "intelligent-key", config.APIKey)

	// Test cost policy
	config, err = GetModelByPolicy(PolicyCost)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.APIKey)

	// Test balance policy
	config, err = GetModelByPolicy(PolicyBalance)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.APIKey)

	// Test auto policy
	config, err = GetModelByPolicy(PolicyAuto)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.APIKey)
}

func TestSelectTierByPolicy(t *testing.T) {
	// Test performance policy (always intelligent)
	assert.Equal(t, TierIntelligent, SelectTierByPolicy(PolicyPerformance, false))
	assert.Equal(t, TierIntelligent, SelectTierByPolicy(PolicyPerformance, true))

	// Test cost policy (always lightweight)
	assert.Equal(t, TierLightweight, SelectTierByPolicy(PolicyCost, false))
	assert.Equal(t, TierLightweight, SelectTierByPolicy(PolicyCost, true))

	// Test balance policy
	assert.Equal(t, TierLightweight, SelectTierByPolicy(PolicyBalance, false))
	assert.Equal(t, TierIntelligent, SelectTierByPolicy(PolicyBalance, true))

	// Test auto policy
	assert.Equal(t, TierLightweight, SelectTierByPolicy(PolicyAuto, false))
	assert.Equal(t, TierIntelligent, SelectTierByPolicy(PolicyAuto, true))
}

func TestIsComplexTask(t *testing.T) {
	// Short simple prompts should not be complex
	assert.False(t, IsComplexTask("hello"))
	assert.False(t, IsComplexTask("what time is it"))

	// Long prompts should be complex
	longPrompt := make([]byte, 600)
	for i := range longPrompt {
		longPrompt[i] = 'a'
	}
	assert.True(t, IsComplexTask(string(longPrompt)))

	// Prompts with complexity keywords should be complex
	assert.True(t, IsComplexTask("please analyze this"))
	assert.True(t, IsComplexTask("write code for me"))
	assert.True(t, IsComplexTask("explain in detail"))
}
