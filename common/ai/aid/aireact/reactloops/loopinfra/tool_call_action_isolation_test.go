package loopinfra

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type isolatedToolCallTestInvoker struct {
	*executingToolScalarTestInvoker
}

func (i *isolatedToolCallTestInvoker) ExecuteToolBatch(ctx context.Context, task aicommon.AIStatefulTask, request *aicommon.ToolBatchRequest) (*aicommon.ToolBatchResult, error) {
	result := &aicommon.ToolBatchResult{BatchID: request.BatchID}
	for _, call := range request.Calls {
		params := call.Params
		if call.Mode == aicommon.ToolCallModeRequire {
			params = i.generated[call.ToolName]
		}
		toolResult, _, err := i.invokeTool(ctx, call.ToolName, nil, params)
		if err != nil {
			return nil, err
		}
		result.Outcomes = append(result.Outcomes, aicommon.ToolCallOutcome{
			Index: call.Index, RequestedTool: call.ToolName, FinalTool: call.ToolName,
			Stage: aicommon.ToolCallStageDone, Result: toolResult,
		})
	}
	return result, nil
}

// Native transactions validate every action before executing any of them.
// Each handler must still use its own verified target and batch, even when
// several calls have the same action name or mix scalar and batch forms.
func TestToolCallsKeepTheirOwnVerifiedState(t *testing.T) {
	directRead := `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/first"}}`
	directGrep := `{"@action":"directly_call_tool","directly_call_tool_name":"grep","directly_call_tool_params":{"path":"/second","pattern":"needle"}}`
	directBatch := `{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"/batch-a"}},{"tool_name":"read_file","params":{"file":"/batch-b"}}]}`
	requireRead := `{"@action":"require_tool","tool_require_payload":"read_file"}`
	requireGrep := `{"@action":"require_tool","tool_require_payload":"grep"}`
	requireBatch := `{"@action":"require_tool","tool_require_calls":[{"tool_name":"grep"},{"tool_name":"read_file"}]}`
	composeRead := `{"@action":"tool_compose","tool_compose_payload":"[{\"call_id\":\"read_node\",\"tool_name\":\"read_file\",\"call_intent\":\"read file\"}]"}`
	composeGrep := `{"@action":"tool_compose","tool_compose_payload":"[{\"call_id\":\"grep_node\",\"tool_name\":\"grep\",\"call_intent\":\"search file\"}]"}`
	for _, tc := range []struct {
		name        string
		calls       []string
		wantTools   []string
		wantTargets []string
	}{
		{"direct distinct tools", []string{directRead, directGrep}, []string{"read_file", "grep"}, []string{"/first", "/second"}},
		{"require distinct tools", []string{requireRead, requireGrep}, []string{"read_file", "grep"}, []string{"/generated-file", "/generated-path"}},
		{"direct scalar then batch", []string{directGrep, directBatch}, []string{"grep", "read_file", "read_file"}, []string{"/second", "/batch-a", "/batch-b"}},
		{"direct batch then scalar", []string{directBatch, directGrep}, []string{"read_file", "read_file", "grep"}, []string{"/batch-a", "/batch-b", "/second"}},
		{"two direct batches", []string{directBatch, `{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"grep","params":{"path":"/batch-c","pattern":"third"}},{"tool_name":"read_file","params":{"file":"/batch-d"}}]}`}, []string{"read_file", "read_file", "grep", "read_file"}, []string{"/batch-a", "/batch-b", "/batch-c", "/batch-d"}},
		{"require scalar then batch", []string{requireRead, requireBatch}, []string{"read_file", "grep", "read_file"}, []string{"/generated-file", "/generated-path", "/generated-file"}},
		{"require batch then scalar", []string{requireBatch, requireGrep}, []string{"grep", "read_file", "grep"}, []string{"/generated-path", "/generated-file", "/generated-path"}},
		{"mixed actions", []string{directRead, requireBatch, directGrep, requireRead}, []string{"read_file", "grep", "read_file", "grep", "read_file"}, []string{"/first", "/generated-path", "/generated-file", "/second", "/generated-file"}},
		{"compose distinct DAGs", []string{composeRead, composeGrep}, []string{"read_file", "grep"}, []string{"/generated-file", "/generated-path"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			manager, readFile, grep := newToolBatchTestManager(t)
			manager.AddRecentlyUsedTool(readFile)
			manager.AddRecentlyUsedTool(grep)
			invoker := &isolatedToolCallTestInvoker{&executingToolScalarTestInvoker{
				testInvoker: newTestInvoker(ctx), manager: manager,
				generated: map[string]aitool.InvokeParams{
					"read_file": {"file": "/generated-file"},
					"grep":      {"path": "/generated-path", "pattern": "generated"},
				},
			}}
			task := newTestTask(ctx)
			invoker.currentTask = task
			loop := reactloops.NewMinimalReActLoop(&aicommon.Config{AiToolManager: manager}, invoker)
			loop.SetCurrentTask(task)
			var calls []reactloops.LoopCall
			for _, raw := range tc.calls {
				var params aitool.InvokeParams
				require.NoError(t, json.Unmarshal([]byte(raw), &params))
				name := params.GetString("@action")
				action := aicommon.NewSimpleAction(name, params)
				handler := loopAction_directlyCallTool
				if name == "require_tool" {
					handler = loopAction_toolRequireAndCall
				} else if name == "tool_compose" {
					handler = loopAction_toolCompose
				}
				require.NoError(t, handler.ActionVerifier(loop, action))
				calls = append(calls, reactloops.LoopCall{Action: action, LoopAction: handler})
			}
			for _, call := range calls {
				call.LoopAction.ActionHandler(loop, call.Action, reactloops.NewActionHandlerOperator(task))
			}
			require.Equal(t, tc.wantTools, invoker.executed)
			var targets []string
			for _, params := range invoker.received {
				target := params.GetString("file")
				if target == "" {
					target = params.GetString("path")
				}
				targets = append(targets, target)
			}
			require.Equal(t, tc.wantTargets, targets)
		})
	}
}
