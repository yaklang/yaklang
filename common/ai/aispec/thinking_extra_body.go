package aispec

import (
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// 思考强度参数构建：注册模式
// ---------------------------------------------------------------------------
//
// 设计目标：
//   - 这一层只做「格式构建」——把已确定好的 ThinkingLevel 放进正确的 JSON 字段。
//   - 不做降级——降级由上层（utils.go 的探测逻辑）在调用前完成。
//   - 可扩展——第三方通过 RegisterThinkingParamBuilder 注册自定义构建器。
//
// 调度顺序：
//  1. MiniMax 模型名短路（百炼下 type=tongyi 但需 thinking.type 而非 enable_thinking）
//  2. 按 typeName 匹配已注册的 builder
//  3. 按 baseURL/domain 匹配已注册的 builder
//  4. 按 modelName 匹配已注册的 builder
//  5. 默认 fallback: {"thinking":{"type":"enabled"|"disabled"}}

// ThinkingParamsContext 是传给 builder 的上下文。
type ThinkingParamsContext struct {
	// Level 是已归一化、已降级的思考等级：
	// "none"/"low"/"medium"/"high"/"xhigh"/"max"。
	// 空字符串表示 auto（不注入）。
	Level string
	// Model 是模型名（小写），如 "deepseek-v4-pro"、"kimi-k3"。
	Model string
	// APIType 是请求协议："responses" / "chat_completions" / ""（未知）。
	APIType string
}

// ThinkingParamBuilder 将 ThinkingParamsContext 转换为请求体顶层 JSON 字段。
// 返回 nil 表示不注入任何参数。
type ThinkingParamBuilder func(ctx ThinkingParamsContext) map[string]any

// thinkingBuilderEntry 描述一个 builder 的匹配条件。
type thinkingBuilderEntry struct {
	// builder 是实际构建函数
	builder ThinkingParamBuilder
	// typeNames 是该 builder 匹配的 AIConfig.Type 值（小写）
	typeNames []string
	// hostPatterns 是该 builder 匹配的 baseURL/domain 子串（小写）
	hostPatterns []string
	// modelTokens 是该 builder 匹配的 modelName 子串（小写）
	modelTokens []string
}

var (
	thinkingBuildersMu sync.RWMutex
	thinkingBuilders   []thinkingBuilderEntry
)

// RegisterThinkingParamBuilder 注册一个思考参数构建器。
// typeNames/hostPatterns/modelTokens 均为可选匹配条件，传空切片表示不按该维度匹配。
// 后注册的 builder 优先级更高（插队到队列头部），便于覆盖内置实现。
func RegisterThinkingParamBuilder(builder ThinkingParamBuilder, typeNames, hostPatterns, modelTokens []string) {
	if builder == nil {
		return
	}
	thinkingBuildersMu.Lock()
	defer thinkingBuildersMu.Unlock()
	thinkingBuilders = append([]thinkingBuilderEntry{{
		builder:      builder,
		typeNames:    typeNames,
		hostPatterns: hostPatterns,
		modelTokens:  modelTokens,
	}}, thinkingBuilders...)
}

// ThinkingExtraBodyForProvider 根据 provider 类型/域名/模型名，返回请求体顶层的思考控制 JSON 字段。
//
// thinkingLevel 是已归一化的等级（"none"/"low"/"medium"/"high"/"xhigh"/"max"），
// 空字符串表示 auto（不注入）。
func ThinkingExtraBodyForProvider(typeName, modelName, baseURL, domain, apiType, thinkingLevel string) map[string]any {
	ctx := ThinkingParamsContext{
		Level:   strings.ToLower(strings.TrimSpace(thinkingLevel)),
		Model:   strings.ToLower(strings.TrimSpace(modelName)),
		APIType: strings.ToLower(strings.TrimSpace(apiType)),
	}

	// MiniMax 短路：百炼下 type=tongyi 会被 Qwen builder 命中，
	// 但 MiniMax 需要 thinking.type 而非 enable_thinking，因此先行判断。
	if strings.Contains(ctx.Model, "minimax") {
		return buildMiniMaxParams(ctx)
	}

	typ := strings.ToLower(strings.TrimSpace(typeName))
	bu := strings.ToLower(baseURL)
	dm := strings.ToLower(domain)

	thinkingBuildersMu.RLock()
	builders := make([]thinkingBuilderEntry, len(thinkingBuilders))
	copy(builders, thinkingBuilders)
	thinkingBuildersMu.RUnlock()

	// Phase 1: 按 typeName 匹配
	if typ != "" {
		for _, e := range builders {
			for _, tn := range e.typeNames {
				if tn == typ {
					return e.builder(ctx)
				}
			}
		}
	}

	// Phase 2: 按 host 匹配
	for _, e := range builders {
		for _, h := range e.hostPatterns {
			if strings.Contains(bu, h) || strings.Contains(dm, h) {
				return e.builder(ctx)
			}
		}
	}

	// Phase 3: 按 model 匹配
	for _, e := range builders {
		for _, tok := range e.modelTokens {
			if strings.Contains(ctx.Model, tok) {
				return e.builder(ctx)
			}
		}
	}

	// Phase 4: 默认 fallback
	return buildDefaultParams(ctx)
}

// ---------------------------------------------------------------------------
// 内置 builder
// ---------------------------------------------------------------------------

// buildOpenAIParams — OpenAI / Gemini / OpenRouter
// Responses: {"reasoning":{"effort": level}}
// Chat Completions: {"reasoning_effort": level}
func buildOpenAIParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.Level == "" {
		return nil
	}
	if ctx.APIType == "responses" {
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	return map[string]any{"reasoning_effort": ctx.Level}
}

// buildxAIParams — xAI / Grok
// 与 OpenAI 格式一致，仅 typeName/modelToken 不同。
func buildxAIParams(ctx ThinkingParamsContext) map[string]any {
	return buildOpenAIParams(ctx)
}

// buildDeepSeekParams — DeepSeek
// Responses: {"reasoning":{"effort": level}}
// Chat Completions: {"thinking":{"type":"enabled"},"reasoning_effort": level}
func buildDeepSeekParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.Level == "" {
		return nil
	}
	if ctx.APIType == "responses" {
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	if ctx.Level == "none" {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return map[string]any{
		"thinking":         map[string]any{"type": "enabled"},
		"reasoning_effort": ctx.Level,
	}
}

// buildQwenParams — Qwen / 百炼
// Responses: {"reasoning":{"effort": level}}
// Chat Completions: {"enable_thinking": true/false, "reasoning_effort": level}
func buildQwenParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.APIType == "responses" {
		if ctx.Level == "" {
			return nil
		}
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	// Chat Completions
	if ctx.Level == "" || ctx.Level == "none" {
		return map[string]any{"enable_thinking": false}
	}
	return map[string]any{
		"enable_thinking":  true,
		"reasoning_effort": ctx.Level,
	}
}

// buildKimiParams — Kimi / Moonshot
// Responses: {"reasoning":{"effort": level}}
// Chat Completions K3: {"reasoning_effort": level}
// Chat Completions K2.x: {"thinking":{"type":"enabled","keep":"all"}} / {"thinking":{"type":"disabled"}}
func buildKimiParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.APIType == "responses" {
		if ctx.Level == "" {
			return nil
		}
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	if strings.Contains(ctx.Model, "k3") {
		// K3 用 reasoning_effort
		if ctx.Level == "" {
			return nil
		}
		return map[string]any{"reasoning_effort": ctx.Level}
	}
	// K2.x 用 thinking.type 开关
	if ctx.Level == "" || ctx.Level == "none" {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return map[string]any{"thinking": map[string]any{"type": "enabled", "keep": "all"}}
}

// buildAnthropicParams — Claude
// Claude 4.7+/5.x: {"thinking":{"type":"adaptive","display":"summarized"}}
// Claude ≤4.6: {"thinking":{"type":"enabled","budget_tokens": N, "display":"detailed"}}
func buildAnthropicParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.Level == "" || ctx.Level == "none" {
		return nil
	}
	if isClaudeAdaptiveOnly(ctx.Model) {
		return map[string]any{"thinking": map[string]any{"type": "adaptive", "display": "summarized"}}
	}
	budgetMap := map[string]int64{
		"low":    2048,
		"medium": 8192,
		"high":   16384,
		"xhigh":  32768,
		"max":    32768,
	}
	budget, ok := budgetMap[ctx.Level]
	if !ok {
		budget = 8192
	}
	return map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": budget,
			"display":       "detailed",
		},
	}
}

