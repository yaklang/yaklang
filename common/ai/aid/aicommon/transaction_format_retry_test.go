package aicommon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsActionFormatError(t *testing.T) {
	require.True(t, isActionFormatError(errors.New(`action resolution failed: requested="<missing>"`)))
	require.True(t, isActionFormatError(errors.New("failed to parse action")))
	require.False(t, isActionFormatError(errors.New("connection reset")))
}

func TestResolveAIFormatAutoRetryCount(t *testing.T) {
	require.Equal(t, int64(3), resolveAIFormatAutoRetryCount(nil, 5))
	require.Equal(t, int64(2), resolveAIFormatAutoRetryCount(nil, 2))

	cfg := newTransactionTestConfig(context.Background())
	cfg.formatRetry = 2
	require.Equal(t, int64(2), resolveAIFormatAutoRetryCount(cfg, 5))
}

func TestCallAITransaction_FormatRetryBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 5
	cfg.formatRetry = 2
	cfg.retryWait = func(context.Context, time.Duration) error { return nil }

	var calls int64
	err := CallAITransaction(cfg, "prompt", func(req *AIRequest) (*AIResponse, error) {
		atomic.AddInt64(&calls, 1)
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}, func(rsp *AIResponse) error {
		return errors.New(`action resolution failed: requested="<missing>"; reason=no @action`)
	})
	require.Error(t, err)
	require.True(t, isActionFormatError(err))
	require.Equal(t, int64(2), atomic.LoadInt64(&calls), "format retry should stop at formatRetry=2")
}
