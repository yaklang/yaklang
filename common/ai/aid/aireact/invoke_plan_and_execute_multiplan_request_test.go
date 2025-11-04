package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedRequestPlanAndExecuting_MultiPlans(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	fmt.Println(prompt)

	// If the prompt contains the error message about another plan execution task running,
	// return a directly_answer action instead
	if utils.MatchAllOfSubString(prompt, "another plan execution task is already running") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "` + "mocked answer directly after plan" + `" },
"human_readable_thought": "mocked thought for answer", "cumulative_summary": "..cumulative-mocked for answer after plan and exec.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "` + flag + `" },
"human_readable_thought": "mocked thought for plan-exec", "cumulative_summary": "..cumulative-mocked for plan and exec.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "` + "mocked answer directly after plan" + `" },
"human_readable_thought": "mocked thought for answer", "cumulative_summary": "..cumulative-mocked for answer after plan and exec.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_PlanAndExecute_MultiPlan(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
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

	planDo := false
	planMatchFlag := false
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedRequestPlanAndExecuting_MultiPlans(i, r, flag)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithTools(sleepTool),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			planDo = true
			if payload == flag {
				planMatchFlag = true
			}
			time.Sleep(time.Second * 30)
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 2; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(1)
	}
	after := time.After(du * time.Second)

	planStart := false
	planEnd := false
	var iid string
	_ = iid
	var processCount = 0
	directlyAnswer := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				t.Fatal("Did not expect any tool use review event")
			}

			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				planStart = true
			}

			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				resultRaw := jsonpath.FindFirst(e.Content, `$..react_task_now_status`)
				result := utils.InterfaceToString(resultRaw)
				if result == "processing" {
					processCount++
				}
			}

			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnd = true
			}

			if e.Type == string(schema.EVENT_TYPE_RESULT) {
				directlyAnswer = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !planStart {
		t.Fatal("Expected plan start event")
	}

	if planEnd {
		t.Fatal("Did not expect plan end event")
	}

	if toolCalled {
		t.Fatal("Did not expect tool to be called")
	}

	if !planDo {
		t.Fatal("Expected planDo to be true")
	}

	if !planMatchFlag {
		t.Fatal("Expected planMatchFlag to be true")
	}

	if !directlyAnswer {
		t.Fatal("Expected directly answer to be true")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	if !utils.MatchAllOfSubString(tl, flag) {
		t.Fatal("Did not match flag")
	}
	fmt.Println(tl)
	fmt.Println("--------------------------------------")
}
