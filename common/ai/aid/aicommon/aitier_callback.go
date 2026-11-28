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

// AICallbackFactory generates the callback for one routing decision.
//
// Config stores factories instead of live callbacks so a consumer can pick the
// generation point. The main ReAct round generates once from the same model
// snapshot that sized its prompt, holds the result, and sends every request of
// that round through it. A hot-patch or tier switch after generation therefore
// cannot desynchronize prompt budget and actual routing.
//
// model is the caller's pinned snapshot. A nil model lets the generated
// callback consult the latest tier configuration at invocation time; callers
// that size prompts must pass their snapshot so both sides describe one model.
type AICallbackFactory func(model *ypb.AIModelConfig) (AICallbackType, error)

// FixedAICallbackFactory adapts a callback whose routing does not depend on a
// tier configuration. A nil callback produces a nil factory so callers keep
// the existing "not configured" semantics.
func FixedAICallbackFactory(cb AICallbackType) AICallbackFactory {
	if cb == nil {
		return nil
	}
	return func(*ypb.AIModelConfig) (AICallbackType, error) {
		return cb, nil
	}
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

// newTierAICallbackFactory performs the complete tier callback setup: check
// routing availability, remember the explicit single-model flag, then return a
// generator. The concrete model is resolved when a callback is generated, so
// each ReAct round can pin the quality route without freezing auxiliary tiers.
func newTierAICallbackFactory(tier consts.ModelTier, modes ...bool) (AICallbackFactory, error) {
	single := resolveTierCallbackMode(modes...)
	if !single && !aiconfig.IsTieredAIConfig() {
		return nil, aiconfig.ErrTieredConfigDisabled
	}
	return func(model *ypb.AIModelConfig) (AICallbackType, error) {
		// A pinned snapshot is authoritative for the quality tier. Build the
		// callback now so its budget identity and provider options are fixed to
		// the same configuration even if the global tier changes later.
		if tier == consts.TierIntelligent && model != nil {
			return CreateCallbackFromConfigWithExtraOpts(model)
		}
		return func(caller AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			config := request.GetImageModel()
			if config == nil {
				var err error
				config, err = resolveTierModelConfig(tier, single)
				if err != nil {
					return nil, err
				}
			}
			callback, err := CreateCallbackFromConfigWithExtraOpts(config, extractUserUsageCallbackOpts(caller)...)
			if err != nil {
				return nil, err
			}
			return callback(caller, request)
		}, nil
	}, nil
}

func generateTierAICallback(tier consts.ModelTier, modes ...bool) (AICallbackType, error) {
	factory, err := newTierAICallbackFactory(tier, modes...)
	if err != nil {
		return nil, err
	}
	return factory(nil)
}

// GetIntelligentAIModelCallback returns the AI callback for intelligent (high-quality) models.
func GetIntelligentAIModelCallback(modes ...bool) (AICallbackType, error) {
	return generateTierAICallback(consts.TierIntelligent, modes...)
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
	return generateTierAICallback(consts.TierLightweight, modes...)
}

// GetVisionAIModelCallback returns the AI callback for vision models.
func GetVisionAIModelCallback(modes ...bool) (AICallbackType, error) {
	return generateTierAICallback(consts.TierVision, modes...)
}

// GetDefaultAIModelCallback returns the callback selected by the current policy.
func GetDefaultAIModelCallback(modes ...bool) (AICallbackType, error) {
	tier := consts.TierLightweight
	if aiconfig.GetCurrentPolicy() == consts.PolicyPerformance {
		tier = consts.TierIntelligent
	}
	return generateTierAICallback(tier, modes...)
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
