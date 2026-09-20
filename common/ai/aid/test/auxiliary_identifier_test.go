package test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/aiforge"
)

func TestSemanticIdentifierAuxiliaryProtocolAndFallback(t *testing.T) {
	for _, mode := range []string{"success", "failure", "single-model", "short-name"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var calls, qualityCalls atomic.Int32
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAIAutoRetry(1),
				aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithSingleAIModelMode(mode == "single-model"),
				aicommon.WithQualityPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					qualityCalls.Add(1)
					return nil, errors.New("unexpected Intelligence call")
				}),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls.Add(1)
					require.Equal(t, "liteforge[task-short-id]", req.GetCallerLabel())
					require.Contains(t, req.GetPrompt(), `{"@action":"task-short-id","identifier":"YOUR_IDENTIFIER"}`)
					require.NotContains(t, req.GetPrompt(), `{"@action":"object","identifier":"YOUR_IDENTIFIER"}`)
					if mode == "failure" {
						return nil, errors.New("identifier unavailable")
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(`{"@action":"task-short-id","identifier":"review_code"}`))
					response.Close()
					return response, nil
				}),
			)
			coordinator := &aid.Coordinator{Config: cfg}
			plan := `{"@action":"plan","main_task":"Review code and produce a complete verified security report","main_task_goal":"report","tasks":[]}`
			if mode == "short-name" {
				plan = `{"@action":"plan","main_task":"review_code","main_task_goal":"report","tasks":[]}`
			}
			task, err := aid.ExtractTaskFromRawResponse(coordinator, plan)
			require.NoError(t, err)
			require.NotNil(t, task)
			require.Zero(t, qualityCalls.Load())
			if mode == "success" || mode == "short-name" {
				require.Equal(t, "review_code", task.GetSemanticIdentifier())
			} else {
				require.NotEmpty(t, task.GetSemanticIdentifier())
				require.LessOrEqual(t, len([]rune(task.GetSemanticIdentifier())), 20)
				require.NotEqual(t, "review_code", task.GetSemanticIdentifier())
			}
			if mode == "single-model" || mode == "short-name" {
				require.Zero(t, calls.Load())
			} else {
				require.EqualValues(t, 1, calls.Load())
			}
		})
	}
}
