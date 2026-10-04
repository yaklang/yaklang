package test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_PlanPrompt_OnlyInPlanPhase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	marker := "PLAN_PROMPT_UNIQUE_MARKER_" + utils.RandStringBytes(16)
	var mu sync.Mutex
	reviewed := false
	before, after := 0, 0
	foundBefore, foundAfter := false, false
	coordinator, err := coordinator_legacy.NewCoordinatorContext(ctx, "verify plan prompt phases",
		aicommon.WithAgreeYOLO(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithPlanPrompt(marker),
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithAIRetryWaitFunc(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				mu.Lock()
				reviewed = true
				mu.Unlock()
			}
		}),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			mu.Lock()
			if reviewed {
				after++
				foundAfter = foundAfter || strings.Contains(prompt, marker)
			} else {
				before++
				foundBefore = foundBefore || strings.Contains(prompt, marker)
			}
			mu.Unlock()
			plan := `{"@action":"plan_from_document","main_task":"test plan","main_task_goal":"finish test","tasks":[{"subtask_name":"test step","subtask_goal":"finish test"}]}`
			if rsp, err := tryHandleNewPlanFlowPrompt(cfg, prompt, plan); rsp != nil {
				return rsp, err
			}
			response := `{"@action":"finish","reason":"test complete"}`
			if isVerifySatisfactionPrompt(prompt) {
				response = `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"done"}`
			} else if isSummaryPrompt(prompt) {
				response = `{"@action":"summary","task_summary":"done","task_short_summary":"done","task_long_summary":"done"}`
			}
			rsp := cfg.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(response))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	require.NoError(t, coordinator.Run())
	require.NoError(t, ctx.Err())
	mu.Lock()
	defer mu.Unlock()
	require.True(t, reviewed, "the plan review phase must run")
	require.Positive(t, before)
	require.Positive(t, after, "task execution must run after plan review")
	require.True(t, foundBefore, "PlanPrompt must be rendered during planning")
	require.False(t, foundAfter, "PlanPrompt must not leak into task execution")
}
