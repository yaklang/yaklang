package test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func recoverPlan(t *testing.T, uuid string) {
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10) // 增加缓冲区大小
	outputChan := make(chan *schema.AiOutputEvent, 100)                              // 增加缓冲区大小

	// 确保通道在函数结束时被正确关闭
	defer func() {
		inputChan.Close()
		// 清空outputChan避免goroutine阻塞
		go func() {
			for range outputChan {
				// 消费剩余的事件
			}
		}()
		time.Sleep(100 * time.Millisecond) // 给goroutine一些时间清理
		close(outputChan)
	}()
	recoverCtx, recoverCancel := context.WithCancel(context.Background())
	defer recoverCancel()
	ord, err := aid.NewFastRecoverCoordinatorContext(
		recoverCtx,
		uuid,
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			prompt := request.GetPrompt()

			// 根据请求类型返回不同的响应格式
			if strings.Contains(prompt, "角色设定") && strings.Contains(prompt, "任务执行助手") {
				// 这是任务执行请求，返回direct-answer格式
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "direct-answer",
    "direct_answer": "测试任务已完成，已成功扫描指定目录并找到最大文件",
    "direct_answer_long": "这是一个用于测试AI基础设施恢复功能的模拟响应。任务执行过程：1) 已递归扫描 /Users/v1ll4n/Projects/yaklang 目录；2) 已获取所有文件的路径和大小信息；3) 已成功识别出最大的文件。测试任务已成功完成。"
}`))
			} else if strings.Contains(prompt, "plan") || strings.Contains(prompt, "规划") || strings.Contains(prompt, "任务分解") {
				// 这是计划请求，返回plan格式
				rsp.EmitOutputStream(strings.NewReader(`{
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
}`))
			} else {
				// 默认返回直接回答格式
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "direct-answer",
    "direct_answer": "测试任务已完成",
    "direct_answer_long": "这是一个用于测试AI基础设施恢复功能的模拟响应，任务已成功完成。"
}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in ord.Run(): %v", r)
			}
		}()
		if err := ord.Run(); err != nil {
			log.Errorf("ord.Run() error: %v", err)
		}
	}()

	parsedTask := false
	timeout := time.NewTimer(10 * time.Second) // 增加超时时间
	defer timeout.Stop()

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				break LOOP
			}
		case <-timeout.C:
			t.Fatal("timeout waiting for plan review require event")
		case <-recoverCtx.Done():
			t.Fatal("context cancelled")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
}

func TestCoordinator_RecoverCase(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10) // 增加缓冲区大小
	outputChan := make(chan *schema.AiOutputEvent, 100)                              // 增加缓冲区大小

	// 确保通道在测试结束时被正确关闭
	defer func() {
		inputChan.Close()
		// 清空outputChan避免goroutine阻塞
		go func() {
			for range outputChan {
				// 消费剩余的事件
			}
		}()
		time.Sleep(100 * time.Millisecond) // 给goroutine一些时间清理
		close(outputChan)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ins, err := aid.NewCoordinator(
		"test",
		aicommon.WithContext(ctx),
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
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in ins.Run(): %v", r)
			}
		}()
		if err := ins.Run(); err != nil {
			log.Errorf("ins.Run() error: %v", err)
		}
	}()

	parsedTask := false
	consumptionCheck := false
	mainTimeout := time.NewTimer(30 * time.Second) // 增加主超时时间
	defer mainTimeout.Stop()

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true

				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					log.Errorf("failed to unmarshal consumption data: %v", err)
					continue
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
		case <-mainTimeout.C:
			t.Fatal("timeout waiting for test completion")
		}
	}

	if !parsedTask {
		t.Fatal("cannot parse task and not sent suggestion")
	}
	if !consumptionCheck {
		t.Fatal("consumption check failed")
	}

	recoverPlan(t, ins.Config.Id)
}
