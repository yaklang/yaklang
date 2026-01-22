package aiconfig

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	configLoadedOnce sync.Once
	configLoaded     bool
)

func init() {
	// Register a post-init database function to ensure TieredAIConfig is loaded
	// This will be called after the database is initialized
	yakit.RegisterPostInitDatabaseFunction(func() error {
		return ensureConfigLoadedFromDB()
	}, "load-tiered-ai-config")
}

// ensureConfigLoadedFromDB ensures that the tiered AI configuration is loaded from the database
func ensureConfigLoadedFromDB() error {
	configLoadedOnce.Do(func() {
		// The config should already be loaded by ConfigureNetWork in sync-global-config-from-db
		// We just verify that it's available
		tieredConfig := consts.GetTieredAIConfig()
		if tieredConfig != nil && tieredConfig.Enabled {
			log.Debugf("Tiered AI config loaded: policy=%s, intelligent=%d, lightweight=%d, vision=%d",
				tieredConfig.RoutingPolicy,
				len(tieredConfig.IntelligentConfigs),
				len(tieredConfig.LightweightConfigs),
				len(tieredConfig.VisionConfigs))
			configLoaded = true
		} else {
			log.Debugf("Tiered AI config not enabled or not available")
		}
	})
	return nil
}

// EnsureConfigLoaded ensures the tiered AI configuration is loaded
// This is useful for cases where you need to ensure config is loaded before use
// It will try to load from database if not already loaded
func EnsureConfigLoaded() {
	// If already loaded, return immediately
	if configLoaded {
		return
	}

	// Try to load from database
	config := yakit.GetNetworkConfig()
	if config == nil {
		log.Debugf("No network config available in database")
		return
	}

	// Check if tiered config is enabled but not loaded
	if config.GetEnableTieredAIModelConfig() && consts.GetTieredAIConfig() == nil {
		log.Debugf("Tiered AI config enabled but not loaded, loading from network config")
		loadTieredConfigFromNetworkConfig(config)
	}
}

// loadTieredConfigFromNetworkConfig loads tiered config from a GlobalNetworkConfig
func loadTieredConfigFromNetworkConfig(c *ypb.GlobalNetworkConfig) {
	if c == nil {
		return
	}

	tieredConfig := &consts.TieredAIConfig{
		Enabled:            c.GetEnableTieredAIModelConfig(),
		DisableFallback:    false,
		IntelligentConfigs: c.GetIntelligentAIModelConfig(),
		LightweightConfigs: c.GetLightweightAIModelConfig(),
		VisionConfigs:      c.GetVisionAIModelConfig(),
	}

	// Parse routing policy from TieredAIModelConfig
	if c.GetTieredAIModelConfig() != nil {
		policy := c.GetTieredAIModelConfig().GetModelRoutingPolicy()
		switch policy {
		case "auto":
			tieredConfig.RoutingPolicy = consts.PolicyAuto
		case "performance":
			tieredConfig.RoutingPolicy = consts.PolicyPerformance
		case "cost":
			tieredConfig.RoutingPolicy = consts.PolicyCost
		case "balance":
			tieredConfig.RoutingPolicy = consts.PolicyBalance
		default:
			tieredConfig.RoutingPolicy = consts.PolicyBalance
		}
		tieredConfig.DisableFallback = c.GetTieredAIModelConfig().GetDisableFallbackToLightweightModel()
	} else {
		tieredConfig.RoutingPolicy = consts.PolicyBalance
	}

	consts.SetTieredAIConfig(tieredConfig)
	configLoaded = true
	log.Debugf("Tiered AI config loaded via EnsureConfigLoaded")
}

// IsConfigLoaded returns whether the config has been loaded
func IsConfigLoaded() bool {
	return configLoaded
}

// ResetConfigLoaded resets the config loaded state (mainly for testing)
func ResetConfigLoaded() {
	configLoadedOnce = sync.Once{}
	configLoaded = false
}
