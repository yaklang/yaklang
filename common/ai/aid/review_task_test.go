package aid

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_TaskReview(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				time.Sleep(100 * time.Millisecond)
				rsp.Close()
			}()
			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), `["short_summary", "long_summary"]`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "short_summary": "short", "long_summary": "long"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), `"@action"`, `"plan"`) {
				rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在给定路径下寻找体积最大的文件",
    "main_task_goal": "识别 /Users/v1ll4n/Projects/yaklang 目录中占用存储空间最多的文件，并展示其完整路径与大小信息",
    "tasks": [
        {
            "subtask_name": "扫描目录结构",
            "subtask_goal": "递归遍历 /Users/v1ll4n/Projects/yaklang 目录下所有文件，记录每个文件的位置和占用空间"
        },
        {
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), "continue-current-task", "proceed-next-task") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "proceed-next-task"}`))
				return rsp, nil
			}
			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: now`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "now", "params": {}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "now"}`))
				return rsp, nil
			}
			rsp.EmitOutputStream(strings.NewReader(`TODO`))
			return rsp, nil
		}),
	)
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
			if count > 100 {
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
