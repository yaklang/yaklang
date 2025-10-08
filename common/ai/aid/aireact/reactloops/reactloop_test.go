package reactloops

import (
	"strings"
	"testing"
)

// TestRegisterAction 测试动作注册功能
func TestRegisterAction(t *testing.T) {
	action := &LoopAction{
		ActionType:  "test-action",
		Description: "Test action",
	}

	RegisterAction(action)

	retrieved, ok := GetLoopAction("test-action")
	if !ok {
		t.Fatal("Action should be retrievable after registration")
	}

	if retrieved.ActionType != "test-action" {
		t.Errorf("Expected action type 'test-action', got '%s'", retrieved.ActionType)
	}

	if retrieved.Description != "Test action" {
		t.Errorf("Expected description 'Test action', got '%s'", retrieved.Description)
	}
}

// TestRegisterAction_Duplicate 测试重复注册动作（应该允许覆盖）
func TestRegisterAction_Duplicate(t *testing.T) {
	action1 := &LoopAction{
		ActionType:  "duplicate-action",
		Description: "First action",
	}

	action2 := &LoopAction{
		ActionType:  "duplicate-action",
		Description: "Second action",
	}

	RegisterAction(action1)
	RegisterAction(action2)

	retrieved, ok := GetLoopAction("duplicate-action")
	if !ok {
		t.Fatal("Action should be retrievable")
	}

	// 应该返回最后注册的动作
	if retrieved.Description != "Second action" {
		t.Errorf("Expected description 'Second action', got '%s'", retrieved.Description)
	}
}

// TestGetLoopAction_NotFound 测试获取不存在的动作
func TestGetLoopAction_NotFound(t *testing.T) {
	_, ok := GetLoopAction("non-existent-action")
	if ok {
		t.Error("Should return false for non-existent action")
	}
}

// TestCreateLoopByName_NotFound 测试创建不存在的循环
func TestCreateLoopByName_NotFound(t *testing.T) {
	_, err := CreateLoopByName("non-existent-factory", nil)
	if err == nil {
		t.Error("Should return error for non-existent factory")
	}
}

// TestLoopAction_BuiltinActionsExist 测试内置动作变量是否存在
func TestLoopAction_BuiltinActionsExist(t *testing.T) {
	// 测试内置动作变量是否定义
	if loopAction_DirectlyAnswer == nil {
		t.Error("loopAction_DirectlyAnswer should not be nil")
	}
	if loopAction_Finish == nil {
		t.Error("loopAction_Finish should not be nil")
	}

	// 验证动作的基本属性
	if loopAction_DirectlyAnswer.ActionType != "directly_answer" {
		t.Errorf("Expected action type 'directly_answer', got '%s'", loopAction_DirectlyAnswer.ActionType)
	}
	if loopAction_Finish.ActionType != "finish" {
		t.Errorf("Expected action type 'finish', got '%s'", loopAction_Finish.ActionType)
	}
}

// TestLoopAction_BuildSchema 测试动作架构构建
func TestLoopAction_BuildSchema(t *testing.T) {
	actions := []*LoopAction{
		{
			ActionType:  "test_action",
			Description: "Test action",
		},
		{
			ActionType:  "another_action",
			Description: "Another action",
		},
	}

	schema := buildSchema(actions...)
	if schema == "" {
		t.Error("Schema should not be empty")
	}

	// 验证 schema 包含必要的字段
	expectedFields := []string{"@action", "human_readable_thought", "test_action", "another_action"}
	for _, field := range expectedFields {
		if !strings.Contains(schema, field) {
			t.Errorf("Schema should contain field '%s'", field)
		}
	}
}
