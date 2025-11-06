package test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestCoordinator_ConfigHotpatch(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(20)
	keywordsToken := uuid.New().String()
	newKeywordsToken := uuid.New().String()
	hotpatchOptionChan := chanx.NewUnlimitedChan[aicommon.ConfigOption](ctx, 10)
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithKeywords(keywordsToken),
		aicommon.WithHotPatchOptionChan(hotpatchOptionChan),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
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
			time.Sleep(100 * time.Millisecond)
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		ins.Run()
	}()

	originConfigCheck := false
	optionHotpatchOk := false
	hotpatchUpdateOptionCheck := true

LOOP:
	for {
		select {
		case result := <-outputChan:
			if result.Type == schema.EVENT_TYPE_AID_CONFIG {
				if strings.Contains(string(result.Content), keywordsToken) {
					originConfigCheck = true
					hotpatchOptionChan.SafeFeed(aicommon.WithKeywords(newKeywordsToken))
					optionHotpatchOk = true
				}
				if optionHotpatchOk && strings.Contains(string(result.Content), newKeywordsToken) {
					hotpatchUpdateOptionCheck = true
					break LOOP
				}
			}

			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				time.Sleep(100 * time.Millisecond)
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

		case <-time.After(time.Second * 10):
			t.Fatal("timeout")
		}
	}

	if !originConfigCheck {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !hotpatchUpdateOptionCheck {
		t.Fatal("consumption check failed")
	}
}
