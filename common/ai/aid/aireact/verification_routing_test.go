package aireact

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestVerifyUserSatisfaction_PrefersSpeedModelAndPreservesContext(t *testing.T) {
	var speedCalls, otherCalls int
	var receivedPrompt string
	callback := func(speed bool) aicommon.AICallbackType {
		return func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if speed {
				speedCalls++
			} else {
				otherCalls++
			}
			require.Equal(t, "verification", req.GetCallerLabel())
			receivedPrompt = req.GetPrompt()
			response := cfg.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"Model thinks the task is complete","evidence":[]}`))
			response.Close()
			return response, nil
		}
	}
	react, err := NewTestReAct(
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAICallback(callback(false)),
		aicommon.WithQualityPriorityAICallback(callback(false)),
		aicommon.WithSpeedPriorityAICallback(callback(true)),
	)
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("audit-task", "audit-goal", context.Background(), react.Emitter, true)
	react.SetCurrentTask(task)
	scope := aicommon.BuildVerificationTodoScope(task)
	require.False(t, scope.IsZero())
	results := react.config.ApplyTodoDelta(scope, &aicommon.TodoDelta{Add: []aicommon.TodoAdd{{ID: "pending-audit", Text: "pending-audit-marker"}}})
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
	react.AddToTimeline("tool", "timeline-observation-marker")
	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{ID: "observed", Op: "add", Content: "evidence-observation-marker"}})
	result, err := react.VerifyUserSatisfaction(context.Background(), "audit-goal", true, "sources-still-pending")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Satisfied)
	require.Contains(t, result.Reasoning, "[OVERRIDE]", "open TODOs must still override a model's premature satisfaction claim")
	for _, marker := range []string{"audit-goal", "sources-still-pending", "timeline-observation-marker", "evidence-observation-marker", "pending-audit-marker"} {
		if !strings.Contains(receivedPrompt, marker) {
			t.Errorf("missing verification context material %q", marker)
		}
	}
	require.Equal(t, 1, speedCalls)
	require.Zero(t, otherCalls, "progress observation must not consume the main model when a speed model is configured")
}
