package consts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRoutingPolicyConstants(t *testing.T) {
	assert.Equal(t, RoutingPolicy("auto"), PolicyAuto)
	assert.Equal(t, RoutingPolicy("performance"), PolicyPerformance)
	assert.Equal(t, RoutingPolicy("cost"), PolicyCost)
	assert.Equal(t, RoutingPolicy("balance"), PolicyBalance)
}

func TestTieredAIConfig(t *testing.T) {
	// Save original state
	original := GetTieredAIConfig()
	defer SetTieredAIConfig(original)

	// Test nil config
	SetTieredAIConfig(nil)
	assert.Nil(t, GetTieredAIConfig())
	assert.False(t, IsTieredAIModelConfigEnabled())
	assert.Equal(t, PolicyBalance, GetTieredAIRoutingPolicy())

	// Test enabled config
	config := &TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: PolicyPerformance,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{
				Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test"},
				ModelName: "test-model",
			},
		},
	}
	SetTieredAIConfig(config)
	assert.NotNil(t, GetTieredAIConfig())
	assert.True(t, IsTieredAIModelConfigEnabled())
	assert.Equal(t, PolicyPerformance, GetTieredAIRoutingPolicy())

	// Test disabled config
	config.Enabled = false
	SetTieredAIConfig(config)
	assert.False(t, IsTieredAIModelConfigEnabled())
}

func TestGetTieredAIConfigs(t *testing.T) {
	// Save original state
	original := GetTieredAIConfig()
	defer SetTieredAIConfig(original)

	intelligentCfg := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "intelligent-key"},
		ModelName: "intelligent-model",
	}
	lightweightCfg := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "lightweight-key"},
		ModelName: "lightweight-model",
	}
	visionCfg := &ypb.AIModelConfig{
		Provider:  &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "vision-key"},
		ModelName: "vision-model",
	}

	config := &TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.AIModelConfig{intelligentCfg},
		LightweightConfigs: []*ypb.AIModelConfig{lightweightCfg},
		VisionConfigs:      []*ypb.AIModelConfig{visionCfg},
	}
	SetTieredAIConfig(config)

	// Test getting configs
	intelligentConfigs := GetIntelligentAIConfigs()
	assert.Len(t, intelligentConfigs, 1)
	assert.Equal(t, "intelligent-key", intelligentConfigs[0].GetProvider().GetAPIKey())

	lightweightConfigs := GetLightweightAIConfigs()
	assert.Len(t, lightweightConfigs, 1)
	assert.Equal(t, "lightweight-key", lightweightConfigs[0].GetProvider().GetAPIKey())

	visionConfigs := GetVisionAIConfigs()
	assert.Len(t, visionConfigs, 1)
	assert.Equal(t, "vision-key", visionConfigs[0].GetProvider().GetAPIKey())

	// Test with nil config
	SetTieredAIConfig(nil)
	assert.Nil(t, GetIntelligentAIConfigs())
	assert.Nil(t, GetLightweightAIConfigs())
	assert.Nil(t, GetVisionAIConfigs())
}

func TestIsTieredAIFallbackDisabled(t *testing.T) {
	// Save original state
	original := GetTieredAIConfig()
	defer SetTieredAIConfig(original)

	// Test with nil config
	SetTieredAIConfig(nil)
	assert.False(t, IsTieredAIFallbackDisabled())

	// Test with fallback enabled (default)
	SetTieredAIConfig(&TieredAIConfig{
		Enabled:         true,
		DisableFallback: false,
	})
	assert.False(t, IsTieredAIFallbackDisabled())

	// Test with fallback disabled
	SetTieredAIConfig(&TieredAIConfig{
		Enabled:         true,
		DisableFallback: true,
	})
	assert.True(t, IsTieredAIFallbackDisabled())
}

func TestGetTieredAIRoutingPolicyEmptyPolicy(t *testing.T) {
	// Save original state
	original := GetTieredAIConfig()
	defer SetTieredAIConfig(original)

	// Test with empty policy (should default to balance)
	SetTieredAIConfig(&TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: "",
	})
	assert.Equal(t, PolicyBalance, GetTieredAIRoutingPolicy())
}
