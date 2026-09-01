package antlr4yak

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type crossVMCapture struct {
	index            int
	line             int
	runtimeID        string
	value            string
	engineLine       int
	engineRuntimeID  string
	engineRuntimeErr error
}

func newCrossVMSandboxFixture(t *testing.T, captures chan<- crossVMCapture) (*Engine, *yakvm.Function) {
	t.Helper()

	definitionEngine := New()
	definitionEngine.SetVars(map[string]any{"runtimeId": "definition-runtime"})
	definitionEngine.ImportLibs(map[string]any{
		"outerOnly": func() string { return "outer-library" },
		"capture": func(index, line int, runtimeID, value string) {
			capture := crossVMCapture{
				index:     index,
				line:      line,
				runtimeID: runtimeID,
				value:     value,
			}
			engineLine, lineErr := definitionEngine.RuntimeInfo("line")
			engineRuntimeID, runtimeIDErr := definitionEngine.RuntimeInfo("runtimeId")
			if lineErr != nil {
				capture.engineRuntimeErr = lineErr
			} else if runtimeIDErr != nil {
				capture.engineRuntimeErr = runtimeIDErr
			} else {
				capture.engineLine, _ = engineLine.(int)
				capture.engineRuntimeID = fmt.Sprint(engineRuntimeID)
			}
			captures <- capture
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, definitionEngine.SafeEvalWithoutCache(ctx, `
factory = func(prefix) {
    return func(index) {
        line = runtime.GetInfo("line")~
        runtimeID = runtime.GetInfo("runtimeId")~
        value = prefix + ":" + outerOnly()
        capture(index, line, runtimeID, value)
        return value
    }
}
external = factory("closed")
unstable = func(shouldPanic) {
    if shouldPanic { panic("cross-vm-panic") }
    return "recovered"
}
`))

	rawFunction, ok := definitionEngine.GetVar("external")
	require.True(t, ok)
	function, ok := rawFunction.(*yakvm.Function)
	require.True(t, ok)
	require.NotNil(t, function)
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
	return definitionEngine, function
}

func assertCrossVMCapture(t *testing.T, capture crossVMCapture, index int) {
	t.Helper()
	require.Equal(t, index, capture.index)
	require.Positive(t, capture.line)
	require.Equal(t, "definition-runtime", capture.runtimeID)
	require.Equal(t, "closed:outer-library", capture.value)
	require.NoError(t, capture.engineRuntimeErr)
	// The Yak query runs on its assignment while Engine.RuntimeInfo runs from
	// inside capture(), so they intentionally observe different instructions in
	// the same external function frame.
	require.Positive(t, capture.engineLine)
	require.Equal(t, capture.runtimeID, capture.engineRuntimeID)
}

func TestCrossVMSandboxFunctionUsesDefinitionFrame(t *testing.T) {
	captures := make(chan crossVMCapture, 1)
	definitionEngine, function := newCrossVMSandboxFixture(t, captures)

	sandboxEngine := New()
	sandboxEngine.SetVars(map[string]any{"runtimeId": "sandbox-runtime"})
	sandboxEngine.ImportLibs(map[string]any{"external": function})
	sandboxEngine.SetSandboxMode(true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sandboxEngine.SafeEvalWithoutCache(ctx, `result = external(7)`))
	result, ok := sandboxEngine.GetVar("result")
	require.True(t, ok)
	require.Equal(t, "closed:outer-library", result)
	assertCrossVMCapture(t, <-captures, 7)

	// Both the orchestration VM and the definition VM must be back at an idle
	// stack after the cross-VM call. A subsequent call proves that no stale top
	// frame changes parent selection.
	require.Nil(t, sandboxEngine.GetVM().CurrentFM())
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
	require.NoError(t, sandboxEngine.SafeEvalWithoutCache(ctx, `result = external(8)`))
	assertCrossVMCapture(t, <-captures, 8)
	require.Nil(t, sandboxEngine.GetVM().CurrentFM())
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
}

func TestCrossVMSandboxPanicDoesNotPoisonNextCall(t *testing.T) {
	captures := make(chan crossVMCapture, 1)
	definitionEngine, function := newCrossVMSandboxFixture(t, captures)
	rawUnstable, ok := definitionEngine.GetVar("unstable")
	require.True(t, ok)
	unstable, ok := rawUnstable.(*yakvm.Function)
	require.True(t, ok)

	sandboxEngine := New()
	sandboxEngine.ImportLibs(map[string]any{
		"external": function,
		"unstable": unstable,
	})
	sandboxEngine.SetSandboxMode(true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := sandboxEngine.SafeEvalWithoutCache(ctx, `unstable(true)`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cross-vm-panic")
	require.Nil(t, sandboxEngine.GetVM().CurrentFM())
	require.Nil(t, definitionEngine.GetVM().CurrentFM())

	// Recovering the first call must clear only that invocation's coroutine.
	// A fresh call must not inherit its panic or a stale definition-side frame.
	require.NoError(t, sandboxEngine.SafeEvalWithoutCache(ctx, `
result = unstable(false)
external(9)
`))
	result, ok := sandboxEngine.GetVar("result")
	require.True(t, ok)
	require.Equal(t, "recovered", result)
	assertCrossVMCapture(t, <-captures, 9)
	require.Nil(t, sandboxEngine.GetVM().CurrentFM())
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
}

func TestCrossVMSandboxAsyncFunctionsUseDefinitionFrame(t *testing.T) {
	const workers = 24
	captures := make(chan crossVMCapture, workers)
	definitionEngine, function := newCrossVMSandboxFixture(t, captures)

	sandboxEngine := New()
	sandboxEngine.SetVars(map[string]any{"runtimeId": "sandbox-runtime"})
	sandboxEngine.ImportLibs(map[string]any{"external": function})
	sandboxEngine.SetSandboxMode(true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sandboxEngine.SafeEvalWithoutCache(ctx, `
for index in 24 {
    go external(index)
}
waitAllAsyncCallFinish()
`))

	seen := make(map[int]struct{}, workers)
	for i := 0; i < workers; i++ {
		select {
		case capture := <-captures:
			assertCrossVMCapture(t, capture, capture.index)
			seen[capture.index] = struct{}{}
		case <-ctx.Done():
			t.Fatalf("received %d/%d cross-VM async captures: %v", i, workers, ctx.Err())
		}
	}
	require.Len(t, seen, workers)
	require.Nil(t, sandboxEngine.GetVM().CurrentFM())
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
}

func TestConcurrentCrossVMSandboxFunctionsKeepFramesIsolated(t *testing.T) {
	const workers = 16
	captures := make(chan crossVMCapture, workers)
	definitionEngine, function := newCrossVMSandboxFixture(t, captures)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			sandboxEngine := New()
			sandboxEngine.SetVars(map[string]any{"runtimeId": fmt.Sprintf("sandbox-%d", index)})
			sandboxEngine.ImportLibs(map[string]any{"external": function, "index": index})
			sandboxEngine.SetSandboxMode(true)
			errs <- sandboxEngine.SafeEvalWithoutCache(ctx, `external(index)`)
		}(i)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	seen := make(map[int]struct{}, workers)
	for i := 0; i < workers; i++ {
		capture := <-captures
		assertCrossVMCapture(t, capture, capture.index)
		seen[capture.index] = struct{}{}
	}
	require.Len(t, seen, workers)
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
}

func TestConcurrentCrossVMSandboxPanicDoesNotLeakBetweenCallers(t *testing.T) {
	const workers = 16
	started := make(chan struct{}, workers)
	release := make(chan struct{})

	definitionEngine := New()
	definitionEngine.ImportLibs(map[string]any{
		"blockUntilReleased": func() {
			started <- struct{}{}
			<-release
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, definitionEngine.SafeEvalWithoutCache(ctx, `
concurrentUnstable = func(shouldPanic) {
    blockUntilReleased()
    if shouldPanic { panic("cross-vm-concurrent-panic") }
    return "clean"
}
`))
	rawFunction, ok := definitionEngine.GetVar("concurrentUnstable")
	require.True(t, ok)
	function, ok := rawFunction.(*yakvm.Function)
	require.True(t, ok)

	type outcome struct {
		index  int
		result any
		err    error
	}
	outcomes := make(chan outcome, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			sandboxEngine := New()
			sandboxEngine.SetVars(map[string]any{"shouldPanic": index%2 == 0})
			sandboxEngine.ImportLibs(map[string]any{"concurrentUnstable": function})
			sandboxEngine.SetSandboxMode(true)
			err := sandboxEngine.SafeEvalWithoutCache(ctx, `result = concurrentUnstable(shouldPanic)`)
			result, _ := sandboxEngine.GetVar("result")
			outcomes <- outcome{index: index, result: result, err: err}
		}(i)
	}

	// Release every invocation together so successful and panicking calls share
	// the definition VM concurrently. Reusing the function's definition
	// coroutine would let one invocation's lastPanic poison an unrelated caller.
	for i := 0; i < workers; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatalf("started %d/%d cross-VM calls: %v", i, workers, ctx.Err())
		}
	}
	close(release)
	wait.Wait()
	close(outcomes)

	seen := make(map[int]struct{}, workers)
	for result := range outcomes {
		seen[result.index] = struct{}{}
		if result.index%2 == 0 {
			require.Error(t, result.err)
			require.Contains(t, result.err.Error(), "cross-vm-concurrent-panic")
			continue
		}
		require.NoError(t, result.err)
		require.Equal(t, "clean", result.result)
	}
	require.Len(t, seen, workers)
	require.Nil(t, definitionEngine.GetVM().CurrentFM())
}
