package loop_fast_context

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestFastContextPromptsOnlyPromiseAdvertisedSearchActions(t *testing.T) {
	require.Contains(t, persistentInstructionTpl, "leave report and candidate-file reading to the caller")
	require.Contains(t, persistentInstructionTpl, "Call submit_fast_context_result")
	require.NotContains(t, persistentInstructionTpl, "directly_call_tool")
	require.NotContains(t, persistentInstructionTpl, "`read_file`")
	require.NotContains(t, persistentInstructionTpl, "<final_answer>")
	require.NotContains(t, outputExample, "FunctionCallMode")
	rendered, err := utils.RenderTemplate(outputExample, map[string]any{})
	require.NoError(t, err)
	blocks := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")
	examples := blocks.FindAllStringSubmatch(rendered, -1)
	require.Len(t, examples, 2)
	for i, example := range examples {
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(example[1]), &args))
		require.Equal(t, []string{"grep_files_batch", "submit_fast_context_result"}[i], args["@action"])
		if i == 1 {
			locations, ok := args["locations"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, locations)
			for _, location := range locations {
				fields, ok := location.(map[string]any)
				require.True(t, ok)
				require.Contains(t, fields, "start_line")
				require.Contains(t, fields, "end_line")
			}
		}
	}
}
