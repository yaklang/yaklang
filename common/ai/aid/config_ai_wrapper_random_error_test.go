package aid

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCoordinator_RandomAICallbackError(t *testing.T) {
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	m := new(sync.Mutex)
	var errLimit int64 = 2
	var count = new(int64)

	ins, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			m.Lock()
			defer m.Unlock()

			countInt64 := atomic.AddInt64(count, 1)
			if countInt64 <= errLimit {
				return nil, utils.Errorf("mock, unknown err[%v]", count)
			} else {
				count = new(int64)
			}

			rsp := aicommon.NewAIResponse(config)
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
	pingPongCheck := false
	syncTaskCheck := false
LOOP:
	for {
		select {
		case result := <-outputChan:
			fmt.Println("result:" + result.String())
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				// 解析JSON数据
				var data = map[string]any{}
				err := json.Unmarshal([]byte(result.Content), &data)
				if err != nil {
					t.Fatal(err)
				}

				// 检查是否包含预期的任务描述
				if plansRaw, ok := data["plans"]; ok {
					plansJson, _ := json.Marshal(plansRaw)
					if strings.Contains(string(plansJson), `将最大文件的路径和大小以可读格式输出`) {
						parsedTask = true
						inputChan <- &InputEvent{
							Id: result.GetInteractiveId(),
							Params: aitool.InvokeParams{
								"suggestion": "continue",
							},
						}
						continue
					}
				}
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
