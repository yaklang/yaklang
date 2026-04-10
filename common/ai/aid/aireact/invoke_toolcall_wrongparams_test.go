package aireact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
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

func mockedToolCallingWrongParam_Normal(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if isPrimaryDecisionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isToolParamGenerationPrompt(prompt, toolName) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "input" : "mocked-echo-params" }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "<|OLD_PARAMS_", "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		rsp.Close()
		return rsp, nil
	}

	if strings.Contains(prompt, "重新生成一套参数") || strings.Contains(prompt, "参数名不匹配") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		rsp.Close()
		return rsp, nil
	}

	if strings.Contains(prompt, "用户中断了工具执行") || strings.Contains(prompt, "请根据你刚才执行的所有步骤") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "directly answer after '` + toolName + `' require and user reject it..........."}`))
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	if isDirectAnswerPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "directly answer after '` + toolName + `' require and user reject it..........."}`))
		rsp.Close()
		return rsp, nil

	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_ToolUse_WrongParams(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	sleepToolCalled := false
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			sleepToolCalled = true
			sleepInt := params.GetFloat("seconds", 0.3)
			if sleepInt <= 0 {
				sleepInt = 0.3
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	echoCalled := false
	echoTool, err := aitool.New(
		"echo",
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			i := params.GetAnyToString("input")
			if !echoCalled {
				echoCalled = true
			}
			return i, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingWrongParam_Normal(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool, echoTool),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	reviewed := false
	reviewReleased := false
	toolCallOutputEvent := false
	reActFinished := false
	var iid string

	wrongParamDone := false
	normalReview := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				fmt.Println(string(e.Content))
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				if !wrongParamDone {
					in <- &ypb.AIInputEvent{
						IsInteractiveMessage: true,
						InteractiveId:        utils.InterfaceToString(iid),
						InteractiveJSONInput: `{"suggestion": "wrong_params"}`,
					}
					wrongParamDone = true
				} else {
					normalReview = true
					in <- &ypb.AIInputEvent{
						IsInteractiveMessage: true,
						InteractiveId:        utils.InterfaceToString(iid),
						InteractiveJSONInput: `{"suggestion": "continue"}`,
					}
				}
			}

			if e.Type == string(schema.EVENT_TYPE_REVIEW_RELEASE) {
				gotId := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				if gotId == iid {
					reviewReleased = true
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_USER_CANCEL) || e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallOutputEvent = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					reActFinished = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	if !sleepToolCalled {
		t.Fatal("Expected to have at least one tool call, but got none")
	}

	if echoCalled {
		t.Fatal("Did not expect echo tool to be called, but it was")
	}

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one output event, but got none")
	}

	if !reActFinished {
		t.Fatal("Expected to have at least one re-act terminal event, but got none")
	}

	if !normalReview {
		t.Fatal("Expected to have normal review after wrong param, but got none")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !strings.Contains(tl, `mocked thought for tool calling`) {
		t.Fatal("timeline does not contain mocked thought")
	}
	if !utils.MatchAllOfSubString(tl, `system-question`, "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}
	if !utils.MatchAllOfSubString(tl, `ReAct iteration 1`, `ReAct Iteration Done[1]`) {
		t.Fatal("timeline does not contain ReAct iteration")
	}
	if !utils.MatchAllOfSubString(tl, `Regenerating parameters for tool: sleep`) {
		t.Fatal("timeline does not contain Regenerating parameters for tool: sleep")
	}
	fmt.Println("--------------------------------------")
}

func TestReAct_ToolUse_EmitFinalInvokeParams(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 32)
	toolName := "echo_" + ksuid.New().String()
	expectedInput := "param_" + ksuid.New().String()

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
			if isPrimaryDecisionPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
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

			if isDirectAnswerPrompt(prompt) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "done"}`))
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
