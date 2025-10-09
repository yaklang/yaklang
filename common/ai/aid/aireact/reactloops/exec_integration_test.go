package reactloops

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
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

// TestAITagFieldsManagement 测试 AI 标签字段管理
func TestAITagFieldsManagement(t *testing.T) {
	// 由于 ReActLoop 需要完整的runtime，这里只测试字段结构
	field := &LoopAITagField{
		TagName:      "test-tag",
		VariableName: "test_var",
	}

	if field.TagName != "test-tag" {
		t.Error("TagName should be set correctly")
	}

	if field.VariableName != "test_var" {
		t.Error("VariableName should be set correctly")
	}
}

// TestStreamFieldsManagement 测试流字段管理
func TestStreamFieldsManagement(t *testing.T) {
	field := &LoopStreamField{
		FieldName: "thought",
		Prefix:    "Thinking",
	}

	if field.FieldName != "thought" {
		t.Error("FieldName should be set correctly")
	}

	if field.Prefix != "Thinking" {
		t.Error("Prefix should be set correctly")
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
