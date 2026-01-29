package aireact

import (
	"bytes"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_PlanLoop_Basic(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	promptReceived := make(chan string, 1)
	finished := make(chan struct{})

	go func() {
		for e := range out {
			t.Logf("Received output event: %s", e.String())
		}
	}()
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			select {
			case promptReceived <- prompt:
			default:
			}

			// 返回一个 finish action 完成测试
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "测试完成"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithFocus(schema.AI_REACT_LOOP_NAME_PLAN),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
			if e.Type == schema.EVENT_TYPE_SUCCESS_REACT {
				close(finished)
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "测试计划循环的提示词",
		}
	}()

	// 等待 prompt 接收
	select {
	case prompt := <-promptReceived:
		t.Logf("Received prompt, checking for prohibited actions...")

		// 验证 prompt 不应该包含这些 action
		prohibitedActions := []string{
			schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT,   // "require_ai_blueprint"
			schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,           // "require_tool"
			schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION, // "request_plan_and_execution"
		}

		for _, action := range prohibitedActions {
			if utils.MatchAllOfSubString(prompt, action) {
				t.Errorf("Plan Loop prompt should NOT contain action '%s', but it does", action)
			}
		}

		t.Log("✓ Plan Loop prompt correctly excludes prohibited actions")

	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for prompt")
	}

	// 等待任务完成
	select {
	case <-finished:
		t.Log("✓ Test completed successfully")
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for task completion")
	}
}
