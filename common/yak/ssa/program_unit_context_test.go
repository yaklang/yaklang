package ssa

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// The context belongs to the registering worker, not to the Program it happens
// to visit. A directory import or a deferred helper can visit the same Program
// as another worker without inheriting (or clearing) that worker's unit key.
func TestCompileUnitContextIsWorkerLocal(t *testing.T) {
	for _, scope := range []string{"application", "library", "application-fallback"} {
		t.Run(scope, func(t *testing.T) {
			app := newDeferredBuildTestProgram(t, "worker-unit-context-"+scope)
			contextProgram, registerProgram := app, app
			if scope != "application" {
				lib := app.NewLibrary("shared", []string{"shared"})
				registerProgram = lib
				if scope == "library" {
					contextProgram = lib
				}
			}

			const workers = 8
			var ready, done sync.WaitGroup
			ready.Add(workers)
			done.Add(workers)
			observed := make(chan string, workers)
			for i := 0; i < workers; i++ {
				go func(index int) {
					defer done.Done()
					key := fmt.Sprintf("unit-%d", index)
					contextProgram.BeginCompileUnit(key)
					defer contextProgram.EndCompileUnit()
					ready.Done()
					ready.Wait()
					if got := registerProgram.CurrentCompileUnit(); got != key {
						observed <- fmt.Sprintf("worker %s inherited %q", key, got)
					}
					registerProgram.RegisterDeferredBuild(DeferredBuildKindHelper, key, func() {})
				}(i)
			}
			done.Wait()
			close(observed)
			for problem := range observed {
				t.Error(problem)
			}
			require.Empty(t, contextProgram.CurrentCompileUnit(), "finished workers must not leave a unit on the caller")
			for i := 0; i < workers; i++ {
				key := fmt.Sprintf("unit-%d", i)
				task, ok := app.deferredBuilds.Get(string(DeferredBuildKindHelper) + ":" + key)
				require.True(t, ok)
				require.Equal(t, key, task.unitKey, "deferred registration must retain its own worker's unit")
			}
		})
	}
}
