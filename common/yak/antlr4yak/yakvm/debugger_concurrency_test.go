package yakvm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

func TestDebuggerPopsTheFinishingFramesStack(t *testing.T) {
	frameA := &Frame{ThreadID: 1}
	frameB := &Frame{ThreadID: 2}
	stackA := vmstack.New()
	stackB := vmstack.New()
	stackA.Push(&DebuggerState{frame: frameA})
	stackB.Push(&DebuggerState{frame: frameB})

	debugger := &Debugger{
		frame: frameB,
		StackTraces: map[int]*vmstack.Stack{
			frameA.ThreadID: stackA,
			frameB.ThreadID: stackB,
		},
	}
	debugger.StackTracePopForFrame(frameA)

	if stackA.Len() != 0 {
		t.Fatalf("finishing frame stack length = %d, want 0", stackA.Len())
	}
	if stackB.Len() != 1 {
		t.Fatalf("most recently observed frame stack length = %d, want 1", stackB.Len())
	}
}

func TestConcurrentRootExecutionsUseUniqueThreadIDs(t *testing.T) {
	vm := New()
	type result struct {
		rootID  int
		childID int
		err     error
	}
	results := make(chan result, 2)
	release := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)

	for range 2 {
		go func() {
			var got result
			got.err = vm.Exec(context.Background(), func(root *Frame) {
				got.rootID = root.ThreadID
				got.err = vm.exec(context.Background(), root, func(child *Frame) {
					got.childID = child.ThreadID
				}, Sub)
				ready.Done()
				<-release
			})
			results <- got
		}()
	}

	readyDone := make(chan struct{})
	go func() {
		ready.Wait()
		close(readyDone)
	}()
	select {
	case <-readyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent root executions did not overlap")
	}
	close(release)

	first, second := <-results, <-results
	for _, got := range []result{first, second} {
		if got.err != nil {
			t.Fatalf("execution failed: %v", got.err)
		}
		if got.rootID == 0 {
			t.Fatal("root execution received thread ID zero")
		}
		if got.childID != got.rootID {
			t.Fatalf("nested thread ID = %d, want root ID %d", got.childID, got.rootID)
		}
	}
	if first.rootID == second.rootID {
		t.Fatalf("concurrent roots shared thread ID %d", first.rootID)
	}
}

func TestConcurrentCrossVMSandboxExecutionsUseDefinitionVMThreadIDs(t *testing.T) {
	definitionVM := New()
	definitionFrame := NewFrame(definitionVM)
	definitionFrame.ThreadID = definitionVM.nextThreadID()

	function := NewFunction(nil, definitionVM.rootScope.GetSymTable())
	function.scope = definitionVM.rootScope
	function.defineFrame = definitionFrame

	callerVM := New()
	callerVM.SetSandboxMode(true)

	type result struct {
		threadID int
		vm       *VirtualMachine
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			got := result{}
			_, got.err = callerVM.ExecYakFunctionEx(context.Background(), function, nil, func(frame *Frame) {
				got.threadID = frame.ThreadID
				got.vm = frame.vm
			}, None)
			results <- got
		}()
	}

	first, second := <-results, <-results
	for _, got := range []result{first, second} {
		if got.err != nil {
			t.Fatalf("cross-VM execution failed: %v", got.err)
		}
		if got.vm != definitionVM {
			t.Fatalf("execution VM = %p, want definition VM %p", got.vm, definitionVM)
		}
		if got.threadID == 0 || got.threadID == definitionFrame.ThreadID {
			t.Fatalf("cross-VM execution reused retained definition thread ID %d", got.threadID)
		}
	}
	if first.threadID == second.threadID {
		t.Fatalf("concurrent cross-VM executions shared thread ID %d", first.threadID)
	}
}

