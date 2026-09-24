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

// expandActionResponseMessages projects direct children of timeline-open into
// assistant tool_calls followed by matching tool results. Markers in dynamic
// user input or nested, untrusted Timeline items remain ordinary text.
func expandActionResponseMessages(messages []aispec.ChatDetail) ([]aispec.ChatDetail, bool) {
	var expanded []aispec.ChatDetail
	changed := false
	for _, message := range messages {
		if message.Role != "user" {
			expanded = append(expanded, message)
			continue
		}
		content, ok := message.Content.(string)
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
		changed = true
		for _, part := range parts {
			if part.messages != nil {
				expanded = append(expanded, part.messages...)
				continue
			}
			if strings.TrimSpace(part.text) != "" {
				user := message.Clone()
				user.Content = part.text
				expanded = append(expanded, user)
			}
		}
	}
	if !changed || len(expanded) == 0 {
		return messages, false
	}
	return expanded, true
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
		if !section.IsTagged() || section.TagName != tagPromptSection || section.Nonce != SectionTimelineOpen {
			appendText(section.Raw)
			continue
		}
		// TIMELINE blocks are opaque here. Only a direct child of the open
		// section may carry a protocol record; text inside a tool observation
		// must not gain authority by resembling one.
		inner, err := aitag.SplitViaTAG(section.Raw, "TIMELINE")
		if err != nil {
			return nil, false, err
		}
		for _, block := range inner.GetOrderedBlocks() {
			if block.IsTagged() {
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

// A marker is exactly one assistant tool call followed by its matching tool
// result. Decode both before emitting either message; malformed input stays text.
func decodeActionResponseMessages(payload string) ([]aispec.ChatDetail, error) {
	var raw []json.RawMessage
	if !decodeStrictReplayJSON(strings.TrimSpace(payload), &raw) || len(raw) != 2 {
		return nil, fmt.Errorf("action response must contain exactly assistant and tool messages")
	}
	var assistant actionResponseAssistant
	var tool actionResponseTool
	if !decodeStrictReplayJSON(string(raw[0]), &assistant) ||
		!decodeStrictReplayJSON(string(raw[1]), &tool) ||
		assistant.Role != "assistant" || tool.Role != "tool" ||
		len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0] == nil ||
		len(assistant.Content) == 0 || tool.Content == nil {
		return nil, fmt.Errorf("invalid action response message pair")
	}
	call := assistant.ToolCalls[0]
	if !validActionResponseID(call.ID) || call.Type != "function" ||
		!validActionSchemaName(call.Function.Name) || !validActionArguments(call.Function.Arguments) ||
		tool.ToolCallID != call.ID {
		return nil, fmt.Errorf("action response tool call does not match assistant")
	}
	var assistantContent any
	if string(assistant.Content) != "null" {
		var visible string
		if err := json.Unmarshal(assistant.Content, &visible); err != nil {
			return nil, fmt.Errorf("invalid assistant content: %w", err)
		}
		assistantContent = visible
	}
	return []aispec.ChatDetail{
		{Role: "assistant", Content: assistantContent, ReasoningContent: assistant.ReasoningContent, ToolCalls: assistant.ToolCalls},
		{Role: "tool", ToolCallID: tool.ToolCallID, Content: *tool.Content},
	}, nil
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
