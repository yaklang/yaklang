package aiconfig

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	verifyOnce   sync.Once
	verifyResult error
)

// VerifyAIConfig performs a one-time global verification of AI configuration
// This function uses sync.Once to ensure the verification is only performed once
// Returns nil if configuration is valid, error otherwise
func VerifyAIConfig() error {
	verifyOnce.Do(func() {
		verifyResult = doVerifyAIConfig()
		if verifyResult != nil {
			log.Warnf("AI configuration verification failed: %v", verifyResult)
		} else {
			log.Debugf("AI configuration verification passed")
		}
	})
	return verifyResult
}

// ResetVerification resets the verification state (mainly for testing)
func ResetVerification() {
	verifyOnce = sync.Once{}
	verifyResult = nil
}

// doVerifyAIConfig performs the actual verification logic
func doVerifyAIConfig() error {
	// Check if tiered config is enabled
	if !consts.IsTieredAIModelConfigEnabled() {
		// If tiered config is not enabled, we don't need to verify it
		// The system will fall back to legacy configuration
		log.Debugf("Tiered AI config is not enabled, skipping tiered config verification")
		return nil
	}

	config := consts.GetTieredAIConfig()
	if config == nil {
		return utils.Error("tiered AI config is nil but enabled flag is set")
	}

	// Verify at least one model tier has configuration
	hasIntelligent := len(config.IntelligentConfigs) > 0
	hasLightweight := len(config.LightweightConfigs) > 0
	hasVision := len(config.VisionConfigs) > 0

	if !hasIntelligent && !hasLightweight && !hasVision {
		return utils.Error("tiered AI config is enabled but no model configurations are available")
	}

	// Verify each tier's configurations
	var errors []string

	if hasIntelligent {
		for i, cfg := range config.IntelligentConfigs {
			if err := verifyThirdPartyConfig(cfg, "intelligent", i); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if hasLightweight {
		for i, cfg := range config.LightweightConfigs {
			if err := verifyThirdPartyConfig(cfg, "lightweight", i); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if hasVision {
		for i, cfg := range config.VisionConfigs {
			if err := verifyThirdPartyConfig(cfg, "vision", i); err != nil {
				errors = append(errors, err.Error())
			}
		}
	}

	if len(errors) > 0 {
		return utils.Errorf("tiered AI config has %d validation errors: %v", len(errors), errors)
	}

	// Verify routing policy is valid
	switch config.RoutingPolicy {
	case consts.PolicyAuto, consts.PolicyPerformance, consts.PolicyCost, consts.PolicyBalance, "":
		// Valid policies
	default:
		log.Warnf("Unknown routing policy: %s, will default to 'balance'", config.RoutingPolicy)
	}

	log.Debugf("Tiered AI config verified: intelligent=%d, lightweight=%d, vision=%d, policy=%s",
		len(config.IntelligentConfigs), len(config.LightweightConfigs), len(config.VisionConfigs), config.RoutingPolicy)

	return nil
}

// verifyThirdPartyConfig verifies a single ThirdPartyApplicationConfig
func verifyThirdPartyConfig(cfg *ypb.ThirdPartyApplicationConfig, tier string, index int) error {
	if cfg == nil {
		return utils.Errorf("%s config[%d] is nil", tier, index)
	}

	if cfg.Type == "" {
		return utils.Errorf("%s config[%d] has empty type", tier, index)
	}

	// APIKey is optional for some providers (e.g., local ollama)
	// So we only warn if it's empty
	if cfg.APIKey == "" {
		log.Debugf("%s config[%d] has empty API key, this may be intentional for local providers", tier, index)
	}

	return nil
}
