package lowhttp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnPoolDebugPrinterStopsAndRestarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewHttpConnPool(ctx, 1, 1)

	pool.EnableConnPoolDebug(true)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&pool.debugRunning) == 1
	}, time.Second, time.Millisecond)

	pool.EnableConnPoolDebug(false)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&pool.debugRunning) == 0
	}, time.Second, time.Millisecond)

	pool.EnableConnPoolDebug(true)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&pool.debugRunning) == 1
	}, time.Second, time.Millisecond)

	cancel()
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&pool.debugRunning) == 0
	}, time.Second, time.Millisecond)
}
