package aiconfig

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	routingPolicyAuto        = string(consts.PolicyAuto)
	routingPolicyPerformance = string(consts.PolicyPerformance)
	routingPolicyCost        = string(consts.PolicyCost)
	routingPolicyBalance     = string(consts.PolicyBalance)
	defaultRoutingPolicy     = routingPolicyBalance
	modelExtraParamKey       = "model"
)

var (
	configLoadedOnce sync.Once
	configLoaded     bool

	getNetworkConfig  = yakit.GetNetworkConfig
	getAIGlobalConfig = func() (*ypb.AIGlobalConfig, error) {
		return yakit.GetAIGlobalConfig(consts.GetGormProfileDatabase())
	}
)

func init() {
	// ensure AIBalanceProviderConfig is loaded after database initialization
	yakit.RegisterPostInitDatabaseFunction(func() error {
		yakit.EnsureAIBalanceProviderConfig(consts.GetGormProfileDatabase())
		return nil
	})

	// Register a post-init database function to ensure TieredAIConfig is loaded
	// This will be called after the database is initialized
	yakit.RegisterPostInitDatabaseFunction(func() error {
		EnsureConfigLoaded()
		return nil
	}, "tiered-ai-config-loader")
}

// EnsureConfigLoaded ensures the tiered AI configuration is loaded.
// The ONLY authoritative source is the database (AIGlobalConfig).
// If the database has no config yet, built-in defaults are applied
// and written back to the database.
// Config files on disk are NOT loaded -- they are a legacy mechanism;
// use `yak tiered-ai-config` to write config into the database.
func EnsureConfigLoaded() {
	if configLoaded {
		return
	}

	db := consts.GetGormProfileDatabase()
	if cfg, err := getAIGlobalConfig(); err == nil && cfg != nil {
		_ = yakit.ApplyAIGlobalConfig(db, cfg)
		configLoaded = true
		warnIfLegacyConfigFileExists()
		return
	}

	var cfg *ypb.AIGlobalConfig
	source := "unknown"

	config := getNetworkConfig()
	if config != nil {
		cfg = buildAIGlobalConfigFromNetworkConfig(config)
		source = "network-config"
	} else if tiered := consts.GetTieredAIConfig(); tiered != nil {
		cfg = buildAIGlobalConfigFromTiered(tiered)
		source = "memory-config"
	}

	if cfg == nil { // use default config if no config found from DB, network, or memory
		cfg = buildDefaultAIGlobalConfig()
		source = "built-in defaults"
	}

	if cfg != nil {
		if _, err := yakit.SetAIGlobalConfig(db, cfg); err != nil {
			log.Warnf("failed to persist ai global config from %s: %v", source, err)
		}
		if err := yakit.ApplyAIGlobalConfig(db, cfg); err != nil {
			log.Warnf("failed to apply ai global config from %s: %v", source, err)
		}
	}

	configLoaded = true
	if source == "built-in defaults" {
		log.Infof("tiered AI config loaded from built-in defaults (no DB config found)")
	}
	warnIfLegacyConfigFileExists()
}

func buildDefaultAIGlobalConfig() *ypb.AIGlobalConfig {
	aibalanceId := yakit.EnsureAIBalanceProviderConfig(consts.GetGormProfileDatabase())
	return &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "balance",
		DisableFallback: false,
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ProviderId: aibalanceId,
				ModelName:  "memfit-standard-free",
				ExtraParams: []*ypb.KVPair{
					{Key: modelExtraParamKey, Value: "memfit-standard-free"},
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ProviderId: aibalanceId,
				ModelName:  "memfit-light-free",
				ExtraParams: []*ypb.KVPair{
					{Key: modelExtraParamKey, Value: "memfit-light-free"},
				},
			},
		},
		VisionModels: []*ypb.AIModelConfig{
			{
				ProviderId: aibalanceId,
				ModelName:  "memfit-vision-free",
				ExtraParams: []*ypb.KVPair{
					{Key: modelExtraParamKey, Value: "memfit-vision-free"},
				},
			},
		},
	}
}

func buildAIGlobalConfigFromNetworkConfig(c *ypb.GlobalNetworkConfig) *ypb.AIGlobalConfig {
	if c == nil {
		return nil
	}
	cfg := &ypb.AIGlobalConfig{
		Enabled: c.GetEnableTieredAIModelConfig(),
	}

	if c.GetTieredAIModelConfig() != nil {
		cfg.RoutingPolicy = c.GetTieredAIModelConfig().GetModelRoutingPolicy()
		cfg.DisableFallback = c.GetTieredAIModelConfig().GetDisableFallbackToLightweightModel()
	} else {
		return nil
	}

	if cfg.RoutingPolicy == "" {
		cfg.RoutingPolicy = defaultRoutingPolicy
	}

	cfg.IntelligentModels = buildAIModelConfigs(c.GetIntelligentAIModelConfig())
	cfg.LightweightModels = buildAIModelConfigs(c.GetLightweightAIModelConfig())
	cfg.VisionModels = buildAIModelConfigs(c.GetVisionAIModelConfig())

	return cfg
}

