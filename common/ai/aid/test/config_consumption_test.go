package test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_Consumption_SingleTime(t *testing.T) {
	basicTestCoordinator_Consumption(t)
}

func TestCoordinator_Consumption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testCount := 20
	if utils.InGithubActions() {
		testCount = 3
	}
	swg := utils.NewSizedWaitGroup(400)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				time.Sleep(time.Second)
				log.Infof("swg wait: %v", swg.WaitingEventCount.Load())
			}
		}
	}()
	for i := 0; i < testCount; i++ {
		swg.Add(1)
		go func() {
			defer swg.Done()
			basicTestCoordinator_Consumption(t)
		}()
	}
	swg.Wait()
}

func basicTestCoordinator_Consumption(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10) // 增加缓冲区大小，避免阻塞
	outChan := chanx.NewUnlimitedChan[*schema.AiOutputEvent](context.Background(), 100)
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outChan.SafeFeed(event)
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

	// 给协调器一点启动时间
	time.Sleep(100 * time.Millisecond)

	parsedTask := false
	consumptionCheck := false
	outChannel := outChan.OutputChannel()

	// 添加调试计数器
	eventCount := 0

LOOP:
	for {
		select {
		case result := <-outChannel:
			eventCount++
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}
			if result.Type == schema.EVENT_TYPE_CONSUMPTION && parsedTask {
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
				log.Infof("consumption check: input=%d, output=%d", inputConsumption, outputConsumption)
				if inputConsumption > 0 && outputConsumption > 0 {
					consumptionCheck = true
					log.Info("consumption check passed, breaking loop")
					break LOOP
				}
			}
		case <-time.After(time.Second * 15):
			log.Errorf("test timeout after 15 seconds: parsedTask=%t, consumptionCheck=%t, eventCount=%d",
				parsedTask, consumptionCheck, eventCount)
			t.Fatal("timeout waiting for test completion")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !consumptionCheck {
		t.Fatal("consumption check failed")
	}
}
