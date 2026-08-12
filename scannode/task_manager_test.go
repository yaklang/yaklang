package scannode

import (
	"context"
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
