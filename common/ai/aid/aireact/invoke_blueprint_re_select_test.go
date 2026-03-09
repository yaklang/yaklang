package aireact

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedRequireBlueprint_ChangeBlueprint(config aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string, forgeName1 string, forgeName2 string) (*aicommon.AIResponse, error) {

	rsp := config.NewAIResponse()
	if utils.MatchAllOfSubString(req.GetPrompt(), `require_ai_blueprint`, `require_tool`, "USER_QUERY", `directly_answer`, `ask_for_clarification`) {
		rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "require_ai_blueprint",
	"blueprint_payload": "` + forgeName1 + `",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	prompt := req.GetPrompt()

	if utils.MatchAllOfSubString(
		req.GetPrompt(), forgeName1,
		"Blueprint Schema:", `Blueprint Description:`,
		`call-ai-blueprint`,
	) && !utils.MatchAllOfSubString(prompt, `<|OLD_PARAMS_`) {
		rs := bytes.NewBufferString(`
{"@action": "call-ai-blueprint", "params": {
	"query": "...[` + codec.Sha256(flag) + `]...",
}, "human_readable_thought": "mocked thought` + flag + `", "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(
		req.GetPrompt(), "<|OLD_PARAMS_", "Current AI Blueprint", "change-ai-blueprint",
		"reasoning", "new_blueprint",
	) {
		rs := bytes.NewBufferString(`
{"@action": "change-ai-blueprint", "reasoning": "...[` + codec.Sha1(flag) + `]...",
"new_blueprint": "` + forgeName2 + `"}
`)
		rsp.EmitOutputStream(rs)
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") {
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "mocked summary"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "任务执行引擎", "task_long_summary") {
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary", "status_summary": "done", "task_short_summary": "completed", "task_long_summary": "completed"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println(prompt)
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "fallback"}`))
	rsp.Close()
	return rsp, nil
}

func TestReAct_RequireBlueprint_ChangeBlueprint(t *testing.T) {
	nonce := utils.RandStringBytes(16)
	forgeName1 := "mock_forge_alpha_" + nonce
	forgeName2 := "mock_forge_beta_" + nonce

	forge1 := &schema.AIForge{
		ForgeName:   forgeName1,
		ForgeType:   "yak",
		Description: "mock forge alpha " + forgeName1,
		InitPrompt:  "test init prompt",
		PlanPrompt:  `{"@action":"plan","query":"-","main_task":"test","main_task_goal":"test","tasks":[{"subtask_name":"t","subtask_goal":"t"}]}`,
	}
	forge2 := &schema.AIForge{
		ForgeName:   forgeName2,
		ForgeType:   "yak",
		Description: "mock forge beta " + forgeName2,
		InitPrompt:  "test init prompt",
		PlanPrompt:  `{"@action":"plan","query":"-","main_task":"test","main_task_goal":"test","tasks":[{"subtask_name":"t","subtask_goal":"t"}]}`,
	}
	db := consts.GetGormProfileDatabase()
	if err := yakit.CreateAIForge(db, forge1); err != nil {
		t.Fatalf("failed to create mock forge1: %v", err)
	}
	if err := yakit.CreateAIForge(db, forge2); err != nil {
		yakit.DeleteAIForgeByName(db, forgeName1)
		t.Fatalf("failed to create mock forge2: %v", err)
	}
	defer yakit.DeleteAIForgeByName(db, forgeName1)
	defer yakit.DeleteAIForgeByName(db, forgeName2)

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
			return mockedRequireBlueprint_ChangeBlueprint(i, r, flag, forgeName1, forgeName2)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithDisableDynamicPlanning(true),
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
	after := time.After(15 * time.Second)

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
						InteractiveJSONInput: `{"suggestion": "change_blueprint", "extra_prompt": "hhh"}`,
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

	ins.WaitForStream()
	timeline := ins.DumpTimeline()
	fmt.Println(timeline)
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	if !strings.Contains(timeline, codec.Sha256(flag)) {
		t.Fatal("timeline does not contain codec.Sha256(flag)", flag)
	}
	if !strings.Contains(timeline, codec.Sha1(flag)) {
		t.Fatal("timeline does not contain codec.Sha1(flag)", flag)
	}
	if !strings.Contains(timeline, forgeName2) {
		t.Fatal("timeline does not contain re-selection for forgeName2", flag)
	}
	fmt.Println(timeline)
}
