package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCoordinator_PlanInteraction_Timeline(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)

	token := utils.RandStringBytes(100)

	userInteractTrigger := false

	timelineShowed := false

	ins, err := NewCoordinator(
		"test",
		WithAllowPlanUserInteract(true),
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()

			prompts := request.GetPrompt()

			if strings.Contains(prompts, token) {
				timelineShowed = true
			}

			if utils.MatchAllOfSubString(prompts, `"require-user-interact"`) {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "require-user-interact", "question": "你喜欢红色还是蓝色？", "options": [
	{"option_name": "红色", "option_description": "红色"},
{"option_name": "蓝色", "option_description": "蓝色"}, { "option_name": "` + token + `", "option_description": "` + token + `"}
]}`))
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
	go func() {
		ins.Run()
	}()

LOOP:
	for {
		if timelineShowed {
			break LOOP
		}
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if result.Type == EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				if result.GetInteractiveId() != "" && strings.Contains(result.String(), token) {
					inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"extra_prompt": token,
						},
					}
					userInteractTrigger = true
					continue
				} else {
					t.Fatal("unexpected interactive event: " + result.String())
				}
			}

			_ = inputChan
		case <-time.After(time.Second * 10):
			t.Fatal("timeout")
		}
	}

	if !userInteractTrigger {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !timelineShowed {
		t.Fatal("timeline not showed, please check your AI model or prompt")
	}
}

func TestCoordinator_PlanInteraction(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)

	token := utils.RandStringBytes(100)

	userInteractTrigger := false

	ins, err := NewCoordinator(
		"test",
		WithAllowPlanUserInteract(true),
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()

			prompts := request.GetPrompt()
			if utils.MatchAllOfSubString(prompts, `"require-user-interact"`) {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "require-user-interact", "question": "你喜欢红色还是蓝色？", "options": [
	{"option_name": "红色", "option_description": "红色"},
{"option_name": "蓝色", "option_description": "蓝色"}, { "option_name": "` + token + `", "option_description": "` + token + `"}
]}`))
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
	go func() {
		ins.Run()
	}()

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if result.Type == EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
				if result.GetInteractiveId() != "" && strings.Contains(result.String(), token) {
					inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"extra_prompt": token,
						},
					}
					userInteractTrigger = true
					break LOOP
				} else {
					t.Fatal("unexpected interactive event: " + result.String())
				}
			}

			_ = inputChan
		case <-time.After(time.Second * 10):
			t.Fatal("timeout")
		}
	}

	if !userInteractTrigger {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}
