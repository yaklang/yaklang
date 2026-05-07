package aicache

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 与 aireact/prompt_loop_materials.go 中 wrapPromptMessageSection(model-thinking)
// 的字面量保持一致 (避免 aicache → aireact import cycle).
const (
	priorModelThinkingSectionNonce = "model-thinking"
	priorModelThinkingStartTag     = "<|PROMPT_SECTION_" + priorModelThinkingSectionNonce + "|>"
	priorModelThinkingEndTag       = "<|PROMPT_SECTION_END_" + priorModelThinkingSectionNonce + "|>"
	// priorAssistantMessageSurface 是发往上游的 assistant 消息的可见 content，
	// 实际思考正文放在 ChatDetail.ReasoningContent -> JSON reasoning_content。
	priorAssistantMessageSurface = "我需要思考一下，以更好的解决用户的问题。"
)

// stripPriorModelThinkingBlock 从一段 prompt 中移除首个
// PROMPT_SECTION_model-thinking 块, 返回剩余文本与块内正文 (不含外层标签).
// 未找到块时 rest==s, thinking=="".
func stripPriorModelThinkingBlock(s string) (rest string, thinking string) {
	if s == "" {
		return s, ""
	}
	start := strings.Index(s, priorModelThinkingStartTag)
	if start < 0 {
		return s, ""
	}
	afterTag := start + len(priorModelThinkingStartTag)
	for afterTag < len(s) && (s[afterTag] == '\n' || s[afterTag] == '\r') {
		afterTag++
	}
	endRel := strings.Index(s[afterTag:], priorModelThinkingEndTag)
	if endRel < 0 {
		return s, ""
	}
	endAbs := afterTag + endRel
	thinking = strings.TrimSpace(s[afterTag:endAbs])
	suffix := s[endAbs+len(priorModelThinkingEndTag):]
	for len(suffix) > 0 && (suffix[0] == '\n' || suffix[0] == '\r') {
		suffix = suffix[1:]
	}
	prefix := s[:start]
	rest = strings.TrimSpace(prefix + suffix)
	return rest, thinking
}

func priorThinkingAssistantMessage(thinking string) aispec.ChatDetail {
	return aispec.NewAssistantChatDetailWithReasoningContent(priorAssistantMessageSurface, thinking)
}

func appendAssistantThinkingIfAny(msgs []aispec.ChatDetail, thinking string) []aispec.ChatDetail {
	if strings.TrimSpace(thinking) == "" {
		return msgs
	}
	return append(msgs, priorThinkingAssistantMessage(thinking))
}
