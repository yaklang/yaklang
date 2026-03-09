package aireact

import (
	"bytes"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	_ "github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func safeSend(ch chan *ypb.AIInputEvent, event *ypb.AIInputEvent) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	ch <- event
	return true
}

// TestReAct_PlanAndExecute_SkipAfterCancel reproduces the original deadlock:
//
//	react_cancel_task kills planCtx -> coordinator event loop dies
//	skip_subtask_in_plan arrives -> previously had no handler alive
//	system stuck in memory_sync loop
//
// After the fix the flow terminates gracefully without deadlock.
func TestReAct_PlanAndExecute_SkipAfterCancel(t *testing.T) {
	testNonce := utils.RandStringBytes(16)

	testForgeName := "test_forge_skip_after_cancel_" + testNonce
	planFlag := "plan_flag_skip_cancel_" + testNonce
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
  "main_task_goal": "test skip after cancel scenario",
  "tasks": [
    {
      "subtask_name": "subtask_a",
      "subtask_goal": "first subtask that will be cancelled then skipped"
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

	in := make(chan *ypb.AIInputEvent, 20)
	out := make(chan *ypb.AIOutputEvent, 100)

	var toolCalled int32
	var unreachableCode bool
	var currentTaskID string

	mockToolName := "mock_skip_cancel_tool_" + utils.RandStringBytes(16)
	mockTool, err := aitool.New(
		mockToolName,
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			atomic.AddInt32(&toolCalled, 1)

			// Simulate the exact bug scenario:
			// 1. Send cancel to kill the planCtx (and coordinator event loop)
			safeSend(in, &ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_REACT_CANCEL_TASK,
				SyncJsonInput: `{"task_id":"` + currentTaskID + `"}`,
				SyncID:        utils.RandStringBytes(8),
			})

			// Small delay to let cancel propagate
			time.Sleep(200 * time.Millisecond)

			// 2. Send skip_subtask_in_plan after coordinator event loop is dead
			safeSend(in, &ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
				SyncJsonInput: `{"reason":"skip after cancel","skip_current_task":true}`,
				SyncID:        utils.RandStringBytes(8),
			})

			time.Sleep(100 * time.Millisecond)
			return "", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", mockToolName) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{
  "@action": "call-tool",
  "tool": "` + mockToolName + `",
  "params": {}
}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "Blueprint Description:", "call-ai-blueprint", testForgeName) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-ai-blueprint","blueprint": "` + testForgeName + `", "params": {"target": "http://example.com", "query": "test"},
"human_readable_thought": "generating blueprint parameters", "cumulative_summary": "forge parameters"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_", planFlag) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "require_tool", "tool_require_payload": "` + mockToolName + `", 
"human_readable_thought": "using mock tool to trigger cancel and skip sequence"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool", "ask_for_clarification") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "` + testForgeName + `" },
"human_readable_thought": "requesting forge", "cumulative_summary": "forge analysis"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") && !utils.MatchAllOfSubString(prompt, "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "mocked post-iteration summary"}`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "任务执行引擎", "task_long_summary") && !utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary", "status_summary": "done", "task_short_summary": "completed", "task_long_summary": "task completed"}`))
				rsp.Close()
				return rsp, nil
			}

			unreachableCode = true
			log.Warnf("unexpected prompt in skip_after_cancel test, length=%d", len(prompt))
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "fallback"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithTools(mockTool),
		aicommon.WithShowForgeListInPrompt(true),
		aicommon.WithDisableDynamicPlanning(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test cancel then skip scenario",
		}
	}()

	// If the fix works, the flow should terminate well within 15s.
	// The original bug caused infinite memory_sync loops - a 15s timeout detects that.
	after := time.After(15 * time.Second)
	flowEnded := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC) {
				taskID := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..task_id"))
				if taskID != "" {
					currentTaskID = taskID
				}
			}

			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) ||
				e.Type == string(schema.EVENT_TYPE_FAIL_PLAN_AND_EXECUTION) {
				flowEnded = true
				break LOOP
			}

			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) && e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "completed" || status == "aborted" {
					flowEnded = true
					break LOOP
				}
			}

		case <-after:
			t.Fatal("DEADLOCK DETECTED: P&E did not terminate within 15s after cancel+skip sequence")
		}
	}

	// Wait for tool callback goroutine to finish before cleanup
	time.Sleep(500 * time.Millisecond)
	close(in)

	if atomic.LoadInt32(&toolCalled) == 0 {
		t.Fatal("mock tool was never called")
	}
	if unreachableCode {
		t.Fatal("AI callback reached unreachable code")
	}
	if !flowEnded {
		t.Fatal("flow did not end properly")
	}
	log.Infof("cancel+skip test passed: flow terminated gracefully without deadlock")
}
