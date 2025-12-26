package aicommon

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestExtractActionWithAITag tests extracting action with AITAG parameters
func TestExtractActionWithAITag(t *testing.T) {
	nonce := "abc123"

	tests := []struct {
		name           string
		input          string
		expectedParams map[string]string
		paramNames     []string
	}{
		{
			name: "pure JSON params",
			input: `{
				"@action": "call-tool",
				"tool": "test_tool",
				"params": {
					"name": "test",
					"value": "simple"
				}
			}`,
			expectedParams: map[string]string{
				"name":  "test",
				"value": "simple",
			},
			paramNames: []string{"name", "value"},
		},
		{
			name: "pure AITAG params",
			input: `{
				"@action": "call-tool",
				"tool": "test_tool",
				"params": {}
			}
<|TOOL_PARAM_script_abc123|>
#!/bin/bash
echo "Hello World"
for i in {1..10}; do
    echo "Line $i"
done
<|TOOL_PARAM_script_END_abc123|>`,
			expectedParams: map[string]string{
				"script": "#!/bin/bash\necho \"Hello World\"\nfor i in {1..10}; do\n    echo \"Line $i\"\ndone",
			},
			paramNames: []string{"script"},
		},
		{
			name: "hybrid JSON and AITAG params",
			input: `{
				"@action": "call-tool",
				"tool": "bash",
				"params": {
					"timeout": 30,
					"verbose": true
				}
			}

<|TOOL_PARAM_command_abc123|>
find /home/user -name "*.txt" -maxdepth 3 | while read file; do
    echo "Processing: $file"
    grep -l "pattern" "$file"
done
<|TOOL_PARAM_command_END_abc123|>`,
			expectedParams: map[string]string{
				"command": "find /home/user -name \"*.txt\" -maxdepth 3 | while read file; do\n    echo \"Processing: $file\"\n    grep -l \"pattern\" \"$file\"\ndone",
			},
			paramNames: []string{"timeout", "verbose", "command"},
		},
		{
			name: "AITAG with special characters",
			input: `{
				"@action": "call-tool",
				"tool": "write_file",
				"params": {
					"path": "/tmp/test.json"
				}
			}
<|TOOL_PARAM_content_abc123|>
{
  "nested": "json",
  "with": ["arrays", "and", "special chars: \"quotes\""],
  "backslash": "path\\to\\file"
}
<|TOOL_PARAM_content_END_abc123|>`,
			expectedParams: map[string]string{
				"path":    "/tmp/test.json",
				"content": "{\n  \"nested\": \"json\",\n  \"with\": [\"arrays\", \"and\", \"special chars: \\\"quotes\\\"\"],\n  \"backslash\": \"path\\\\to\\\\file\"\n}",
			},
			paramNames: []string{"path", "content"},
		},
		{
			name: "multiple AITAG params",
			input: `{
				"@action": "call-tool",
				"tool": "execute",
				"params": {
					"name": "test"
				}
			}

<|TOOL_PARAM_script_abc123|>
#!/bin/bash
echo "Script content"
<|TOOL_PARAM_script_END_abc123|>

<|TOOL_PARAM_config_abc123|>
key1=value1
key2=value2
<|TOOL_PARAM_config_END_abc123|>`,
			expectedParams: map[string]string{
				"name":   "test",
				"script": "#!/bin/bash\necho \"Script content\"",
				"config": "key1=value1\nkey2=value2",
			},
			paramNames: []string{"name", "script", "config"},
		},
		{
			name: "AITAG overrides JSON param",
			input: `{
				"@action": "call-tool",
				"tool": "test",
				"params": {
					"content": "old value from json"
				}
			}

<|TOOL_PARAM_content_abc123|>
new value from aitag
<|TOOL_PARAM_content_END_abc123|>`,
			expectedParams: map[string]string{
				"content": "new value from aitag",
			},
			paramNames: []string{"content"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build action maker options for AITAG support
			var actionOpts []ActionMakerOption
			actionOpts = append(actionOpts, WithActionNonce(nonce))

			// Register AITAG handlers for each parameter
			for _, paramName := range tt.paramNames {
				tagName := fmt.Sprintf("TOOL_PARAM_%s", paramName)
				actionOpts = append(actionOpts, WithActionTagToKey(tagName, fmt.Sprintf("__aitag__%s", paramName)))
			}

			// Extract action from stream
			ctx := context.Background()
			action, err := ExtractValidActionFromStream(ctx, strings.NewReader(tt.input), "call-tool", actionOpts...)
			require.NoError(t, err, "failed to extract action")

			// Get params from JSON
			invokeParams := aitool.InvokeParams{}
			for k, v := range action.GetInvokeParams("params") {
				invokeParams.Set(k, v)
			}

			// Merge AITAG params
			for _, paramName := range tt.paramNames {
				aitagKey := fmt.Sprintf("__aitag__%s", paramName)
				if aitagValue := action.GetString(aitagKey); aitagValue != "" {
					invokeParams.Set(paramName, aitagValue)
				}
			}

			// Verify expected params
			for key, expectedValue := range tt.expectedParams {
				actualValue := invokeParams.GetString(key)
				require.Equal(t, expectedValue, actualValue, "param %s mismatch", key)
			}
		})
	}
}

