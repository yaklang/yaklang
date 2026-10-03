package aireact

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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise the actual input event loop and queue. In particular, Yakit waits
// for the planning task's terminal status before sending detached approval.
func TestCoordinatorApprovalContinuesThroughInputQueue(t *testing.T) {
	for _, mode := range []string{"default_detached", "default_detached_edited", "default_continue", "legacy_alias", "plan_alias", "detached", "detached_edited", "detached_interrupt", "continue", "edited"} {
		t.Run(mode, func(t *testing.T) {
			detached, edited := strings.Contains(mode, "detached"), strings.HasSuffix(mode, "edited")
			defaultEntry := strings.HasPrefix(mode, "default_")
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			out := make(chan *schema.AiOutputEvent, 4096)
			var mu sync.Mutex
			workers := map[string]int{}
			accepted, submittedAgain := 0, false
			pattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+)`)
			model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				if wire.ToolCallCallback == nil || wire.FinishReasonCallback == nil {
					return nil, fmt.Errorf("unexpected non-native request: %s", req.GetCallerLabel())
				}
				respond := func(name string, args any) (*aicommon.AIResponse, error) {
					data, _ := json.Marshal(args)
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("approval-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
					wire.FinishReasonCallback("tool_calls", nil)
					response := c.NewAIResponse()
					response.Close()
					return response, nil
				}
				prompt := req.GetPrompt()
				if defaultEntry && strings.Contains(req.GetCallerLabel(), "default") {
					return respond("request_plan_and_execution", map[string]any{"plan_request_payload": "Plan and execute two dependent checks"})
				}
				if strings.Contains(prompt, "执行已批准的冻结任务书。") {
					if strings.Contains(prompt, "PLANNER ONLY PREFERENCE") {
						return nil, fmt.Errorf("planning preferences leaked into worker role")
					}
					if edited && strings.Contains(prompt, `"First"`) && !strings.Contains(prompt, "USER APPROVED EDIT") {
						return nil, fmt.Errorf("worker did not receive the approved edit")
					}
					id := req.GetTaskIndex()
					workers[id]++
					if workers[id] == 1 {
						return respond("submit_task_result", map[string]any{"summary": "Approved task completed"})
					}
					return respond("finish", map[string]any{})
				}
				if strings.Contains(prompt, "Draft version: 0") {
					if !strings.Contains(prompt, "PLANNER ONLY PREFERENCE") {
						return nil, fmt.Errorf("planner preference was lost at the default-loop handoff")
					}
					tasks := []any{
						map[string]any{"name": "First", "goal": "Check the source", "identifier": "first", "depends_on": []string{}},
						map[string]any{"name": "Second", "goal": "Verify the accepted first result", "identifier": "second", "depends_on": []string{"first"}},
					}
					if edited {
						tasks = append(tasks, map[string]any{"name": "Removed", "goal": "User will remove this task", "identifier": "removed", "depends_on": []string{}})
					}
					return respond("create_plan", map[string]any{"plan": map[string]any{"name": "Single approval", "goal": "Run both tasks after one approval", "tasks": tasks}, "plan_document": "# Execute both tasks after approval"})
				}
				if strings.Contains(prompt, "Detached submitted version:") {
					if mode == "detached_interrupt" {
						<-c.GetContext().Done()
						return nil, c.GetContext().Err()
					}
					return respond("finish", map[string]any{})
				}
				if strings.Contains(prompt, "approved version: 0") {
					return respond("submit_plan", map[string]any{"plan_version": 1})
				}
				// Repeated submission of an already approved version must not
				// force the user to approve the same plan a second time.
				if !submittedAgain {
					submittedAgain = true
					return respond("submit_plan", map[string]any{"plan_version": 1})
				}
				matches := pattern.FindAllStringSubmatch(prompt, -1)
				for _, m := range matches {
					if m[2] == "awaiting_review" {
						attempt, _ := strconv.Atoi(m[3])
						accepted++
						return respond("review_task", map[string]any{"task_id": m[1], "attempt_id": attempt, "decision": "accept", "reason": "The inspected result satisfies the approved task."})
					}
				}
				for _, m := range matches {
					if m[2] == "running" {
						return respond("wait_tasks", map[string]any{"timeout_seconds": 1})
					}
				}
				for _, m := range matches {
					if m[2] == "pending" {
						return respond("start_tasks", map[string]any{})
					}
				}
				return respond("finish", map[string]any{})
			}
			focus := coordinator.Name
			if defaultEntry {
				focus = ""
			}
			if mode == "legacy_alias" {
				focus = "coordinator_legacy"
			}
			if mode == "plan_alias" {
				focus = "plan"
			}
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithFocus(focus), aicommon.WithEnableFunctionCallMode(true),
				aicommon.WithPlanPrompt("PLANNER ONLY PREFERENCE: create two dependent checks"),
				aicommon.WithPersistentSessionId(uuid.NewString()), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEnableDetachedPlan(detached), aicommon.WithForceManualPlanReview(true),
				aicommon.WithAgreeYOLO(), aicommon.WithAICallback(model), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					copy := *e
					copy.Content = append([]byte(nil), e.Content...)
					select {
					case out <- &copy:
					case <-ctx.Done():
					}
				}))
			require.NoError(t, err)
			require.NoError(t, r.config.GetDB().AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
			require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Plan and execute two dependent checks"}))
			var owner, parentTask, executionTask string
			panels, starts, ends, pushes, pops := 0, 0, 0, 0, 0
			var approval *ypb.AIInputEvent
			completed := false
			for !completed {
				select {
				case <-ctx.Done():
					t.Fatalf("approval did not advance: panels=%d starts=%d pushes=%d pops=%d", panels, starts, pushes, pops)
				case e := <-out:
					var data map[string]any
					_ = json.Unmarshal(e.Content, &data)
					if e.NodeId == "react_task_dequeue" && parentTask == "" {
						parentTask, _ = data["react_task_id"].(string)
						require.Equal(t, parentTask, e.TaskId, "Yakit uses TaskId as the active question ID")
					}
					if e.NodeId == "react_task_dequeue" && strings.HasPrefix(fmt.Sprint(data["react_task_id"]), recoveryTaskIDPrefix) {
						executionTask, _ = data["react_task_id"].(string)
						require.Contains(t, data["react_task_input"], "执行已批准计划")
						require.NotContains(t, data["react_task_input"], "恢复执行")
					}
					switch e.Type {
					case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE:
						require.Zero(t, pushes, "no execution before approval")
						panels++
						if panels > 1 {
							t.Fatal("a single approved plan requested another user submission")
						}
						plans := data["plans"].(map[string]any)
						root := plans["root_task"].(map[string]any)
						if edited {
							children := root["subtasks"].([]any)
							children[0].(map[string]any)["goal"] = "USER APPROVED EDIT: check the source"
							children[2].(map[string]any)["isRemove"] = true
						}
						if detached {
							owner, _ = data["coordinator_id"].(string)
							params := map[string]any{"coordinator_id": owner}
							if edited {
								plans["document"] = "# User approved document"
								params["plans"] = plans
							}
							input, _ := json.Marshal(params)
							approval = &ypb.AIInputEvent{IsSyncMessage: true, SyncType: SYNC_TYPE_EXECUTE_DETACHED_PLAN, SyncID: "one-click", SyncJsonInput: string(input)}
							if mode == "detached_interrupt" {
								require.NotEmpty(t, parentTask)
								require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{IsSyncMessage: true, SyncType: "react_cancel_task", SyncID: "stop-planning", SyncJsonInput: fmt.Sprintf(`{"task_id":%q}`, parentTask)}))
							}
						} else {
							require.Equal(t, true, data["force_manual_review"], "manual review policy must reach the coordinator")
							owner = e.CoordinatorId
							params := map[string]any{"suggestion": "continue"}
							if edited {
								params["suggestion"] = "freedom-review"
								params["reviewed-task-tree"] = root
							}
							input, _ := json.Marshal(params)
							require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: fmt.Sprint(data["id"]), InteractiveJSONInput: string(input)}))
						}
					case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
						starts++
					case schema.EVENT_TYPE_END_PLAN_AND_EXECUTION:
						ends++
					case schema.EVENT_TYPE_TASK_REVIEW_REQUIRE:
						t.Fatal("task acceptance must not require another user submission")
					}
					if e.NodeId == "system" {
						if data["type"] == "push_task" {
							pushes++
						}
						if data["type"] == "pop_task" {
							pops++
						}
					}
					status := data["react_task_now_status"]
					if e.NodeId == "react_task_status_changed" && (status == "completed" || status == "aborted" || status == "skipped") {
						id, _ := data["react_task_id"].(string)
						if detached && id == parentTask && approval != nil {
							require.Zero(t, pushes, "no execution before approval")
							require.NoError(t, r.SendInputEvent(approval))
							approval = nil
						} else if (!detached && id == parentTask) || (detached && id == executionTask) {
							require.False(t, e.IsSync)
							require.NotEqual(t, owner, e.CoordinatorId, "Yakit must receive the outer task's terminal status")
							require.Equal(t, "completed", status)
							completed = true
						}
					}
					if e.NodeId == "execute_detached_plan" && data["error"] != nil {
						t.Fatalf("approval failed: %s", e.Content)
					}
				}
			}
			require.Equal(t, 1, panels)
			require.Equal(t, 1, starts)
			require.Equal(t, 1, ends)
			require.Equal(t, 2, pushes)
			require.Equal(t, 2, pops)
			mu.Lock()
			workerCount, acceptedCount := len(workers), accepted
			mu.Unlock()
			require.Equal(t, 2, workerCount)
			require.Equal(t, 2, acceptedCount)
			for _, input := range r.config.GetUserInputHistory() {
				require.NotContains(t, input.UserInput, "执行已批准计划")
			}
			require.NotContains(t, collectTimelineText(r), "执行已批准计划")
			record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(r.config.GetDB(), owner)
			require.NoError(t, err)
			require.Contains(t, record.TaskProgress, `"finished":true`)
			if edited {
				require.Contains(t, record.TaskTree, "USER APPROVED EDIT")
				require.NotContains(t, record.TaskTree, "User will remove this task")
				if detached {
					require.Contains(t, record.TaskProgress, "# User approved document")
				}
			}
		})
	}
}
