package aiconfig

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Chat performs a conversation using the default configuration based on user policy
// If tiered configuration is enabled, it selects the model based on the routing policy
// If tiered configuration is not enabled, it falls back to the legacy ai.Chat function
func Chat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	if !IsTieredAIConfig() {
		// Fall back to legacy chat
		return ai.Chat(msg, opts...)
	}

	policy := GetCurrentPolicy()
	return chatWithPolicy(msg, policy, opts...)
}

// IntelligentChat performs a conversation using the intelligent (high-quality) model
// This is suitable for complex reasoning, code generation, and other high-quality tasks
func IntelligentChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	if !IsTieredAIConfig() {
		// Fall back to legacy chat
		log.Debugf("Tiered config not enabled, using legacy chat for IntelligentChat")
		return ai.Chat(msg, opts...)
	}

	return chatWithTier(msg, TierIntelligent, opts...)
}

// LightweightChat performs a conversation using the lightweight model
// This is suitable for simple conversations and fast responses
func LightweightChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	if !IsTieredAIConfig() {
		// Fall back to legacy chat
		log.Debugf("Tiered config not enabled, using legacy chat for LightweightChat")
		return ai.Chat(msg, opts...)
	}

	return chatWithTier(msg, TierLightweight, opts...)
}

// VisionChat performs a conversation using the vision model
// This is suitable for image understanding and image analysis tasks
func VisionChat(msg string, opts ...aispec.AIConfigOption) (string, error) {
	if !IsTieredAIConfig() {
		// Fall back to legacy chat
		log.Debugf("Tiered config not enabled, using legacy chat for VisionChat")
		return ai.Chat(msg, opts...)
	}

	return chatWithTier(msg, TierVision, opts...)
}

// chatWithPolicy performs a conversation based on the routing policy
func chatWithPolicy(msg string, policy RoutingPolicy, opts ...aispec.AIConfigOption) (string, error) {
	config, err := GetModelByPolicy(policy)
	if err != nil {
		log.Warnf("Failed to get model by policy %s: %v, falling back to legacy chat", policy, err)
		return ai.Chat(msg, opts...)
	}

	return chatWithConfig(msg, config, opts...)
}

// chatWithTier performs a conversation using a specific model tier
func chatWithTier(msg string, tier ModelTier, opts ...aispec.AIConfigOption) (string, error) {
	mgr := GetGlobalManager()
	config := mgr.GetFirstConfig(tier)
	if config == nil {
		// Try fallback if enabled
		if !IsFallbackDisabled() && tier != TierLightweight {
			log.Debugf("No config for tier %s, trying fallback to lightweight", tier)
			config = mgr.GetFirstConfig(TierLightweight)
		}

		if config == nil {
			log.Warnf("No config available for tier %s, falling back to legacy chat", tier)
			return ai.Chat(msg, opts...)
		}
	}

	return chatWithConfig(msg, config, opts...)
}

// chatWithConfig performs a conversation using a specific configuration
func chatWithConfig(msg string, config *ypb.ThirdPartyApplicationConfig, opts ...aispec.AIConfigOption) (string, error) {
	chatter, err := CreateChatterFromConfig(config)
	if err != nil {
		log.Warnf("Failed to create chatter from config: %v, falling back to legacy chat", err)
		return ai.Chat(msg, opts...)
	}

	return chatter(msg, opts...)
}
