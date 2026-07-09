package aispec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ConvertChatDetailsToResponsesInput exports convertChatDetailsToResponsesInput
// for gateways (aibalance) that need Chat → Responses request shaping.
// 关键词: ConvertChatDetailsToResponsesInput, 导出 ChatDetail→responses input
func ConvertChatDetailsToResponsesInput(msgs []ChatDetail) []map[string]any {
	return convertChatDetailsToResponsesInput(msgs)
}

// ConvertToolsToResponses exports convertToolsToResponses.
// 关键词: ConvertToolsToResponses
func ConvertToolsToResponses(tools []Tool) []any {
	return convertToolsToResponses(tools)
}

// ConvertToolChoiceToResponses exports convertToolChoiceToResponses.
// 关键词: ConvertToolChoiceToResponses
func ConvertToolChoiceToResponses(choice any) any {
	return convertToolChoiceToResponses(choice)
}

// ConvertResponsesCreateRequestToChatMessage maps a Responses create body
// (raw JSON) into the internal ChatMessage used by ChatBase / gateways.
// 关键词: ConvertResponsesCreateRequestToChatMessage, Responses raw→ChatMessage
func ConvertResponsesCreateRequestToChatMessage(raw []byte) (*ChatMessage, error) {
	var req ResponsesCreateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("invalid responses body: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	messages, err := ConvertResponsesInputToChatDetails(req.Input)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("input is empty")
	}
	msg := &ChatMessage{
		Model:          req.Model,
		Messages:       messages,
		Stream:         req.Stream,
		EnableThinking: req.EnableThinking,
		MaxTokens:      req.MaxOutputTokens,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		Tools:          ConvertResponsesToolsToChat(req.Tools),
		ToolChoice:     ConvertResponsesToolChoiceToChat(req.ToolChoice),
	}
	if req.Reasoning != nil && strings.TrimSpace(req.Reasoning.Effort) != "" {
		msg.ReasoningEffort = strings.TrimSpace(req.Reasoning.Effort)
	}
	return msg, nil
}

// ConvertResponsesInputToChatDetails maps OpenAI Responses `input` (string or
// item array JSON) into chat-completions style []ChatDetail.
// Used by aibalance /v1/responses inbound path.
// 关键词: ConvertResponsesInputToChatDetails, Responses→Chat 入站反向
func ConvertResponsesInputToChatDetails(input json.RawMessage) ([]ChatDetail, error) {
	if len(input) == 0 || string(input) == "null" {
		return nil, fmt.Errorf("input is required")
	}
	var asString string
	if err := json.Unmarshal(input, &asString); err == nil {
		return []ChatDetail{{
			Role:    "user",
			Content: asString,
		}}, nil
	}
	var items []map[string]any
	if err := json.Unmarshal(input, &items); err != nil {
		return nil, fmt.Errorf("input must be string or array: %w", err)
	}
	out := make([]ChatDetail, 0, len(items))
	for _, item := range items {
		detail, ok := convertResponsesInputItemToChatDetail(item)
		if !ok {
			continue
		}
		out = append(out, detail)
	}
	return out, nil
}

