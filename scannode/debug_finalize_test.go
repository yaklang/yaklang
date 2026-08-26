package scannode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDebugFinalizeContextSurvivesParentCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := debugFinalizeContext(parent)
	defer cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("finalize context should not be cancelled with parent: %v", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected finalize context deadline")
	}
	if remaining := time.Until(deadline); remaining < 30*time.Second {
		t.Fatalf("finalize timeout too short: %s", remaining)
	}
}

func TestDebugStatusForScriptError(t *testing.T) {
	t.Parallel()

	manager := newTaskManager()
	node := &ScanNode{manager: manager}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := newScriptTask(ctx, cancel, "job", "job", "sub", "attempt-1")
	task.SetCancelReason("platform cancel requested")
	manager.Add(task.TaskId, task)

	if got := debugStatusForScriptError(node, "attempt-1", errors.New("boom")); got != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got)
	}
	if got := debugStatusForScriptError(node, "missing", context.Canceled); got != "cancelled" {
		t.Fatalf("status = %q, want cancelled for context.Canceled", got)
	}
	if got := debugStatusForScriptError(node, "missing", errors.New("script failed")); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
}

func TestPublishDebugAnalysisWritesLocalCacheWithoutUploadConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cpu-pprof"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte("scan running\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	node := &ScanNode{}
	reporter := &ScannerAgentReporter{
		TaskId:    "job-1",
		RuntimeId: "attempt-1",
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	node.publishDebugAnalysis(cancelled, reporter, dir, "cancelled")

	cached, ok := readCachedDebugAnalysis(dir)
	if !ok {
		t.Fatal("expected local analysis.cache.json after finalize without upload config")
	}
	if len(cached) == 0 {
		t.Fatal("expected non-empty analysis cache")
	}
}
