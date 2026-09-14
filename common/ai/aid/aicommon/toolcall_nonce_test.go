package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestFixedToolParamNonceRecovery(t *testing.T) {
	for _, test := range []struct {
		name   string
		params aitool.InvokeParams
		raw    string
		want   aitool.InvokeParams
	}{
		{"single_mismatch", aitool.InvokeParams{}, `<|TOOL_PARAM_command_bad12|>
echo hello
<|TOOL_PARAM_command_END_bad12|>`, aitool.InvokeParams{"command": "echo hello"}},
		{"multiple_mismatches", aitool.InvokeParams{}, `<|TOOL_PARAM_command_bad12|>
echo one
<|TOOL_PARAM_command_END_bad12|>
<|TOOL_PARAM_script_bad34|>
echo two
<|TOOL_PARAM_script_END_bad34|>`, aitool.InvokeParams{}},
		{"exact_blocks_prevent_mismatch_recovery", aitool.InvokeParams{}, `<|TOOL_PARAM_command_good99|>
echo current
<|TOOL_PARAM_command_END_good99|>
<|TOOL_PARAM_script_bad12|>
echo stale
<|TOOL_PARAM_script_END_bad12|>`, aitool.InvokeParams{"command": "echo current"}},
		{"nonempty_json_wins_over_mismatch", aitool.InvokeParams{"command": "new JSON proposal"}, `<|TOOL_PARAM_command_stale12|>
old checkpoint block
<|TOOL_PARAM_command_END_stale12|>`, aitool.InvokeParams{"command": "new JSON proposal"}},
		{"exact_tag_wins_over_json", aitool.InvokeParams{"command": "old JSON proposal"}, `<|TOOL_PARAM_command_good99|>
new block
<|TOOL_PARAM_command_END_good99|>`, aitool.InvokeParams{"command": "new block"}},
		{"nonce_with_underscores", aitool.InvokeParams{}, `<|TOOL_PARAM_command_bad_nonce_12|>
echo hello
<|TOOL_PARAM_command_END_bad_nonce_12|>`, aitool.InvokeParams{"command": "echo hello"}},
		{"parameter_name_with_underscores", aitool.InvokeParams{}, `<|TOOL_PARAM_command_line_bad_nonce_12|>
echo hello
<|TOOL_PARAM_command_line_END_bad_nonce_12|>`, aitool.InvokeParams{"command_line": "echo hello"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Prefix-related names must be disambiguated by the matching end tag.
			names := []string{"command", "command_bad", "command_line", "script"}
			_, blocks, err := decodeToolParamObject("{}\n"+test.raw, names)
			require.NoError(t, err)
			err = mergeFixedToolParamBlocks(test.params, blocks, &ToolParamsPromptMeta{Nonce: "good99", ParamNames: names})
			require.NoError(t, err)
			require.Equal(t, test.want, test.params)
		})
	}
	_, _, err := decodeToolParamObject(`{} <|TOOL_PARAM_command_line_nonce|>
text
<|TOOL_PARAM_command_END_line_nonce|>
<|TOOL_PARAM_command_line_END_nonce|>`, []string{"command", "command_line"})
	require.ErrorContains(t, err, "ambiguous parameter AITAG")
}
