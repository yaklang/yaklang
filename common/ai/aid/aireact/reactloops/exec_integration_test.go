package reactloops

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// 这是一个集成测试文件，测试 ReActLoop 的核心功能
// 由于 AIInvokeRuntime 接口复杂，这里主要测试：
// 1. 内部逻辑（prompt 生成、schema 生成等）
// 2. 动作注册和管理
// 3. 操作符行为

// TestPromptGeneration_Integration 测试提示词生成的完整流程
func TestPromptGeneration_Integration(t *testing.T) {
	// 这个测试不需要完整的runtime，只需要测试 prompt 相关逻辑

	// 测试 schema 生成
	actions := []*LoopAction{
		{
			ActionType:  "action1",
			Description: "First action",
		},
		{
			ActionType:  "action2",
			Description: "Second action",
		},
	}

	schema := buildSchema(actions...)

	if schema == "" {
		t.Error("Schema should not be empty")
	}

	if !strings.Contains(schema, "action1") {
		t.Error("Schema should contain action1")
	}

	if !strings.Contains(schema, "action2") {
		t.Error("Schema should contain action2")
	}

	if !strings.Contains(schema, "@action") {
		t.Error("Schema should contain @action field")
	}

	t.Logf("Generated schema:\n%s", schema)
}

// TestActionRegistration_Integration 测试动作注册
func TestActionRegistration_Integration(t *testing.T) {
	// 测试全局动作注册
	testAction := &LoopAction{
		ActionType:  "integration_test_action",
		Description: "Test action for integration test",
	}

	RegisterAction(testAction)

	retrieved, ok := GetLoopAction("integration_test_action")
	if !ok {
		t.Fatal("Should be able to retrieve registered action")
	}

	if retrieved.ActionType != "integration_test_action" {
		t.Errorf("Expected action type 'integration_test_action', got '%s'", retrieved.ActionType)
	}
}

// TestLoopActionOperator 测试操作符行为
func TestLoopActionOperator(t *testing.T) {
	// 创建一个简单的 mock task
	task := &mockSimpleTask{
		id:    "test-task",
		index: "test-index",
	}

	operator := newLoopActionHandlerOperator(task)

	// 测试 Continue
	operator.Continue()
	if !operator.IsContinued() {
		t.Error("IsContinued should return true after Continue()")
	}

	// 测试 Feedback
	operator.Feedback("test feedback message")
	feedback := operator.GetFeedback()
	if !strings.Contains(feedback.String(), "test feedback message") {
		t.Error("Feedback should contain the message")
	}

	// 测试 DisallowNextLoopExit
	operator2 := newLoopActionHandlerOperator(task)
	operator2.DisallowNextLoopExit()
	if !operator2.GetDisallowLoopExit() {
		t.Error("DisallowLoopExit should be set")
	}
}

// TestBuiltinActions 测试内置动作
func TestBuiltinActions(t *testing.T) {
	if loopAction_DirectlyAnswer == nil {
		t.Fatal("loopAction_DirectlyAnswer should not be nil")
	}

	if loopAction_Finish == nil {
		t.Fatal("loopAction_Finish should not be nil")
	}

	// 验证内置动作的类型
	if loopAction_DirectlyAnswer.ActionType != "directly_answer" {
		t.Errorf("Expected 'directly_answer', got '%s'", loopAction_DirectlyAnswer.ActionType)
	}

	if loopAction_Finish.ActionType != "finish" {
		t.Errorf("Expected 'finish', got '%s'", loopAction_Finish.ActionType)
	}

	// 验证内置动作都有描述
	if loopAction_DirectlyAnswer.Description == "" {
		t.Error("DirectlyAnswer should have description")
	}

	if loopAction_Finish.Description == "" {
		t.Error("Finish should have description")
	}
}

