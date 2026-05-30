package aibalance

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 默认四维 Token 倍率（仅在 AiModelMeta 中对应字段为 0/缺省时生效）
// 关键词: ComputeWeightedTokens 默认倍率, dashscope cache_creation 1.25, cache_hit 0.1
const (
	defaultInputTokenMultiplier    = 1.0
	defaultOutputTokenMultiplier   = 1.0
	defaultCacheCreationMultiplier = 1.25
	defaultCacheHitMultiplier      = 0.1
)

// resolvedMultipliers 是 AiModelMeta -> 实际 4 维倍率的解析结果。
// 关键词: resolvedMultipliers, 倍率回落策略
type resolvedMultipliers struct {
	Input         float64
	Output        float64
	CacheCreate   float64
	CacheHit      float64
	LegacyTraffic float64
}

// resolveMultipliers 根据 AiModelMeta 解析出实际生效的四维 Token 倍率：
//   - 若四维新字段全部 ≤ 0，则整体回落到老 TrafficMultiplier（保持存量行为）。
//   - 若某一维 ≤ 0，则用该维的默认值（input=1.0/output=1.0/cache_create=1.25/cache_hit=0.1）。
//   - meta == nil 时全部按默认值。
//
// 关键词: resolveMultipliers, 四维倍率回落, 老 TrafficMultiplier 兜底
func resolveMultipliers(meta *AiModelMeta) resolvedMultipliers {
	r := resolvedMultipliers{
		Input:         defaultInputTokenMultiplier,
		Output:        defaultOutputTokenMultiplier,
		CacheCreate:   defaultCacheCreationMultiplier,
		CacheHit:      defaultCacheHitMultiplier,
		LegacyTraffic: 1.0,
	}
	if meta == nil {
		return r
	}
	if meta.TrafficMultiplier > 0 {
		r.LegacyTraffic = meta.TrafficMultiplier
	}

	// 全部新字段为 0/缺省 -> 整体回落到老 TrafficMultiplier
	allZero := meta.InputTokenMultiplier <= 0 &&
		meta.OutputTokenMultiplier <= 0 &&
		meta.CacheCreationMultiplier <= 0 &&
		meta.CacheHitMultiplier <= 0
	if allZero {
		r.Input = r.LegacyTraffic
		r.Output = r.LegacyTraffic
		r.CacheCreate = r.LegacyTraffic * defaultCacheCreationMultiplier
		r.CacheHit = r.LegacyTraffic * defaultCacheHitMultiplier
		return r
	}

	if meta.InputTokenMultiplier > 0 {
		r.Input = meta.InputTokenMultiplier
	}
	if meta.OutputTokenMultiplier > 0 {
		r.Output = meta.OutputTokenMultiplier
	}
	if meta.CacheCreationMultiplier > 0 {
		r.CacheCreate = meta.CacheCreationMultiplier
	}
	if meta.CacheHitMultiplier > 0 {
		r.CacheHit = meta.CacheHitMultiplier
	}
	return r
}

// applyMultiplierDim 仅在 v>0 时覆盖目标维度，用于「逐维分层回落」。
// 关键词: applyMultiplierDim, 逐维覆盖
func applyMultiplierDim(dst *float64, v float64) {
	if v > 0 {
		*dst = v
	}
}

