package aid

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func Test_MemoryTools(t *testing.T) {
	m := GetDefaultMemory()
	tools, err := m.CreateMemoryTools()
	if err != nil {
		return
	}

	var persistentDataSet, persistentDataGet, persistentDataDelete, persistentDataList *aitool.Tool
	var userQuery *aitool.Tool

	for _, tool := range tools {
		name := tool.Name
		switch name {
		case "memory_persistent_data_set":
			persistentDataSet = tool
		case "memory_persistent_data_get":
			persistentDataGet = tool
		case "memory_persistent_data_delete":
			persistentDataDelete = tool
		case "memory_persistent_data_list":
			persistentDataList = tool
		case "memory_query":
			userQuery = tool
		default:
		}
	}

	// test user data
	tokenKey := uuid.New().String()
	tokenValue := uuid.New().String()
	_, err = persistentDataSet.InvokeWithParams(map[string]any{
		"key":   tokenKey,
		"value": tokenValue,
	})
	require.NoError(t, err)

	callRes, err := persistentDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 1)
	require.Contains(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenKey)

	callRes, err = persistentDataGet.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)
	require.Equal(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenValue)

	_, err = persistentDataDelete.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)

	callRes, err = persistentDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 0)

	_, err = persistentDataGet.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.Error(t, err)

	tokenQuery := uuid.New().String()
	m.StoreQuery(tokenQuery)
	callRes, err = userQuery.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Equal(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenQuery)

}

func TestCoodinator_Delete_Memory(t *testing.T) {
	var callCount int
	var timeLineDeleteCheck, timeLineSaveCheck bool
	var testCallKey int64
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(rawConfig aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			config := rawConfig.(*Config)
			timeline := config.memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Printf("=== AI CALLBACK CALLED ===\n")
			fmt.Printf("firstToolCall: %v, firstToolDecision: %v, testCallKey: %d\n", firstToolCall, firstToolDecision, testCallKey)
			fmt.Printf("Request prompt contains:\n")
			if strings.Contains(request.GetPrompt(), `工具名称: now`) {
				fmt.Printf("  - 工具名称: now\n")
			}
			if strings.Contains(request.GetPrompt(), `工具名称: delete_memory`) {
				fmt.Printf("  - 工具名称: delete_memory\n")
			}
			if strings.Contains(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				fmt.Printf("  - 当前任务: 扫描目录结构\n")
			}
			if strings.Contains(request.GetPrompt(), `"continue-current-task"`) {
				fmt.Printf("  - continue-current-task\n")
			}
			
			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: now`, `"call-tool"`) {
				fmt.Printf(">>> Calling NOW tool\n")
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": ""}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: delete_memory`, `"call-tool"`) {
				fmt.Printf(">>> Calling DELETE_MEMORY tool with ID: %d\n", testCallKey)
				rsp.EmitOutputStream(strings.NewReader(
					fmt.Sprintf(`{"@action": "call-tool", "tool": "delete_memory", "params": {"id": %d}}`, testCallKey)))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"continue-current-task"`, `"proceed-next-task"`, `"status_summary"`) {
				if firstToolDecision {
					fmt.Printf(">>> FIRST TOOL DECISION\n")
					firstToolDecision = false
					keys := timeline.idToTimelineItem.Keys()
					if keys == nil || len(keys) == 0 {
						panic("timeline.summary.GetByIndex fail")
					}
					callResult, _ := timeline.idToTimelineItem.Get(keys[0])
					result := callResult.value.(*aitool.ToolResult)
					if result.Name != "now" {
						panic("timeline.idToToolResult.Get now fail")
					}
					testCallKey = keys[0]
					fmt.Printf(">>> Set testCallKey to: %d\n", testCallKey)
					timeLineSaveCheck = true
					rsp.EmitReasonStream(strings.NewReader(`{"@action": "continue-current-task", "status_summary": "Now tool executed, proceeding to next step"}`))
				} else {
					fmt.Printf(">>> SECOND TOOL DECISION - checking deletion\n")
					// SoftDelete只是标记删除，不会改变idToTimelineItem的长度
					// 正确的检查是验证Dump输出是否显示所有timeline项都被删除了
					timelineDump := timeline.Dump()
					fmt.Printf("===== TIMELINE DUMP =====\n%s\n==========================\n", timelineDump)
					
					// 检查timeline中的所有项是否被删除
					deletedCount := 0
					totalCount := timeline.idToTimelineItem.Len()
					timeline.idToTimelineItem.ForEach(func(id int64, item *timelineItem) bool {
						if item.deleted {
							deletedCount++
						}
						fmt.Printf("Timeline item ID: %d, deleted: %v\n", id, item.deleted)
						return true
					})
					fmt.Printf("Total items: %d, Deleted items: %d\n", totalCount, deletedCount)
					
					if strings.Contains(timelineDump, "no timeline generated in DumpBefore") || deletedCount == totalCount {
						timeLineDeleteCheck = true
					} else {
						panic(fmt.Sprintf("timeline.summary fail - dump: %s", timelineDump))
					}
					cancel()
				}
				return rsp, nil

			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				if firstToolCall {
					fmt.Printf(">>> FIRST TOOL CALL - requesting NOW\n")
					firstToolCall = false
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "now"}`))
				} else {
					fmt.Printf(">>> SECOND TOOL CALL - requesting DELETE_MEMORY\n")
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "delete_memory"}`))
				}
				return rsp, nil
			}

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
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	count := 0
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case result, ok := <-outputChan:
			if !ok {
				break LOOP
			}
			count++
			if count > 100 {
				break LOOP
			}
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
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
	assert.True(t, timeLineDeleteCheck, " timeline delete check failed")
	assert.True(t, timeLineSaveCheck, " timeline delete check failed")
	//assert.True(t, interactiveCheck, "interactive check failed")
	//assert.True(t, checkToken2, "token2 check failed")
}

func TestCoodinator_Add_Persistent_Memory(t *testing.T) {

	var memoryPersistentCheck bool

	var persistentMemory = utils.RandStringBytes(20)
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *schema.AiOutputEvent)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		WithAICallback(func(raw aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			config := raw.(*Config)
			timeline := config.memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: add_persistent_memory`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": {"content": "` + persistentMemory + `"}}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"continue-current-task"`, `"proceed-next-task"`, `"status_summary"`) {
				config.memory.PushPersistentData(persistentMemory)
				if timeline.idToTimelineItem.Len() > 0 {
					panic("skip add persistent memory to timeline fail")
				}
				memoryPersistentCheck = true
				cancel()
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "add_persistent_memory"}`))
				return rsp, nil
			}

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
            "subtask_name": "计算文件大小",
            "subtask_goal": "遍历所有文件，计算每个文件的大小"
        }
    ]
}
			`))
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCoordinator failed: %v", err)
	}
	go coordinator.Run()

	count := 0
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		case result, ok := <-outputChan:
			if !ok {
				break LOOP
			}
			count++
			if count > 100 {
				break LOOP
			}
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
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

	assert.True(t, memoryPersistentCheck, "memory struct persistent check")
	//assert.True(t, interactiveCheck, "interactive check failed")
	//assert.True(t, checkToken2, "token2 check failed")
}
