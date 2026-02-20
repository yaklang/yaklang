package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type mockForgeFactoryForEquiv struct {
	forges map[string]*schema.AIForge
}

func (m *mockForgeFactoryForEquiv) Query(_ context.Context, _ ...aicommon.ForgeQueryOption) ([]*schema.AIForge, error) {
	var result []*schema.AIForge
	for _, f := range m.forges {
		result = append(result, f)
	}
	return result, nil
}
func (m *mockForgeFactoryForEquiv) GetAIForge(name string) (*schema.AIForge, error) {
	f, ok := m.forges[name]
	if !ok {
		return nil, fmt.Errorf("forge %q not found", name)
	}
	return f, nil
}
func (m *mockForgeFactoryForEquiv) GenerateAIForgeListForPrompt(_ []*schema.AIForge) (string, error) {
	return "", nil
}
func (m *mockForgeFactoryForEquiv) GenerateAIJSONSchemaFromSchemaAIForge(_ *schema.AIForge) (string, error) {
	return `{}`, nil
}

type forgeEquivResult struct {
	asyncTriggerCalled bool
	asyncTriggerAction string
	taskIsAsync        bool
	finishCalled       bool
	executeErr         error
}

func runForgeEquivTest(
	t *testing.T,
	testName string,
	forgeName string,
	aiResponseJSON string,
	promptMatchStrings []string,
) forgeEquivResult {
	t.Helper()

	var result forgeEquivResult
	var mu sync.Mutex
	var finishCh = make(chan struct{}, 1)

	forgeMgr := &mockForgeFactoryForEquiv{
		forges: map[string]*schema.AIForge{
			forgeName: {
				ForgeName:        forgeName,
				ForgeVerboseName: forgeName + "-verbose",
			},
		},
	}

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if utils.MatchAllOfSubString(prompt, promptMatchStrings...) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(aiResponseJSON))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAnyOfSubString(prompt, "call-forge", "call-ai-blueprint", "Blueprint Parameter") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-ai-blueprint", "params": {"query": "test input"}}`))
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("[%s] Failed to create ReAct: %v", testName, err)
	}

	if cfg, ok := reactIns.GetConfig().(*aicommon.Config); ok {
		cfg.AiForgeManager = forgeMgr
	} else {
		t.Fatalf("[%s] Failed to cast config to *aicommon.Config", testName)
	}

	var capturedTask aicommon.AIStatefulTask

	loop, err := reactloops.NewReActLoop(testName+"-loop", reactIns,
		reactloops.WithOnAsyncTaskTrigger(func(action *reactloops.LoopAction, task aicommon.AIStatefulTask) {
			mu.Lock()
			result.asyncTriggerCalled = true
			result.asyncTriggerAction = action.ActionType
			mu.Unlock()
			t.Logf("[%s] onAsyncTaskTrigger: action=%s", testName, action.ActionType)
		}),
		reactloops.WithOnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
			mu.Lock()
			result.finishCalled = true
			mu.Unlock()
			finishCh <- struct{}{}
			t.Logf("[%s] onAsyncTaskFinished: task=%s", testName, task.GetId())
		}),
		reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
			capturedTask = task
		}),
	)
	if err != nil {
		t.Fatalf("[%s] Failed to create loop: %v", testName, err)
	}

	result.executeErr = loop.Execute(testName+"-task", context.Background(), "test forge "+testName)

	select {
	case <-finishCh:
	case <-time.After(10 * time.Second):
		t.Logf("[%s] Timeout waiting for async finish", testName)
	}

	if capturedTask != nil {
		result.taskIsAsync = capturedTask.IsAsyncMode()
	}

	return result
}

// TestForgeEquivalence_RequireAIBlueprint_vs_LoadCapability verifies that
// load_capability produces the same async lifecycle as require_ai_blueprint
// when dispatching to a forge.
//
// Both paths should produce identical observable behavior:
// 1. onAsyncTaskTrigger is called (task enters async mode)
// 2. task.IsAsyncMode() is true
// 3. onAsyncTaskFinished is called (forge callback completes)
//
// Note: invokePlanAndExecute panics due to incomplete mock (GetCurrentTask nil),
// but this is identical in both paths and does not affect the lifecycle equivalence.
func TestForgeEquivalence_RequireAIBlueprint_vs_LoadCapability(t *testing.T) {
	forgeName := "test-forge"

	var baselineResult forgeEquivResult

	t.Run("require_ai_blueprint_baseline", func(t *testing.T) {
		baselineResult = runForgeEquivTest(
			t,
			"require-blueprint",
			forgeName,
			`{"@action": "object", "next_action": {"type": "require_ai_blueprint", "blueprint_payload": "`+forgeName+`"},
			"human_readable_thought": "requesting ai blueprint"}`,
			[]string{"directly_answer", "require_ai_blueprint"},
		)

		if !baselineResult.asyncTriggerCalled {
			t.Error("expected onAsyncTaskTrigger to be called")
		}
		if !baselineResult.taskIsAsync {
			t.Error("expected task to be in async mode")
		}
		if !baselineResult.finishCalled {
			t.Error("expected onAsyncTaskFinished to be called")
		}
		t.Logf("baseline: asyncTrigger=%v, taskIsAsync=%v, finish=%v",
			baselineResult.asyncTriggerCalled, baselineResult.taskIsAsync, baselineResult.finishCalled)
	})

	t.Run("load_capability_equivalent", func(t *testing.T) {
		loadCapResult := runForgeEquivTest(
			t,
			"load-cap-forge",
			forgeName,
			`{"@action": "load_capability", "identifier": "`+forgeName+`",
			"human_readable_thought": "loading capability for forge"}`,
			[]string{"directly_answer", "load_capability"},
		)

		if !loadCapResult.asyncTriggerCalled {
			t.Error("expected onAsyncTaskTrigger to be called (equivalence)")
		}
		if !loadCapResult.taskIsAsync {
			t.Error("expected task to be in async mode (equivalence)")
		}
		if !loadCapResult.finishCalled {
			t.Error("expected onAsyncTaskFinished to be called (equivalence)")
		}

		if baselineResult.asyncTriggerCalled != loadCapResult.asyncTriggerCalled {
			t.Errorf("asyncTriggerCalled mismatch: baseline=%v, load_cap=%v",
				baselineResult.asyncTriggerCalled, loadCapResult.asyncTriggerCalled)
		}
		if baselineResult.taskIsAsync != loadCapResult.taskIsAsync {
			t.Errorf("taskIsAsync mismatch: baseline=%v, load_cap=%v",
				baselineResult.taskIsAsync, loadCapResult.taskIsAsync)
		}
		if baselineResult.finishCalled != loadCapResult.finishCalled {
			t.Errorf("finishCalled mismatch: baseline=%v, load_cap=%v",
				baselineResult.finishCalled, loadCapResult.finishCalled)
		}

		t.Logf("load_cap: asyncTrigger=%v, taskIsAsync=%v, finish=%v",
			loadCapResult.asyncTriggerCalled, loadCapResult.taskIsAsync, loadCapResult.finishCalled)
		t.Log("EQUIVALENCE: require_ai_blueprint and load_capability produce identical forge async lifecycle")
	})
}
