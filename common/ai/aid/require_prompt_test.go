package aid

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestCoodinator_AllowRequireForUserInteract(t *testing.T) {
	// 首先从 coordinator 开始
	// 测试需要尊崇如下几个要点，任务随便是啥都行，只执行第一个工具为止
	// 第一个工具 require 为 require-user-interact，
	// 这个工具比较特殊：无法用户审核，因为它本身就包含了用户交互

	token1 := utils.RandStringBytes(200)
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	checkToken1 := false
	interactiveCheck := false
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: require-user-interact`, `"call-tool"`, "interactive_type") {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "require-user-interact", "params": {"prompt": "你好` + token1 + `", "interactive_type": "text", "options": []}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "require-user-interact"}`))
				return rsp, nil
			}

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
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

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
			if result.Type == EVENT_TYPE_CONSUMPTION {
				continue
			}

			if checkToken1 {
				fmt.Println("result:" + result.String())
				if result.Type == EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
					interactiveCheck = true
					break LOOP
				}
			}

			if result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				t.Fatal("no tool review required")
			}

			if !checkToken1 && strings.Contains(result.String(), token1) {
				checkToken1 = true
				continue
			}

		}
	}

	assert.True(t, interactiveCheck, "interactive check failed")
}
