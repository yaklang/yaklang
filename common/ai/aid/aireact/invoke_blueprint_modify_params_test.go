package aireact

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedRequireBlueprint_ModifyParams(config aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {

	rsp := config.NewAIResponse()
	if utils.MatchAllOfSubString(req.GetPrompt(), `require_ai_blueprint`, `require_tool`, "USER_QUERY", `directly_answer`, `ask_for_clarification`) {
		rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "require_ai_blueprint",
	"blueprint_payload": "xss",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(
		req.GetPrompt(), `xss`,
		"Blueprint Schema:", `Blueprint Description:`,
		`call-ai-blueprint`,
	) && !utils.MatchAllOfSubString(prompt, `<|OLD_PARAMS_`) {
		rs := bytes.NewBufferString(`
{"@action": "call-ai-blueprint", "params": {
	"query": "...[` + flag + `]...",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(
		req.GetPrompt(), `xss`,
		"Blueprint Schema:", `Blueprint Description:`,
		`call-ai-blueprint`, "<|OLD_PARAMS_",
	) {
		rs := bytes.NewBufferString(`
{"@action": "call-ai-blueprint", "params": {
	"query": "...[` + codec.Sha256(flag) + `]...",
}, "human_readable_thought": "mocked thought` + codec.Sha256(flag) + `", "cumulative_summary": "..cumulative-mocked` + codec.Sha256(flag) + `.."}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	fmt.Println(prompt)

	return rsp, nil
}

func TestReAct_RequireBlueprint_ModifyParams(t *testing.T) {
	// t.Skip()

	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeExecute := false
	forgeHaveFlag := false
	forgeHaveOldFlag := false

	abort, cancel := context.WithCancel(context.Background())
	defer cancel()
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedRequireBlueprint_ModifyParams(i, r, flag)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithHijackPERequest(func(ctx context.Context, planPayload string) error {
			forgeExecute = true
			if strings.Contains(planPayload, codec.Sha256(flag)) {
				forgeHaveFlag = true
			}
			if strings.Contains(planPayload, flag) {
				forgeHaveOldFlag = true
			}
			go func() {
				time.Sleep(time.Second * 3)
				cancel()
			}()
			return nil
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
	after := time.After(60 * time.Second)

	endforge := false
	reActFinished := false
	reviewCount := 0
	reviewed := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.GetType() == string(schema.EVENT_TYPE_ITERATION) {
				result := utils.InterfaceToInt(jsonpath.FindFirst(e.GetContent(), "$.current"))
				if result > 20 {
					break LOOP
				}
			}

			if e.GetType() == string(schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE) {
				fmt.Println(string(e.GetContent()))
				epid := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.id"))
				if reviewCount <= 0 {
					in <- &ypb.AIInputEvent{
						IsInteractiveMessage: true,
						InteractiveId:        epid,
						InteractiveJSONInput: `{"suggestion": "modify_params", "extra_prompt": "hhh"}`,
					}
					reviewCount++
					reviewed = true
				} else {
					in <- &ypb.AIInputEvent{
						IsInteractiveMessage: true,
						InteractiveId:        epid,
						InteractiveJSONInput: `{"suggestion": "continue", "extra_prompt": "hhh"}`,
					}
				}
				continue
			}

			if e.GetType() == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				endforge = true
			}

			if endforge && e.GetType() == string(schema.EVENT_TYPE_STRUCTURED) {
				if e.GetNodeId() == "react_task_status_changed" {
					if utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_now_status")) == "completed" {
						reActFinished = true
						break LOOP
					}
				}
			}
		case <-after:
			break LOOP
		case <-abort.Done():
			break LOOP
		}
	}

	if !reviewed {
		t.Fatal("no reviewed (modified params)")
	}

	if !forgeExecute {
		t.Fatal("forged plan and execute not executed")
	}

	if !forgeHaveFlag {
		t.Fatal("forged plan and execute not have flag")
	}

	if !reActFinished {
		t.Fatal("re-act not finished")
	}

	if !endforge {
		t.Fatal("not receive end of forge")
	}

	if forgeHaveOldFlag {
		t.Fatal("old params leaked")
	}

	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	if !strings.Contains(timeline, codec.Sha256(flag)) {
		t.Fatal("timeline does not contain codec.Sha256(flag)", flag)
	}
	fmt.Println(timeline)
}

func TestReAct_RequireBlueprint_InputParams(t *testing.T) {
	// t.Skip()

	flag := aitool.InvokeParams(map[string]interface{}{
		"key": ksuid.New().String(),
	})
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeExecute := false
	forgeHaveFlag := false

	abort, cancel := context.WithCancel(context.Background())
	defer cancel()
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedRequireBlueprint_ModifyParams(i, r, "")
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithHijackPERequest(func(ctx context.Context, planPayload string) error {
			forgeExecute = true
			if strings.Contains(codec.Sha256(planPayload), codec.Sha256(utils.InterfaceToString(flag))) {
				forgeHaveFlag = true
			}
			go func() {
				time.Sleep(time.Second * 3)
				cancel()
			}()
			return nil
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
	after := time.After(60 * time.Second)

	endforge := false
	reActFinished := false
	reviewed := false
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.GetType() == string(schema.EVENT_TYPE_ITERATION) {
				result := utils.InterfaceToInt(jsonpath.FindFirst(e.GetContent(), "$.current"))
				if result > 20 {
					break LOOP
				}
			}

			if e.GetType() == string(schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE) {
				fmt.Println(string(e.GetContent()))
				epid := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        epid,
					InteractiveJSONInput: fmt.Sprintf(`{"suggestion": "input_params", "params": %s}`, utils.InterfaceToString(flag)),
				}
				reviewed = true
				continue
			}

			if e.GetType() == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				endforge = true
			}

			if endforge && e.GetType() == string(schema.EVENT_TYPE_STRUCTURED) {
				if e.GetNodeId() == "react_task_status_changed" {
					if utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$.react_task_now_status")) == "completed" {
						reActFinished = true
						break LOOP
					}
				}
			}
		case <-after:
			break LOOP
		case <-abort.Done():
			break LOOP
		}
	}

	if !reviewed {
		t.Fatal("no reviewed (modified params)")
	}

	if !forgeExecute {
		t.Fatal("forged plan and execute not executed")
	}

	if !forgeHaveFlag {
		t.Fatal("forged plan and execute not have flag")
	}

	if !reActFinished {
		t.Fatal("re-act not finished")
	}

	if !endforge {
		t.Fatal("not receive end of forge")
	}

	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, utils.InterfaceToString(flag)) {
		t.Fatal("timeline does not contain flag", flag)
	}
	fmt.Println(timeline)
}
