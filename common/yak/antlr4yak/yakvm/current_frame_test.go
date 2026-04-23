package yakvm

import (
	"context"
	"testing"
)

func TestExecPanicClearsCurrentFrame(t *testing.T) {
	vm := New()

	gotPanic := false
	func() {
		defer func() {
			if recover() != nil {
				gotPanic = true
			}
		}()

		err := vm.Exec(context.Background(), func(frame *Frame) {
			panic("boom")
		})
		if err != nil {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}()

	if !gotPanic {
		t.Fatal("expected panic from Exec")
	}
	if frame := vm.CurrentFM(); frame != nil {
		t.Fatalf("expected current frame to be cleared after panic, got %#v", frame)
	}
}
