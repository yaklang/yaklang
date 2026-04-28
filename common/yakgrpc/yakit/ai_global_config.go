package yakit

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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

var (
	cachedAIGlobalConfig     *ypb.AIGlobalConfig
	cachedAIGlobalConfigLock sync.RWMutex
)

func setCachedAIGlobalConfig(cfg *ypb.AIGlobalConfig) {
	cachedAIGlobalConfigLock.Lock()
	defer cachedAIGlobalConfigLock.Unlock()
	cachedAIGlobalConfig = cloneAIGlobalConfig(cfg)
}

func GetCachedAIGlobalConfig() *ypb.AIGlobalConfig {
	cachedAIGlobalConfigLock.RLock()
	defer cachedAIGlobalConfigLock.RUnlock()
	return cloneAIGlobalConfig(cachedAIGlobalConfig)
}

// SetCachedAIGlobalConfigForTest overrides the in-memory AI global config cache.
// For testing only.
func SetCachedAIGlobalConfigForTest(cfg *ypb.AIGlobalConfig) {
	setCachedAIGlobalConfig(cfg)
}

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
	recoverProvidersFromDeprecatedConfig(db, cfg)
	persistMigratedAIGlobalConfig(db, cfg)
	return cfg, nil
}

func SetAIGlobalConfig(db *gorm.DB, cfg *ypb.AIGlobalConfig) (*ypb.AIGlobalConfig, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}

	if err := validateModelConfigs(cfg.IntelligentModels); err != nil {
		return nil, err
	}
	if err := validateModelConfigs(cfg.LightweightModels); err != nil {
		return nil, err
	}
	if err := validateModelConfigs(cfg.VisionModels); err != nil {
		return nil, err
	}
	migrateAIGlobalConfigBaseURLs(cfg)

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
		setCachedAIGlobalConfig(nil)
		consts.SetTieredAIConfig(nil)
		return nil
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
			providerCfg := cloneThirdPartyConfig(model.GetProvider())
			if providerCfg == nil {
				continue
			}
			result = append(result, &ypb.AIModelConfig{
				ProviderId:  model.GetProviderId(),
				Provider:    providerCfg,
				ModelName:   model.GetModelName(),
				ExtraParams: cloneKVPairs(model.GetExtraParams()),
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

	setCachedAIGlobalConfig(cfg)
	consts.SetTieredAIConfig(tiered)
	return nil
}

func cloneAIGlobalConfig(cfg *ypb.AIGlobalConfig) *ypb.AIGlobalConfig {
	if cfg == nil {
		return nil
	}
	return &ypb.AIGlobalConfig{
		Enabled:           cfg.GetEnabled(),
		RoutingPolicy:     cfg.GetRoutingPolicy(),
		DisableFallback:   cfg.GetDisableFallback(),
		DefaultModelId:    cfg.GetDefaultModelId(),
		GlobalWeight:      cfg.GetGlobalWeight(),
		IntelligentModels: cloneAIModelConfigs(cfg.GetIntelligentModels()),
		LightweightModels: cloneAIModelConfigs(cfg.GetLightweightModels()),
		VisionModels:      cloneAIModelConfigs(cfg.GetVisionModels()),
		AIPresetPrompt:    cfg.GetAIPresetPrompt(),
	}
}

func cloneAIModelConfigs(models []*ypb.AIModelConfig) []*ypb.AIModelConfig {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]*ypb.AIModelConfig, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		cloned = append(cloned, &ypb.AIModelConfig{
			ProviderId:  model.GetProviderId(),
			Provider:    cloneThirdPartyConfig(model.GetProvider()),
			ModelName:   model.GetModelName(),
			ExtraParams: cloneKVPairs(model.GetExtraParams()),
		})
	}
	return cloned
}

func validateModelConfigs(models []*ypb.AIModelConfig) error {
	if len(models) == 0 {
		return nil
	}
	for _, model := range models {
		if model == nil {
			continue
		}
		if model.GetProvider() == nil {
			return utils.Error("model config missing provider")
		}
	}
	return nil
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
		BaseURL:        provider.GetBaseURL(),
		Endpoint:       provider.GetEndpoint(),
		EnableEndpoint: provider.GetEnableEndpoint(),
		EnableThinking: provider.GetEnableThinking(),
		WebhookURL:     provider.GetWebhookURL(),
		Disabled:       provider.GetDisabled(),
		Proxy:          provider.GetProxy(),
		NoHttps:        provider.GetNoHttps(),
		APIType:        provider.GetAPIType(),
		Headers:        cloneHTTPHeaders(provider.GetHeaders()),
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
		EnableThinking: cfg.GetEnableThinking(),
		WebhookURL:     cfg.GetWebhookURL(),
		Disabled:       cfg.GetDisabled(),
		Proxy:          cfg.GetProxy(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
		Headers:        cloneHTTPHeaders(cfg.GetHeaders()),
		ExtraParams:    cloneKVPairs(cfg.GetExtraParams()),
	}
}

func modelNeedsLegacyRecovery(model *ypb.AIModelConfig) bool {
	if model == nil {
		return false
	}
	provider := model.GetProvider()
	if provider == nil {
		return true
	}
	if strings.TrimSpace(provider.GetType()) == "" {
		return true
	}
	if strings.TrimSpace(provider.GetAPIKey()) == "" {
		return true
	}
	return false
}

