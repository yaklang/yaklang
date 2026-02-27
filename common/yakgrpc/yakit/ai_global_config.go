package yakit

import (
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

func HasAIGlobalConfig(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	return GetKey(db, consts.AI_GLOBAL_CONFIG_KEY) != ""
}

func GetAIGlobalConfig(db *gorm.DB) (*ypb.AIGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if !HasAIGlobalConfig(db) {
		return nil, nil
	}
	raw := GetKey(db, consts.AI_GLOBAL_CONFIG_KEY)
	if raw == "" {
		return nil, nil
	}
	cfg := &ypb.AIGlobalConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	if cfg.RoutingPolicy == "" {
		cfg.RoutingPolicy = defaultRoutingPolicy
	}
	providerMap, _ := LoadAIProviderMap(db)
	fillProviders(cfg.IntelligentModels, providerMap)
	fillProviders(cfg.LightweightModels, providerMap)
	fillProviders(cfg.VisionModels, providerMap)
	return cfg, nil
}

func SetAIGlobalConfig(db *gorm.DB, cfg *ypb.AIGlobalConfig) (*ypb.AIGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}

	if err := normalizeModelConfigs(db, cfg.IntelligentModels); err != nil {
		return nil, err
	}
	if err := normalizeModelConfigs(db, cfg.LightweightModels); err != nil {
		return nil, err
	}
	if err := normalizeModelConfigs(db, cfg.VisionModels); err != nil {
		return nil, err
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := SetKey(db, consts.AI_GLOBAL_CONFIG_KEY, string(data)); err != nil {
		return nil, err
	}
	return cfg, nil
}

func ApplyAIGlobalConfig(db *gorm.DB, cfg *ypb.AIGlobalConfig) error {
	if cfg == nil {
		consts.SetTieredAIConfig(nil)
		return nil
	}
	providerMap, _ := LoadAIProviderMap(db)

	cloneExtraParams := func(extra []*ypb.KVPair) []*ypb.KVPair {
		if len(extra) == 0 {
			return nil
		}
		cloned := make([]*ypb.KVPair, 0, len(extra))
		for _, kv := range extra {
			if kv == nil {
				continue
			}
			cloned = append(cloned, &ypb.KVPair{Key: kv.GetKey(), Value: kv.GetValue()})
		}
		return cloned
	}

	buildModels := func(models []*ypb.AIModelConfig) []*ypb.AIModelConfig {
		if len(models) == 0 {
			return nil
		}
		result := make([]*ypb.AIModelConfig, 0, len(models))
		for _, model := range models {
			if model == nil {
				continue
			}
			providerCfg := resolveProviderForModel(model, providerMap)
			if providerCfg == nil {
				continue
			}
			result = append(result, &ypb.AIModelConfig{
				ProviderId:  model.GetProviderId(),
				Provider:    providerCfg,
				ModelName:   model.GetModelName(),
				ExtraParams: cloneExtraParams(model.GetExtraParams()),
			})
		}
		return result
	}

	routing := consts.PolicyBalance
	switch cfg.GetRoutingPolicy() {
	case routingPolicyAuto:
		routing = consts.PolicyAuto
	case routingPolicyPerformance:
		routing = consts.PolicyPerformance
	case routingPolicyCost:
		routing = consts.PolicyCost
	case routingPolicyBalance:
		routing = consts.PolicyBalance
	}

	tiered := &consts.TieredAIConfig{
		Enabled:         cfg.GetEnabled(),
		DisableFallback: cfg.GetDisableFallback(),
		RoutingPolicy:   routing,
		DefaultModelID:  cfg.GetDefaultModelId(),
		GlobalWeight:    cfg.GetGlobalWeight(),
	}

	tiered.IntelligentConfigs = buildModels(cfg.IntelligentModels)
	tiered.LightweightConfigs = buildModels(cfg.LightweightModels)
	tiered.VisionConfigs = buildModels(cfg.VisionModels)

	consts.SetTieredAIConfig(tiered)
	return nil
}

func normalizeModelConfigs(db *gorm.DB, models []*ypb.AIModelConfig) error {
	if len(models) == 0 {
		return nil
	}
	for _, model := range models {
		if model == nil {
			continue
		}
		providerId := model.GetProviderId()
		if model.GetProvider() != nil {
			provider := schema.AIThirdPartyConfigFromGRPC(model.GetProvider())
			if providerId > 0 {
				provider.ID = uint(providerId)
			}
			saved, err := UpsertAIProvider(db, provider)
			if err != nil {
				return err
			}
			model.ProviderId = int64(saved.ID)
			model.Provider = nil
			providerId = model.ProviderId
		}

		if providerId == 0 {
			return utils.Error("model config missing provider")
		}
		if _, err := GetAIProvider(db, providerId); err != nil {
			return err
		}
	}
	return nil
}

func resolveProviderForModel(model *ypb.AIModelConfig, providerMap map[int64]*schema.AIThirdPartyConfig) *ypb.ThirdPartyApplicationConfig {
	if model == nil {
		return nil
	}
	if model.GetProviderId() != 0 {
		if provider, ok := providerMap[model.GetProviderId()]; ok {
			return provider.ToThirdPartyConfig()
		}
	}
	return model.GetProvider()
}

func mergeProviderAndModel(provider *ypb.ThirdPartyApplicationConfig, model *ypb.AIModelConfig) *ypb.ThirdPartyApplicationConfig {
	if provider == nil {
		return nil
	}
	merged := &ypb.ThirdPartyApplicationConfig{
		Type:           provider.GetType(),
		APIKey:         provider.GetAPIKey(),
		UserIdentifier: provider.GetUserIdentifier(),
		UserSecret:     provider.GetUserSecret(),
		Namespace:      provider.GetNamespace(),
		Domain:         provider.GetDomain(),
		WebhookURL:     provider.GetWebhookURL(),
		Disabled:       provider.GetDisabled(),
	}

	extra := mapFromKVPairs(provider.GetExtraParams())
	if model != nil {
		if model.GetModelName() != "" {
			extra[modelExtraParamKey] = model.GetModelName()
		}
		for _, kv := range model.GetExtraParams() {
			extra[kv.GetKey()] = kv.GetValue()
		}
	}
	if len(extra) > 0 {
		merged.ExtraParams = kvPairsFromMap(extra)
	}
	return merged
}

func fillProviders(models []*ypb.AIModelConfig, providerMap map[int64]*schema.AIThirdPartyConfig) {
	for _, model := range models {
		if model == nil || model.Provider != nil || model.ProviderId == 0 {
			continue
		}
		if provider, ok := providerMap[model.ProviderId]; ok {
			model.Provider = provider.ToThirdPartyConfig()
		}
	}
}

func mapFromKVPairs(kvs []*ypb.KVPair) map[string]string {
	if len(kvs) == 0 {
		return map[string]string{}
	}
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[kv.GetKey()] = kv.GetValue()
	}
	return m
}

func kvPairsFromMap(m map[string]string) []*ypb.KVPair {
	if len(m) == 0 {
		return nil
	}
	pairs := make([]*ypb.KVPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, &ypb.KVPair{Key: k, Value: v})
	}
	return pairs
}
