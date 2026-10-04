package aiforge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const forgePlan = `{"main_task":"Forge sources","main_task_goal":"Verify sources","tasks":[{"subtask_name":"Read A","subtask_goal":"Verify A","subtask_identifier":"a","depends_on":[]},{"subtask_name":"Compare B","subtask_goal":"Compare B to A","subtask_identifier":"b","depends_on":["a"]}]}`

func forgeResponse(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, native bool, name string, args map[string]any) (*aicommon.AIResponse, error) {
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	if native {
		if wire.ToolCallCallback == nil || wire.FinishReasonCallback == nil {
			return nil, fmt.Errorf("native request lost its protocol: %s", req.GetCallerLabel())
		}
		raw, _ := json.Marshal(args)
		// Exercise fragmented arguments rather than handing the parser a full object.
		for _, part := range []string{string(raw[:len(raw)/2]), string(raw[len(raw)/2:])} {
			wire.ToolCallCallback([]*aispec.ToolCall{{ID: "forge-call", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: part}}})
			name = ""
		}
		wire.FinishReasonCallback("tool_calls", nil)
		response := c.NewAIResponse()
		response.Close()
		return response, nil
	}
	if len(wire.Tools) != 0 || wire.ToolCallCallback != nil {
		return nil, fmt.Errorf("text mode advertised native tools")
	}
	args["@action"] = name
	args["identifier"] = name
	raw, _ := json.Marshal(args)
	response := c.NewAIResponse()
	response.EmitOutputStream(strings.NewReader(string(raw)))
	response.Close()
	return response, nil
}

func TestForgeParentRuntimeInheritsConfigurationAndOwnsOneInputChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	parentFrozen := aicommon.NewFrozenBlockPartitionProducer()
	parent, err := aireact.NewTestReAct(aicommon.WithContext(ctx), aicommon.WithEnablePlanAndExec(false), aicommon.WithEnableDetachedPlan(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(false), aicommon.WithPlanExecTaskConcurrency(3), aicommon.WithFrozenBlockPartitionProducer(parentFrozen), aicommon.WithAppendPersistentContext("parent preference"), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls++
		response := c.NewAIResponse()
		response.Close()
		return response, nil
	}))
	require.NoError(t, err)
	parentConfig := parent.GetConfig().(*aicommon.Config)
	task := aicommon.NewStatefulTaskBase("outer-forge-task", "caller query", ctx, parent.Emitter, true)
	input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	hotpatch := parentConfig.HotPatchBroadcaster.Subscribe()
	defer parentConfig.HotPatchBroadcaster.Unsubscribe(hotpatch)
	blueprint := aiforge.NewForgeBlueprint("owned-child", aiforge.WithInitializePrompt("Create a plan"))
	options := aicommon.ConvertConfigToOptionsWithoutHotPatch(parentConfig)
	options = append(options, aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch), aicommon.WithID("must-not-replace-owned-id"))
	execution, err := blueprint.CreateCoordinator(coordinator.WithForgeParent(ctx, parent, task, "owned-forge-id"), map[string]any{"query": "caller query"}, options...)
	require.NoError(t, err, "an explicitly selected Forge is independent of the generic PLAN action toggle")
	defer execution.Close()
	require.Equal(t, "owned-forge-id", execution.Id)
	require.False(t, execution.EnableFunctionCallMode)
	require.False(t, execution.EnableDetachedPlan, "a Forge must keep approval and delivery in its own live invocation")
	require.False(t, execution.PlanningOnly())
	require.True(t, parentConfig.EnableDetachedPlan, "ordinary detached PLAN must remain available in the parent")
	require.Equal(t, parentConfig.GetPlanExecTaskConcurrency(), execution.GetPlanExecTaskConcurrency())
	require.Contains(t, execution.PersistentMemory, "parent preference")
	require.Equal(t, []string{"parent preference"}, execution.PersistentMemory, "parent append-only options must be applied exactly once")
	require.NotSame(t, parentFrozen, execution.FrozenBlockPartitionProducer)
	require.Empty(t, parentFrozen.ProducePartitions(), "child business context must not rewrite the parent prefix")
	require.Contains(t, execution.FrozenBlockPartitionProducer.ProducePartitions()[0].Content, "parent preference")
	require.Same(t, input, execution.EventInputChan)
	require.Same(t, hotpatch, execution.HotPatchOptionChan)
	require.Same(t, parentConfig.Timeline, execution.Timeline)
	_, err = execution.Config.CallAI(aicommon.NewAIRequest("verify inherited transport"))
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	cancel()
	require.ErrorIs(t, execution.GetContext().Err(), context.Canceled)
}

