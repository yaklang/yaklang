package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type executionHost struct {
	plan    *Plan
	mu      sync.Mutex
	starts  map[string][]Attempt
	started chan Attempt
	gates   map[string]chan struct{}
	execute func(context.Context, Attempt) (Result, error)
}

func (h *executionHost) Prepare(_ context.Context, _ string, document string) (*Plan, error) {
	p := clone(h.plan)
	p.Document = document
	return p, nil
}
func (h *executionHost) Approve(_ context.Context, p *Plan) (*Plan, error) { return p, nil }
func (h *executionHost) Changed(Snapshot)                                  {}
func (h *executionHost) Execute(ctx context.Context, a Attempt) (Result, error) {
	h.mu.Lock()
	h.starts[a.Task.ID] = append(h.starts[a.Task.ID], a)
	h.mu.Unlock()
	h.started <- a
	if h.execute != nil {
		return h.execute(ctx, a)
	}
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-h.gates[a.Task.ID]:
		return Result{Summary: a.Task.Name + " verified", EvidenceIDs: []string{"evidence." + a.Task.ID}}, nil
	}
}

func executionFixture(t *testing.T, manual bool) (*Controller, *executionHost) {
	t.Helper()
	root := PlanNode{TaskID: "root", Name: "执行实验", Goal: "检查DAG", Identifier: "root", Subtasks: []*PlanNode{
		{TaskID: "a", Name: "A", Goal: "检查A", Identifier: "a"}, {TaskID: "b", Name: "B", Goal: "检查B", Identifier: "b"},
		{TaskID: "c", Name: "C", Goal: "检查C", Identifier: "c", DependsOn: []string{"a"}}, {TaskID: "d", Name: "D", Goal: "检查D", Identifier: "d", DependsOn: []string{"b", "c"}},
	}}
	raw, _ := json.Marshal(root)
	p, err := ParsePlan(string(raw), "# 执行计划\nA/B并发，C等待A，D等待B/C。", nil)
	require.NoError(t, err)
	h := &executionHost{plan: p, starts: map[string][]Attempt{}, started: make(chan Attempt, 32), gates: map[string]chan struct{}{}}
	for _, task := range p.Tasks {
		h.gates[task.ID] = make(chan struct{})
	}
	c := New(context.Background(), h, 2)
	c.patchDir = t.TempDir()
	t.Cleanup(c.Close)
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage())
	cfg.Timeline.SetTimelineBucketByteSize(-1)
	require.NoError(t, c.attachResultTimeline(cfg))
	c.EnableExecution(manual, 0)
	_, err = c.CreatePlan(context.Background(), string(raw), p.Document)
	require.NoError(t, err)
	return c, h
}

func nextStart(t *testing.T, h *executionHost) Attempt {
	t.Helper()
	select {
	case a := <-h.started:
		return a
	case <-time.After(2 * time.Second):
		t.Fatal("task did not automatically start")
		return Attempt{}
	}
}
func settled(t *testing.T, c *Controller, id string) Attempt {
	t.Helper()
	require.Eventually(t, func() bool {
		s := c.Snapshot().Attempts[id].State
		return s != Running && s != Cancelling && s != Pending
	}, 2*time.Second, time.Millisecond)
	return c.Snapshot().Attempts[id]
}

func TestExecutionAutomaticDAGApprovalAndReview(t *testing.T) {
	c, h := executionFixture(t, false)
	select {
	case <-h.started:
		t.Fatal("started before approval")
	default:
	}
	require.NoError(t, c.SubmitPlan(context.Background()))
	a, b := nextStart(t, h), nextStart(t, h)
	require.ElementsMatch(t, []string{"a", "b"}, []string{a.Task.ID, b.Task.ID})
	close(h.gates["a"])
	a = settled(t, c, "a")
	require.Equal(t, AwaitingReview, a.State)
	select {
	case <-h.started:
		t.Fatal("C started before A review")
	default:
	}
	require.NoError(t, c.ReviewTask("a", a.ID, "accept", "evidence.a confirms completion"))
	cc := nextStart(t, h)
	require.Equal(t, "c", cc.Task.ID)
	require.Equal(t, "a", cc.Predecessors[0].Task.ID)
	require.Equal(t, Accepted, cc.Predecessors[0].State)
	require.Error(t, c.ReviewTask("a", a.ID, "accept", "duplicate"))
	close(h.gates["b"])
	b = settled(t, c, "b")
	require.NoError(t, c.ReviewTask("b", b.ID, "accept", "B evidence checked"))
	close(h.gates["c"])
	cc = settled(t, c, "c")
	require.NoError(t, c.ReviewTask("c", cc.ID, "accept", "C checked"))
	d := nextStart(t, h)
	require.Equal(t, "d", d.Task.ID)
	close(h.gates["d"])
	d = settled(t, c, "d")
	require.NoError(t, c.ReviewTask("d", d.ID, "accept", "D checked"))
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.checkedMessages(through)
	require.True(t, c.ReportReady())
	require.False(t, c.Snapshot().Finished)
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, attempts := range h.starts {
		require.Len(t, attempts, 1, id)
	}
}