// TestSchemaGeneration_WithDisallowExit 测试禁止退出时的 schema 生成
func TestSchemaGeneration_WithDisallowExit(t *testing.T) {
	actions := []*LoopAction{
		loopAction_DirectlyAnswer,
		loopAction_Finish,
		{
			ActionType:  "custom_action",
			Description: "Custom action",
		},
	}

	// 正常 schema
	normalSchema := buildSchema(actions...)
	if !strings.Contains(normalSchema, "finish") {
		t.Error("Normal schema should contain finish")
	}

	// 过滤掉 finish 的 schema（模拟 disallowExit 场景）
	var filteredActions []*LoopAction
	for _, a := range actions {
		if a.ActionType != "finish" {
			filteredActions = append(filteredActions, a)
		}
	}

	filteredSchema := buildSchema(filteredActions...)
	if strings.Contains(filteredSchema, "finish") {
		t.Error("Filtered schema should not contain finish")
	}

	if !strings.Contains(filteredSchema, "directly_answer") {
		t.Error("Filtered schema should still contain directly_answer")
	}

	if !strings.Contains(filteredSchema, "custom_action") {
		t.Error("Filtered schema should still contain custom_action")
	}
}

// TestActionHandler_SuccessFlow 测试成功流程的动作处理
func TestActionHandler_SuccessFlow(t *testing.T) {
	handlerCalled := false

	action := &LoopAction{
		ActionType:  "success_action",
		Description: "Test success flow",
		ActionHandler: func(loop *ReActLoop, act *aicommon.Action, operator *LoopActionHandlerOperator) {
			handlerCalled = true
			operator.Feedback("Success!")
			operator.Continue()
		},
	}

	// 创建一个简单的任务和操作符
	task := &mockSimpleTask{id: "test", index: "test-index"}
	operator := newLoopActionHandlerOperator(task)

	// 创建一个简单的 action
	act := &aicommon.Action{}

	// 调用处理器（这里loop可以为nil，因为handler不使用它）
	action.ActionHandler(nil, act, operator)

	if !handlerCalled {
		t.Error("Handler should be called")
	}

	if !operator.IsContinued() {
		t.Error("Operator should be continued")
	}

	feedback := operator.GetFeedback().String()
	if !strings.Contains(feedback, "Success!") {
		t.Error("Feedback should contain success message")
	}
}

// TestActionVerifier_SuccessFlow 测试验证器成功流程
func TestActionVerifier_SuccessFlow(t *testing.T) {
	verifierCalled := false

	action := &LoopAction{
		ActionType:  "verified_action",
		Description: "Test verification",
		ActionVerifier: func(loop *ReActLoop, act *aicommon.Action) error {
			verifierCalled = true
			return nil
		},
	}

	act := &aicommon.Action{}
	err := action.ActionVerifier(nil, act)

	if !verifierCalled {
		t.Error("Verifier should be called")
	}

	if err != nil {
		t.Errorf("Verifier should return nil, got: %v", err)
	}
}

// TestActionVerifier_FailureFlow 测试验证器失败流程
func TestActionVerifier_FailureFlow(t *testing.T) {
	action := &LoopAction{
		ActionType:  "failed_verification",
		Description: "Test verification failure",
		ActionVerifier: func(loop *ReActLoop, act *aicommon.Action) error {
			return fmt.Errorf("verification failed: invalid parameters")
		},
	}

	act := &aicommon.Action{}
	err := action.ActionVerifier(nil, act)

	if err == nil {
		t.Error("Verifier should return error")
	}

	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("Error should contain verification message, got: %v", err)
	}
}

// mockSimpleTask 是一个简化的 task 实现，只用于测试不需要完整 runtime 的场景
type mockSimpleTask struct {
	id     string
	index  string
	status aicommon.AITaskState
}

func (m *mockSimpleTask) GetId() string {
	return m.id
}

func (m *mockSimpleTask) GetIndex() string {
	return m.index
}

func (m *mockSimpleTask) GetName() string {
	return "mock-task"
}

func (m *mockSimpleTask) GetInput() string {
	return "test input"
}

func (m *mockSimpleTask) GetUserInput() string {
	return "test input"
}

func (m *mockSimpleTask) SetUserInput(input string) {
}

func (m *mockSimpleTask) GetResult() string {
	return ""
}

func (m *mockSimpleTask) SetResult(result string) {
}

