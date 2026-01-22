package aiconfig

import (
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
	return globalManager
}

// IsTieredAIConfig checks if tiered AI model configuration is enabled
// This is used for compatibility checks to determine which config system to use
func IsTieredAIConfig() bool {
	return consts.IsTieredAIModelConfigEnabled()
}

// GetCurrentPolicy returns the current user-configured routing policy
func GetCurrentPolicy() RoutingPolicy {
	return consts.GetTieredAIRoutingPolicy()
}

// GetIntelligentConfigs returns the intelligent model configurations
func (m *AIConfigManager) GetIntelligentConfigs() []*ypb.ThirdPartyApplicationConfig {
	return consts.GetIntelligentAIConfigs()
}

// GetLightweightConfigs returns the lightweight model configurations
func (m *AIConfigManager) GetLightweightConfigs() []*ypb.ThirdPartyApplicationConfig {
	return consts.GetLightweightAIConfigs()
}

// GetVisionConfigs returns the vision model configurations
func (m *AIConfigManager) GetVisionConfigs() []*ypb.ThirdPartyApplicationConfig {
	return consts.GetVisionAIConfigs()
}

// GetConfigsByTier returns configurations for a specific model tier
func (m *AIConfigManager) GetConfigsByTier(tier ModelTier) []*ypb.ThirdPartyApplicationConfig {
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
func (m *AIConfigManager) GetFirstConfig(tier ModelTier) *ypb.ThirdPartyApplicationConfig {
	configs := m.GetConfigsByTier(tier)
	if len(configs) == 0 {
		return nil
	}
	return configs[0]
}

// GetModelByPolicy returns the appropriate model configuration based on the routing policy
// - auto: automatically selects based on context (defaults to lightweight)
// - performance: uses intelligent model
// - cost: uses lightweight model
// - balance: uses lightweight model by default
func GetModelByPolicy(policy RoutingPolicy) (*ypb.ThirdPartyApplicationConfig, error) {
	mgr := GetGlobalManager()

	var config *ypb.ThirdPartyApplicationConfig
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
