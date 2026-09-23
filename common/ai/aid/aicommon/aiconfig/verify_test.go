package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestVerifyAIConfig(t *testing.T) {
	// Reset verification state before test
	ResetVerification()

	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetVerification()
	}()

	// The legacy Enabled field is ignored when a valid global config exists.
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: false,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test-key"}, ModelName: "test-model"},
		},
	})
	err := VerifyAIConfig()
	assert.NoError(t, err)

	// Reset for next test
	ResetVerification()

	// Test with enabled config and valid configurations
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyBalance,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test-key"}, ModelName: "test-model"},
		},
	})
	err = VerifyAIConfig()
	assert.NoError(t, err)

	// Reset for next test
	ResetVerification()

	// Test with enabled config but no configurations (should fail)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: nil,
		LightweightConfigs: nil,
		VisionConfigs:      nil,
	})
	err = VerifyAIConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no model configurations available")

	// Reset for next test
	ResetVerification()

	// No global config keeps the legacy AI path available and needs no verification.
	consts.SetTieredAIConfig(nil)
	err = VerifyAIConfig()
	assert.NoError(t, err)
}

func TestVerifyOnce(t *testing.T) {
	// Reset verification state
	ResetVerification()

	// Save original state
	originalConfig := consts.GetTieredAIConfig()
	defer func() {
		consts.SetTieredAIConfig(originalConfig)
		ResetVerification()
	}()

	// Set up a valid config
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: true,
		IntelligentConfigs: []*ypb.AIModelConfig{
			{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test-key"}, ModelName: "test-model"},
		},
	})

	// First call should succeed
	err1 := VerifyAIConfig()
	assert.NoError(t, err1)

	// Now change to invalid config
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: nil,
		LightweightConfigs: nil,
		VisionConfigs:      nil,
	})

	// Second call should return cached result (still success)
	err2 := VerifyAIConfig()
	assert.NoError(t, err2)
	assert.Equal(t, err1, err2)
}

func TestVerifyThirdPartyConfig(t *testing.T) {
	// Test nil config
	err := verifyThirdPartyConfig(nil, "test", 0)
	assert.Error(t, err)

	// Test empty type
	err = verifyThirdPartyConfig(&ypb.AIModelConfig{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: ""},
	}, "test", 0)
	assert.Error(t, err)

	// Test valid config
	err = verifyThirdPartyConfig(&ypb.AIModelConfig{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "test-key"},
	}, "test", 0)
	assert.NoError(t, err)

	// Test config without API key (should still pass with warning)
	err = verifyThirdPartyConfig(&ypb.AIModelConfig{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: "ollama", APIKey: ""},
	}, "test", 0)
	assert.NoError(t, err)
}
