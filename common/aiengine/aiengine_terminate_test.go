package aiengine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// newEngineForTerminalTest builds a minimal AIEngine with its task state maps
// initialized and a live context, so the terminal-state mapping of
// WaitTaskFinishByTaskName can be driven without spinning up a ReAct loop.
func newEngineForTerminalTest(t *testing.T) *AIEngine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	epm := aicommon.NewEndpointManagerContext(ctx)
	return &AIEngine{
		ctx:              ctx,
		cancel:           cancel,
		activeTasks:      make(map[string]aicommon.AITaskState),
		allTasksEndpoint: epm.CreateEndpoint(),
		taskEndpoints:    make(map[string]*aicommon.Endpoint),
	}
}

func TestProcessOutputEventAcceptsMixedTaskMetadata(t *testing.T) {
	e := newEngineForTerminalTest(t)
	e.processOutputEvent(&schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "react_task_created",
		Content: []byte(`{
			"react_task_id":"task-with-flags",
			"react_task_status":"created",
			"react_user_input":"进行主机体检",
			"is_root_task":true,
			"iteration":1
		}`),
	})

	requireTaskState := func(want aicommon.AITaskState) {
		t.Helper()
		e.tasksMutex.Lock()
		defer e.tasksMutex.Unlock()
		if got := e.activeTasks["task-with-flags"]; got != want {
			t.Fatalf("unexpected task state: got %q want %q", got, want)
		}
	}
	requireTaskState(aicommon.AITaskState_Created)

	e.processOutputEvent(&schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "react_task_status_changed",
		Content: []byte(`{
			"react_task_id":"task-with-flags",
			"react_task_now_status":"processing",
			"react_task_old_status":"created",
			"success":true
		}`),
	})
	requireTaskState(aicommon.AITaskState_Processing)
}

