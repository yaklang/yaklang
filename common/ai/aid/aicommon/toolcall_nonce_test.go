package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestExtractKnownToolParamAITagBlocks(t *testing.T) {
	raw := `{"@action":"call-tool","params":{}}
<|TOOL_PARAM_command_bad12|>
echo "hello"
<|TOOL_PARAM_command_END_bad12|>
<|TOOL_PARAM_unknown_bad12|>
ignored
<|TOOL_PARAM_unknown_END_bad12|>`

	blocks := extractKnownToolParamAITagBlocks(raw, []string{"command"})
	require.Len(t, blocks, 1)
	require.Equal(t, "command", blocks[0].ParamName)
	require.Equal(t, "bad12", blocks[0].Nonce)
	require.Equal(t, `echo "hello"`, blocks[0].Content)
}

func TestRecoverSingleMismatchedAITagParam(t *testing.T) {
	invokeParams := aitool.InvokeParams{}
	raw := `{"@action":"call-tool","params":{}}
<|TOOL_PARAM_command_bad12|>
#!/bin/bash
echo "hello"
<|TOOL_PARAM_command_END_bad12|>`

	recovered, reason := recoverSingleMismatchedAITagParam(invokeParams, raw, "good99", []string{"command"}, map[string]struct{}{})
	require.Empty(t, reason)
	require.NotNil(t, recovered)
	require.Equal(t, "command", recovered.ParamName)
	require.Equal(t, "bad12", recovered.Nonce)
	require.Equal(t, "#!/bin/bash\necho \"hello\"", invokeParams.GetString("command"))
}

func TestRecoverSingleMismatchedAITagParamRejectsMultipleBlocks(t *testing.T) {
	invokeParams := aitool.InvokeParams{}
	raw := `{"@action":"call-tool","params":{}}
<|TOOL_PARAM_command_bad12|>
echo one
<|TOOL_PARAM_command_END_bad12|>
<|TOOL_PARAM_script_bad34|>
echo two
<|TOOL_PARAM_script_END_bad34|>`

	recovered, reason := recoverSingleMismatchedAITagParam(invokeParams, raw, "good99", []string{"command", "script"}, map[string]struct{}{})
	require.Nil(t, recovered)
	require.Equal(t, "found 2 mismatched aitag blocks", reason)
	require.Empty(t, invokeParams.GetString("command"))
	require.Empty(t, invokeParams.GetString("script"))
}

func TestRecoverSingleMismatchedAITagParamRejectsWhenExactMergedExists(t *testing.T) {
	invokeParams := aitool.InvokeParams{}
	raw := `{"@action":"call-tool","params":{}}
<|TOOL_PARAM_command_bad12|>
echo one
<|TOOL_PARAM_command_END_bad12|>`

	recovered, reason := recoverSingleMismatchedAITagParam(invokeParams, raw, "good99", []string{"command"}, map[string]struct{}{"command": {}})
	require.Nil(t, recovered)
	require.Equal(t, "exact nonce aitag already merged", reason)
}
