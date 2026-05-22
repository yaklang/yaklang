package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
