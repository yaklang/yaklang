package jsonpath

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
)

const (
	raw = `{
    "store": {
        "book": [
            {
                "category": "reference",
                "author": "Nigel Rees",
                "title": "Sayings of the Century",
                "price": 8.95
            },
            {
                "category": "fiction",
                "author": "Evelyn Waugh",
                "title": "Sword of Honour",
                "price": 12.99
            },
            {
                "category": "fiction",
                "author": "Herman Melville",
                "title": "Moby Dick",
                "isbn": "0-553-21311-3",
                "price": 8.99
            },
            {
                "category": "fiction",
                "author": "J. R. R. Tolkien",
                "title": "The Lord of the Rings",
                "isbn": "0-395-19395-8",
                "price": 22.99
            }
        ],
        "bicycle": {
            "color": "red",
            "price": 19.95
        }
    },
    "expensive": 10
}`
)

func TestRead1(t *testing.T) {
	var a = Find(raw, "$..bicycle.color")
	spew.Dump(a)
	var b = FindFirst(raw, "$..bicycle.color")
	spew.Dump(b)
}

func TestReadAll(t *testing.T) {
	var result = Find(raw, `$..*`)
	spew.Dump(result)
}

func TestReplaceAll_ByteSlice(t *testing.T) {
	// Test case 1: []byte input (like json.Marshal result)
	jsonBytes := []byte(`{"key":"123"}`)
	result := ReplaceAll(jsonBytes, "$.key", "111111")
	if result == nil {
		t.Fatalf("ReplaceAll with []byte input should not return nil")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("ReplaceAll result should be map[string]interface{}, got %T", result)
	}
	if resultMap["key"] != "111111" {
		t.Fatalf("expected key to be '111111', got '%v'", resultMap["key"])
	}
	t.Logf("ReplaceAll with []byte input succeeded: %+v", result)
}

func TestReplaceAll_Map(t *testing.T) {
	// Test case 2: map input
	inputMap := map[string]interface{}{
		"key": "123",
	}
	result := ReplaceAll(inputMap, "$.key", "111111")
	if result == nil {
		t.Fatalf("ReplaceAll with map input should not return nil")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("ReplaceAll result should be map[string]interface{}, got %T", result)
	}
	if resultMap["key"] != "111111" {
		t.Fatalf("expected key to be '111111', got '%v'", resultMap["key"])
	}
	t.Logf("ReplaceAll with map input succeeded: %+v", result)
}

func TestReplaceAll_NestedMap(t *testing.T) {
	// Test case 3: nested map input
	inputMap := map[string]interface{}{
		"outer": map[string]interface{}{
			"inner": "value",
		},
	}
	result := ReplaceAll(inputMap, "$.outer.inner", "new_value")
	if result == nil {
		t.Fatalf("ReplaceAll with nested map input should not return nil")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("ReplaceAll result should be map[string]interface{}, got %T", result)
	}
	outerMap, ok := resultMap["outer"].(map[string]interface{})
	if !ok {
		t.Fatalf("outer should be map[string]interface{}, got %T", resultMap["outer"])
	}
	if outerMap["inner"] != "new_value" {
		t.Fatalf("expected inner to be 'new_value', got '%v'", outerMap["inner"])
	}
	t.Logf("ReplaceAll with nested map input succeeded: %+v", result)
}

func TestReplaceAll_StringInput(t *testing.T) {
	// Test case 4: string input (original behavior)
	inputStr := `{"key":"123"}`
	result := ReplaceAll(inputStr, "$.key", "111111")
	if result == nil {
		t.Fatalf("ReplaceAll with string input should not return nil")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("ReplaceAll result should be map[string]interface{}, got %T", result)
	}
	if resultMap["key"] != "111111" {
		t.Fatalf("expected key to be '111111', got '%v'", resultMap["key"])
	}
	t.Logf("ReplaceAll with string input succeeded: %+v", result)
}

func TestReplaceAll_MapWithStringKey(t *testing.T) {
	// Test case 5: map[string]string input
	inputMap := map[string]string{
		"key": "123",
	}
	result := ReplaceAll(inputMap, "$.key", "111111")
	if result == nil {
		t.Fatalf("ReplaceAll with map[string]string input should not return nil")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("ReplaceAll result should be map[string]interface{}, got %T", result)
	}
	if resultMap["key"] != "111111" {
		t.Fatalf("expected key to be '111111', got '%v'", resultMap["key"])
	}
	t.Logf("ReplaceAll with map[string]string input succeeded: %+v", result)
}
