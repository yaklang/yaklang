package aispec

import (
	"strings"
	"sync"
)

// ThinkingBodyMatcher maps provider type / host / model hints to chat-completions extra JSON fields.
// Built-in matchers run first in fixed order; RegisterThinkingBodyMatcher appends custom matchers.
type ThinkingBodyMatcher interface {
	// MatchType matches gateway / provider registration name (AIConfig.Type), e.g. tongyi、openai。
	// 在基于域名/URL 的 MatchHost 之前优先执行。
	MatchType(typeName string) bool
	MatchHost(baseURL, domain string) bool
	MatchModel(modelName string) bool
	Params(enabled bool, reasoningEffort string) map[string]any
}



func allThinkingMatchers() []ThinkingBodyMatcher {
	thinkingMatchersMu.RLock()
	defer thinkingMatchersMu.RUnlock()
	out := make([]ThinkingBodyMatcher, 0, len(builtinThinkingMatchers)+len(extraThinkingMatchers))
	out = append(out, builtinThinkingMatchers...)
	out = append(out, extraThinkingMatchers...)
	return out
}

// RegisterThinkingBodyMatcher registers an extra matcher evaluated after built-ins.
func RegisterThinkingBodyMatcher(m ThinkingBodyMatcher) {
	if m == nil {
		return
	}
	thinkingMatchersMu.Lock()
	defer thinkingMatchersMu.Unlock()
	extraThinkingMatchers = append(extraThinkingMatchers, m)
}

var (
	thinkingMatchersMu    sync.RWMutex
	extraThinkingMatchers []ThinkingBodyMatcher // evaluated after built-ins
	builtinThinkingMatchers = []ThinkingBodyMatcher{
		qwenThinkingMatcher{},
		deepseekFamilyThinkingMatcher{},
		openAICompatibleReasoningMatcher{},
	}
)

// ThinkingExtraBodyForProvider returns top-level JSON fields to merge into the request body
// when the user has set EnableThinking (non-nil). Match order:
//  1) every matcher's MatchType(typeName)（厂商 / aispec 注册名）；
//  2) every matcher's MatchHost(baseURL, domain)；
//  3) every matcher's MatchModel(modelName)；
// 若仍无命中，默认 {"thinking":{"type":"enabled"|"disabled"}}。
//
// reasoningEffort is the raw reasoning effort string (e.g. "low", "medium", "high", "none")
// from AIConfig.ReasoningEffort; it is forwarded to matchers so that providers like OpenAI
// can emit the correct effort level instead of a hardcoded "medium".
func ThinkingExtraBodyForProvider(typeName, modelName, baseURL, domain string, enabled bool, reasoningEffort string) map[string]any {
	// MiniMax 系模型（百炼直供 / 稀宇科技直供）忽略 enable_thinking，仅通过 thinking.type 控制思考。
	// 由于 type=tongyi 会在 MatchType 阶段被 qwenThinkingMatcher 先命中（注入 enable_thinking），
	// 基于模型名的判定无法走到 MatchModel 阶段，因此在此先行短路处理。
	// 注意：MiniMax 仅允许 thinking.type 为 adaptive / disabled，不接受 enabled。
	if strings.Contains(strings.ToLower(modelName), "minimax") {
		return minimaxThinkingExtraBody(enabled)
	}

	ms := allThinkingMatchers()
	typ := strings.ToLower(strings.TrimSpace(typeName))
	for _, m := range ms {
		if m.MatchType(typ) {
			return shallowCloneTopMap(m.Params(enabled, reasoningEffort))
		}
	}
	bu := strings.ToLower(baseURL)
	dm := strings.ToLower(domain)
	for _, m := range ms {
		if m.MatchHost(bu, dm) {
			return shallowCloneTopMap(m.Params(enabled, reasoningEffort))
		}
	}
	ml := strings.ToLower(modelName)
	for _, m := range ms {
		if m.MatchModel(ml) {
			return shallowCloneTopMap(m.Params(enabled, reasoningEffort))
		}
	}
	return defaultThinkingExtraBody(enabled)
}

// minimaxThinkingExtraBody 返回 MiniMax 系模型的思考控制字段。
// 关闭思考使用 disabled；开启时使用 adaptive（MiniMax 不支持 enabled 取值）。
func minimaxThinkingExtraBody(enabled bool) map[string]any {
	t := "disabled"
	if enabled {
		t = "adaptive"
	}
	return map[string]any{
		"thinking": map[string]any{"type": t},
	}
}

func defaultThinkingExtraBody(enabled bool) map[string]any {
	t := "disabled"
	if enabled {
		t = "enabled"
	}
	return map[string]any{
		"thinking": map[string]any{"type": t},
	}
}

func shallowCloneTopMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

type qwenThinkingMatcher struct{}

func (qwenThinkingMatcher) MatchType(typeName string) bool {
	if typeName == "" {
		return false
	}
	if typeName == "tongyi" {
		return true
	}
	if strings.Contains(typeName, "qwen") {
		return true
	}
	switch typeName {
	case "yaklang-writer", "yaklang-rag", "yaklang-com-search", "yakit-plugin-search":
		return true
	default:
		return false
	}
}

