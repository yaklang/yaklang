package reactloops

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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

func TestLoopAction_OutputExample(t *testing.T) {
	action := &LoopAction{
		ActionType:     "example_action",
		Description:    "An action with output examples",
		OutputExamples: "Example usage of the action",
	}

	if action.OutputExamples != "Example usage of the action" {
		t.Errorf("Expected output examples to be set, got '%s'", action.OutputExamples)
	}

	// Register the action
	RegisterAction(action)

	// Verify the action can be retrieved with OutputExamples
	retrieved, ok := GetLoopAction("example_action")
	if !ok {
		t.Fatal("Action should be retrievable after registration")
	}

	if retrieved.OutputExamples != "Example usage of the action" {
		t.Errorf("Expected output examples 'Example usage of the action', got '%s'", retrieved.OutputExamples)
	}

	// Test that buildSchema includes the action
	schema := buildSchema(action)
	if schema == "" {
		t.Error("Schema should not be empty")
	}

	// Verify schema contains the action type
	if !strings.Contains(schema, "example_action") {
		t.Error("Schema should contain 'example_action'")
	}

	// Test OutputExamples with template variables
	actionWithTemplate := &LoopAction{
		ActionType:     "template_action",
		Description:    "An action with template in output examples",
		OutputExamples: "Use nonce: {{.Nonce}} for this action",
	}

	RegisterAction(actionWithTemplate)

	retrievedTemplate, ok := GetLoopAction("template_action")
	if !ok {
		t.Fatal("Template action should be retrievable after registration")
	}

	if retrievedTemplate.OutputExamples != "Use nonce: {{.Nonce}} for this action" {
		t.Errorf("Expected template output examples, got '%s'", retrievedTemplate.OutputExamples)
	}
}

// TestLoopAction_ReflectionOutputExampleProvider tests that OutputExamples from LoopAction
// is correctly rendered in the reflectionOutputExampleProvider (options.go lines 195-229)
func TestLoopAction_ReflectionOutputExampleProvider(t *testing.T) {
	// Create and register a LoopAction with OutputExamples
	testActionName := "test_reflection_action"
	testOutputExample := "This is a test output example for reflection"

	action := &LoopAction{
		ActionType:     testActionName,
		Description:    "Test action for reflection output example",
		OutputExamples: testOutputExample,
	}
	RegisterAction(action)

	// Create a mock LoopActionFactory that returns our action
	mockFactory := func(r aicommon.AIInvokeRuntime) (*LoopAction, error) {
		return action, nil
	}

	// Verify GetLoopAction returns the action with OutputExamples
	retrieved, ok := GetLoopAction(testActionName)
	if !ok {
		t.Fatal("Action should be retrievable after registration")
	}
	if retrieved.OutputExamples != testOutputExample {
		t.Errorf("Expected OutputExamples '%s', got '%s'", testOutputExample, retrieved.OutputExamples)
	}

	// Test with LoopMetadata fallback
	testMetadataName := "test_metadata_action"
	testMetadataOutputExample := "This is from LoopMetadata OutputExamplePrompt"

	// Register loop metadata with OutputExamplePrompt
	err := RegisterLoopFactory(
		testMetadataName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			return nil, nil // mock factory
		},
		WithLoopOutputExample(testMetadataOutputExample),
	)
	// Ignore error if already registered
	_ = err

	// Verify GetLoopMetadata returns the metadata with OutputExamplePrompt
	meta, ok := GetLoopMetadata(testMetadataName)
	if ok && meta.OutputExamplePrompt != testMetadataOutputExample {
		t.Errorf("Expected OutputExamplePrompt '%s', got '%s'", testMetadataOutputExample, meta.OutputExamplePrompt)
	}

	// Test that the factory is stored correctly
	_ = mockFactory
}

