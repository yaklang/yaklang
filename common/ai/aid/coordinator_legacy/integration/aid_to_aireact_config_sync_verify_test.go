package test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAIDToAIReact_ConfigSync_AllowAskForClarification_False(t *testing.T) {
	verifyCoordinatorLoopConfig(t, false, true)
}
func TestAIDToAIReact_ConfigSync_AllowPlan_False(t *testing.T) {
	verifyCoordinatorLoopConfig(t, true, false)
}
func TestAIDToAIReact_ConfigSync_Both_False(t *testing.T) {
	verifyCoordinatorLoopConfig(t, false, false)
}
func TestAIDToAIReact_ConfigSync_Both_True(t *testing.T) {
	verifyCoordinatorLoopConfig(t, true, true)
}

// Visit a real PE task's decision prompt and wait for the entire coordinator
// run. A missing prompt or unfinished task must fail, including negative cases.
func verifyCoordinatorLoopConfig(t *testing.T, allowAsk, allowPlan bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 256)
	type observation struct {
		prompt              string
		allowAsk, allowPlan bool
	}
	observations := make(chan observation, 16)
	coordinator, err := coordinator_legacy.NewCoordinatorContext(ctx, "verify coordinator loop configuration",
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithAllowRequireForUserInteract(allowAsk),
		aicommon.WithAllowPlanUserInteract(allowPlan),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			plan := `{"@action":"plan_from_document","main_task":"verify configuration","main_task_goal":"verify child loop config","tasks":[{"subtask_name":"check configuration","subtask_goal":"check interaction settings"}]}`
			if rsp, err := tryHandleNewPlanFlowPrompt(i, prompt, plan); rsp != nil {
				return rsp, err
			}
			output := ""
			switch {
			case aicommon.IsPrimaryDecisionPrompt(prompt):
				config, ok := i.(interface{ SimpleInfoMap() map[string]interface{} })
				if !ok {
					return nil, fmt.Errorf("unexpected loop config type %T", i)
				}
				info := config.SimpleInfoMap()
				observations <- observation{prompt, i.GetAllowUserInteraction(), info["AllowPlanUserInteract"].(bool)}
				output = `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"configuration checked"}`
			case isSummaryPrompt(prompt):
				output = `{"@action":"summary","status_summary":"done","task_short_summary":"checked","task_long_summary":"configuration checked"}`
			case isVerifySatisfactionPrompt(prompt):
				output = `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"checked"}`
			default:
				return nil, fmt.Errorf("unexpected configuration-test prompt: caller=%s", r.GetCallerLabel())
			}
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(output))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- coordinator.Run() }()
	var interactive bool
	checkEvent := func(e *schema.AiOutputEvent) {
		if e.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
			interactive = true
		}
	}
	for {
		select {
		case e := <-events:
			checkEvent(e)
		case err := <-done:
			require.NoError(t, err)
			for len(events) > 0 {
				checkEvent(<-events)
			}
			close(observations)
			var visited int
			for got := range observations {
				visited++
				require.Equal(t, allowAsk, got.allowAsk, "user-interaction policy must reach the executing loop")
				require.Equal(t, allowPlan, got.allowPlan, "plan-interaction policy must reach the executing loop")
				require.Equal(t, allowAsk, strings.Contains(got.prompt, `"ask_for_clarification"`), "actual decision schema must reflect the interaction policy")
				// PE tasks execute an existing plan and intentionally never offer nested
				// request_plan_and_execution, irrespective of the plan-review policy.
				require.NotContains(t, got.prompt, `"request_plan_and_execution"`)
			}
			require.Positive(t, visited, "the real task decision prompt must be visited")
			if !allowAsk {
				require.False(t, interactive)
			}
			return
		case <-ctx.Done():
			t.Fatal("timeout waiting for coordinator execution and configuration observation")
		}
	}
}
