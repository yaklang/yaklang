package aireact

import (
	"bytes"
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
	yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge)
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

			// Phase: LiteForge intent finalize (post-iteration hook)
			if strings.Contains(prompt, "intent-finalize-summary") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "intent-finalize-summary", "intent_analysis": "test intent analysis", "context_enrichment": "test context"}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Intent loop (during PE task init)
			if utils.MatchAllOfSubString(prompt, "finalize_enrichment", "query_capabilities") &&
				!utils.MatchAllOfSubString(prompt, "directly_answer") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				atomic.AddInt32(&intentLoopCalled, 1)
				log.Infof("intent loop called during PE task init")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "query_capabilities", "human_readable_thought": "searching capabilities", "search_query": "test tools"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Tool parameter generation
			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-tool", "tool": "noop", "params": {}}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Blueprint parameter generation
			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "Blueprint Description:", "call-ai-blueprint", testForgeName) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-ai-blueprint","blueprint": "` + testForgeName + `", "params": {"query": "test"},
"human_readable_thought": "calling blueprint", "cumulative_summary": "blueprint params"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: PE task execution (contains PROGRESS_TASK_ and planFlag)
			if utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_", planFlag) {
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

			// Phase: Main loop → request blueprint
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool", testForgeName) &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "` + testForgeName + `" },
"human_readable_thought": "requesting blueprint", "cumulative_summary": "test"}
`))
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

// TestReAct_PlanExec_DeepIntentRecognition verifies that deep intent
// recognition runs during both plan generation and PE task execution
// when using the full request_plan_and_execution flow.
//
// Flow:
//  1. Main loop → request_plan_and_execution
//  2. Coordinator starts: plan loop init → intent recognition
//  3. Plan loop AI → generate plan
//  4. Plan review → auto-approve
//  5. PE task init → intent recognition
//  6. PE task AI → directly_answer
//  7. Assert: intent loop called at least once
func TestReAct_PlanExec_DeepIntentRecognition(t *testing.T) {
	testNonce := utils.RandStringBytes(16)
	planPayload := "plan_intent_test_" + testNonce
	subtaskName := "subtask_intent_" + testNonce

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var intentLoopCalled int32
	var planLoopCalled int32
	var peTaskCalled int32

	_, err := NewTestReAct(
		aicommon.WithDisableIntentRecognition(false),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			// Phase: LiteForge intent finalize (post-iteration hook)
			if strings.Contains(prompt, "intent-finalize-summary") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "intent-finalize-summary", "intent_analysis": "test intent analysis", "context_enrichment": "test context"}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Intent loop (during plan or PE task init)
			if utils.MatchAllOfSubString(prompt, "finalize_enrichment", "query_capabilities") &&
				!utils.MatchAllOfSubString(prompt, "directly_answer") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") &&
				!strings.Contains(prompt, "search_knowledge") {
				atomic.AddInt32(&intentLoopCalled, 1)
				log.Infof("intent loop called (count=%d)", atomic.LoadInt32(&intentLoopCalled))
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "query_capabilities", "human_readable_thought": "searching capabilities", "search_query": "test"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Plan loop (contains search_knowledge + plan actions)
			if utils.MatchAllOfSubString(prompt, "search_knowledge") &&
				strings.Contains(prompt, "plan") &&
				!utils.MatchAllOfSubString(prompt, "directly_answer") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				atomic.AddInt32(&planLoopCalled, 1)
				log.Infof("plan loop AI called")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "plan",
  "main_task": "` + planPayload + `",
  "main_task_goal": "test plan with intent recognition",
  "tasks": [
    {"task_name": "` + subtaskName + `", "task_description": "execute test subtask"}
  ]
}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: PE task execution
			if utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				atomic.AddInt32(&peTaskCalled, 1)
				log.Infof("PE task main loop called")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_answer", "answer_payload": "subtask done ` + testNonce + `", "human_readable_thought": "completing subtask"}
`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Satisfaction verification
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done"}`))
				rsp.Close()
				return rsp, nil
			}

			// Phase: Main loop → request_plan_and_execution
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "` + planPayload + `" },
"human_readable_thought": "requesting plan execution", "cumulative_summary": "plan exec test"}
`))
				rsp.Close()
				return rsp, nil
			}

			log.Warnf("unexpected prompt in TestReAct_PlanExec_DeepIntentRecognition, length=%d", len(prompt))
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
		aicommon.WithAllowPlanUserInteract(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "plan security assessment",
		}
	}()

	du := time.Duration(30)
	if utils.InGithubActions() {
		du = 15
	}
	after := time.After(du * time.Second)

	planStarted := false
	planEnded := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				planStarted = true
				log.Infof("plan execution started")
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnded = true
				log.Infof("plan execution ended")
				break LOOP
			}
		case <-after:
			t.Log("timeout reached")
			break LOOP
		}
	}
	close(in)

	intentCount := atomic.LoadInt32(&intentLoopCalled)
	planCount := atomic.LoadInt32(&planLoopCalled)
	peCount := atomic.LoadInt32(&peTaskCalled)

	t.Logf("intent loop: %d, plan loop: %d, PE task: %d", intentCount, planCount, peCount)
	t.Logf("plan started: %v, plan ended: %v", planStarted, planEnded)

	if !planStarted {
		t.Fatal("plan execution did not start")
	}

	if intentCount == 0 {
		t.Fatal("intent loop was NOT called - deep intent recognition did not trigger during plan/PE init")
	}
}
