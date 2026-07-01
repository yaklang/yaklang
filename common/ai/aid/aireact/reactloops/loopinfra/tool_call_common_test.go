package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils"
)

func TestHandleToolCallResult_MCPInitializing_ErrorPath(t *testing.T) {
	ctx := context.Background()
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.SetCurrentTask(task)
	op := reactloops.NewActionHandlerOperator(task)

	toolName := "mcp_test_srv_echo"
	err := utils.Errorf("%s remote server not ready", buildinaitools.MCPToolInitializingErrPrefix)
	handleToolCallResult(loop, ctx, invoker, toolName, nil, false, err, op)

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "still connecting")
	assert.Contains(t, feedback, "same tool")
	assert.True(t, op.IsContinued())
}

func TestHandleToolCallResult_MCPInitializing_ResultErrorPath(t *testing.T) {
	ctx := context.Background()
	invoker := newTestInvoker(ctx)
	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)
	// Leave loop task unset so verification gate does not Exit early.
	op := reactloops.NewActionHandlerOperator(task)

	toolName := "mcp_test_srv_echo"
	result := &aitool.ToolResult{
		Success: false,
		Error:   buildinaitools.MCPToolInitializingErrPrefix + " waiting for connection",
	}
	handleToolCallResult(loop, ctx, invoker, toolName, result, false, nil, op)

	feedback := op.GetFeedback().String()
	assert.Contains(t, feedback, "still initializing")
	assert.Contains(t, feedback, "same tool")
	assert.True(t, op.IsContinued())
}

func TestResolveToolCallReason(t *testing.T) {
	// reason field wins over human_readable_thought
	action, err := aicommon.ExtractAction(
		`{"@action":"require_tool","tool_call_reason":"scan port 443","human_readable_thought":"thinking..."}`,
		"require_tool",
	)
	require.NoError(t, err)
	assert.Equal(t, "scan port 443", resolveToolCallReason(action, "tool_call_reason"))

	// fallback to human_readable_thought when reason field is omitted
	action2, err := aicommon.ExtractAction(
		`{"@action":"require_tool","human_readable_thought":"read the file first"}`,
		"require_tool",
	)
	require.NoError(t, err)
	assert.Equal(t, "read the file first", resolveToolCallReason(action2, "tool_call_reason"))

	// empty when neither is present
	action3, err := aicommon.ExtractAction(
		`{"@action":"require_tool","tool_require_payload":"foo"}`,
		"require_tool",
	)
	require.NoError(t, err)
	assert.Empty(t, resolveToolCallReason(action3, "tool_call_reason"))

	// directly_call_reason key path
	action4, err := aicommon.ExtractAction(
		`{"@action":"directly_call_tool","directly_call_reason":"retry with cached params"}`,
		"directly_call_tool",
	)
	require.NoError(t, err)
	assert.Equal(t, "retry with cached params", resolveToolCallReason(action4, "directly_call_reason"))

	// nil action is safe
	assert.Empty(t, resolveToolCallReason(nil, "tool_call_reason"))
}