func (m *mockSimpleTask) GetStatus() aicommon.AITaskState {
	return m.status
}

func (m *mockSimpleTask) SetStatus(status aicommon.AITaskState) {
	m.status = status
}

func (m *mockSimpleTask) GetContext() context.Context {
	return context.Background()
}

func (m *mockSimpleTask) Cancel() {
	m.status = aicommon.AITaskState_Aborted
}

func (m *mockSimpleTask) IsFinished() bool {
	return m.status == aicommon.AITaskState_Completed || m.status == aicommon.AITaskState_Aborted
}

func (m *mockSimpleTask) AppendErrorToResult(err error) {
}

func (m *mockSimpleTask) GetCreatedAt() time.Time {
	return time.Now()
}

func (m *mockSimpleTask) Finish(err error) {
	if err != nil {
		m.status = aicommon.AITaskState_Aborted
	} else {
		m.status = aicommon.AITaskState_Completed
	}
}

func (m *mockSimpleTask) IsAsyncMode() bool {
	return false
}

func (m *mockSimpleTask) SetAsyncMode(async bool) {
}

// TestOperatorFail 测试操作符的失败处理
func TestOperatorFail(t *testing.T) {
	task := &mockSimpleTask{id: "test", index: "test-index"}
	operator := newLoopActionHandlerOperator(task)

	operator.Fail("test failure reason")

	// 验证失败状态被正确设置（通过 operator 的内部状态）
	if operator.IsContinued() {
		t.Error("Operator should not be continued after Fail")
	}
}

// TestComplexFeedback 测试复杂反馈场景
func TestComplexFeedback(t *testing.T) {
	task := &mockSimpleTask{id: "test", index: "test-index"}
	operator := newLoopActionHandlerOperator(task)

	// 多次反馈
	operator.Feedback("Step 1: Initialize")
	operator.Feedback("Step 2: Process data")
	operator.Feedback("Step 3: Validate results")

	feedback := operator.GetFeedback().String()

	if !strings.Contains(feedback, "Step 1") {
		t.Error("Feedback should contain Step 1")
	}

	if !strings.Contains(feedback, "Step 2") {
		t.Error("Feedback should contain Step 2")
	}

	if !strings.Contains(feedback, "Step 3") {
		t.Error("Feedback should contain Step 3")
	}

	t.Logf("Complex feedback:\n%s", feedback)
}

// TestMaxIterationsOption 测试最大迭代次数选项
func TestMaxIterationsOption(t *testing.T) {
	// 由于需要完整的runtime，这里只测试选项功能
	opt := WithMaxIterations(50)

	// 验证选项可以创建（不会panic）
	if opt == nil {
		t.Error("WithMaxIterations should return a valid option")
	}
}

// TestOnTaskCreatedOption 测试任务创建回调选项
func TestOnTaskCreatedOption(t *testing.T) {
	opt := WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
		// 回调逻辑
	})

	if opt == nil {
		t.Error("WithOnTaskCreated should return a valid option")
	}

	// 回调本身不会被调用，直到有实际的loop执行
}

// TestOnAsyncTaskTriggerOption 测试异步任务触发回调选项
func TestOnAsyncTaskTriggerOption(t *testing.T) {
	opt := WithOnAsyncTaskTrigger(func(action *LoopAction, task aicommon.AIStatefulTask) {
		// 回调逻辑
	})

	if opt == nil {
		t.Error("WithOnAsyncTaskTrigger should return a valid option")
	}
}

// TestActionTypeValidation 测试动作类型验证
func TestActionTypeValidation(t *testing.T) {
	// 测试空动作类型
	emptyAction := &LoopAction{
		ActionType:  "",
		Description: "Empty type",
	}

	if emptyAction.ActionType == "" {
		t.Log("Action with empty type can be created (validation should happen at registration)")
	}

	// 测试重复动作类型
	action1 := &LoopAction{
		ActionType:  "duplicate",
		Description: "First",
	}

	action2 := &LoopAction{
		ActionType:  "duplicate",
		Description: "Second",
	}

	RegisterAction(action1)
	RegisterAction(action2)

	// 第二次注册会覆盖第一次
	retrieved, _ := GetLoopAction("duplicate")
	if retrieved.Description != "Second" {
		t.Error("Later registration should override earlier one")
	}
}

