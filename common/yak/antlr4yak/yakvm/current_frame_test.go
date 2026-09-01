package yakvm

import (
	"context"
	"sync"
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

func TestCurrentGoroutineID(t *testing.T) {
	if id := currentGoroutineID(); id <= 0 {
		t.Fatalf("expected a positive goroutine ID, got %d", id)
	}
}

func TestConcurrentBareVMGetVarDoesNotRelinkGlobals(t *testing.T) {
	vm := New()
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 64; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			if _, ok := vm.GetVar("missing"); ok {
				t.Error("unexpected missing variable")
			}
		}()
	}
	close(start)
	wait.Wait()
}

func TestNestedExecReusesGoroutineIDAndClearsFrames(t *testing.T) {
	vm := New()

	err := vm.Exec(context.Background(), func(parent *Frame) {
		parentID := parent.ownerGoroutineID
		if parentID <= 0 {
			t.Fatalf("expected parent goroutine ID, got %d", parentID)
		}

		if err := vm.exec(context.Background(), parent, func(child *Frame) {
			if child.ownerGoroutineID != parentID {
				t.Fatalf("child goroutine ID %d does not match parent %d", child.ownerGoroutineID, parentID)
			}
			if current := vm.CurrentFM(); current != child {
				t.Fatalf("expected child to be current frame, got %#v", current)
			}
		}, Sub); err != nil {
			t.Fatalf("nested exec failed: %v", err)
		}

		if current := vm.CurrentFM(); current != parent {
			t.Fatalf("expected parent frame after nested exec, got %#v", current)
		}
	})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if frame := vm.CurrentFM(); frame != nil {
		t.Fatalf("expected all frames to be cleared, got %#v", frame)
	}
	if count := vm.activeFrames.Load(); count != 0 {
		t.Fatalf("expected zero active frames, got %d", count)
	}
}

func TestAsyncSubFrameUsesItsOwnGoroutineAndClearsFrames(t *testing.T) {
	type asyncResult struct {
		ownerID       int64
		currentIsSelf bool
		poppedIsSelf  bool
	}

	vm := New()
	var parentID int64
	err := vm.Exec(context.Background(), func(parent *Frame) {
		parentID = parent.ownerGoroutineID
		resultCh := make(chan asyncResult, 1)

		go func() {
			child := NewSubFrame(parent)
			vm.pushCurrentFrame(child)
			resultCh <- asyncResult{
				ownerID:       child.ownerGoroutineID,
				currentIsSelf: vm.CurrentFM() == child,
				poppedIsSelf:  vm.popCurrentFrame(child) == child,
			}
		}()

		result := <-resultCh
		if result.ownerID <= 0 {
			t.Fatalf("expected async child goroutine ID, got %d", result.ownerID)
		}
		if result.ownerID == parentID {
			t.Fatalf("expected async child to use a different goroutine ID from parent %d", parentID)
		}
		if !result.currentIsSelf || !result.poppedIsSelf {
			t.Fatalf("async frame lifecycle mismatch: current=%v popped=%v", result.currentIsSelf, result.poppedIsSelf)
		}
		if current := vm.CurrentFM(); current != parent {
			t.Fatalf("expected parent frame after async child exit, got %#v", current)
		}
		if count := vm.activeFrames.Load(); count != 1 {
			t.Fatalf("expected only the parent frame to remain active, got %d", count)
		}
	})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if frame := vm.CurrentFM(); frame != nil {
		t.Fatalf("expected all frames to be cleared, got %#v", frame)
	}
	if count := vm.activeFrames.Load(); count != 0 {
		t.Fatalf("expected zero active frames, got %d", count)
	}
}
