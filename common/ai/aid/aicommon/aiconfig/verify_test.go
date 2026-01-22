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

	// Test with disabled config (should pass)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: false,
	})
	err := VerifyAIConfig()
	assert.NoError(t, err)

	// Reset for next test
	ResetVerification()

	// Test with enabled config and valid configurations
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyBalance,
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "test-key"},
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
	assert.Contains(t, err.Error(), "no model configurations are available")

	// Reset for next test
	ResetVerification()

	// Test with nil config but enabled flag (should fail via GetTieredAIConfig returning nil)
	// This is a special case that shouldn't happen in practice
	consts.SetTieredAIConfig(nil)
	err = VerifyAIConfig()
	assert.NoError(t, err) // nil config means disabled, so no error
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
		IntelligentConfigs: []*ypb.ThirdPartyApplicationConfig{
			{Type: "aibalance", APIKey: "test-key"},
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
	err = verifyThirdPartyConfig(&ypb.ThirdPartyApplicationConfig{
		Type: "",
	}, "test", 0)
	assert.Error(t, err)

	// Test valid config
	err = verifyThirdPartyConfig(&ypb.ThirdPartyApplicationConfig{
		Type:   "aibalance",
		APIKey: "test-key",
	}, "test", 0)
	assert.NoError(t, err)

	// Test config without API key (should still pass with warning)
	err = verifyThirdPartyConfig(&ypb.ThirdPartyApplicationConfig{
		Type:   "ollama",
		APIKey: "",
	}, "test", 0)
	assert.NoError(t, err)
}
