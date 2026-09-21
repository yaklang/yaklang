package aireact

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

func TestGoalAcceptanceUsesAuxiliaryConfig(t *testing.T) {
	for _, name := range []string{"pass", "reject", "single-model", "error", "malformed", "disabled", "empty-criteria", "sub-agent"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			type contextKey struct{}
			ctx = context.WithValue(ctx, contextKey{}, "goal-review-context")
			var speedCalls, intelligenceCalls atomic.Int32
			criteria := "must deliver a verified report"
			if name == "empty-criteria" {
				criteria = ""
			}
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAIAutoRetry(1),
				aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithEnableGoalMode(name != "disabled"),
				aicommon.WithGoalAcceptanceCriteria(criteria),
				aicommon.WithSingleAIModelMode(name == "single-model"),
				aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					intelligenceCalls.Add(1)
					return nil, errors.New("unexpected intelligence call")
				}),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					speedCalls.Add(1)
					require.Equal(t, "liteforge[goal-acceptance-review]", req.GetCallerLabel())
					require.Equal(t, "goal-review-context", req.GetContext().Value(contextKey{}))
					require.Contains(t, req.GetPrompt(), criteria)
					require.Contains(t, req.GetPrompt(), "(no timeline content available)")
					var options aispec.AIConfig
					for _, option := range req.GetExtraSpecOpts() {
						option(&options)
					}
					if name == "single-model" {
						require.Equal(t, "none", options.ThinkingLevel)
					} else {
						require.Empty(t, options.ThinkingLevel)
					}
					if name == "error" {
						return nil, errors.New("review unavailable")
					}
					if name == "malformed" {
						response := c.NewAIResponse()
						response.EmitOutputStream(strings.NewReader("{"))
						response.Close()
						return response, nil
					}
					return mockSpeedActionAI(aicommon.CallerLabelGoalAcceptanceReview, map[string]any{
						"passed": name == "pass",
						"reason": "  missing verified report  ",
					})(c, req)
				}),
			)
			// The runtime mock has a different Config; execution must use the
			// loop's owning Config rather than the old runtime LiteForge method.
			loop := reactloops.NewMinimalReActLoop(cfg, mock.NewMockInvoker(ctx))
			if name == "sub-agent" {
				loop.Set(reactloops.SubAgentDepthLoopVar, 1)
			}
			result := loop.CheckGoalAcceptanceCriteria(ctx)
			require.NotNil(t, result)
			require.Zero(t, intelligenceCalls.Load())
			require.Equal(t, aicommon.SingleModelRun, aicommon.GetSingleModelAction(aicommon.CallerLabelGoalAcceptanceReview))
			switch name {
			case "disabled", "empty-criteria", "sub-agent":
				require.Zero(t, speedCalls.Load())
				require.True(t, result.Passed)
			case "error", "malformed":
				require.EqualValues(t, 1, speedCalls.Load())
				require.True(t, result.Passed, "preserve fail-open behavior")
				require.Empty(t, result.Reason)
			default:
				require.EqualValues(t, 1, speedCalls.Load())
				require.Equal(t, name == "pass", result.Passed)
				require.Equal(t, "missing verified report", result.Reason)
			}
		})
	}
}
