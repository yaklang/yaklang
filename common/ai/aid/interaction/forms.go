package interaction

// Validate the embedded form definition as a schema, not merely as JSON data.
// Cross-field constraints prevent rendering forms which cannot be submitted.
func normalizeForm(node ComponentNode, diagnose func(string, string)) bool {
	if node.Component != "ParameterForm" {
		return true
	}
	invalid := func() bool {
		diagnose("props.schema", "invalid or unsatisfiable form schema; component removed")
		return false
	}
	schema, ok := node.Props["schema"].(map[string]any)
	if !ok {
		return invalid()
	}
	if _, err := compileSchema(schema); err != nil {
		return invalid()
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || schema["type"] != "object" {
		return invalid()
	}
	required, _ := schema["required"].([]any)
	for _, key := range required {
		if _, exists := properties[key.(string)]; !exists {
			return invalid()
		}
	}
	for _, value := range properties {
		field, ok := value.(map[string]any)
		if !ok {
			return invalid()
		}
		if field["type"] == "array" && field["items"] == nil {
			return invalid()
		}
		for _, bounds := range [][2]string{{"minimum", "maximum"}, {"minLength", "maxLength"}, {"minItems", "maxItems"}} {
			min, hasMin := field[bounds[0]].(float64)
			max, hasMax := field[bounds[1]].(float64)
			if hasMin && hasMax && min > max {
				return invalid()
			}
		}
	}
	return true
}
