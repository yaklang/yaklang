package loopinfra

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func buildRequireToolAction(toolName string) *aicommon.Action {
	action, err := aicommon.ExtractAction(
		fmt.Sprintf(`{"@action":"require_tool","tool_require_payload":"%s"}`, toolName),
		schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	)
	if err != nil {
		panic(err)
	}
	return action
}

func TestDirectlyCallToolVerifier_WarnsButAllowsBash(t *testing.T) {
	ctx := context.Background()
	bashTool := mustNewTool(
		"bash",
		aitool.WithStringParam("command"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(bashTool)}
	cfg.GetAiToolManager().AddRecentlyUsedTool(bashTool)
	invoker := &directlyCallTestInvoker{testInvoker: newTestInvoker(ctx)}
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.Set("user_query", "修改这个脚本并重新执行")
	loop.Set(reactloops.LoopStateRequireEditBeforeExecution, "true")

	action := buildDirectlyCallAction(`{"@action":"directly_call_tool","directly_call_tool_name":"bash"}`)
	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	assert.Equal(t, "bash", loop.Get("directly_call_tool_name"))
	assert.Contains(t, invoker.getTimelineString(), "tool_routing_warning")
	assert.Contains(t, invoker.getTimelineString(), "如果你继续使用 bash 也可以")
}

func TestRequireToolVerifier_WarnsButAllowsBash(t *testing.T) {
	ctx := context.Background()
	bashTool := mustNewTool(
		"bash",
		aitool.WithStringParam("command"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(bashTool)}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.Set("user_query", "修改这个脚本并重新执行")
	loop.Set(reactloops.LoopStateRequireEditBeforeExecution, "true")

	action := buildRequireToolAction("bash")
	require.NoError(t, loopAction_toolRequireAndCall.ActionVerifier(loop, action))
	assert.Equal(t, "bash", loop.Get("tool_require_payload"))
	assert.Contains(t, invoker.getTimelineString(), "tool_routing_warning")
	assert.Contains(t, invoker.getTimelineString(), "modify_file")
}