func testDebuggerForCodes(t *testing.T, vm *VirtualMachine, source string, codes []*Code, callback func(*Debugger)) *Debugger {
	t.Helper()
	if callback == nil {
		callback = func(*Debugger) {}
	}
	debugger := NewDebugger(vm, source, codes, func(*Debugger) {}, callback)
	vm.debugMode = true
	vm.debugger = debugger
	return debugger
}

func testCode(source, path string, opcode OpcodeFlag, line int) *Code {
	return &Code{
		Opcode:             opcode,
		SourceCodePointer:  &source,
		SourceCodeFilePath: &path,
		StartLineNumber:    line,
		EndLineNumber:      line,
	}
}

func TestDebuggerDoesNotPushCrossVMCallOnCallerStack(t *testing.T) {
	for _, test := range []struct {
		name      string
		crossVM   bool
		wantDepth int
	}{
		{name: "same VM", crossVM: false, wantDepth: 1},
		{name: "cross VM", crossVM: true, wantDepth: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			callerVM := New()
			callerVM.SetSandboxMode(true)
			definitionVM := callerVM
			if test.crossVM {
				definitionVM = New()
			}

			definitionFrame := NewFrame(definitionVM)
			function := NewFunction(nil, definitionVM.rootScope.GetSymTable())
			function.defineFrame = definitionFrame
			callable := NewAutoValue(function)

			source, path := "f()", "debugger-cross-vm.yak"
			callCode := testCode(source, path, OpCall, 1)
			frame := NewFrame(callerVM)
			frame.ThreadID = callerVM.nextThreadID()
			frame.codes = []*Code{callCode}
			frame.push(callable)

			debugger := testDebuggerForCodes(t, callerVM, source, frame.codes, nil)
			debugger.ShouldCallback(frame)
			stack := debugger.StackTraces[frame.ThreadID]
			if stack == nil {
				t.Fatal("debugger did not create a thread stack")
			}
			if stack.Len() != test.wantDepth {
				t.Fatalf("caller debugger stack depth = %d, want %d", stack.Len(), test.wantDepth)
			}
		})
	}
}

func TestDebuggerTerminalPanicIsSerialized(t *testing.T) {
	var callbackCount atomic.Int32
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	debugger := &Debugger{
		started: true,
		callbackFunc: func(*Debugger) {
			if callbackCount.Add(1) == 1 {
				close(callbackEntered)
			}
			<-releaseCallback
		},
		pausedThreads: make(map[int]struct{}),
	}

	const workers = 16
	panicValue := &VMPanic{contextInfos: vmstack.New(), data: "concurrent panic"}
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			debugger.HandleForPanic(panicValue)
		}()
	}

	select {
	case <-callbackEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal callback was not entered")
	}
	close(releaseCallback)
	wg.Wait()

	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("terminal callback count = %d, want 1", got)
	}
	if !debugger.Finished() {
		t.Fatal("debugger was not marked finished")
	}
}

func TestDebuggerCallbackPanicStillPopsExpectedCaller(t *testing.T) {
	vm := New()
	source, path := "panic(1)", "debugger-callback-panic.yak"
	pushCode := testCode(source, path, OpPush, 1)
	pushCode.Op1 = NewIntValue(1)
	panicCode := testCode(source, path, OpPanic, 1)
	codes := []*Code{pushCode, panicCode}

	function := NewFunction(codes, vm.rootScope.GetSymTable())
	function.scope = vm.rootScope
	var callbackCount atomic.Int32
	debugger := testDebuggerForCodes(t, vm, source, codes, func(*Debugger) {
		callbackCount.Add(1)
		panic("debugger adapter failed")
	})
	debugger.codes[function.GetUUID()] = codes

	caller := NewFrame(vm)
	caller.ctx = context.Background()
	caller.ThreadID = vm.nextThreadID()
	outer := NewFrame(vm)
	callSite := testCode(source, path, OpCall, 1)
	stack := vmstack.New()
	stack.Push(&DebuggerState{frame: outer, code: callSite})
	stack.Push(&DebuggerState{frame: caller, code: callSite})
	debugger.StackTraces[caller.ThreadID] = stack

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		caller.CallYakFunction(false, function, nil)
	}()
	if recovered == nil {
		t.Fatal("Yak panic did not propagate to the caller")
	}
	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("terminal callback count = %d, want 1", got)
	}
	if stack.Len() != 1 || stack.Peek().(*DebuggerState).frame != outer {
		t.Fatalf("caller pop did not preserve the outer debugger frame; depth=%d", stack.Len())
	}

	// The terminal callback panic must not leave Debugger.lock held.
	unlocked := make(chan struct{})
	go func() {
		debugger.StackTracePopExpected(outer)
		close(unlocked)
	}()
	select {
	case <-unlocked:
	case <-time.After(2 * time.Second):
		t.Fatal("debugger lock remained held after terminal callback panic")
	}
}

