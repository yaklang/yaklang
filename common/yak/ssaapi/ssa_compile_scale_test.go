package ssaapi

import (
	"strings"
	"testing"
)

func TestEmitCompileScale(t *testing.T) {
	t.Parallel()

	var gotProcess float64
	var gotMsg string
	emitCompileScale(func(process float64, msg string, _ ...any) {
		gotProcess = process
		gotMsg = msg
	}, 0.0, &ScanResult{
		HandlerTotal:    12,
		PreHandlerTotal: 15,
		HandlerBytes:    4096,
	})

	if gotProcess != 0 {
		t.Fatalf("process = %v, want 0", gotProcess)
	}
	if !strings.HasPrefix(gotMsg, "ssa-compile-scale:") {
		t.Fatalf("message prefix missing: %q", gotMsg)
	}
	for _, want := range []string{
		`"total_files":15`,
		`"handler_files":12`,
		`"prehandler_files":15`,
		`"total_bytes":4096`,
	} {
		if !strings.Contains(gotMsg, want) {
			t.Fatalf("message %q missing %s", gotMsg, want)
		}
	}
}

func TestEmitCompileScaleNilSafe(t *testing.T) {
	t.Parallel()
	emitCompileScale(nil, 0, &ScanResult{HandlerTotal: 1})
	emitCompileScale(func(float64, string, ...any) {
		t.Fatal("should not call callback for nil scan result")
	}, 0, nil)
}

func TestDeferredBuildProcessFractionSpreadsAcrossBatches(t *testing.T) {
	t.Parallel()

	// Completing the first of 10 batches must stay near 0.448, not jump to ~0.88.
	firstBatchDone := deferredBuildProcessFraction(0, 10, 10, 10)
	if firstBatchDone < 0.44 || firstBatchDone > 0.45 {
		t.Fatalf("first batch complete = %v, want ~0.448", firstBatchDone)
	}

	midBatchStart := deferredBuildProcessFraction(4, 10, 0, 10)
	if midBatchStart < 0.59 || midBatchStart > 0.60 {
		t.Fatalf("mid batch start = %v, want ~0.592", midBatchStart)
	}

	lastBatchDone := deferredBuildProcessFraction(9, 10, 10, 10)
	if lastBatchDone < 0.87 || lastBatchDone > 0.88 {
		t.Fatalf("last batch complete = %v, want ~0.88", lastBatchDone)
	}

	// Legacy single-batch path still fills the full deferred band.
	legacyDone := deferredBuildProcessFraction(0, 1, 10, 10)
	if legacyDone < 0.87 || legacyDone > 0.88 {
		t.Fatalf("legacy single batch = %v, want ~0.88", legacyDone)
	}
}
