package aireact

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestMiniAITasksUseConfigLiteCall(t *testing.T) {
	cases := []struct {
		name      string
		handler   MiniAITaskHandler
		input     map[string]any
		output    map[string]any
		resultKey string
	}{
		{aicommon.CallerLabelMiniPromptOptimize, handlePromptOptimize,
			map[string]any{"prompt": "scan target"}, map[string]any{"optimized_prompt": "scan specific target", "reason": "clarified"}, "optimized_prompt"},
		{aicommon.CallerLabelMiniTimelineSummary, handleTimelineSummary,
			nil, map[string]any{"summary": "completed reconnaissance", "key_points": []any{"recon"}}, "summary"},
		{aicommon.CallerLabelMiniTodoDraft, handleTodoDraft,
			map[string]any{"user_input": "scan target"}, map[string]any{"todo_text": "scan and verify", "target": "target", "source": "user", "acceptance_criteria": "report"}, "todo_text"},
	}
	for _, test := range cases {
		for _, mode := range []string{"normal", "single-model", "failure"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				type requestContextKey struct{}
				ctx = context.WithValue(ctx, requestContextKey{}, test.name)
				var speedCalls, qualityCalls atomic.Int32
				cfg := aicommon.NewConfig(ctx,
					aicommon.WithDisableAutoSkills(true),
					aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithAIAutoRetry(1),
					aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithSingleAIModelMode(mode == "single-model"),
					aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						qualityCalls.Add(1)
						return nil, errors.New("unexpected intelligence call")
					}),
					aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						speedCalls.Add(1)
						var options aispec.AIConfig
						for _, option := range req.GetExtraSpecOpts() {
							option(&options)
						}
						require.Empty(t, options.ThinkingLevel, "custom Speed callback keeps its configured parameters")
						require.Equal(t, "liteforge["+test.name+"]", req.GetCallerLabel())
						require.Equal(t, test.name, req.GetContext().Value(requestContextKey{}))
						if mode == "failure" {
							return nil, errors.New("mini task provider failed")
						}
						return mockSpeedActionAI(test.name, test.output)(c, req)
					}),
				)
				// Built-in handlers must work with Config alone, without a ReAct caller.
				result, err := test.handler(ctx, &MiniAITaskContext{Config: cfg, Timeline: cfg.GetTimeline()}, test.input)
				require.EqualValues(t, 1, speedCalls.Load(), "LiteCall must execute rather than skip")
				require.Zero(t, qualityCalls.Load())
				require.Equal(t, aicommon.SingleModelRun, aicommon.GetSingleModelAction(test.name))
				if mode == "failure" {
					require.ErrorContains(t, err, "mini task provider failed")
					require.Nil(t, result)
				} else {
					require.NoError(t, err)
					data, ok := result.(map[string]any)
					require.True(t, ok)
					require.Equal(t, test.output[test.resultKey], data[test.resultKey])
				}
			})
		}
	}
}
