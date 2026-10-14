package asyncexecmanager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdoptCancelledBeforeJoinStillWaitsForCleanup(t *testing.T) {
	for i := 0; i < 20; i++ {
		owner, cancel := context.WithTimeout(context.Background(), time.Second)
		manager, err := New(owner, Config{})
		require.NoError(t, err)
		originalCtx, cancelOriginal := context.WithCancel(owner)
		cleanupStarted, allowCleanup, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() { <-originalCtx.Done(); close(cleanupStarted); <-allowCleanup; close(done) }()
		var joins atomic.Int32
		resource, err := NewResource(func(ctx context.Context) error {
			joins.Add(1)
			stop := context.AfterFunc(ctx, cancelOriginal)
			defer stop()
			if ctx.Err() != nil {
				cancelOriginal()
			}
			<-done
			return originalCtx.Err()
		}, func(context.Context) (Observation, error) { return Observation{}, nil })
		require.NoError(t, err)
		receipt, err := manager.Adopt(Request{Resource: resource})
		require.NoError(t, err)
		_, err = manager.Stop(receipt.ID)
		require.NoError(t, err)
		select {
		case <-cleanupStarted:
		case <-owner.Done():
			t.Fatal("cancel did not reach already-running work")
		}
		poll, pollCancel := context.WithCancel(owner)
		pollCancel()
		summary, err := manager.Wait(poll, receipt.ID)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, StateStopping, summary.State)
		close(allowCleanup)
		summary, err = manager.Wait(owner, receipt.ID)
		require.NoError(t, err)
		require.Equal(t, StateCancelled, summary.State)
		require.EqualValues(t, 1, joins.Load())
		require.NoError(t, manager.Shutdown(owner))
		cancelOriginal()
		cancel()
	}
}
