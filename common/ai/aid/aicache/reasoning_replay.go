package aicache

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

const (
	timelineModelThinkingReplayTagName       = "TIMELINE_MODEL_THINKING_V1"
	legacyTimelineModelThinkingReplayTagName = "TIMELINE_MODEL_THINKING"
	timelineModelThinkingReplayVersion       = 1
)

type modelThinkingReplayRecord struct {
	Version          int    `json:"v"`
	ReasoningContent string `json:"reasoning_content"`
	Content          string `json:"content"`
}

type legacyModelThinkingReplayRecord struct {
	ReasoningContent string `json:"reasoning_content"`
	Content          string `json:"content"`
}

type reasoningReplayPart struct {
	text      string
	assistant *aispec.ChatDetail
}

// expandReasoningReplayMessages converts internal TIMELINE_MODEL_THINKING
// markers inside user messages into standard assistant messages. Malformed or
// untrusted marker-like text is isolated as ordinary user content so one bad
// Timeline item cannot suppress later valid replay records. Conversion still
// fails closed when cache-control ownership cannot be preserved or when the
// transformed request would end in an assistant message.
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
	parts := make([]reasoningReplayPart, 0, 4)
	found := false
	textStart := 0
	searchFrom := 0
	for searchFrom < len(input) {
		markerStart, tagName, legacy := nextReasoningReplayCandidate(input, searchFrom)
		if markerStart < 0 {
			break
		}
		markerPrefix := "<|" + tagName + "_"
		// Prompt projections are emitted as standalone lines. Protocol literals
		// outside a Timeline-owned container, or inside source code/diffs/prose,
		// must remain ordinary text rather than becoming an assistant message.
		if (markerStart > 0 && input[markerStart-1] != '\n' && !(found && markerStart == textStart)) ||
			!isInsideReasoningReplayContainer(input, markerStart) ||
			(legacy && !hasLegacyModelThinkingHeader(input, markerStart)) {
			searchFrom = markerStart + len(markerPrefix)
			continue
		}

		openEndRelative := strings.Index(input[markerStart:], "|>")
		if openEndRelative < 0 {
			break
		}
		openEnd := markerStart + openEndRelative + len("|>")
		nonce := input[markerStart+len(markerPrefix) : markerStart+openEndRelative]
		if !isReasoningReplayNonce(nonce) || openEnd >= len(input) || input[openEnd] != '\n' {
			searchFrom = markerStart + len(markerPrefix)
			continue
		}

		endMarker := "\n<|" + tagName + "_END_" + nonce + "|>"
		payloadStart := openEnd + 1
		endRelative := strings.Index(input[payloadStart:], endMarker)
		if endRelative < 0 {
			searchFrom = markerStart + len(markerPrefix)
			continue
		}
		payloadEnd := payloadStart + endRelative
		payload := input[payloadStart:payloadEnd]
		if strings.Contains(payload, "<|TIMELINE_MODEL_THINKING") {
			searchFrom = markerStart + len(markerPrefix)
			continue
		}

		replay, ok := decodeReasoningReplayPayload(payload, legacy)
		if !ok {
			searchFrom = markerStart + len(markerPrefix)
			continue
		}
		if markerStart > textStart {
			parts = append(parts, reasoningReplayPart{text: input[textStart:markerStart]})
		}
		assistant := aispec.NewAssistantChatDetailWithReasoningContent(replay.Content, replay.ReasoningContent)
		parts = append(parts, reasoningReplayPart{assistant: &assistant})
		found = true
		textStart = payloadEnd + len(endMarker)
		searchFrom = textStart
	}
	if !found {
		return nil, false, nil
	}
	if textStart < len(input) {
		parts = append(parts, reasoningReplayPart{text: input[textStart:]})
	}
	return parts, found, nil
}