// buildFamilyParams — volcengine / chatglm / siliconflow / doubao / glm
// Responses: {"reasoning":{"effort": level}}
// Chat Completions: 通用 thinking.type 开关 + reasoning_effort 等级控制。
func buildFamilyParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.APIType == "responses" {
		if ctx.Level == "" || ctx.Level == "none" {
			return nil
		}
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	// Chat Completions
	if ctx.Level == "" || ctx.Level == "none" {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return map[string]any{
		"thinking":         map[string]any{"type": "enabled"},
		"reasoning_effort": ctx.Level,
	}
}

// buildMiniMaxParams — MiniMax（稀宇科技直供）
// {"thinking":{"type":"adaptive"}} 或 {"thinking":{"type":"disabled"}}
func buildMiniMaxParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.Level == "none" {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return map[string]any{"thinking": map[string]any{"type": "adaptive"}}
}

// buildDefaultParams — 未知 provider 的 fallback
// Responses: {"reasoning":{"effort": level}}
// Chat Completions: thinking.type 开关 + reasoning_effort
func buildDefaultParams(ctx ThinkingParamsContext) map[string]any {
	if ctx.APIType == "responses" {
		if ctx.Level == "" || ctx.Level == "none" {
			return nil
		}
		return map[string]any{"reasoning": map[string]any{"effort": ctx.Level}}
	}
	if ctx.Level == "none" {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	if ctx.Level == "" {
		return nil
	}
	return map[string]any{
		"thinking":         map[string]any{"type": "enabled"},
		"reasoning_effort": ctx.Level,
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// isClaudeAdaptiveOnly 通过模型名判断是否仅支持 adaptive 模式。
// Claude 4.7+、Claude 5.x（opus-5/sonnet-5/haiku-5 等）不再支持手动 budget_tokens。
func isClaudeAdaptiveOnly(model string) bool {
	if strings.Contains(model, "4.7") || strings.Contains(model, "4.8") || strings.Contains(model, "4.9") {
		return true
	}
	for _, tok := range []string{"opus-5", "sonnet-5", "haiku-5", "fable-5", "mythos-5", "claude-5"} {
		if strings.Contains(model, tok) {
			return true
		}
	}
	return false
}

// MapReasoningEffortToThinkingConfig — legacy 兼容函数
// 将用户输入的 effort 字符串归一化为 ThinkingLevel。
func MapReasoningEffortToThinkingConfig(effort string) string {
	return normalizeThinkingLevel(effort)
}

// ---------------------------------------------------------------------------
// 内置 builder 注册（init 时自动执行）
// ---------------------------------------------------------------------------

func init() {
	// OpenAI / Gemini / OpenRouter
	RegisterThinkingParamBuilder(buildOpenAIParams,
		[]string{"openai", "gemini", "openrouter"},
		[]string{"api.openai.com", "generativelanguage.googleapis.com"},
		[]string{"gpt", "gemini"},
	)

	// xAI / Grok
	RegisterThinkingParamBuilder(buildxAIParams,
		[]string{"xai", "grok"},
		[]string{"api.x.ai"},
		[]string{"grok"},
	)

	// Anthropic / Claude
	RegisterThinkingParamBuilder(buildAnthropicParams,
		[]string{"anthropic", "claude"},
		[]string{"api.anthropic.com"},
		[]string{"claude"},
	)

	// Qwen / 百炼
	RegisterThinkingParamBuilder(buildQwenParams,
		[]string{"tongyi", "qwen", "yaklang-writer", "yaklang-rag", "yaklang-com-search", "yakit-plugin-search"},
		[]string{"dashscope.aliyuncs.com", "dashscope-us.aliyuncs.com", "dashscope-intl.aliyuncs.com"},
		[]string{"qwen"},
	)

	// DeepSeek
	RegisterThinkingParamBuilder(buildDeepSeekParams,
		[]string{"deepseek"},
		[]string{"api.deepseek.com"},
		[]string{"deepseek"},
	)

	// Kimi / Moonshot
	RegisterThinkingParamBuilder(buildKimiParams,
		[]string{"moonshot"},
		[]string{"api.moonshot.ai"},
		[]string{"kimi"},
	)

	// 通用 family: volcengine / chatglm / siliconflow / doubao / glm
	RegisterThinkingParamBuilder(buildFamilyParams,
		[]string{"volcengine", "chatglm", "siliconflow"},
		[]string{"open.bigmodel.cn", "ark.cn-beijing.volces.com"},
		[]string{"glm", "doubao"},
	)
}
