package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestPlanExplorationConfigCopyRetainsRoleBoundary(t *testing.T) {
	f := newActionFixture(t, false)
	require.False(t, f.cfg.EnableSubagentsInPlan)
	require.NoError(t, aicommon.WithEnableSubagentsInPlan(true)(f.cfg))
	f.cfg.EnableDispatchSubReactAgents = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	copy := aicommon.NewConfig(ctx, aicommon.ConvertConfigToOptions(f.cfg)...)
	require.True(t, copy.EnableSubagentsInPlan)
	require.False(t, copy.EnableDispatchSubReactAgents, "generic dispatch remains non-inheritable")
}

func TestPlanExplorationDisabledRejectsDirectDispatch(t *testing.T) {
	f := newActionFixture(t, false)
	require.NotContains(t, f.loop.GetAllActionNames(), "dispatch_sub_react_agents")
	_, err := f.loop.SubmitSubAgents(f.task, []reactloops.SubAgentJob{{Goal: "investigate"}}, reactloops.SubAgentOptions{}, "bypass")
	require.ErrorContains(t, err, "disabled")
	require.Nil(t, f.loop.GetSubAgentManager(), "rejected calls must not create background workers")
}

func TestPlanExplorationPolicyRejectsReviewAndSpecializedLoops(t *testing.T) {
	f := newActionFixture(t, false)
	f.cfg.EnableSubagentsInPlan = true
	loop, err := NewLoop(f.loop.GetInvoker(), WithController(f.c))
	require.NoError(t, err)
	loop.SetCurrentTask(f.task)
	require.Contains(t, loop.GetAllActionNames(), "dispatch_sub_react_agents")
	_, err = loop.SubmitSubAgents(f.task, []reactloops.SubAgentJob{{Goal: "investigate", LoopName: "coordinator"}}, reactloops.SubAgentOptions{}, "recursive")
	require.ErrorContains(t, err, "default investigation")
	f.c.mu.Lock()
	f.c.state.ReviewPending = true
	f.c.mu.Unlock()
	_, err = loop.SubmitSubAgents(f.task, []reactloops.SubAgentJob{{Goal: "investigate"}}, reactloops.SubAgentOptions{}, "locked")
	require.ErrorContains(t, err, "locked")
	require.Nil(t, loop.GetSubAgentManager())
}

func TestPlanInvestigatorCannotWriteExecuteRecurseOrEdit(t *testing.T) {
	parent := newActionFixture(t, false)
	child := newActionFixture(t, false)
	child.cfg.EnablePlanAndExec = true
	child.cfg.EnableDispatchSubReactAgents = true
	child.cfg.EnableSubagentsInPlan = true
	configureInvestigator(child.loop, parent.loop)
	require.False(t, child.cfg.EnablePlanAndExec)
	require.False(t, child.cfg.EnableDispatchSubReactAgents)
	require.False(t, child.cfg.EnableSubagentsInPlan)
	for _, name := range []string{"create_plan", "modify_plan", "submit_plan", "start_tasks", "dispatch_sub_react_agents", "coordinator", "plan"} {
		_, err := child.loop.GetActionHandler(name)
		require.Error(t, err, name)
	}
	for _, name := range []string{"write_file", "bash", "exec_yak", "http"} {
		allow, reason := reactloops.CheckToolInvokeGuard(child.loop, name, aitool.InvokeParams{})
		require.False(t, allow, name)
		require.NotEmpty(t, reason)
	}
	allow, _ := reactloops.CheckToolInvokeGuard(child.loop, "read_file", aitool.InvokeParams{"file": "source.txt"})
	require.True(t, allow)
}

func TestPlanActionHandlerRevalidatesWithoutVerifier(t *testing.T) {
	f := newActionFixture(t, false)
	_, err := f.c.CreatePlan(context.Background(), "plan", "document")
	require.NoError(t, err)
	before := f.c.Snapshot()
	h, err := f.loop.GetActionHandler("modify_plan")
	require.NoError(t, err)
	for _, args := range []map[string]any{{}, {"document": nil}, {"document": false}, {"document": "candidate", "document_patch": "patch"}, {"plan_version": 1}} {
		op := reactloops.NewActionHandlerOperator(f.task)
		h.ActionHandler(f.loop, actionForTest(t, "modify_plan", args), op)
		require.Contains(t, op.GetFeedback().String(), "rejected")
		require.Equal(t, before, f.c.Snapshot())
	}
}
