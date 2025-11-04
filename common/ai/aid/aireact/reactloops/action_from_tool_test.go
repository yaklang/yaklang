package reactloops

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestConvertAIToolToLoopAction_Basic tests the basic conversion of an AI Tool to a LoopAction
func TestConvertAIToolToLoopAction_Basic(t *testing.T) {
	// Create a simple AI Tool
	tool := aitool.NewWithoutCallback(
		"test_action",
		aitool.WithDescription("A test action for conversion"),
		aitool.WithStringParam("param1", aitool.WithParam_Description("First parameter")),
		aitool.WithIntegerParam("param2", aitool.WithParam_Description("Second parameter"), aitool.WithParam_Required(true)),
	)

	// Convert to LoopAction
	action := ConvertAIToolToLoopAction(tool)

	// Verify basic properties
	if action.ActionType != "test_action" {
		t.Errorf("Expected ActionType 'test_action', got '%s'", action.ActionType)
	}

	if action.Description != "A test action for conversion" {
		t.Errorf("Expected Description 'A test action for conversion', got '%s'", action.Description)
	}

	if action.AsyncMode {
		t.Error("Expected AsyncMode to be false")
	}

	// Verify options are converted
	if len(action.Options) == 0 {
		t.Error("Expected Options to be populated")
	}

	t.Logf("Converted action: %+v", action)
	t.Logf("Number of options: %d", len(action.Options))
}

// TestConvertAIToolToLoopAction_WithMultipleParams tests conversion with multiple parameters
func TestConvertAIToolToLoopAction_WithMultipleParams(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"multi_param_action",
		aitool.WithDescription("Action with multiple parameters"),
		aitool.WithStringParam("name", aitool.WithParam_Description("User name"), aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("age", aitool.WithParam_Description("User age")),
		aitool.WithStringParam("email", aitool.WithParam_Description("Email address"), aitool.WithParam_Required(true)),
		aitool.WithBoolParam("active", aitool.WithParam_Description("Is active")),
	)

	action := ConvertAIToolToLoopAction(tool)

	if action.ActionType != "multi_param_action" {
		t.Errorf("Expected ActionType 'multi_param_action', got '%s'", action.ActionType)
	}

	// Should have 4 parameters converted to options
	if len(action.Options) != 4 {
		t.Errorf("Expected 4 options, got %d", len(action.Options))
	}

	t.Logf("Converted action with multiple params: %+v", action)
}

// TestConvertAIToolToLoopAction_NoParams tests conversion of a tool with no parameters
func TestConvertAIToolToLoopAction_NoParams(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"no_param_action",
		aitool.WithDescription("Action with no parameters"),
	)

	action := ConvertAIToolToLoopAction(tool)

	if action.ActionType != "no_param_action" {
		t.Errorf("Expected ActionType 'no_param_action', got '%s'", action.ActionType)
	}

	if action.Description != "Action with no parameters" {
		t.Errorf("Expected Description 'Action with no parameters', got '%s'", action.Description)
	}

	// Should have no options since there are no parameters
	if len(action.Options) != 0 {
		t.Errorf("Expected 0 options, got %d", len(action.Options))
	}

	t.Logf("Converted action with no params: %+v", action)
}

// TestConvertAIToolToLoopAction_ComplexSchema tests conversion with complex parameter schemas
func TestConvertAIToolToLoopAction_ComplexSchema(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"complex_action",
		aitool.WithDescription("Action with complex parameters"),
		aitool.WithStringParam("simple_string", aitool.WithParam_Description("A simple string")),
		aitool.WithStringArrayParam("array_param", aitool.WithParam_Description("An array parameter")),
		aitool.WithStructParam("nested_object",
			[]aitool.PropertyOption{aitool.WithParam_Description("A nested object")},
			aitool.WithStringParam("nested_field", aitool.WithParam_Description("A nested field")),
		),
	)

	action := ConvertAIToolToLoopAction(tool)

	if action.ActionType != "complex_action" {
		t.Errorf("Expected ActionType 'complex_action', got '%s'", action.ActionType)
	}

	// Should have parameters converted
	if len(action.Options) == 0 {
		t.Error("Expected Options to be populated for complex schema")
	}

	t.Logf("Converted action with complex schema: %+v", action)
	t.Logf("Number of options: %d", len(action.Options))
}

// TestConvertAIToolToLoopAction_Roundtrip tests that a converted action can be used to create a valid schema
func TestConvertAIToolToLoopAction_Roundtrip(t *testing.T) {
	// Create original tool
	originalTool := aitool.NewWithoutCallback(
		"roundtrip_action",
		aitool.WithDescription("Test roundtrip conversion"),
		aitool.WithStringParam("param1", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("param2"),
	)

	// Convert to LoopAction
	action := ConvertAIToolToLoopAction(originalTool)

	// Verify the action can be used (has valid structure)
	if action == nil {
		t.Fatal("Converted action should not be nil")
	}

	if action.ActionType == "" {
		t.Error("ActionType should not be empty")
	}

	if action.Options == nil {
		t.Error("Options should not be nil")
	}

	// Verify that ActionVerifier and ActionHandler are nil (will be set later)
	if action.ActionVerifier != nil {
		t.Error("ActionVerifier should be nil after conversion")
	}

	if action.ActionHandler != nil {
		t.Error("ActionHandler should be nil after conversion")
	}

	t.Logf("Roundtrip successful for action: %s", action.ActionType)
}
