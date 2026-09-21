package aiconfig

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIConfigManager manages tiered AI model configurations
type AIConfigManager struct {
	mu sync.RWMutex
}

var (
	globalManager     *AIConfigManager
	globalManagerOnce sync.Once

	saveAIGlobalConfigForManager = func(cfg *ypb.AIGlobalConfig) (*ypb.AIGlobalConfig, error) {
		return yakit.SetAIGlobalConfig(consts.GetGormProfileDatabase(), cfg)
	}
	applyAIGlobalConfigForManager = func(cfg *ypb.AIGlobalConfig) error {
		return yakit.ApplyAIGlobalConfig(consts.GetGormProfileDatabase(), cfg)
	}
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

// IsAIModelRoutingEnabled includes single-model routing even when the legacy
// tier-enabled switch is off. IsTieredAIConfig retains its historical meaning.
func IsAIModelRoutingEnabled(modes ...bool) bool {
	EnsureConfigLoaded()
	return IsAIModelRoutingConfigured(modes...)
}

// IsAIModelRoutingConfigured inspects the currently applied configuration.
// Callback setup must not load built-in defaults and unexpectedly replace an
// explicitly supplied fallback callback while checking whether to use tiers.
func IsAIModelRoutingConfigured(modes ...bool) bool {
	return (&AIConfigManager{}).singleModelMode(modes...) || consts.IsTieredAIModelConfigEnabled()
}

// BindModelConfig fixes the role at creation, but reads the latest model
// configuration in that role on each invocation. Global mode changes cannot
// redirect an already-bound callback.
func (m *AIConfigManager) BindModelConfig(tier consts.ModelTier, provider, name string, modes ...bool) func() (*ypb.AIModelConfig, error) {
	single := m.singleModelMode(modes...)
	if single {
		tier, provider, name = consts.TierIntelligent, "", ""
	}
	return func() (*ypb.AIModelConfig, error) {
		model := m.GetFirstConfigByTierAndProviderAndModel(tier, provider, name)
		if provider == "" && name == "" {
			model = m.GetFirstConfig(tier)
		}
		if single {
			if err := consts.ValidateSingleAIModel(model); err != nil {
				return nil, err
			}
		} else if model == nil || model.GetProvider() == nil || strings.TrimSpace(model.GetProvider().GetType()) == "" {
			return nil, ErrNoConfigAvailable
		}
		return proto.Clone(model).(*ypb.AIModelConfig), nil
	}
}

func (m *AIConfigManager) singleModelMode(modes ...bool) bool {
	if len(modes) > 0 {
		return modes[0]
	}
	return consts.IsSingleAIModelMode()
}

// BindRequestOptions fixes both the source tier and the LiteCall policy.
// Only provider/model settings are reread by the returned function.
func (m *AIConfigManager) BindRequestOptions(tier consts.ModelTier, provider, name string, modes ...bool) func(...aispec.AIConfigOption) ([]aispec.AIConfigOption, error) {
	single := m.singleModelMode(modes...)
	frozen := single
	getModel := m.BindModelConfig(tier, provider, name, frozen)
	lite := single && tier == consts.TierLightweight
	return func(opts ...aispec.AIConfigOption) ([]aispec.AIConfigOption, error) {
		model, err := getModel()
		if err != nil {
			return nil, err
		}
		resolved := append(aispec.BuildOptionsFromConfig(model), opts...)
		resolved = append(resolved, aispec.WithType(model.GetProvider().GetType()), aispec.WithModel(getModelFromConfig(model)), aispec.WithDisableProviderFallback(true))
		if lite {
			resolved = append(resolved, aispec.WithThinkingLevel("none"))
		}
		return resolved, nil
	}
}

func (m *AIConfigManager) NewChatCallback(tier consts.ModelTier, provider, name string, modes ...bool) aispec.GeneralChatter {
	resolve := m.BindRequestOptions(tier, provider, name, modes...)
	return func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		resolved, err := resolve(opts...)
		if err != nil {
			return "", err
		}
		return ai.Chat(prompt, resolved...)
	}
}

