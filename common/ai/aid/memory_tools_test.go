package aid

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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

	var userDataSet, userDataGet, userDataDelete, userDataList *aitool.Tool
	var userQuery *aitool.Tool

	for _, tool := range tools {
		name := tool.Name
		switch name {
		case "memory_user_data_set":
			userDataSet = tool
		case "memory_user_data_get":
			userDataGet = tool
		case "memory_user_data_delete":
			userDataDelete = tool
		case "memory_user_data_list":
			userDataList = tool
		case "memory_query":
			userQuery = tool
		default:
		}
	}

	// test user data
	tokenKey := uuid.New().String()
	tokenValue := uuid.New().String()
	_, err = userDataSet.InvokeWithParams(map[string]any{
		"key":   tokenKey,
		"value": tokenValue,
	})
	require.NoError(t, err)

	callRes, err := userDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 1)
	require.Contains(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenKey)

	callRes, err = userDataGet.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)
	require.Equal(t, callRes.Data.(*aitool.ToolExecutionResult).Result, tokenValue)

	_, err = userDataDelete.InvokeWithParams(map[string]any{
		"key": tokenKey,
	})
	require.NoError(t, err)

	callRes, err = userDataList.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	require.Len(t, callRes.Data.(*aitool.ToolExecutionResult).Result, 0)

	_, err = userDataGet.InvokeWithParams(map[string]any{
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

	var firstToolCall, firstToolDecision = true, true
	var timeLineDeleteCheck, timeLineSaveCheck bool

	var testCallKey int64
	inputChan := make(chan *InputEvent)
	outputChan := make(chan *Event)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			timeline := config.memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			fmt.Println(request.GetPrompt())
			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: now`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": ""}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: delete_memory`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					fmt.Sprintf(`{"@action": "call-tool", "tool": "delete_memory", "params": {"id": %d}}`, testCallKey)))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"require-more-tool"`, `"finished"`, `"status_summary"`) {
				if firstToolDecision {
					firstToolDecision = false
					keys := timeline.idToTimelineItem.Keys()
					if keys == nil || len(keys) == 0 {
						panic("timeline.summary.GetByIndex fail")
					}
					callResult, _ := timeline.idToTimelineItem.Get(keys[0])
					if callResult.Name != "now" {
						panic("timeline.idToToolResult.Get now fail")
					}
					testCallKey = keys[0]
					timeLineSaveCheck = true
					rsp.EmitReasonStream(strings.NewReader(`{"@action": "require-more-tool"}`))
				} else {
					if timeline.idToTimelineItem.Len() != 1 {
						panic("timeline.summary.Len() != 1")
					}
					timelineDump := timeline.Dump()
					fmt.Println(timelineDump)
					if strings.Contains(timelineDump, "no timeline generated in DumpBefore") {
						timeLineDeleteCheck = true
					} else {
						panic("timeline.summary fail")
					}
					cancel()
				}
				return rsp, nil

			} else if utils.MatchAllOfSubString(request.GetPrompt(), `当前任务: "扫描目录结构"`) {
				if firstToolCall {
					firstToolCall = false
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "require-tool", "tool": "now"}`))
				} else {
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
			if result.Type == EVENT_TYPE_CONSUMPTION {
				continue
			}

			if result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == EVENT_TYPE_TASK_REVIEW_REQUIRE {
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
	outputChan := make(chan *Event)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		WithEventInputChan(inputChan),
		WithSystemFileOperator(),
		WithEventHandler(func(event *Event) {
			outputChan <- event
		}),
		WithAICallback(func(config *Config, request *AIRequest) (*AIResponse, error) {
			timeline := config.memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: add_persistent_memory`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": {"content": "` + persistentMemory + `"}}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"require-more-tool"`, `"finished"`, `"status_summary"`) {
				if !utils.StringArrayContains(config.memory.PersistentData, persistentMemory) {
					panic("persistent set fail")
				}
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
			if result.Type == EVENT_TYPE_CONSUMPTION {
				continue
			}

			if result.Type == EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == EVENT_TYPE_TASK_REVIEW_REQUIRE {
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
