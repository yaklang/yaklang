package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_SyncTaskInDatabase(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)
	ins, err := aid.NewCoordinator(
		uuid.New().String(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

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
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "capability matcher", "matched_identifiers") ||
				utils.MatchAllOfSubString(prompt, `"const": "capability-catalog-match"`, "matched_identifiers") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "capability-catalog-match", "matched_identifiers": []}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") ||
				strings.Contains(prompt, "GenerateTaskSummaryPrompt") ||
				(strings.Contains(prompt, "@action") && strings.Contains(prompt, "summary")) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "summary",
    "status_summary": "测试状态摘要：任务执行中",
    "task_short_summary": "测试任务摘要：查找最大文件",
    "task_long_summary": "测试详细摘要：正在遍历目录并查找最大文件"
}`))
				return rsp, nil
			}

			if strings.Contains(prompt, "Background") && (strings.Contains(prompt, "Current Time:") || strings.Contains(prompt, "OS/Arch:")) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "object",
    "next_action": {
        "type": "finish",
        "answer_payload": "测试模式：任务已完成"
    },
    "cumulative_summary": "测试累积摘要",
    "human_readable_thought": "测试模式：跳过子任务执行"
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "角色设定", "任务执行引擎") ||
				utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") ||
				strings.Contains(prompt, "任务状态") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "测试模式：跳过子任务执行"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "测试模式：任务已完成"}`))
				return rsp, nil
			}

			if strings.Contains(prompt, "tool") && (strings.Contains(prompt, "result") || strings.Contains(prompt, "decision")) {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "continue-current-task",
    "status_summary": "工具调用完成",
    "task_short_summary": "测试工具调用摘要"
}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "测试模式：默认完成"}`))
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
	pingPongCheck := false
	syncTaskCheck := false

LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				time.Sleep(100 * time.Millisecond)
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}
			if parsedTask && result.Type == schema.EVENT_TYPE_CONSUMPTION {
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
					inputChan.SafeFeed(SyncInputEvent(aicommon.SYNC_TYPE_PING))
					continue
				}
			}

			if consumptionCheck && result.Type == schema.EVENT_TYPE_PONG {
				pingPongCheck = true
				inputChan.SafeFeed(SyncInputEvent(aicommon.SYNC_TYPE_PLAN))
				continue
			}

			if pingPongCheck && result.Type == schema.EVENT_TYPE_PLAN {
				var i = make(aitool.InvokeParams, 0)
				if err := json.Unmarshal([]byte(result.Content), &i); err != nil {
					t.Fatal(err)
				}
				tasksRaw, ok := i.GetObject("root_task")["subtasks"]
				if !ok {
					t.Fatal("subtasks not found")
				}
				tasks := tasksRaw.([]any)
				var taskCount = len(tasks)
				if taskCount > 0 {
					syncTaskCheck = true
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
	if !pingPongCheck {
		t.Fatal("pingPong check failed")
	}
	if !syncTaskCheck {
		t.Fatal("sync check failed")
	}

	rt, err := yakit.GetAgentRuntime(ins.Config.GetDB(), ins.Config.Id)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, rt.Uuid, ins.Config.Id)
	t.Logf("rt: %+v", rt)
	count := 0
	aiInteractivity := 0
	reviewCount := 0
	for i := range yakit.YieldCheckpoint(context.Background(), ins.Config.GetDB(), ins.Config.Id) {
		t.Logf("i: %+v", i)
		count++
		if i.Type == schema.AiCheckpointType_AIInteractive {
			aiInteractivity++
		} else if i.Type == schema.AiCheckpointType_Review {
			reviewCount++
		}
	}
	assert.GreaterOrEqual(t, count, 2)
	assert.GreaterOrEqual(t, aiInteractivity, 1)
	assert.GreaterOrEqual(t, reviewCount, 1)
}