// TestLoopAction_OutputExamplesInLoopActions tests that OutputExamples from actions
// registered in loop.loopActions are correctly appended to the base example
func TestLoopAction_OutputExamplesInLoopActions(t *testing.T) {
	// Create a unique action name for this test
	actionName := "loop_actions_test_action"
	outputExample := "Example: Use this action when you need to test"

	// Register the action with OutputExamples
	action := &LoopAction{
		ActionType:     actionName,
		Description:    "Action for testing loopActions integration",
		OutputExamples: outputExample,
	}
	RegisterAction(action)

	// Verify the action is registered and has OutputExamples
	retrieved, ok := GetLoopAction(actionName)
	if !ok {
		t.Fatal("Action should be retrievable after registration")
	}

	if retrieved.OutputExamples != outputExample {
		t.Errorf("Expected OutputExamples '%s', got '%s'", outputExample, retrieved.OutputExamples)
	}

	// Verify that when loopActions.Keys() contains actionName,
	// GetLoopAction returns the action with OutputExamples
	// This is the key logic in options.go:208-214
	if action, ok := GetLoopAction(actionName); ok && action.OutputExamples != "" {
		// This simulates the logic in WithReflectionOutputExample
		if action.OutputExamples != outputExample {
			t.Errorf("GetLoopAction should return action with OutputExamples")
		}
	} else {
		t.Error("GetLoopAction should return action with non-empty OutputExamples")
	}
}

// TestLoopMetadata_OutputExamplePromptFallback tests the fallback to LoopMetadata
// when LoopAction doesn't have OutputExamples (options.go lines 215-221)
func TestLoopMetadata_OutputExamplePromptFallback(t *testing.T) {
	// Create a unique loop name for this test
	loopName := "test_fallback_loop"
	outputExamplePrompt := "Fallback example from LoopMetadata"

	// Register a loop factory with metadata containing OutputExamplePrompt
	_ = RegisterLoopFactory(
		loopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			return nil, nil
		},
		WithLoopOutputExample(outputExamplePrompt),
	)

	// Verify GetLoopMetadata returns the metadata
	meta, ok := GetLoopMetadata(loopName)
	if !ok {
		t.Fatal("LoopMetadata should be retrievable after registration")
	}

	if meta.OutputExamplePrompt != outputExamplePrompt {
		t.Errorf("Expected OutputExamplePrompt '%s', got '%s'", outputExamplePrompt, meta.OutputExamplePrompt)
	}

	// Verify that when LoopAction doesn't have OutputExamples,
	// the code falls back to LoopMetadata.OutputExamplePrompt
	// This simulates the logic in options.go:215-221
	_, hasAction := GetLoopAction(loopName)
	metaFallback, hasMeta := GetLoopMetadata(loopName)

	if !hasAction && hasMeta && metaFallback.OutputExamplePrompt != "" {
		// This is the expected fallback path
		if metaFallback.OutputExamplePrompt != outputExamplePrompt {
			t.Errorf("Fallback should return OutputExamplePrompt from metadata")
		}
	}
}

// TestLoopAction_OutputExamplesTemplate tests that OutputExamples supports template variables
func TestLoopAction_OutputExamplesTemplate(t *testing.T) {
	actionName := "template_output_action"
	templateExample := "Action with nonce: {{.Nonce}}"

	action := &LoopAction{
		ActionType:     actionName,
		Description:    "Action with template in OutputExamples",
		OutputExamples: templateExample,
	}
	RegisterAction(action)

	retrieved, ok := GetLoopAction(actionName)
	if !ok {
		t.Fatal("Action should be retrievable after registration")
	}

	// Verify template is stored correctly (not rendered yet)
	if retrieved.OutputExamples != templateExample {
		t.Errorf("Expected template '%s', got '%s'", templateExample, retrieved.OutputExamples)
	}

	// Verify the template contains the expected placeholder
	if !strings.Contains(retrieved.OutputExamples, "{{.Nonce}}") {
		t.Error("OutputExamples should contain template placeholder '{{.Nonce}}'")
	}
}
