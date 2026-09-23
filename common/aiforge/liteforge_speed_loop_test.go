package aiforge

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Exercise a complete loop round against the real Config -> LiteForge bridge,
// not a scheduler mock. The only mock is the provider's emitted response.
func TestSpeedLoopLiteForgeIntegration(t *testing.T) {
	for _, mode := range []string{"text", "functioncall", "single-model", "retry", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var speedCalls, qualityCalls, verifications, handled atomic.Int32
			var observed *aicommon.Action
			var activeTask aicommon.AIStatefulTask
			var firstPrompt string
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisablePerception(true), aicommon.WithAIAutoRetry(1),
				aicommon.WithAITransactionAutoRetry(2),
				aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
				aicommon.WithSingleAIModelMode(mode == "single-model"),
				aicommon.WithEnableFunctionCallMode(mode == "functioncall"),
				aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					qualityCalls.Add(1)
					return nil, errors.New("unexpected Intelligence call")
				}),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					attempt := speedCalls.Add(1)
					require.Equal(t, "react-loop:auxiliary-integration", req.GetCallerLabel())
					if mode == "single-model" {
						require.Equal(t, activeTask.GetContext().Done(), req.GetContext().Done())
					} else {
						require.Same(t, activeTask.GetContext(), req.GetContext())
					}
					require.Equal(t, mode == "functioncall", req.IsToolCallArgumentsStreamEnabled())
					var opts aispec.AIConfig
					for _, option := range req.GetExtraSpecOpts() {
						option(&opts)
					}
					require.Empty(t, opts.ThinkingLevel, "custom Speed callback keeps its configured parameters")
					if mode == "functioncall" {
						require.NotEmpty(t, opts.Tools)
					}
					if attempt == 1 {
						firstPrompt = req.GetPrompt()
						require.Equal(t, 1, strings.Count(firstPrompt, "perform auxiliary round"))
						require.Contains(t, firstPrompt, "# Response Schema")
					} else {
						require.True(t, strings.HasPrefix(req.GetPrompt(), firstPrompt))
						require.Contains(t, req.GetPrompt(), "reject first attempt")
					}
					if mode == "cancel" {
						cancel()
						return nil, req.GetContext().Err()
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"action":"accept","human_readable_thought":"ready"}` + "\n<|BODY_CURRENT_NONCE|>accepted body<|BODY_END_CURRENT_NONCE|>"))
					response.Close()
					return response, nil
				}),
			)
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			loop, err := reactloops.NewReActLoop("auxiliary-integration", invoker,
				reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
				reactloops.WithRegisterLoopAction("require_tool", "unused tool route", nil, nil, nil),
				reactloops.WithUseSpeedPriorityAICallback(true),
				reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithMaxIterations(1),
				reactloops.WithAITagField("BODY", "body"),
				reactloops.WithRegisterLoopAction("accept", "accept the result", nil,
					func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
						if verifications.Add(1) == 1 && mode == "retry" {
							return errors.New("reject first attempt")
						}
						return nil
					},
					func(_ *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
						observed = action
						handled.Add(1)
						operator.Exit()
					}),
			)
			require.NoError(t, err)
			activeTask = aicommon.NewStatefulTaskBase("speed-loop-task", "perform auxiliary round", ctx, cfg.GetEmitter())
			err = loop.ExecuteWithExistedTask(activeTask)
			require.Zero(t, qualityCalls.Load())
			if mode == "cancel" {
				require.Error(t, err)
				require.Zero(t, handled.Load())
				require.EqualValues(t, 1, speedCalls.Load())
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, 1, handled.Load())
			require.NotNil(t, observed)
			require.Equal(t, "accepted body", observed.GetString("body"))
			require.Contains(t, loop.Get("last_ai_decision_response"), "BODY_CURRENT_NONCE")
			if mode == "retry" {
				require.EqualValues(t, 2, speedCalls.Load())
			} else {
				require.EqualValues(t, 1, speedCalls.Load())
			}
		})
	}
}
