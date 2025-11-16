package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/google/uuid"
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
		uuid.New().String(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outChan.SafeFeed(event)
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			// 处理 plan 请求 - 匹配实际的 plan prompt 特征
			// Plan prompt 的关键标识：
			// 1. 包含 "# 任务规划使命" 或 "你是一个输出JSON的任务规划的工具"
			// 2. 包含 "<|PERSISTENT_NcSB|>" 标记
			// 3. 包含 "任务设计输出要求"
			// 4. 可能包含 "```schema"
			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_NcSB") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
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
				return rsp, nil
			}

			// 处理 summary 请求 - 必须包含所有必需字段
			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") ||
				strings.Contains(prompt, "GenerateTaskSummaryPrompt") ||
				(strings.Contains(prompt, "@action") && strings.Contains(prompt, "summary")) {
				summaryJSON := `{
    "@action": "summary",
    "status_summary": "测试状态摘要：任务执行中",
    "task_short_summary": "测试任务摘要：查找最大文件",
    "task_long_summary": "测试详细摘要：正在遍历目录并查找最大文件"
}`
				rsp.EmitOutputStream(strings.NewReader(summaryJSON))
				time.Sleep(50 * time.Millisecond)
				return rsp, nil
			}

			// 处理 ReAct loop 的 action 请求（包含 Background, Current Time, OS/Arch, working dir）
			if strings.Contains(prompt, "Background") && (strings.Contains(prompt, "Current Time:") || strings.Contains(prompt, "OS/Arch:")) {
				reactJSON := `{
    "@action": "object",
    "next_action": {
        "type": "finish",
        "answer_payload": "测试模式：任务已完成"
    },
    "cumulative_summary": "测试累积摘要",
    "human_readable_thought": "测试模式：跳过子任务执行"
}`
				rsp.EmitOutputStream(strings.NewReader(reactJSON))
				time.Sleep(50 * time.Millisecond)
				return rsp, nil
			}

			// 处理子任务执行请求 - 返回 finish 动作，避免重试
			if utils.MatchAllOfSubString(prompt, "角色设定", "任务执行引擎") ||
				utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") ||
				strings.Contains(prompt, "任务状态") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试模式：跳过子任务执行"}`))
				time.Sleep(50 * time.Millisecond)
				return rsp, nil
			}

			// 处理 tool call decision 请求
			if strings.Contains(prompt, "tool") && (strings.Contains(prompt, "result") || strings.Contains(prompt, "decision")) {
				decisionJSON := `{
    "@action": "continue-current-task",
    "status_summary": "工具调用完成",
    "task_short_summary": "测试工具调用摘要"
}`
				rsp.EmitOutputStream(strings.NewReader(decisionJSON))
				time.Sleep(50 * time.Millisecond)
				return rsp, nil
			}

			// 默认返回 finish，避免未处理的请求导致重试
			rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "测试模式：默认完成"}`))
			time.Sleep(50 * time.Millisecond)
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
