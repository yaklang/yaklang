package scannode

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTaskManagerActiveAttemptHeartbeats(t *testing.T) {
	t.Parallel()

	manager := newTaskManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := newScriptTask(
		ctx,
		cancel,
		taskIDForSubtask("subtask-1"),
		"job-1",
		"subtask-1",
		"attempt-1",
	)
	manager.Add(task.TaskId, task)

	now := time.Date(2026, time.April, 1, 10, 0, 0, 0, time.UTC)
	task.UpdateProgressAt(3000, 10000, now)
	beats := manager.ActiveAttemptHeartbeats(now)
	if len(beats) != 1 {
		t.Fatalf("unexpected active attempt count: %d", len(beats))
	}
	if beats[0].AttemptID != "attempt-1" {
		t.Fatalf("unexpected attempt id: %s", beats[0].AttemptID)
	}
	if beats[0].JobID != "job-1" || beats[0].SubtaskID != "subtask-1" {
		t.Fatalf("unexpected heartbeat payload: %+v", beats[0])
	}
	if beats[0].Status != "running" {
		t.Fatalf("unexpected attempt status: %s", beats[0].Status)
	}
	if beats[0].CompletedUnits != 3000 || beats[0].TotalUnits != 10000 {
		t.Fatalf("unexpected progress snapshot: %+v", beats[0])
	}
	if !beats[0].LastActivityAt.Equal(now) {
		t.Fatalf("unexpected last_activity_at: %v", beats[0].LastActivityAt)
	}

	manager.MarkCancelRequested(task.TaskId)
	beats = manager.ActiveAttemptHeartbeats(now.Add(time.Second))
	if beats[0].Status != "cancel_requested" {
		t.Fatalf("unexpected cancel_requested status: %s", beats[0].Status)
	}
}

func TestTaskManagerLoadOrStoreAttemptIsAtomicAndAttemptAware(t *testing.T) {
	manager := newTaskManager()
	const workers = 16
	var wg sync.WaitGroup
	accepted := make(chan *Task, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			task := newScriptTask(ctx, cancel, "task", "job", "subtask", "attempt")
			actual, loaded, ok := manager.LoadOrStoreAttempt(task)
			if !ok || actual == nil {
				t.Errorf("LoadOrStoreAttempt rejected active manager")
				return
			}
			if !loaded {
				accepted <- task
			} else {
				cancel()
			}
		}()
	}
	wg.Wait()
	close(accepted)
	winners := 0
	var winner *Task
	for task := range accepted {
		winners++
		winner = task
	}
	if winners != 1 {
		t.Fatalf("stored attempts = %d, want 1", winners)
	}
	if manager.Count() != 1 {
		t.Fatalf("manager count = %d, want 1", manager.Count())
	}
	beats := manager.ActiveAttemptHeartbeats(time.Now().UTC())
	if len(beats) != 1 || beats[0].Status != "claimed" {
		t.Fatalf("claimed heartbeat = %+v", beats)
	}
	winner.Cancel()
	manager.RemoveAttempt(winner.AttemptID)
}

func TestTaskManagerAttemptLookupSurvivesSameSubtaskRetry(t *testing.T) {
	manager := newTaskManager()
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	first := newScriptTask(firstCtx, firstCancel, "task-shared", "job", "subtask", "attempt-first")
	second := newScriptTask(secondCtx, secondCancel, "task-shared", "job", "subtask", "attempt-second")
	if _, loaded, accepted := manager.LoadOrStoreAttempt(first); !accepted || loaded {
		t.Fatal("first attempt was not registered")
	}
	if _, loaded, accepted := manager.LoadOrStoreAttempt(second); !accepted || loaded {
		t.Fatal("second attempt was not registered")
	}
	if got, err := manager.GetTaskByAttemptID(first.AttemptID); err != nil || got != first {
		t.Fatalf("first attempt lookup got=%p err=%v", got, err)
	}
	if got, err := manager.GetTaskByAttemptID(second.AttemptID); err != nil || got != second {
		t.Fatalf("second attempt lookup got=%p err=%v", got, err)
	}
	manager.RemoveAttempt(first.AttemptID)
	if got, err := manager.GetTaskById("task-shared"); err != nil || got != second {
		t.Fatalf("legacy subtask lookup lost current attempt: got=%p err=%v", got, err)
	}
	manager.RemoveAttempt(second.AttemptID)
}

func TestTaskManagerShutdownRejectsNewTasksAndDrains(t *testing.T) {
	manager := newTaskManager()
	ctx, cancel := context.WithCancel(context.Background())
	task := newScriptTask(ctx, cancel, "task-active", "job", "subtask", "attempt")
	if !manager.Add(task.TaskId, task) {
		t.Fatal("expected active task to be accepted")
	}

	manager.BeginShutdown()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel active task")
	}

	rejectedCtx, rejectedCancel := context.WithCancel(context.Background())
	defer rejectedCancel()
	rejected := newScriptTask(rejectedCtx, rejectedCancel, "task-rejected", "job", "subtask", "attempt")
	if manager.Add(rejected.TaskId, rejected) {
		t.Fatal("expected task added after shutdown to be rejected")
	}

	manager.Remove(task.TaskId)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := manager.WaitForEmpty(waitCtx); err != nil {
		t.Fatalf("wait for empty task manager: %v", err)
	}
}
