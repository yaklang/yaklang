package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlanPhaseReviewLocksUnlocksAndFreezesFinalContent(t *testing.T) {
	c := planningFixture(t)
	h := c.host.(*testHost)
	reviews := make(chan *Plan, 2)
	decisions := make(chan bool, 2)
	h.approve = func(_ context.Context, p *Plan) (*Plan, error) {
		reviews <- p
		if !<-decisions {
			return nil, fmt.Errorf("user requested clearer boundaries")
		}
		p.Document = "用户最终编辑文档"
		return p, nil
	}
	done := make(chan error, 1)
	go func() { done <- c.SubmitPlan(context.Background()) }()
	<-reviews
	require.Equal(t, PhasePlan, c.Snapshot().Phase)
	require.True(t, c.Snapshot().ReviewPending)
	_, err := c.ModifyPlan(context.Background(), map[string]any{"document": "blocked"})
	require.ErrorContains(t, err, "locked")
	_, err = c.CreatePlan(context.Background(), nestedPresetPlan, "blocked")
	require.ErrorContains(t, err, "locked")
	require.Error(t, c.SubmitPlan(context.Background()))
	decisions <- false
	require.Error(t, <-done)
	require.False(t, c.Snapshot().ReviewPending)
	_, err = c.ModifyPlan(context.Background(), map[string]any{"document": "澄清后的文档"})
	require.NoError(t, err)
	go func() { done <- c.SubmitPlan(context.Background()) }()
	p := <-reviews
	require.Equal(t, "澄清后的文档", p.Document)
	decisions <- true
	require.NoError(t, <-done)
	require.Equal(t, PhaseExec, c.Snapshot().Phase)
	require.Equal(t, "用户最终编辑文档", c.Snapshot().Plan.Document)
	_, err = c.ModifyPlan(context.Background(), map[string]any{"document": "no return to PLAN"})
	require.NoError(t, err)
	require.Equal(t, PhaseExec, c.Snapshot().Phase)
	require.Error(t, c.SubmitPlan(context.Background()), "duplicate confirmation must not hand off again")
	for _, a := range c.Snapshot().Attempts {
		require.Zero(t, a.ID)
		require.Equal(t, Pending, a.State)
	}
}

func TestPlanPhaseAllExecutionEntrypointsRejectBeforeApproval(t *testing.T) {
	c := planningFixture(t)
	var starts atomic.Int64
	c.host.(*testHost).execute = func(context.Context, Attempt) (Result, error) {
		starts.Add(1)
		return Result{Summary: "unexpected"}, nil
	}
	_, err := c.StartTasks(nil)
	require.Error(t, err)
	_, err = c.WaitTasks(context.Background(), nil, time.Millisecond)
	require.Error(t, err)
	_, err = c.RetryTask("a", 1, "retry")
	require.Error(t, err)
	require.Error(t, c.ReviewTask("a", 1, "accept", "evidence"))
	require.Error(t, c.CancelTasks(nil, "cancel"))
	for _, name := range []string{"start_tasks", "wait_tasks", "review_task", "retry_task", "cancel_tasks", "write_report"} {
		require.False(t, c.actionAllowed(name))
	}
	require.Zero(t, starts.Load())
	require.Empty(t, c.Snapshot().Attempts)
}

type failingPlanCommitHost struct {
	*testHost
	fail bool
}

func (h *failingPlanCommitHost) CommitPlan(Snapshot) error {
	if h.fail {
		return fmt.Errorf("snapshot write failed")
	}
	return nil
}

func TestPlanPhasePersistenceFailureRollsBackEdit(t *testing.T) {
	c := planningFixture(t)
	h := &failingPlanCommitHost{testHost: c.host.(*testHost), fail: true}
	c.host = h
	before := c.Snapshot()
	_, err := c.ModifyPlan(context.Background(), map[string]any{"document": "never publish"})
	require.ErrorContains(t, err, "snapshot write failed")
	require.Equal(t, before, c.Snapshot())
	require.Error(t, c.SubmitPlan(context.Background()))
	require.Equal(t, before, c.Snapshot())
}

func TestPlanPhaseSnapshotBoundaryRemovesLegacyVersions(t *testing.T) {
	c := planningFixture(t)
	p := c.Snapshot().Plan
	for _, approved := range []bool{false, true} {
		old := map[string]any{"schema": 1, "draft_version": 1, "draft": p, "attempts": map[string]Attempt{}}
		if approved {
			old["approved_version"] = 1
			old["approved"] = p
			for _, task := range p.Tasks {
				old["attempts"].(map[string]Attempt)[task.ID] = Attempt{Task: task, State: Pending}
			}
		}
		data, err := json.Marshal(old)
		require.NoError(t, err)
		var s Snapshot
		require.NoError(t, json.Unmarshal(data, &s))
		require.Equal(t, 2, s.Schema)
		current, err := json.Marshal(s)
		require.NoError(t, err)
		require.NotContains(t, string(current), "version")
		require.NotContains(t, string(current), "draft")
		require.NotContains(t, string(current), "approved")
		restored := New(context.Background(), &testHost{plan: p}, 1)
		require.NoError(t, restored.Restore(s))
		restored.Close()
	}
	data, _ := json.Marshal(map[string]any{"schema": 1, "draft_version": 2, "approved_version": 1, "draft": p, "approved": p})
	var s Snapshot
	require.ErrorContains(t, json.Unmarshal(data, &s), "executing replacement draft")
}

func TestPlanPhaseConcurrentStrictEditsHaveOneWinner(t *testing.T) {
	c := planningFixture(t)
	before := c.Snapshot()
	patch := "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +1,2 @@\n # 检查计划\n-先核对来源。\n+核对实际路径。\n"
	start := make(chan struct{})
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := c.ModifyPlan(context.Background(), map[string]any{"document_patch": patch})
			done <- err
		}()
	}
	close(start)
	one, two := <-done, <-done
	require.NotEqual(t, one == nil, two == nil, "one patch applies; the other sees a strict mismatch")
	require.Equal(t, "# 检查计划\n核对实际路径。\n", c.Snapshot().Plan.Document)
	require.Equal(t, before.Plan.Tree, c.Snapshot().Plan.Tree)
	require.Equal(t, before.Revision+1, c.Snapshot().Revision)
}

func TestPlanPhaseRestoreRejectsIndependentlyEditedDAG(t *testing.T) {
	c := planningFixture(t)
	s := c.Snapshot()
	s.Plan.Tasks[0].Goal = "definition bypass"
	restored := New(context.Background(), c.host, 1)
	defer restored.Close()
	require.ErrorContains(t, restored.Restore(s), "does not match")
}
