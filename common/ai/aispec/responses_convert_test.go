package aispec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertResponsesInputToChatDetails_String(t *testing.T) {
	msgs, err := ConvertResponsesInputToChatDetails(json.RawMessage(`"hello"`))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "hello", msgs[0].Content)
}

func TestConvertResponsesInputToChatDetails_RoundTripTools(t *testing.T) {
	raw := json.RawMessage(`[
  {"role":"user","content":"hi"},
  {"type":"function_call","call_id":"c1","name":"fn","arguments":"{}"},
  {"type":"function_call_output","call_id":"c1","output":"ok"}
]`)
	msgs, err := ConvertResponsesInputToChatDetails(raw)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, "assistant", msgs[1].Role)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "fn", msgs[1].ToolCalls[0].Function.Name)
	assert.Equal(t, "tool", msgs[2].Role)

	// outbound round-trip including tool history
	out := ConvertChatDetailsToResponsesInput(msgs)
	require.Len(t, out, 3)
	assert.Equal(t, "user", out[0]["role"])
	assert.Equal(t, "function_call", out[1]["type"])
	assert.Equal(t, "c1", out[1]["call_id"])
	assert.Equal(t, "fn", out[1]["name"])
	assert.Equal(t, "function_call_output", out[2]["type"])
	assert.Equal(t, "ok", out[2]["output"])
}

func TestConvertResponsesToolsToChat_Flat(t *testing.T) {
	tools := ConvertResponsesToolsToChat([]any{
		map[string]any{
			"type":        "function",
			"name":        "get_weather",
			"description": "d",
			"parameters":   map[string]any{"type": "object"},
		},
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "get_weather", tools[0].Function.Name)
}

func TestConvertResponsesCreateRequestToChatMessage(t *testing.T) {
	raw := []byte(`{
  "model":"m",
  "input":"hello",
  "stream":true,
  "max_output_tokens":128,
  "reasoning":{"effort":"medium"},
  "tools":[{"type":"function","name":"fn","parameters":{"type":"object"}}],
  "tool_choice":{"type":"function","name":"fn"}
}`)
	msg, err := ConvertResponsesCreateRequestToChatMessage(raw)
	require.NoError(t, err)
	assert.Equal(t, "m", msg.Model)
	assert.True(t, msg.Stream)
	require.NotNil(t, msg.MaxTokens)
	assert.EqualValues(t, 128, *msg.MaxTokens)
	assert.Equal(t, "medium", msg.ReasoningEffort)
	require.Len(t, msg.Messages, 1)
	require.Len(t, msg.Tools, 1)
	assert.Equal(t, "fn", msg.Tools[0].Function.Name)
}

func TestConvertResponsesCreateRequestToChatMessage_RequiresModelAndInput(t *testing.T) {
	_, err := ConvertResponsesCreateRequestToChatMessage([]byte(`{"input":"x"}`))
	assert.Error(t, err)
	_, err = ConvertResponsesCreateRequestToChatMessage([]byte(`{"model":"m"}`))
	assert.Error(t, err)
}