// TestExtractActionWithAITagStream tests extracting action from streaming input
func TestExtractActionWithAITagStream(t *testing.T) {
	nonce := "xyz789"

	// Simulate streaming input
	input := `Some thinking process here...

Based on the context, I'll generate the parameters:

` + "```json" + `
{
	"@action": "call-tool",
	"tool": "bash",
	"params": {
		"timeout": 60
	}
}
` + "```" + `

<|TOOL_PARAM_command_xyz789|>
#!/bin/bash
set -e

echo "Starting process..."

for file in *.log; do
    echo "Processing $file"
    gzip "$file"
done

echo "Done!"
<|TOOL_PARAM_command_END_xyz789|>

This command will compress all log files in the current directory.
`

	// Build action maker options
	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAM_command", "__aitag__command"))

	ctx := context.Background()
	action, err := ExtractValidActionFromStream(ctx, strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err, "failed to extract action")

	// Verify JSON params
	invokeParams := aitool.InvokeParams{}
	for k, v := range action.GetInvokeParams("params") {
		invokeParams.Set(k, v)
	}

	// Get timeout from JSON
	timeout := invokeParams.GetInt("timeout")
	require.Equal(t, int64(60), timeout, "timeout should be 60")

	// Merge AITAG param
	commandValue := action.GetString("__aitag__command")
	require.NotEmpty(t, commandValue, "command should not be empty")
	require.Contains(t, commandValue, "#!/bin/bash", "command should contain shebang")
	require.Contains(t, commandValue, "for file in *.log", "command should contain for loop")
}

// TestExtractActionWithEmptyAITag tests behavior when AITAG is empty or missing
func TestExtractActionWithEmptyAITag(t *testing.T) {
	nonce := "test123"

	input := `{
		"@action": "call-tool",
		"tool": "test",
		"params": {
			"name": "value"
		}
	}`

	// Build action maker options with AITAG handler for a param that doesn't exist in input
	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAM_missing", "__aitag__missing"))

	ctx := context.Background()
	action, err := ExtractValidActionFromStream(ctx, strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err, "failed to extract action")

	// Get params from JSON
	invokeParams := aitool.InvokeParams{}
	for k, v := range action.GetInvokeParams("params") {
		invokeParams.Set(k, v)
	}

	// Verify JSON param exists
	require.Equal(t, "value", invokeParams.GetString("name"))

	// Verify AITAG param is empty (not present)
	missingValue := action.GetString("__aitag__missing")
	require.Empty(t, missingValue, "missing aitag param should be empty")
}

// TestExtractActionWithMultilineJSON tests handling of multiline content in JSON vs AITAG
func TestExtractActionWithMultilineJSON(t *testing.T) {
	nonce := "multi123"

	// This tests the advantage of AITAG - JSON requires escaping, AITAG doesn't
	input := `{
		"@action": "call-tool",
		"tool": "write",
		"params": {
			"escaped_content": "line1\\nline2\\nline3"
		}
	}

<|TOOL_PARAM_raw_content_multi123|>
line1
line2
line3
<|TOOL_PARAM_raw_content_END_multi123|>`

	var actionOpts []ActionMakerOption
	actionOpts = append(actionOpts, WithActionNonce(nonce))
	actionOpts = append(actionOpts, WithActionTagToKey("TOOL_PARAM_raw_content", "__aitag__raw_content"))

	ctx := context.Background()
	action, err := ExtractValidActionFromStream(ctx, strings.NewReader(input), "call-tool", actionOpts...)
	require.NoError(t, err)

	// Get JSON param (escaped)
	invokeParams := aitool.InvokeParams{}
	for k, v := range action.GetInvokeParams("params") {
		invokeParams.Set(k, v)
	}
	escapedContent := invokeParams.GetString("escaped_content")

	// Get AITAG param (raw multiline)
	rawContent := action.GetString("__aitag__raw_content")

	// Verify both contain the same logical content but in different formats
	require.Equal(t, "line1\\nline2\\nline3", escapedContent, "JSON content should be escaped")
	require.Equal(t, "line1\nline2\nline3", rawContent, "AITAG content should be raw multiline")
}

// TestToolParamsPromptMeta tests the ToolParamsPromptMeta structure
func TestToolParamsPromptMeta(t *testing.T) {
	meta := &ToolParamsPromptMeta{
		Prompt:     "test prompt",
		Nonce:      "abc123",
		ParamNames: []string{"param1", "param2", "param3"},
	}

	require.Equal(t, "test prompt", meta.Prompt)
	require.Equal(t, "abc123", meta.Nonce)
	require.Len(t, meta.ParamNames, 3)
	require.Contains(t, meta.ParamNames, "param1")
	require.Contains(t, meta.ParamNames, "param2")
	require.Contains(t, meta.ParamNames, "param3")
}
