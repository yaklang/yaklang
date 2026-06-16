package reactloops

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestBuildSchemaOneOf(t *testing.T) {
	// Create test actions
	action1 := &LoopAction{
		ActionType:  "test_action1",
		Description: "Test action 1 description",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"param1",
				aitool.WithParam_Description("Parameter 1 for action 1"),
				aitool.WithParam_Required(true),
			),
		},
	}

	action2 := &LoopAction{
		ActionType:  "test_action2",
		Description: "Test action 2 description",
		Options: []aitool.ToolOption{
			aitool.WithStringParam(
				"param2",
				aitool.WithParam_Description("Parameter 2 for action 2"),
				aitool.WithParam_Required(true),
			),
			aitool.WithIntegerParam(
				"count",
				aitool.WithParam_Description("Count parameter"),
			),
		},
	}

	// Build schema
	schemaStr := buildSchema(action1, action2)

	// Parse schema to verify structure
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		t.Fatalf("Failed to parse schema: %v", err)
	}

	t.Logf("Generated schema:\n%s", schemaStr)

	// Verify top-level structure
	if schema["type"] != "object" {
		t.Errorf("Expected type 'object', got %v", schema["type"])
	}

	// Verify oneOf exists at the root level
	oneOf, exists := schema["oneOf"]
	if !exists {
		t.Fatalf("Expected oneOf at root level")
	}

	oneOfArray, ok := oneOf.([]interface{})
	if !ok {
		t.Fatalf("Expected oneOf to be an array")
	}

	if len(oneOfArray) != 2 {
		t.Errorf("Expected 2 oneOf schemas, got %d", len(oneOfArray))
	}

	// Verify each oneOf schema has the expected structure
	for i, schemaItem := range oneOfArray {
		schemaMap, ok := schemaItem.(map[string]interface{})
		if !ok {
			t.Errorf("oneOf[%d] is not a map", i)
			continue
		}

		// Check for properties
		props, ok := schemaMap["properties"].(map[string]interface{})
		if !ok {
			t.Errorf("oneOf[%d] missing properties", i)
			continue
		}

		// Verify @action field exists
		actionField, exists := props["@action"]
		if !exists {
			t.Errorf("oneOf[%d] missing @action field", i)
		} else {
			actionMap, ok := actionField.(map[string]interface{})
			if ok {
				// Verify enum has exactly one value (the specific action type)
				if enum, exists := actionMap["enum"]; exists {
					enumArray, ok := enum.([]interface{})
					if ok && len(enumArray) == 1 {
						t.Logf("oneOf[%d] @action enum: %v", i, enumArray[0])
					} else {
						t.Errorf("oneOf[%d] @action enum should have exactly 1 value", i)
					}
				}
			}
		}

		// Verify identifier field exists
		if _, exists := props["identifier"]; !exists {
			t.Errorf("oneOf[%d] missing identifier field", i)
		}

		// Verify human_readable_thought field exists
		if _, exists := props["human_readable_thought"]; !exists {
			t.Errorf("oneOf[%d] missing human_readable_thought field", i)
		}

		// Check required fields
		required, ok := schemaMap["required"].([]interface{})
		if !ok {
			t.Errorf("oneOf[%d] missing or invalid required array", i)
		} else {
			t.Logf("oneOf[%d] required fields: %v", i, required)
		}

		t.Logf("oneOf[%d] has %d properties", i, len(props))
	}

	t.Logf("Successfully verified oneOf structure with %d schemas", len(oneOfArray))
}
