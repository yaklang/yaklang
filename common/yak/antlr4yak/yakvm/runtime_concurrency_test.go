package yakvm

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

type runtimeLineResult struct {
	line int
	err  error
}

func TestRuntimeInfoUsesCallingGoroutineFrame(t *testing.T) {
	vm := New()
	aReady := make(chan struct{})
	bReady := make(chan struct{})
	releaseB := make(chan struct{})
	aResult := make(chan runtimeLineResult, 1)
	bResult := make(chan runtimeLineResult, 1)
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		err := vm.Exec(context.Background(), func(frame *Frame) {
			frame.codes = []*Code{{StartLineNumber: 101}}
			close(aReady)
			<-bReady
			line, lineErr := runtimeLine(frame)
			aResult <- runtimeLineResult{line: line, err: lineErr}
		})
		if err != nil {
			aResult <- runtimeLineResult{err: err}
		}
	}()
	<-aReady

	go func() {
		defer workers.Done()
		err := vm.Exec(context.Background(), func(frame *Frame) {
			frame.codes = []*Code{{StartLineNumber: 202}}
			close(bReady)
			line, lineErr := runtimeLine(frame)
			bResult <- runtimeLineResult{line: line, err: lineErr}
			<-releaseB
		})
		if err != nil {
			bResult <- runtimeLineResult{err: err}
		}
	}()

	a := <-aResult
	b := <-bResult
	close(releaseB)
	workers.Wait()
	if a.err != nil || a.line != 101 {
		t.Fatalf("goroutine A runtime line mismatch: line=%d err=%v", a.line, a.err)
	}
	if b.err != nil || b.line != 202 {
		t.Fatalf("goroutine B runtime line mismatch: line=%d err=%v", b.line, b.err)
	}

	assertNoActiveFrames(t, vm)
}

func TestRuntimeInfoLineRejectsFrameWithoutCurrentCode(t *testing.T) {
	vm := New()
	var lineErr error
	err := vm.Exec(context.Background(), func(frame *Frame) {
		if frame.CurrentCode() != nil {
			t.Fatal("empty frame unexpectedly has a current code")
		}
		_, lineErr = runtimeLine(frame)
	})
	if err != nil {
		t.Fatal(err)
	}
	if lineErr == nil {
		t.Fatal("runtime.GetInfo(line) accepted a frame without code")
	}
	assertNoActiveFrames(t, vm)
}

func TestImportRuntimeLibKeepsExplicitFrameBinding(t *testing.T) {
	vm := New()
	frame := NewFrame(vm)
	frame.codes = []*Code{{StartLineNumber: 303}}
	ImportRuntimeLib(frame)

	line, err := runtimeLine(frame)
	if err != nil {
		t.Fatal(err)
	}
	if line != 303 {
		t.Fatalf("explicit frame runtime line = %d, want 303", line)
	}

	// The explicit frame binding must not replace the VM-wide dynamic runtime
	// helper used by later or concurrent executions.
	var dynamicLine int
	err = vm.Exec(context.Background(), func(dynamicFrame *Frame) {
		dynamicFrame.codes = []*Code{{StartLineNumber: 404}}
		dynamicLine, err = runtimeLine(dynamicFrame)
	})
	if err != nil {
		t.Fatal(err)
	}
	if dynamicLine != 404 {
		t.Fatalf("dynamic VM runtime line = %d, want 404", dynamicLine)
	}
	assertNoActiveFrames(t, vm)
}

func runtimeLine(frame *Frame) (int, error) {
	raw, ok := frame.GlobalVariables.Load("runtime")
	if !ok {
		return 0, fmt.Errorf("runtime library not found")
	}
	lib, ok := raw.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("unexpected runtime library type %T", raw)
	}
	getInfo, ok := lib["GetInfo"].(func(string, ...interface{}) (interface{}, error))
	if !ok {
		return 0, fmt.Errorf("runtime.GetInfo has type %T", lib["GetInfo"])
	}
	line, err := getInfo("line")
	if err != nil {
		return 0, err
	}
	value, ok := line.(int)
	if !ok {
		return 0, fmt.Errorf("runtime line has type %T", line)
	}
	return value, nil
}
