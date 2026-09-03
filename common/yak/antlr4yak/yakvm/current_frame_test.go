package yakvm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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

func TestSubFrameKeepsParentVMThreadID(t *testing.T) {
	vm := New()
	parent := NewFrame(vm)
	parent.ThreadID = 7
	child := NewSubFrame(parent)
	if child.ThreadID != parent.ThreadID {
		t.Fatalf("subframe thread ID = %d, want parent ID %d", child.ThreadID, parent.ThreadID)
	}

	// A concurrent async spawn may advance the VM-wide counter, but executing a
	// synchronous child must not make it drift to that unrelated ID.
	vm.ThreadIDCount = 99
	child.Exec(nil)
	if child.ThreadID != parent.ThreadID {
		t.Fatalf("executed subframe thread ID drifted to %d, want %d", child.ThreadID, parent.ThreadID)
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

func TestAsyncStartFailuresDoNotLeakWaitGroup(t *testing.T) {
	waitReturns := func(t *testing.T, vm *VirtualMachine) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			vm.AsyncWait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("AsyncWait stayed blocked after async startup failed")
		}
	}

	t.Run("pre-canceled Yak function", func(t *testing.T) {
		vm := New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		parent := NewFrame(vm)
		parent.ctx = ctx
		function := NewFunction(nil, vm.rootScope.GetSymTable())
		function.scope = vm.rootScope

		parent.asyncCall(NewAutoValue(function), false, nil)
		waitReturns(t, vm)
	})

	t.Run("invalid native arguments", func(t *testing.T) {
		vm := New()
		parent := NewFrame(vm)
		parent.ctx = context.Background()
		panicked := false
		func() {
			defer func() { panicked = recover() != nil }()
			parent.asyncCall(NewAutoValue(func(_ int) {}), false, nil)
		}()
		if !panicked {
			t.Fatal("expected invalid async native call to panic")
		}
		waitReturns(t, vm)
	})
}

func TestTraceFailureClearsCurrentFrame(t *testing.T) {
	t.Run("panic", func(t *testing.T) {
		vm := New()
		gotPanic := false
		func() {
			defer func() {
				gotPanic = recover() != nil
			}()
			_ = vm.Exec(context.Background(), func(frame *Frame) {
				panic("trace failed")
			}, Trace)
		}()

		if !gotPanic {
			t.Fatal("expected trace panic")
		}
		assertNoActiveFrames(t, vm)
	})

	t.Run("canceled", func(t *testing.T) {
		vm := New()
		ctx, cancel := context.WithCancel(context.Background())
		err := vm.Exec(ctx, func(frame *Frame) {
			cancel()
		}, Trace)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
		assertNoActiveFrames(t, vm)
	})
}

func TestSuccessfulTraceInlineKeepsExactlyOneFrame(t *testing.T) {
	vm := New()
	var traceFrame *Frame
	if err := vm.Exec(context.Background(), func(frame *Frame) {
		traceFrame = frame
	}, Trace); err != nil {
		t.Fatalf("trace exec failed: %v", err)
	}

	if current := vm.CurrentFM(); current != traceFrame {
		t.Fatalf("expected retained trace frame, got %#v", current)
	}
	if count := vm.activeFrames.Load(); count != 1 {
		t.Fatalf("expected one retained trace frame, got %d", count)
	}

	for i := 0; i < 100; i++ {
		if err := vm.Exec(context.Background(), func(frame *Frame) {
			if frame != traceFrame {
				t.Fatalf("inline execution switched frame: got %#v want %#v", frame, traceFrame)
			}
		}, Inline); err != nil {
			t.Fatalf("inline exec %d failed: %v", i, err)
		}
		if count := vm.activeFrames.Load(); count != 1 {
			t.Fatalf("inline exec %d leaked a frame: active=%d", i, count)
		}
	}

	if popped := vm.popCurrentFrame(traceFrame); popped != traceFrame {
		t.Fatalf("failed to remove retained trace frame: %#v", popped)
	}
	assertNoActiveFrames(t, vm)
}

func TestPopCurrentFrameRefusesMismatchedFrame(t *testing.T) {
	vm := New()
	if err := vm.Exec(context.Background(), func(current *Frame) {
		intruder := NewFrame(vm)
		intruder.ownerGoroutineID = current.ownerGoroutineID
		if popped := vm.popCurrentFrame(intruder); popped != nil {
			t.Fatalf("mismatched pop removed %#v", popped)
		}
		if got := vm.CurrentFM(); got != current {
			t.Fatalf("mismatched pop corrupted current frame: %#v", got)
		}
		if count := vm.activeFrames.Load(); count != 1 {
			t.Fatalf("mismatched pop changed active count: %d", count)
		}
	}); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	assertNoActiveFrames(t, vm)
}

func assertNoActiveFrames(t *testing.T, vm *VirtualMachine) {
	t.Helper()
	if frame := vm.CurrentFM(); frame != nil {
		t.Fatalf("expected current frame to be empty, got %#v", frame)
	}
	if count := vm.activeFrames.Load(); count != 0 {
		t.Fatalf("expected zero active frames, got %d", count)
	}
	vm.frameStacksMu.RLock()
	defer vm.frameStacksMu.RUnlock()
	if len(vm.frameStacks) != 0 {
		t.Fatalf("expected no goroutine frame stacks, got %d", len(vm.frameStacks))
	}
}
