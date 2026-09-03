package aicommon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAICallbackInfrastructureError_TransportMarkers(t *testing.T) {
	// 引擎日志实证的失败形态：base.go "request post to" 包裹的 TLS 拨号失败，
	// 以及未被包裹直接冒泡的 dial / connectex / 中文 TLS 错误。
	for _, msg := range []string{
		"request post to https://api.example.com/v1/chat: TLS连接失败(api.example.com): remote error: tls: handshake failure",
		"dial tcp 10.0.0.8:443: connectex: No connection could be made because the target machine actively refused it.",
		"dial tcp 10.0.0.8:443: i/o timeout",
		"TLS连接失败(api.example.com): certificate is valid for example.io, not api.example.com",
	} {
		require.True(t, isAICallbackInfrastructureError(errors.New(msg)), "expected transport marker match: %s", msg)
	}

	// 非传输类回调错误不得命中。
	for _, msg := range []string{
		"provider rejected empty api key",
		"model tier not configured",
	} {
		require.False(t, isAICallbackInfrastructureError(errors.New(msg)), "unexpected transport marker match: %s", msg)
	}
}

func TestIsTransportCausedEmptyResponse(t *testing.T) {
	t.Run("nil callback error is not transport", func(t *testing.T) {
		rsp := NewUnboundAIResponse()
		require.False(t, isTransportCausedEmptyResponse(nil, rsp))
	})
	t.Run("nil response is not transport", func(t *testing.T) {
		require.False(t, isTransportCausedEmptyResponse(errors.New("dial tcp: refused"), nil))
	})
	t.Run("infra callback error with zero output is transport", func(t *testing.T) {
		rsp := NewUnboundAIResponse()
		err := fmt.Errorf("request post to https://api.example.com: dial tcp 10.0.0.8:443: connectex: refused")
		require.True(t, isTransportCausedEmptyResponse(err, rsp))
	})
	t.Run("non-infra callback error with zero output is not transport", func(t *testing.T) {
		rsp := NewUnboundAIResponse()
		err := errors.New("provider rejected empty api key")
		require.False(t, isTransportCausedEmptyResponse(err, rsp))
	})
}

// TestCallAITransaction_TransportEmptyResponseKeepsNetworkBudget 复现引擎日志
// 中的误分类链：TLS 拨号失败 → 异步 SetError → postHandler 解析空流报
// "action resolution failed"。修复后该失败不得消耗 format 预算，必须走满
// 网络重试预算，并使用传输类长退避。
func TestCallAITransaction_TransportEmptyResponseKeepsNetworkBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3
	cfg.formatRetry = 2 // 若仍误分类，format 预算会在第 2 次尝试后提前终止

	var mu sync.Mutex
	waits := []time.Duration{}
	cfg.retryWait = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		return nil
	}

	var calls int64
	err := CallAITransaction(cfg, "prompt", func(req *AIRequest) (*AIResponse, error) {
		atomic.AddInt64(&calls, 1)
		rsp := NewAIResponse(nil)
		go func() {
			defer rsp.markCallbackDone()
			// 模拟 AIChatToAICallbackType 的异步错误路径：HTTP 层 TLS 失败。
			rsp.SetError(fmt.Errorf(
				"request post to https://ai-gateway.local/v1/chat: TLS连接失败(ai-gateway.local): dial tcp 10.0.0.8:443: connectex: connection refused"))
		}()
		return rsp, nil
	}, func(rsp *AIResponse) error {
		return errors.New(`action resolution failed: requested="<missing>"; reason=no @action emitted`)
	})

	require.Error(t, err)
	// 网络预算（3）必须用满——format 预算（2）不再被传输错误消耗。
	require.Equal(t, int64(3), atomic.LoadInt64(&calls),
		"transport-caused empty responses must burn the full network retry budget, not the format budget")
	// 最终错误以传输类信息为主因。
	assert.True(t, strings.Contains(err.Error(), "request post to") || strings.Contains(err.Error(), "connectex"),
		"final error should surface the transport cause, got: %s", err.Error())
	// 传输类退避：10s、20s（而非格式类的 2s）。
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []time.Duration{10 * time.Second, 20 * time.Second}, waits,
		"transport failures should use the longer transport backoff schedule")
}

// TestCallAITransaction_FormatErrorKeepsShortBackoff 确认真实格式错误（有
// postHandler 解析失败、无回调基础设施错误）保持原行为：format 预算提前停
// 且使用 2s 起步的短退避。
func TestCallAITransaction_FormatErrorKeepsShortBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 5
	cfg.formatRetry = 2

	var mu sync.Mutex
	waits := []time.Duration{}
	cfg.retryWait = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		return nil
	}

	var calls int64
	err := CallAITransaction(cfg, "prompt", func(req *AIRequest) (*AIResponse, error) {
		atomic.AddInt64(&calls, 1)
		rsp := NewUnboundAIResponse()
		rsp.Close()
		return rsp, nil
	}, func(rsp *AIResponse) error {
		return errors.New(`action resolution failed: requested="<missing>"; reason=no @action emitted`)
	})

	require.Error(t, err)
	require.Equal(t, int64(2), atomic.LoadInt64(&calls), "format retry should stop at formatRetry=2")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []time.Duration{2 * time.Second}, waits,
		"format failures should keep the short backoff schedule")
}

func TestAITransportRetryBackoffSchedule(t *testing.T) {
	require.Equal(t, time.Duration(0), aiTransportRetryBackoff(0))
	require.Equal(t, 10*time.Second, aiTransportRetryBackoff(1))
	require.Equal(t, 20*time.Second, aiTransportRetryBackoff(2))
	require.Equal(t, 40*time.Second, aiTransportRetryBackoff(3))
	require.Equal(t, 60*time.Second, aiTransportRetryBackoff(4))
	require.Equal(t, 60*time.Second, aiTransportRetryBackoff(9))
}
