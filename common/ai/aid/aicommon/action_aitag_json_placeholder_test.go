package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractActionWithAITagJSONPlaceholder_Object(t *testing.T) {
	nonce := "p1"

	input := `{
  "@action": "call-tool",
  "tool": "write_file",
  "params": { "__aitag_json__": "TOOL_PARAMS" }
}

<|TOOL_PARAMS_p1|>
{
  "path": "/tmp/test.json",
  "content": {
    "nested": 1,
    "quote": "a\\\"b",
    "backslash": "path\\\\to\\\\file",
    "arr": [1, 2, {"k": "v"}]
  }
}
<|TOOL_PARAMS_END_p1|>`

	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAMS", "__aitag__params"))

	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err)

	require.Equal(t, "write_file", action.GetString("tool"))
	require.NotEmpty(t, action.GetString("__aitag__params"))

	params := action.GetInvokeParams("params")
	require.False(t, params.Has(AITagJSONPlaceholderKey), "placeholder should be substituted")
	require.Equal(t, "/tmp/test.json", params.GetString("path"))

	content := params.GetObject("content")
	require.Equal(t, int64(1), content.GetInt("nested"))
	require.Equal(t, "a\\\"b", content.GetString("quote"))
	require.Equal(t, "path\\\\to\\\\file", content.GetString("backslash"))
}

func TestExtractActionWithAITagJSONPlaceholder_StringForm(t *testing.T) {
	nonce := "p2"

	input := `{
  "@action": "call-tool",
  "tool": "bash",
  "params": "__aitag_json__:TOOL_PARAMS"
}

<|TOOL_PARAMS_p2|>
{"timeout": 60, "verbose": true}
<|TOOL_PARAMS_END_p2|>`

	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAMS", "__aitag__params"))

	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err)

	params := action.GetInvokeParams("params")
	require.Equal(t, int64(60), params.GetInt("timeout"))
	require.True(t, params.GetBool("verbose"))
}

func TestExtractActionWithAITagJSONPlaceholder_Nested(t *testing.T) {
	nonce := "p3"

	input := `{
  "@action": "call-tool",
  "tool": "http",
  "params": {
    "method": "POST",
    "options": { "__aitag_json__": "TOOL_OPTIONS" }
  }
}

<|TOOL_OPTIONS_p3|>
{"retry": 3, "headers": {"x-test": "1"}}
<|TOOL_OPTIONS_END_p3|>`

	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_OPTIONS", "__aitag__options"))

	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err)

	params := action.GetInvokeParams("params")
	require.Equal(t, "POST", params.GetString("method"))

	options := params.GetObject("options")
	require.Equal(t, int64(3), options.GetInt("retry"))
	require.Equal(t, "1", options.GetObject("headers").GetString("x-test"))
}

func TestExtractActionWithAITagJSONPlaceholder_WithNonceSuffixCompat(t *testing.T) {
	nonce := "px"

	input := `{
  "@action": "call-tool",
  "tool": "bash",
  "params": { "__aitag_json__": "TOOL_PARAMS_px" }
}

<|TOOL_PARAMS_px|>
{"timeout": 10}
<|TOOL_PARAMS_END_px|>`

	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAMS", "__aitag__params"))

	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err)

	params := action.GetInvokeParams("params")
	require.Equal(t, int64(10), params.GetInt("timeout"))
}
