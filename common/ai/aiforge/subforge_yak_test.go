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
			sessionID := uuid.NewString()
			var liveEvents []*schema.AiOutputEvent
			childOrdinals := map[string]int{}
			input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
			var childReviews, childReleases, treeSnapshots atomic.Int32
			var snapshotsBeforeReplay int32
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
							return forgeResponse(c, req, native, "submit_task_result", map[string]any{"summary": fmt.Sprintf("INVENTORY_CHILD_EVIDENCE_%d", childOrdinals[id])})
						}
					} else {
						if step < 2 {
							if !strings.Contains(prompt, "sub_forge_inventory") || !strings.Contains(prompt, "sub_forge_glossary") {
								return nil, fmt.Errorf("worker did not receive both candidates")
							}
							return forgeResponse(c, req, native, "directly_call_tool", map[string]any{
								"directly_call_tool_name":   "sub_forge_inventory",
								"directly_call_tool_params": map[string]any{"query": "inventory-ref"},
								"directly_call_reason":      "Inspect the inventory through the bound specialist.",
								"async_directly_call_tool":  true, // Sub-Forge calls still wait for the result.
							})
						}
						if step == 2 {
							if !strings.Contains(prompt, "INVENTORY_CHILD_EVIDENCE_1") || !strings.Contains(prompt, "INVENTORY_CHILD_EVIDENCE_2") {
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
						childOrdinals[id] = len(childOrdinals) + 1
					}
					planName := "Inventory brief"
					if child {
						planName = fmt.Sprintf("Inventory specialist run %d", childOrdinals[id])
					}
					var plan map[string]any
					_ = json.Unmarshal([]byte(`{"main_task":"Inventory brief","main_task_goal":"Inspect evidence","tasks":[{"subtask_name":"Produce evidence","subtask_goal":"Verify input and provide a result","subtask_identifier":"evidence","depends_on":[]}]}`), &plan)
					plan["main_task"] = planName
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
				vm.SetVars(map[string]any{
					"ATTACHMENT_LIVE": func() []*schema.AiOutputEvent {
						mu.Lock()
						defer mu.Unlock()
						return append([]*schema.AiOutputEvent(nil), liveEvents...)
					},
					"ATTACHMENT_HISTORY": func() []*schema.AiOutputEvent {
						return recoverForgeTaskTreeEvents(t, ctx, sessionID)
					},
					"VERIFY_ATTACHMENT": func(live, recovered, reordered, interrupted string) {
						assertForgeTaskTree(t, parentID, live, recovered, reordered, interrupted)
					},
					"START_ATTACHMENT_RETRY": func() {
						mu.Lock()
						defer mu.Unlock()
						workerSteps[parentID] = 0
						parentUsedChildResult = false
						snapshotsBeforeReplay = treeSnapshots.Load()
					},
					"VERIFY_ATTACHMENT_RETRY": func(before, after string) {
						require.JSONEq(t, before, after, "checkpoint replay preserves the original child attachments")
						require.Equal(t, snapshotsBeforeReplay, treeSnapshots.Load(), "checkpoint replay emitted a new child snapshot")
					},
					"VERIFY_SUB_FORGE": func(execution *aiforge.ForgeExecution) {
						require.True(t, execution.Snapshot().Finished)
						require.Contains(t, fmt.Sprint(execution.Result().Formated), "PARENT_USED_INVENTORY_CHILD_EVIDENCE")
					}})
				yak.BindAIConfigToEngine(vm, yak.WithContext(ctx), aicommon.WithID(parentID),
					aicommon.WithWorkdir(workdir), aicommon.WithPersistentSessionId(sessionID), aicommon.WithAICallback(model), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAgreeYOLO(), aicommon.WithForceManualPlanReview(true),
					aicommon.WithNoOpMemoryTriage(), aicommon.WithDisablePerception(true),
					aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEventInputChanx(input),
					aicommon.WithMaxIterationCount(30), aicommon.WithDisableIncreaseIteration(true),
					aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
						if event.Type == "sub_forge_task_tree" {
							treeSnapshots.Add(1)
						}
						if event.Type == schema.EVENT_TYPE_PLAN || event.Type == "sub_forge_task_tree" {
							snapshot := *event
							mu.Lock()
							liveEvents = append(liveEvents, &snapshot)
							mu.Unlock()
						}
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
			_, err := engine.ExecuteExWithContext(ctx, string(code)+forgeAttachmentAssertions, nil)
			require.NoError(t, err)
			require.EqualValues(t, 2, childReviews.Load())
			require.Positive(t, childReleases.Load())
			mu.Lock()
			require.True(t, childParamsSeen)
			require.True(t, parentUsedChildResult)
			require.Len(t, childOrdinals, 2, "checkpoint replay must not execute the child again")
			require.NotEmpty(t, lastChildEndpoint)
			t.Logf("Yak entry completed with native=%t, child arguments and default observed, two child reviews routed, both results consumed, live and replayed trees agree, and task recovery reuses both child checkpoints", native)
			mu.Unlock()
		})
	}
}

