package aicommon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
)

func TestAIRetryBackoffSchedule(t *testing.T) {
	tests := []struct {
		retryNumber int64
		want        time.Duration
	}{
		{retryNumber: 0, want: 0},
		{retryNumber: 1, want: 2 * time.Second},
		{retryNumber: 2, want: 4 * time.Second},
		{retryNumber: 3, want: 8 * time.Second},
		{retryNumber: 4, want: 16 * time.Second},
		{retryNumber: 5, want: 32 * time.Second},
		{retryNumber: 6, want: 32 * time.Second},
		{retryNumber: 100, want: 32 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("retry_%d", tt.retryNumber), func(t *testing.T) {
			require.Equal(t, tt.want, aiRetryBackoff(tt.retryNumber))
		})
	}
}

func TestCallAITransaction_502UsesExponentialBackoff(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 6

	var waits []time.Duration
	cfg.retryWait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	callCount := 0
	callAI := func(*AIRequest) (*AIResponse, error) {
		callCount++
		return make502Response(), fmt.Errorf("HTTP 502: bad gateway")
	}

	err := CallAITransaction(cfg, "test 502 backoff", callAI, func(*AIResponse) error {
		t.Fatal("post handler must not run after an API error")
		return nil
	})
	require.ErrorContains(t, err, "502")
	require.Equal(t, 6, callCount)
	require.Equal(t, []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
	}, waits, "the final failed attempt must not add a useless sleep")
}

func TestCallAITransaction_PostHandlerErrorUsesExponentialBackoff(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 4

	var waits []time.Duration
	cfg.retryWait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	callCount := 0
	err := CallAITransaction(cfg, "test post-handler backoff", func(*AIRequest) (*AIResponse, error) {
		callCount++
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}, func(*AIResponse) error {
		return fmt.Errorf("invalid model action")
	})

	require.ErrorContains(t, err, "invalid model action")
	require.Equal(t, 4, callCount)
	require.Equal(t, []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
	}, waits)
}

func TestCallAITransaction_429KeepsRetryAfterBackoff(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 1

	var waits []time.Duration
	cfg.retryWait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	callCount := 0
	err := CallAITransaction(cfg, "test 429 backoff", func(*AIRequest) (*AIResponse, error) {
		callCount++
		if callCount == 1 {
			return make429Response("Retry-After: 10"), fmt.Errorf("HTTP 429: rate limited")
		}
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}, func(*AIResponse) error { return nil })

	require.NoError(t, err)
	require.Equal(t, 2, callCount, "retryable 429 must not consume the transaction attempt")
	require.Len(t, waits, 1)
	require.GreaterOrEqual(t, waits[0], 10*time.Second)
	require.LessOrEqual(t, waits[0], 12*time.Second)
}

func TestCallAITransaction_BackoffStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 6
	cfg.retryWait = waitBeforeAIRetryDefault

	callCount := 0
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	err := CallAITransaction(cfg, "test canceled backoff", func(*AIRequest) (*AIResponse, error) {
		callCount++
		return make502Response(), fmt.Errorf("HTTP 502: bad gateway")
	}, func(*AIResponse) error { return nil })

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, callCount, "cancellation during backoff must prevent the next request")
	require.Less(t, time.Since(start), time.Second)
}

func TestAIRequestWrapper_502UsesExponentialBackoff(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.AiAutoRetry = 6

	var waits []time.Duration
	cfg.aiRetryWaitFunc = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	callCount := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		callCount++
		return make502Response(), fmt.Errorf("HTTP 502: bad gateway")
	}, consts.TierIntelligent)
	request := NewAIRequest("test request backoff")
	request.SetDetachCheckpoint(true)

	_, err := callback(cfg, request)
	require.ErrorContains(t, err, "502")
	require.Equal(t, 6, callCount)
	require.Equal(t, []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
	}, waits)
}

func TestWaitBeforeAIRetryDefault_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := waitBeforeAIRetryDefault(ctx, 32*time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestConvertConfigToOptions_PreservesAIRetryWaitFunc(t *testing.T) {
	var waited time.Duration
	waitFunc := func(ctx context.Context, delay time.Duration) error {
		waited = delay
		return ctx.Err()
	}

	parent := NewConfig(context.Background(), WithAIRetryWaitFunc(waitFunc))
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.NoError(t, child.waitBeforeAIRetry(context.Background(), 8*time.Second))
	require.Equal(t, 8*time.Second, waited)
}

func TestNewConfig_PreservesAIRetryWaitFuncFromContext(t *testing.T) {
	var waited time.Duration
	waitFunc := func(ctx context.Context, delay time.Duration) error {
		waited = delay
		return ctx.Err()
	}

	parent := NewConfig(context.Background(), WithAIRetryWaitFunc(waitFunc))
	child := NewConfig(parent.GetContext())
	require.NoError(t, child.waitBeforeAIRetry(context.Background(), 16*time.Second))
	require.Equal(t, 16*time.Second, waited)
}

func make502Response() *AIResponse {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: application/json\r\n\r\n"),
		[]byte(`{"error":"bad gateway"}`),
	)
	return rsp
}
