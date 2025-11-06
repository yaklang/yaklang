package test

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/davecgh/go-spew/spew"
)

func TestCoordinator_ReviewPlan(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
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
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
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
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), "plan", "ask_for_clarification") {
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
			if strings.Contains(result.String(), `目录中占用存储空间最多的文件，并展示其完整路径与大小信息`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				time.Sleep(time.Millisecond * 50)
				inputChan.SafeFeed(SuggestionInputEvent(result.GetInteractiveId(), "incomplete", ""))
				continue
			}

			if utils.MatchAllOfSubString(result.String(), "ABC", "CCC", "BCD") &&
				result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
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

func TestCoordinator_ReviewPlan_Incomplete_2(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	extraPromptToken := utils.RandStringBytes(100)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			fmt.Println(request.GetPrompt())
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if !strings.Contains(request.GetPrompt(), extraPromptToken) {
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
			if strings.Contains(result.String(), `目录中占用存储空间最多的文件，并展示其完整路径与大小信息`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				time.Sleep(time.Millisecond * 50)
				inputChan.SafeFeed(SuggestionInputEvent(result.GetInteractiveId(), "incomplete", extraPromptToken))
				continue
			}

			if utils.MatchAllOfSubString(result.String(), "ABC", "CCC", "BCD") &&
				result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
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

func TestCoordinator_ReviewPlan_CreateSubtask(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	extraPromptToken := utils.RandStringBytes(100)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			fmt.Println(request.GetPrompt())
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if !strings.Contains(request.GetPrompt(), extraPromptToken) {
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
    "@action": "plan-create-subtask",
    "subtasks": [
        {
			"parent_index": "1-1",
            "name": "ABC1",
            "goal": "ABC1"
        },
		{
			"parent_index": "1-1",
            "name": "ABC2",
            "goal": "ABC2"
        },
        {
			"parent_index": "1-2",
            "name": "1-2-1",
            "goal": "1-2-2details"
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
			if strings.Contains(result.String(), `目录中占用存储空间最多的文件，并展示其完整路径与大小信息`) && !strings.Contains(result.String(), `1-2-2details`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				time.Sleep(time.Millisecond * 50)
				inputChan.SafeFeed(SuggestionInputEventEx(result.GetInteractiveId(), map[string]any{
					"suggestion":   "create-subtask",
					"target_plans": []string{"1-1", "1-2"},
					"extra_prompt": extraPromptToken,
				}))
				continue
			}

			if utils.MatchAllOfSubString(result.String(), "ABC1", "ABC2", "1-2-2details", "扫描目录结构") &&
				result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
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
