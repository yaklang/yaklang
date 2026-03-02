package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestModelTierConstants(t *testing.T) {
	assert.Equal(t, consts.ModelTier("intelligent"), consts.TierIntelligent)
	assert.Equal(t, consts.ModelTier("lightweight"), consts.TierLightweight)
	assert.Equal(t, consts.ModelTier("vision"), consts.TierVision)
}

func TestRoutingPolicyConstants(t *testing.T) {
	assert.Equal(t, consts.PolicyAuto, consts.PolicyAuto)
	assert.Equal(t, consts.PolicyPerformance, consts.PolicyPerformance)
	assert.Equal(t, consts.PolicyCost, consts.PolicyCost)
	assert.Equal(t, consts.PolicyBalance, consts.PolicyBalance)
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
	assert.Equal(t, consts.PolicyBalance, GetCurrentPolicy())

	// Test with explicit policy
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyPerformance,
	})
	assert.Equal(t, consts.PolicyPerformance, GetCurrentPolicy())
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

	intelligentConfig := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test-key"},
		ModelName: "intelligent-model",
	}
	lightweightConfig := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "light-key"},
		ModelName: "lightweight-model",
	}
	visionConfig := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "vision-key"},
		ModelName: "vision-model",
	}

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.AIModelConfig{intelligentConfig},
		LightweightConfigs: []*ypb.AIModelConfig{lightweightConfig},
		VisionConfigs:      []*ypb.AIModelConfig{visionConfig},
	})

	mgr := &AIConfigManager{}

	// Test getting configs by tier
	intelligentConfigs := mgr.GetConfigsByTier(consts.TierIntelligent)
	assert.Len(t, intelligentConfigs, 1)
	assert.Equal(t, "test-key", intelligentConfigs[0].GetProvider().GetAPIKey())

	lightweightConfigs := mgr.GetConfigsByTier(consts.TierLightweight)
	assert.Len(t, lightweightConfigs, 1)
	assert.Equal(t, "light-key", lightweightConfigs[0].GetProvider().GetAPIKey())

	visionConfigs := mgr.GetConfigsByTier(consts.TierVision)
	assert.Len(t, visionConfigs, 1)
	assert.Equal(t, "vision-key", visionConfigs[0].GetProvider().GetAPIKey())
}

func TestGetFirstConfig(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "first"}, ModelName: "first-model"},
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "second"}, ModelName: "second-model"},
		},
	})

	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(consts.TierIntelligent)
	assert.NotNil(t, config)
	assert.Equal(t, "first", config.GetProvider().GetAPIKey())

	// Test with empty configs
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: nil,
	})
	config = mgr.GetFirstConfig(consts.TierIntelligent)
	assert.Nil(t, config)
}

func TestGetFirstConfigByTierAndProviderAndModel(t *testing.T) {
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{
				Provider:  &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "first"},
				ModelName: "gpt-4o",
			},
			{
				Provider:  &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "second"},
				ModelName: "gpt-4.1",
			},
			{
				Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "third"},
				ModelName: "memfit-standard-free",
			},
		},
	})

	mgr := &AIConfigManager{}

	config := mgr.GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "openai", "gpt-4.1")
	assert.NotNil(t, config)
	assert.Equal(t, "second", config.GetProvider().GetAPIKey())

	config = mgr.GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "OPENAI", "GPT-4O")
	assert.NotNil(t, config)
	assert.Equal(t, "first", config.GetProvider().GetAPIKey())

	config = mgr.GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "", "gpt-4o")
	assert.NotNil(t, config)
	assert.Equal(t, "first", config.GetProvider().GetAPIKey())

	config = mgr.GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "aibalance", "")
	assert.NotNil(t, config)
	assert.Equal(t, "third", config.GetProvider().GetAPIKey())

	config = mgr.GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "openai", "not-exist")
	assert.Nil(t, config)
}

func TestPromoteFirstConfigByTierAndProviderAndModel(t *testing.T) {
	originalLoaded := configLoaded
	originalGetAIGlobalConfig := getAIGlobalConfig
	originalSaveAIGlobalConfigForManager := saveAIGlobalConfigForManager
	originalApplyAIGlobalConfigForManager := applyAIGlobalConfigForManager
	defer func() {
		configLoaded = originalLoaded
		getAIGlobalConfig = originalGetAIGlobalConfig
		saveAIGlobalConfigForManager = originalSaveAIGlobalConfigForManager
		applyAIGlobalConfigForManager = originalApplyAIGlobalConfigForManager
	}()

	// Bypass EnsureConfigLoaded side effects in this unit test.
	configLoaded = true

	cfg := &ypb.AIGlobalConfig{
		Enabled: true,
		IntelligentModels: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "first"}, ModelName: "gpt-4o"},
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "second"}, ModelName: "gpt-4.1"},
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "third"}, ModelName: "memfit-standard-free"},
		},
	}

	getAIGlobalConfig = func() (*ypb.AIGlobalConfig, error) { return cfg, nil }

	saveCalled := false
	applyCalled := false
	saveAIGlobalConfigForManager = func(saved *ypb.AIGlobalConfig) (*ypb.AIGlobalConfig, error) {
		saveCalled = true
		assert.Equal(t, "second", saved.GetIntelligentModels()[0].GetProvider().GetAPIKey())
		return saved, nil
	}
	applyAIGlobalConfigForManager = func(applied *ypb.AIGlobalConfig) error {
		applyCalled = true
		assert.Equal(t, "second", applied.GetIntelligentModels()[0].GetProvider().GetAPIKey())
		return nil
	}

	mgr := &AIConfigManager{}
	err := mgr.PromoteFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "openai", "gpt-4.1")
	assert.NoError(t, err)
	assert.True(t, saveCalled)
	assert.True(t, applyCalled)
	assert.Equal(t, "second", cfg.GetIntelligentModels()[0].GetProvider().GetAPIKey())
}

