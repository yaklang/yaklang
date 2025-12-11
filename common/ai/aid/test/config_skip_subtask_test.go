package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

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

// extractCurrentTaskContent 从 prompt 中提取 <|CURRENT_TASK|> 和 <|CURRENT_TASK_END|> 之间的内容
// 返回提取的内容，如果未找到则返回空字符串
func extractCurrentTaskContent(prompt string) string {
	re := regexp.MustCompile(`(?s)<\|CURRENT_TASK\|>(.*?)<\|CURRENT_TASK_END\|>`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// isCurrentTask 判断当前执行的任务是否为指定的任务名称
// 通过提取 <|CURRENT_TASK|> 标签内的内容，检查其中是否包含 "任务名称: taskName"
func isCurrentTask(prompt string, taskName string) bool {
	currentTaskContent := extractCurrentTaskContent(prompt)
	if currentTaskContent == "" {
		return false
	}
	return strings.Contains(currentTaskContent, "任务名称: "+taskName)
}

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
				// 使用正则表达式提取 <|CURRENT_TASK|> 标签内容来精确判断当前执行的任务
				if isCurrentTask(prompt, "第一个任务") {
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

			if result.Type == schema.EVENT_TYPE_STRUCTURED && utils.StringContainsAllOfSubString(string(result.Content), []string{"push_task", "1-3"}) {
				if skipSuccess { // 检查跳过 task 2 有没有正常 来到 task3
					break LOOP
				}
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
				// 使用正则表达式提取 <|CURRENT_TASK|> 标签内容来精确判断当前执行的任务
				if isCurrentTask(prompt, "任务一") {
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

// TestCoordinator_SkipSubtaskAndContinueNext 验证 skip 子任务后，下一个子任务立即开始执行
// 这是一个关键测试，确保：
// 1. 测试中有 1-1 和 1-2 两个任务
// 2. 当 1-1 执行过程中接收到 skip 后，1-2 立即开始执行
//
// 测试策略：
// 1. plan 正常 continue
// 2. 任务 1-1 开始执行时：
//   - 通知外部任务已开始
//   - 使用 goroutine 异步发送响应，避免阻塞核心处理 skip 请求
//   - 等待 skip 确认后再返回响应
//
// 3. 外部收到任务开始通知后发送 skip 请求
// 4. skip 被核心处理并确认成功
// 5. callback 收到确认后返回响应
// 6. runtime 检测到 Skipped 状态并继续执行任务 1-2
//
// 关键点：AI callback 使用异步方式，不阻塞核心处理 skip 请求
func TestCoordinator_SkipSubtaskAndContinueNext(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 用于协调 callback 和外部测试循环
	task11Started := make(chan struct{}, 1) // callback 通知外部任务 1-1 开始
	skipConfirmed := make(chan struct{}, 1) // 外部通知 callback skip 已确认成功
	task11DidStart := false
	task11AlreadyNotified := false // 防止重试时重复通知
	task12Started := false

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

			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "PERSISTENT_") || strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "```schema"))

			if isPlanRequest {
				defer rsp.Close()
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
				defer rsp.Close()
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "s", "task_short_summary": "s", "task_long_summary": "s"}`))
				return rsp, nil
			}

			// 处理任务执行请求
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_tool") {
				// 使用正则表达式提取 <|CURRENT_TASK|> 标签内容来精确判断当前执行的任务

				// 任务 1-1：使用 pipe 异步写入，先写不完整的 JSON 让解析器等待
				if isCurrentTask(prompt, "任务1-1") {
					task11DidStart = true

					// 如果已经通知过（可能是重试），用 pipe 卡住不返回，避免干扰
					if task11AlreadyNotified {
						pr, pw := io.Pipe()
						go func() {
							// 写入部分数据让解析器等待
							pw.Write([]byte(`{"@a`))
							// sleep 15s 后关闭（测试应该在这之前完成）
							time.Sleep(15 * time.Second)
							pw.Close()
							rsp.Close()
						}()
						rsp.EmitOutputStream(pr)
						return rsp, nil
					}
					task11AlreadyNotified = true

					// 使用 pipe 来控制响应流
					pr, pw := io.Pipe()

					// 在 goroutine 中异步写入响应
					go func() {
						// 先写入不完整的 JSON，让解析器等待更多数据
						pw.Write([]byte(`{"@a`))

						// 通知外部任务已开始
						select {
						case task11Started <- struct{}{}:
						default:
						}

						// 等待 skip 成功确认信号
						<-skipConfirmed

						// skip 已确认，写入完整的响应
						pw.Write([]byte(`ction": "object", "next_action": {"type": "finish", "answer_payload": "任务1-1完成"}}`))

						// 关闭 pipe，此时核心才会开始解析完整响应
						pw.Close()

						// 关闭响应
						rsp.Close()
					}()

					// 使用 pipe reader 作为响应流
					rsp.EmitOutputStream(pr)
					// 不在这里调用 rsp.Close()，由 goroutine 负责关闭
					return rsp, nil
				}

				// 任务 1-2：正常执行
				if isCurrentTask(prompt, "任务1-2") {
					defer rsp.Close()
					task12Started = true
					rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "任务1-2完成"}}`))
					return rsp, nil
				}

				defer rsp.Close()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "finish", "answer_payload": "完成"}}`))
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				defer rsp.Close()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "完成"}`))
				return rsp, nil
			}

			defer rsp.Close()
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

	// 计数器：验证只触发一次
	task11StartedCount := 0
	skipConfirmedCount := 0

	ctx := utils.TimeoutContextSeconds(15)

LOOP:
	for {
		select {
		case <-task11Started:
			// 验证：task11Started 只能触发一次
			task11StartedCount++
			require.Equal(t, 1, task11StartedCount, "task11Started should only be triggered once, but got %d times", task11StartedCount)

			// 任务 1-1 开始执行后，立即发送 skip 请求
			// 由于 callback 是异步的，核心可以立即处理 skip 请求
			if !skipSent {
				skipSent = true
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-1",
						"reason":        "用户决定跳过任务1-1，希望立即执行任务1-2",
					},
				))
			}

		case result := <-outputChan:
			// plan 审核正常 continue
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			// 检查 skip 响应
			if result.Type == schema.EVENT_TYPE_STRUCTURED && result.SyncID == syncId {
				var data map[string]any
				if err := json.Unmarshal([]byte(result.Content), &data); err == nil {
					if success, ok := data["success"].(bool); ok && success {
						// 验证：skipConfirmed 只能发送一次
						skipConfirmedCount++
						require.Equal(t, 1, skipConfirmedCount, "skipConfirmed should only be sent once, but got %d times", skipConfirmedCount)

						skipSuccess = true
						require.Equal(t, "1-1", data["subtask_index"])
						// 通知 callback skip 已成功确认，可以返回响应了
						select {
						case skipConfirmed <- struct{}{}:
						default:
						}
					}
				}
			}

			// 检查完成
			if result.Type == schema.EVENT_TYPE_STRUCTURED && strings.Contains(string(result.Content), "coordinator run finished") {
				break LOOP
			}

		case <-ctx.Done():
			t.Fatalf("timeout: skipSent=%v, skipSuccess=%v, task11DidStart=%v, task12Started=%v",
				skipSent, skipSuccess, task11DidStart, task12Started)
		}
	}

	// 最终验证：确保都恰好触发了一次
	require.Equal(t, 1, task11StartedCount, "task11Started should be triggered exactly once")
	require.Equal(t, 1, skipConfirmedCount, "skipConfirmed should be sent exactly once")
	require.True(t, skipSent, "skip request should be sent")
	require.True(t, skipSuccess, "skip should succeed")
	require.True(t, task11DidStart, "task 1-1 should be started before being skipped")
	require.True(t, task12Started, "task 1-2 should start after 1-1 is skipped")
}

// TestCoordinator_RedoSubtaskInPlan_MissingUserMessage 验证 redo 请求缺少 user_message 时返回错误
// 测试策略：
// 1. plan 正常 continue
// 2. 任务开始执行时，通知外部并等待 redo 错误响应被接收的信号
// 3. 外部在任务执行过程中发送缺少 user_message 的 redo 请求
// 4. 验证收到错误响应后，通知 callback 继续
func TestCoordinator_RedoSubtaskInPlan_MissingUserMessage(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	taskStarted := make(chan struct{}, 1)   // callback 通知外部任务开始
	redoErrorDone := make(chan struct{}, 1) // 外部通知 callback redo 错误已处理

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
				// 通知外部任务已开始
				select {
				case taskStarted <- struct{}{}:
				default:
				}
				// 等待 redo 错误响应被处理的信号
				<-redoErrorDone
				// 正常返回
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

	redoSent := false
	errorReceived := false
	syncId := uuid.New().String()

	ctx := utils.TimeoutContextSeconds(15)

LOOP:
	for {
		select {
		case <-taskStarted:
			// 任务开始执行后，发送缺少 user_message 的 redo 请求
			if !redoSent {
				redoSent = true
				inputChan.SafeFeed(SyncInputEventWithJSON(
					aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN,
					syncId,
					map[string]any{
						"subtask_index": "1-1",
						// 故意不提供 user_message
					},
				))
			}

		case result := <-outputChan:
			// plan 审核正常 continue
			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
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
							// 通知 callback 继续
							select {
							case redoErrorDone <- struct{}{}:
							default:
							}
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
			t.Fatalf("timeout: redoSent=%v, errorReceived=%v", redoSent, errorReceived)
		}
	}

	require.True(t, redoSent, "redo request should be sent")
	require.True(t, errorReceived, "error should be received for missing user_message")
}
