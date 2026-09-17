package yakvm

import (
	"context"
	"testing"
)

func TestSynchronousExecutionFrameLifetime(t *testing.T) {
	vm := New()
	vm.GetConfig().SetSynchronousExecution(true)
	for _, fail := range []bool{false, true, false} {
		func() {
			defer func() {
				if got := recover(); (got != nil) != fail {
					t.Errorf("panic=%v, expected=%t", got, fail)
				}
			}()
			err := vm.Exec(context.Background(), func(parent *Frame) {
				if vm.CurrentFM() != parent || parent.nativeCallbackFrame() != parent {
					t.Fatal("wrong synchronous parent")
				}
				if err := vm.Exec(context.Background(), func(child *Frame) {
					if vm.CurrentFM() != child {
						t.Fatal("wrong synchronous child")
					}
				}, Sub); err != nil {
					t.Fatal(err)
				}
				if vm.CurrentFM() != parent {
					t.Fatal("child did not restore parent")
				}
				if fail {
					panic("expected")
				}
			})
			if err != nil {
				t.Fatal(err)
			}
		}()
		if vm.CurrentFM() != nil || len(vm.synchronousFrames) != 0 {
			t.Fatal("execution retained a frame")
		}
		if len(vm.frameStacks) != 0 || vm.activeFrames.Load() != 0 {
			t.Fatal("synchronous execution registered a goroutine stack")
		}
	}
}