func TestPromoteFirstConfigByTierAndProviderAndModel_NotFound(t *testing.T) {
	originalLoaded := configLoaded
	originalGetAIGlobalConfig := getAIGlobalConfig
	originalSaveAIGlobalConfigForManager := saveAIGlobalConfigForManager
	originalApplyAIGlobalConfigForManager := applyAIGlobalConfigForManager
	defer func() {
		configLoaded = originalLoaded
		getAIGlobalConfig = originalGetAIGlobalConfig
		saveAIGlobalConfigForManager = originalSaveAIGlobalConfigForManager
		applyAIGlobalConfigForManager = originalApplyAIGlobalConfigForManager
	}()

	configLoaded = true
	cfg := &ypb.AIGlobalConfig{
		Enabled: true,
		IntelligentModels: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "first"}, ModelName: "gpt-4o"},
		},
	}
	getAIGlobalConfig = func() (*ypb.AIGlobalConfig, error) { return cfg, nil }

	saveCalled := false
	applyCalled := false
	saveAIGlobalConfigForManager = func(saved *ypb.AIGlobalConfig) (*ypb.AIGlobalConfig, error) {
		saveCalled = true
		return saved, nil
	}
	applyAIGlobalConfigForManager = func(applied *ypb.AIGlobalConfig) error {
		applyCalled = true
		return nil
	}

	mgr := &AIConfigManager{}
	err := mgr.PromoteFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, "openai", "gpt-4.1")
	assert.Error(t, err)
	assert.False(t, saveCalled)
	assert.False(t, applyCalled)
	assert.Equal(t, "first", cfg.GetIntelligentModels()[0].GetProvider().GetAPIKey())
}

func TestGetModelByPolicy(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(originalConfig)

	intelligentConfig := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "intelligent-key"},
		ModelName: "intelligent-model",
	}
	lightweightConfig := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "lightweight-key"},
		ModelName: "lightweight-model",
	}

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.AIModelConfig{intelligentConfig},
		LightweightConfigs: []*ypb.AIModelConfig{lightweightConfig},
	})

	// Test performance policy
	config, err := GetModelByPolicy(consts.PolicyPerformance)
	assert.NoError(t, err)
	assert.Equal(t, "intelligent-key", config.GetProvider().GetAPIKey())

	// Test cost policy
	config, err = GetModelByPolicy(consts.PolicyCost)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.GetProvider().GetAPIKey())

	// Test balance policy
	config, err = GetModelByPolicy(consts.PolicyBalance)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.GetProvider().GetAPIKey())

	// Test auto policy
	config, err = GetModelByPolicy(consts.PolicyAuto)
	assert.NoError(t, err)
	assert.Equal(t, "lightweight-key", config.GetProvider().GetAPIKey())
}

func TestSelectTierByPolicy(t *testing.T) {
	// Test performance policy (always intelligent)
	assert.Equal(t, consts.TierIntelligent, SelectTierByPolicy(consts.PolicyPerformance, false))
	assert.Equal(t, consts.TierIntelligent, SelectTierByPolicy(consts.PolicyPerformance, true))

	// Test cost policy (always lightweight)
	assert.Equal(t, consts.TierLightweight, SelectTierByPolicy(consts.PolicyCost, false))
	assert.Equal(t, consts.TierLightweight, SelectTierByPolicy(consts.PolicyCost, true))

	// Test balance policy
	assert.Equal(t, consts.TierLightweight, SelectTierByPolicy(consts.PolicyBalance, false))
	assert.Equal(t, consts.TierIntelligent, SelectTierByPolicy(consts.PolicyBalance, true))

	// Test auto policy
	assert.Equal(t, consts.TierLightweight, SelectTierByPolicy(consts.PolicyAuto, false))
	assert.Equal(t, consts.TierIntelligent, SelectTierByPolicy(consts.PolicyAuto, true))
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
