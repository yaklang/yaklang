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
	Input        float64
	Output       float64
	CacheCreate  float64
	CacheHit     float64
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

// ComputeWeightedTokens 按上游 SSE 末帧 ChatUsage 与 AiModelMeta 的四维倍率
// 计算本次请求实际消耗的"加权 token"（用于免费用户日限额扣费、付费 key Token 累加）。
//
// 公式：
//   weighted = input_tokens   * inputMul
//            + completion     * outputMul
//            + cache_create   * cacheCreateMul
//            + cached_tokens  * cacheHitMul
// 其中 input_tokens = max(0, prompt_tokens - cached_tokens - cache_create)
// 避免 prompt_tokens 与 cached/cache_create 字段重叠重复计费。
//
// usage == nil 时返回 0；任何字段缺失按 0 处理。
//
// 关键词: ComputeWeightedTokens, 四维加权 token, 防重叠扣费
func ComputeWeightedTokens(meta *AiModelMeta, usage *aispec.ChatUsage) int64 {
	if usage == nil {
		return 0
	}

	mul := resolveMultipliers(meta)

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