func buildAIGlobalConfigFromTiered(tiered *consts.TieredAIConfig) *ypb.AIGlobalConfig {
	if tiered == nil {
		return nil
	}
	cfg := &ypb.AIGlobalConfig{
		Enabled:         tiered.Enabled,
		RoutingPolicy:   string(tiered.RoutingPolicy),
		DisableFallback: tiered.DisableFallback,
		DefaultModelId:  tiered.DefaultModelID,
		GlobalWeight:    tiered.GlobalWeight,
	}
	if cfg.RoutingPolicy == "" {
		cfg.RoutingPolicy = defaultRoutingPolicy
	}

	cfg.IntelligentModels = cloneAIModelConfigs(tiered.IntelligentConfigs)
	cfg.LightweightModels = cloneAIModelConfigs(tiered.LightweightConfigs)
	cfg.VisionModels = cloneAIModelConfigs(tiered.VisionConfigs)

	return cfg
}

func buildAIModelConfigs(configs []*ypb.ThirdPartyApplicationConfig) []*ypb.AIModelConfig {
	if len(configs) == 0 {
		return nil
	}
	models := make([]*ypb.AIModelConfig, 0, len(configs))
	for _, cfg := range configs {
		model := thirdPartyConfigToModelConfig(cfg)
		if model == nil {
			continue
		}
		models = append(models, model)
	}
	return models
}

func cloneAIModelConfigs(configs []*ypb.AIModelConfig) []*ypb.AIModelConfig {
	if len(configs) == 0 {
		return nil
	}
	models := make([]*ypb.AIModelConfig, 0, len(configs))
	models = append(models, configs...)
	return models
}

func thirdPartyConfigToModelConfig(cfg *ypb.ThirdPartyApplicationConfig) *ypb.AIModelConfig {
	if cfg == nil {
		return nil
	}
	modelName := ""
	extras := make([]*ypb.KVPair, 0)
	for _, kv := range cfg.GetExtraParams() {
		if kv.GetKey() == modelExtraParamKey {
			modelName = kv.GetValue()
			continue
		}
		extras = append(extras, &ypb.KVPair{Key: kv.GetKey(), Value: kv.GetValue()})
	}

	provider := &ypb.ThirdPartyApplicationConfig{
		Type:           cfg.GetType(),
		APIKey:         cfg.GetAPIKey(),
		UserIdentifier: cfg.GetUserIdentifier(),
		UserSecret:     cfg.GetUserSecret(),
		Namespace:      cfg.GetNamespace(),
		Domain:         cfg.GetDomain(),
		WebhookURL:     cfg.GetWebhookURL(),
		Disabled:       cfg.GetDisabled(),
	}

	return &ypb.AIModelConfig{
		Provider:    provider,
		ModelName:   modelName,
		ExtraParams: extras,
	}
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
		IntelligentConfigs: buildAIModelConfigs(c.GetIntelligentAIModelConfig()),
		LightweightConfigs: buildAIModelConfigs(c.GetLightweightAIModelConfig()),
		VisionConfigs:      buildAIModelConfigs(c.GetVisionAIModelConfig()),
	}

	// Parse routing policy from TieredAIModelConfig
	if c.GetTieredAIModelConfig() != nil {
		policy := c.GetTieredAIModelConfig().GetModelRoutingPolicy()
		switch policy {
		case routingPolicyAuto:
			tieredConfig.RoutingPolicy = consts.PolicyAuto
		case routingPolicyPerformance:
			tieredConfig.RoutingPolicy = consts.PolicyPerformance
		case routingPolicyCost:
			tieredConfig.RoutingPolicy = consts.PolicyCost
		case routingPolicyBalance:
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

// SetAIGlobalConfigGetter overrides the function used to fetch
// AIGlobalConfig. For testing only.
func SetAIGlobalConfigGetter(fn func() (*ypb.AIGlobalConfig, error)) {
	getAIGlobalConfig = fn
}

// ResetAIGlobalConfigGetter restores the default getter. For testing only.
func ResetAIGlobalConfigGetter() {
	getAIGlobalConfig = func() (*ypb.AIGlobalConfig, error) {
		return yakit.GetAIGlobalConfig(consts.GetGormProfileDatabase())
	}
}
