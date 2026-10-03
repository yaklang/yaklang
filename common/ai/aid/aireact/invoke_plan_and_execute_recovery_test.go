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

// Recovery now uses native snapshots; old AiTask records must not reactivate
// the retired PLAN engine, even when the client explicitly selects that focus.
func TestReAct_RecoveryPlanAndExec_RejectsLegacyBeforeQueue(t *testing.T) {
	for _, focus := range []string{"", "coordinator_legacy"} {
		t.Run("focus="+focus, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := make(chan *schema.AiOutputEvent, 100)
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithFocus(focus),
				aicommon.WithPersistentSessionId(uuid.NewString()), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) { out <- e }))
			require.NoError(t, err)
			id := uuid.NewString()
			record := &schema.AISessionPlanAndExec{SessionID: r.config.PersistentSessionId, CoordinatorID: id,
				TaskTree: `{"name":"Old plan","goal":"Must not run","subtasks":[]}`, TaskProgress: `{"phase":"executing"}`}
			require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(r.config.GetDB(), record))
			require.NoError(t, r.HandleSyncTypeRecoveryPlanAndExecEvent(&ypb.AIInputEvent{SyncID: "retired", SyncJsonInput: fmt.Sprintf(`{"coordinator_id":%q}`, id)}))
			require.Empty(t, r.taskQueue.GetQueueingTasks())
			select {
			case e := <-out:
				require.Equal(t, "recover_plan_and_exec", e.NodeId)
				require.Equal(t, "retired", e.SyncID)
				require.Contains(t, string(e.Content), "legacy PLAN execution is disabled")
				require.NotContains(t, string(e.Content), `"started":true`)
			case <-time.After(time.Second):
				t.Fatal("missing explicit legacy rejection")
			}
		})
	}
}

