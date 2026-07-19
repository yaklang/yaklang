package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCoordinator_TaskReview(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	// toolCalled 记录是否已经完成过一次工具调用. verification 收缩为纯观测
	// 角色后, 任务退出由 AI 主动 finish 决定 (不再由 verification 满意即退);
	// 本测试的 mock 在工具调用过一次后, 下一轮 next-action 直接 finish, 模拟
	// "AI 判断任务完成后主动调 finish"的新正确行为.
	var toolCalled int32
	coordinator, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			// 工具已调用过一次后, 主循环再问 next-action 时主动 finish 收口
			if isNextActionDecisionPrompt(prompt) && atomic.LoadInt32(&toolCalled) > 0 {
				rsp := i.NewAIResponse()
				defer rsp.Close()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "mocked: task done after one tool call"}`))
				return rsp, nil
			}
			rsp, err := mockedToolCalling(i, r, "now", `{"@action": "call-tool", "tool": "now", "params": {}}`)
			if err == nil && isToolParamGenerationPrompt(prompt, "now") {
				atomic.AddInt32(&toolCalled, 1)
			}
			return rsp, err
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
