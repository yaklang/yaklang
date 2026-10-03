package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testHost struct {
	plan      *Plan
	execute   func(context.Context, Attempt) (Result, error)
	approve   func(context.Context, *Plan) (*Plan, error)
	mu        sync.Mutex
	revisions []uint64
}

func (h *testHost) Prepare(_ context.Context, _ string, document string) (*Plan, error) {
	p := clone(h.plan)
	p.Document = document
	return p, nil
}
func (h *testHost) Approve(ctx context.Context, p *Plan) (*Plan, error) {
	if h.approve != nil {
		return h.approve(ctx, p)
	}
	return p, nil
}
func (h *testHost) Execute(ctx context.Context, a Attempt) (Result, error) {
	if h.execute != nil {
		return h.execute(ctx, a)
	}
	return Result{Summary: a.Task.Name + " verified", EvidenceIDs: []string{"e1"}}, nil
}
func (h *testHost) Changed(s Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.revisions = append(h.revisions, s.Revision)
}
func testPlan() *Plan {
	p, err := ParsePlan(`{"task_id":"root","name":"test","goal":"verify","semantic_identifier":"root","subtasks":[{"task_id":"a","name":"A","goal":"verify A","semantic_identifier":"a"},{"task_id":"b","name":"B","goal":"verify B","semantic_identifier":"b","depends_on":["a"]}]}`, "document", nil)
	if err != nil {
		panic(err)
	}
	return p
}
func readyController(t *testing.T, h *testHost, concurrency int) *Controller {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	c := New(ctx, h, concurrency)
	t.Cleanup(c.Close)
	_, err := c.CreatePlan(ctx, "plan", "document")
	require.NoError(t, err)
	require.NoError(t, c.SubmitPlan(ctx))
	return c
}
func awaitResult(t *testing.T, c *Controller, id string) Attempt {
	t.Helper()
	require.Eventually(t, func() bool {
		a := c.Snapshot().Attempts[id]
		return a.State != Running && a.State != Cancelling && a.State != Pending
	}, time.Second, time.Millisecond)
	a, err := c.InspectTasks([]string{id})
	require.NoError(t, err)
	return a[0]
}

func TestCoordinatorLoopApprovalDependenciesReviewAndFinish(t *testing.T) {
	h := &testHost{plan: testPlan()}
	c := readyController(t, h, 2)
	_, err := c.StartTasks([]string{"b"})
	require.ErrorContains(t, err, "unaccepted dependencies")
	_, err = c.StartTasks([]string{"a", "a"})
	require.Error(t, err)
	require.Equal(t, Pending, c.Snapshot().Attempts["a"].State)
	started, err := c.StartTasks(nil)
	require.NoError(t, err)
	require.Len(t, started, 1)
	require.Eventually(t, func() bool { return c.Snapshot().Attempts["a"].State == AwaitingReview }, time.Second, time.Millisecond)
	a := c.Snapshot().Attempts["a"]
	require.Error(t, c.ReviewTask("a", a.ID+1, "accept", "stale attempt"))
	_, err = c.StartTasks([]string{"b"})
	require.Error(t, err)
	a = awaitResult(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "accept", "e1 verifies A"))
	require.Error(t, c.CanFinish())
	_, err = c.StartTasks([]string{"b"})
	require.NoError(t, err)
	b := awaitResult(t, c, "b")
	require.NoError(t, c.ReviewTask("b", b.ID, "accept", "e1 and task A verify B"))
	require.NoError(t, c.CanFinish())
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 1; i < len(h.revisions); i++ {
		require.Greater(t, h.revisions[i], h.revisions[i-1])
	}
}

func TestCoordinatorLoopApprovedSubmissionIsIdempotent(t *testing.T) {
	approvals := 0
	h := &testHost{plan: testPlan(), approve: func(_ context.Context, p *Plan) (*Plan, error) {
		approvals++
		return p, nil
	}}
	c := readyController(t, h, 1)
	_, err := c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "accept", "verified"))
	before := c.Snapshot()
	require.Error(t, c.SubmitPlan(context.Background()))
	require.Equal(t, 1, approvals)
	require.Equal(t, before, c.Snapshot(), "resubmission must preserve accepted results")

	_, err = c.ModifyPlan(context.Background(), map[string]any{"document": "changed"})
	require.NoError(t, err)
	require.Equal(t, PhaseExec, c.Snapshot().Phase)
	require.Equal(t, before.Attempts, c.Snapshot().Attempts, "document edits preserve accepted results")
	require.Equal(t, 1, approvals, "EXEC editing does not ask for plan approval")
}

