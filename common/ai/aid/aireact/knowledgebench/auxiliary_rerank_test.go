package knowledgebench

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestLLMRerankAuxiliaryResultErrorAndSkip(t *testing.T) {
	for _, mode := range []string{"success", "whole-score", "error", "skip"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			invoker := mock.NewMockInvoker(ctx)
			cause := errors.New("reranker unavailable")
			cfg := invoker.GetConfig().(*mock.MockedAIConfig)
			cfg.ScheduleAuxiliaryTaskFunc = func(_ context.Context, name string, build func() string, onResult func(*aicommon.Action), opts ...aicommon.AuxiliaryTaskOption) {
				require.Equal(t, aicommon.CallerLabelLLMRerank, name)
				if mode == "skip" {
					return
				}
				require.Contains(t, build(), "query")
				spec := &aicommon.AuxiliaryTaskSpec{}
				for _, opt := range opts {
					opt(spec)
				}
				if mode == "error" {
					require.NotNil(t, spec.OnError)
					spec.OnError(cause)
					return
				}
				reply := `{"@action":"llm-rerank","scores":[{"index":2,"score":0.9}]}`
				if mode == "whole-score" {
					reply = `{"@action":"llm-rerank","scores":[{"index":2,"score":1}]}`
				}
				action, err := aicommon.ExtractAction(reply, name)
				require.NoError(t, err)
				onResult(action)
			}
			candidates := []*RerankCandidate{{EntryID: "first"}, {EntryID: "second"}}
			result, err := LLMRerankTopK(ctx, invoker, "query", candidates, 1)
			switch mode {
			case "error":
				require.ErrorIs(t, err, cause)
				require.Nil(t, result)
			case "skip":
				require.NoError(t, err)
				require.Equal(t, candidates[:1], result)
				require.Zero(t, candidates[1].RerankScore)
			case "success", "whole-score":
				require.NoError(t, err)
				require.Equal(t, candidates[1:], result)
				expectedScore := 0.9
				if mode == "whole-score" {
					expectedScore = 1
				}
				require.Equal(t, expectedScore, candidates[1].RerankScore)
			}
		})
	}
}
