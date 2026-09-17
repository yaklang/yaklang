package antlr4yak

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestConcurrentNativeCallbacksKeepCurrentFrames(t *testing.T) {
	engine := New()
	var callers sync.WaitGroup
	entered := make(chan *yakvm.Frame, 2)
	release := make(chan struct{})
	results := make(chan int, 2)
	engine.SetVars(map[string]any{
		"observe": func() {
			entered <- engine.GetVM().CurrentFM()
			<-release
		},
		"parallel": func(callback func(int) int) {
			parent := engine.GetVM().CurrentFM()
			for i := 0; i < 2; i++ {
				callers.Add(1)
				go func(i int) { defer callers.Done(); results <- callback(i) }(i)
			}
			first, second := <-entered, <-entered
			if first == nil || second == nil || first == second || first == parent || second == parent {
				t.Error("native callbacks must have independent current frames")
			}
			if engine.GetVM().CurrentFM() != parent {
				t.Error("native callback replaced the caller frame")
			}
			close(release)
			callers.Wait()
		},
	})
	require.NoError(t, engine.SafeEval(context.Background(), `parallel(func(i) { observe(); return i + 1 })`))
	require.Equal(t, 3, <-results+<-results)
	require.Nil(t, engine.GetVM().CurrentFM())
}

func TestCoreRetainedNativeCallbackAfterCallerReturns(t *testing.T) {
	engine := New()
	var callback func(int) int
	engine.SetVars(map[string]any{
		"retain": func(fn func(int) int) { callback = fn },
	})
	require.NoError(t, engine.SafeEval(context.Background(), `
install = () => { captured = 41; retain((n) => captured + n) }
install()
`))
	require.NotNil(t, callback)
	// Both the original goroutine and concurrent callers use the retained
	// closure after its defining function/frame has returned.
	require.Equal(t, 42, callback(1))
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			for n := 0; n < 20; n++ {
				if got := callback(n); got != 41+n {
					t.Errorf("retained callback: got %d, want %d", got, 41+n)
				}
			}
			if engine.GetVM().CurrentFM() != nil {
				t.Error("callback retained its current frame after return")
			}
		}()
	}
	callers.Wait()
	require.Nil(t, engine.GetVM().CurrentFM())
}
