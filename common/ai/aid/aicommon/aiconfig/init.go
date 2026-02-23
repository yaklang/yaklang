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

	getNetworkConfig = yakit.GetNetworkConfig
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
		tieredConfig := consts.GetTieredAIConfig()
		if tieredConfig != nil {
			log.Debugf("tiered AI config loaded from DB: enabled=%v, policy=%s, intelligent=%d, lightweight=%d, vision=%d",
				tieredConfig.Enabled,
				tieredConfig.RoutingPolicy,
				len(tieredConfig.IntelligentConfigs),
				len(tieredConfig.LightweightConfigs),
				len(tieredConfig.VisionConfigs))
			configLoaded = true
		} else {
			log.Debugf("tiered AI config not available from DB yet")
		}
	})
	return nil
}

// EnsureConfigLoaded ensures the tiered AI configuration is loaded.
// The ONLY authoritative source is the database (GlobalNetworkConfig).
// If the database has no config yet, built-in defaults are applied.
// Config files on disk are NOT loaded -- they are a legacy mechanism;
// use `yak tiered-ai-config` to write config into the database.
func EnsureConfigLoaded() {
	if configLoaded {
		return
	}

	config := getNetworkConfig()
	if config != nil {
		loadTieredConfigFromNetworkConfig(config)
		warnIfLegacyConfigFileExists()
		return
	}

	if cfg := consts.GetTieredAIConfig(); cfg != nil {
		configLoaded = true
		warnIfLegacyConfigFileExists()
		return
	}

	defaultCfg := GetDefaultTieredAIConfigFile()
	tiered := ConfigFileToTieredAIConfig(defaultCfg)
	consts.SetTieredAIConfig(tiered)
	configLoaded = true
	log.Infof("tiered AI config loaded from built-in defaults (no DB config found)")
	warnIfLegacyConfigFileExists()
}

func warnIfLegacyConfigFileExists() {
	configPath := ResolveConfigFilePath("")
	if utils.GetFirstExistedFile(configPath) != "" {
		log.Warnf("legacy tiered AI config file found at %s, "+
			"this file is no longer used. The authoritative source is the database. "+
			"Use `yak tiered-ai-config` to update the database configuration.", configPath)
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

// SetNetworkConfigGetter overrides the function used to fetch
// GlobalNetworkConfig. For testing only.
func SetNetworkConfigGetter(fn func() *ypb.GlobalNetworkConfig) {
	getNetworkConfig = fn
}

// ResetNetworkConfigGetter restores the default getter. For testing only.
func ResetNetworkConfigGetter() {
	getNetworkConfig = yakit.GetNetworkConfig
}
