package aicommon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestRequestWrapperAuthenticationFailureIsTerminal(t *testing.T) {
	cfg := newConfig(context.Background())
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"), []byte(`{"error":{"message":"invalid credentials"}}`))
		rsp.Close()
		return rsp, fmt.Errorf("401 unauthorized")
	}, consts.TierIntelligent)
	req := NewAIRequest("test")
	req.SetDetachCheckpoint(true)
	_, err := callback(cfg, req)
	require.ErrorContains(t, err, "401")
	require.Equal(t, 1, calls)
}

// All three model tiers use aispec recovery; Agent validation sees only success.
func TestTierCallbacks429RecoverAfterRepeatedCooldowns(t *testing.T) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: true})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
		for _, detached := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/detached=%t", tier, detached), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				cfg := NewTestConfig(ctx)
				var cooldowns []time.Duration
				require.NoError(t, WithAIRetryWaitFunc(func(ctx context.Context, delay time.Duration) error {
					cooldowns = append(cooldowns, delay)
					return ctx.Err()
				})(cfg))
				cfg.AiAutoRetry = 1
				calls := 0
				cb := func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
					calls++
					require.Equal(t, string(tier), req.GetModelTier())
					if calls == 1 {
						rsp := make429Response("Retry-After: 1")
						rsp.Close()
						return rsp, nil
					}
					rsp := NewUnboundAIResponse()
					rsp.Close()
					return rsp, nil
				}
				var callAI func(*AIRequest) (*AIResponse, error)
				switch tier {
				case consts.TierIntelligent:
					require.NoError(t, WithQualityPriorityAICallback(cb)(cfg))
					callAI = cfg.CallQualityPriorityAI
				case consts.TierLightweight:
					require.NoError(t, WithSpeedPriorityAICallback(cb)(cfg))
					callAI = cfg.CallSpeedPriorityAI
				case consts.TierVision:
					require.NoError(t, WithVisionPriorityAICallback(cb)(cfg))
					callAI = func(req *AIRequest) (*AIResponse, error) {
						return cfg.GetVisionPriorityAICallback()(cfg, req)
					}
				}
				parsed := 0
				err := CallAITransaction(cfg, "retry after cooldown", func(req *AIRequest) (*AIResponse, error) {
					req.SetDetachCheckpoint(detached)
					return callAI(req)
				}, func(*AIResponse) error { parsed++; return nil })
				require.NoError(t, err)
				require.Equal(t, 2, calls)
				require.Equal(t, 1, parsed)
				require.Len(t, cooldowns, 1)
				for _, delay := range cooldowns {
					require.GreaterOrEqual(t, delay, time.Second)
					require.LessOrEqual(t, delay, 4*time.Second)
				}
			})
		}
	}
}