func nextReasoningReplayCandidate(input string, searchFrom int) (int, string, bool) {
	currentPrefix := "<|" + timelineModelThinkingReplayTagName + "_"
	legacyPrefix := "<|" + legacyTimelineModelThinkingReplayTagName + "_"
	currentAt := strings.Index(input[searchFrom:], currentPrefix)
	legacyAt := strings.Index(input[searchFrom:], legacyPrefix)
	if currentAt < 0 && legacyAt < 0 {
		return -1, "", false
	}
	if currentAt >= 0 && (legacyAt < 0 || currentAt <= legacyAt) {
		return searchFrom + currentAt, timelineModelThinkingReplayTagName, false
	}
	return searchFrom + legacyAt, legacyTimelineModelThinkingReplayTagName, true
}

func decodeReasoningReplayPayload(payload string, legacy bool) (modelThinkingReplayRecord, bool) {
	if legacy {
		var old legacyModelThinkingReplayRecord
		if !decodeStrictReplayJSON(payload, &old) {
			return modelThinkingReplayRecord{}, false
		}
		return modelThinkingReplayRecord{
			Version:          timelineModelThinkingReplayVersion,
			ReasoningContent: old.ReasoningContent,
			Content:          old.Content,
		}, strings.TrimSpace(old.ReasoningContent) != "" && strings.TrimSpace(old.Content) != ""
	}

	var replay modelThinkingReplayRecord
	if !decodeStrictReplayJSON(payload, &replay) || replay.Version != timelineModelThinkingReplayVersion {
		return modelThinkingReplayRecord{}, false
	}
	return replay, strings.TrimSpace(replay.ReasoningContent) != "" && strings.TrimSpace(replay.Content) != ""
}

func decodeStrictReplayJSON(payload string, target any) bool {
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

func isInsideReasoningReplayContainer(input string, markerStart int) bool {
	if markerStart < 0 || markerStart > len(input) {
		return false
	}
	for _, container := range []struct {
		openPrefix string
		fixedNonce string
	}{
		{openPrefix: "<|TIMELINE_b"},
		{openPrefix: "<|TIMELINE_RECENT|>", fixedNonce: "RECENT"},
	} {
		searchFrom := 0
		for searchFrom < markerStart {
			relative := strings.Index(input[searchFrom:markerStart], container.openPrefix)
			if relative < 0 {
				break
			}
			openStart := searchFrom + relative
			searchFrom = openStart + len(container.openPrefix)
			if openStart > 0 && input[openStart-1] != '\n' {
				continue
			}

			nonce := container.fixedNonce
			openEnd := openStart + len(container.openPrefix)
			if nonce == "" {
				relativeEnd := strings.Index(input[openStart:], "|>")
				if relativeEnd < 0 {
					continue
				}
				openEnd = openStart + relativeEnd + len("|>")
				nonce = input[openStart+len("<|TIMELINE_") : openStart+relativeEnd]
				if !isReasoningReplayNonce(nonce) || !strings.HasPrefix(nonce, "b") {
					continue
				}
			}
			if openEnd >= len(input) || input[openEnd] != '\n' || markerStart <= openEnd {
				continue
			}
			endMarker := "\n<|TIMELINE_END_" + nonce + "|>"
			endRelative := strings.Index(input[openEnd+1:], endMarker)
			if endRelative < 0 {
				continue
			}
			endStart := openEnd + 1 + endRelative
			if markerStart < endStart {
				return true
			}
		}
	}
	return false
}

func hasLegacyModelThinkingHeader(input string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	before := strings.TrimSuffix(input[:markerStart], "\n")
	lineStart := strings.LastIndex(before, "\n") + 1
	lastLine := strings.TrimSpace(before[lineStart:])
	if !strings.Contains(strings.ToLower(lastLine), "[model_thinking]") {
		return false
	}
	// Full Timeline rendering has an additional typed entry header. The recent
	// lightweight projection only has the model_thinking line, so that line is
	// the minimum compatibility proof accepted for persisted legacy records.
	return true
}

func isReasoningReplayNonce(nonce string) bool {
	if nonce == "" || strings.HasPrefix(nonce, "END_") {
		return false
	}
	for _, char := range nonce {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
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
