package reactservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultServiceIsProcessWide(t *testing.T) {
	require.Same(t, Default(), Default())
}

func TestRetireKeepsOldRuntimeQuiescedUntilProjectBind(t *testing.T) {
	service := New(nil)
	oldRuntime := service.Runtime()
	reservation, err := oldRuntime.ReserveSession(context.Background(), "old-project-session", "old-execution")
	require.NoError(t, err)

	retired := make(chan error, 1)
	go func() { retired <- service.RetireCurrentProject(context.Background()) }()
	select {
	case <-reservation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("retiring the project service did not cancel its reservation")
	}
	reservation.Release()
	select {
	case err := <-retired:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("project service retirement did not finish")
	}

	_, err = oldRuntime.ReserveSession(context.Background(), "stale-session", "stale-execution")
	require.Error(t, err, "a retired runtime must stay permanently quiesced")
	require.Equal(t, oldRuntime, service.Runtime(), "the retired runtime must cover the project-switch window")

	service.BindProject(nil)
	newRuntime := service.Runtime()
	require.NotEqual(t, oldRuntime, newRuntime)
	newReservation, err := newRuntime.ReserveSession(context.Background(), "new-project-session", "new-execution")
	require.NoError(t, err)
	newReservation.Release()
}

func TestRetireSerializesConcurrentSchedulerStart(t *testing.T) {
	service := New(nil)
	service.StartScheduler()
	reservation, err := service.Runtime().ReserveSession(context.Background(), "retiring-session", "retiring-owner")
	require.NoError(t, err)

	retired := make(chan error, 1)
	go func() { retired <- service.RetireCurrentProject(context.Background()) }()
	select {
	case <-reservation.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("project retirement did not reach runtime quiescence")
	}

	startReturned := make(chan struct{})
	go func() {
		service.StartScheduler()
		close(startReturned)
	}()
	select {
	case <-startReturned:
		t.Fatal("scheduler start crossed an in-progress project retirement")
	case <-time.After(50 * time.Millisecond):
	}

	reservation.Release()
	require.NoError(t, <-retired)
	select {
	case <-startReturned:
	case <-time.After(time.Second):
		t.Fatal("scheduler start remained blocked after project retirement")
	}
	service.mu.Lock()
	require.Nil(t, service.scheduler, "a retired project must not acquire a new scheduler")
	service.mu.Unlock()
}
