package aicommon

import (
	"context"

	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
)

func TestClarificationSharedBudgetAndRolePolicy(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, c.AcquireClarification("normal", false))
	require.ErrorContains(t, c.AcquireClarification("normal", true), "limit")
	require.NoError(t, c.CheckClarificationAllowed("next-task", true))
	c.SetClarificationPolicy(func() (bool, bool, int64) { return true, true, 3 })
	for i := 0; i < 3; i++ {
		require.NoError(t, c.AcquireClarification("plan", i%2 == 0))
	}
	require.ErrorContains(t, c.AcquireClarification("plan", false), "limit")
	require.NoError(t, WithPerTaskUserInteractiveLimitedTimes(2)(c))
	_, limit := c.ClarificationUsage("other-plan")
	require.EqualValues(t, 2, limit)
	c.SetClarificationPolicy(func() (bool, bool, int64) { return false, true, 3 })
	require.ErrorContains(t, c.AcquireClarification("disabled-plan", false), "disabled")
	c.SetClarificationPolicy(nil)
	require.NoError(t, WithEnableGoalMode(true)(c))
	require.ErrorContains(t, c.AcquireClarification("goal", true), "blocking")
	require.NoError(t, c.AcquireClarification("goal", false))
}

func TestClarificationBudgetConcurrentAdmission(t *testing.T) {
	c := NewTestConfig(context.Background(), WithPerTaskUserInteractiveLimitedTimes(2))
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.AcquireClarification("same", false) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 2, accepted.Load())
}

func TestQuestionRuntimePolicyRejectsBeforeEmission(t *testing.T) {
	c := NewTestConfig(context.Background(), WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`), WithEnableGoalMode(true))
	_, err := c.RequestQuestions(c.GetContext(), "goal", uiQuestions(), true, 0)
	require.ErrorContains(t, err, "blocking")
	require.Empty(t, c.UISession().questions)
	_, err = c.RequestQuestions(c.GetContext(), "goal", uiQuestions(), false, 0)
	require.NoError(t, err)
	_, err = c.RequestQuestions(c.GetContext(), "goal", uiQuestions(), false, 0)
	require.ErrorContains(t, err, "limit")
	require.Len(t, c.UISession().questions, 1)
}

func TestGoalClarificationBoundarySurvivesRuntimeHandoff(t *testing.T) {
	parent := NewTestConfig(context.Background(), WithEnableGoalMode(true))
	child := NewTestConfig(context.Background(), ConvertConfigToOptionsWithoutHotPatch(parent)...)
	require.False(t, child.EnableGoalMode, "nested runtime must not inherit the goal completion gate")
	child.SetClarificationPolicy(func() (bool, bool, int64) { return true, true, 3 })
	require.ErrorContains(t, child.CheckClarificationAllowed("plan", true), "blocking")
	require.NoError(t, child.CheckClarificationAllowed("plan", false))
}

func TestClarificationPendingLimitRefundsUnemittedRequest(t *testing.T) {
	c := NewTestConfig(context.Background(), WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`))
	for _, task := range []string{"a", "b", "c"} {
		_, err := c.RequestQuestions(c.GetContext(), task, uiQuestions(), false, 0)
		require.NoError(t, err)
	}
	_, err := c.RequestQuestions(c.GetContext(), "overflow", uiQuestions(), false, 0)
	require.ErrorContains(t, err, "already pending")
	count, _ := c.ClarificationUsage("overflow")
	require.Zero(t, count)
}
