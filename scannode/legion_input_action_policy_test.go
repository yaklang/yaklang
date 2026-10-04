//go:build linux

package scannode

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"net/http"
	"testing"
	"time"
)

func TestManagedInputPlanningUsesScopedTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	content := "scoped planning evidence"
	command := managedInputBindFixture(t, "planning_input", content)
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, content) }))
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	_, err := manager.Bind(ctx, command, nil, options)
	require.NoError(t, err)
	binding := driver.bindings[0]
	defer binding.InputWorkspace.Cleanup()
	runtime := binding.LegionResultRuntime.(*legionServerFocusRuntime)
	require.NoError(t, runtime.activateFocusTurn(command.ResultContext.FocusReleaseId, inputExecutionContract("input.read")))
	opts, err := managedInputTools(runtime)
	require.NoError(t, err)

	// The adapter must preserve explicit task choices in both directions.
	for _, enabled := range []bool{false, true} {
		cfg := &aicommon.Config{EnablePlanAndExec: enabled, EnableDetachedPlan: enabled}
		for _, opt := range opts {
			require.NoError(t, opt(cfg))
		}
		require.Equal(t, enabled, cfg.GetEnablePlanAndExec())
		require.Equal(t, enabled, cfg.GetEnableDetachedPlan())
	}
	opts = append(opts, aicommon.WithContext(ctx), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAgreeAuto(), aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, fmt.Errorf("scoped direct tool call unexpectedly requested AI: %s", req.GetCallerLabel())
	}), aicommon.WithDisableAutoSkills(true), aicommon.WithLegionResultRuntime(runtime))
	invoker, err := aireact.NewReAct(opts...)
	require.NoError(t, err)
	require.True(t, invoker.GetConfig().(*aicommon.Config).GetEnablePlanAndExec())

	defaultFactory, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_DEFAULT)
	require.True(t, ok)
	defaultLoop, err := defaultFactory(invoker)
	require.NoError(t, err)
	_, err = defaultLoop.GetActionHandler("request_plan_and_execution")
	require.NoError(t, err)
	for _, name := range []string{"load_capability", "require_ai_blueprint", "loading_skills", "query_mcp_tools"} {
		_, err := defaultLoop.GetActionHandler(name)
		require.Error(t, err, name)
	}
	factory, ok := reactloops.GetLoopFactory(coordinator.Name)
	require.True(t, ok)
	controller := coordinator.New(ctx, nil, 1)
	defer controller.Close()
	loop, err := factory(invoker, coordinator.WithController(controller))
	require.NoError(t, err)
	for _, name := range []string{"create_plan", "modify_plan", "submit_plan", "directly_call_tool"} {
		_, err := loop.GetActionHandler(name)
		require.NoError(t, err, name)
	}
	for _, name := range []string{"search_knowledge", "web_search", "scan_port", "loading_skills"} {
		_, err := loop.GetActionHandler(name)
		require.Error(t, err, name)
		require.NotContains(t, loop.GetAllActionNames(), name)
	}
	tool, err := invoker.GetConfig().GetAiToolManager().GetToolByName("read_file")
	require.NoError(t, err)
	require.Contains(t, tool.Description, "bounded page")
	read, err := loop.GetActionHandler("directly_call_tool")
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("scoped-planning", "Read authorized input", ctx, invoker.GetConfig().GetEmitter(), true)
	loop.SetCurrentTask(task)
	path := command.InputManifest.Resources[0].RelativePath
	action, err := aicommon.ExtractAction(fmt.Sprintf(`{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"path":%q},"directly_call_reason":"Read scoped planning input"}`, path), "directly_call_tool")
	require.NoError(t, err)
	require.NoError(t, read.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	read.ActionHandler(loop, action, op)
	require.Equal(t, 1, op.GetExecutedToolCallCount(), "the scoped read callback must settle")
	cfg := invoker.GetConfig().(*aicommon.Config)
	require.Contains(t, cfg.GetSessionEvidenceRendered(), content)
	denied, err := aicommon.ExtractAction(`{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"path":"/etc/passwd"},"directly_call_reason":"Verify path confinement"}`, "directly_call_tool")
	require.NoError(t, err)
	beforeDenied := cfg.GetSessionEvidenceRendered()
	require.NoError(t, read.ActionVerifier(loop, denied))
	op = reactloops.NewActionHandlerOperator(task)
	read.ActionHandler(loop, denied, op)
	require.Contains(t, cfg.Timeline.Dump(), "input_path_denied")
	require.Equal(t, beforeDenied, cfg.GetSessionEvidenceRendered())
	for _, name := range []string{"request_plan", "request_plan_and_execution", "ask_for_clarification", "list_async_tasks", "dispatch_sub_react_agents"} {
		require.True(t, managedInputActionAllowed("default", name))
	}
}
