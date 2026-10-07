package aireact

import (
	"bytes"
	"encoding/json"

	"io"
	"reflect"

	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_ToolUse_EmitFinalInvokeParams(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 32)
	toolName := "echo_" + ksuid.New().String()
	expectedInput := "param_" + ksuid.New().String()

	var finalDecisionCount int32

	var invokedParams aitool.InvokeParams
	var toolCalled bool
	echoTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			invokedParams = make(aitool.InvokeParams)
			for k, v := range params {
				if k == "runtime_id" {
					continue
				}
				invokedParams[k] = v
			}
			return params.GetAnyToString("input"), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if isToolCallReasonLiteForgePrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(mockedToolCallReasonActionJSON))
				rsp.Close()
				return rsp, nil
			}

			if isPrimaryDecisionPrompt(prompt) {
				// verification 收缩为纯观测角色后, satisfied=true 不再自动退出.
				// 工具调用过一次后(finalDecisionCount>=2), 主循环再次决策时
				// 主动 finish 收口.
				if atomic.AddInt32(&finalDecisionCount, 1) >= 2 {
					rsp := i.NewAIResponse()
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: task done after final invoke params"}`))
					rsp.Close()
					return rsp, nil
				}
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "directly_call_tool", "directly_call_tool_name": "` + toolName + `", "directly_call_tool_params": { "input" : "` + expectedInput + `" },
"human_readable_thought": "mocked thought for final param emit", "cumulative_summary": "..cumulative-mocked for final param emit.."}
`))
				rsp.Close()
				return rsp, nil
			}

			if isToolParamGenerationPrompt(prompt, toolName) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "input" : "` + expectedInput + `" }}`))
				rsp.Close()
				return rsp, nil
			}

			if isVerifySatisfactionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "final invoke params captured"}`))
				rsp.Close()
				return rsp, nil
			}

			// verification 收缩为纯观测角色后, satisfied=true 不再自动退出, 主动 finish 收口.
			if isDirectAnswerPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: finish after final invoke params"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(echoTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "abc",
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var paramEventCount int
	var paramEventCallToolID string
	var startEventCallToolID string
	var resultEventCallToolID string
	var eventParams aitool.InvokeParams
	var taskCompleted bool

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TOOL_CALL_PARAM) {
				paramEventCount++
				paramEventCallToolID = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.call_tool_id"))
				var payload struct {
					Params aitool.InvokeParams `json:"params"`
				}
				if err := json.Unmarshal(e.Content, &payload); err != nil {
					t.Fatalf("failed to unmarshal tool_call_param event: %v", err)
				}
				eventParams = payload.Params
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				startEventCallToolID = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.call_tool_id"))
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_RESULT) {
				resultEventCallToolID = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.call_tool_id"))
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCalled {
		t.Fatal("expected final tool invocation, but tool was not called")
	}
	if !taskCompleted {
		t.Fatal("expected react task to complete")
	}
	if paramEventCount != 1 {
		t.Fatalf("expected exactly 1 tool_call_param event for final invoke, got %d", paramEventCount)
	}
	if paramEventCallToolID == "" {
		t.Fatal("expected tool_call_param event to carry call tool id")
	}
	if startEventCallToolID == "" {
		t.Fatal("expected tool_call_start event to carry call tool id")
	}
	if resultEventCallToolID == "" {
		t.Fatal("expected tool_call_result event to carry call tool id")
	}
	if paramEventCallToolID != startEventCallToolID || paramEventCallToolID != resultEventCallToolID {
		t.Fatalf("tool_call_param call_tool_id mismatch: param=%s start=%s result=%s", paramEventCallToolID, startEventCallToolID, resultEventCallToolID)
	}
	if eventParams == nil {
		t.Fatal("expected tool_call_param event to contain params")
	}
	if invokedParams == nil {
		t.Fatal("expected tool callback to receive invoke params")
	}
	if !reflect.DeepEqual(map[string]any(eventParams), map[string]any(invokedParams)) {
		t.Fatalf("tool_call_param params mismatch: event=%v invoked=%v", eventParams, invokedParams)
	}
	if eventParams.GetString("input") != expectedInput {
		t.Fatalf("expected tool_call_param to emit random final input %q, got %q", expectedInput, eventParams.GetString("input"))
	}
}
