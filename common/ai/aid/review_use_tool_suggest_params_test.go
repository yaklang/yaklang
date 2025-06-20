package aid

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_ToolUseReview_SuggestParams(t *testing.T) {
	coordinatorTestMutex.Lock()
	defer coordinatorTestMutex.Unlock()

	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	coordinator, err := NewCoordinator(
		"test-suggest-params",
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
			prompt := request.GetPrompt()

			// After user suggestion, AI should call with /new-path
			if utils.MatchAllOfSubString(prompt, `工具名称: ls`, `"/new-path"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/new-path"}}`))
				return rsp, nil
			}

			// AI first suggests ls with /abc-target, this is a call for params
			if utils.MatchAllOfSubString(prompt, `工具名称: ls`) && !strings.Contains(prompt, `当前任务:`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "ls", "params": {"path": "/abc-target"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "ls"}`))
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

	useToolReview := false
	useToolReviewPass := false
	count := 0
LOOP:
	for {
		select {
		case <-time.After(2 * time.Second):
			break LOOP
		case result := <-outputChan:
			count++
			if count > 100 {
				break LOOP
			}
			if result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
			}

			if result.Type == EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				useToolReview = true
				var a = make(aitool.InvokeParams)
				json.Unmarshal(result.Content, &a)
				if a.GetObject("params").GetString("path") == "/abc-target" && a.GetString("tool") == "ls" {
					inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion":        "suggest_params",
							"suggestion_params": `{"path": "/new-path"}`,
						},
					}
				}
			}

			if useToolReview && utils.MatchAllOfSubString(string(result.Content), "start to execute tool:", "ls", `"/new-path"`) {
				useToolReviewPass = true
				break LOOP
			}
		}
	}

	if !useToolReview {
		t.Fatal("tool review did not happen")
	}

	if !useToolReviewPass {
		t.Fatal("tool was not executed with new params")
	}
}