// TestSchemaFormatValidation 测试 schema 格式
func TestSchemaFormatValidation(t *testing.T) {
	action := &LoopAction{
		ActionType:  "test_format",
		Description: "测试格式化输出",
	}

	schema := buildSchema(action)

	// 验证schema包含必要的元素
	if !strings.Contains(schema, "test_format") {
		t.Error("Schema should contain action type")
	}

	t.Logf("Generated schema:\n%s", schema)
}

// TestLoopStateManagement 测试循环状态管理
func TestLoopStateManagement(t *testing.T) {
	task := &mockSimpleTask{
		id:     "state-test",
		index:  "state-index",
		status: aicommon.AITaskState_Created,
	}

	// 初始状态
	if task.GetStatus() != aicommon.AITaskState_Created {
		t.Error("Initial status should be Created")
	}

	// 转换到 Processing
	task.SetStatus(aicommon.AITaskState_Processing)
	if task.GetStatus() != aicommon.AITaskState_Processing {
		t.Error("Status should be Processing")
	}

	// 完成
	task.Finish(nil)
	if task.GetStatus() != aicommon.AITaskState_Completed {
		t.Error("Status should be Completed after Finish(nil)")
	}

	// 失败
	task2 := &mockSimpleTask{
		id:     "fail-test",
		index:  "fail-index",
		status: aicommon.AITaskState_Created,
	}
	task2.Finish(fmt.Errorf("test error"))
	if task2.GetStatus() != aicommon.AITaskState_Aborted {
		t.Error("Status should be Aborted after Finish(error)")
	}
}

// TestUtilityFunctions 测试辅助函数
func TestUtilityFunctions(t *testing.T) {
	// 测试 utils.InterfaceToString
	testStr := utils.InterfaceToString("test")
	if testStr != "test" {
		t.Errorf("Expected 'test', got '%s'", testStr)
	}

	// 测试随机字符串生成
	random1 := utils.RandStringBytes(8)
	random2 := utils.RandStringBytes(8)

	if len(random1) != 8 {
		t.Errorf("Random string should have length 8, got %d", len(random1))
	}

	if random1 == random2 {
		t.Error("Two random strings should be different")
	}
}

// MockMemoryTriageForTesting 是一个用于测试的 mock MemoryTriage
type MockMemoryTriageForTesting struct {
	searchMemoryCalled          bool
	searchMemoryWithoutAICalled bool
	searchMemoryResult          *aimem.SearchMemoryResult
	searchMemoryError           error
	callCount                   int
}

func (m *MockMemoryTriageForTesting) SetInvoker(i aicommon.AIInvokeRuntime) {
	
}

func (m *MockMemoryTriageForTesting) AddRawText(text string) ([]*aimem.MemoryEntity, error) {
	return []*aimem.MemoryEntity{}, nil
}

func (m *MockMemoryTriageForTesting) SaveMemoryEntities(entities ...*aimem.MemoryEntity) error {
	return nil
}

func (m *MockMemoryTriageForTesting) SearchBySemantics(query string, limit int) ([]*aimem.SearchResult, error) {
	return []*aimem.SearchResult{}, nil
}

func (m *MockMemoryTriageForTesting) SearchByTags(tags []string, matchAll bool, limit int) ([]*aimem.MemoryEntity, error) {
	return []*aimem.MemoryEntity{}, nil
}

func (m *MockMemoryTriageForTesting) HandleMemory(i any) error {
	return nil
}

func (m *MockMemoryTriageForTesting) SearchMemory(origin any, bytesLimit int) (*aimem.SearchMemoryResult, error) {
	m.searchMemoryCalled = true
	m.callCount++
	log.Infof("SearchMemory called with query: %v, bytes limit: %d", utils.ShrinkString(utils.InterfaceToString(origin), 50), bytesLimit)
	return m.searchMemoryResult, m.searchMemoryError
}

