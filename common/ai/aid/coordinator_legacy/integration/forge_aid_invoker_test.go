package test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	_ "github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Execute the real registered forge synchronously. Dispatch lifecycle is covered
// separately; these tests must finish the plan before inspecting prompt counts.
func testForgePromptMarkers(t *testing.T, steps int, native bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nonce := utils.RandStringBytes(16)
	persistentMarker := "PERSISTENT_UNIQUE_MARKER_" + nonce
	initMarker := "INIT_UNIQUE_MARKER_" + nonce
	queryMarker := "USER_QUERY_MARKER_" + nonce
	tasks := make([]map[string]string, steps)
	for i := range tasks {
		tasks[i] = map[string]string{"subtask_name": fmt.Sprintf("step-%d", i), "subtask_goal": "verify prompt markers", "subtask_identifier": fmt.Sprintf("step_%d", i)}
	}
	plan := map[string]any{"main_task": "verify markers", "main_task_goal": "verify prompt markers", "tasks": tasks}
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
	calls, decisions := 0, 0
	workers := map[string]int{}
	briefPattern := regexp.MustCompile(`\[CURRENT_EXECUTION\]\n([^\n]+)`)
	reviewPattern := regexp.MustCompile(`\[([^\]]+)\]: awaiting_review; attempt=(\d+)`)
	maxPersistent := 0
	sawInit, sawQuery := false, false
	_, err := aicommon.ExecuteForgeFromDB(forge.ForgeName, ctx, map[string]any{"query": queryMarker},
		aicommon.WithEnableFunctionCallMode(native),
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
			defer mu.Unlock()
			calls++
			if calls > 16+8*steps {
				return nil, fmt.Errorf("Forge marker fixture did not converge: %s", req.GetCallerLabel())
			}
			maxPersistent = max(maxPersistent, strings.Count(prompt, persistentMarker))
			sawInit = sawInit || strings.Contains(prompt, initMarker)
			sawQuery = sawQuery || strings.Contains(prompt, queryMarker)
			respond := func(name string, args map[string]any) (*aicommon.AIResponse, error) {
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				if (wire.ToolCallCallback != nil) != native {
					return nil, fmt.Errorf("Forge lost its configured function-call mode")
				}
				rsp := cfg.NewAIResponse()
				if native {
					raw, err := json.Marshal(args)
					if err != nil {
						return nil, err
					}
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("forge-markers-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
					wire.FinishReasonCallback("tool_calls", nil)
				} else {
					args["@action"], args["identifier"] = name, name
					raw, err := json.Marshal(args)
					if err != nil {
						return nil, err
					}
					rsp.EmitOutputStream(strings.NewReader(string(raw)))
				}
				rsp.Close()
				return rsp, nil
			}
			// The DB Forge entry now runs the native coordinator and pe_task;
			// legacy planning prompt matchers cannot drive this lifecycle.
			if brief := briefPattern.FindStringSubmatch(prompt); len(brief) == 2 {
				var task struct {
					ID string `json:"task_id"`
				}
				if err := json.Unmarshal([]byte(brief[1]), &task); err != nil {
					return nil, err
				}
				step := workers[task.ID]
				workers[task.ID]++
				if step == 0 {
					decisions++
					return respond("submit_task_result", map[string]any{"summary": "markers verified"})
				}
				return respond("finish", map[string]any{})
			}
			if req.GetCallerLabel() == "react-loop:coordinator" {
				start := strings.LastIndex(prompt, "# PLAN STATUS")
				if start < 0 {
					return nil, fmt.Errorf("Forge coordinator lost its plan status")
				}
				status := prompt[start:]
				if strings.Contains(status, "已有计划：false") {
					return respond("create_plan", map[string]any{"plan": plan, "plan_document": "# Prompt markers\nVerify each task's markers."})
				}
				if strings.Contains(status, "阶段：PLAN") {
					return respond("submit_plan", map[string]any{})
				}
				if match := reviewPattern.FindStringSubmatch(status); len(match) == 3 {
					attempt, err := strconv.ParseUint(match[2], 10, 64)
					if err != nil {
						return nil, err
					}
					return respond("review_task", map[string]any{"task_id": match[1], "attempt_id": attempt, "decision": "accept", "reason": "markers verified"})
				}
				return respond("wait_messages", map[string]any{})
			}
			return nil, fmt.Errorf("unexpected Forge marker request: %s", req.GetCallerLabel())
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
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", native), func(t *testing.T) { testForgePromptMarkers(t, 1, native) })
	}
}

func TestForge_PersistentAndInitNotDuplicated(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", native), func(t *testing.T) { testForgePromptMarkers(t, 2, native) })
	}
}

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
