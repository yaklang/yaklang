package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
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

	if utils.MatchAllOfSubString(prompt, "directly_answer", "plan_request_payload", "require_tool") {
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

	if utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") && !utils.MatchAllOfSubString(prompt, "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "mocked post-iteration summary"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_PlanAndExecute_MultiPlan(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 100)
	out := make(chan *ypb.AIOutputEvent, 10000)

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
			if strings.Contains(payload, flag) {
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

	// 关键词: TestReAct_PlanAndExecute_MultiPlan, github_actions_timeout
	// CI 上 ReAct 主循环 + post-iteration summary mock 至少需要 2 轮 AI 调用 +
	// 事件投递，1s 太紧导致 EVENT_TYPE_RESULT 还没派发到 out channel 测试就退出。
	// 这里改为 5s，仍远小于本地 10s 上限，不会显著拉长 CI。
	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	planStart := false
	planEnd := false
	var iid string
	_ = iid
	var processCount = 0
	directlyAnswer := false
	var answerStream bytes.Buffer
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
			if e.Type == string(schema.EVENT_TYPE_STREAM) && e.NodeId == "re-act-loop-answer-payload" {
				answerStream.Write(e.GetContent())
				if strings.Contains(answerStream.String(), "mocked answer directly after plan") ||
					strings.Contains(answerStream.String(), "mocked post-iteration summary") {
					directlyAnswer = true
					break LOOP
				}
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
