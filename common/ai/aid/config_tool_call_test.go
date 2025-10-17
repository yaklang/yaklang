package aid

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestLocalUserCancel(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("Github Actions")
	}
	for i := 0; i < 10; i++ {
		TestConfig_EmitToolCallUserCancel(t)
	}
}

func TestConfig_EmitToolCallUserCancel(t *testing.T) {
	inputChan := make(chan *InputEvent, 10)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithTool(TimeDelayTool()),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: `, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "delay", "params": {"delay": 100000000}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "delay"}`))
				return rsp, nil
			}

			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
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

	toolCallWatcherEventCheck := false
	toolCallCancelCheck := false

	count := 0
LOOP:
	for {
		select {
		case <-time.After(11111110 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 100 {
				break LOOP
			}
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					IsInteractive: true,
					Id:            result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_CALL_WATCHER {
				if utils.MatchAllOfSubString(result.Content, "delay", "enough-cancel") {
					toolCallWatcherEventCheck = true
					inputChan <- &InputEvent{
						IsInteractive: true,
						Id:            result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "enough-cancel",
						},
					}
				}
				continue
			}

			if toolCallWatcherEventCheck && result.Type == schema.EVENT_TOOL_CALL_USER_CANCEL {
				toolCallCancelCheck = true
				break LOOP
			}
			fmt.Println("review task result:" + result.String())
		}
	}
	require.True(t, toolCallWatcherEventCheck, "tool call watcher should be triggered")
	require.True(t, toolCallCancelCheck, "tool call should be cancelled by user input")
}

func TestConfig_EmitToolCallOK(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithTool(TimeDelayTool()),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: `, `"call-tool"`, "const") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "delay", "params": {"delay": 1}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "delay"}`))
				return rsp, nil
			}

			fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())
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

	toolCallWatcherEventCheck := false
	watcherId := ""
	watcherReleaseCheck := false
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
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				continue
			}

			if result.Type == schema.EVENT_TYPE_TOOL_CALL_WATCHER {
				watcherId = result.GetInteractiveId()
				if utils.MatchAllOfSubString(result.Content, "delay", "enough-cancel") {
					toolCallWatcherEventCheck = true
				}
			}

			if toolCallWatcherEventCheck {
				if result.Type == schema.EVENT_TYPE_REVIEW_RELEASE && result.GetInteractiveId() == watcherId {
					watcherReleaseCheck = true
					break LOOP
				}
			}
			fmt.Println("review task result:" + result.String())
		}
	}
	require.True(t, toolCallWatcherEventCheck, "tool call watcher should be triggered")
	require.True(t, watcherReleaseCheck, "watcher should be released after tool call finished")
}
