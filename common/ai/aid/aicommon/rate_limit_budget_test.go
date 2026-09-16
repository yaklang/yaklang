package aicommon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestRateLimitBudgetNestedRetries(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return make429ResponseWithBody([]string{"Retry-After: 1"}, `{"error":{"code":"providers_circuit_open","message":"all upstreams unavailable"}}`), nil
	}, consts.TierIntelligent)
	txn := newTransactionTestConfig(context.Background())
	err := CallAITransaction(txn, "test", func(req *AIRequest) (*AIResponse, error) {
		req.SetDetachCheckpoint(true)
		return callback(cfg, req)
	}, func(*AIResponse) error { t.Fatal("provider failure must not reach content parser"); return nil })
	var rateErr *AIRateLimitError
	require.ErrorAs(t, err, &rateErr)
	require.Equal(t, aiRateLimitMaxAttempts, calls, "outer transaction must not reset wrapper budget")
	require.True(t, rateErr.BudgetExceeded)
	require.Equal(t, `"providers_circuit_open"`, string(rateErr.Code))
	require.Equal(t, 7*time.Second, rateErr.WaitDuration)
}

func TestRateLimitBudgetTransactionOnly(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(fmt.Sprint(recovery), func(t *testing.T) {
			cfg := newTransactionTestConfig(context.Background())
			calls := 0
			err := CallAITransaction(cfg, "test", func(*AIRequest) (*AIResponse, error) {
				calls++
				if recovery && calls == 4 {
					rsp := NewUnboundAIResponse()
					rsp.Close()
					return rsp, nil
				}
				return make429Response("Retry-After: 1"), fmt.Errorf("circuit open")
			}, func(*AIResponse) error { return nil })
			if recovery {
				require.NoError(t, err)
				require.Equal(t, 4, calls)
			} else {
				var rateErr *AIRateLimitError
				require.ErrorAs(t, err, &rateErr)
				require.Equal(t, aiRateLimitMaxAttempts, calls)
			}
		})
	}
}

func TestRateLimitBudgetHonorsLongCooldownAndCancel(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error {
		t.Fatal("must not wait or shorten cooldown beyond budget")
		return nil
	}
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return make429Response("Retry-After: 180"), nil
	}, consts.TierIntelligent)
	req := NewAIRequest("test")
	req.SetDetachCheckpoint(true)
	_, err := callback(cfg, req)
	var rateErr *AIRateLimitError
	require.ErrorAs(t, err, &rateErr)
	require.True(t, rateErr.BudgetExceeded)
	require.Equal(t, 1, calls)

	ctx, cancel := context.WithCancel(context.Background())
	cfg = newConfig(ctx)
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	callback = cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) { return make429Response("Retry-After: 1"), nil }, consts.TierIntelligent)
	_, err = callback(cfg, req)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTransaction429WithoutCallbackErrorDoesNotParseContent(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	calls := 0
	err := CallAITransaction(cfg, "test", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := make429Response("Retry-After: 1")
		rsp.Close()
		return rsp, nil
	}, func(*AIResponse) error { t.Fatal("429 body must not reach action parser"); return nil })
	var rateErr *AIRateLimitError
	require.ErrorAs(t, err, &rateErr)
	require.Equal(t, aiRateLimitMaxAttempts, calls)
}

func TestRetryAfterHTTPDate(t *testing.T) {
	rsp := make429Response("Retry-After: " + time.Now().Add(90*time.Second).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	seconds := parseRetryAfterSeconds(rsp, 0)
	require.GreaterOrEqual(t, seconds, 89)
	require.LessOrEqual(t, seconds, 90)
}

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

// Exercise the public tier configuration paths, including the checkpoint branch
// of Config.wrapper. A terminal callback error must not restart in Transaction.
func TestRateLimitBudgetTierCallbacks(t *testing.T) {
	for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
		for _, detached := range []bool{false, true} {
			for _, recoverOutput := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/detached=%t/recover=%t", tier, detached, recoverOutput), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					cfg := NewTestConfig(ctx)
					var waits []time.Duration
					cfg.aiRetryWaitFunc = func(_ context.Context, delay time.Duration) error {
						waits = append(waits, delay)
						return nil
					}
					calls := 0
					cb := func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
						calls++
						require.Equal(t, string(tier), req.GetModelTier())
						if recoverOutput && calls == 3 {
							rsp := NewUnboundAIResponse()
							rsp.Close()
							return rsp, nil
						}
						return make429Response("Retry-After: 1"), nil
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
					err := CallAITransaction(cfg, "tier retry", func(req *AIRequest) (*AIResponse, error) {
						req.SetDetachCheckpoint(detached)
						return callAI(req)
					}, func(rsp *AIResponse) error { parsed++; return nil })
					if recoverOutput {
						require.NoError(t, err)
						require.Equal(t, 3, calls)
						require.Equal(t, 1, parsed)
					} else {
						var rateErr *AIRateLimitError
						require.ErrorAs(t, err, &rateErr)
						require.True(t, rateErr.BudgetExceeded)
						require.Equal(t, aiRateLimitMaxAttempts, calls)
						require.Zero(t, parsed)
					}
					require.Len(t, waits, calls-1, "only the callback should wait for each 429")
					for _, delay := range waits {
						require.Equal(t, time.Second, delay)
					}
				})
			}
		}
	}
}

