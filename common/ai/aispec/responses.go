package aispec

import "encoding/json"

// ResponsesCreateRequest is the subset of OpenAI Responses API create body
// that yaklang / aibalance support for Codex and OpenAI-compatible clients.
// Full OpenAI SDK field surface is intentionally not mirrored.
// 关键词: ResponsesCreateRequest, Responses 入站 subset, aispec
type ResponsesCreateRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Stream          bool            `json:"stream"`
	Tools           []any           `json:"tools,omitempty"`
	ToolChoice      any             `json:"tool_choice,omitempty"`
	MaxOutputTokens *int64          `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Reasoning       *ResponsesReasoning `json:"reasoning,omitempty"`
	EnableThinking  bool            `json:"enable_thinking,omitempty"`
}

// ResponsesReasoning is the subset of Responses `reasoning` object.
// 关键词: ResponsesReasoning
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}
