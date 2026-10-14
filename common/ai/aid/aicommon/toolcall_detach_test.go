package aicommon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolDetachCompletionRaceHasOneOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithDisableCreateDBRuntime(true))
	for i := 0; i < 50; i++ {
		manager, err := asyncexecmanager.New(ctx, asyncexecmanager.Config{})
		require.NoError(t, err)
		caller, err := NewToolCaller(ctx, WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(cfg.DefaultTask))
		require.NoError(t, err)
		var completions atomic.Int32
		var detachedOwner atomic.Bool
		execution, err := NewToolCallExecution(caller, func(resource asyncexecmanager.Resource) (asyncexecmanager.Summary, error) {
			return manager.Adopt(asyncexecmanager.Request{Resource: resource})
		}, func(_ *aitool.ToolResult, _ bool, _ error, detached bool) {
			completions.Add(1)
			detachedOwner.Store(detached)
		})
		require.NoError(t, err)
		race := make(chan struct{})
		execution.Start(func() (*aitool.ToolResult, bool, error) {
			<-race
			return &aitool.ToolResult{Success: true, Name: "race"}, false, nil
		})
		var detachErr error
		var id string
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); <-race; id, detachErr = caller.Detach() }()
		close(race)
		result, _, callErr := execution.Wait()
		wg.Wait()
		<-execution.Done()
		require.EqualValues(t, 1, completions.Load())
		if detachErr == nil {
			require.True(t, detachedOwner.Load())
			require.Nil(t, result)
			receipt, ok := callErr.(*ToolCallDetached)
			require.True(t, ok)
			require.Equal(t, id, receipt.Receipt.ID)
			summary, err := manager.Wait(ctx, id)
			require.NoError(t, err)
			require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
		} else {
			require.ErrorIs(t, detachErr, ErrToolCallSettled)
			require.False(t, detachedOwner.Load())
			require.NoError(t, callErr)
			require.NotNil(t, result)
			require.Empty(t, manager.List())
		}
		require.NoError(t, manager.Shutdown(ctx))
	}
}
