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
