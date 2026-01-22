package consts

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AI primary type, default is "openai"
var _aiPrimaryType string

// GetAIPrimaryType returns the primary type of AI
func GetAIPrimaryType() string {
	return _aiPrimaryType
}

func SetAIPrimaryType(t string) {
	// openai / chatglm / moonshot
	switch t {
	case "openai", "chatglm", "moonshot", "":
	default:
		log.Warnf("unstable AI primary type: %s", t)

	}
	_aiPrimaryType = t
}

// ==================== Tiered AI Model Configuration ====================

// RoutingPolicy defines the model routing strategy
type RoutingPolicy string

const (
	// PolicyAuto automatically selects the appropriate model based on the request
	PolicyAuto RoutingPolicy = "auto"
	// PolicyPerformance prioritizes performance (uses intelligent model)
	PolicyPerformance RoutingPolicy = "performance"
	// PolicyCost prioritizes cost efficiency (uses lightweight model)
	PolicyCost RoutingPolicy = "cost"
	// PolicyBalance balances between performance and cost
	PolicyBalance RoutingPolicy = "balance"
)

// TieredAIConfig stores the tiered AI model configuration
type TieredAIConfig struct {
	// Enabled indicates whether tiered AI model configuration is enabled
	Enabled bool
	// RoutingPolicy defines how to route requests to different models
	RoutingPolicy RoutingPolicy
	// DisableFallback disables fallback to lightweight model when intelligent model fails
	DisableFallback bool
	// IntelligentConfigs contains configurations for high-intelligence models
	IntelligentConfigs []*ypb.ThirdPartyApplicationConfig
	// LightweightConfigs contains configurations for lightweight models
	LightweightConfigs []*ypb.ThirdPartyApplicationConfig
	// VisionConfigs contains configurations for vision models
	VisionConfigs []*ypb.ThirdPartyApplicationConfig
}

// tieredAIConfig stores the global tiered AI configuration
var (
	tieredAIConfig     *TieredAIConfig
	tieredAIConfigLock sync.RWMutex
)

// SetTieredAIConfig sets the global tiered AI configuration
func SetTieredAIConfig(config *TieredAIConfig) {
	tieredAIConfigLock.Lock()
	defer tieredAIConfigLock.Unlock()
	tieredAIConfig = config
	if config != nil {
		log.Debugf("Tiered AI config set: enabled=%v, policy=%s, intelligent=%d, lightweight=%d, vision=%d",
			config.Enabled, config.RoutingPolicy,
			len(config.IntelligentConfigs), len(config.LightweightConfigs), len(config.VisionConfigs))
	}
}

// GetTieredAIConfig returns the global tiered AI configuration
func GetTieredAIConfig() *TieredAIConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	return tieredAIConfig
}

// IsTieredAIModelConfigEnabled checks if tiered AI model configuration is enabled
func IsTieredAIModelConfigEnabled() bool {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return false
	}
	return tieredAIConfig.Enabled
}

// GetTieredAIRoutingPolicy returns the current routing policy
func GetTieredAIRoutingPolicy() RoutingPolicy {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return PolicyBalance // default policy
	}
	if tieredAIConfig.RoutingPolicy == "" {
		return PolicyBalance
	}
	return tieredAIConfig.RoutingPolicy
}

// GetIntelligentAIConfigs returns the intelligent model configurations
func GetIntelligentAIConfigs() []*ypb.ThirdPartyApplicationConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return nil
	}
	return tieredAIConfig.IntelligentConfigs
}

// GetLightweightAIConfigs returns the lightweight model configurations
func GetLightweightAIConfigs() []*ypb.ThirdPartyApplicationConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return nil
	}
	return tieredAIConfig.LightweightConfigs
}

// GetVisionAIConfigs returns the vision model configurations
func GetVisionAIConfigs() []*ypb.ThirdPartyApplicationConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return nil
	}
	return tieredAIConfig.VisionConfigs
}

// IsTieredAIFallbackDisabled checks if fallback to lightweight model is disabled
func IsTieredAIFallbackDisabled() bool {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return false
	}
	return tieredAIConfig.DisableFallback
}
