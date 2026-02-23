package aiconfig

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

// EnsureConfigLoaded ensures the tiered AI configuration is loaded.
// Priority: 1) database GlobalNetworkConfig  2) config file on disk  3) built-in defaults
func EnsureConfigLoaded() {
	if configLoaded {
		return
	}

	config := yakit.GetNetworkConfig()
	if config != nil && config.GetEnableTieredAIModelConfig() && consts.GetTieredAIConfig() == nil {
		log.Debugf("tiered AI config enabled in DB but not loaded, loading from network config")
		loadTieredConfigFromNetworkConfig(config)
		return
	}

	if consts.GetTieredAIConfig() != nil {
		configLoaded = true
		return
	}

	configPath := ResolveConfigFilePath("")
	if utils.GetFirstExistedFile(configPath) != "" {
		cfg, err := LoadTieredAIConfigFile(configPath)
		if err != nil {
			log.Debugf("failed to load tiered AI config file %s: %v", configPath, err)
		} else if cfg.Enabled {
			tiered := ConfigFileToTieredAIConfig(cfg)
			consts.SetTieredAIConfig(tiered)
			configLoaded = true
			log.Infof("tiered AI config loaded from file: %s", configPath)
			return
		}
	}

	defaultCfg := GetDefaultTieredAIConfigFile()
	tiered := ConfigFileToTieredAIConfig(defaultCfg)
	consts.SetTieredAIConfig(tiered)
	configLoaded = true
	log.Infof("tiered AI config loaded from built-in defaults (no DB config or config file found)")
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
