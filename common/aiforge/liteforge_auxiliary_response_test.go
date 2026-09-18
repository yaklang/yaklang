package aiforge

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestAuxiliaryResponseHandlerTransaction(t *testing.T) {
	for _, mode := range []string{"normal", "retry", "invalid", "nil-action", "cancel", "skip", "lite-call"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var calls, parses, results, qualityCalls atomic.Int32
			var receivedErr error
			invalid := errors.New("response protocol rejected")
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(2),
				aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
				aicommon.WithSingleAIModelMode(mode == "skip" || mode == "lite-call"),
				aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					qualityCalls.Add(1)
					return nil, errors.New("unexpected Intelligence call")
				}),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					attempt := calls.Add(1)
					if attempt == 1 {
						require.Equal(t, "complete protocol prompt", req.GetPrompt(), "must not wrap or duplicate the loop prompt")
					} else if mode == "nil-action" {
						require.Contains(t, req.GetPrompt(), "returned no action")
					} else {
						require.Contains(t, req.GetPrompt(), invalid.Error())
					}
					var opts aispec.AIConfig
					for _, opt := range req.GetExtraSpecOpts() {
						opt(&opts)
					}
					if mode == "lite-call" {
						require.Equal(t, "none", opts.ThinkingLevel)
					} else {
						require.Empty(t, opts.ThinkingLevel)
					}
					if mode == "cancel" {
						cancel()
						return nil, ctx.Err()
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader("custom protocol response"))
					response.Close()
					return response, nil
				}),
			)
			name := "custom-protocol"
			if mode == "skip" {
				name = aicommon.CallerLabelToolCallReason
			}
			if mode == "lite-call" {
				name = aicommon.CallerLabelMiniTodoDraft
			}
			cfg.ScheduleAuxiliaryTask(ctx, name, func() string {
				require.NotEqual(t, "skip", mode)
				return "complete protocol prompt"
			}, func(action *aicommon.Action) {
				results.Add(1)
				require.Equal(t, "accepted", action.Name())
			}, aicommon.WithAuxiliaryOnError(func(err error) { receivedErr = err }),
				aicommon.WithAuxiliaryResponseHandler(func(response *aicommon.AIResponse) (*aicommon.Action, error) {
					attempt := parses.Add(1)
					body, err := io.ReadAll(response.GetOutputStreamReader("custom", true, cfg.GetEmitter()))
					require.NoError(t, err)
					require.Equal(t, "custom protocol response", string(body))
					if mode == "nil-action" {
						return nil, nil
					}
					if mode == "invalid" || (mode == "retry" && attempt == 1) {
						return nil, invalid
					}
					return aicommon.NewSimpleAction("accepted", nil), nil
				}),
			)
			require.Zero(t, qualityCalls.Load())
			switch mode {
			case "skip":
				require.NoError(t, receivedErr)
				require.Zero(t, calls.Load())
				require.Zero(t, parses.Load())
				require.Zero(t, results.Load())
			case "cancel":
				require.ErrorIs(t, receivedErr, context.Canceled)
				require.EqualValues(t, 1, calls.Load())
				require.Zero(t, results.Load())
			case "invalid", "nil-action":
				require.Error(t, receivedErr)
				require.EqualValues(t, 2, calls.Load())
				require.Zero(t, results.Load())
			default:
				require.NoError(t, receivedErr)
				require.EqualValues(t, 1, results.Load())
				if mode == "retry" {
					require.EqualValues(t, 2, calls.Load())
				} else {
					require.EqualValues(t, 1, calls.Load())
				}
			}
		})
	}
}
