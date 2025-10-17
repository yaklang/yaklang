package aid

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestCoordinator_SyncTask(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)
	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
				inputChan <- &InputEvent{
					Id: result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
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
					inputChan <- &InputEvent{
						IsSyncInfo: true,
						SyncType:   SYNC_TYPE_PING,
					}
					continue
				}
			}

			if consumptionCheck && result.Type == schema.EVENT_TYPE_PONG {
				pingPongCheck = true
				inputChan <- &InputEvent{
					IsSyncInfo: true,
					SyncType:   SYNC_TYPE_PLAN,
				}
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
	inputChan := make(chan *InputEvent)
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

	ins, err := NewCoordinator(
		"test-upgrade",
		WithTools(EchoTool(), ErrorTool()),
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()
			// 模拟任务规划

			//fmt.Println("===========" + "request:" + "===========\n" + request.GetPrompt())

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称:`, `"call-tool"`, "const") {
				if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: echo`) {
					rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "call-tool", "tool": "echo", "params": {"input": "%s"}}`, echoToken[echoToolRequestCount])))
					if echoToolRequestCount < 2 {
						echoToolRequestCount++
					}
				} else if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: error`) {
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "call-tool", "tool": "error", "params": {}}`))
				}
				return rsp, nil
			}
			// 处理任务执行阶段
			if utils.MatchAllOfSubString(request.GetPrompt(), `["require-tool", "direct-answer"]`, ``) {
				if utils.MatchAllOfSubString(request.GetPrompt(), "当前任务: \"步骤三") {
					canSync = true
					time.Sleep(2 * time.Second)
				}
				if taskExecRequestCount < 3 { //  前三次echo调用工具
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "echo"}`))
				} else { // 第四次调用error工具
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "error"}`))
				}
				taskExecRequestCount++
				return rsp, nil
			}

			if utils.MatchAllOfSubString(request.GetPrompt(), "continue-current-task", "proceed-next-task", "task-failed", "task-skipped") {
				if decisionRequestCount%2 == 0 { // 隔一次 continue一次
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "continue-current-task"}`))
				} else {
					if firstSummary {
						rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "proceed-next-task","task_short_summary":"%s"}`, task1Summary)))
						firstSummary = false
					} else {
						rsp.EmitOutputStream(strings.NewReader(fmt.Sprintf(`{"@action": "proceed-next-task","task_short_summary":"%s"}`, task2Summary)))
					}

				}
				decisionRequestCount++
				return rsp, nil
			}

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
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		ins.Run()
	}()

	var sendSync = false

	ctx := utils.TimeoutContextSeconds(5)

LOOP:
	for {
		select {
		case result := <-outputChan:
			// 任务规划后，自动继续
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}
			//fmt.Println(utils.ShrinkString(result.String(),50))

			if canSync {
				canSync = false
				sendSync = true
				inputChan <- &InputEvent{
					IsSyncInfo: true,
					SyncType:   SYNC_TYPE_PLAN,
				}
				continue
			}

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
				inputChan <- &InputEvent{
					IsInteractive: true,
					Id:            result.GetInteractiveId(),
					Params: aitool.InvokeParams{
						"suggestion": "continue",
					},
				}
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
