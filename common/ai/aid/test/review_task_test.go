package test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_TaskReview(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "now", `{"@action": "call-tool", "tool": "now", "params": {}}`)
		}))
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	useToolReview := false
	useToolReviewPass := false
	taskReview := false
	taskReviewPass := false
	count := 0
LOOP:
	for {
		select {
		case <-time.After(30 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 1000 {
				break LOOP
			}

			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				time.Sleep(100 * time.Millisecond)
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetString("tool") == "now" && a.GetString("tool_description") != "" {
					useToolReview = true
					time.Sleep(100 * time.Millisecond)
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
					continue
				}
			}

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to invoke tool:", "now") {
				useToolReviewPass = true
			}

			if useToolReviewPass {
				if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
					fmt.Println("task result:" + result.String())
					time.Sleep(200 * time.Millisecond)
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
					taskReview = true
					continue
				}
			}

			if taskReview {
				fmt.Println("task result:" + result.String())
				if utils.MatchAllOfSubString(string(result.Content), "start to handle review task event:") {
					taskReviewPass = true
					break LOOP
				}
			}
		}
	}

	if !useToolReview {
		t.Fatal("tool review fail")
	}

	if !useToolReviewPass {
		t.Fatal("tool review not finished")
	}

	if !taskReview {
		t.Fatal("task review fail")
	}

	if !taskReviewPass {
		t.Fatal("task review not finished")
	}
}
