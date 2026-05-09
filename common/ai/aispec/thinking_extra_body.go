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
	Params(enabled bool) map[string]any
}

var (
	thinkingMatchersMu      sync.RWMutex
	extraThinkingMatchers   []ThinkingBodyMatcher
	builtinThinkingMatchers = []ThinkingBodyMatcher{
		qwenThinkingMatcher{},
		deepseekFamilyThinkingMatcher{},
		openAICompatibleReasoningMatcher{},
	}
)

// RegisterThinkingBodyMatcher registers an extra matcher evaluated after built-ins.
func RegisterThinkingBodyMatcher(m ThinkingBodyMatcher) {
	if m == nil {
		return
	}
	thinkingMatchersMu.Lock()
	defer thinkingMatchersMu.Unlock()
	extraThinkingMatchers = append(extraThinkingMatchers, m)
}

func allThinkingMatchers() []ThinkingBodyMatcher {
	thinkingMatchersMu.RLock()
	defer thinkingMatchersMu.RUnlock()
	out := make([]ThinkingBodyMatcher, 0, len(builtinThinkingMatchers)+len(extraThinkingMatchers))
	out = append(out, builtinThinkingMatchers...)
	out = append(out, extraThinkingMatchers...)
	return out
}

// ThinkingExtraBodyForProvider returns top-level JSON fields to merge into the request body
// when the user has set EnableThinking (non-nil). Match order:
//  1) every matcher’s MatchType(typeName)（厂商 / aispec 注册名）；
//  2) every matcher’s MatchHost(baseURL, domain)；
//  3) every matcher’s MatchModel(modelName)；
// 若仍无命中，默认 {"thinking":{"type":"enabled"|"disabled"}}。
func ThinkingExtraBodyForProvider(typeName, modelName, baseURL, domain string, enabled bool) map[string]any {
	ms := allThinkingMatchers()
	typ := strings.ToLower(strings.TrimSpace(typeName))
	for _, m := range ms {
		if m.MatchType(typ) {
			return shallowCloneTopMap(m.Params(enabled))
		}
	}
	bu := strings.ToLower(baseURL)
	dm := strings.ToLower(domain)
	for _, m := range ms {
		if m.MatchHost(bu, dm) {
			return shallowCloneTopMap(m.Params(enabled))
		}
	}
	ml := strings.ToLower(modelName)
	for _, m := range ms {
		if m.MatchModel(ml) {
			return shallowCloneTopMap(m.Params(enabled))
		}
	}
	return defaultThinkingExtraBody(enabled)
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

func (qwenThinkingMatcher) Params(enabled bool) map[string]any {
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

func (deepseekFamilyThinkingMatcher) Params(enabled bool) map[string]any {
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

func (openAICompatibleReasoningMatcher) Params(enabled bool) map[string]any {
	effort := "none"
	if enabled {
		effort = "medium"
	}
	return map[string]any{"reasoning": map[string]any{"effort": effort}}
}