func TestExecutionManualAcceptanceHasNoModelGate(t *testing.T) {
	c, h := executionFixture(t, true)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	close(h.gates["a"])
	a := settled(t, c, "a")
	require.ErrorContains(t, c.ReviewTask("a", a.ID, "accept", "attempt to bypass human"), "manual")
	require.NoError(t, c.applyReview("a", a.ID, "accept", "user approved", true))
	require.Equal(t, "c", nextStart(t, h).Task.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _ = c.deliverMessages()
	c.checkedMessages(c.Snapshot().DeliveredThrough)
	require.ErrorIs(t, c.WaitMessages(ctx, 5*time.Millisecond, nil), context.DeadlineExceeded)
}

func TestExecutionLocalSplitPreservesUnrelatedRunningBranch(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	close(h.gates["a"])
	a := settled(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "deepen", "need two independent checks"))
	b := c.Snapshot().Attempts["b"]
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{map[string]any{"operator": "update", "task_id": "a", "changes": map[string]any{"subtasks": []any{
		map[string]any{"name": "A1", "goal": "深入A1", "identifier": "a1"}, map[string]any{"name": "A2", "goal": "深入A2", "identifier": "a2"},
	}}}}})
	require.NoError(t, err)
	first := nextStart(t, h)
	require.Contains(t, []string{"A1", "A2"}, first.Task.Name)
	require.Equal(t, b.ID, c.Snapshot().Attempts["b"].ID)
	require.Equal(t, Running, c.Snapshot().Attempts["b"].State)
	require.Len(t, c.Snapshot().History["a"], 1)
	require.Equal(t, Rejected, c.Snapshot().History["a"][0].State)
	require.Equal(t, PhaseExec, c.Snapshot().Phase)
	// One occupied unrelated branch plus one new verification respects limit=2.
	_, err = c.ModifyPlan(context.Background(), map[string]any{"document": "must roll back", "tasks_patch": []any{map[string]any{"operator": "update", "task_id": "c", "changes": map[string]any{"depends_on": []string{"missing"}}}}})
	require.Error(t, err)
	require.NotEqual(t, "must roll back", c.Snapshot().Plan.Document)
}

func TestExecutionSettlesFailurePanicAndRetries(t *testing.T) {
	for _, mode := range []string{"panic", "initialization", "no-result"} {
		t.Run(mode, func(t *testing.T) {
			c, h := executionFixture(t, false)
			h.execute = func(context.Context, Attempt) (Result, error) {
				switch mode {
				case "panic":
					panic("fixture crash")
				case "initialization":
					return Result{}, fmt.Errorf("runtime construction failed")
				default:
					return Result{}, nil
				}
			}
			require.NoError(t, c.SubmitPlan(context.Background()))
			nextStart(t, h)
			nextStart(t, h)
			a := settled(t, c, "a")
			require.Equal(t, Failed, a.State)
			require.NotEmpty(t, a.Result.Error)
			require.False(t, c.ReportReady())
			_, err := c.RetryTask("a", a.ID, "repair confirmed")
			require.NoError(t, err)
			newA := nextStart(t, h)
			require.Equal(t, "a", newA.Task.ID)
			require.Greater(t, newA.ID, a.ID)
			require.NotEmpty(t, c.Snapshot().History["a"])
		})
	}
}

func TestExecutionSplitGroupReleasesExternalDependenciesWithoutGroupReview(t *testing.T) {
	c, h := executionFixture(t, false)
	gates := map[string]chan struct{}{}
	for _, name := range []string{"A", "B", "A1", "A2", "C", "D"} {
		gates[name] = make(chan struct{})
	}
	h.execute = func(ctx context.Context, a Attempt) (Result, error) {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-gates[a.Task.Name]:
			return Result{Summary: a.Task.Name + " verified"}, nil
		}
	}
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	close(gates["A"])
	a := settled(t, c, "a")
	require.NoError(t, c.ReviewTask("a", a.ID, "deepen", "missing independent proof"))
	b := c.Snapshot().Attempts["b"]
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{map[string]any{"operator": "update", "task_id": "a", "changes": map[string]any{"subtasks": []any{
		map[string]any{"name": "A1", "goal": "复核A1", "identifier": "a1"}, map[string]any{"name": "A2", "goal": "复核A2", "identifier": "a2"},
	}}}}})
	require.NoError(t, err)
	first := nextStart(t, h)
	require.Len(t, first.PriorResults, 1, "new verification leaf receives preliminary result as background")
	close(gates[first.Task.Name])
	first = settled(t, c, first.Task.ID)
	second := nextStart(t, h)
	require.NotEqual(t, first.Task.ID, second.Task.ID)
	require.NoError(t, c.ReviewTask(first.Task.ID, first.ID, "accept", "verified leaf"))
	select {
	case early := <-h.started:
		t.Fatalf("C started before all group leaves accepted: %s", early.Task.Name)
	default:
	}
	close(gates[second.Task.Name])
	second = settled(t, c, second.Task.ID)
	require.NoError(t, c.ReviewTask(second.Task.ID, second.ID, "accept", "verified second leaf"))
	cc := nextStart(t, h)
	require.Equal(t, "C", cc.Task.Name)
	require.Len(t, cc.Predecessors, 2)
	require.Equal(t, b.ID, c.Snapshot().Attempts["b"].ID)
	close(gates["B"])
	b = settled(t, c, "b")
	require.NoError(t, c.ReviewTask("b", b.ID, "accept", "B verified"))
	close(gates["C"])
	cc = settled(t, c, "c")
	require.NoError(t, c.ReviewTask("c", cc.ID, "accept", "C verified"))
	d := nextStart(t, h)
	require.Equal(t, "D", d.Task.Name)
	close(gates["D"])
	d = settled(t, c, "d")
	require.NoError(t, c.ReviewTask("d", d.ID, "accept", "D verified"))
	require.Len(t, c.Snapshot().Attempts, 5, "group has no synthetic attempt or review")
	through, err := c.deliverMessages()
	require.NoError(t, err)
	c.checkedMessages(through)
	require.True(t, c.ReportReady())
}
