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

type ModelTier string

const (
	// TierIntelligent represents high-intelligence models for complex tasks
	TierIntelligent ModelTier = "intelligent"
	// TierLightweight represents lightweight models for simple and fast tasks
	TierLightweight ModelTier = "lightweight"
	// TierVision represents vision models for image understanding tasks
	TierVision ModelTier = "vision"
)

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
	// DefaultModelID is the default model identifier for AI calls
	DefaultModelID string
	// GlobalWeight is a global weight used by AI routing strategies
	GlobalWeight float64
	// IntelligentConfigs contains configurations for high-intelligence models
	IntelligentConfigs []*ypb.AIModelConfig
	// LightweightConfigs contains configurations for lightweight models
	LightweightConfigs []*ypb.AIModelConfig
	// VisionConfigs contains configurations for vision models
	VisionConfigs []*ypb.AIModelConfig
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
func GetIntelligentAIConfigs() []*ypb.AIModelConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return nil
	}
	return tieredAIConfig.IntelligentConfigs
}

// GetLightweightAIConfigs returns the lightweight model configurations
func GetLightweightAIConfigs() []*ypb.AIModelConfig {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	if tieredAIConfig == nil {
		return nil
	}
	return tieredAIConfig.LightweightConfigs
}

// GetVisionAIConfigs returns the vision model configurations
func GetVisionAIConfigs() []*ypb.AIModelConfig {
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

func BuildAIModelConfigs(configs []*ypb.ThirdPartyApplicationConfig) []*ypb.AIModelConfig {
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

func thirdPartyConfigToModelConfig(cfg *ypb.ThirdPartyApplicationConfig) *ypb.AIModelConfig {
	if cfg == nil {
		return nil
	}
	modelName := ""
	extras := make([]*ypb.KVPair, 0)
	for _, kv := range cfg.GetExtraParams() {
		if kv.GetKey() == ModelExtraParamKey {
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
		BaseURL:        cfg.GetBaseURL(),
		Endpoint:       cfg.GetEndpoint(),
		EnableEndpoint: cfg.GetEnableEndpoint(),
		EnableThinking: cfg.GetEnableThinking(),
		WebhookURL:     cfg.GetWebhookURL(),
		Disabled:       cfg.GetDisabled(),
		Proxy:          cfg.GetProxy(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
		Headers:        cloneHTTPHeadersForAIConfig(cfg.GetHeaders()),
	}

	return &ypb.AIModelConfig{
		Provider:    provider,
		ModelName:   modelName,
		ExtraParams: extras,
	}
}

func cloneHTTPHeadersForAIConfig(headers []*ypb.KVPair) []*ypb.KVPair {
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

const (
	RoutingPolicyAuto        = string(PolicyAuto)
	RoutingPolicyPerformance = string(PolicyPerformance)
	RoutingPolicyCost        = string(PolicyCost)
	RoutingPolicyBalance     = string(PolicyBalance)
	DefaultRoutingPolicy     = RoutingPolicyBalance
	ModelExtraParamKey       = "model"
)
