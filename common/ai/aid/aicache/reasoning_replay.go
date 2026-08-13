package aicache

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

const timelineModelThinkingReplayTagName = "TIMELINE_MODEL_THINKING"

type modelThinkingReplayRecord struct {
	ReasoningContent string `json:"reasoning_content"`
	Content          string `json:"content"`
}

type reasoningReplayPart struct {
	text      string
	assistant *aispec.ChatDetail
}

// expandReasoningReplayMessages converts internal TIMELINE_MODEL_THINKING
// markers inside user messages into standard assistant messages. Conversion is
// atomic: a malformed marker, invalid payload, or unsafe terminal assistant
// leaves the complete original message list untouched.
func expandReasoningReplayMessages(result *aispec.ChatBaseMirrorResult) *aispec.ChatBaseMirrorResult {
	if result == nil || !result.IsHijacked || len(result.Messages) == 0 {
		return result
	}

	expanded := make([]aispec.ChatDetail, 0, len(result.Messages)+2)
	changed := false
	for _, message := range result.Messages {
		if message.Role != "user" {
			expanded = append(expanded, message)
			continue
		}
		fragments, found, err := expandReasoningReplayUserMessage(message)
		if err != nil {
			log.Warnf("reasoning replay message expansion skipped: %v", err)
			return result
		}
		if !found {
			expanded = append(expanded, message)
			continue
		}
		changed = true
		expanded = append(expanded, fragments...)
	}

	if !changed || len(expanded) == 0 {
		return result
	}
	if expanded[len(expanded)-1].Role == "assistant" {
		log.Warn("reasoning replay message expansion skipped: transformed history would end with assistant")
		return result
	}

	copyResult := *result
	copyResult.Messages = expanded
	return &copyResult
}

func expandReasoningReplayUserMessage(message aispec.ChatDetail) ([]aispec.ChatDetail, bool, error) {
	switch content := message.Content.(type) {
	case string:
		parts, found, err := splitReasoningReplayText(content)
		if err != nil || !found {
			return nil, found, err
		}
		return buildStringReplayMessages(message, parts), true, nil
	case []*aispec.ChatContent:
		return buildChatContentReplayMessages(message, content)
	default:
		return nil, false, nil
	}
}

func splitReasoningReplayText(input string) ([]reasoningReplayPart, bool, error) {
	result, err := aitag.SplitViaTAG(input, timelineModelThinkingReplayTagName)
	if err != nil {
		return nil, false, err
	}
	if result == nil {
		return nil, false, nil
	}

	parts := make([]reasoningReplayPart, 0, result.Len())
	found := false
	for _, block := range result.GetOrderedBlocks() {
		if block == nil {
			continue
		}
		if block.IsText() {
			if block.Content != "" {
				parts = append(parts, reasoningReplayPart{text: block.Content})
			}
			continue
		}
		if strings.Contains(block.Content, "<|"+timelineModelThinkingReplayTagName+"_") {
			return nil, false, fmt.Errorf("nested %s marker is not allowed", timelineModelThinkingReplayTagName)
		}

		var replay modelThinkingReplayRecord
		if err := json.Unmarshal([]byte(block.Content), &replay); err != nil {
			return nil, false, fmt.Errorf("decode %s payload: %w", timelineModelThinkingReplayTagName, err)
		}
		if strings.Contains(replay.ReasoningContent, "<|"+timelineModelThinkingReplayTagName+"_") ||
			strings.Contains(replay.Content, "<|"+timelineModelThinkingReplayTagName+"_") {
			return nil, false, fmt.Errorf("nested %s marker is not allowed", timelineModelThinkingReplayTagName)
		}
		if strings.TrimSpace(replay.ReasoningContent) == "" || strings.TrimSpace(replay.Content) == "" {
			return nil, false, fmt.Errorf("%s payload requires non-empty reasoning_content and content", timelineModelThinkingReplayTagName)
		}
		assistant := aispec.NewAssistantChatDetailWithReasoningContent(replay.Content, replay.ReasoningContent)
		parts = append(parts, reasoningReplayPart{assistant: &assistant})
		found = true
	}
	return parts, found, nil
}

func buildStringReplayMessages(source aispec.ChatDetail, parts []reasoningReplayPart) []aispec.ChatDetail {
	messages := make([]aispec.ChatDetail, 0, len(parts)+1)
	var text strings.Builder
	flushUser := func() {
		if strings.TrimSpace(text.String()) == "" {
			text.Reset()
			return
		}
		user := source.Clone()
		user.Content = text.String()
		messages = append(messages, user)
		text.Reset()
	}
	for _, part := range parts {
		if part.assistant == nil {
			text.WriteString(part.text)
			continue
		}
		flushUser()
		messages = append(messages, part.assistant.Clone())
	}
	flushUser()
	return messages
}

func buildChatContentReplayMessages(source aispec.ChatDetail, contents []*aispec.ChatContent) ([]aispec.ChatDetail, bool, error) {
	type contentPart struct {
		content   *aispec.ChatContent
		assistant *aispec.ChatDetail
	}
	parts := make([]contentPart, 0, len(contents)+2)
	foundAny := false

	for _, content := range contents {
		if content == nil {
			parts = append(parts, contentPart{})
			continue
		}
		if content.Type != "text" {
			parts = append(parts, contentPart{content: cloneChatContent(content)})
			continue
		}

		textParts, found, err := splitReasoningReplayText(content.Text)
		if err != nil {
			return nil, false, err
		}
		if !found {
			parts = append(parts, contentPart{content: cloneChatContent(content)})
			continue
		}
		foundAny = true

		lastText := -1
		for index, part := range textParts {
			if part.assistant == nil && part.text != "" {
				lastText = index
			}
		}
		if content.CacheControl != nil && lastText < 0 {
			return nil, false, fmt.Errorf("cannot preserve cache_control for replay-only ChatContent")
		}
		for index, part := range textParts {
			if part.assistant != nil {
				parts = append(parts, contentPart{assistant: part.assistant})
				continue
			}
			if part.text == "" {
				continue
			}
			cloned := cloneChatContent(content)
			cloned.Text = part.text
			cloned.CacheControl = nil
			if index == lastText {
				cloned.CacheControl = content.CacheControl
			}
			parts = append(parts, contentPart{content: cloned})
		}
	}

	if !foundAny {
		return nil, false, nil
	}

	messages := make([]aispec.ChatDetail, 0, len(parts)+1)
	current := make([]*aispec.ChatContent, 0, len(contents))
	flushUser := func() {
		if !hasMeaningfulChatContent(current) {
			current = nil
			return
		}
		user := source.Clone()
		user.Content = current
		messages = append(messages, user)
		current = nil
	}
	for _, part := range parts {
		if part.assistant == nil {
			current = append(current, part.content)
			continue
		}
		flushUser()
		messages = append(messages, part.assistant.Clone())
	}
	flushUser()
	return messages, true, nil
}

func cloneChatContent(content *aispec.ChatContent) *aispec.ChatContent {
	if content == nil {
		return nil
	}
	cloned := *content
	return &cloned
}

func hasMeaningfulChatContent(contents []*aispec.ChatContent) bool {
	for _, content := range contents {
		if content == nil {
			continue
		}
		if content.Type != "text" || strings.TrimSpace(content.Text) != "" || content.CacheControl != nil {
			return true
		}
	}
	return false
}