func recoverProvidersFromDeprecatedConfig(db *gorm.DB, cfg *ypb.AIGlobalConfig) {
	if db == nil || cfg == nil {
		return
	}
	needsRecovery := false
	updated := false
	collectIDs := func(models []*ypb.AIModelConfig, ids map[int64]struct{}) {
		for _, model := range models {
			if model == nil || !modelNeedsLegacyRecovery(model) {
				continue
			}
			if model.GetProviderId() == 0 {
				continue
			}
			needsRecovery = true
			ids[model.GetProviderId()] = struct{}{}
		}
	}

	ids := make(map[int64]struct{})
	collectIDs(cfg.GetIntelligentModels(), ids)
	collectIDs(cfg.GetLightweightModels(), ids)
	collectIDs(cfg.GetVisionModels(), ids)
	if !needsRecovery || len(ids) == 0 {
		return
	}

	idList := make([]int64, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}

	var legacyProviders []*schema.AIThirdPartyConfig
	if err := db.Model(&schema.AIThirdPartyConfig{}).Where("id in (?)", idList).Find(&legacyProviders).Error; err != nil {
		log.Debugf("recover deprecated ai providers failed: %v", err)
		return
	}

	legacyMap := make(map[int64]*schema.AIThirdPartyConfig, len(legacyProviders))
	for _, provider := range legacyProviders {
		if provider == nil {
			continue
		}
		legacyMap[int64(provider.ID)] = provider
	}

	fillProvider := func(models []*ypb.AIModelConfig) {
		for _, model := range models {
			if model == nil || model.GetProviderId() == 0 || !modelNeedsLegacyRecovery(model) {
				continue
			}
			if legacy, ok := legacyMap[model.GetProviderId()]; ok {
				model.Provider = legacy.ToThirdPartyConfig()
				updated = true
			}
		}
	}

	fillProvider(cfg.IntelligentModels)
	fillProvider(cfg.LightweightModels)
	fillProvider(cfg.VisionModels)

	if !updated {
		return
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		log.Debugf("persist recovered ai global config failed: %v", err)
		return
	}
	if err := SetKey(db, consts.AI_GLOBAL_CONFIG_KEY, string(data)); err != nil {
		log.Debugf("persist recovered ai global config failed: %v", err)
	}
}

func persistMigratedAIGlobalConfig(db *gorm.DB, cfg *ypb.AIGlobalConfig) {
	if db == nil || cfg == nil {
		return
	}
	if !migrateAIGlobalConfigBaseURLs(cfg) {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Debugf("persist migrated ai global config failed: %v", err)
		return
	}
	if err := SetKey(db, consts.AI_GLOBAL_CONFIG_KEY, string(data)); err != nil {
		log.Debugf("persist migrated ai global config failed: %v", err)
	}
}

func migrateAIGlobalConfigBaseURLs(cfg *ypb.AIGlobalConfig) bool {
	if cfg == nil {
		return false
	}
	changed := false
	migrateModels := func(models []*ypb.AIModelConfig) {
		for _, model := range models {
			if model == nil {
				continue
			}
			if migrateThirdPartyConfigBaseURL(model.Provider) {
				changed = true
			}
		}
	}
	migrateModels(cfg.IntelligentModels)
	migrateModels(cfg.LightweightModels)
	migrateModels(cfg.VisionModels)
	return changed
}

func migrateThirdPartyConfigBaseURL(cfg *ypb.ThirdPartyApplicationConfig) bool {
	if cfg == nil || strings.TrimSpace(cfg.GetBaseURL()) != "" {
		return false
	}
	rootURL, defaultURI := aiProviderDefaultEndpoint(cfg.GetType())
	baseURL := aispec.GetBaseURLRootFromConfig(&aispec.AIConfig{
		Type:           cfg.GetType(),
		BaseURL:        cfg.GetBaseURL(),
		Endpoint:       cfg.GetEndpoint(),
		EnableEndpoint: cfg.GetEnableEndpoint(),
		Domain:         cfg.GetDomain(),
		NoHttps:        cfg.GetNoHttps(),
		APIType:        cfg.GetAPIType(),
	}, rootURL, defaultURI)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return false
	}
	cfg.BaseURL = baseURL
	return true
}

func aiProviderDefaultEndpoint(providerType string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "deepseek":
		return "https://api.deepseek.com", "/chat/completions"
	case "volcengine":
		return "https://ark.cn-beijing.volces.com", "/api/v3/chat/completions"
	case "tongyi":
		return "https://dashscope.aliyuncs.com", "/compatible-mode/v1/chat/completions"
	case "openrouter":
		return "https://openrouter.ai", "/api/v1/chat/completions"
	case "chatglm":
		return "https://open.bigmodel.cn", "/api/paas/v4/chat/completions"
	case "ollama":
		return "http://127.0.0.1:11434", "/v1/chat/completions"
	case "aibalance":
		return "https://aibalance.yaklang.com", "/v1/chat/completions"
	case "moonshot":
		return "https://api.moonshot.cn", "/v1/chat/completions"
	case "siliconflow":
		return "https://api.siliconflow.cn", "/v1/chat/completions"
	case "openai", "":
		return "https://api.openai.com", "/v1/chat/completions"
	default:
		return "https://api.openai.com", "/v1/chat/completions"
	}
}
