package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	"github.com/davecgh/go-spew/spew"
)

func TestCoordinator_ReviewPlan(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(request *AIRequest) (*AIResponse, error) {
			rsp := NewAIResponse()
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

	parsedTask := false

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			spew.Dump(result)
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
				break LOOP
			}

			_ = inputChan
		case <-time.After(time.Second * 10):
			t.Fatal("timeout")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}

func TestCoordinator_ReviewPlan_Incomplete(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(request *AIRequest) (*AIResponse, error) {
			rsp := NewAIResponse()
			defer func() {
				time.Sleep(100 * time.Millisecond)
				rsp.Close()
			}()

			if !strings.Contains(request.GetPrompt(), "incomplete") {
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
            "subtask_name": "确定最大文件",
            "subtask_goal": "通过比对文件大小数据，找出占用空间最多的那个文件"
        },
        {
            "subtask_name": "格式化输出",
            "subtask_goal": "以人类易读的方式显示最大文件的完整路径及其大小信息"
        }
    ]
}
				`))
				return rsp, nil
			}
			rsp.EmitOutputStream(strings.NewReader(`
{
    "@action": "plan",
    "query": "找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
    "main_task": "CCC",
    "main_task_goal": "CCC",
    "tasks": [
        {
            "subtask_name": "ABC",
            "subtask_goal": "ABC"
        },
        {
            "subtask_name": "BCD",
            "subtask_goal": "BCD"
        }
    ]
}
			`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		ins.Run()
	}()

	parsedTask := false
	regeneratePlan := false
	_ = regeneratePlan
LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `目录中占用存储空间最多的文件，并展示其完整路径与大小信息`) && strings.Contains(result.String(), `"incomplete"`) {
				parsedTask = true
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "incomplete",
					},
				}
				continue
			}

			if utils.MatchAllOfSubString(result.String(), "ABC", "CCC", "BCD") &&
				result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				regeneratePlan = true
				break LOOP
			}
			_ = inputChan
		case <-time.After(time.Second * 60):
			t.Fatal("timeout")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}

	if !regeneratePlan {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}
