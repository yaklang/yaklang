package followup

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/jsonextractor"
)

func TestFollowupExamplesSeparateSchemaLoadingAndExecution(t *testing.T) {
	require.NotContains(t, followupOutputExample, "FunctionCallMode")
	examples := jsonextractor.ExtractStandardJSON(followupOutputExample)
	require.Len(t, examples, 3)
	for i, example := range examples {
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(example), &args))
		require.Equal(t, []string{"require_tool", "directly_call_tool", "directly_answer"}[i], args["@action"])
		switch i {
		case 0:
			require.Equal(t, "read_file", args["require_tool_payload"])
			require.NotContains(t, args, "params")
			require.NotContains(t, args, "tool")
		case 1:
			require.Equal(t, "read_file", args["directly_call_tool_name"])
			require.Equal(t, map[string]any{"file": "/abs/path/to/file.go"}, args["directly_call_tool_params"])
		case 2:
			require.Equal(t, map[string]any{
				"@action": "directly_answer", "answer_payload": "...",
				"human_readable_thought": "基于审计报告与选区代码给出结论",
			}, args)
		}
	}
}
