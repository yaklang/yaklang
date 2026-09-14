//go:build linux

package scannode

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"net/http"
	"strings"
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
	opts = append(opts, aicommon.WithContext(ctx), aicommon.WithAgreeAuto(), aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		require.Contains(t, req.GetPrompt(), "tool-call-reason")
		response := cfg.NewAIResponse()
		response.EmitOutputStream(strings.NewReader(`{"@action":"tool-call-reason","reason":"Read scoped planning input"}`))
		response.Close()
		return response, nil
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
	factory, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_PLAN)
	require.True(t, ok)
	loop, err := factory(invoker)
	require.NoError(t, err)
	for _, name := range []string{"generate_direct_plan", "finish_exploration", "read_file"} {
		_, err := loop.GetActionHandler(name)
		require.NoError(t, err, name)
	}
	for _, name := range []string{"search_knowledge", "web_search", "scan_port", "loading_skills"} {
		_, err := loop.GetActionHandler(name)
		require.Error(t, err, name)
		require.NotContains(t, loop.GetAllActionNames(), name)
	}
	read, err := loop.GetActionHandler("read_file")
	require.NoError(t, err)
	require.Contains(t, read.Description, "bounded page")
	path := command.InputManifest.Resources[0].RelativePath
	action, err := aicommon.ExtractAction(fmt.Sprintf(`{"@action":"read_file","path":%q}`, path), "read_file")
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(nil)
	read.ActionHandler(loop, action, op)
	require.Contains(t, op.GetFeedback().String(), "completed")
	require.Contains(t, loop.Get("plan_file_results"), content)
	denied, err := aicommon.ExtractAction(`{"@action":"read_file","path":"/etc/passwd"}`, "read_file")
	require.NoError(t, err)
	beforeDenied := loop.Get("plan_file_results")
	op = reactloops.NewActionHandlerOperator(nil)
	read.ActionHandler(loop, denied, op)
	require.Contains(t, op.GetFeedback().String(), "failed")
	require.Equal(t, beforeDenied, loop.Get("plan_file_results"))
	for _, name := range []string{"request_plan", "request_plan_and_execution", "ask_for_clarification", "list_async_tasks", "dispatch_sub_react_agents"} {
		require.True(t, managedInputActionAllowed("default", name))
	}
}
