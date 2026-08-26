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
