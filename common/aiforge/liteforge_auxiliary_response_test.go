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
	"github.com/yaklang/yaklang/common/consts"
)

func TestAuxiliaryCustomCallbackOptionsAreRequestLocal(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	consts.SetTieredAIConfig(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var roles, thinking []string
	callback := func(role string) aicommon.AICallbackType {
		return func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			var opts aispec.AIConfig
			for _, opt := range req.GetExtraSpecOpts() {
				opt(&opts)
			}
			roles = append(roles, role)
			thinking = append(thinking, opts.ThinkingLevel)
			response := c.NewAIResponse()
			response.EmitOutputStream(strings.NewReader("ok"))
			response.Close()
			return response, nil
		}
	}
	cfg := aicommon.NewConfig(ctx, aicommon.WithSingleAIModelMode(true),
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithSpeedPriorityAICallback(callback("speed")),
		aicommon.WithQualityPriorityAICallback(callback("quality")))
	// Reuse the same caller-owned options across LiteCall and ordinary calls.
	extra := []aispec.AIConfigOption{aispec.WithThinkingLevel("high")}
	shared := []aicommon.GeneralKVConfigOption{
		aicommon.WithGeneralConfigExtraRequestOpts(aicommon.WithAIRequest_ExtraSpecOpts(extra...)),
	}
	invoke := func(name string) {
		var gotError error
		results := 0
		cfg.ScheduleAuxiliaryTask(ctx, name, func() string { return "unchanged prompt" },
			func(*aicommon.Action) { results++ }, aicommon.WithAuxiliaryOpts(shared...),
			aicommon.WithAuxiliaryOnError(func(err error) { gotError = err }),
			aicommon.WithAuxiliaryResponseHandler(func(response *aicommon.AIResponse) (*aicommon.Action, error) {
				body, err := io.ReadAll(response.GetOutputStreamReader("test", true, cfg.GetEmitter()))
				require.NoError(t, err)
				require.Equal(t, "ok", string(body))
				return aicommon.NewSimpleAction("done", nil), nil
			}))
		require.NoError(t, gotError)
		require.Equal(t, 1, results)
	}
	invoke(aicommon.CallerLabelMiniTodoDraft)
	invoke(aicommon.CallerLabelGoalAcceptanceReview)
	response, err := cfg.CallQualityPriorityAI(aicommon.NewAIRequest("normal quality", aicommon.WithAIRequest_ExtraSpecOpts(extra...)))
	require.NoError(t, err)
	_, err = io.ReadAll(response.GetOutputStreamReader("test", true, cfg.GetEmitter()))
	require.NoError(t, err)
	require.NoError(t, response.GetError())
	cfg = aicommon.NewConfig(ctx, aicommon.WithSingleAIModelMode(false), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithSpeedPriorityAICallback(callback("speed")), aicommon.WithQualityPriorityAICallback(callback("quality")))
	invoke(aicommon.CallerLabelMiniTodoDraft) // The same task in multi-model mode preserves high.
	require.Equal(t, []string{"speed", "speed", "quality", "speed"}, roles)
	require.Equal(t, []string{"high", "high", "high", "high"}, thinking)
	var original aispec.AIConfig
	for _, opt := range extra {
		opt(&original)
	}
	require.Equal(t, "high", original.ThinkingLevel)
}

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
					require.Empty(t, opts.ThinkingLevel, "custom Speed callback keeps its configured parameters")
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
