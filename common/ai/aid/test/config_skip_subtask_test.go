package test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyncInputEventWithJSON 创建带 JSON 参数的同步事件
func SyncInputEventWithJSON(syncType string, syncId string, params map[string]any) *ypb.AIInputEvent {
	jsonInput, _ := json.Marshal(params)
	return &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncType,
		SyncID:        syncId,
		SyncJsonInput: string(jsonInput),
	}
}

func TestCoordinator_SkipSubtaskInPlan(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	task1Started := false

	ins, err := aid.NewCoordinator(
		"测试跳过子任务功能",
		aicommon.WithAgreeYOLO(), // 自动同意所有审核，加快测试
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			// 处理 plan 请求
			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试跳过子任务",
    "main_task": "测试跳过子任务功能",
    "main_task_goal": "验证 skip_subtask_in_plan 功能正常工作",
    "tasks": [
        {"subtask_name": "第一个任务", "subtask_goal": "执行第一个任务"},
        {"subtask_name": "第二个任务-需要跳过", "subtask_goal": "这个任务需要被跳过"},
        {"subtask_name": "第三个任务", "subtask_goal": "执行第三个任务"}
    ]
}`))
				return rsp, nil
			}

			// 处理 summary 请求
			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			// 处理任务执行请求
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				if strings.Contains(prompt, "任务名称: 第一个任务") {
					task1Started = true
				}
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			// 处理 verify-satisfaction 请求
			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		ins.Run()
	}()

	skipSent := false
	skipSuccess := false
	syncId := uuid.New().String()
	userReason := "用户认为这个任务不需要执行"

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			// 当第一个任务开始后，发送跳过第二个任务的请求
			if task1Started && !skipSent {
				skipSent = true
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-2",
						"reason":        userReason,
					},
				))
			}

			// 检查跳过响应
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && success {
						skipSuccess = true
						require.Equal(t, "1-2", data["subtask_index"])
						require.Equal(t, userReason, data["reason"])
						require.Contains(t, data["message"], "用户主动跳过了当前子任务")
						require.Contains(t, data["message"], userReason)
					}
				}
			}

			// 一旦跳过成功，就可以结束测试
			if skipSuccess {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: task1Started=%v, skipSent=%v, skipSuccess=%v", task1Started, skipSent, skipSuccess)
		}
	}

	require.True(t, task1Started, "task1 should be started")
	require.True(t, skipSent, "skip request should be sent")
	require.True(t, skipSuccess, "skip should succeed")
}

func TestCoordinator_SkipSubtaskInPlan_NotFound(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	ins, err := aid.NewCoordinator(
		"测试跳过不存在的子任务",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试跳过不存在的子任务",
    "main_task": "测试错误处理",
    "main_task_goal": "验证跳过不存在的子任务时的错误处理",
    "tasks": [{"subtask_name": "唯一任务", "subtask_goal": "执行唯一任务"}]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		ins.Run()
	}()

	planReviewed := false
	errorReceived := false
	syncId := uuid.New().String()

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				planReviewed = true
				// 发送跳过不存在的子任务请求
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-99",
					},
				))
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			// 检查错误响应（通过 success=false 返回）
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && !success {
						if errMsg, ok := data["error"].(string); ok && strings.Contains(errMsg, "subtask not found") {
							errorReceived = true
							break LOOP
						}
					}
				}
			}

			// 检查完成
			if result.Type == schema.EVENT_TYPE_STRUCTURED && strings.Contains(string(result.Content), "coordinator run finished") {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: planReviewed=%v, errorReceived=%v", planReviewed, errorReceived)
		}
	}

	require.True(t, planReviewed, "plan should be reviewed")
	require.True(t, errorReceived, "error should be received for non-existent subtask")
}

func TestCoordinator_FindSubtaskByIndex(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	var coordinator *aid.Coordinator

	ins, err := aid.NewCoordinator(
		"测试查找子任务",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试查找子任务",
    "main_task": "主任务",
    "main_task_goal": "测试 FindSubtaskByIndex",
    "tasks": [
        {"subtask_name": "子任务A", "subtask_goal": "目标A"},
        {"subtask_name": "子任务B", "subtask_goal": "目标B"},
        {"subtask_name": "子任务C", "subtask_goal": "目标C"}
    ]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	coordinator = ins

	go func() {
		ins.Run()
	}()

	planReviewed := false
	syncId := uuid.New().String()
	taskFound := false

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				planReviewed = true
				inputChan.SafeFeed(SyncInputEventEx(aicommon.SYNC_TYPE_PLAN, syncId))
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			// 当收到 plan 同步响应时，验证 FindSubtaskByIndex
			if result.Type == schema.EVENT_TYPE_PLAN && result.SyncID == syncId {
				var data aitool.InvokeParams
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					rootTask := data.GetObject("root_task")
					subtasks := rootTask.GetObjectArray("subtasks")
					require.Len(t, subtasks, 3)

					task1 := coordinator.FindSubtaskByIndex("1-1")
					require.NotNil(t, task1, "should find task with index 1-1")
					require.Equal(t, "子任务A", task1.Name)

					task2 := coordinator.FindSubtaskByIndex("1-2")
					require.NotNil(t, task2, "should find task with index 1-2")
					require.Equal(t, "子任务B", task2.Name)

					task3 := coordinator.FindSubtaskByIndex("1-3")
					require.NotNil(t, task3, "should find task with index 1-3")
					require.Equal(t, "子任务C", task3.Name)

					taskNil := coordinator.FindSubtaskByIndex("1-99")
					require.Nil(t, taskNil, "should not find task with index 1-99")

					taskFound = true
					break LOOP
				}
			}

		case <-ctx.Done():
			t.Fatalf("timeout: planReviewed=%v, taskFound=%v", planReviewed, taskFound)
		}
	}

	require.True(t, planReviewed, "plan should be reviewed")
	require.True(t, taskFound, "tasks should be found by index")
}

func TestCoordinator_SkipSubtaskInPlan_WithReason(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	task1Started := false

	ins, err := aid.NewCoordinator(
		"测试跳过子任务并提供理由",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试跳过子任务并提供理由",
    "main_task": "测试理由功能",
    "main_task_goal": "验证跳过任务时理由被正确记录",
    "tasks": [
        {"subtask_name": "任务一", "subtask_goal": "目标一"},
        {"subtask_name": "任务二", "subtask_goal": "目标二"}
    ]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				if strings.Contains(prompt, "任务名称: 任务一") {
					task1Started = true
				}
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		ins.Run()
	}()

	skipSent := false
	skipSuccess := false
	syncId := uuid.New().String()
	customReason := "这个任务与当前目标无关，我已经获得了足够的信息"

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			if task1Started && !skipSent {
				skipSent = true
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-2",
						"reason":        customReason,
					},
				))
			}

			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && success {
						skipSuccess = true
						require.Equal(t, "1-2", data["subtask_index"])
						require.Equal(t, customReason, data["reason"])
						// 验证 message 中包含用户理由
						message := data["message"].(string)
						require.Contains(t, message, customReason, "message should contain user's reason")
						require.Contains(t, message, "用户给出的理由", "message should indicate user provided reason")
					}
				}
			}

			if skipSuccess {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: task1Started=%v, skipSent=%v, skipSuccess=%v", task1Started, skipSent, skipSuccess)
		}
	}

	require.True(t, skipSent, "skip request should be sent")
	require.True(t, skipSuccess, "skip with reason should succeed")
}

func TestCoordinator_RedoSubtaskInPlan(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	taskExecCount := 0
	userMessageFoundInPrompt := false
	userMessage := "请注意：我需要你在执行这个任务时，特别关注安全性问题，确保所有操作都是安全的"

	var coordinator *aid.Coordinator

	ins, err := aid.NewCoordinator(
		"测试重做子任务功能",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试重做子任务",
    "main_task": "测试重做子任务功能",
    "main_task_goal": "验证 redo_subtask_in_plan 功能正常工作",
    "tasks": [
        {"subtask_name": "需要重做的任务", "subtask_goal": "这个任务需要被重做"}
    ]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			// 任务执行请求
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				taskExecCount++

				// 检查 prompt 中是否包含用户消息（用于验证 redo 后 timeline 被传递）
				if strings.Contains(prompt, userMessage) {
					userMessageFoundInPrompt = true
				}

				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	coordinator = ins

	go func() {
		ins.Run()
	}()

	redoSent := false
	redoSuccess := false
	syncId := uuid.New().String()

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			// 当第一次任务执行后，发送 redo 请求
			if taskExecCount == 1 && !redoSent {
				redoSent = true
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-1",
						"user_message":  userMessage,
					},
				))
			}

			// 检查 redo 响应
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && success {
						redoSuccess = true
						require.Equal(t, "1-1", data["subtask_index"])
						require.Equal(t, userMessage, data["user_message"])
						require.Contains(t, data["message"], "用户请求重新执行当前子任务")
						require.Contains(t, data["message"], userMessage)
						require.Contains(t, data["message"], "<用户补充信息>")
						require.Contains(t, data["message"], "</用户补充信息>")

						// 验证 timeline 包含用户消息
						timeline := coordinator.Timeline.Dump()
						require.Contains(t, timeline, userMessage, "timeline should contain user message")
						require.Contains(t, timeline, "user-redo-subtask", "timeline should contain redo marker")

						// 验证任务状态保持 Processing（未改变）
						task := coordinator.FindSubtaskByIndex("1-1")
						require.NotNil(t, task, "task should be found")
						require.Equal(t, aicommon.AITaskState_Processing, task.GetStatus(), "task status should remain Processing after redo")
					}
				}
			}

			// redo 成功后即可结束测试（验证了核心功能）
			if redoSuccess {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: taskExecCount=%v, redoSent=%v, redoSuccess=%v, userMessageFoundInPrompt=%v",
				taskExecCount, redoSent, redoSuccess, userMessageFoundInPrompt)
		}
	}

	require.True(t, redoSent, "redo request should be sent")
	require.True(t, redoSuccess, "redo should succeed")
}

// TestCoordinator_SkipSubtaskAndContinueNext 验证 skip 子任务后，下一个子任务立即开始执行
// 这是一个关键测试，确保：
// 1. 测试中有 1-1 和 1-2 两个任务
// 2. 当 1-1 执行过程中接收到 skip 后，1-2 立即开始执行
//
// 测试策略：由于 skip 是通过 context.Cancel() 实现的，而 AI callback 可能在 context 取消前就返回了
// 所以我们采用"预跳过"策略：在任务规划完成后、任务开始执行前，先跳过任务 1-1
// 然后验证 runtime 能正确识别 Skipped 状态并跳过任务 1-1，直接执行任务 1-2
func TestCoordinator_SkipSubtaskAndContinueNext(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	task11Started := false
	task12Started := false
	task12Completed := false

	ins, err := aid.NewCoordinator(
		"测试跳过子任务后继续执行下一个任务",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				// 确保创建两个子任务 1-1 和 1-2
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试跳过后继续",
    "main_task": "测试跳过子任务后立即执行下一个任务",
    "main_task_goal": "验证 skip 1-1 后 1-2 立即开始执行",
    "tasks": [
        {"subtask_name": "任务1-1", "subtask_goal": "这个任务会被跳过"},
        {"subtask_name": "任务1-2", "subtask_goal": "这个任务应该在1-1被跳过后立即执行"}
    ]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			// 处理任务执行请求
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				// 检查是否是 1-1 任务（理论上不应该被调用，因为已被跳过）
				if strings.Contains(prompt, "任务名称: 任务1-1") {
					task11Started = true
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "任务1-1完成"}}`))
					return rsp, nil
				}

				// 检查是否是 1-2 任务
				if strings.Contains(prompt, "任务名称: 任务1-2") {
					task12Started = true
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "任务1-2完成"}}`))
					return rsp, nil
				}

				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				if strings.Contains(prompt, "任务1-2") {
					task12Completed = true
				}
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		ins.Run()
	}()

	skipSent := false
	skipSuccess := false
	syncId := uuid.New().String()

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			// 在计划审核时发送 skip 请求（任务开始执行前）
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE && !skipSent {
				skipSent = true
				// 先发送 skip 请求跳过任务 1-1
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-1",
						"reason":        "用户决定跳过任务1-1，希望立即执行任务1-2",
					},
				))
				// 然后继续执行计划
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			// 检查 skip 响应
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && success {
						skipSuccess = true
						require.Equal(t, "1-1", data["subtask_index"])
					}
				}
			}

			// 检查完成（YOLO 模式下会快速完成，不会调用 verify-satisfaction）
			if result.Type == schema.EVENT_TYPE_STRUCTURED && strings.Contains(string(result.Content), "coordinator run finished") {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: skipSent=%v, skipSuccess=%v, task11Started=%v, task12Started=%v, task12Completed=%v",
				skipSent, skipSuccess, task11Started, task12Started, task12Completed)
		}
	}

	require.True(t, skipSent, "skip request should be sent")
	require.True(t, skipSuccess, "skip should succeed")
	require.False(t, task11Started, "task 1-1 should NOT be started (it was skipped)")
	require.True(t, task12Started, "task 1-2 should start after 1-1 is skipped")
}

func TestCoordinator_RedoSubtaskInPlan_MissingUserMessage(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	ins, err := aid.NewCoordinator(
		"测试重做子任务缺少用户消息",
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			outputChan <- event
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()
			defer rsp.Close()

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "plan",
    "query": "测试",
    "main_task": "测试",
    "main_task_goal": "测试",
    "tasks": [{"subtask_name": "任务", "subtask_goal": "目标"}]
}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	go func() {
		ins.Run()
	}()

	planReviewed := false
	errorReceived := false
	syncId := uuid.New().String()

	ctx := utils.TimeoutContextSeconds(10)

LOOP:
	for {
		select {
		case result := <-outputChan:
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				planReviewed = true
				// 发送缺少 user_message 的 redo 请求
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-1",
						// 故意不提供 user_message
					},
				))
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE || result.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			// 检查错误响应（通过 success=false 返回）
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && !success {
						if errMsg, ok := data["error"].(string); ok && strings.Contains(errMsg, "user_message is required") {
							errorReceived = true
							break LOOP
						}
					}
				}
			}

			// 检查完成
			if result.Type == schema.EVENT_TYPE_STRUCTURED && strings.Contains(string(result.Content), "coordinator run finished") {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: planReviewed=%v, errorReceived=%v", planReviewed, errorReceived)
		}
	}

	require.True(t, planReviewed, "plan should be reviewed")
	require.True(t, errorReceived, "error should be received for missing user_message")
}

