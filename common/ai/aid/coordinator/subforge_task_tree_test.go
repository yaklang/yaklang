package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSubForgeTaskTreeLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 12)
	cfg := aicommon.NewConfig(ctx, aicommon.WithID("parent-forge"), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type == schema.EVENT_TYPE_SUB_FORGE_TASK_TREE {
				events <- event
			}
		}))
	parentPlan, err := ParsePlan(`{"name":"Parent","goal":"Deliver evidence","subtasks":[{"task_id":"parent-task","name":"Collect","goal":"Collect evidence","semantic_identifier":"collect"},{"task_id":"deliver","name":"Deliver","goal":"Deliver results","semantic_identifier":"deliver","depends_on":["parent-task"]}]}`, "Verify evidence", nil)
	require.NoError(t, err)
	session := &Session{Config: cfg, controller: New(ctx, nil, 1)}
	defer session.controller.Close()
	session.controller.state = session.controller.approvedSnapshotLocked(parentPlan)
	before := session.controller.Snapshot()
	require.Equal(t, []string{"parent-task"}, before.Plan.Tasks[1].DependsOn)
	binding := SubForgeTool{Schema: mcp.NewTool("sub_forge_inspect")}
	var runErr error
	binding.Run = func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
		child := aicommon.NewConfig(ctx, opts...)
		plan := map[string]any{"root_task": map[string]string{"name": "child", "progress": "processing"}}
		for range 2 {
			if _, err := child.GetEmitter().EmitJSON(schema.EVENT_TYPE_PLAN, "system", plan); err != nil {
				return nil, err
			}
		}
		return "child result", runErr
	}

	var previousID, previousAttempt string
	for _, tc := range []struct {
		call, status string
		err          error
	}{{"first", "succeeded", nil}, {"retry", "failed", errors.New("child execution failed")}} {
		task := aicommon.NewStatefulTaskBase("parent-task", "query", ctx, cfg.GetEmitter(), true)
		runErr = tc.err
		_, err := session.invokeSubForge(ctx, task, tc.call, binding, nil)
		require.ErrorIs(t, err, tc.err)
		start, update, end := readSubForgeTree(t, events), readSubForgeTree(t, events), readSubForgeTree(t, events)
		require.Equal(t, "running", start.Status)
		require.JSONEq(t, "null", string(start.TaskTree))
		require.Equal(t, uint64(1), start.Revision)
		require.Equal(t, "running", update.Status)
		require.Equal(t, uint64(2), update.Revision)
		require.JSONEq(t, `{"name":"child","progress":"processing"}`, string(update.TaskTree))
		require.Equal(t, update.TaskTree, end.TaskTree)
		require.Equal(t, tc.status, end.Status)
		require.Equal(t, uint64(3), end.Revision)
		for _, snapshot := range []subForgeTaskTree{start, update, end} {
			require.Equal(t, cfg.GetRuntimeId(), snapshot.ParentCoordinatorID)
			require.Equal(t, task.GetId(), snapshot.ParentTaskID)
			require.Equal(t, task.GetUUID(), snapshot.ParentTaskUUID)
			require.Equal(t, tc.call, snapshot.CallToolID)
			require.Equal(t, "sub_forge_inspect", snapshot.ToolName)
			require.Equal(t, start.ChildCoordinatorID, snapshot.ChildCoordinatorID)
		}
		require.NotEmpty(t, start.ChildCoordinatorID)
		require.NotEqual(t, previousID, start.ChildCoordinatorID)
		require.NotEqual(t, previousAttempt, start.ParentTaskUUID)
		previousID, previousAttempt = start.ChildCoordinatorID, start.ParentTaskUUID
		require.Empty(t, events, "identical plan updates are coalesced")
	}
	require.Equal(t, before, session.controller.Snapshot(), "display events must not change the parent plan or its attempts")
}

func TestResilienceSubForgeTaskTreeDeadlineWithBlockedHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 8)
	release := make(chan struct{})
	unblock := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	defer unblock()
	cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type == schema.EVENT_TYPE_SUB_FORGE_TASK_TREE {
				events <- event
				if event.GetContentJSONPath("$.revision") == "2" {
					<-release
				}
			}
		}))
	session := &Session{Config: cfg}
	task := aicommon.NewStatefulTaskBase("parent-task", "query", ctx, cfg.GetEmitter(), true)
	binding := SubForgeTool{Schema: mcp.NewTool("sub_forge_inspect"), Run: func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
		child := aicommon.NewConfig(ctx, opts...)
		_, err := child.GetEmitter().EmitJSON(schema.EVENT_TYPE_PLAN, "system",
			map[string]any{"root_task": map[string]string{"name": "child"}})
		return nil, err
	}}
	callCtx, stop := context.WithTimeout(ctx, 250*time.Millisecond)
	defer stop()
	returned := make(chan error, 1)
	go func() {
		_, err := session.invokeSubForge(callCtx, task, "deadline", binding, nil)
		returned <- err
	}()
	start, update := readSubForgeTree(t, events), readSubForgeTree(t, events)
	require.Equal(t, "running", start.Status)
	require.Equal(t, uint64(2), update.Revision)
	_, err := session.invokeSubForge(ctx, task, "rejected", binding, nil)
	require.ErrorIs(t, err, aicommon.ErrSubForgeBusy)
	select {
	case err = <-returned:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("child deadline waited for the event callback")
	}
	// The terminal is already durable at return, without waiting for its callback.
	var persisted []*schema.AiOutputEvent
	require.NoError(t, consts.GetGormProjectDatabase().Where("coordinator_id = ? AND type = ?", start.ParentCoordinatorID, schema.EVENT_TYPE_SUB_FORGE_TASK_TREE).Find(&persisted).Error)
	found := false
	for _, row := range persisted {
		if row.GetContentJSONPath("$.child_coordinator_id") == start.ChildCoordinatorID && row.GetContentJSONPath("$.status") == "timed_out" {
			found = true
		}
	}
	require.True(t, found, "terminal must be saved before invokeSubForge returns")
	// The terminal callback also remains independent of the blocked progress callback.
	end := readSubForgeTree(t, events)
	unblock()
	require.Equal(t, "timed_out", end.Status)
	require.Equal(t, start.ChildCoordinatorID, end.ChildCoordinatorID)
	require.Equal(t, update.TaskTree, end.TaskTree)
	require.Equal(t, uint64(3), end.Revision)
	require.Empty(t, events, "rejected calls must not have a task tree")
}