func (qwenThinkingMatcher) MatchHost(baseURL, domain string) bool {
	for _, h := range []string{
		"dashscope.aliyuncs.com",
		"dashscope-us.aliyuncs.com",
		"dashscope-intl.aliyuncs.com",
	} {
		if strings.Contains(baseURL, h) || strings.Contains(domain, h) {
			return true
		}
	}
	return false
}

func (qwenThinkingMatcher) MatchModel(modelName string) bool {
	return strings.Contains(modelName, "qwen")
}

func (qwenThinkingMatcher) Params(enabled bool, _ string) map[string]any {
	return map[string]any{"enable_thinking": enabled}
}

type deepseekFamilyThinkingMatcher struct{}

func (deepseekFamilyThinkingMatcher) MatchType(typeName string) bool {
	if typeName == "" {
		return false
	}
	switch typeName {
	case "deepseek", "moonshot", "volcengine", "chatglm", "siliconflow":
		return true
	default:
		return false
	}
}

func (deepseekFamilyThinkingMatcher) MatchHost(baseURL, domain string) bool {
	for _, h := range []string{
		"api.deepseek.com",
		"api.moonshot.ai",
		"open.bigmodel.cn",
		"ark.cn-beijing.volces.com",
	} {
		if strings.Contains(baseURL, h) || strings.Contains(domain, h) {
			return true
		}
	}
	return false
}

func (deepseekFamilyThinkingMatcher) MatchModel(modelName string) bool {
	for _, tok := range []string{"deepseek", "kimi", "glm", "doubao"} {
		if strings.Contains(modelName, tok) {
			return true
		}
	}
	return false
}

func (deepseekFamilyThinkingMatcher) Params(enabled bool, _ string) map[string]any {
	t := "disabled"
	if enabled {
		t = "enabled"
	}
	return map[string]any{"thinking": map[string]any{"type": t}}
}

type openAICompatibleReasoningMatcher struct{}

func (openAICompatibleReasoningMatcher) MatchType(typeName string) bool {
	if typeName == "" {
		return false
	}
	switch typeName {
	case "openai", "gemini", "openrouter":
		return true
	default:
		return false
	}
}

func (openAICompatibleReasoningMatcher) MatchHost(baseURL, domain string) bool {
	for _, h := range []string{"api.openai.com", "generativelanguage.googleapis.com"} {
		if strings.Contains(baseURL, h) || strings.Contains(domain, h) {
			return true
		}
	}
	return false
}

func (openAICompatibleReasoningMatcher) MatchModel(modelName string) bool {
	for _, tok := range []string{"gpt", "gemini"} {
		if strings.Contains(modelName, tok) {
			return true
		}
	}
	return false
}

func (openAICompatibleReasoningMatcher) Params(enabled bool, reasoningEffort string) map[string]any {
	effort := "none"
	if enabled {
		re := strings.TrimSpace(strings.ToLower(reasoningEffort))
		switch re {
		case "low", "medium", "high", "xhigh", "max":
			// Pass through as-is. xhigh/max are only set when ProbedExtendedEfforts
			// confirmed support (enforced in BuildOptionsFromConfig). Unknown or
			// unsupported values are clamped to high before reaching here.
			effort = re
		default:
			if re == "" {
				effort = "medium"
			} else {
				effort = "high"
			}
		}
	}
	return map[string]any{"reasoning": map[string]any{"effort": effort}}
}

// MapReasoningEffortToThinkingConfig interprets the ReasoningEffort field
// and derives the (EnableThinking, ReasoningEffort) pair to inject into
// AIConfig.
//
// ReasoningEffort serves double duty:
//   - Control values: "off" → disable thinking; "auto"/"" → don't inject
//   - Effort levels:  "low"/"medium"/"high"/"xhigh"/"max" → enable + level
//
// The frontend can expose all values including xhigh and max. This function
// only handles the semantic mapping — it does NOT clamp provider-specific
// extensions. Clamping of unsupported xhigh/max values is done in
// BuildOptionsFromConfig based on ProbedExtendedEfforts.
//
//   - "" / "auto"  → (false, "")      — do not inject any thinking params
//   - "off"        → (true, "none")   — explicitly disable thinking
//   - "low"        → (true, "low")    — enable thinking with low effort
//   - "medium"     → (true, "medium") — enable thinking with medium effort
//   - "high"       → (true, "high")   — enable thinking with high effort
//   - "xhigh"      → (true, "xhigh")  — extra-high (clamped to high if not probed)
//   - "max"        → (true, "max")    — maximum (clamped to high if not probed)
//   - other        → (true, <raw>)    — passthrough (matcher clamps if unsupported)
func MapReasoningEffortToThinkingConfig(effort string) (enableThinking bool, reasoningEffort string) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "auto", "default":
		return false, ""
	case "off", "none", "disabled":
		return true, "none"
	case "low":
		return true, "low"
	case "medium":
		return true, "medium"
	case "high":
		return true, "high"
	case "xhigh":
		return true, "xhigh"
	case "max":
		return true, "max"
	default:
		return true, strings.ToLower(strings.TrimSpace(effort))
	}
}
