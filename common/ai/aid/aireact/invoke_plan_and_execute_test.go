package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

func mockedRequestPlanAndExecuting_Normal(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, flag string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "` + flag + `" },
"human_readable_thought": "mocked thought for plan-exec", "cumulative_summary": "..cumulative-mocked for plan and exec.."}
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

func TestReAct_PlanAndExecute_Basic(t *testing.T) {
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
			return mockedRequestPlanAndExecuting_Normal(i, r, flag)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithHijackPERequest(func(ctx context.Context, payload string) error {
			planDo = true
			if payload == flag {
				planMatchFlag = true
			}
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

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	planStart := false
	planEnd := false
	switchedToAsync := false
	var oldCoordinatorId string
	var newCoordinatorId string
	// Track all unique CoordinatorIds to verify only two exist
	allCoordinatorIds := make(map[string]bool)
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				t.Fatal("Did not expect any tool use review event")
			}

			// Track all unique CoordinatorIds from events
			if e.CoordinatorId != "" {
				allCoordinatorIds[e.CoordinatorId] = true
			}

			// Capture the old CoordinatorId from the first event we receive
			if oldCoordinatorId == "" && e.CoordinatorId != "" {
				oldCoordinatorId = e.CoordinatorId
			}

			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				planStart = true
				// Extract the new coordinator_id from the event content
				result := utils.InterfaceToString(jsonpath.FindFirst(e.Content, `$..coordinator_id`))
				if result != "" {
					newCoordinatorId = result
				}
			}

			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				planEnd = true
			}

			if e.Type == string(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC) {
				switchedToAsync = true
			}

			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) && e.NodeId == "react_task_status_changed" {
				result := utils.InterfaceToString(jsonpath.FindFirst(e.Content, `$..react_task_now_status`))
				if result == "completed" {
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

	if !planEnd {
		t.Fatal("Expected plan end event")
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

	if !switchedToAsync {
		t.Fatal("Expected switchedToAsync to be true")
	}

	// Verify that the new CoordinatorId from plan execution is different from the old one
	if oldCoordinatorId == "" {
		t.Fatal("Expected to capture old CoordinatorId")
	}
	if newCoordinatorId == "" {
		t.Fatal("Expected to capture new CoordinatorId from plan execution event")
	}
	if oldCoordinatorId == newCoordinatorId {
		t.Fatalf("Expected new CoordinatorId (%s) to be different from old CoordinatorId (%s)", newCoordinatorId, oldCoordinatorId)
	}
	fmt.Printf("CoordinatorId verification passed: old=%s, new=%s\n", oldCoordinatorId, newCoordinatorId)

	// Since HijackPERequest is used, the actual Coordinator is not created,
	// so events will only have the ReAct's CoordinatorId.
	// The newCoordinatorId only appears in the event content JSON.
	// Verify that only ONE unique CoordinatorId exists in event headers (ReAct's)
	fmt.Printf("All unique CoordinatorIds found in event headers: %v (count: %d)\n", allCoordinatorIds, len(allCoordinatorIds))
	if len(allCoordinatorIds) != 1 {
		var ids []string
		for id := range allCoordinatorIds {
			ids = append(ids, id)
		}
		t.Fatalf("Expected exactly 1 unique CoordinatorId in event headers (ReAct's ID only when using HijackPERequest), but found %d: %v", len(allCoordinatorIds), ids)
	}
	// Verify the new coordinator_id in event content is different and not appearing in event headers
	if allCoordinatorIds[newCoordinatorId] {
		t.Fatalf("New CoordinatorId (%s) should NOT appear in event headers when using HijackPERequest", newCoordinatorId)
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	if !utils.MatchAllOfSubString(tl, flag) {
		t.Fatal("Did not match flag")
	}
	fmt.Println(tl)
	fmt.Println("--------------------------------------")
}