func TestCoordinatorLoopRetryInvalidatesDependentsAndRejectsStaleAttempt(t *testing.T) {
	c := readyController(t, &testHost{plan: testPlan()}, 2)
	_, err := c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "accept", "verified"))
	_, err = c.StartTasks([]string{"b"})
	require.NoError(t, err)
	b := awaitResult(t, c, "b")
	require.NoError(t, c.ReviewTask("b", b.ID, "accept", "verified"))
	_, err = c.RetryTask("a", a.ID, "new user requirement")
	require.NoError(t, err)
	require.Equal(t, Pending, c.Snapshot().Attempts["b"].State)
	require.Error(t, c.ReviewTask("a", a.ID, "accept", "stale result"))
	next := awaitResult(t, c, "a")
	require.Greater(t, next.ID, a.ID)
	require.NoError(t, c.ReviewTask("a", next.ID, "accept", "new result checked"))
}

func TestCoordinatorLoopCancelWaitTimeoutAndWake(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := &testHost{plan: testPlan(), execute: func(ctx context.Context, _ Attempt) (Result, error) {
		close(started)
		<-ctx.Done()
		<-release
		return Result{}, ctx.Err()
	}}
	c := readyController(t, h, 1)
	_, err := c.StartTasks([]string{"a"})
	require.NoError(t, err)
	<-started
	w, err := c.WaitTasks(context.Background(), []string{"a"}, 5*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "timeout", w.Reason)
	require.Equal(t, Running, c.Snapshot().Attempts["a"].State)
	waited := make(chan WaitResult, 1)
	go func() { r, _ := c.WaitTasks(context.Background(), []string{"a"}, time.Second); waited <- r }()
	time.Sleep(10 * time.Millisecond)
	c.Wake()
	select {
	case r := <-waited:
		require.Equal(t, "changed", r.Reason)
	case <-time.After(time.Second):
		t.Fatal("user input did not wake wait")
	}
	require.NoError(t, c.CancelTasks([]string{"a"}, "user skip"))
	require.Equal(t, Cancelling, c.Snapshot().Attempts["a"].State)
	_, err = c.RetryTask("a", c.Snapshot().Attempts["a"].ID, "retry too soon")
	require.Error(t, err)
	close(release)
	a := awaitResult(t, c, "a")
	require.Equal(t, Cancelled, a.State)
	require.Error(t, c.CanFinish())
}

func TestCoordinatorLoopPersistenceInterruptedAttemptsAndPanic(t *testing.T) {
	h := &testHost{plan: testPlan(), execute: func(context.Context, Attempt) (Result, error) { panic("worker bug") }}
	c := readyController(t, h, 1)
	_, err := c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, c, "a")
	require.Equal(t, Failed, a.State)
	require.Contains(t, a.Result.Error, "worker panic")
	s := c.Snapshot()
	s.Attempts["a"] = Attempt{Task: s.Plan.Tasks[0], ID: 1, State: Running}
	s.NextAttempt = 1
	data, err := json.Marshal(s)
	require.NoError(t, err)
	var restored Snapshot
	require.NoError(t, json.Unmarshal(data, &restored))
	next := New(context.Background(), h, 1)
	defer next.Close()
	require.NoError(t, next.Restore(restored))
	require.Equal(t, Failed, next.Snapshot().Attempts["a"].State)
	restored.Schema = 7
	empty := New(context.Background(), h, 1)
	defer empty.Close()
	require.Error(t, empty.Restore(restored))
}

func TestCoordinatorLoopRejectInvalidDAGAndUnapprovedStart(t *testing.T) {
	for _, p := range []*Plan{{}, {Tasks: []Task{{ID: "a", Name: "A", Goal: "A", DependsOn: []string{"missing"}}}}, {Tasks: []Task{{ID: "a", Name: "A", Goal: "A", DependsOn: []string{"b"}}, {ID: "b", Name: "B", Goal: "B", DependsOn: []string{"a"}}}}} {
		t.Run(fmt.Sprint(p.Tasks), func(t *testing.T) {
			c := New(context.Background(), &testHost{plan: p}, 1)
			defer c.Close()
			_, err := c.CreatePlan(context.Background(), "x", "x")
			require.Error(t, err)
			_, err = c.StartTasks(nil)
			require.Error(t, err)
		})
	}
}

func TestCoordinatorLoopFinishMustObserveNewUserInput(t *testing.T) {
	c := readyController(t, &testHost{plan: testPlan()}, 1)
	for _, id := range []string{"a", "b"} {
		_, err := c.StartTasks([]string{id})
		require.NoError(t, err)
		a := awaitResult(t, c, id)
		require.NoError(t, c.ReviewTask(id, a.ID, "accept", "checked evidence"))
	}
	observed := c.Snapshot().UserRevision
	c.Wake()
	require.ErrorContains(t, c.Finalize(observed), "new user input")
	require.False(t, c.Snapshot().Finished)
	require.NoError(t, c.Finalize(c.Snapshot().UserRevision))
	require.True(t, c.Snapshot().Finished)
}

