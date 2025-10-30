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
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	var firstToolCall, firstToolDecision = true, true
	var timeLineDeleteCheck, timeLineSaveCheck bool

	var testCallKey int64
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(rawConfig aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			config := rawConfig.(*Coordinator)
			timeline := config.Memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: now`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": ""}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: delete_memory`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					fmt.Sprintf(`{"@action": "call-tool", "tool": "delete_memory", "params": {"id": %d}}`, testCallKey)))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"continue-current-task"`, `"proceed-next-task"`, `"status_summary"`) {
				if firstToolDecision {
					firstToolDecision = false
					keys := timeline.GetIdToTimelineItem().Keys()
					if keys == nil || len(keys) == 0 {
						panic("timeline.summary.GetByIndex fail")
					}
					timeline.GetIdToTimelineItem().ForEach(func(id int64, item *aicommon.TimelineItem) bool {
						result, ok := item.GetValue().(*aitool.ToolResult)
						if ok {
							if result.Name != "now" {
								panic("timeline.idToToolResult.Get now fail")
							} else {
								timeLineSaveCheck = true
							}
						}
						config.Memory.SoftDeleteTimeline(id)
						return true
					})
					rsp.EmitReasonStream(strings.NewReader(`{"@action": "continue-current-task"}`))
				} else {
					// 检查timeline中的所有项是否被删除
					deletedCount := 0
					totalCount := timeline.GetIdToTimelineItem().Len()
					timeline.GetIdToTimelineItem().ForEach(func(id int64, item *aicommon.TimelineItem) bool {
						if item.IsDeleted() {
							deletedCount++
						}
						return true
					})

					timelineDump := timeline.Dump()
					// 如果所有项目都被标记为删除，或者dump显示"no timeline generated"，则测试通过
					if strings.Contains(timelineDump, "no timeline generated in DumpBefore") || deletedCount == totalCount {
						timeLineDeleteCheck = true
					} else {
						panic(fmt.Sprintf("timeline delete check failed - deleted: %d/%d, dump: %s", deletedCount, totalCount, timelineDump))
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
			if result.Type == schema.EVENT_TYPE_CONSUMPTION {
				continue
			}

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
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
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent)

	timeoutDurationSecond := time.Duration(60) * time.Second
	if utils.InGithubActions() {
		timeoutDurationSecond = time.Duration(10) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDurationSecond)
	coordinator, err := NewCoordinator(
		"test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithSystemFileOperator(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(raw aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			config := raw.(*Coordinator)
			timeline := config.Memory.timeline
			rsp := config.NewAIResponse()
			defer func() {
				rsp.Close()
			}()

			if utils.MatchAllOfSubString(request.GetPrompt(), `工具名称: add_persistent_memory`, `"call-tool"`) {
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action": "call-tool", "tool": "now", "params": {"content": "` + persistentMemory + `"}}}`))
				return rsp, nil
			} else if utils.MatchAllOfSubString(request.GetPrompt(), `"continue-current-task"`, `"proceed-next-task"`, `"status_summary"`) {
				config.Memory.PushPersistentData(persistentMemory)
				if timeline.GetIdToTimelineItem().Len() != 1 {
					panic("skip add persistent memory to timeline fail")
				}
				if v, ok := timeline.GetIdToTimelineItem().GetByIndex(0); !ok || v.GetValue().(*aicommon.UserInteraction).Stage != aicommon.UserInteractionStage_Review {
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
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

		}
	}

	assert.True(t, memoryPersistentCheck, "memory struct persistent check")
	//assert.True(t, interactiveCheck, "interactive check failed")
	//assert.True(t, checkToken2, "token2 check failed")
}
