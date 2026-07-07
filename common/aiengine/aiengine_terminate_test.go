package aiengine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// newEngineForTerminalTest builds a minimal AIEngine with its task state maps
// initialized and a live context, so the terminal-state mapping of
// WaitTaskFinishByTaskName can be driven without spinning up a ReAct loop.
func newEngineForTerminalTest(t *testing.T) *AIEngine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &AIEngine{
		ctx:           ctx,
		cancel:        cancel,
		activeTasks:   make(map[string]aicommon.AITaskState),
		taskEndpoints: make(map[string]*aicommon.Endpoint),
	}
}

// TestWaitTaskFinishByTaskNameFastPath verifies the fast-path terminal-state
// mapping: a task already in a terminal state returns immediately without
// blocking, with the state mapped to the correct return value.
func TestWaitTaskFinishByTaskNameFastPath(t *testing.T) {
	e := newEngineForTerminalTest(t)
	e.activeTasks["task-aborted"] = aicommon.AITaskState_Aborted

	cases := []struct {
		name    string
		taskID  string
		wantErr error // nil means expect a non-nil error (guards), only checked when set
	}{
		{"aborted returns ErrAITaskAborted", "task-aborted", ErrAITaskAborted},
		{"empty taskID errors", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.WaitTaskFinishByTaskName(tc.taskID)
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