func TestCoordinatorLoopRejectedRetryDoesNotPartiallyInvalidateResults(t *testing.T) {
	plan := testPlan()
	var root PlanNode
	require.NoError(t, json.Unmarshal(plan.Tree, &root))
	root.Subtasks = append(root.Subtasks, &PlanNode{TaskID: "independent", Name: "C", Goal: "check independently", Identifier: "independent"})
	raw, _ := json.Marshal(root)
	plan, err := ParsePlan(string(raw), plan.Document, plan)
	require.NoError(t, err)
	release := make(chan struct{})
	h := &testHost{plan: plan, execute: func(_ context.Context, a Attempt) (Result, error) {
		if a.Task.ID == "independent" {
			<-release
		}
		return Result{Summary: "checked"}, nil
	}}
	c := readyController(t, h, 1)
	_, err = c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "accept", "checked"))
	_, err = c.StartTasks([]string{"independent"})
	require.NoError(t, err)
	_, err = c.RetryTask("a", a.ID, "try again")
	require.ErrorContains(t, err, "worker slot")
	require.Equal(t, Accepted, c.Snapshot().Attempts["a"].State)
	require.Equal(t, a.Result, c.Snapshot().Attempts["a"].Result)
	close(release)
	awaitResult(t, c, "independent")
}

func TestCoordinatorLoopRestoresDraftAndRejectsFalseCompletion(t *testing.T) {
	h := &testHost{plan: testPlan()}
	c := New(context.Background(), h, 1)
	defer c.Close()
	_, err := c.CreatePlan(context.Background(), "draft", "document")
	require.NoError(t, err)
	next := New(context.Background(), h, 1)
	defer next.Close()
	require.NoError(t, next.Restore(c.Snapshot()))
	require.Equal(t, PhasePlan, next.Snapshot().Phase)
	require.NoError(t, next.SubmitPlan(context.Background()))
	s := next.Snapshot()
	s.Finished = true
	bad := New(context.Background(), h, 1)
	defer bad.Close()
	require.ErrorContains(t, bad.Restore(s), "unfinished work")
}

func TestCoordinatorLoopAllWaitRequiresEverySelectedAttempt(t *testing.T) {
	plan := testPlan()
	var root PlanNode
	require.NoError(t, json.Unmarshal(plan.Tree, &root))
	root.Subtasks[1].DependsOn = nil
	raw, _ := json.Marshal(root)
	plan, err := ParsePlan(string(raw), plan.Document, plan)
	require.NoError(t, err)
	first, second := make(chan struct{}), make(chan struct{})
	h := &testHost{plan: plan, execute: func(_ context.Context, a Attempt) (Result, error) {
		if a.Task.ID == "a" {
			<-first
		} else {
			<-second
		}
		return Result{Summary: "done"}, nil
	}}
	c := readyController(t, h, 2)
	_, err = c.WaitTasksMode(context.Background(), nil, time.Second, "all")
	require.Error(t, err)
	_, err = c.StartTasks(nil)
	require.NoError(t, err)
	done := make(chan WaitResult, 1)
	go func() {
		result, _ := c.WaitTasksMode(context.Background(), []string{"a", "b"}, time.Second, "all")
		done <- result
	}()
	close(first)
	awaitResult(t, c, "a")
	select {
	case <-done:
		t.Fatal("all wait returned before the second worker exited")
	case <-time.After(10 * time.Millisecond):
	}
	close(second)
	select {
	case result := <-done:
		require.Equal(t, "all_settled", result.Reason)
		require.Len(t, result.Tasks, 2)
	case <-time.After(time.Second):
		t.Fatal("all wait failed to wake")
	}
}

func TestCoordinatorLoopDetachedSubmitDoesNotApproveExecution(t *testing.T) {
	h := &testHost{plan: testPlan(), approve: func(context.Context, *Plan) (*Plan, error) { return nil, ErrDetachedPlanPublished }}
	c := New(context.Background(), h, 1)
	defer c.Close()
	_, err := c.CreatePlan(context.Background(), "draft", "document")
	require.NoError(t, err)
	require.NoError(t, c.SubmitPlan(context.Background()))
	require.Equal(t, PhasePlan, c.Snapshot().Phase)
	_, err = c.StartTasks(nil)
	require.Error(t, err)
	require.Error(t, c.CanFinish())
	require.NoError(t, c.CanFinishPlanning())
	require.Contains(t, c.PromptStatus(), "等待审核：true")
}
