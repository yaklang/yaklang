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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCoordinator_SyncTask(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
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

			// 调试打印：输出 prompt 的关键特征（仅在需要时启用）
			// promptPreview := utils.ShrinkString(prompt, 800)
			// fmt.Printf("[TEST DEBUG] AI Callback Prompt preview:\n%s\n", promptPreview)

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
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 处理 summary 请求 - 必须包含所有必需字段
			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") ||
				strings.Contains(prompt, "GenerateTaskSummaryPrompt") ||
				(strings.Contains(prompt, "@action") && strings.Contains(prompt, "summary")) {
				// 返回完整的 summary action，包含所有必需字段
				summaryJSON := `{
    "@action": "summary",
    "status_summary": "测试状态摘要：任务执行中",
    "task_short_summary": "测试任务摘要：查找最大文件",
    "task_long_summary": "测试详细摘要：正在遍历目录并查找最大文件"
}`
				rsp.EmitOutputStream(strings.NewReader(summaryJSON))
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 处理 ReAct loop 的 action 请求（包含 Background, Current Time, OS/Arch, working dir）
			if strings.Contains(prompt, "Background") && (strings.Contains(prompt, "Current Time:") || strings.Contains(prompt, "OS/Arch:")) {
				// 返回 object action，包含 next_action
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
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 处理子任务执行请求 - 返回 finish 动作，避免重试
			if utils.MatchAllOfSubString(prompt, "角色设定", "任务执行引擎") ||
				utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") ||
				strings.Contains(prompt, "任务状态") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "测试模式：跳过子任务执行"}`))
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 处理 verify-satisfaction 请求
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "测试模式：任务已完成"}`))
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 处理 tool call decision 请求
			if strings.Contains(prompt, "tool") && (strings.Contains(prompt, "result") || strings.Contains(prompt, "decision")) {
				// 返回 continue-current-task 或 proceed-next-task
				decisionJSON := `{
    "@action": "continue-current-task",
    "status_summary": "工具调用完成",
    "task_short_summary": "测试工具调用摘要"
}`
				rsp.EmitOutputStream(strings.NewReader(decisionJSON))
				// 移除 sleep，加快测试速度
				return rsp, nil
			}

			// 默认返回 finish，避免未处理的请求导致重试
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "测试模式：默认完成"}`))
			// 移除 sleep，加快测试速度
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
	syncId := uuid.New().String()

LOOP:
	for {
		select {
		case result := <-outputChan:
			if strings.Contains(result.String(), `将最大文件的路径和大小以可读格式输出`) && result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				parsedTask = true
				// 移除不必要的 sleep，加快测试速度
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
				inputChan.SafeFeed(SyncInputEventEx(aicommon.SYNC_TYPE_PLAN, syncId))
				continue
			}

			if pingPongCheck && result.Type == schema.EVENT_TYPE_PLAN && result.SyncID == syncId {
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
		case <-time.After(15 * time.Second):
			t.Fatalf("timeout: parsedTask=%v, consumptionCheck=%v, pingPongCheck=%v, syncTaskCheck=%v",
				parsedTask, consumptionCheck, pingPongCheck, syncTaskCheck)
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
}

func TestCoordinator_SyncTask_Upgrade(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)

	taskExecRequestCount := 0
	echoToolRequestCount := 0
	decisionRequestCount := 0

	firstSummary := true
	task1Summary := uuid.NewString()
	task2Summary := uuid.NewString()

	canSync := false

	echoToken := []string{
		uuid.NewString(),
		uuid.NewString(),
		uuid.NewString(),
	}

	ins, err := aid.NewCoordinator(
		"test-upgrade",
		aicommon.WithTools(aid.EchoTool(), aid.ErrorTool()),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			// 模拟任务规划
			i := config
			prompt := request.GetPrompt()
			rsp := i.NewAIResponse()
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
    "query": "测试同步任务升级",
    "main_task": "执行两个步骤并同步检查",
    "main_task_goal": "完成两个步骤后进行同步检查",
    "tasks": [
        {
            "subtask_name": "步骤一",
            "subtask_goal": "执行第一个任务"
        },
        {
            "subtask_name": "步骤二",
            "subtask_goal": "执行第二个任务"
        },
        {
            "subtask_name": "步骤三",
            "subtask_goal": "执行第三个任务"
        }
    ]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				if utils.MatchAllOfSubString(request.GetPrompt(), "任务名称: 步骤三") {
					canSync = true
					// 移除长时间 sleep，避免测试卡住
				}

				toolName := "echo"

				if taskExecRequestCount >= 3 { //  前三次echo调用工具
					toolName = "error"
				}
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
				taskExecRequestCount++
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {

				if utils.MatchAllOfSubString(request.GetPrompt(), `echo`) {
					rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "call-tool", "tool": "echo", "params": {"input": "%s"}}`, echoToken[echoToolRequestCount])))
					if echoToolRequestCount < 2 {
						echoToolRequestCount++
					}
				} else if utils.MatchAllOfSubString(request.GetPrompt(), `error`) {
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "error", "params": {}}`))
				}
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				if decisionRequestCount%2 == 0 { // 隔一次 continue一次
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "abc-mocked-reason"}`))
				} else {
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
				}
				decisionRequestCount++
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), "status_summary", "task_long_summary", "task_short_summary") {
				if firstSummary {
					rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "summary","task_short_summary":"%s"}`, task1Summary)))
					firstSummary = false
				} else {
					rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "summary","task_short_summary":"%s"}`, task2Summary)))
				}
				return rsp, nil
			}

			return nil, utils.Errorf("unexpect prompt: %s", prompt)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		ins.Run()
	}()

	var sendSync = false

	ctx := utils.TimeoutContextSeconds(15)

LOOP:
	for {
		select {
		case result := <-outputChan:
			// 任务规划后，自动继续
			if canSync {
				canSync = false
				sendSync = true
				inputChan.SafeFeed(SyncInputEvent(aicommon.SYNC_TYPE_PLAN))
				continue
			}

			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}
			//fmt.Println(utils.ShrinkString(result.String(),50))

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if sendSync && result.Type == schema.EVENT_TYPE_PLAN {
				var i = make(aitool.InvokeParams, 0)
				if err := json.Unmarshal([]byte(result.Content), &i); err != nil {
					t.Fatal(err)
				}
				subTask := i.GetObject("root_task").GetObjectArray("subtasks")
				require.Len(t, subTask, 3)

				subTask1 := subTask[0]
				subTask2 := subTask[1]

				require.Equal(t, task1Summary, subTask1.GetString("summary"))
				require.Equal(t, task2Summary, subTask2.GetString("summary"))
				// 检查工具调用次数
				//total_tool_call_count success_tool_call_count fail_tool_call_count
				require.Equal(t, int64(2), subTask1.GetInt("total_tool_call_count"))
				require.Equal(t, int64(2), subTask1.GetInt("success_tool_call_count"))
				require.Equal(t, int64(0), subTask1.GetInt("fail_tool_call_count"))

				require.Equal(t, int64(2), subTask2.GetInt("total_tool_call_count"))
				require.Equal(t, int64(1), subTask2.GetInt("success_tool_call_count"))
				require.Equal(t, int64(1), subTask2.GetInt("fail_tool_call_count"))

				break LOOP
			}

		case <-ctx.Done():
			t.Fatal("timeout")
		}
	}
}
