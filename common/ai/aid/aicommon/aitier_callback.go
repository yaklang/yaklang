package aicommon

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// extractUserUsageCallbackOpts 从 wrapper 后的 i 取出 user 端注册的 UsageCallback,
// 包成 aispec.WithUsageCallback 返回. wrapper 把 *Config 包成
// *tierAwareConsumptionCaller, 也兼容直接传入 *Config 的场景 (CallAI 等).
//
// 取 callback 的优先级:
//  1. cfg.userUsageCallback (P1-D / P1-D2 已修复的同 Config / inherit 链路);
//  2. cfg.GetContext() 中 ctx-based 透传 (P3-T5 新增, 修复 aicommon.InvokeLiteForge
//     -> MustGetSpeedPriorityAIModelCallback 创建的子 Config 没继承 user callback 的 BUG).
//
// 关键词: extractUserUsageCallbackOpts, Tiered AI usageCallback 透传, ctx fallback
func extractUserUsageCallbackOpts(i AICallerConfigIf) []aispec.AIConfigOption {
	if i == nil {
		return nil
	}
	var cfg *Config
	if t, ok := i.(*tierAwareConsumptionCaller); ok && t != nil {
		cfg = t.Config
	} else if c, ok := i.(*Config); ok {
		cfg = c
	}
	var cb func(*aispec.ChatUsage)
	if cfg != nil {
		cb = cfg.GetUserUsageCallback()
	}
	if cb == nil {
		// 走 ctx-based fallback, 兼容 enhancesearch 等子调用场景:
		// 子 Config 自身没显式继承 userUsageCallback, 但父 React loop 的 ctx 里携带了
		// user callback (Config.GetContext 自动注入), aicommon.InvokeLiteForge 通过
		// aicommon.WithContext(ctx) 把 ctx 复制到子 Config 上, 此处再取出.
		ctx := i.GetContext()
		if ctx != nil {
			cb = GetUserUsageCallbackFromContext(ctx)
		}
	}
	if cb == nil {
		return nil
	}
	return []aispec.AIConfigOption{aispec.WithUsageCallback(cb)}
}

func MustGetIntelligentAIModelCallback() AICallbackType {
	callback, err := GetIntelligentAIModelCallback()
	if err != nil {
		log.Warnf("you are using aiconfig to get intelligent model callback, but got error: %v", err)
		return AIChatToAICallbackType(ai.Chat)
	}
	return callback
}

func MustGetLightweightAIModelCallback() AICallbackType {
	callback, err := GetLightweightAIModelCallback()
	if err != nil {
		log.Warnf("you are using aiconfig to get lightweight model callback, but got error: %v", err)
		return AIChatToAICallbackType(ai.Chat)
	}
	return callback
}

func MustGetQualityPriorityAIModelCallback() AICallbackType {
	return MustGetIntelligentAIModelCallback()
}

func MustGetSpeedPriorityAIModelCallback() AICallbackType {
	return MustGetLightweightAIModelCallback()
}

func MustGetVisionAIModelCallback() AICallbackType {
	callback, err := GetVisionAIModelCallback()
	if err != nil {
		log.Warnf("you are using aiconfig to get vision model callback, but got error: %v", err)
		return AIChatToAICallbackType(ai.Chat)
	}
	return callback
}

func MustGetDefaultAIModelCallback() AICallbackType {
	callback, err := GetDefaultAIModelCallback()
	if err != nil {
		log.Warnf("you are using aiconfig to get default model callback, but got error: %v", err)
		return AIChatToAICallbackType(ai.Chat)
	}
	return callback
}

func MustGetAIModelCallbackByTierAndProviderAndModel(tier consts.ModelTier, providerName, modelName string) AICallbackType {
	callback, err := GetAIModelCallbackByTierAndProviderAndModel(tier, providerName, modelName)
	if err != nil {
		log.Warnf("you are using aiconfig to get model callback by tier/provider/model, but got error: %v", err)
		return AIChatToAICallbackType(ai.Chat)
	}
	return callback
}

// resolveTierCallbackMode fixes the callback role at construction. The bool is
// supplied by Config initialization; without it, only the global mode applies.
func resolveTierCallbackMode(modes ...bool) bool {
	if len(modes) > 0 {
		return modes[0]
	}
	return consts.IsSingleAIModelMode()
}

func resolveTierModelConfig(tier consts.ModelTier, single bool) (*ypb.AIModelConfig, error) {
	liteCall := single && tier == consts.TierLightweight
	if single {
		tier = consts.TierIntelligent
	}
	config := aiconfig.GetGlobalManager().GetFirstConfig(tier)
	if config == nil || config.GetProvider() == nil || config.GetProvider().GetType() == "" || config.GetModelName() == "" {
		return nil, aiconfig.ErrNoConfigAvailable
	}
	config = proto.Clone(config).(*ypb.AIModelConfig)
	if liteCall {
		level := "none"
		config.Provider.ReasoningEffort = &level
	}
	return config, nil
}