func TestDebuggerMultipleDefersPopCallStackOnce(t *testing.T) {
	vm := New()
	source, path := "defer a(); defer b()", "debugger-multiple-defer.yak"
	firstDefer := testCode(source, path, OpDefer, 1)
	firstDefer.Op1 = NewAutoValue([]*Code{})
	secondDefer := testCode(source, path, OpDefer, 1)
	secondDefer.Op1 = NewAutoValue([]*Code{})
	codes := []*Code{firstDefer, secondDefer}

	function := NewFunction(codes, vm.rootScope.GetSymTable())
	function.scope = vm.rootScope
	debugger := testDebuggerForCodes(t, vm, source, codes, nil)
	debugger.codes[function.GetUUID()] = codes

	caller := NewFrame(vm)
	caller.ctx = context.Background()
	caller.ThreadID = vm.nextThreadID()
	outer := NewFrame(vm)
	callSite := testCode(source, path, OpCall, 1)
	stack := vmstack.New()
	stack.Push(&DebuggerState{frame: outer, code: callSite})
	stack.Push(&DebuggerState{frame: caller, code: callSite})
	debugger.StackTraces[caller.ThreadID] = stack

	caller.CallYakFunction(false, function, nil)
	if stack.Len() != 1 || stack.Peek().(*DebuggerState).frame != outer {
		t.Fatalf("multiple defers over-popped debugger stack; depth=%d", stack.Len())
	}
}

func TestDebuggerArgumentValidationPopsCallStack(t *testing.T) {
	vm := New()
	debugger := &Debugger{
		StackTraces:      make(map[int]*vmstack.Stack),
		ThreadStackTrace: make(map[int]*DebuggerState),
		pausedThreads:    make(map[int]struct{}),
		Reference:        NewReference(),
	}
	vm.debugMode = true
	vm.debugger = debugger

	caller := NewFrame(vm)
	caller.ctx = context.Background()
	caller.ThreadID = vm.nextThreadID()
	outer := NewFrame(vm)
	stack := vmstack.New()
	stack.Push(&DebuggerState{frame: outer})
	stack.Push(&DebuggerState{frame: caller})
	debugger.StackTraces[caller.ThreadID] = stack

	function := NewFunction(nil, vm.rootScope.GetSymTable())
	function.scope = vm.rootScope
	function.paramSymbols = []int{1}
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		caller.CallYakFunction(false, function, nil)
	}()
	if !panicked {
		t.Fatal("argument validation unexpectedly succeeded")
	}

	if stack.Len() != 1 || stack.Peek().(*DebuggerState).frame != outer {
		t.Fatalf("argument validation panic left a phantom call frame; depth=%d", stack.Len())
	}
}

func TestDebuggerStepOutWithoutFrameReturnsError(t *testing.T) {
	debugger := &Debugger{StackTraces: make(map[int]*vmstack.Stack)}
	if err := debugger.StepOut(); err == nil {
		t.Fatal("StepOut without a current frame unexpectedly succeeded")
	}
}
