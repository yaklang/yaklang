package reactloops

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ActionToolName is the single tool name used in functioncall mode.
// Instead of registering one tool per LoopAction, we register a single
// "execute_action" tool whose parameters schema is the full action protocol
// JSON Schema (including the @action enum field). The model calls this tool
// with the complete action JSON as arguments, and we parse it using the
// existing ExtractAction pipeline.
const ActionToolName = "execute_action"

// ActionToolDescription is the description for the single execute_action tool.
const ActionToolDescription = "Call this tool to execute a ReAct loop action. " +
	"The arguments must be a JSON object matching the action protocol schema, " +
	"including the required @action field that selects the action type, plus " +
	"the action-specific fields. Use the @action enum to choose the action; " +
	"all other fields (identifier, human_readable_thought, todo_delta, and " +
	"action-specific options) follow the same schema as described in the system prompt."

// buildActionToolParameters builds the JSON Schema for the single execute_action
// tool. It reuses buildSchema (the same function used in text mode) so the
// model sees an identical schema whether in text mode or functioncall mode.
// The schema includes the @action enum field, common fields (identifier,
// human_readable_thought, todo_delta), and all action-specific options.
func buildActionToolParameters(actions []*LoopAction) (any, error) {
	schemaText := buildSchema(actions...)
	// Also apply tool-batch max-items constraints, same as text mode.
	maxBatchCalls := aicommon.DefaultToolBatchMaxCalls
	schemaText, err := applyToolBatchSchemaMaxItems(schemaText, maxBatchCalls)
	if err != nil {
		return nil, utils.Wrap(err, "apply tool batch max items")
	}
	var params any
	if err := json.Unmarshal([]byte(schemaText), &params); err != nil {
		return nil, utils.Wrapf(err, "failed to unmarshal action schema as tool parameters")
	}
	return params, nil
}

// buildActionTool creates the single execute_action aispec.Tool from the
// filtered actions list. The tool's parameters are the full action protocol
// JSON Schema (with @action enum), so the model's tool_call arguments are a
// complete action JSON that can be parsed by ExtractAction.
func buildActionTool(actions []*LoopAction) ([]aispec.Tool, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	params, err := buildActionToolParameters(actions)
	if err != nil {
		return nil, err
	}
	return []aispec.Tool{{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        ActionToolName,
			Description: ActionToolDescription,
			Parameters:  params,
		},
	}}, nil
}

// fmtToolCallSummary returns a short summary of a tool call for logging.
func fmtToolCallSummary(tc *aispec.ToolCall) string {
	if tc == nil {
		return "<nil>"
	}
	return fmt.Sprintf("name=%s args_len=%d", tc.Function.Name, len(tc.Function.Arguments))
}

// buildFunctionCallTools generates the single execute_action aispec.Tool for
// native functioncall mode based on the current iteration's filtered actions.
// It reuses the same filtering logic as generateSchemaString so both modes
// see the same action set.
func (r *ReActLoop) buildFunctionCallTools(operator *LoopActionHandlerOperator) []aispec.Tool {
	if r == nil || !r.functionCallMode {
		return nil
	}
	disallowExit := false
	if operator != nil {
		disallowExit = operator.disallowLoopExit
	}
	filteredActions := r.getFilteredActions(disallowExit, operator)
	if len(filteredActions) == 0 {
		return nil
	}
	tools, err := buildActionTool(filteredActions)
	if err != nil {
		log.Errorf("functioncall: failed to build action tool: %v", err)
		return nil
	}
	log.Infof("functioncall: built 1 execute_action tool from %d filtered actions", len(filteredActions))
	return tools
}
