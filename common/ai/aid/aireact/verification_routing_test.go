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
	boundary := strings.LastIndex(receivedPrompt, "# Verification response boundary")
	require.Greater(t, boundary, strings.Index(receivedPrompt, "pending-audit-marker"),
		"the read-only response boundary must follow the task and its TODO data")
	require.Contains(t, receivedPrompt[boundary:], `"verify-satisfaction"`)
	require.Equal(t, 1, speedCalls)
	require.Zero(t, otherCalls, "progress observation must not consume the main model when a speed model is configured")
}

func TestVerifyUserSatisfaction_RejectsBusinessActionAndRetriesOnSpeedModel(t *testing.T) {
	var speedCalls int
	react, err := NewTestReAct(
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAITransactionAutoRetry(2),
		aicommon.WithSpeedPriorityAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			speedCalls++
			raw := `{"@action":"directly_call_tool","tool_name":"write_file","user_satisfied":true}`
			if speedCalls > 1 {
				raw = `{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"Observed work remains incomplete","evidence":[]}`
			}
			response := cfg.NewAIResponse()
			response.EmitOutputStream(strings.NewReader(raw))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)
	result, err := react.VerifyUserSatisfaction(context.Background(), "audit-goal", true, "sources-still-pending")
	require.NoError(t, err)
	require.Equal(t, 2, speedCalls, "a business action must be rejected, not accepted as a verification")
	require.NotNil(t, result)
	require.False(t, result.Satisfied)
	require.Equal(t, "Observed work remains incomplete", result.Reasoning)
}