func readSubForgeTree(t *testing.T, events <-chan *schema.AiOutputEvent) subForgeTaskTree {
	t.Helper()
	select {
	case event := <-events:
		require.Empty(t, event.CallToolID)
		require.True(t, event.IsRecoveryBlock)
		require.NotZero(t, event.ID, "snapshot was not saved to the event store")
		var snapshot subForgeTaskTree
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.Equal(t, snapshot.ParentCoordinatorID, event.CoordinatorId)
		return snapshot
	case <-time.After(2 * time.Second):
		t.Fatal("missing sub-Forge task tree event")
		return subForgeTaskTree{}
	}
}

func TestResilienceSubForgeTaskTreeCancellationBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocked := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.Type == schema.EVENT_TYPE_SUB_FORGE_TASK_TREE &&
				event.GetContentJSONPath("$.call_tool_id") == "first" &&
				event.GetContentJSONPath("$.revision") == "1" {
				close(blocked)
				<-release
			}
		}))
	session := &Session{Config: cfg}
	task := aicommon.NewStatefulTaskBase("task", "query", ctx, cfg.GetEmitter(), true)
	started := make(chan struct{}, 2)
	binding := SubForgeTool{Schema: mcp.NewTool("sub_forge_inspect"), Run: func(context.Context, aitool.InvokeParams, []*aitool.Tool, ...aicommon.ConfigOption) (any, error) {
		started <- struct{}{}
		return "child result", nil
	}}
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	finished := make(chan error, 1)
	go func() {
		_, err := session.invokeSubForge(callCtx, task, "first", binding, nil)
		finished <- err
	}()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("initial snapshot did not reach the event handler")
	}
	cancelCall()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, aitool.ErrInvocationNotStarted)
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation waited for the event handler")
	}
	require.Empty(t, started)
	result, err := session.invokeSubForge(ctx, task, "retry", binding, nil)
	require.NoError(t, err)
	require.Equal(t, "child result", result)
	require.Len(t, started, 1)
}

func TestSubForgeTaskTreeTracksIndividualNodeStates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()), aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type == schema.EVENT_TYPE_SUB_FORGE_TASK_TREE {
			events <- event
		} else if event.NodeId == "react_task_status_changed" {
			t.Error("child retry leaked into parent event stream")
		}
	}))
	parent := &Session{Config: cfg}
	task := aicommon.NewStatefulTaskBase("parent", "inspect", ctx, cfg.GetEmitter(), true)
	plan, err := ParsePlan(`{"name":"Child","goal":"Inspect","subtasks":[{"task_id":"read","name":"Read","goal":"Read evidence"},{"task_id":"verify","name":"Verify","goal":"Verify evidence"}]}`, "Inspect", nil)
	require.NoError(t, err)
	original := string(plan.Tree)
	binding := SubForgeTool{Schema: mcp.NewTool("inspect"), Run: func(ctx context.Context, _ aitool.InvokeParams, _ []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
		child := &Session{Config: aicommon.NewConfig(ctx, opts...), parent: cfg, tasks: map[string]*aicommon.AIStatefulTaskBase{}, opened: map[string]bool{}, closed: map[string]bool{}}
		for i, state := range []State{Pending, Running, Accepted, Running, Failed, Cancelled, Running} {
			snapshot := Snapshot{Revision: uint64(i + 1), Phase: PhaseExec, Plan: plan, Attempts: map[string]Attempt{}}
			for index, node := range plan.Tasks {
				progress := Pending
				if index == 0 {
					progress = state
				}
				snapshot.Attempts[node.ID] = Attempt{ID: 1, Task: node, State: progress}
			}
			child.Changed(snapshot)
		}
		return "done", nil
	}}
	_, err = parent.invokeSubForge(ctx, task, "call", binding, nil)
	require.NoError(t, err)
	require.Equal(t, "running", readSubForgeTree(t, events).Status)
	for i, expected := range []string{"", "processing", "completed", "processing", "aborted", "skipped", "processing"} {
		snapshot := readSubForgeTree(t, events)
		require.Equal(t, uint64(i+2), snapshot.Revision)
		var tree PlanNode
		require.NoError(t, json.Unmarshal(snapshot.TaskTree, &tree))
		require.Equal(t, expected, tree.Subtasks[0].Progress)
		require.Empty(t, tree.Subtasks[1].Progress, "another node must retain its independent state")
	}
	require.Equal(t, "succeeded", readSubForgeTree(t, events).Status)
	require.Equal(t, original, string(plan.Tree), "display projection must not modify the executable plan")
}