// wrapperConfiguredDims 返回 wrapper (AiModelMeta) 级别「显式配置」的逐维倍率。
// 返回结构中某维 == 0 表示「该维 wrapper 级未配置」，调用方应继续向下回落。
//
// 兼容策略（与老 resolveMultipliers 对齐，保证存量计费零破坏）：
//   - 任一新四维 > 0：按新四维逐维取（未设的维返回 0 = 未配置）。
//   - 四维全 0 且老 TrafficMultiplier 被显式设为非默认值（>0 且 !=1.0）：
//     按老 legacy 公式视为「四维均已在 wrapper 级配置」。
//   - 四维全 0 且 TrafficMultiplier 为默认 1.0/缺省：wrapper 级不配置任何维，
//     交给全局默认 / 系统常量（这样新加的全局默认才能对存量 x1.00 模型生效）。
//
// 关键词: wrapperConfiguredDims, wrapper 级逐维配置, legacy 非默认才阻断
func wrapperConfiguredDims(meta *AiModelMeta) resolvedMultipliers {
	var r resolvedMultipliers // 全 0 = 该层什么都没配
	if meta == nil {
		return r
	}
	anyNew := meta.InputTokenMultiplier > 0 ||
		meta.OutputTokenMultiplier > 0 ||
		meta.CacheCreationMultiplier > 0 ||
		meta.CacheHitMultiplier > 0
	if anyNew {
		if meta.InputTokenMultiplier > 0 {
			r.Input = meta.InputTokenMultiplier
		}
		if meta.OutputTokenMultiplier > 0 {
			r.Output = meta.OutputTokenMultiplier
		}
		if meta.CacheCreationMultiplier > 0 {
			r.CacheCreate = meta.CacheCreationMultiplier
		}
		if meta.CacheHitMultiplier > 0 {
			r.CacheHit = meta.CacheHitMultiplier
		}
		return r
	}
	if meta.TrafficMultiplier > 0 && meta.TrafficMultiplier != 1.0 {
		r.Input = meta.TrafficMultiplier
		r.Output = meta.TrafficMultiplier
		r.CacheCreate = meta.TrafficMultiplier * defaultCacheCreationMultiplier
		r.CacheHit = meta.TrafficMultiplier * defaultCacheHitMultiplier
	}
	return r
}

// ResolveBillingMultipliers 按 (外部暴露名 wrapperName + 内部转发名 internalModelName)
// 双标识，逐维分层回落解析出最终生效的四维 Token 倍率。
//
// 优先级（高 -> 低，逐维独立回落）：
//
//	(W,I) override 覆盖行 -> AiModelMeta(W) 逐 wrapper 默认 -> 全局默认 -> 系统常量
//
// 某层某维 > 0 即采用该层该维，否则继续向下回落。internalModelName 为空时跳过 override 层。
//
// 关键词: ResolveBillingMultipliers, 倍率双标识, 四层逐维回落, 计费精确化
func ResolveBillingMultipliers(wrapperName, internalModelName string) resolvedMultipliers {
	cfg, _ := GetGlobalMultiplierConfig()
	meta, _ := GetModelMeta(wrapperName)
	var override *AiModelMultiplierOverride
	if internalModelName != "" {
		override, _ = GetModelMultiplierOverride(wrapperName, internalModelName)
	}
	return resolveBillingMultipliersFrom(cfg, meta, override)
}

// resolveBillingMultipliersFrom 是 ResolveBillingMultipliers 的纯内存版：按已加载好的
// 全局默认 / wrapper meta / (W,I) override 三层数据做逐维分层回落，便于批量场景（如
// portal data 构建）一次性加载后避免 N 次 DB 查询。任一入参为 nil 表示该层缺省。
//
// 优先级（高 -> 低，逐维独立回落）：
//
//	override -> wrapper meta -> 全局默认 -> 系统常量
//
// 关键词: resolveBillingMultipliersFrom, 内存分层回落, 批量解析
func resolveBillingMultipliersFrom(globalCfg *AiModelMultiplierConfig, meta *AiModelMeta, override *AiModelMultiplierOverride) resolvedMultipliers {
	// 最底层：系统常量兜底
	r := resolvedMultipliers{
		Input:         defaultInputTokenMultiplier,
		Output:        defaultOutputTokenMultiplier,
		CacheCreate:   defaultCacheCreationMultiplier,
		CacheHit:      defaultCacheHitMultiplier,
		LegacyTraffic: 1.0,
	}

	// Layer 3: 全局默认
	if globalCfg != nil {
		applyMultiplierDim(&r.Input, globalCfg.InputTokenMultiplier)
		applyMultiplierDim(&r.Output, globalCfg.OutputTokenMultiplier)
		applyMultiplierDim(&r.CacheCreate, globalCfg.CacheCreationMultiplier)
		applyMultiplierDim(&r.CacheHit, globalCfg.CacheHitMultiplier)
	}

	// Layer 2: wrapper meta (AiModelMeta(W)) 逐 wrapper 默认
	w := wrapperConfiguredDims(meta)
	applyMultiplierDim(&r.Input, w.Input)
	applyMultiplierDim(&r.Output, w.Output)
	applyMultiplierDim(&r.CacheCreate, w.CacheCreate)
	applyMultiplierDim(&r.CacheHit, w.CacheHit)

	// Layer 1: (W,I) override 覆盖行（最高优先）
	if override != nil {
		applyMultiplierDim(&r.Input, override.InputTokenMultiplier)
		applyMultiplierDim(&r.Output, override.OutputTokenMultiplier)
		applyMultiplierDim(&r.CacheCreate, override.CacheCreationMultiplier)
		applyMultiplierDim(&r.CacheHit, override.CacheHitMultiplier)
	}

	return r
}

