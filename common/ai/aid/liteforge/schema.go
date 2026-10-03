package liteforge

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// The action marker belongs to the text transport. Native arguments use the
// same business schema with that root property removed. Nested schemas, open
// maps and unconstrained {} fields are preserved verbatim.
func outputSchema(raw, name string, native bool) (map[string]any, *jsonschema.Schema, error) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return nil, nil, fmt.Errorf("invalid liteforge output schema: %w", err)
	}
	if schema == nil {
		return nil, nil, fmt.Errorf("liteforge output schema must describe an object")
	}
	if kind, ok := schema["type"]; ok && kind != "object" {
		return nil, nil, fmt.Errorf("liteforge output is an object; put arbitrary JSON values in a field with schema {}")
	}
	properties, _ := schema["properties"].(map[string]any)
	if marker, ok := properties["@action"].(map[string]any); ok {
		if constant, ok := marker["const"]; ok && constant != name {
			return nil, nil, fmt.Errorf("liteforge @action const %v does not match %q", constant, name)
		}
	}
	if native {
		delete(properties, "@action")
		if required, ok := schema["required"].([]any); ok {
			filtered := make([]any, 0, len(required))
			for _, key := range required {
				if key != "@action" {
					filtered = append(filtered, key)
				}
			}
			schema["required"] = filtered
		}
		// Native providers require an object parameter schema, even for {}.
		schema["type"] = "object"
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("liteforge-output.json", schema); err != nil {
		return nil, nil, err
	}
	validator, err := compiler.Compile("liteforge-output.json")
	return schema, validator, err
}

func admitOutput(value map[string]any, name string, validator *jsonschema.Schema, native bool) (*aicommon.Action, error) {
	if value == nil {
		return nil, fmt.Errorf("liteforge requires a complete output object")
	}
	if !native {
		if marker, exists := value["@action"]; exists && marker != name {
			return nil, fmt.Errorf("unexpected liteforge action %v, expected %q", marker, name)
		}
	}
	if err := validator.Validate(value); err != nil {
		return nil, fmt.Errorf("liteforge output schema validation failed: %w", err)
	}
	value["@action"] = name
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return aicommon.ExtractAction(string(data), name)
}
