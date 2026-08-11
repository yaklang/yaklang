package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolBatchActionInferenceAndHistoryExtraction(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		actionName string
		aliases    []string
		wantType   string
		wantTools  []string
	}{
		{
			name: "top_level_direct_batch",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/a"}},
					{"tool_name":"grep","params":{"pattern":"auth"}}
				]
			}`,
			actionName: "object",
			aliases:    []string{schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL},
			wantType:   schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			wantTools:  []string{"read_file", "grep"},
		},
		{
			name: "legacy_wrapper_require_batch",
			payload: `{
				"@action":"object",
				"next_action":{
					"type":"require_tool",
					"tool_require_calls":[
						{"tool_name":"grep"},
						{"tool_name":"read_file"}
					]
				}
			}`,
			actionName: "object",
			wantType:   schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			wantTools:  []string{"grep", "read_file"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, err := aicommon.ExtractAction(test.payload, test.actionName, test.aliases...)
			require.NoError(t, err)
			require.Equal(t, test.wantType, inferActionTypeFromPayload(action, ""))
			require.Equal(t, test.wantTools, extractToolNamesFromAction(action))
			require.Equal(t, test.wantTools[0], extractToolNameFromAction(action), "legacy consumers keep the first tool")
		})
	}
}

func TestToolBatchHistoryCountsChildrenAndClonesParams(t *testing.T) {
	records := []*ActionRecord{
		{
			ActionType:    schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			ToolName:      "read_file",
			ToolNames:     []string{"read_file", "grep"},
			ToolCallCount: 2,
		},
		{
			ActionType:    schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			ToolName:      "search",
			ToolNames:     []string{"search", "search", "read_file"},
			ToolCallCount: 3,
		},
		// A legacy record has no ToolCallCount and remains one call.
		{ActionType: schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE, ToolName: "legacy"},
	}
	require.Equal(t, 6, countToolCallsFromActionRecords(records))
	require.Equal(
		t,
		"directly_call_tool(read_file,grep) -> require_tool(search,search,read_file) -> tool_compose(legacy)",
		summarizeValueFeedbackActions(records),
	)
	feedbackAction := valueFeedbackActionFromRecord(records[0])
	require.Equal(t, "read_file", feedbackAction.ToolName)
	require.Equal(t, []string{"read_file", "grep"}, feedbackAction.ToolNames)
	require.Equal(t, 2, feedbackAction.ToolCallCount)
	records[0].ToolNames[0] = "mutated_after_projection"
	require.Equal(t, []string{"read_file", "grep"}, feedbackAction.ToolNames,
		"value-feedback projection must own its ordered tool-name slice")

	original := map[string]any{
		"calls": []any{map[string]any{"tool_name": "read_file"}},
	}
	cloned := cloneActionParams(original)
	clonedCalls := cloned["calls"].([]any)
	clonedCalls[0].(map[string]any)["tool_name"] = "mutated"
	require.Equal(t, "read_file", original["calls"].([]any)[0].(map[string]any)["tool_name"])
}
