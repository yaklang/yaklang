package aiprojection

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

const (
	actionResponseTagName = "FUNCTION_CALL_ACTION_RESPONSE"
	actionResponseOpenTag = "<|FUNCTION_CALL_ACTION_RESPONSE|>"
	actionResponseEndTag  = "<|FUNCTION_CALL_ACTION_RESPONSE_END|>"
)

type actionResponseAssistant struct {
	Role             string             `json:"role"`
	Content          json.RawMessage    `json:"content"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []*aispec.ToolCall `json:"tool_calls"`
}

type actionResponseTool struct {
	Role       string  `json:"role"`
	ToolCallID string  `json:"tool_call_id"`
	Content    *string `json:"content"`
}

type actionResponsePart struct {
	text     string
	messages []aispec.ChatDetail
}

// expandActionResponseMessages projects trusted timeline records into an
// assistant tool_calls message followed by all matching tool results. Ordinary
// timeline text has its AITAG delimiters escaped before reaching this layer.
func expandActionResponseMessages(messages []aispec.ChatDetail) ([]aispec.ChatDetail, bool) {
	var expanded []aispec.ChatDetail
	changed := false
	for _, message := range messages {
		if message.Role != "user" {
			expanded = append(expanded, message)
			continue
		}
		content, ok := actionResponseMessageText(message.Content)
		if !ok || !strings.Contains(content, "<|"+actionResponseTagName) {
			expanded = append(expanded, message)
			continue
		}
		parts, found, err := splitActionResponseTimeline(content)
		if err != nil {
			log.Warnf("action response projection skipped: %v", err)
			return messages, false
		}
		if !found {
			expanded = append(expanded, message)
			continue
		}
		lastText := -1
		for index, part := range parts {
			if part.messages == nil && strings.TrimSpace(part.text) != "" {
				lastText = index
			}
		}
		cached, cacheContent := message.Content.([]*aispec.ChatContent)
		if cacheContent && lastText < 0 {
			log.Warn("action response projection skipped: cached message has no trailing user text to own cache control")
			return messages, false
		}
		changed = true
		for index, part := range parts {
			if part.messages != nil {
				expanded = append(expanded, part.messages...)
				continue
			}
			if strings.TrimSpace(part.text) != "" {
				user := message.Clone()
				if cacheContent && index == lastText {
					copyPart := *cached[0]
					copyPart.Text = part.text
					user.Content = []*aispec.ChatContent{&copyPart}
				} else {
					user.Content = part.text
				}
				expanded = append(expanded, user)
			}
		}
	}
	if !changed || len(expanded) == 0 {
		return messages, false
	}
	return expanded, true
}

func actionResponseMessageText(content any) (string, bool) {
	switch value := content.(type) {
	case string:
		return value, true
	case []*aispec.ChatContent:
		if len(value) == 1 && value[0] != nil && value[0].Type == "text" {
			return value[0].Text, true
		}
	}
	return "", false
}

func splitActionResponseTimeline(input string) ([]actionResponsePart, bool, error) {
	outer, err := aitag.SplitViaTAG(input, tagPromptSection, tagPromptSectionDynamic)
	if err != nil {
		return nil, false, err
	}
	parts := make([]actionResponsePart, 0, 5)
	appendText := func(raw string) {
		if raw == "" {
			return
		}
		if len(parts) > 0 && parts[len(parts)-1].messages == nil {
			parts[len(parts)-1].text += raw
			return
		}
		parts = append(parts, actionResponsePart{text: raw})
	}
	found := false
	for _, section := range outer.GetOrderedBlocks() {
		openSection := section.IsTagged() && section.TagName == tagPromptSection && section.Nonce == SectionTimelineOpen
		frozenSection := section.IsText() && strings.Contains(section.Raw, "<|AI_CACHE_FROZEN_")
		if !openSection && !frozenSection {
			appendText(section.Raw)
			continue
		}
		// Ordinary timeline text has its AITAG open delimiters escaped by the
		// prompt renderer. Only an internal prompt projection can leave this
		// marker intact inside a TIMELINE block.
		inner, err := aitag.SplitViaTAG(section.Raw, "TIMELINE")
		if err != nil {
			return nil, false, err
		}
		for _, block := range inner.GetOrderedBlocks() {
			if block.IsTagged() {
				if !strings.Contains(block.Content, actionResponseOpenTag) {
					appendText(block.Raw)
					continue
				}
				fragmentParts, fragmentFound, err := splitActionResponseText(block.Content)
				if err != nil {
					return nil, false, err
				}
				for _, part := range fragmentParts {
					if part.messages != nil {
						parts = append(parts, part)
					} else if strings.TrimSpace(part.text) != "" {
						appendText((&aitag.Block{Type: aitag.BlockTypeTagged, TagName: block.TagName, Nonce: block.Nonce, Content: part.text}).Render())
					}
				}
				found = found || fragmentFound
				continue
			}
			if !openSection {
				appendText(block.Raw)
				continue
			}
			fragmentParts, fragmentFound, err := splitActionResponseText(block.Raw)
			if err != nil {
				return nil, false, err
			}
			for _, part := range fragmentParts {
				if part.messages != nil {
					parts = append(parts, part)
				} else {
					appendText(part.text)
				}
			}
			found = found || fragmentFound
		}
	}
	return parts, found, nil
}

func splitActionResponseText(input string) ([]actionResponsePart, bool, error) {
	var parts []actionResponsePart
	found := false
	for len(input) > 0 {
		start := strings.Index(input, "<|"+actionResponseTagName)
		if start < 0 {
			parts = append(parts, actionResponsePart{text: input})
			break
		}
		if start > 0 {
			parts = append(parts, actionResponsePart{text: input[:start]})
		}
		input = input[start:]
		if !strings.HasPrefix(input, actionResponseOpenTag) {
			return nil, false, fmt.Errorf("invalid action response opening tag")
		}
		payloadStart := len(actionResponseOpenTag)
		end := strings.Index(input[payloadStart:], actionResponseEndTag)
		if end < 0 {
			return nil, false, fmt.Errorf("missing action response closing tag")
		}
		end += payloadStart
		messages, err := decodeActionResponseMessages(input[payloadStart:end])
		if err != nil {
			return nil, false, err
		}
		parts = append(parts, actionResponsePart{messages: messages})
		found = true
		input = input[end+len(actionResponseEndTag):]
	}
	return parts, found, nil
}

// A marker is one assistant with N tool calls followed immediately by N
// matching tool results. Decode the entire group before emitting any message;
// malformed input stays ordinary text.
func decodeActionResponseMessages(payload string) ([]aispec.ChatDetail, error) {
	var raw []json.RawMessage
	if !decodeStrictReplayJSON(strings.TrimSpace(payload), &raw) || len(raw) < 2 {
		return nil, fmt.Errorf("action response must contain assistant and tool messages")
	}
	var assistant actionResponseAssistant
	if !decodeStrictReplayJSON(string(raw[0]), &assistant) ||
		assistant.Role != "assistant" || len(assistant.ToolCalls) == 0 ||
		len(raw) != len(assistant.ToolCalls)+1 || len(assistant.Content) == 0 {
		return nil, fmt.Errorf("invalid action response message group")
	}
	var assistantContent any
	if string(assistant.Content) != "null" {
		var visible string
		if err := json.Unmarshal(assistant.Content, &visible); err != nil {
			return nil, fmt.Errorf("invalid assistant content: %w", err)
		}
		assistantContent = visible
	}
	messages := []aispec.ChatDetail{
		{Role: "assistant", Content: assistantContent, ReasoningContent: assistant.ReasoningContent, ToolCalls: assistant.ToolCalls},
	}
	seen := make(map[string]struct{}, len(assistant.ToolCalls))
	for index, call := range assistant.ToolCalls {
		if call == nil || !validActionResponseID(call.ID) || call.Type != "function" ||
			!validActionSchemaName(call.Function.Name) || !validActionArguments(call.Function.Arguments) {
			return nil, fmt.Errorf("invalid action response tool call at index %d", index)
		}
		if _, exists := seen[call.ID]; exists {
			return nil, fmt.Errorf("duplicate action response tool call id %q", call.ID)
		}
		seen[call.ID] = struct{}{}
		var tool actionResponseTool
		if !decodeStrictReplayJSON(string(raw[index+1]), &tool) || tool.Role != "tool" ||
			tool.Content == nil || tool.ToolCallID != call.ID {
			return nil, fmt.Errorf("action response tool result at index %d does not match assistant", index)
		}
		messages = append(messages, aispec.ChatDetail{Role: "tool", ToolCallID: tool.ToolCallID, Content: *tool.Content})
	}
	return messages, nil
}

func validActionResponseID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validActionArguments(arguments string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(arguments), &object) == nil && object != nil
}
