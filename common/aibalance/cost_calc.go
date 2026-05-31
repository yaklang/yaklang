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
// 关键词: resolvedMultipliers, 倍率解析
type resolvedMultipliers struct {
	Input       float64
	Output      float64
	CacheCreate float64
	CacheHit    float64
}

// resolveMultipliers 根据 AiModelMeta 解析出实际生效的四维 Token 倍率：
//   - 若某一维 ≤ 0，则用该维的默认值（input=1.0/output=1.0/cache_create=1.25/cache_hit=0.1）。
//   - meta == nil 时全部按默认值。
//
// 老的 TrafficMultiplier 字节倍率体系已停用：四维全 0 时直接回落到标准默认倍率，
// 不再受 TrafficMultiplier 影响。
// 关键词: resolveMultipliers, 四维倍率, 老 TrafficMultiplier 停用
func resolveMultipliers(meta *AiModelMeta) resolvedMultipliers {
	r := resolvedMultipliers{
		Input:       defaultInputTokenMultiplier,
		Output:      defaultOutputTokenMultiplier,
		CacheCreate: defaultCacheCreationMultiplier,
		CacheHit:    defaultCacheHitMultiplier,
	}
	if meta == nil {
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

// ResolveModelMultipliers 按「实际模型(内部转发名 internalModelName)」逐维分层回落，
// 解析出最终生效的四维 Token 倍率。这是计费的唯一标识：同一实际模型无论被哪个 wrapper
// 暴露，单价都一致。
//
// 优先级（高 -> 低，逐维独立回落）：
//
//	实际模型倍率(internalModelName) -> 全局默认 -> 系统常量
//
// 某层某维 > 0 即采用该层该维，否则继续向下回落。internalModelName 为空时跳过实际模型层
// （仅全局默认 + 系统常量），用于 provider 尚未选定的 in-flight 预估。
//
// 关键词: ResolveModelMultipliers, 实际模型计费, 三层逐维回落, 计费精确化
func ResolveModelMultipliers(internalModelName string) resolvedMultipliers {
	cfg, _ := GetGlobalMultiplierConfig()
	var m *AiModelMultiplier
	if internalModelName != "" {
		m, _ = GetModelMultiplier(internalModelName)
	}
	return resolveModelMultipliersFrom(cfg, m)
}

// resolveModelMultipliersFrom 是 ResolveModelMultipliers 的纯内存版：按已加载好的
// 全局默认 / 实际模型倍率两层数据做逐维分层回落，便于批量场景（如 portal data 构建）
// 一次性加载后避免 N 次 DB 查询。任一入参为 nil 表示该层缺省。
//
// 优先级（高 -> 低，逐维独立回落）：
//
//	实际模型倍率 -> 全局默认 -> 系统常量
//
// 关键词: resolveModelMultipliersFrom, 内存分层回落, 批量解析
func resolveModelMultipliersFrom(globalCfg *AiModelMultiplierConfig, m *AiModelMultiplier) resolvedMultipliers {
	// 最底层：系统常量兜底
	r := resolvedMultipliers{
		Input:       defaultInputTokenMultiplier,
		Output:      defaultOutputTokenMultiplier,
		CacheCreate: defaultCacheCreationMultiplier,
		CacheHit:    defaultCacheHitMultiplier,
	}

	// Layer 2: 全局默认
	if globalCfg != nil {
		applyMultiplierDim(&r.Input, globalCfg.InputTokenMultiplier)
		applyMultiplierDim(&r.Output, globalCfg.OutputTokenMultiplier)
		applyMultiplierDim(&r.CacheCreate, globalCfg.CacheCreationMultiplier)
		applyMultiplierDim(&r.CacheHit, globalCfg.CacheHitMultiplier)
	}

	// Layer 1: 实际模型倍率（最高优先）
	if m != nil {
		applyMultiplierDim(&r.Input, m.InputTokenMultiplier)
		applyMultiplierDim(&r.Output, m.OutputTokenMultiplier)
		applyMultiplierDim(&r.CacheCreate, m.CacheCreationMultiplier)
		applyMultiplierDim(&r.CacheHit, m.CacheHitMultiplier)
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
// 计算加权 token。这是仅按 wrapper(AiModelMeta) 单层解析的纯工具函数，
// 不再参与计费正路（计费已切换为「实际模型」维度，见 ComputeModelWeightedTokens），
// 仅保留供 wrapper meta 维度的单元测试与诊断使用。
//
// usage == nil 时返回 0；任何字段缺失按 0 处理。
//
// 关键词: ComputeWeightedTokens, 四维加权 token, wrapper meta 工具函数
func ComputeWeightedTokens(meta *AiModelMeta, usage *aispec.ChatUsage) int64 {
	return WeightUsage(resolveMultipliers(meta), usage)
}

// ComputeModelWeightedTokens 按「实际模型(内部转发名 internalModelName)」分层解析倍率后，
// 计算加权 token。这是计费正路（onUsageForward / fallback 估算）应当使用的唯一入口：
// 同一实际模型单价一致，与对外 wrapper 无关。
//
// 若该实际模型被标记为「免费(IsFree)」，则无论四维倍率如何设置，一律返回 0（不计费）。
// 这是替代旧 config per-model exempt 的统一计费豁免开关：免费用户日桶、付费 key Token、
// 付费用户全局日 Token 三道计费都因 weighted=0 而自动豁免。
//
// 关键词: ComputeModelWeightedTokens, 实际模型计费正路, 精确扣费, IsFree 计费豁免
func ComputeModelWeightedTokens(internalModelName string, usage *aispec.ChatUsage) int64 {
	if usage == nil {
		return 0
	}
	var m *AiModelMultiplier
	if internalModelName != "" {
		m, _ = GetModelMultiplier(internalModelName)
	}
	// IsFree 实际模型计费豁免：倍率怎么设都不生效，直接返回 0。
	if m != nil && m.IsFree {
		return 0
	}
	cfg, _ := GetGlobalMultiplierConfig()
	return WeightUsage(resolveModelMultipliersFrom(cfg, m), usage)
}
