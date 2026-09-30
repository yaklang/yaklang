package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Execute the real registered forge synchronously. Dispatch lifecycle is covered
// separately; these tests must finish the plan before inspecting prompt counts.
func testForgePromptMarkers(t *testing.T, steps int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nonce := utils.RandStringBytes(16)
	persistentMarker := "PERSISTENT_UNIQUE_MARKER_" + nonce
	initMarker := "INIT_UNIQUE_MARKER_" + nonce
	queryMarker := "USER_QUERY_MARKER_" + nonce
	tasks := make([]map[string]string, steps)
	for i := range tasks {
		tasks[i] = map[string]string{"subtask_name": fmt.Sprintf("step-%d", i), "subtask_goal": "verify prompt markers"}
	}
	plan, err := json.Marshal(map[string]any{
		"@action": "plan_from_document", "main_task": "verify markers", "main_task_goal": "verify prompt markers", "tasks": tasks,
	})
	require.NoError(t, err)
	forge := &schema.AIForge{
		ForgeName:        "test_forge_markers_" + nonce,
		ForgeVerboseName: "Prompt marker test",
		ForgeType:        schema.FORGE_TYPE_Config,
		InitPrompt:       initMarker + " {{ .Forge.UserParams }}",
		PersistentPrompt: persistentMarker,
	}
	db := consts.GetGormProfileDatabase()
	require.NoError(t, yakit.CreateAIForge(db, forge))
	t.Cleanup(func() {
		_, err := yakit.DeleteAIForge(db, &ypb.AIForgeFilter{ForgeName: forge.ForgeName})
		require.NoError(t, err)
	})

	var mu sync.Mutex
	decisions := 0
	maxPersistent := 0
	sawInit, sawQuery := false, false
	_, err = aicommon.ExecuteForgeFromDB(forge.ForgeName, ctx, map[string]any{"query": queryMarker},
		aicommon.WithAgreeYOLO(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithPeriodicVerificationInterval(0),
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithAIRetryWaitFunc(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			mu.Lock()
			maxPersistent = max(maxPersistent, strings.Count(prompt, persistentMarker))
			sawInit = sawInit || strings.Contains(prompt, initMarker)
			sawQuery = sawQuery || strings.Contains(prompt, queryMarker)
			if isNextActionDecisionPrompt(prompt) {
				decisions++
			}
			mu.Unlock()
			if rsp, err := tryHandleNewPlanFlowPrompt(cfg, prompt, string(plan)); rsp != nil {
				return rsp, err
			}
			response := `{"@action":"finish","reason":"markers verified"}`
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
	require.NoError(t, err, "forge execution must complete successfully")
	require.NoError(t, ctx.Err(), "forge execution must not consume its timeout")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, steps, decisions, "every planned task must execute")
	require.Equal(t, 1, maxPersistent, "persistent content must be present exactly once per prompt")
	require.True(t, sawInit, "forge initialization must be rendered")
	require.True(t, sawQuery, "the original query must reach the forge")
}

func TestForge_PersistentContentOnlyOnce(t *testing.T) {
	testForgePromptMarkers(t, 1)
}

func TestForge_PersistentAndInitNotDuplicated(t *testing.T) {
	testForgePromptMarkers(t, 2)
}

func TestCoordinator_PlanPrompt_OnlyInPlanPhase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	marker := "PLAN_PROMPT_UNIQUE_MARKER_" + utils.RandStringBytes(16)
	var mu sync.Mutex
	reviewed := false
	before, after := 0, 0
	foundBefore, foundAfter := false, false
	coordinator, err := aid.NewCoordinatorContext(ctx, "verify plan prompt phases",
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
