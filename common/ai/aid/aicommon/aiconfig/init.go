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

	getNetworkConfig  = yakit.GetNetworkConfig
	getAIGlobalConfig = func() (*ypb.AIGlobalConfig, error) {
		return yakit.GetAIGlobalConfig(consts.GetGormProfileDatabase())
	}
)

func init() {
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
	cfg, err := getAIGlobalConfig() // from db ai global config
	source := "unknown"
	if err != nil || cfg == nil {
		config := getNetworkConfig()
		if config != nil { // from network config
			cfg = buildAIGlobalConfigFromNetworkConfig(config)
			source = "network-config"
		} else if tiered := consts.GetTieredAIConfig(); tiered != nil { // from consts
			cfg = buildAIGlobalConfigFromTiered(tiered)
			source = "memory-config"
		}
	}

	if cfg == nil { // use default config if no config found from DB, network, or memory
		cfg = buildDefaultAIGlobalConfig()
		source = "built-in defaults"
	}

	ensureTierModelConfigsAvailable(cfg)                        // ensure config base avail model
	if _, err := yakit.SetAIGlobalConfig(db, cfg); err != nil { // set it to database
		log.Warnf("failed to persist ai global config from %s: %v", source, err)
	}
	if err := yakit.ApplyAIGlobalConfig(db, cfg); err != nil { // set to consts
		log.Warnf("failed to apply ai global config from %s: %v", source, err)
	}

	configLoaded = true
	if source == "built-in defaults" {
		log.Infof("tiered AI config loaded from built-in defaults (no DB config found)")
	}
	warnIfLegacyConfigFileExists()
}

func buildDefaultAIGlobalConfig() *ypb.AIGlobalConfig {
	aibalanceProvider := yakit.DefaultAIBalanceProviderConfig()
	return &ypb.AIGlobalConfig{
		Enabled:         true,
		RoutingPolicy:   "balance",
		DisableFallback: false,
		IntelligentModels: []*ypb.AIModelConfig{
			{
				Provider:  cloneThirdPartyConfig(aibalanceProvider),
				ModelName: "memfit-standard-free",
				ExtraParams: []*ypb.KVPair{
					{Key: consts.ModelExtraParamKey, Value: "memfit-standard-free"},
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				Provider:  cloneThirdPartyConfig(aibalanceProvider),
				ModelName: "memfit-light-free",
				ExtraParams: []*ypb.KVPair{
					{Key: consts.ModelExtraParamKey, Value: "memfit-light-free"},
				},
			},
		},
		VisionModels: []*ypb.AIModelConfig{
			{
				Provider:  cloneThirdPartyConfig(aibalanceProvider),
				ModelName: "memfit-vision-free",
				ExtraParams: []*ypb.KVPair{
					{Key: consts.ModelExtraParamKey, Value: "memfit-vision-free"},
				},
			},
		},
	}
}

func ensureTierModelConfigsAvailable(cfg *ypb.AIGlobalConfig) bool {
	if cfg == nil {
		return false
	}

	needIntelligent := !hasAvailableModelConfig(cfg.GetIntelligentModels())
	needLightweight := !hasAvailableModelConfig(cfg.GetLightweightModels())
	needVision := !hasAvailableModelConfig(cfg.GetVisionModels())
	if !needIntelligent && !needLightweight && !needVision {
		return false
	}

	defaultCfg := buildDefaultAIGlobalConfig()
	if defaultCfg == nil {
		return false
	}

	if needIntelligent {
		cfg.IntelligentModels = cloneAIModelConfigs(defaultCfg.GetIntelligentModels())
	}
	if needLightweight {
		cfg.LightweightModels = cloneAIModelConfigs(defaultCfg.GetLightweightModels())
	}
	if needVision {
		cfg.VisionModels = cloneAIModelConfigs(defaultCfg.GetVisionModels())
	}
	return true
}

func hasAvailableModelConfig(models []*ypb.AIModelConfig) bool {
	for _, model := range models {
		if model == nil {
			continue
		}
		if model.GetProvider() != nil {
			return true
		}
	}
	return false
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
		cfg.RoutingPolicy = consts.DefaultRoutingPolicy
	}

	cfg.IntelligentModels = consts.BuildAIModelConfigs(c.GetIntelligentAIModelConfig())
	cfg.LightweightModels = consts.BuildAIModelConfigs(c.GetLightweightAIModelConfig())
	cfg.VisionModels = consts.BuildAIModelConfigs(c.GetVisionAIModelConfig())

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
		cfg.RoutingPolicy = consts.DefaultRoutingPolicy
	}

	cfg.IntelligentModels = cloneAIModelConfigs(tiered.IntelligentConfigs)
	cfg.LightweightModels = cloneAIModelConfigs(tiered.LightweightConfigs)
	cfg.VisionModels = cloneAIModelConfigs(tiered.VisionConfigs)

	return cfg
}

