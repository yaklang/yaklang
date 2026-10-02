package aireact

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_PETask_DeepIntentRecognition verifies that pe_task init runs
// deep intent recognition when intent recognition is enabled.
//
// Uses a SHORT user input (<100 runes) so the default loop's init uses
// fast matching (not deep intent). Only the PE task's init unconditionally
// runs deep intent, so any intent loop calls must come from the PE task.
//
// Flow:
//  1. Create a forge with PlanPrompt generating one sub-task
//  2. Enable intent recognition (override NewTestReAct default)
//  3. Main loop → require_ai_blueprint (short input → fast match in default init)
//  4. Blueprint params → call-ai-blueprint
//  5. Forge executes, plan is generated from PlanPrompt, PE task starts
//  6. PE task init → deep intent recognition → intent loop AI called
//  7. PE task main loop → directly_answer
//  8. Assert: intent loop was invoked during PE task phase
func TestReAct_PETask_DeepIntentRecognition(t *testing.T) {
	testNonce := utils.RandStringBytes(16)
	testForgeName := "test_forge_pe_intent_" + testNonce
	planFlag := "plan_flag_pe_intent_" + testNonce

	forge := &schema.AIForge{
		ForgeName:    testForgeName,
		ForgeType:    "yak",
		ForgeContent: "",
		InitPrompt: `{{ if .Forge.UserParams }}
## Task Parameters
<content_wait_for_review>
{{ .Forge.UserParams }}
</content_wait_for_review>
{{end}}
**Target**: {{ .Forge.UserQuery }}`,
		PlanPrompt: `{
  "@action": "plan",
  "query": "-",
  "main_task": "` + planFlag + `",
  "main_task_goal": "Execute the test task",
  "tasks": [
    {
      "subtask_name": "test_subtask",
      "subtask_goal": "execute test subtask for intent verification"
    }
  ]
}`,
	}
	if err := yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge); err != nil {
		t.Fatalf("failed to create test forge: %v", err)
	}
	defer func() {
		yakit.DeleteAIForge(consts.GetGormProfileDatabase(), &ypb.AIForgeFilter{
			ForgeName: testForgeName,
		})
	}()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var intentLoopCalled int32
	var peTaskCalled int32
	finishedCh := make(chan bool, 1)

	_, err := NewTestReAct(
		aicommon.WithDisableIntentRecognition(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// Phase: Intent loop — single LiteForge "intent-keyword-gen" call
			// (during PE task init). The simplified loop_intent runs entirely
			// in InitTask with one LiteForge call; no ReAct iterations.
			if aicommon.IsIntentKeywordGenPrompt(prompt) &&
				!strings.Contains(prompt, "PLAN_STATUS_") {
				atomic.AddInt32(&intentLoopCalled, 1)
				log.Infof("intent loop (intent-keyword-gen) called during PE task init")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "intent-keyword-gen", "intent_summary": "test intent analysis", "search_keywords": ["test tools"], "tags": ["test"], "questions": ["what tools are available?"]}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Intent capability recommendation (conditional second call)
			if aicommon.IsIntentRecommendPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "intent-capability-recommend", "recommended_capabilities": []}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Capability catalog match (BM25 chunk matching LiteForge)
			if aicommon.IsCapabilityCatalogMatchPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "capability-catalog-match", "matched_identifiers": []}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: PE task execution (contains PLAN_STATUS_ and planFlag)
			if strings.Contains(prompt, "PLAN_STATUS_") && strings.Contains(prompt, planFlag) {
				atomic.AddInt32(&peTaskCalled, 1)
				log.Infof("PE task main loop called")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "task done ` + testNonce + `", "human_readable_thought": "completing task"}
