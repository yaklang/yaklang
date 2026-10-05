package aiforge_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// A narrow VM binding regression. Business flows are standalone Yak cases in aismoking.
func TestForgeYakFactoriesInheritBoundConfig(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call=%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			verified := 0
			engine := yak.NewScriptEngine(1)
			engine.RegisterEngineHooks(func(vm *antlr4yak.Engine) error {
				vm.SetVars(map[string]any{"BOUND_NATIVE": native, "VERIFY": func(e *aiforge.ForgeExecution, expected bool) {
					defer e.Close()
					verified++
					cfg := e.GetConfig()
					require.Equal(t, expected, cfg.EnableFunctionCallMode)
					require.Equal(t, []string{"bound business preference"}, cfg.PersistentMemory)
					require.EqualValues(t, 17, cfg.MaxIterationCount)
				}})
				yak.BindAIConfigToEngine(vm, yak.WithContext(ctx), aicommon.WithEnableFunctionCallMode(native), aicommon.WithAppendPersistentContext("bound business preference"), aicommon.WithMaxIterationCount(17), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithWorkdir(t.TempDir()))
				return nil
			})
			_, err := engine.ExecuteExWithContext(ctx, `
VERIFY(aiagent.NewExecutor("bound-forge", {"query":"bound query"})~, BOUND_NATIVE)
VERIFY(aiagent.NewExecutorFromJson("{\"name\":\"bound-json\",\"init_prompt\":\"Bound plan\"}", {"query":"bound query"})~, BOUND_NATIVE)
VERIFY(aiagent.NewExecutor("bound-override", {"query":"bound query"}, aiagent.functionCallMode(!BOUND_NATIVE))~, !BOUND_NATIVE)
`, nil)
			require.NoError(t, err)
			require.Equal(t, 3, verified)
		})
	}
}

func TestLiteForgeYakFactoryDoesNotInheritBoundProtocol(t *testing.T) {
	for _, parentNative := range []bool{false, true} {
		t.Run(fmt.Sprint(parentNative), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			verified := 0
			engine := yak.NewScriptEngine(1)
			engine.RegisterEngineHooks(func(vm *antlr4yak.Engine) error {
				vm.SetVars(map[string]any{"VERIFY_LITEFORGE": func(lf *liteforgeapp.LiteForge, expected bool) {
					verified++
					cfg := aicommon.NewConfig(ctx, lf.ExtendAIDOptions...)
					require.Equal(t, expected, cfg.EnableFunctionCallMode)
					require.Equal(t, []string{"bound business preference"}, cfg.PersistentMemory)
				}})
				yak.BindAIConfigToEngine(vm, aicommon.WithEnableFunctionCallMode(parentNative),
					aicommon.WithAppendPersistentContext("bound business preference"),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true),
					aicommon.WithWorkdir(t.TempDir()))
				return nil
			})
			_, err := engine.ExecuteExWithContext(ctx, `
VERIFY_LITEFORGE(aiagent.CreateLiteForge("bound-default")~, false)
VERIFY_LITEFORGE(aiagent.CreateLiteForge("bound-native", aiagent.functionCallMode(true))~, true)
VERIFY_LITEFORGE(aiagent.CreateLiteForge("bound-text", aiagent.functionCallMode(false))~, false)
`, nil)
			require.NoError(t, err)
			require.Equal(t, 3, verified)
		})
	}
}

func TestForgeDBGatewayPreservesNativeParentContext(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call=%v", native), func(t *testing.T) {
			type contextKey struct{}
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "caller context"), 15*time.Second)
			defer cancel()
			name := "parent-gateway-" + uuid.NewString()
			// Checkpoint replay is keyed by runtime ID; each protocol needs a fresh child.
			childID, taskID := "gateway-child-"+uuid.NewString(), "gateway-task-"+uuid.NewString()
			db := consts.GetGormProfileDatabase()
			forge := &schema.AIForge{ForgeName: name, ForgeType: schema.FORGE_TYPE_Config,
				InitPrompt: "Use the supplied source plan", PlanPrompt: forgePlan}
			require.NoError(t, yakit.CreateAIForge(db, forge))
			t.Cleanup(func() { _ = yakit.DeleteAIForgeByName(db, name) })
			var parent *aireact.ReAct
			var mu sync.Mutex
			workerSteps := map[string]int{}
			calls := 0
			model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				if calls > 20 {
					return nil, fmt.Errorf("DB Forge did not converge")
				}
				if c.GetContext().Value(contextKey{}) != "caller context" {
					return nil, fmt.Errorf("DB Forge lost the caller context")
				}
				views := coordinator.CollectPlanExecutionSnapshots(parent.GetReActID(), nil)
				if len(views) != 1 || views[0].CoordinatorID != childID || views[0].AsyncReactTaskID != taskID {
					return nil, fmt.Errorf("DB Forge lost its parent runtime: %#v", views)
				}
				if req.GetCallerLabel() == "react-loop:pe_task" {
					// Text-mode requests need not carry TaskIndex; the real native
					// registry still identifies the executing attempt in both modes.
					id := views[0].CurrentTaskID
					workerSteps[id]++
					if workerSteps[id] == 1 {
						return forgeResponse(c, req, native, "submit_task_result", map[string]any{"summary": "gateway source verified"})
					}
					return forgeResponse(c, req, native, "finish", map[string]any{})
				}
				for _, s := range coordinator.GetRunningSessions() {
					if s.Id != childID {
						continue
					}
					state := s.Snapshot()
					if state.Phase == coordinator.PhasePlan {
						return forgeResponse(c, req, native, "submit_plan", map[string]any{})
					}
					for _, a := range state.Attempts {
						if a.State == coordinator.AwaitingReview {
							return forgeResponse(c, req, native, "review_task", map[string]any{"task_id": a.Task.ID, "attempt_id": a.ID, "decision": "accept", "reason": "gateway result verified"})
						}
					}
				}
				return forgeResponse(c, req, native, "wait_messages", map[string]any{})
			}
			var err error
			parent, err = aireact.NewTestReAct(aicommon.WithContext(ctx), aicommon.WithEnablePlanAndExec(false), aicommon.WithAICallback(model),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}))
			require.NoError(t, err)
			config := parent.GetConfig().(*aicommon.Config)
			task := aicommon.NewStatefulTaskBase(taskID, "gateway caller query", ctx, parent.Emitter, true)
			ownedContext := coordinator.WithForgeParent(ctx, parent, task, childID)
			options := aicommon.ConvertConfigToOptionsWithoutHotPatch(config)
			result, err := aicommon.ExecuteForgeFromDB(name, ownedContext, map[string]any{"query": "gateway caller query"}, options...)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, workerSteps, 2)
			require.Empty(t, coordinator.CollectPlanExecutionSnapshots(parent.GetReActID(), nil), "DB Forge must unregister after actual exit")
		})
	}
}
