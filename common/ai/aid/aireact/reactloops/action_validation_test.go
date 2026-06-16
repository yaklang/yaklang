package reactloops

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestSchemaValidationExample demonstrates how the oneOf schema provides better validation
func TestSchemaValidationExample(t *testing.T) {
	// Define two actions with different parameters
	readAction := &LoopAction{
		ActionType:  "read_file",
		Description: "Read content from a file",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"file_path",
				aitool.WithParam_Description("Path to the file to read"),
				aitool.WithParam_Required(true),
			),
			aitool.WithIntegerParam(
				"max_lines",
				aitool.WithParam_Description("Maximum number of lines to read"),
			),
		},
	}

	writeAction := &LoopAction{
		ActionType:  "write_file",
		Description: "Write content to a file",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"file_path",
				aitool.WithParam_Description("Path to the file to write"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam(
				"content",
				aitool.WithParam_Description("Content to write to the file"),
				aitool.WithParam_Required(true),
			),
			aitool.WithBoolParam(
				"append",
				aitool.WithParam_Description("Whether to append to existing file"),
			),
		},
	}

	// Generate schema
	schemaStr := buildSchema(readAction, writeAction)

	// Parse and pretty print
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		t.Fatalf("Failed to parse schema: %v", err)
	}

	prettySchema, _ := json.MarshalIndent(schema, "", "  ")
	fmt.Println("Generated OneOf Schema:")
	fmt.Println(string(prettySchema))

	// Verify oneOf structure
	oneOf, exists := schema["oneOf"].([]interface{})
	if !exists {
		t.Fatal("Expected oneOf in schema")
	}

	if len(oneOf) != 2 {
		t.Fatalf("Expected 2 action schemas, got %d", len(oneOf))
	}

	// Verify read_file schema
	readSchema := oneOf[0].(map[string]interface{})
	readProps := readSchema["properties"].(map[string]interface{})

	// Should have: @action, identifier, human_readable_thought, file_path, max_lines
	if len(readProps) != 5 {
		t.Errorf("read_file schema should have 5 properties, got %d", len(readProps))
	}

	if _, hasMaxLines := readProps["max_lines"]; !hasMaxLines {
		t.Error("read_file schema should have max_lines parameter")
	}

	// Should NOT have write-specific parameters
	if _, hasContent := readProps["content"]; hasContent {
		t.Error("read_file schema should NOT have content parameter (that's write_file's)")
	}

	if _, hasAppend := readProps["append"]; hasAppend {
		t.Error("read_file schema should NOT have append parameter (that's write_file's)")
	}

	// Verify write_file schema
	writeSchema := oneOf[1].(map[string]interface{})
	writeProps := writeSchema["properties"].(map[string]interface{})

	// Should have: @action, identifier, human_readable_thought, file_path, content, append
	if len(writeProps) != 6 {
		t.Errorf("write_file schema should have 6 properties, got %d", len(writeProps))
	}

	if _, hasContent := writeProps["content"]; !hasContent {
		t.Error("write_file schema should have content parameter")
	}

	if _, hasAppend := writeProps["append"]; !hasAppend {
		t.Error("write_file schema should have append parameter")
	}

	// Should NOT have read-specific parameters
	if _, hasMaxLines := writeProps["max_lines"]; hasMaxLines {
		t.Error("write_file schema should NOT have max_lines parameter (that's read_file's)")
	}

	t.Log("✓ Schema correctly isolates parameters per action")
	t.Log("✓ Each action has only its relevant parameters")
	t.Log("✓ No parameter mixing between actions")
}

// TestSchemaRequiredFields verifies that required fields are correctly set per action
func TestSchemaRequiredFields(t *testing.T) {
	action1 := &LoopAction{
		ActionType:  "action_with_required",
		Description: "Action with required parameters",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"required_param",
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam(
				"optional_param",
			),
		},
	}

	action2 := &LoopAction{
		ActionType:  "action_all_optional",
		Description: "Action with all optional parameters",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"opt1",
			),
			aitool.WithStringParam(
				"opt2",
			),
		},
	}

	schemaStr := buildSchema(action1, action2)

	var schema map[string]interface{}
	json.Unmarshal([]byte(schemaStr), &schema)

	oneOf := schema["oneOf"].([]interface{})

	// Check first action's required fields
	schema1 := oneOf[0].(map[string]interface{})
	required1 := schema1["required"].([]interface{})

	// Should require: @action, identifier, required_param
	expectedRequired := map[string]bool{
		"@action":        true,
		"identifier":     true,
		"required_param": true,
	}

	for _, req := range required1 {
		reqStr := req.(string)
		if !expectedRequired[reqStr] {
			t.Errorf("Unexpected required field: %s", reqStr)
		}
		delete(expectedRequired, reqStr)
	}

	if len(expectedRequired) > 0 {
		t.Errorf("Missing required fields: %v", expectedRequired)
	}

	// Check second action's required fields
	schema2 := oneOf[1].(map[string]interface{})
	required2 := schema2["required"].([]interface{})

	// Should only require: @action, identifier (both opt1 and opt2 are optional)
	if len(required2) != 2 {
		t.Errorf("action_all_optional should have 2 required fields, got %d: %v", len(required2), required2)
	}

	t.Log("✓ Required fields correctly set per action")
	t.Log("✓ Optional parameters not marked as required")
}