func cloneAIModelConfigs(configs []*ypb.AIModelConfig) []*ypb.AIModelConfig {
	if len(configs) == 0 {
		return nil
	}
	models := make([]*ypb.AIModelConfig, 0, len(configs))
	models = append(models, configs...)
	return models
}

func cloneThirdPartyConfig(cfg *ypb.ThirdPartyApplicationConfig) *ypb.ThirdPartyApplicationConfig {
	if cfg == nil {
		return nil
	}
	return &ypb.ThirdPartyApplicationConfig{
		Type:           cfg.GetType(),
		APIKey:         cfg.GetAPIKey(),
		UserIdentifier: cfg.GetUserIdentifier(),
		UserSecret:     cfg.GetUserSecret(),
		Namespace:      cfg.GetNamespace(),
		Domain:         cfg.GetDomain(),
		BaseURL:        cfg.GetBaseURL(),
		Endpoint:       cfg.GetEndpoint(),
		EnableEndpoint: cfg.GetEnableEndpoint(),
		WebhookURL:     cfg.GetWebhookURL(),
		Disabled:       cfg.GetDisabled(),
		Proxy:          cfg.GetProxy(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
		Headers:        cloneHTTPHeaders(cfg.GetHeaders()),
		ExtraParams:    cloneKVPairs(cfg.GetExtraParams()),
	}
}

func cloneKVPairs(kvs []*ypb.KVPair) []*ypb.KVPair {
	if len(kvs) == 0 {
		return nil
	}
	cloned := make([]*ypb.KVPair, 0, len(kvs))
	for _, kv := range kvs {
		if kv == nil {
			continue
		}
		cloned = append(cloned, &ypb.KVPair{Key: kv.GetKey(), Value: kv.GetValue()})
	}
	return cloned
}

func cloneHTTPHeaders(headers []*ypb.KVPair) []*ypb.KVPair {
	if len(headers) == 0 {
		return nil
	}
	cloned := make([]*ypb.KVPair, 0, len(headers))
	for _, header := range headers {
		if header == nil {
			continue
		}
		cloned = append(cloned, &ypb.KVPair{
			Key:   header.GetKey(),
			Value: header.GetValue(),
		})
	}
	return cloned
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
		IntelligentConfigs: consts.BuildAIModelConfigs(c.GetIntelligentAIModelConfig()),
		LightweightConfigs: consts.BuildAIModelConfigs(c.GetLightweightAIModelConfig()),
		VisionConfigs:      consts.BuildAIModelConfigs(c.GetVisionAIModelConfig()),
	}

	// Parse routing policy from TieredAIModelConfig
	if c.GetTieredAIModelConfig() != nil {
		policy := c.GetTieredAIModelConfig().GetModelRoutingPolicy()
		switch policy {
		case consts.RoutingPolicyAuto:
			tieredConfig.RoutingPolicy = consts.PolicyAuto
		case consts.RoutingPolicyPerformance:
			tieredConfig.RoutingPolicy = consts.PolicyPerformance
		case consts.RoutingPolicyCost:
			tieredConfig.RoutingPolicy = consts.PolicyCost
		case consts.RoutingPolicyBalance:
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
