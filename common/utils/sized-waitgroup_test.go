package utils

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSizedWaitGroup_AddWithContext(t *testing.T) {
	swg := NewSizedWaitGroup(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 10000; i++ {
		err := swg.AddWithContext(ctx)
		if err == nil {
			swg.Done()
			t.Fatal("Smoking test AddWithContext fail")
		}
	}
}

func TestSizedWaitGroupSetZeroWithLateDone(t *testing.T) {
	swg := NewSizedWaitGroup(2)
	swg.Add(2)
	swg.SetZero()
	swg.Done()
	swg.Done()
	// Late completions must not poison the counter, even when no Wait was
	// running at the time of cancellation. Reuse starts after they finish.
	require.NotPanics(t, swg.SetZero)
	require.EqualValues(t, 0, swg.WaitingEventCount.Load())
	swg.Add()
	swg.Done()
	swg.Wait()
}

func TestSizedWaitGroupConcurrentSetZeroAndDone(t *testing.T) {
	for round := 0; round < 100; round++ {
		const workers = 16
		swg := NewSizedWaitGroup(workers)
		swg.Add(workers)
		start := make(chan struct{})
		var finished sync.WaitGroup
		for i := 0; i < workers; i++ {
			finished.Add(2)
			go func() {
				defer finished.Done()
				<-start
				swg.Done()
			}()
			go func() {
				defer finished.Done()
				<-start
				swg.SetZero()
			}()
		}
		close(start)
		finished.Wait()
		require.EqualValues(t, 0, swg.WaitingEventCount.Load())
		swg.Wait()
	}
}

func TestSizedWaitGroupCancellationReleasesWaitAndBlockedAdd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	swg := NewSizedWaitGroup(1, ctx)
	swg.Add()
	added := make(chan error, 1)
	go func() { added <- swg.AddWithContext(ctx) }()
	waited := make(chan struct{})
	go func() {
		swg.Wait()
		close(waited)
	}()
	cancel()
	select {
	case err := <-added:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock AddWithContext")
	}
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock Wait")
	}
	swg.Done()
	require.NotPanics(t, swg.SetZero)
	require.EqualValues(t, 0, swg.WaitingEventCount.Load())
}