func TestForgeNativeExecutionMigration(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, source := range []string{"generated", "preset", "mocker"} {
			t.Run(fmt.Sprintf("%s/function_call_%v", source, native), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				builder := aiforge.NewYakForgeBlueprintConfig("migration", "Verify the supplied sources", "keep source provenance").WithResultPrompt("Return the business result in the original JSON shape").WithActionName("business_result,alias_result")
				if source == "preset" {
					builder.WithPlanPrompt(`{"@action":"plan",` + forgePlan[1:])
				}
				blueprint, err := builder.Build()
				require.NoError(t, err)
				mockerCalls := 0
				if source == "mocker" {
					blueprint.PlanMocker = func(*coordinator.Session) *coordinator.PlanResponse {
						mockerCalls++
						p, err := coordinator.ExtractPlan(nil, `{"@action":"plan",`+forgePlan[1:])
						require.NoError(t, err)
						return p
					}
				}
				var execution *aiforge.ForgeExecution
				var mu sync.Mutex
				workerSteps := map[string]int{}
				requests, reviews, callbacks, formatCalls := 0, 0, 0, 0
				execution, err = blueprint.CreateCoordinator(ctx, map[string]any{"query": "compare A and B"},
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithEnableDetachedPlan(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeYOLO(),
					aiforge.WithExecutorResultHandler(func(e *aiforge.ForgeExecution) {
						callbacks++
						require.Equal(t, "completed", e.GetContextProvider().RootTask.Progress)
					}),
					aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
						if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
							mu.Lock()
							reviews++
							mu.Unlock()
						}
					}),
					aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						mu.Lock()
						defer mu.Unlock()
						requests++
						if requests > 40 {
							return nil, fmt.Errorf("Forge failed to converge")
						}
						prompt := req.GetPrompt()
						state := execution.Snapshot()
						if strings.Contains(req.GetCallerLabel(), "migration-result") {
							formatCalls++
							require.True(t, execution.Snapshot().Phase == coordinator.PhaseExec)
							for _, a := range state.Attempts {
								require.Equal(t, coordinator.Accepted, a.State)
							}
							require.False(t, c.GetConfigBool("EnableFunctionCallMode"), "result extraction must not inherit the loop protocol")
							return forgeResponse(c, req, false, "business_result", map[string]any{"summary": "两份来源已核对", "payload": map[string]any{"ok": true, "values": []any{1, "two", nil}}})
						}
						if strings.Contains(prompt, "执行已批准的冻结任务书。") {
							require.Contains(t, prompt, "keep source provenance", "worker lost the Forge persistent prompt")
							id := req.GetTaskIndex()
							if state.Attempts[id].Task.ID == "" {
								for k, a := range state.Attempts {
									if a.State == coordinator.Running {
										id = k
									}
								}
							}
							step := workerSteps[id]
							workerSteps[id]++
							if step == 0 {
								return forgeResponse(c, req, native, "save_evidence", map[string]any{"evidence_id": "forge-source-" + state.Attempts[id].Task.Index, "evidence_content": "已核对来源，结论可靠，可供后继任务使用。"})
							}
							if step == 1 {
								return forgeResponse(c, req, native, "submit_task_result", map[string]any{"summary": "来源核对完成", "evidence_ids": []string{"forge-source-" + state.Attempts[id].Task.Index}})
							}
							return forgeResponse(c, req, native, "finish", map[string]any{})
						}
						if state.Plan == nil {
							var plan map[string]any
							_ = json.Unmarshal([]byte(forgePlan), &plan)
							return forgeResponse(c, req, native, "create_plan", map[string]any{"plan": plan, "plan_document": "# 方案\n先核对 A，再比对 B。"})
						}
						if state.Phase == coordinator.PhasePlan {
							return forgeResponse(c, req, native, "submit_plan", map[string]any{})
						}
						for _, a := range state.Attempts {
							if a.State == coordinator.AwaitingReview {
								return forgeResponse(c, req, native, "review_task", map[string]any{"task_id": a.Task.ID, "attempt_id": a.ID, "decision": "accept", "reason": "已核对真实 Evidence，与任务书一致"})
							}
						}
						return forgeResponse(c, req, native, "wait_messages", map[string]any{})
					}))
				require.NoError(t, err)
				defer execution.Close()
				require.NoError(t, execution.Run())
				require.NoError(t, execution.Run(), "Run and callbacks must be idempotent")
				require.True(t, execution.Snapshot().Finished)
				require.Equal(t, 1, reviews)
				require.Equal(t, 1, callbacks)
				require.Equal(t, 1, formatCalls)
				if source == "mocker" {
					require.Equal(t, 1, mockerCalls)
				}
				result := execution.Result()
				require.NotNil(t, result.Action)
				require.True(t, result.Action.GetInvokeParams("payload").GetBool("ok"))
				require.Contains(t, execution.GetSessionEvidenceRendered(), "forge-source-1")
				require.Contains(t, execution.GetSessionEvidenceRendered(), "forge-source-2")
				proof, err := os.ReadFile(execution.Snapshot().Report.Path)
				require.NoError(t, err)
				require.Contains(t, string(proof), "Forge execution proof")
				result.Action.GetParams()["summary"] = "caller mutation"
				require.Equal(t, "两份来源已核对", execution.Result().Action.GetString("summary"))
				require.Nil(t, builder.ForgeResult.Action, "builder must not share mutable execution results")
			})
		}
	}
}
