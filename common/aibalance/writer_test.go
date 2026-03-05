package aibalance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDelta_ReasoningContentFieldName(t *testing.T) {
	w := &chatJSONChunkWriter{
		uid:     "test-uid",
		created: time.Unix(1000000, 0),
		model:   "test-model",
	}

	t.Run("reason=true should use reasoning_content", func(t *testing.T) {
		raw, err := w.buildDelta(true, "thinking about it")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))

		choices := result["choices"].([]any)
		delta := choices[0].(map[string]any)["delta"].(map[string]any)

		_, hasReasoningContent := delta["reasoning_content"]
		assert.True(t, hasReasoningContent, "delta should have 'reasoning_content' field")
		assert.Equal(t, "thinking about it", delta["reasoning_content"])

		_, hasContent := delta["content"]
		assert.False(t, hasContent, "delta should NOT have 'content' field when reason=true")

		_, hasReasonContent := delta["reason_content"]
		assert.False(t, hasReasonContent, "delta should NOT have old 'reason_content' field")
	})

	t.Run("reason=false should use content", func(t *testing.T) {
		raw, err := w.buildDelta(false, "hello world")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))

		choices := result["choices"].([]any)
		delta := choices[0].(map[string]any)["delta"].(map[string]any)

		_, hasContent := delta["content"]
		assert.True(t, hasContent, "delta should have 'content' field")
		assert.Equal(t, "hello world", delta["content"])

		_, hasReasoningContent := delta["reasoning_content"]
		assert.False(t, hasReasoningContent, "delta should NOT have 'reasoning_content' field when reason=false")
	})
}

func TestBuildMessage_ReasoningContentFieldName(t *testing.T) {
	w := &chatJSONChunkWriter{
		uid:     "test-uid",
		created: time.Unix(1000000, 0),
		model:   "test-model",
	}

	t.Run("with reasoning content", func(t *testing.T) {
		raw, err := w.buildMessage("I thought carefully", "the answer is 42")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))

		choices := result["choices"].([]any)
		message := choices[0].(map[string]any)["message"].(map[string]any)

		assert.Equal(t, "I thought carefully", message["reasoning_content"])
		assert.Equal(t, "the answer is 42", message["content"])

		_, hasReasonContent := message["reason_content"]
		assert.False(t, hasReasonContent, "message should NOT have old 'reason_content' field")
	})

	t.Run("without reasoning content", func(t *testing.T) {
		raw, err := w.buildMessage("", "just the answer")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))

		choices := result["choices"].([]any)
		message := choices[0].(map[string]any)["message"].(map[string]any)

		_, hasReasoningContent := message["reasoning_content"]
		assert.False(t, hasReasoningContent, "message should NOT have 'reasoning_content' field when empty")
		assert.Equal(t, "just the answer", message["content"])
	})
}