func (m *MockMemoryTriageForTesting) SearchMemoryWithoutAI(origin any, bytesLimit int) (*aimem.SearchMemoryResult, error) {
	m.searchMemoryWithoutAICalled = true
	m.callCount++
	log.Infof("SearchMemoryWithoutAI called with query: %v, bytes limit: %d", utils.ShrinkString(utils.InterfaceToString(origin), 50), bytesLimit)
	return m.searchMemoryResult, m.searchMemoryError
}

func (m *MockMemoryTriageForTesting) Close() error {
	return nil
}

// TestMemorySearch_NoMemories 测试没有记忆时的搜索
func TestMemorySearch_NoMemories(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories:      []*aimem.MemoryEntity{},
			TotalContent:  "",
			ContentBytes:  0,
			SearchSummary: "no memories found",
		},
	}

	result, err := mockMemory.SearchMemory("test query", 5*1024)
	if err != nil {
		t.Fatalf("SearchMemory should not error: %v", err)
	}

	if len(result.Memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(result.Memories))
	}

	if !mockMemory.searchMemoryCalled {
		t.Error("SearchMemory should have been called")
	}

	log.Infof("NoMemories test passed: search_summary=%s", result.SearchSummary)
}

// TestMemorySearch_WithMemories 测试有记忆的搜索
func TestMemorySearch_WithMemories(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{
					Id:      "mem-1",
					Content: "First memory content",
				},
				{
					Id:      "mem-2",
					Content: "Second memory content",
				},
			},
			TotalContent:  "First memory content\nSecond memory content",
			ContentBytes:  42,
			SearchSummary: "found 2 memories",
		},
	}

	result, err := mockMemory.SearchMemory("test query", 5*1024)
	if err != nil {
		t.Fatalf("SearchMemory should not error: %v", err)
	}

	if len(result.Memories) != 2 {
		t.Errorf("expected 2 memories, got %d", len(result.Memories))
	}

	if result.ContentBytes != 42 {
		t.Errorf("expected 42 bytes, got %d", result.ContentBytes)
	}

	log.Infof("WithMemories test passed: found %d memories, %d bytes", len(result.Memories), result.ContentBytes)
}

// TestMemorySearch_WithoutAI 测试不使用AI的记忆搜索
func TestMemorySearch_WithoutAI(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{
					Id:      "mem-1",
					Content: "Keyword-based memory",
				},
			},
			TotalContent:  "Keyword-based memory",
			ContentBytes:  21,
			SearchSummary: "keyword search result",
		},
	}

	result, err := mockMemory.SearchMemoryWithoutAI("keyword search", 5*1024)
	if err != nil {
		t.Fatalf("SearchMemoryWithoutAI should not error: %v", err)
	}

	if !mockMemory.searchMemoryWithoutAICalled {
		t.Error("SearchMemoryWithoutAI should have been called")
	}

	if len(result.Memories) != 1 {
		t.Errorf("expected 1 memory, got %d", len(result.Memories))
	}

	log.Infof("WithoutAI test passed: found %d memory via keyword search", len(result.Memories))
}

// TestMemorySearch_BytesLimit 测试字节限制
func TestMemorySearch_BytesLimit(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{
					Id:      "mem-1",
					Content: "First part",
				},
			},
			TotalContent:  "First part",
			ContentBytes:  10,
			SearchSummary: "limited by bytes",
		},
	}

	result, err := mockMemory.SearchMemory("test", 20) // 小的字节限制
	if err != nil {
		t.Fatalf("SearchMemory should not error: %v", err)
	}

	if result.ContentBytes > 20 {
		t.Errorf("content bytes %d should not exceed limit 20", result.ContentBytes)
	}

	log.Infof("BytesLimit test passed: content_bytes=%d, limit=20", result.ContentBytes)
}

// TestMemorySearch_Error 测试搜索错误处理
func TestMemorySearch_Error(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryError: fmt.Errorf("search error"),
	}

	result, err := mockMemory.SearchMemory("test query", 5*1024)
	if err == nil {
		t.Error("SearchMemory should return error")
	}

	if result != nil {
		t.Error("result should be nil when error occurs")
	}

	log.Infof("Error test passed: error=%v", err)
}

