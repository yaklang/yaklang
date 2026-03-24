package aicommon

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryFlushBuffer_FlushOnIterationLimit(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	differ := NewTimelineDiffer(timeline)
	differ.SetBaseline()
	buffer := NewMemoryFlushBuffer("test", differ, &MemoryFlushBufferConfig{MaxPendingIterations: 3, MaxPendingBytes: 4096})
	task := NewStatefulTaskBase("task-1", "test-input", context.Background(), nil, true)

	timeline.PushText(1, "first diff")
	payload, err := buffer.Capture(MemoryFlushSignal{Iteration: 1, Task: task})
	if err != nil {
		t.Fatalf("capture iteration 1 failed: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected no flush on iteration 1")
	}

	timeline.PushText(2, "second diff")
	payload, err = buffer.Capture(MemoryFlushSignal{Iteration: 2, Task: task})
	if err != nil {
		t.Fatalf("capture iteration 2 failed: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected no flush on iteration 2")
	}

	timeline.PushText(3, "third diff")
	payload, err = buffer.Capture(MemoryFlushSignal{Iteration: 3, Task: task})
	if err != nil {
		t.Fatalf("capture iteration 3 failed: %v", err)
	}
	if payload == nil {
		t.Fatalf("expected flush on iteration limit")
	}
	if payload.FlushReason != "batch_iteration_limit" {
		t.Fatalf("unexpected flush reason: %s", payload.FlushReason)
	}
	if !strings.Contains(payload.ContextualInput, "first diff") || !strings.Contains(payload.ContextualInput, "third diff") {
		t.Fatalf("expected contextual input to contain aggregated diffs")
	}
}

func TestMemoryFlushBuffer_FlushPendingOnTaskDoneWithoutNewDiff(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	differ := NewTimelineDiffer(timeline)
	differ.SetBaseline()
	buffer := NewMemoryFlushBuffer("test", differ, &MemoryFlushBufferConfig{MaxPendingIterations: 3, MaxPendingBytes: 4096})
	task := NewStatefulTaskBase("task-1", "test-input", context.Background(), nil, true)

	timeline.PushText(1, "pending diff")
	payload, err := buffer.Capture(MemoryFlushSignal{Iteration: 1, Task: task})
	if err != nil {
		t.Fatalf("capture iteration 1 failed: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected no flush before completion")
	}

	payload, err = buffer.Capture(MemoryFlushSignal{Iteration: 2, Task: task, IsDone: true})
	if err != nil {
		t.Fatalf("capture completion failed: %v", err)
	}
	if payload == nil {
		t.Fatalf("expected flush on task completion")
	}
	if payload.FlushReason != "task_done" {
		t.Fatalf("unexpected flush reason: %s", payload.FlushReason)
	}
	if !strings.Contains(payload.ContextualInput, "pending diff") {
		t.Fatalf("expected contextual input to retain pending diff")
	}
}

func TestMemoryFlushBuffer_FlushOnEndIterationMilestone(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	differ := NewTimelineDiffer(timeline)
	differ.SetBaseline()
	buffer := NewMemoryFlushBuffer("test", differ, &MemoryFlushBufferConfig{MaxPendingIterations: 3, MaxPendingBytes: 4096})
	task := NewStatefulTaskBase("task-1", "test-input", context.Background(), nil, true)

	timeline.PushText(1, "milestone diff")
	payload, err := buffer.Capture(MemoryFlushSignal{Iteration: 1, Task: task, ShouldEndIteration: true, Reason: "milestone reached"})
	if err != nil {
		t.Fatalf("capture milestone failed: %v", err)
	}
	if payload == nil {
		t.Fatalf("expected flush on milestone")
	}
	if payload.FlushReason != "milestone_end_iteration" {
		t.Fatalf("unexpected flush reason: %s", payload.FlushReason)
	}
	if !strings.Contains(payload.ContextualInput, "milestone reached") {
		t.Fatalf("expected contextual input to include milestone reason")
	}
}