func TestRateLimitBudgetSurvivesOutputRepair(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		if calls == 4 {
			rsp := NewUnboundAIResponse()
			rsp.Close()
			return rsp, nil
		}
		return make429Response("Retry-After: 1"), nil
	}, consts.TierIntelligent)
	parsed := 0
	err := CallAITransaction(newTransactionTestConfig(context.Background()), "test", func(req *AIRequest) (*AIResponse, error) {
		req.SetDetachCheckpoint(true)
		return callback(cfg, req)
	}, func(*AIResponse) error {
		parsed++
		return fmt.Errorf("invalid action, repair required")
	})
	var rateErr *AIRateLimitError
	require.ErrorAs(t, err, &rateErr)
	require.Equal(t, 1, parsed)
	require.Equal(t, aiRateLimitMaxAttempts+1, calls, "output repair must retain earlier 429 attempts")
	require.Equal(t, aiRateLimitMaxAttempts, rateErr.Attempts)
}

func TestRateLimitBudgetIndependentTransactions(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error { return nil }
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return make429Response("Retry-After: 1"), nil
	}, consts.TierIntelligent)
	for run := 0; run < 2; run++ {
		err := CallAITransaction(newTransactionTestConfig(context.Background()), "test", func(req *AIRequest) (*AIResponse, error) {
			req.SetDetachCheckpoint(true)
			return callback(cfg, req)
		}, func(*AIResponse) error { t.Fatal("429 is not model output"); return nil })
		var rateErr *AIRateLimitError
		require.ErrorAs(t, err, &rateErr)
		require.Equal(t, aiRateLimitMaxAttempts, rateErr.Attempts)
		require.Equal(t, (run+1)*aiRateLimitMaxAttempts, calls)
	}
}

func TestRateLimitBudgetRawCallbackLegacyQueue(t *testing.T) {
	cfg := newTransactionTestConfig(context.Background())
	var waits []time.Duration
	cfg.retryWait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	calls := 0
	err := CallAITransaction(cfg, "legacy queue", func(*AIRequest) (*AIResponse, error) {
		calls++
		if calls == 3 {
			rsp := NewUnboundAIResponse()
			rsp.Close()
			return rsp, nil
		}
		return make429Response("X-AIBalance-Info: 2"), fmt.Errorf("queue busy")
	}, func(*AIResponse) error { return nil })
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{6 * time.Second, 6 * time.Second}, waits)
}

func TestRateLimitBudgetTerminalResponseStillNotifies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		headers    []string
		notifyType string
		exhausted  bool
	}{
		{"quota", nil, notify429TypeQuotaExceeded, false},
		{"cooldown beyond budget", []string{"Retry-After: 180"}, notify429TypeRateLimited, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, snapshot := newTestConfigForHandle429WithEvents(context.Background())
			cfg.aiRetryWaitFunc = func(context.Context, time.Duration) error {
				t.Fatal("terminal responses must not wait")
				return nil
			}
			callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
				return make429ResponseWithBody(tc.headers, `{"error":{"message":"upstream unavailable"}}`), nil
			}, consts.TierIntelligent)
			_, err := callback(cfg, NewAIRequest("test", WithAIRequest_DetachCheckpoint()))
			var rateErr *AIRateLimitError
			require.ErrorAs(t, err, &rateErr)
			require.Equal(t, tc.exhausted, rateErr.BudgetExceeded)
			cfg.Emitter.WaitForStream()
			payload := requireNotifyPayload(t, snapshot())
			require.Equal(t, tc.notifyType, payload["type"])
			require.Contains(t, payload["content"], "upstream unavailable")
			require.EqualValues(t, 0, payload["duration"])
		})
	}
}

func TestRateLimitBudgetCumulativeWait(t *testing.T) {
	cfg := newConfig(context.Background())
	var waits []time.Duration
	cfg.aiRetryWaitFunc = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	calls := 0
	callback := cfg.wrapper(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
		calls++
		return make429Response("Retry-After: 45"), nil
	}, consts.TierIntelligent)
	_, err := callback(cfg, NewAIRequest("test", WithAIRequest_DetachCheckpoint()))
	var rateErr *AIRateLimitError
	require.ErrorAs(t, err, &rateErr)
	require.True(t, rateErr.BudgetExceeded)
	require.Equal(t, 3, calls)
	require.Equal(t, 90*time.Second, rateErr.WaitDuration)
	require.Equal(t, []time.Duration{45 * time.Second, 45 * time.Second}, waits)
}
