package yakscript

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// nativeResult is a normalized script result, not an HTTP or provider envelope.
type nativeResult struct {
	Content      string             `json:"content"`
	ToolCalls    []*aispec.ToolCall `json:"tool_calls"`
	FinishReason string             `json:"finish_reason"`
}

func normalizeNativeResult(value any, tools []aispec.Tool) (*nativeResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("yak-script cannot encode native result")
	}
	var result nativeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, errors.New("yak-script expects {content, tool_calls, finish_reason}")
	}
	if len(result.ToolCalls) == 0 {
		if result.FinishReason != "stop" || strings.TrimSpace(result.Content) == "" {
			return nil, errors.New("yak-script native result requires completed content or tool calls")
		}
		return &result, nil
	}
	if result.FinishReason != "tool_calls" {
		return nil, errors.New("yak-script native tool calls did not finish normally")
	}
	allowed := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool.Type == "function" {
			allowed[tool.Function.Name] = true
		}
	}
	seen := make(map[string]bool, len(result.ToolCalls))
	for i, call := range result.ToolCalls {
		if call == nil || strings.TrimSpace(call.ID) == "" || call.Type != "function" || !allowed[call.Function.Name] || seen[call.ID] {
			return nil, fmt.Errorf("yak-script invalid or unadvertised tool call at index %d", i)
		}
		var arguments map[string]json.RawMessage
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil || arguments == nil {
			return nil, fmt.Errorf("yak-script tool call %d arguments must be a complete JSON object", i)
		}
		seen[call.ID] = true
		call.Index = i // Final arrays use their order, not upstream streaming indices.
	}
	return &result, nil
}
