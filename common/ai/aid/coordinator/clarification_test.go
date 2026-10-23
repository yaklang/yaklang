package coordinator

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"
)

func TestCoordinatorClarificationPhasePolicy(t *testing.T) {
	f := newActionFixture(t, false)
	require.NoError(t, aicommon.WithAllowPlanUserInteract(false)(f.cfg))
	require.Error(t, f.loop.CheckClarificationAllowed(false))
	require.NoError(t, aicommon.WithAllowPlanUserInteract(true)(f.cfg))
	require.NoError(t, aicommon.WithPlanUserInteractMaxCount(2)(f.cfg))
	require.NoError(t, f.loop.CheckClarificationAllowed(true))
	for i := 0; i < 2; i++ {
		require.NoError(t, f.cfg.AcquireClarification(f.task.GetId(), i == 0))
	}
	require.Error(t, f.loop.CheckClarificationAllowed(false))
	require.NotContains(t, f.loop.GetAllActionNames(), "ask_for_clarification")
	exec := newActionFixture(t, true)
	require.ErrorContains(t, exec.loop.CheckClarificationAllowed(true), "blocking")
	require.NoError(t, exec.loop.CheckClarificationAllowed(false))
}

func TestWorkerClarificationPolicySurvivesGlobalHotpatch(t *testing.T) {
	f := newActionFixture(t, true)
	worker, err := NewWorkerLoop(f.loop.GetInvoker())
	require.NoError(t, err)
	require.NoError(t, aicommon.WithAllowRequireForUserInteract(true)(f.cfg))
	require.Error(t, worker.CheckClarificationAllowed(false))
	require.ErrorContains(t, f.cfg.CheckClarificationAllowed("worker", true), "disabled")
}

func TestCoordinatorClarificationInstructionsFollowAvailability(t *testing.T) {
	for _, approved := range []bool{false, true} {
		f := newActionFixture(t, approved, aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`), aicommon.WithAllowPlanUserInteract(true))
		require.Contains(t, f.loop.GetPersistentInstruction(), "ask_for_clarification_async")
		require.NoError(t, aicommon.WithAllowRequireForUserInteract(false)(f.cfg))
		require.NotContains(t, f.loop.GetPersistentInstruction(), "ask_for_clarification")
		require.NoError(t, aicommon.WithAllowRequireForUserInteract(true)(f.cfg))
		if !approved {
			require.NoError(t, aicommon.WithAllowPlanUserInteract(false)(f.cfg))
			require.NotContains(t, f.loop.GetPersistentInstruction(), "ask_for_clarification")
		}
	}
}
