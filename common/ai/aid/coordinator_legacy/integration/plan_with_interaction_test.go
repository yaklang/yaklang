package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCoordinator_PlanInteraction_Timeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	outputChan := make(chan *schema.AiOutputEvent, 200)

	token := utils.RandStringBytes(100)

	userInteractTrigger := false

	timelineShown := make(chan struct{}, 1)

	ins, err := coordinator_legacy.NewCoordinatorContext(ctx,
		"test",
		testAIRetryWaitOption(),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			case <-ctx.Done():
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()

			prompts := request.GetPrompt()

			if strings.Contains(prompts, token) {
				select {
				case timelineShown <- struct{}{}:
				default:
				}
			}

			if utils.MatchAllOfSubString(prompts, `"ask_for_clarification"`) {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "ask_for_clarification", "ask_for_clarification_payload" : { "question": "你喜欢红色还是蓝色？", "options": [
	{"option_name": "红色", "option_description": "红色"},
{"option_name": "蓝色", "option_description": "蓝色"}, { "option_name": "` + token + `", "option_description": "` + token + `"}
]}}`))
				rsp.Close()
				return rsp, nil
			}

			rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "在指定目录中找到最大的文件",
    "main_task_goal": "明确 /Users/v1ll4n/Projects/yaklang 目录下哪个文件占用空间最大，并输出该文件的路径和大小",
    "tasks": [
        {
            "subtask_name": "遍历目标目录",
            "subtask_goal": "递归扫描 /Users/v1ll4n/Projects/yaklang 目录，获取所有文件的路径和大小"
        },
        {
            "subtask_name": "筛选最大文件",
            "subtask_goal": "根据文件大小比较，确定目录中占用空间最大的文件"
        },
        {
            "subtask_name": "输出结果",
            "subtask_goal": "将最大文件的路径和大小以可读格式输出"
        }
    ]
}
			`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ins.Run() }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("coordinator did not stop after cancellation")
		}
	})

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				if result.GetInteractiveId() != "" && strings.Contains(result.String(), token) {
					inputChan.SafeFeed(SuggestionInputEvent(result.GetInteractiveId(), "", token))
					userInteractTrigger = true
					continue
				} else {
					t.Fatal("unexpected interactive event: " + result.String())
				}
			}

			if utils.MatchAllOfSubString(result.String(), `react_task_created`, `plan-task`) &&
				!strings.Contains(result.String(), `plan-task_intent`) {
				t.Fatal("should not create plan build task")
			}
			_ = inputChan
		case <-timelineShown:
			break LOOP
		case <-ctx.Done():
			t.Fatal("timeout waiting for the user's reply in a subsequent prompt")
		}
	}

	if !userInteractTrigger {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}