// TestWaitTaskFinishByTaskNameFastPath verifies the fast-path terminal-state
// mapping: a task already in a terminal state returns immediately without
// blocking, with the state mapped to the correct return value.
func TestWaitTaskFinishByTaskNameFastPath(t *testing.T) {
	e := newEngineForTerminalTest(t)
	e.activeTasks["task-aborted"] = aicommon.AITaskState_Aborted
	e.activeTasks["task-skipped"] = aicommon.AITaskState_Skipped

	cases := []struct {
		name    string
		taskID  string
		wantErr error // nil means expect a non-nil error (guards), only checked when set
		wantNil bool
	}{
		{"aborted returns ErrAITaskAborted", "task-aborted", ErrAITaskAborted, false},
		{"skipped returns successfully", "task-skipped", nil, true},
		{"empty taskID errors", "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.WaitTaskFinishByTaskName(tc.taskID)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// TestProcessOutputEventSkippedReleasesWaiter covers the real Stop path:
// ReAct emits a skipped status change, AIEngine releases SendMsg, and skipped
// no longer counts as active work that can hold the next queued turn hostage.
func TestProcessOutputEventSkippedReleasesWaiter(t *testing.T) {
	e := newEngineForTerminalTest(t)
	taskID := "task-user-stopped"
	e.activeTasks[taskID] = aicommon.AITaskState_Processing

	epm := aicommon.NewEndpointManagerContext(e.ctx)
	e.taskEndpoints[taskID] = epm.CreateEndpoint()
	errCh := make(chan error, 1)
	go func() { errCh <- e.WaitTaskFinishByTaskName(taskID) }()
	time.Sleep(20 * time.Millisecond)

	e.processOutputEvent(&schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "react_task_status_changed",
		Content: []byte(`{
			"react_task_id":"task-user-stopped",
			"react_task_now_status":"skipped",
			"react_task_old_status":"processing"
		}`),
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("skipped Stop task returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("skipped Stop task did not release SendMsg waiter")
	}
	if e.hasActiveTasks() {
		t.Fatal("skipped Stop task still counted as active")
	}
}

func TestWaitTaskFinishDrainsQueuedTaskAfterCurrentTaskIsSkipped(t *testing.T) {
	e := newEngineForTerminalTest(t)
	e.activeTasks["task-current"] = aicommon.AITaskState_Processing
	e.activeTasks["task-queued"] = aicommon.AITaskState_Queueing

	done := make(chan error, 1)
	go func() { done <- e.WaitTaskFinish() }()
	time.Sleep(20 * time.Millisecond)

	e.processOutputEvent(&schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "react_task_status_changed",
		Content: []byte(`{
			"react_task_id":"task-current",
			"react_task_now_status":"skipped",
			"react_task_old_status":"processing"
		}`),
	})
	select {
	case err := <-done:
		t.Fatalf("queue drain returned while queued task was still active: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	e.processOutputEvent(&schema.AiOutputEvent{
		Type:   schema.EVENT_TYPE_STRUCTURED,
		NodeId: "react_task_status_changed",
		Content: []byte(`{
			"react_task_id":"task-queued",
			"react_task_now_status":"completed",
			"react_task_old_status":"processing"
		}`),
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("queue drain returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queue drain did not return after every task became terminal")
	}
}

func TestWaitTaskFinishUsesFreshEndpointForNextTaskGeneration(t *testing.T) {
	e := newEngineForTerminalTest(t)
	complete := func(taskID string) {
		t.Helper()
		e.processOutputEvent(&schema.AiOutputEvent{
			Type:   schema.EVENT_TYPE_STRUCTURED,
			NodeId: "react_task_status_changed",
			Content: []byte(`{
				"react_task_id":"` + taskID + `",
				"react_task_now_status":"completed",
				"react_task_old_status":"processing"
			}`),
		})
	}
	create := func(taskID string) {
		t.Helper()
		e.processOutputEvent(&schema.AiOutputEvent{
			Type:   schema.EVENT_TYPE_STRUCTURED,
			NodeId: "react_task_created",
			Content: []byte(`{
				"react_task_id":"` + taskID + `",
				"react_task_status":"processing"
			}`),
		})
	}
	waitAndAssertBlocked := func(taskID string) chan error {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- e.WaitTaskFinish() }()
		select {
		case err := <-done:
			t.Fatalf("task generation %s reused a released endpoint: %v", taskID, err)
		case <-time.After(50 * time.Millisecond):
		}
		return done
	}

	create("task-generation-1")
	done1 := waitAndAssertBlocked("task-generation-1")
	complete("task-generation-1")
	if err := <-done1; err != nil {
		t.Fatalf("first task generation: %v", err)
	}

	create("task-generation-2")
	done2 := waitAndAssertBlocked("task-generation-2")
	complete("task-generation-2")
	if err := <-done2; err != nil {
		t.Fatalf("second task generation: %v", err)
	}
}

// TestWaitTaskFinishByTaskNameAbortedReleasesEndpoint verifies that when the
// endpoint is created and the task is then flipped to Aborted (which calls
// Release()), WaitTaskFinishByTaskName unblocks and returns ErrAITaskAborted.
// This mirrors the real runtime: the waiter blocks, the status-changed event
// releases the endpoint, then we read the final state fresh.
func TestWaitTaskFinishByTaskNameAbortedReleasesEndpoint(t *testing.T) {
	e := newEngineForTerminalTest(t)
	taskID := "task-late-abort"
	e.activeTasks[taskID] = aicommon.AITaskState_Processing

	// Pre-create the endpoint so the waiter does not race with creation.
	epm := aicommon.NewEndpointManagerContext(e.ctx)
	endpoint := epm.CreateEndpoint()
	e.taskEndpoints[taskID] = endpoint

	errCh := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		errCh <- e.WaitTaskFinishByTaskName(taskID)
	}()
	<-ready
	time.Sleep(20 * time.Millisecond) // let the goroutine enter WaitContext

	// Flip to Aborted and release, mimicking the status-changed handler.
	e.tasksMutex.Lock()
	e.activeTasks[taskID] = aicommon.AITaskState_Aborted
	e.tasksMutex.Unlock()
	endpoint.Release()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrAITaskAborted) {
			t.Fatalf("expected ErrAITaskAborted after late abort, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not unblock after endpoint release")
	}
}

// TestWaitTaskFinishByTaskNameContextCancelledPrecedence verifies that a
// cancelled engine context is reported in preference to the task state, so
// callers can distinguish "engine torn down" from "task aborted on its own".
func TestWaitTaskFinishByTaskNameContextCancelledPrecedence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := &AIEngine{
		ctx:           ctx,
		cancel:        cancel,
		activeTasks:   map[string]aicommon.AITaskState{"task-x": aicommon.AITaskState_Processing},
		taskEndpoints: make(map[string]*aicommon.Endpoint),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- e.WaitTaskFinishByTaskName("task-x") }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not unblock after context cancel")
	}
}