// GetCurrentPolicy returns the current user-configured routing policy
func GetCurrentPolicy() consts.RoutingPolicy {
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
func (m *AIConfigManager) GetConfigsByTier(tier consts.ModelTier) []*ypb.AIModelConfig {
	switch tier {
	case consts.TierIntelligent:
		return m.GetIntelligentConfigs()
	case consts.TierLightweight:
		return m.GetLightweightConfigs()
	case consts.TierVision:
		return m.GetVisionConfigs()
	default:
		log.Warnf("Unknown model tier: %s, falling back to intelligent", tier)
		return m.GetIntelligentConfigs()
	}
}

// GetFirstConfig returns the first configuration for a specific tier
// Returns nil if no configuration is available
func (m *AIConfigManager) GetFirstConfig(tier consts.ModelTier) *ypb.AIModelConfig {
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
func (m *AIConfigManager) GetFirstConfigByTierAndProviderAndModel(tier consts.ModelTier, providerName, modelName string) *ypb.AIModelConfig {
	configs := m.GetConfigsByTier(tier)
	if len(configs) == 0 {
		return nil
	}

	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)

	for _, cfg := range configs {
		if isConfigMatchedByProviderAndModel(cfg, providerName, modelName) {
			return cfg
		}
	}

	return nil
}

// PromoteFirstConfigByTierAndProviderAndModel moves the first matched model config
// to the top of the specified tier and persists the change to database.
func (m *AIConfigManager) PromoteFirstConfigByTierAndProviderAndModel(tier consts.ModelTier, providerName, modelName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	EnsureConfigLoaded()

	cfg, err := getAIGlobalConfig()
	if err != nil {
		return utils.Errorf("failed to load ai global config: %v", err)
	}
	if cfg == nil {
		return ErrNoConfigAvailable
	}

	models, err := getModelsByTierFromGlobalConfig(cfg, tier)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return ErrNoConfigAvailable
	}

	providerName = strings.TrimSpace(providerName)
	modelName = strings.TrimSpace(modelName)

	matchedIndex := -1
	for i, modelCfg := range models {
		if isConfigMatchedByProviderAndModel(modelCfg, providerName, modelName) {
			matchedIndex = i
			break
		}
	}
	if matchedIndex < 0 {
		return utils.Errorf("no matched model config found for tier=%s provider=%s model=%s", tier, providerName, modelName)
	}

	if matchedIndex > 0 {
		matched := models[matchedIndex]
		copy(models[1:matchedIndex+1], models[:matchedIndex])
		models[0] = matched
		setModelsByTierInGlobalConfig(cfg, tier, models)
	}

	normalized, err := saveAIGlobalConfigForManager(cfg)
	if err != nil {
		return utils.Errorf("failed to save ai global config: %v", err)
	}
	if err := applyAIGlobalConfigForManager(normalized); err != nil {
		return utils.Errorf("failed to apply ai global config: %v", err)
	}

	return nil
}

func isConfigMatchedByProviderAndModel(cfg *ypb.AIModelConfig, providerName, modelName string) bool {
	if cfg == nil {
		return false
	}

	if providerName != "" && !strings.EqualFold(strings.TrimSpace(cfg.GetProvider().GetType()), providerName) {
		return false
	}

	if modelName != "" && !strings.EqualFold(strings.TrimSpace(getModelFromConfig(cfg)), modelName) {
		return false
	}

	return true
}

func getModelsByTierFromGlobalConfig(cfg *ypb.AIGlobalConfig, tier consts.ModelTier) ([]*ypb.AIModelConfig, error) {
	switch tier {
	case consts.TierIntelligent:
		return cfg.GetIntelligentModels(), nil
	case consts.TierLightweight:
		return cfg.GetLightweightModels(), nil
	case consts.TierVision:
		return cfg.GetVisionModels(), nil
	default:
		return nil, utils.Errorf("invalid model tier: %s", tier)
	}
}

func setModelsByTierInGlobalConfig(cfg *ypb.AIGlobalConfig, tier consts.ModelTier, models []*ypb.AIModelConfig) {
	switch tier {
	case consts.TierIntelligent:
		cfg.IntelligentModels = models
	case consts.TierLightweight:
		cfg.LightweightModels = models
	case consts.TierVision:
		cfg.VisionModels = models
	}
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
		if strings.EqualFold(strings.TrimSpace(param.GetKey()), consts.ModelExtraParamKey) {
			return param.GetValue()
		}
	}
	for _, param := range config.GetProvider().GetExtraParams() {
		if param == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(param.GetKey()), consts.ModelExtraParamKey) {
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
func GetModelByPolicy(policy consts.RoutingPolicy, modes ...bool) (*ypb.AIModelConfig, error) {
	tier := consts.TierLightweight
	if policy == consts.PolicyPerformance {
		tier = consts.TierIntelligent
	}
	return GetGlobalManager().BindModelConfig(tier, "", "", modes...)()
}

// IsFallbackDisabled checks if fallback to lightweight model is disabled
func IsFallbackDisabled() bool {
	return consts.IsSingleAIModelMode() || consts.IsTieredAIFallbackDisabled()
}
