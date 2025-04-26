package aid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func recoverPlan(t *testing.T, uuid string) {
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	recoverCtx, recoverCancel := context.WithCancel(context.Background())
	defer recoverCancel()
	ord, err := NewFastRecoverCoordinatorContext(
		recoverCtx,
		uuid,
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBuffer([]byte("Hello, world!")))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := ord.Run(); err != nil {
		}
	}()

	parsedTask := false
LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				break LOOP
			}
		case <-time.After(time.Second * 3):
			t.Fatal("timeout")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}

func TestCoordinator_RecoverCase(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)
	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
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
	consumptionCheck := false
LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true

				end := false
				for {
					if end {
						break
					}
					select {
					case inputChan <- &InputEvent{
						Id: result.GetInteractiveId(),
						Params: aitool.InvokeParams{
							"suggestion": "continue",
						},
					}:
						end = true
						continue
					case <-time.After(3 * time.Second):
						log.Warn("timeout for write to inputChan, retry it")
					}
				}
				continue
			}
			if result.Type == EVENT_TYPE_CONSUMPTION {
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					t.Fatal(err)
				}
				inputConsumption := int64(0)
				outputConsumption := int64(0)
				if i, ok := data["input_consumption"]; ok {
					inputConsumption = int64(utils.InterfaceToInt(i))
				}
				if o, ok := data["output_consumption"]; ok {
					outputConsumption = int64(utils.InterfaceToInt(o))
				}
				if inputConsumption > 0 && outputConsumption > 0 {
					consumptionCheck = true
					break LOOP
				}
			}
		case <-time.After(time.Second * 10):
			t.Fatal("timeout")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !consumptionCheck {
		t.Fatal("consumption check failed")
	}
	recoverPlan(t, ins.config.id)
}
