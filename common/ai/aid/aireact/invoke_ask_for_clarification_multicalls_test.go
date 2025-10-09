package aireact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedClarification2(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	fmt.Println(prompt)
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") && !utils.MatchAllOfSubString(prompt, `ask_for_clarification`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "...mocked-directly-answer-` + flag + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool", "ask_for_clarification") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "...mocked question...", "options": ["` + flag + `", "option2", "option3"]} },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_AskForClarification_multicall(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	_ = flag
	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedClarification2(i, r, flag)
		}),
		WithDebug(false),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithUserInteractive(true),
		WithUserInteractiveLimitedTimes(4),
		WithMaxIterations(7),
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
	after := time.After(20 * time.Second)

	var iid string
	var flagMatched bool
	var normalCompleted bool
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				title := utils.InterfaceToString(jsonpath.Find(string(e.Content), "$..prompt_title"))
				if utils.MatchAnyOfSubString(title, flag) {
					flagMatched = true
				}
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.GetNodeId() == "react_task_status_changed" {
				status := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$..react_task_now_status"))
				if status == "completed" {
					normalCompleted = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if iid == "" {
		t.Fatal("interactive ID should not be empty")
	}
	_ = flag
	if !flagMatched {
		t.Fatalf("expected flag %s to be matched in interactive prompt, but it was not found", flag)
	}

	if !normalCompleted {
		t.Fatal("expected normal to be completed in interactive prompt, but it was not found")
	}

	tl := ins.DumpTimeline()
	fmt.Println(tl)

	if !utils.MatchAllOfSubString(tl, `ReAct loop finished END[5]`) {
		t.Fatal("iteration count should be 5")
	}
	if !utils.MatchAllOfSubString(tl, `ai directly answer`) {
		t.Fatal("ai directly answer not found")
	}
	if ret := strings.Count(tl, `...mocked question...`); ret < 4 {
		t.Fatal("mocked question found, should be 4 times, got ", ret)
	}
}
