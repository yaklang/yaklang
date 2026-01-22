package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEnsureConfigLoaded(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Reset state
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	// Before loading, config should be nil
	assert.Nil(t, consts.GetTieredAIConfig())
	assert.False(t, IsConfigLoaded())

	// Set up a config
	testConfig := &consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyBalance,
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "test"},
		},
	}
	consts.SetTieredAIConfig(testConfig)

	// Call EnsureConfigLoaded - it should detect config is already loaded
	EnsureConfigLoaded()
}

func TestIsConfigLoaded(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Reset state
	ResetConfigLoaded()
	assert.False(t, IsConfigLoaded())
}

func TestResetConfigLoaded(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Set config loaded state
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true})

	// Reset
	ResetConfigLoaded()
	assert.False(t, IsConfigLoaded())
}

func TestLoadTieredConfigFromNetworkConfig(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Reset state
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	// Create a network config
	networkConfig := &ypb.GlobalNetworkConfig{
		EnableTieredAIModelConfig: true,
		TieredAIModelConfig: &ypb.TieredAIModelConfigDescriptor{
			ModelRoutingPolicy:                "performance",
			DisableFallbackToLightweightModel: true,
		},
		IntelligentAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "intelligent-key"},
		},
		LightweightAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "lightweight-key"},
		},
		VisionAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "vision-key"},
		},
	}

	// Load config
	loadTieredConfigFromNetworkConfig(networkConfig)

	// Verify config was loaded
	tieredConfig := consts.GetTieredAIConfig()
	assert.NotNil(t, tieredConfig)
	assert.True(t, tieredConfig.Enabled)
	assert.Equal(t, consts.PolicyPerformance, tieredConfig.RoutingPolicy)
	assert.True(t, tieredConfig.DisableFallback)
	assert.Len(t, tieredConfig.IntelligentConfigs, 1)
	assert.Len(t, tieredConfig.LightweightConfigs, 1)
	assert.Len(t, tieredConfig.VisionConfigs, 1)
	assert.True(t, IsConfigLoaded())
}

func TestLoadTieredConfigFromNetworkConfig_EmptyPolicy(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Reset state
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	// Create a network config with no TieredAIModelConfig
	networkConfig := &ypb.GlobalNetworkConfig{
		EnableTieredAIModelConfig: true,
		IntelligentAIModelConfig: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "test"},
		},
	}

	// Load config
	loadTieredConfigFromNetworkConfig(networkConfig)

	// Verify default policy is balance
	tieredConfig := consts.GetTieredAIConfig()
	assert.NotNil(t, tieredConfig)
	assert.Equal(t, consts.PolicyBalance, tieredConfig.RoutingPolicy)
}

func TestLoadTieredConfigFromNetworkConfig_NilConfig(t *testing.T) {
	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetConfigLoaded()
	}()

	// Reset state
	consts.SetTieredAIConfig(nil)
	ResetConfigLoaded()

	// Load nil config should not panic
	loadTieredConfigFromNetworkConfig(nil)

	// Config should still be nil
	assert.Nil(t, consts.GetTieredAIConfig())
}
