package aicommon

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

func useGlobalOutputRecovery(t *testing.T, enabled bool) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: enabled})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
}

func TestModelOutputRecoveryBeyondRetryBudget(t *testing.T) {
	useGlobalOutputRecovery(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 1
	calls, validated := 0, 0
	callback := AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		calls++
		if calls < 8 {
			return "no action", nil
		}
		return `{"@action":"finish"}`, nil
	})
	err := CallAITransaction(cfg, "fixture", func(req *AIRequest) (*AIResponse, error) { return callback(cfg, req) }, func(rsp *AIResponse) error {
		_, err := ExtractValidActionFromStream(ctx, rsp.GetOutputStreamReader("fixture", true, cfg.GetEmitter()), "finish")
		if err == nil {
			validated++
		}
		return err
	})
	require.NoError(t, err)
	require.Equal(t, 8, calls)
	require.Equal(t, 1, validated)
}

func TestModelOutputRecoveryCancellationAndNonProtocolErrors(t *testing.T) {
	t.Run("stop while waiting", func(t *testing.T) {
		useGlobalOutputRecovery(t, true)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg := newTransactionTestConfig(ctx)
		cfg.retryWait = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
		calls := 0
		err := CallAITransaction(cfg, "fixture", func(*AIRequest) (*AIResponse, error) {
			calls++
			rsp := NewUnboundAIResponse()
			return rsp, nil
		}, func(*AIResponse) error { return NewModelOutputValidationError(errors.New("missing action")) })
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, calls)
	})
	t.Run("other failures remain bounded", func(t *testing.T) {
		useGlobalOutputRecovery(t, true)
		cfg := newTransactionTestConfig(context.Background())
		cfg.retryMax = 1
		calls := 0
		err := CallAITransaction(cfg, "fixture", func(*AIRequest) (*AIResponse, error) {
			calls++
			rsp := NewUnboundAIResponse()
			return rsp, nil
		}, func(*AIResponse) error { return errors.New("tool execution failed") })
		require.Error(t, err)
		require.Equal(t, 1, calls)
	})
	t.Run("opt in required", func(t *testing.T) {
		useGlobalOutputRecovery(t, false)
		cfg := newTransactionTestConfig(context.Background())
		cfg.retryMax = 1
		calls := 0
		err := CallAITransaction(cfg, "fixture", func(*AIRequest) (*AIResponse, error) { calls++; return NewUnboundAIResponse(), nil }, func(*AIResponse) error { return NewModelOutputValidationError(errors.New("missing action")) })
		require.Error(t, err)
		require.Equal(t, 1, calls)
	})
}

func TestModelOutputValidationTag(t *testing.T) {
	_, err := ExtractValidActionFromStream(context.Background(), strings.NewReader("plain answer"), "finish")
	require.True(t, IsModelOutputValidationError(err))
	require.False(t, IsModelOutputValidationError(errors.New("tool failed")))
}

func TestGlobalModelOutputRecovery(t *testing.T) {
	old := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	for _, enabled := range []bool{true, false} {
		consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: enabled})
		cfg := newTransactionTestConfig(context.Background())
		cfg.retryMax = 1
		calls, executions := 0, 0
		err := CallAITransaction(cfg, "fixture", func(*AIRequest) (*AIResponse, error) {
			calls++
			rsp := NewUnboundAIResponse()
			return rsp, nil
		}, func(*AIResponse) error {
			if calls < 9 {
				return NewModelOutputValidationError(errors.New("invalid action"))
			}
			executions++
			return nil
		})
		if enabled {
			require.NoError(t, err)
			require.Equal(t, 9, calls)
			require.Equal(t, 1, executions)
		} else {
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.Zero(t, executions)
		}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: true})
	cfg := newTransactionTestConfig(context.Background())
	cfg.retryMax = 1
	calls := 0
	err := CallAITransaction(cfg, "fixture", func(*AIRequest) (*AIResponse, error) { return NewUnboundAIResponse(), nil }, func(*AIResponse) error { calls++; return errors.New("local tool execution failed") })
	require.Error(t, err)
	require.Equal(t, 1, calls, "global model recovery must not endlessly replay tools")
}
