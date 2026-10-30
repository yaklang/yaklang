package aicommon

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

func TestProviderFailureIndependentRetryBudgets(t *testing.T) {
	cfg := NewTestConfig(context.Background())
	cfg.AiAutoRetry = 3
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return nil, &aispec.ModelCallFailure{Kind: "protocol", Cause: io.ErrUnexpectedEOF}
	}, consts.TierIntelligent)
	tc := newTransactionTestConfig(context.Background())
	tc.retryMax = 5
	tc.retryWait = cfg.aiRetryWaitFunc
	err := CallAITransaction(tc, "stream", func(req *AIRequest) (*AIResponse, error) { req.SetDetachCheckpoint(true); return callback(cfg, req) }, func(*AIResponse) error { return nil })
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, 3, calls)
}

func TestProviderFailureAsyncStreamIsNotAgentValidation(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 3
	calls := 0
	err := CallAITransaction(cfg, "stream", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewAIResponse(nil)
		rsp.SetError(io.ErrUnexpectedEOF)
		rsp.Close()
		return rsp, nil
	}, func(*AIResponse) error { t.Fatal("incomplete stream must not enter action parsing"); return nil })
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, 1, calls)
	require.NotContains(t, err.Error(), "missing action")
}

func TestProviderFailureQuotaWithoutRequestWrapper(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 5
	cfg.retryWait = func(context.Context, time.Duration) error { return nil }
	calls := 0
	err := CallAITransaction(cfg, "quota", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewAIResponse(nil)
		rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte(`{"error":{"message":"free-models-per-day"}}`))
		rsp.Close()
		return rsp, nil
	}, func(*AIResponse) error { t.Fatal("quota is not model output"); return nil })
	require.ErrorContains(t, err, "free-models-per-day")
	require.Equal(t, 1, calls)
}

func TestProviderFailureHTTPPreservesTransportCause(t *testing.T) {
	for _, mode := range []string{"direct", "detached", "checkpoint"} {
		for _, responseError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/responseError=%v", mode, responseError), func(t *testing.T) {
				cfg := NewTestConfig(context.Background())
				cfg.AiAutoRetry = 1
				cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
				cb := AICallbackType(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
					rsp := NewAIResponse(nil)
					rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"), []byte(`{"error":{"message":"daily quota"}}`))
					if responseError {
						rsp.SetError(io.ErrUnexpectedEOF)
					}
					rsp.Close()
					if responseError {
						return rsp, nil
					}
					return rsp, io.ErrUnexpectedEOF
				})
				if mode != "direct" {
					cb = cfg.wrapper(cb, consts.TierIntelligent)
				}
				tc := newTransactionTestConfig(context.Background())
				tc.retryMax = 1
				err := CallAITransaction(tc, "quota", func(req *AIRequest) (*AIResponse, error) {
					req.SetDetachCheckpoint(mode == "detached")
					return cb(cfg, req)
				}, func(*AIResponse) error { t.Fatal("must not parse quota"); return nil })
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			})
		}
	}
}

func TestProviderFailureHeaderOnly429Returns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tc := newTransactionTestConfig(ctx)
	tc.retryMax = 1
	done := make(chan error, 1)
	go func() {
		done <- CallAITransaction(tc, "header-only", func(*AIRequest) (*AIResponse, error) {
			rsp := NewAIResponse(nil)
			rsp.SetRawHTTPResponseHeader([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"))
			rsp.Close()
			return rsp, io.ErrUnexpectedEOF
		}, func(*AIResponse) error { return nil })
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("completed callback without body blocked transaction")
	}
}
