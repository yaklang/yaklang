package aiconfig

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIConfigManager manages tiered AI model configurations
type AIConfigManager struct {
	mu sync.RWMutex
}

var (
	globalManager     *AIConfigManager
	globalManagerOnce sync.Once
)

// GetGlobalManager returns the global AIConfigManager singleton
func GetGlobalManager() *AIConfigManager {
	globalManagerOnce.Do(func() {
		globalManager = &AIConfigManager{}
	})
	EnsureConfigLoaded()
	return globalManager
}

// IsTieredAIConfig checks if tiered AI model configuration is enabled
// This is used for compatibility checks to determine which config system to use
func IsTieredAIConfig() bool {
	EnsureConfigLoaded()
	return consts.IsTieredAIModelConfigEnabled()
}

// GetCurrentPolicy returns the current user-configured routing policy
func GetCurrentPolicy() RoutingPolicy {
	EnsureConfigLoaded()
	return consts.GetTieredAIRoutingPolicy()
}

// GetIntelligentConfigs returns the intelligent model configurations
func (m *AIConfigManager) GetIntelligentConfigs() []*ypb.AIModelConfig {
	return consts.GetIntelligentAIConfigs()
}

// GetLightweightConfigs returns the lightweight model configurations
func (m *AIConfigManager) GetLightweightConfigs() []*ypb.AIModelConfig {
	return consts.GetLightweightAIConfigs()
}

// GetVisionConfigs returns the vision model configurations
func (m *AIConfigManager) GetVisionConfigs() []*ypb.AIModelConfig {
	return consts.GetVisionAIConfigs()
}

// GetConfigsByTier returns configurations for a specific model tier
func (m *AIConfigManager) GetConfigsByTier(tier ModelTier) []*ypb.AIModelConfig {
	switch tier {
	case TierIntelligent:
		return m.GetIntelligentConfigs()
	case TierLightweight:
		return m.GetLightweightConfigs()
	case TierVision:
		return m.GetVisionConfigs()
	default:
		log.Warnf("Unknown model tier: %s, falling back to intelligent", tier)
		return m.GetIntelligentConfigs()
	}
}

// GetFirstConfig returns the first configuration for a specific tier
// Returns nil if no configuration is available
func (m *AIConfigManager) GetFirstConfig(tier ModelTier) *ypb.AIModelConfig {
	configs := m.GetConfigsByTier(tier)
	if len(configs) == 0 {
		return nil
	}
	return configs[0]
}

// GetFirstConfigByTierAndProviderAndModel returns the first matched config by tier, provider and model.
// providerName maps to AIModelConfig.Provider.Type.
// modelName maps to AIModelConfig.ModelName or the `model` key in ExtraParams.
// Empty providerName or modelName means no filtering on that dimension.
func (m *AIConfigManager) GetFirstConfigByTierAndProviderAndModel(tier ModelTier, providerName, modelName string) *ypb.AIModelConfig {
	configs := m.GetConfigsByTier(tier)
	if len(configs) == 0 {
		return nil
	}

	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		if providerName != "" && !strings.EqualFold(strings.TrimSpace(cfg.GetProvider().GetType()), providerName) {
			continue
		}

		if modelName != "" && !strings.EqualFold(strings.TrimSpace(getModelFromConfig(cfg)), modelName) {
			continue
		}

		return cfg
	}

	return nil
}

func getModelFromConfig(config *ypb.AIModelConfig) string {
	if config == nil {
		return ""
	}
	if strings.TrimSpace(config.GetModelName()) != "" {
		return config.GetModelName()
	}

	for _, param := range config.GetExtraParams() {
		if param == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(param.GetKey()), modelExtraParamKey) {
			return param.GetValue()
		}
	}
	for _, param := range config.GetProvider().GetExtraParams() {
		if param == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(param.GetKey()), modelExtraParamKey) {
			return param.GetValue()
		}
	}

	return ""
}

// GetModelByPolicy returns the appropriate model configuration based on the routing policy
// - auto: automatically selects based on context (defaults to lightweight)
// - performance: uses intelligent model
// - cost: uses lightweight model
// - balance: uses lightweight model by default
func GetModelByPolicy(policy RoutingPolicy) (*ypb.AIModelConfig, error) {
	mgr := GetGlobalManager()

	var config *ypb.AIModelConfig
	switch policy {
	case PolicyPerformance:
		config = mgr.GetFirstConfig(TierIntelligent)
	case PolicyCost:
		config = mgr.GetFirstConfig(TierLightweight)
	case PolicyBalance, PolicyAuto:
		// Balance mode: default to lightweight
		config = mgr.GetFirstConfig(TierLightweight)
	default:
		// Default to lightweight
		config = mgr.GetFirstConfig(TierLightweight)
	}

	if config == nil {
		return nil, ErrNoConfigAvailable
	}

	return config, nil
}

// IsFallbackDisabled checks if fallback to lightweight model is disabled
func IsFallbackDisabled() bool {
	return consts.IsTieredAIFallbackDisabled()
}
