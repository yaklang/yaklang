package yak

import (
	"bytes"
	"context"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestYakFunctionCallerTimeoutDoesNotLeakResultSender(t *testing.T) {
	const callCount = 32
	started := make(chan struct{}, callCount)
	nativeReturned := make(chan struct{}, callCount)
	release := make(chan struct{})

	manager := NewYakToCallerManager()
	manager.callTimeout = 5 * time.Millisecond
	callers, err := manager.fetchFunctionFromSourceCode(
		CreateYakitPluginContext("timeout-leak-test").WithContext(context.Background()),
		WithFetchCode(`
spin = func() {
    blockUntilReleased()
    return "done"
}
`),
		WithFetchEngineHook(func(engine *antlr4yak.Engine) error {
			engine.SetVars(map[string]any{
				// A native Go call deliberately ignores the Yak context. This
				// forces the caller to time out first and the VM worker to publish
				// its result only after the receiver has already returned.
				"blockUntilReleased": func() {
					started <- struct{}{}
					<-release
					nativeReturned <- struct{}{}
				},
			})
			return nil
		}),
		WithFetchFunctionNames("spin"),
		WithFetchCacheDisabled(),
	)
	require.NoError(t, err)
	caller := callers["spin"]
	require.NotNil(t, caller)

	baseline := runtime.NumGoroutine()
	for i := 0; i < callCount; i++ {
		panicValue := callHandlerAndRecover(caller)
		require.NotNil(t, panicValue)
		require.ErrorIs(t, panicValue.(error), context.DeadlineExceeded)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("native worker %d did not start", i)
		}
	}
	close(release)
	for i := 0; i < callCount; i++ {
		select {
		case <-nativeReturned:
		case <-time.After(2 * time.Second):
			t.Fatalf("native worker %d did not return after release", i)
		}
	}

	// A timed-out caller no longer receives the result. The VM worker must still
	// publish into the one-slot result channel and terminate; an unbuffered
	// channel leaves all callCount goroutines blocked in the handler's defer.
	require.Eventually(t, func() bool {
		var stacks bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&stacks, 2); err != nil {
			return false
		}
		return !strings.Contains(strings.ToLower(stacks.String()), "fetchfunctionfromsourcecode.func")
	}, 3*time.Second, 25*time.Millisecond)

	// Keep the broader count as a secondary guard for leaks whose stack name
	// changes during refactoring.
	require.Eventually(t, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= baseline+8
	}, 3*time.Second, 25*time.Millisecond)
}

func callHandlerAndRecover(caller *YakFunctionCaller) (panicValue any) {
	defer func() {
		panicValue = recover()
	}()
	caller.Handler(nil)
	return nil
}