func convertResponsesInputItemToChatDetail(item map[string]any) (ChatDetail, bool) {
	typ := strings.ToLower(utils.MapGetString(item, "type"))
	role := strings.ToLower(utils.MapGetString(item, "role"))
	if typ == "" || typ == "message" {
		if role == "" {
			role = "user"
		}
		content := item["content"]
		switch c := content.(type) {
		case string:
			return ChatDetail{Role: role, Content: c}, true
		case nil:
			return ChatDetail{Role: role, Content: ""}, true
		default:
			parts := convertResponsesContentPartsToChat(c)
			if len(parts) == 1 && parts[0].Type == "text" {
				return ChatDetail{Role: role, Content: parts[0].Text}, true
			}
			if len(parts) == 0 {
				return ChatDetail{Role: role, Content: utils.InterfaceToString(c)}, true
			}
			return ChatDetail{Role: role, Content: parts}, true
		}
	}
	if typ == "function_call" {
		name := utils.MapGetString(item, "name")
		callID := utils.MapGetString(item, "call_id")
		if callID == "" {
			callID = utils.MapGetString(item, "id")
		}
		args := utils.MapGetString(item, "arguments")
		tc := &ToolCall{
			Index: 0,
			ID:    callID,
			Type:  "function",
			Function: FuncReturn{
				Name:      name,
				Arguments: args,
			},
		}
		return ChatDetail{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []*ToolCall{tc},
		}, true
	}
	if typ == "function_call_output" {
		callID := utils.MapGetString(item, "call_id")
		output := item["output"]
		var content string
		switch o := output.(type) {
		case string:
			content = o
		default:
			b, _ := json.Marshal(o)
			content = string(b)
		}
		return ChatDetail{
			Role:       "tool",
			Content:    content,
			ToolCallID: callID,
		}, true
	}
	return ChatDetail{}, false
}

func convertResponsesContentPartsToChat(content any) []*ChatContent {
	arr, ok := content.([]any)
	if !ok {
		raw, err := json.Marshal(content)
		if err != nil {
			return nil
		}
		_ = json.Unmarshal(raw, &arr)
	}
	parts := make([]*ChatContent, 0, len(arr))
	for _, el := range arr {
		m := utils.InterfaceToGeneralMap(el)
		if m == nil {
			continue
		}
		t := strings.ToLower(utils.MapGetString(m, "type"))
		switch t {
		case "input_text", "output_text", "text":
			parts = append(parts, NewUserChatContentText(utils.MapGetString(m, "text")))
		case "input_image", "image_url":
			url := ""
			if img := utils.MapGetMapRaw(m, "image_url"); img != nil {
				url = utils.MapGetString(img, "url")
			}
			if url == "" {
				url = utils.MapGetString(m, "image_url")
			}
			if url == "" {
				url = utils.MapGetString(m, "url")
			}
			if url != "" {
				parts = append(parts, NewUserChatContentImageUrl(url))
			}
		}
	}
	return parts
}

// ConvertResponsesToolsToChat maps Responses flat tools / nested chat tools
// into []Tool for Chat Completions style clients.
// 关键词: ConvertResponsesToolsToChat
func ConvertResponsesToolsToChat(tools []any) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		m := utils.InterfaceToGeneralMap(t)
		if len(m) == 0 {
			b, err := json.Marshal(t)
			if err != nil {
				continue
			}
			var mm map[string]any
			if json.Unmarshal(b, &mm) != nil || len(mm) == 0 {
				continue
			}
			m = mm
		}
		typ := utils.MapGetString(m, "type")
		if typ == "" {
			typ = "function"
		}
		if name := utils.MapGetString(m, "name"); name != "" {
			out = append(out, Tool{
				Type: typ,
				Function: ToolFunction{
					Name:        name,
					Description: utils.MapGetString(m, "description"),
					Parameters:  m["parameters"],
				},
			})
			continue
		}
		if fn := utils.MapGetMapRaw(m, "function"); fn != nil {
			out = append(out, Tool{
				Type: typ,
				Function: ToolFunction{
					Name:        utils.MapGetString(fn, "name"),
					Description: utils.MapGetString(fn, "description"),
					Parameters:  fn["parameters"],
				},
			})
		}
	}
	return out
}

// ConvertResponsesToolChoiceToChat maps Responses tool_choice into chat style.
// 关键词: ConvertResponsesToolChoiceToChat
func ConvertResponsesToolChoiceToChat(choice any) any {
	if choice == nil {
		return nil
	}
	if s, ok := choice.(string); ok {
		return s
	}
	m := utils.InterfaceToGeneralMap(choice)
	if m == nil {
		return choice
	}
	if utils.MapGetString(m, "type") == "function" {
		if name := utils.MapGetString(m, "name"); name != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}
		}
	}
	return choice
}