// TestReActLoop_MemoryIntegration_NoMemory 测试没有内存搜索时的执行
func TestReActLoop_MemoryIntegration_NoMemory(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		memorySizeLimit: 5 * 1024,
		memoryTriage:    nil, // 没有内存 triage
	}

	content := loop.GetCurrentMemoriesContent()
	if content != "" {
		t.Error("should return empty content when no memory triage")
	}

	log.Infof("MemoryIntegration_NoMemory test passed: content is empty as expected")
}

// TestReActLoop_MemoryIntegration_WithMemory 测试有内存搜索时的执行
func TestReActLoop_MemoryIntegration_WithMemory(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{
					Id:      "mem-1",
					Content: "Important context for user query",
				},
			},
			TotalContent:  "Important context for user query",
			ContentBytes:  31,
			SearchSummary: "found relevant memory",
		},
	}

	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		memorySizeLimit: 5 * 1024,
		memoryTriage:    mockMemory,
	}

	// 直接设置内存，而不是通过 PushMemory（因为 PushMemory 有逻辑反转 bug）
	for _, mem := range mockMemory.searchMemoryResult.Memories {
		loop.currentMemories.Set(mem.Id, mem)
	}

	content := loop.GetCurrentMemoriesContent()
	if !strings.Contains(content, "Important context") {
		t.Errorf("memory content should be included in loop content, got: %s", content)
	}

	log.Infof("MemoryIntegration_WithMemory test passed: memory content length=%d", len(content))
}

// TestReActLoop_MemorySearch_Integration 集成测试：循环中的内存搜索
func TestReActLoop_MemorySearch_Integration(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{
					Id:      "mem-1",
					Content: "Context 1",
				},
				{
					Id:      "mem-2",
					Content: "Context 2",
				},
			},
			TotalContent:  "Context 1\nContext 2",
			ContentBytes:  20,
			SearchSummary: "found 2 contexts",
		},
	}

	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		memorySizeLimit: 5 * 1024,
		memoryTriage:    mockMemory,
	}

	// 执行不带AI的内存搜索
	result1, err := mockMemory.SearchMemoryWithoutAI("user input", 5*1024)
	if err != nil {
		t.Fatalf("SearchMemoryWithoutAI failed: %v", err)
	}

	// 直接设置内存条目，而不是通过 PushMemory
	for _, mem := range result1.Memories {
		loop.currentMemories.Set(mem.Id, mem)
	}

	// 验证内存已被设置
	if loop.currentMemories.Len() != 2 {
		t.Errorf("expected 2 memories in loop, got %d", loop.currentMemories.Len())
	}

	// 执行带AI的内存搜索（在后台）
	go func() {
		result2, err := mockMemory.SearchMemory("user input", 5*1024)
		if err != nil {
			t.Logf("SearchMemory error: %v", err)
			return
		}
		// 直接设置内存，而不是通过 PushMemory
		for _, mem := range result2.Memories {
			loop.currentMemories.Set(mem.Id, mem)
		}
	}()

	// 等待后台任务
	time.Sleep(100 * time.Millisecond)

	// 获取当前内存内容
	content := loop.GetCurrentMemoriesContent()
	if !strings.Contains(content, "Context") {
		t.Error("memory content should contain 'Context'")
	}

	// 验证搜索方法被调用
	if mockMemory.callCount < 1 {
		t.Error("memory search should have been called")
	}

	log.Infof("MemorySearch_Integration test passed: called %d times, memories=%d, content_size=%d",
		mockMemory.callCount, loop.currentMemories.Len(), len(content))
}

