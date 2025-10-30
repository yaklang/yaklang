package aireact

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedClarification(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool", "ask_for_clarification") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "ask_for_clarification", "ask_for_clarification_payload": {"question": "...mocked question...", "options": ["` + flag + `", "option2", "option3"]} },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_AskForClarification(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	_ = flag
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedClarification(i, r, flag)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
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
	after := time.After(5 * time.Second)

	var iid string
	var flagMatched bool
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
					break LOOP
				}
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
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
}
