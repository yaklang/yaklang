package yak

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// One cancellation scenario, exercised at each exit boundary: registered
// cleanup runs in LIFO order, nested calls can run, ordinary code stays cancelled,
// and nested blocked cleanup cannot keep renewing its grace period.
func TestYakDeferredCleanupAfterCancellation(t *testing.T) {
	for _, mode := range []string{"return", "panic", "cancel", "deadline", "cancel_during_cleanup"} {
		t.Run(mode, func(t *testing.T) {
			parent := context.Background()
			if mode == "deadline" {
				var stop context.CancelFunc
				parent, stop = context.WithTimeout(parent, 100*time.Millisecond)
				defer stop()
			}
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			var mu sync.Mutex
			var observed []string
			mark := func(value string) { mu.Lock(); observed = append(observed, value); mu.Unlock() }
			code := `
cleanup = func() {
    defer mark("nested_defer")
    mark("cleanup")
}
defer mark("last")
defer cleanup()
if mode == "cancel_during_cleanup" { defer cancel_owner() }
if mode == "cancel" { cancel_owner() }
if mode == "deadline" { wait_cancel() }
if mode == "panic" { panic("expected panic") }
mark("ordinary")
`
			vars := map[string]any{
				"mode": mode, "mark": mark, "cancel_owner": cancel,
				"wait_cancel": func() { <-ctx.Done() },
			}
			engine := NewScriptEngine(1)
			engine.RegisterEngineHooks(func(e *antlr4yak.Engine) error { e.SetVars(vars); return nil })
			_, err := engine.ExecuteExWithContext(ctx, code, nil)
			if mode == "return" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			mu.Lock()
			defer mu.Unlock()
			expected := []string{"cleanup", "nested_defer", "last"}
			if mode == "return" || mode == "panic" || mode == "cancel_during_cleanup" {
				// Panic occurs before ordinary; only normal return and cancellation
				// initiated by the first defer reach this instruction.
				if mode != "panic" {
					expected = append([]string{"ordinary"}, expected...)
				}
			}
			require.Equal(t, expected, observed)
			if mode != "return" && mode != "panic" {
				require.True(t, errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded))
			}
		})
	}
	t.Run("nested_cleanup_shares_cancellation_budget", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := time.Now()
		engine := NewScriptEngine(1)
		engine.RegisterEngineHooks(func(e *antlr4yak.Engine) error { e.SetVars(map[string]any{"cancel_owner": cancel}); return nil })
		_, err := engine.ExecuteExWithContext(ctx, `
defer func() {
    defer func() { blocked = make(chan var); <-blocked }()
    blocked = make(chan var)
    <-blocked
}()
cancel_owner()
`, nil)
		require.Error(t, err)
		require.GreaterOrEqual(t, time.Since(started), 4*time.Second)
		require.Less(t, time.Since(started), 8*time.Second, "nested defers must not each get a new five-second budget")
	})
}