// WeightUsage 按已解析的四维倍率 mul 与上游 SSE 末帧 ChatUsage 计算加权 token。
//
// 公式：
//
//	weighted = input_tokens   * inputMul
//	         + completion     * outputMul
//	         + cache_create   * cacheCreateMul
//	         + cached_tokens  * cacheHitMul
//
// 其中 input_tokens = max(0, prompt_tokens - cached_tokens - cache_create)
// 避免 prompt_tokens 与 cached/cache_create 字段重叠重复计费。
//
// usage == nil 时返回 0；任何字段缺失按 0 处理。
//
// 关键词: WeightUsage, 四维加权 token, 防重叠扣费
func WeightUsage(mul resolvedMultipliers, usage *aispec.ChatUsage) int64 {
	if usage == nil {
		return 0
	}

	prompt := int64(usage.PromptTokens)
	completion := int64(usage.CompletionTokens)
	var cached int64
	var cacheCreate int64
	if usage.PromptTokensDetails != nil {
		cached = int64(usage.PromptTokensDetails.CachedTokens)
		cacheCreate = int64(usage.PromptTokensDetails.CacheCreationInputTokens)
	}

	// 防御：prompt - cached - cache_create 可能因上游 bug 出现负数
	pureInput := prompt - cached - cacheCreate
	if pureInput < 0 {
		pureInput = 0
	}

	weighted := float64(pureInput)*mul.Input +
		float64(completion)*mul.Output +
		float64(cacheCreate)*mul.CacheCreate +
		float64(cached)*mul.CacheHit

	if weighted <= 0 {
		return 0
	}
	return int64(weighted + 0.5)
}

// ComputeWeightedTokens 按上游 SSE 末帧 ChatUsage 与 AiModelMeta 的四维倍率
// 计算本次请求实际消耗的"加权 token"（用于免费用户日限额扣费、付费 key Token 累加）。
//
// 仅按 wrapper(AiModelMeta) 单层解析，保留作为向后兼容入口（旧测试、provider
// 未选定阶段的 in-flight 预估）。精确计费请优先用 ComputeWeightedTokensWithRoute。
//
// usage == nil 时返回 0；任何字段缺失按 0 处理。
//
// 关键词: ComputeWeightedTokens, 四维加权 token, wrapper 单层兼容入口
func ComputeWeightedTokens(meta *AiModelMeta, usage *aispec.ChatUsage) int64 {
	return WeightUsage(resolveMultipliers(meta), usage)
}

// ComputeWeightedTokensWithRoute 按 (外部暴露名 + 内部转发名) 双标识分层解析倍率后，
// 计算加权 token。这是计费正路（onUsageForward / fallback 估算）应当使用的入口。
//
// 关键词: ComputeWeightedTokensWithRoute, 双标识计费正路, 精确扣费
func ComputeWeightedTokensWithRoute(wrapperName, internalModelName string, usage *aispec.ChatUsage) int64 {
	return WeightUsage(ResolveBillingMultipliers(wrapperName, internalModelName), usage)
}
