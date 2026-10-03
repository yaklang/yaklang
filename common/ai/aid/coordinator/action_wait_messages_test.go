package coordinator

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCoordinatorActionWaitMessages(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.WaitMessages(ctx, 3*time.Millisecond, nil), context.DeadlineExceeded)
	require.Equal(t, Running, c.Snapshot().Attempts["a"].State)
	c.Wake()
	require.NoError(t, c.WaitMessages(context.Background(), time.Second, nil))
	require.False(t, c.Snapshot().Finished)
}

func TestCoordinatorActionWaitMessagesYieldsNoncriticalBatchWithoutAutomaticPolling(t *testing.T) {
	c, h := executionFixture(t, true)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	c.mu.Lock()
	c.enqueueLocked("task_settled", "a", 1, "人工通过已由任务管理器应用", nil, false)
	c.mu.Unlock()
	c.publish()
	require.NoError(t, c.WaitMessages(context.Background(), time.Second, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.waitMessages(ctx, time.Millisecond, nil, false), context.DeadlineExceeded)
}
