package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractAIOutputDisplayMessage_JSONStringWithAngleBrackets(t *testing.T) {
	msg := `No code generated in "write_code" action: the response is missing <|GEN_CODE_<nonce>|>...<|GEN_CODE_END_<nonce>|> after the @action JSON.`
	raw, err := json.Marshal(msg)
	require.NoError(t, err)

	got := ExtractAIOutputDisplayMessage(raw, true)
	assert.Equal(t, msg, got)
	assert.Contains(t, got, "<|GEN_CODE_")
	assert.NotContains(t, got, `\u003c`)
}

func TestExtractAIOutputDisplayMessage_JSONObjectMessage(t *testing.T) {
	raw := []byte(`{"message":"ReAct task execution failed: reason: <|GEN_CODE_abcd|>"}`)
	got := ExtractAIOutputDisplayMessage(raw, true)
	assert.Equal(t, "ReAct task execution failed: reason: <|GEN_CODE_abcd|>", got)
}

func TestExtractAIOutputDisplayMessage_PlainText(t *testing.T) {
	raw := []byte("plain error")
	got := ExtractAIOutputDisplayMessage(raw, false)
	assert.Equal(t, "plain error", got)
}

func TestAiOutputEvent_DisplayMessage(t *testing.T) {
	msg := "fail: <|tag|>"
	raw, err := json.Marshal(msg)
	require.NoError(t, err)

	ev := &AiOutputEvent{IsJson: true, Content: raw}
	assert.Equal(t, msg, ev.DisplayMessage())
}
