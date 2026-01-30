package aicommon

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractAction_ResolveAITagPlaceholderInJSON(t *testing.T) {
	nonce := "ph001"

	input := `{
		"@action": "call-tool",
		"tool": "test",
		"params": {
			"name": "ok",
			"content": "__aitag__content"
		}
	}

<|TOOL_PARAM_content_ph001|>
line1
line2 "quotes" \\backslashes\\
<|TOOL_PARAM_content_END_ph001|>`

	action, err := ExtractValidActionFromStream(
		context.Background(),
		strings.NewReader(input),
		"call-tool",
		WithActionNonce(nonce),
		WithActionTagToKey("TOOL_PARAM_content", "__aitag__content"),
	)
	require.NoError(t, err)
	require.Equal(t, "line1\nline2 \"quotes\" \\\\backslashes\\\\", action.GetString("__aitag__content"))

	params := action.GetInvokeParams("params")
	require.Equal(t, "ok", params.GetString("name"))
	require.Equal(t, "line1\nline2 \"quotes\" \\\\backslashes\\\\", params.GetString("content"))
	require.Equal(t, params.GetString("content"), action.GetString("params.content"))
	require.Equal(t, params.GetString("content"), action.GetString("content"))
}

func TestExtractAction_ResolveWrappedAITagPlaceholderInJSON(t *testing.T) {
	nonce := "ph002"

	input := fmt.Sprintf(`{
		"@action": "call-tool",
		"tool": "test",
		"params": {
			"content": "{{__aitag__content}}"
		}
	}

<|TOOL_PARAM_content_%s|>
hello
<|TOOL_PARAM_content_END_%s|>`, nonce, nonce)

	action, err := ExtractValidActionFromStream(
		context.Background(),
		strings.NewReader(input),
		"call-tool",
		WithActionNonce(nonce),
		WithActionTagToKey("TOOL_PARAM_content", "__aitag__content"),
	)
	require.NoError(t, err)

	require.Equal(t, "hello", action.GetString("__aitag__content"))

	params := action.GetInvokeParams("params")
	require.Equal(t, "hello", params.GetString("content"))
}
