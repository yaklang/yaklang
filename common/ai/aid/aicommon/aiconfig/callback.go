package aiconfig

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GetIntelligentAIModelCallback returns the AI callback for intelligent (high-quality) models
// Suitable for complex reasoning, code generation, and other high-quality tasks
func GetIntelligentAIModelCallback() (aicommon.AICallbackType, error) {
	if !IsTieredAIConfig() {
		return nil, ErrTieredConfigDisabled
	}

	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(TierIntelligent)
	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	return createCallbackFromConfig(config)
}

// GetLightweightAIModelCallback returns the AI callback for lightweight models
// Suitable for simple conversations and fast responses
func GetLightweightAIModelCallback() (aicommon.AICallbackType, error) {
	if !IsTieredAIConfig() {
		return nil, ErrTieredConfigDisabled
	}

	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(TierLightweight)
	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	return createCallbackFromConfig(config)
}

// GetVisionAIModelCallback returns the AI callback for vision models
// Suitable for image understanding and image analysis tasks
func GetVisionAIModelCallback() (aicommon.AICallbackType, error) {
	if !IsTieredAIConfig() {
		return nil, ErrTieredConfigDisabled
	}

	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(TierVision)
	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	return createCallbackFromConfig(config)
}

// GetDefaultAIModelCallback returns the default callback based on user-configured policy
// - auto: automatically select based on context
// - performance: use intelligent model
// - cost: use lightweight model
// - balance: use lightweight model by default
func GetDefaultAIModelCallback() (aicommon.AICallbackType, error) {
	if !IsTieredAIConfig() {
		return nil, ErrTieredConfigDisabled
	}

	policy := GetCurrentPolicy()
	config, err := GetModelByPolicy(policy)
	if err != nil {
		return nil, err
	}

	return createCallbackFromConfig(config)
}

// GetCallbackByTier returns the AI callback for a specific model tier
func GetCallbackByTier(tier ModelTier) (aicommon.AICallbackType, error) {
	switch tier {
	case TierIntelligent:
		return GetIntelligentAIModelCallback()
	case TierLightweight:
		return GetLightweightAIModelCallback()
	case TierVision:
		return GetVisionAIModelCallback()
	default:
		log.Warnf("Unknown model tier: %s, using intelligent model", tier)
		return GetIntelligentAIModelCallback()
	}
}

// createCallbackFromConfig creates an AICallbackType from a ThirdPartyApplicationConfig
func createCallbackFromConfig(config *ypb.ThirdPartyApplicationConfig) (aicommon.AICallbackType, error) {
	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	opts := buildOptionsFromConfig(config)
	return aicommon.LoadAIService(config.Type, opts...)
}

// buildOptionsFromConfig builds aispec.AIConfigOption slice from ThirdPartyApplicationConfig
func buildOptionsFromConfig(config *ypb.ThirdPartyApplicationConfig) []aispec.AIConfigOption {
	var opts []aispec.AIConfigOption

	// Set API key
	if config.APIKey != "" {
		opts = append(opts, aispec.WithAPIKey(config.APIKey))
	}

	// Set domain
	if config.Domain != "" {
		opts = append(opts, aispec.WithDomain(config.Domain))
	}

	// Set type
	if config.Type != "" {
		opts = append(opts, aispec.WithType(config.Type))
	}

	// Extract model from ExtraParams
	if len(config.ExtraParams) > 0 {
		for _, param := range config.ExtraParams {
			if param.Key == "model" {
				opts = append(opts, aispec.WithModel(param.Value))
				break
			}
		}
	}

	return opts
}

// TryGetCallbackWithFallback tries to get a callback for the specified tier
// If the tier is not available and fallback is enabled, it falls back to lightweight model
func TryGetCallbackWithFallback(tier ModelTier) (aicommon.AICallbackType, error) {
	callback, err := GetCallbackByTier(tier)
	if err == nil {
		return callback, nil
	}

	// Check if fallback is disabled
	if IsFallbackDisabled() {
		return nil, err
	}

	// Try fallback to lightweight model
	if tier != TierLightweight {
		log.Debugf("Falling back from %s to lightweight model", tier)
		fallbackCallback, fallbackErr := GetLightweightAIModelCallback()
		if fallbackErr == nil {
			return fallbackCallback, nil
		}
	}

	return nil, err
}

// CreateChatterFromConfig creates a chat function from ThirdPartyApplicationConfig
func CreateChatterFromConfig(config *ypb.ThirdPartyApplicationConfig) (func(string, ...aispec.AIConfigOption) (string, error), error) {
	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	opts := buildOptionsFromConfig(config)
	return ai.LoadChater(config.Type, opts...)
}