`))
				rsp.Close()
				select {
				case finishedCh <- true:
				default:
				}
				return rsp, nil
			}

			// Phase: Main loop - request blueprint
			if isPrimaryDecisionPrompt(prompt) &&
				!strings.Contains(prompt, "PLAN_STATUS_") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "` + testForgeName + `" },
"human_readable_thought": "requesting blueprint", "cumulative_summary": "test"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Tool parameter generation
			if isToolParamGenPromptForTool(prompt, "") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-tool", "tool": "noop", "params": {}}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Blueprint parameter generation
			if isToolParamGenPromptForBlueprint(prompt, testForgeName) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-ai-blueprint","blueprint": "` + testForgeName + `", "params": {"query": "test"},
"human_readable_thought": "calling blueprint", "cumulative_summary": "blueprint params"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Task summary
			if strings.Contains(prompt, "任务执行引擎") && strings.Contains(prompt, "task_long_summary") && !strings.Contains(prompt, "PLAN_STATUS_") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary", "status_summary": "done", "task_short_summary": "completed", "task_long_summary": "task completed"}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: FINAL_ANSWER (post-iteration DirectlyAnswer)
			if isDirectAnswerPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "mocked summary"}`))
				rsp.Close()
				return rsp, nil
			}

			log.Warnf("unexpected prompt in TestReAct_PETask_DeepIntentRecognition, length=%d", len(prompt))
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "fallback", "human_readable_thought": "fallback"}
`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithShowForgeListInPrompt(true),
		aicommon.WithDisableDynamicPlanning(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "run blueprint",
		}
	}()

	after := time.After(30 * time.Second)

	planEnded := false

LOOP:
	for {
		select {
		case <-finishedCh:
			break LOOP
		case e := <-out:
			// Only break when forge/plan execution ends (EVENT_TYPE_END_PLAN_AND_EXECUTION)
			// NOT on react_task_status_changed, because sub-loops (intent loop)
			// also emit task status events that would prematurely exit.
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnded = true
			}
			if planEnded && atomic.LoadInt32(&peTaskCalled) > 0 {
				break LOOP
			}
		case <-after:
			t.Log("timeout reached")
			break LOOP
		}
	}
	close(in)

	intentCount := atomic.LoadInt32(&intentLoopCalled)
	peCount := atomic.LoadInt32(&peTaskCalled)

	t.Logf("intent loop called %d time(s), PE task called %d time(s)", intentCount, peCount)

	if intentCount == 0 {
		t.Fatal("intent loop was NOT called during PE task init - deep intent recognition did not trigger for pe_task")
	}
	if peCount == 0 {
		t.Fatal("PE task main loop was NOT called - PE task did not execute")
	}
}

// Native PLAN uses coordinator and worker loops directly. The legacy intent
// initialization must not introduce another loop, even when enabled globally.
func TestReAct_PlanExec_DoesNotStartLegacyIntentLoops(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	native := newNativePlanTestModel("")
	var coordinatorCalls, workerCalls int32
	ins, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithNoOpMemoryTriage(),
		aicommon.WithDisableIntentRecognition(false), aicommon.WithAgreeYOLO(),
		aicommon.WithEventHandler(func(*schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if strings.Contains(req.GetPrompt(), "Execute the assigned frozen plan task.") {
				atomic.AddInt32(&workerCalls, 1)
			} else if strings.Contains(req.GetCallerLabel(), "coordinator") {
				atomic.AddInt32(&coordinatorCalls, 1)
			} else {
				return nil, utils.Errorf("unexpected auxiliary loop: %s", req.GetCallerLabel())
			}
			rsp, handled, err := native(c, req, "native")
			if !handled {
				return nil, utils.Error("native PLAN requires function calls")
			}
			return rsp, err
		}))
	if err != nil {
		t.Fatal(err)
	}
	if err := ins.PlanAndExecute(ctx, "Plan one deterministic check"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&coordinatorCalls) < 5 || atomic.LoadInt32(&workerCalls) != 2 {
		t.Fatalf("incomplete coordinator/worker flow: %d/%d", coordinatorCalls, workerCalls)
	}
}