const forgeAttachmentAssertions = `
VERIFY_SUB_FORGE(execution)
parentID = execution.GetConfig().GetRuntimeId()
live = newTaskTreeView()
reordered = newTaskTreeView()
interrupted = newTaskTreeView()
recovered = newTaskTreeView()
events = ATTACHMENT_LIVE()
for _, event = range events {
    observeTaskTree(live, event)
    if string(event.Type) != "sub_forge_task_tree" || json.loads(string(event.Content))["status"] == "running" {
        observeTaskTree(interrupted, event)
    }
}
for i = len(events) - 1; i >= 0; i-- {
    if string(events[i].Type) == "sub_forge_task_tree" {
        observeTaskTree(reordered, events[i])
        observeTaskTree(reordered, events[i])
    }
}
reordered["plans"] = live["plans"]
for _, event = range ATTACHMENT_HISTORY() {
    observeTaskTree(recovered, event)
}
VERIFY_ATTACHMENT(
    json.dumps(attachSubForgeTrees(live, parentID, false)),
    json.dumps(attachSubForgeTrees(recovered, parentID, true)),
    json.dumps(attachSubForgeTrees(reordered, parentID, false)),
    json.dumps(attachSubForgeTrees(interrupted, parentID, true)),
)
original = attachSubForgeTrees(live, parentID, false)["subtasks"][0]
START_ATTACHMENT_RETRY()
retried = aiagent.NewExecutor(
    "inventory-brief",
    {"query": "Check inventory-ref and prepare a short brief using the specialist result."},
    options...,
)~
defer retried.Close()
retried.SetRecoveryStartTaskID(original["task_id"])
die(retried.Run())
VERIFY_SUB_FORGE(retried)
retryHistory = newTaskTreeView()
for _, event = range ATTACHMENT_HISTORY() {
    observeTaskTree(retryHistory, event)
}
afterRetry = attachSubForgeTrees(retryHistory, parentID, true)["subtasks"][0]
VERIFY_ATTACHMENT_RETRY(json.dumps(original["sub_forge_runs"]), json.dumps(afterRetry["sub_forge_runs"]))
`

func recoverForgeTaskTreeEvents(t *testing.T, ctx context.Context, sessionID string) []*schema.AiOutputEvent {
	t.Helper()
	events := make(chan *schema.AiOutputEvent, 128)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event.IsSync {
				select {
				case events <- event:
				case <-ctx.Done():
				}
			}
		}))
	cfg.StartEventLoop(ctx)
	syncID := uuid.NewString()
	cfg.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_RECOVERY_HISTORY,
		SyncID:        syncID,
		SyncJsonInput: fmt.Sprintf(`{"session_id":%q,"limit":256}`, sessionID),
	})
	var recovered []*schema.AiOutputEvent
	for {
		select {
		case event := <-events:
			if event.SyncID == syncID && event.NodeId == "recovery_history" {
				require.NotContains(t, string(event.Content), `"has_more":true`)
				return recovered
			}
			recovered = append(recovered, event)
		case <-ctx.Done():
			t.Fatal("recovery_history did not respond")
		}
	}
}

func assertForgeTaskTree(t *testing.T, parentID, live, recovered, reordered, interrupted string) {
	t.Helper()
	type childRun struct {
		ParentID   string               `json:"parent_coordinator_id"`
		ParentTask string               `json:"parent_task_id"`
		Attempt    string               `json:"parent_task_uuid"`
		Call       string               `json:"call_tool_id"`
		ChildID    string               `json:"child_coordinator_id"`
		Tool       string               `json:"tool_name"`
		Revision   int                  `json:"revision"`
		Status     string               `json:"status"`
		Tree       coordinator.PlanNode `json:"task_tree"`
	}
	type parentTree struct {
		Subtasks []struct {
			ID       string              `json:"task_id"`
			Subtasks []json.RawMessage   `json:"subtasks"`
			Progress string              `json:"progress"`
			Runs     map[string]childRun `json:"sub_forge_runs"`
		} `json:"subtasks"`
	}
	var tree parentTree
	require.NoError(t, json.Unmarshal([]byte(live), &tree))
	require.Len(t, tree.Subtasks, 1)
	parent := tree.Subtasks[0]
	require.Empty(t, parent.Subtasks, "attachment mutated the parent's executable task shape")
	require.Equal(t, "completed", parent.Progress)
	require.Len(t, parent.Runs, 2, "one attachment per actual sub-Forge invocation")
	require.JSONEq(t, live, recovered)
	require.JSONEq(t, live, reordered)
	calls := map[string]bool{}
	names := map[string]bool{}
	var attempt string
	for id, run := range parent.Runs {
		require.Equal(t, id, run.ChildID)
		require.NotEqual(t, parentID, id)
		require.Equal(t, parentID, run.ParentID)
		require.Equal(t, parent.ID, run.ParentTask)
		require.NotEmpty(t, run.Attempt)
		if attempt == "" {
			attempt = run.Attempt
		}
		require.Equal(t, attempt, run.Attempt)
		require.NotEmpty(t, run.Call)
		require.False(t, calls[run.Call])
		calls[run.Call] = true
		require.Equal(t, "sub_forge_inventory", run.Tool)
		require.GreaterOrEqual(t, run.Revision, 3)
		require.Equal(t, "succeeded", run.Status)
		require.Len(t, run.Tree.Subtasks, 1)
		require.Equal(t, "completed", run.Tree.Subtasks[0].Progress)
		names[run.Tree.Name] = true
	}
	require.Equal(t, map[string]bool{"Inventory specialist run 1": true, "Inventory specialist run 2": true}, names)
	var history parentTree
	require.NoError(t, json.Unmarshal([]byte(interrupted), &history))
	require.Len(t, history.Subtasks, 1)
	require.Len(t, history.Subtasks[0].Runs, 2)
	for _, run := range history.Subtasks[0].Runs {
		require.Equal(t, "unknown", run.Status)
	}
}
