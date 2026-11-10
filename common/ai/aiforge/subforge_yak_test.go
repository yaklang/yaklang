package aiforge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSubForgeYakExecution(t *testing.T) {
	level := log.GetLevel()
	log.SetLevel(log.WarnLevel)
	t.Cleanup(func() { log.SetLevel(level) })
	code, err := os.ReadFile("testdata/subforge.yak")
	require.NoError(t, err)
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			parentID := "sub-forge-test-" + uuid.NewString()
			workdir := t.TempDir()
			input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
			var childReviews, childReleases atomic.Int32
			var mu sync.Mutex
			workerSteps := map[string]int{}
			childParamsSeen := false
			parentUsedChildResult := false
			var lastChildEndpoint string
			requests := 0
			model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				requests++
				if requests > 50 {
					return nil, fmt.Errorf("sub-Forge fixture failed to converge")
				}
				id := c.GetRuntimeId()
				var session *coordinator.Session
				for _, candidate := range coordinator.GetRunningSessions() {
					if candidate.Id == id {
						session = candidate
					}
				}
				if session == nil {
					return nil, fmt.Errorf("unexpected call %q from %q", req.GetCallerLabel(), id)
				}
				child := id != parentID
				prompt := req.GetPrompt()
				state := session.Snapshot()
				if req.GetCallerLabel() == "react-loop:pe_task" {
					step := workerSteps[id]
					workerSteps[id]++
					if child {
						if strings.Contains(prompt, "[DECLARED_SUB_FORGES]") || !strings.HasPrefix(c.GetOrCreateWorkDir(), filepath.Join(workdir, "artifacts", "sub-forges")+string(os.PathSeparator)) {
							return nil, fmt.Errorf("child execution escaped its tool or work directory scope")
						}
						if step == 0 {
							return forgeResponse(c, req, native, "submit_task_result", map[string]any{"summary": "INVENTORY_CHILD_EVIDENCE"})
						}
					} else {
						if step == 0 {
							if !strings.Contains(prompt, "sub_forge_inventory") || !strings.Contains(prompt, "sub_forge_glossary") {
								return nil, fmt.Errorf("worker did not receive both candidates")
							}
							return forgeResponse(c, req, native, "directly_call_tool", map[string]any{
								"directly_call_tool_name":   "sub_forge_inventory",
								"directly_call_tool_params": map[string]any{"query": "inventory-ref"},
								"directly_call_reason":      "Inspect the inventory through the bound specialist.",
							})
						}
						if step == 1 {
							if !strings.Contains(prompt, "INVENTORY_CHILD_EVIDENCE") {
								return nil, fmt.Errorf("parent worker did not receive the child result")
							}
							parentUsedChildResult = true
							return forgeResponse(c, req, native, "submit_task_result", map[string]any{"summary": "PARENT_USED_INVENTORY_CHILD_EVIDENCE"})
						}
					}
					return forgeResponse(c, req, native, "finish", map[string]any{})
				}
				if state.Plan == nil {
					if strings.Contains(prompt, "[DECLARED_SUB_FORGES]") {
						return nil, fmt.Errorf("coordinator received worker-only tools")
					}
					if child {
						if !strings.Contains(prompt, "Inspect the requested inventory") ||
							!strings.Contains(prompt, "<|PARAM_limit_|>\n3\n") ||
							!strings.Contains(prompt, "inventory-ref") {
							return nil, fmt.Errorf("child did not receive its typed parameters and default")
						}
						childParamsSeen = true
					}
					var plan map[string]any
					_ = json.Unmarshal([]byte(`{"main_task":"Inventory brief","main_task_goal":"Inspect evidence","tasks":[{"subtask_name":"Produce evidence","subtask_goal":"Verify input and provide a result","subtask_identifier":"evidence","depends_on":[]}]}`), &plan)
					return forgeResponse(c, req, native, "create_plan", map[string]any{"plan": plan, "plan_document": "# Verify inventory evidence"})
				}
				if state.Phase == coordinator.PhasePlan {
					return forgeResponse(c, req, native, "submit_plan", map[string]any{})
				}
				for _, attempt := range state.Attempts {
					if attempt.State == coordinator.AwaitingReview {
						return forgeResponse(c, req, native, "review_task", map[string]any{"task_id": attempt.Task.ID, "attempt_id": attempt.ID, "decision": "accept", "reason": "Verified the completed evidence."})
					}
				}
				return forgeResponse(c, req, native, "wait_messages", map[string]any{})
			}
			engine := yak.NewScriptEngine(1)
			engine.RegisterEngineHooks(func(vm *antlr4yak.Engine) error {
				vm.SetVars(map[string]any{"VERIFY_SUB_FORGE": func(execution *aiforge.ForgeExecution) {
					require.True(t, execution.Snapshot().Finished)
					require.Contains(t, fmt.Sprint(execution.Result().Formated), "PARENT_USED_INVENTORY_CHILD_EVIDENCE")
				}})
				yak.BindAIConfigToEngine(vm, yak.WithContext(ctx), aicommon.WithID(parentID),
					aicommon.WithWorkdir(workdir), aicommon.WithAICallback(model), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAgreeYOLO(), aicommon.WithForceManualPlanReview(true),
					aicommon.WithNoOpMemoryTriage(), aicommon.WithDisablePerception(true),
					aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEventInputChanx(input),
					aicommon.WithMaxIterationCount(30), aicommon.WithDisableIncreaseIteration(true),
					aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
						endpoint := event.GetInteractiveId()
						if event.Type == schema.EVENT_TYPE_REVIEW_RELEASE && event.CoordinatorId != parentID && endpoint != "" && event.CallToolID != "" {
							childReleases.Add(1)
						}
						if event.Type != schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
							return
						}
						if event.CoordinatorId != parentID {
							if event.TaskId == "" || event.CallToolID == "" {
								t.Error("child review event lost its parent task or tool call")
							}
							childReviews.Add(1)
							mu.Lock()
							lastChildEndpoint = endpoint
							mu.Unlock()
						}
						input.SafeFeed(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: endpoint, InteractiveJSONInput: `{"suggestion":"continue"}`})
					}),
				)
				return nil
			})
			_, err := engine.ExecuteExWithContext(ctx, string(code)+"\nVERIFY_SUB_FORGE(execution)\n", nil)
			require.NoError(t, err)
			require.EqualValues(t, 1, childReviews.Load())
			require.Positive(t, childReleases.Load())
			mu.Lock()
			require.True(t, childParamsSeen)
			require.True(t, parentUsedChildResult)
			require.NotEmpty(t, lastChildEndpoint)
			t.Logf("Yak entry completed with native=%t, child arguments and default observed, one child review routed, and parent consumed the child result", native)
			mu.Unlock()
		})
	}
}
