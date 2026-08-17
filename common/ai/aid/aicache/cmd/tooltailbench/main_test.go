package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestParseRequestShapePreservesReasoning(t *testing.T) {
	reasoning := "first stage\nsecond stage\n第三阶段"
	messages := []aispec.ChatDetail{
		aispec.NewSystemChatDetail("system"),
		aispec.NewUserChatDetail("task"),
		{
			Role:             "assistant",
			Content:          "checkpoint",
			ReasoningContent: reasoning,
			ToolCalls: []*aispec.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: aispec.FuncReturn{
					Name:      controlToolName,
					Arguments: `{"action":"continue"}`,
				},
			}},
		},
		aispec.NewToolChatDetailWithID("call_1", controlToolName, `{"status":"accepted"}`),
	}
	body, err := json.Marshal(map[string]any{"model": "test", "messages": messages})
	require.NoError(t, err)
	packet := append([]byte("POST /v1/chat/completions HTTP/1.1\r\nContent-Type: application/json\r\n\r\n"), body...)

	shape := parseRequestShape(packet)
	require.Equal(t, []string{"system", "user", "assistant", "tool"}, shape.Roles)
	require.Equal(t, "tool", shape.LastRole)
	require.Equal(t, 1, shape.ReasoningCount)
	require.Equal(t, []int{len([]rune(reasoning))}, shape.ReasoningChars)
	require.Equal(t, []string{textSHA256(reasoning)}, shape.ReasoningSHA256)
	require.Equal(t, 1, shape.ToolCallCount)
	require.Equal(t, 1, shape.ToolResultCount)
}

func TestTailPositionShapes(t *testing.T) {
	base := []aispec.ChatDetail{
		aispec.NewSystemChatDetail("system"),
		aispec.NewUserChatDetail("task"),
	}
	dynamic := aispec.NewUserChatDetail("dynamic")
	assistant := aispec.ChatDetail{Role: "assistant", Content: "", ReasoningContent: "reasoning"}
	tool := aispec.NewToolChatDetailWithID("call_1", controlToolName, "result")

	toolTail := appendMessages(base, assistant, tool)
	userBeforePair := appendMessages(base, dynamic, assistant, tool)
	userAfterTool := appendMessages(base, assistant, tool, dynamic)

	require.Equal(t, []string{"system", "user", "assistant", "tool"}, roles(toolTail))
	require.Equal(t, []string{"system", "user", "user", "assistant", "tool"}, roles(userBeforePair))
	require.Equal(t, []string{"system", "user", "assistant", "tool", "user"}, roles(userAfterTool))
}

func TestSelectedContentResult(t *testing.T) {
	action, answer := selectedContentResult("```json\n{\"@action\":\"finish\",\"answer\":\"A|B\"}\n```")
	require.Equal(t, "finish", action)
	require.Equal(t, "A|B", answer)
}

func TestSelectedOrder(t *testing.T) {
	available := map[string]conditionInput{"a": {}, "b": {}, "c": {}}
	selected, err := selectedOrder([]string{"b", "c", "a"}, "a,b", available)
	require.NoError(t, err)
	require.Equal(t, []string{"b", "a"}, selected)
	_, err = selectedOrder([]string{"a"}, "missing", available)
	require.Error(t, err)
}

func roles(messages []aispec.ChatDetail) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.Role)
	}
	return out
}
