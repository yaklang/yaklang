package aireact

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_RequireBlueprint_ModifyParams(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	forgeExecute := false
	forgeHaveFlag := false

	abort, cancel := context.WithCancel(context.Background())
	defer cancel()
	ins, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedRequireBlueprint_BASIC(i, r, flag)
		}),
		WithDebug(false),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithReActHijackPlanRequest(func(ctx context.Context, planPayload string) error {
			forgeExecute = true
			if strings.Contains(planPayload, flag) {
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
					InteractiveJSONInput: `{"suggestion": "modify_params", "extra_prompt": "hhh"}`,
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
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	fmt.Println(timeline)
}
