package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"strings"
	"testing"
	"time"

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
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
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
			if utils.MatchAllOfSubString(prompt, "plan: when user needs to create or refine a plan for a specific task, if need to search") {
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
					time.Sleep(10 * time.Minute)
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

	ctx := utils.TimeoutContextSeconds(10000)

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