func TestReAct_RecoveryPlanAndExec_NativeSnapshot(t *testing.T) {
	for _, fromTask := range []string{"", "2"} {
		t.Run("start="+fromTask, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			out := make(chan *schema.AiOutputEvent, 4096)
			pattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+); observed=(true|false)`)
			var mu sync.Mutex
			workers := map[string]int{}
			model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				if wire.ToolCallCallback == nil || wire.FinishReasonCallback == nil {
					return nil, fmt.Errorf("non-native recovery: %s", req.GetCallerLabel())
				}
				reply := func(name string, args any) (*aicommon.AIResponse, error) {
					data, _ := json.Marshal(args)
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("recover-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
					wire.FinishReasonCallback("tool_calls", nil)
					rsp := cfg.NewAIResponse()
					rsp.Close()
					return rsp, nil
				}
				prompt := req.GetPrompt()
				if strings.Contains(prompt, "执行已批准的冻结任务书。") {
					index := req.GetTaskIndex()
					workers[index]++
					if workers[index] == 1 {
						return reply("submit_task_result", map[string]any{"summary": "Recovered task verified"})
					}
					return reply("finish", map[string]any{})
				}
				matches := pattern.FindAllStringSubmatch(prompt, -1)
				for _, m := range matches {
					attempt, _ := strconv.Atoi(m[3])
					if m[2] == "failed" {
						return reply("retry_task", map[string]any{"task_id": m[1], "attempt_id": attempt, "reason": "Resume interrupted work"})
					}
					if m[2] == "awaiting_review" {
						if m[4] == "false" {
							return reply("inspect_tasks", map[string]any{})
						}
						return reply("review_task", map[string]any{"task_id": m[1], "attempt_id": attempt, "decision": "accept", "reason": "Result meets the approved brief"})
					}
				}
				for _, m := range matches {
					if m[2] == "running" {
						return reply("wait_tasks", map[string]any{"timeout_seconds": 1})
					}
				}
				for _, m := range matches {
					if m[2] == "pending" {
						return reply("start_tasks", map[string]any{})
					}
				}
				return reply("finish", map[string]any{})
			}
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithPersistentSessionId(uuid.NewString()),
				aicommon.WithEnableFunctionCallMode(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithAgreeYOLO(), aicommon.WithAICallback(model),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					select {
					case out <- e:
					case <-ctx.Done():
					}
				}))
			require.NoError(t, err)
			plan, err := coordinator.ParseReviewedPlan(`{"name":"Recovery","goal":"Resume approved checks","subtasks":[
    {"name":"Done","goal":"Already accepted","semantic_identifier":"done"},
    {"name":"Resume","goal":"Resume interrupted check","semantic_identifier":"resume","depends_on":["done"]},
    {"name":"Verify","goal":"Verify resumed result","semantic_identifier":"verify","depends_on":["resume"]}]}`, "# Approved checks", nil)
			require.NoError(t, err)
			snapshot := coordinator.Snapshot{Schema: 1, DraftVersion: 1, ApprovedVersion: 1, Draft: plan, Approved: plan, NextAttempt: 3, Attempts: map[string]coordinator.Attempt{}}
			for i, task := range plan.Tasks {
				state := coordinator.Accepted
				if fromTask == "" {
					if i == 1 {
						state = coordinator.Failed
					}
					if i == 2 {
						state = coordinator.Pending
					}
				}
				snapshot.Attempts[task.ID] = coordinator.Attempt{Task: task, ID: uint64(i + 1), PlanVersion: 1, State: state, Seen: state == coordinator.Accepted, Result: coordinator.Result{Summary: "Persisted prior result"}}
			}
			snapshot.Finished = fromTask != ""
			progress, _ := json.Marshal(coordinator.Progress{PlanEngine: coordinator.Name, CoordinatorState: &snapshot, Phase: "NotCompleted"})
			id := uuid.NewString()
			record := &schema.AISessionPlanAndExec{SessionID: r.config.PersistentSessionId, CoordinatorID: id, TaskTree: string(plan.Tree), TaskProgress: string(progress)}
			require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(r.config.GetDB(), record))
			input, _ := json.Marshal(map[string]any{"coordinator_id": id, "start_task_id": fromTask})
			require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{IsSyncMessage: true, SyncType: SYNC_TYPE_RECOVERY_PLAN_AND_EXEC, SyncID: "native-recovery", SyncJsonInput: string(input)}))
			starts, ends, panels := 0, 0, 0
			acknowledged, completed := false, false
			for !completed {
				select {
				case <-ctx.Done():
					t.Fatalf("recovery did not finish: starts=%d ends=%d", starts, ends)
				case e := <-out:
					var data map[string]any
					_ = json.Unmarshal(e.Content, &data)
					if e.IsSync && e.SyncID == "native-recovery" {
						require.Nil(t, data["error"])
						require.Equal(t, true, data["started"])
						acknowledged = true
					}
					switch e.Type {
					case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
						starts++
					case schema.EVENT_TYPE_END_PLAN_AND_EXECUTION:
						ends++
					case schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE:
						panels++
					}
					if e.NodeId == "react_task_status_changed" && strings.HasPrefix(fmt.Sprint(data["react_task_id"]), recoveryTaskIDPrefix) {
						status := data["react_task_now_status"]
						if status == "completed" || status == "skipped" || status == "aborted" {
							require.Equal(t, "completed", status)
							completed = true
						}
					}
				}
			}
			require.True(t, acknowledged)
			require.Equal(t, 1, starts)
			require.Equal(t, 1, ends)
			require.Zero(t, panels)
			mu.Lock()
			calls := map[string]int{}
			for k, v := range workers {
				calls[k] = v
			}
			mu.Unlock()
			require.Zero(t, calls[plan.Tasks[0].ID], "accepted upstream must not run again")
			require.Positive(t, calls[plan.Tasks[1].ID], "worker calls: %v", calls)
			require.Positive(t, calls[plan.Tasks[2].ID], "worker calls: %v", calls)
			require.Len(t, calls, 2)
			saved, err := yakit.GetAISessionPlanAndExecByCoordinatorID(r.config.GetDB(), id)
			require.NoError(t, err)
			require.Contains(t, saved.TaskProgress, `"finished":true`)
		})
	}
}