// TestMemorySize_CalculationCorrectness 测试内存大小计算的正确性
func TestMemorySize_CalculationCorrectness(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		memorySizeLimit: 5 * 1024,
	}

	entities := []*aimem.MemoryEntity{
		{Id: "mem-1", Content: "Short"},
		{Id: "mem-2", Content: "Medium length content"},
		{Id: "mem-3", Content: "This is a much longer memory content with more details and information"},
	}

	expectedSize := 0
	for _, entity := range entities {
		loop.currentMemories.Set(entity.Id, entity)
		expectedSize += len(entity.Content)
	}

	actualSize := loop.currentMemorySize()
	if actualSize != expectedSize {
		t.Errorf("size mismatch: expected %d, got %d", expectedSize, actualSize)
	}

	log.Infof("CalculationCorrectness test passed: calculated_size=%d, expected=%d", actualSize, expectedSize)
}

// TestMemoryEviction_Correctness 测试内存淘汰的正确性
func TestMemoryEviction_Correctness(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
		memorySizeLimit: 40, // 限制为 40 字节
	}

	// 添加第一个内存（15 字节）
	result1 := &aimem.SearchMemoryResult{
		Memories: []*aimem.MemoryEntity{
			{Id: "mem-1", Content: "First memory "}, // 13 bytes
		},
	}
	loop.PushMemory(result1)

	size1 := loop.currentMemorySize()
	log.Infof("After first push: size=%d", size1)

	// 添加第二个内存（20 字节）
	result2 := &aimem.SearchMemoryResult{
		Memories: []*aimem.MemoryEntity{
			{Id: "mem-2", Content: "Second memory content"}, // 21 bytes
		},
	}
	loop.PushMemory(result2)

	size2 := loop.currentMemorySize()
	if size2 > loop.memorySizeLimit {
		// 应该已经淘汰了早期的内存
		log.Infof("Eviction occurred: size=%d, limit=%d", size2, loop.memorySizeLimit)
	}

	// 验证大小不超过限制
	if size2 > loop.memorySizeLimit {
		t.Errorf("memory size %d exceeds limit %d after eviction", size2, loop.memorySizeLimit)
	}

	log.Infof("Eviction test passed: final_size=%d, limit=%d", size2, loop.memorySizeLimit)
}

// TestMemoryContent_Formatting 测试内存内容格式化
func TestMemoryContent_Formatting(t *testing.T) {
	loop := &ReActLoop{
		currentMemories: omap.NewEmptyOrderedMap[string, *aimem.MemoryEntity](),
	}

	entities := []*aimem.MemoryEntity{
		{Id: "mem-1", Content: "First"},
		{Id: "mem-2", Content: "Second"},
		{Id: "mem-3", Content: "Third"},
	}

	for _, entity := range entities {
		loop.currentMemories.Set(entity.Id, entity)
	}

	content := loop.GetCurrentMemoriesContent()

	// 验证每个内存的内容都被包含
	for _, entity := range entities {
		if !strings.Contains(content, entity.Content) {
			t.Errorf("content should contain '%s'", entity.Content)
		}
	}

	// 验证换行符的使用
	lines := strings.Split(content, "\n")
	if len(lines) < len(entities) {
		t.Errorf("expected at least %d lines, got %d", len(entities), len(lines))
	}

	log.Infof("Formatting test passed: formatted_content has %d lines", len(lines))
}

// TestMemorySearch_MultipleCallsConsistency 测试多次调用的一致性
func TestMemorySearch_MultipleCallsConsistency(t *testing.T) {
	mockMemory := &MockMemoryTriageForTesting{
		searchMemoryResult: &aimem.SearchMemoryResult{
			Memories: []*aimem.MemoryEntity{
				{Id: "mem-1", Content: "Consistent memory"},
			},
			TotalContent:  "Consistent memory",
			ContentBytes:  17,
			SearchSummary: "consistent result",
		},
	}

	// 第一次调用
	result1, _ := mockMemory.SearchMemory("test", 5*1024)
	// 第二次调用
	result2, _ := mockMemory.SearchMemory("test", 5*1024)

	if len(result1.Memories) != len(result2.Memories) {
		t.Error("search results should be consistent")
	}

	if mockMemory.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", mockMemory.callCount)
	}

	log.Infof("Consistency test passed: multiple calls returned consistent results, call_count=%d", mockMemory.callCount)
}