// newTierAIModelCallback performs the complete tier callback setup:
// check routing availability, combine the requested tier with the explicit
// single-model flag, then build an invocation callback. The invocation reads
// the latest model config in the already selected tier.
func newTierAIModelCallback(tier consts.ModelTier, modes ...bool) (AICallbackType, error) {
	single := resolveTierCallbackMode(modes...)
	if !single && !aiconfig.IsTieredAIConfig() {
		return nil, aiconfig.ErrTieredConfigDisabled
	}
	return func(caller AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
		config, err := resolveTierModelConfig(tier, single)
		if err != nil {
			return nil, err
		}
		callback, err := CreateCallbackFromConfigWithExtraOpts(config, extractUserUsageCallbackOpts(caller)...)
		if err != nil {
			return nil, err
		}
		return callback(caller, request)
	}, nil
}

// GetIntelligentAIModelCallback returns the AI callback for intelligent (high-quality) models.
func GetIntelligentAIModelCallback(modes ...bool) (AICallbackType, error) {
	return newTierAIModelCallback(consts.TierIntelligent, modes...)
}

func GetIntelligentAIModelInfo(modes ...bool) (string, string, error) {
	single := resolveTierCallbackMode(modes...)
	if !single && !aiconfig.IsTieredAIConfig() {
		return "", "", aiconfig.ErrTieredConfigDisabled
	}
	config, err := resolveTierModelConfig(consts.TierIntelligent, single)
	if err != nil {
		return "", "", err
	}
	return config.GetProvider().GetType(), config.GetModelName(), nil
}

// GetLightweightAIModelCallback returns the AI callback for lightweight models.
func GetLightweightAIModelCallback(modes ...bool) (AICallbackType, error) {
	return newTierAIModelCallback(consts.TierLightweight, modes...)
}

// GetVisionAIModelCallback returns the AI callback for vision models.
func GetVisionAIModelCallback(modes ...bool) (AICallbackType, error) {
	return newTierAIModelCallback(consts.TierVision, modes...)
}

// GetDefaultAIModelCallback returns the callback selected by the current policy.
func GetDefaultAIModelCallback(modes ...bool) (AICallbackType, error) {
	tier := consts.TierLightweight
	if aiconfig.GetCurrentPolicy() == consts.PolicyPerformance {
		tier = consts.TierIntelligent
	}
	return newTierAIModelCallback(tier, modes...)
}

// GetAIModelCallbackByTierAndProviderAndModel preserves the legacy explicit
// selector API. It is separate from single-model tier initialization.
func GetAIModelCallbackByTierAndProviderAndModel(tier consts.ModelTier, providerName, modelName string) (AICallbackType, error) {
	if !aiconfig.IsTieredAIConfig() {
		return nil, aiconfig.ErrTieredConfigDisabled
	}
	return func(caller AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
		config := aiconfig.GetGlobalManager().GetFirstConfigByTierAndProviderAndModel(tier, providerName, modelName)
		if config == nil {
			return nil, aiconfig.ErrNoConfigAvailable
		}
		callback, err := CreateCallbackFromConfigWithExtraOpts(config, extractUserUsageCallbackOpts(caller)...)
		if err != nil {
			return nil, err
		}
		return callback(caller, request)
	}, nil
}

// GetCallbackByTier returns the AI callback for a specific model tier
func GetCallbackByTier(tier consts.ModelTier) (AICallbackType, error) {
	switch tier {
	case consts.TierIntelligent:
		return GetIntelligentAIModelCallback()
	case consts.TierLightweight:
		return GetLightweightAIModelCallback()
	case consts.TierVision:
		return GetVisionAIModelCallback()
	default:
		log.Warnf("Unknown model tier: %s, using intelligent model", tier)
		return GetIntelligentAIModelCallback()
	}
}

// TryGetCallbackWithFallback tries to get a callback for the specified tier
// If the tier is not available and fallback is enabled, it falls back to lightweight model
func TryGetCallbackWithFallback(tier consts.ModelTier) (AICallbackType, error) {
	callback, err := GetCallbackByTier(tier)
	if err == nil {
		return callback, nil
	}

	// Check if fallback is disabled
	if aiconfig.IsFallbackDisabled() {
		return nil, err
	}

	// Try fallback to lightweight model
	if tier != consts.TierLightweight {
		log.Debugf("Falling back from %s to lightweight model", tier)
		fallbackCallback, fallbackErr := GetLightweightAIModelCallback()
		if fallbackErr == nil {
			return fallbackCallback, nil
		}
	}

	return nil, err
}
