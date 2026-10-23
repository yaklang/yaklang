package interaction

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed question_form.schema.json
var questionFormSchemaJSON []byte

// QuestionForm belongs to the clarification lifecycle, never the display catalog.
type QuestionForm struct {
	Title      string         `json:"title"`
	Schema     map[string]any `json:"schema"`
	FieldOrder []string       `json:"fieldOrder,omitempty"`
	Initial    map[string]any `json:"initial,omitempty"`
}

// FormSchema drives both the model's tool parameters and runtime validation.
func FormSchema() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal(questionFormSchemaJSON, &schema)
	return schema
}

func (f *QuestionForm) Validate() error {
	raw, err := json.Marshal(f)
	if err != nil || len(raw) > 64*1024 {
		return fmt.Errorf("question form must fit within 64 KiB")
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var invalid bool
	normalized, ok := normalize(FormSchema(), value, "form", func(string, string) { invalid = true })
	if !ok || invalid {
		return fmt.Errorf("invalid question form definition")
	}
	raw, _ = json.Marshal(normalized)
	if err := json.Unmarshal(raw, f); err != nil {
		return err
	}
	if _, err := compileSchema(f.Schema); err != nil {
		return fmt.Errorf("invalid form schema: %w", err)
	}
	properties := f.Schema["properties"].(map[string]any)
	required, _ := f.Schema["required"].([]any)
	for _, key := range required {
		if _, exists := properties[key.(string)]; !exists {
			return fmt.Errorf("required form field %s is undefined", key)
		}
	}
	for _, key := range f.FieldOrder {
		if _, exists := properties[key]; !exists {
			return fmt.Errorf("ordered form field %s is undefined", key)
		}
	}
	for key, value := range properties {
		field := value.(map[string]any)
		if field["type"] == "array" && field["items"] == nil {
			return fmt.Errorf("array field %s requires items", key)
		}
		for _, bounds := range [][2]string{{"minimum", "maximum"}, {"minLength", "maxLength"}, {"minItems", "maxItems"}} {
			min, hasMin := field[bounds[0]].(float64)
			max, hasMax := field[bounds[1]].(float64)
			if hasMin && hasMax && min > max {
				return fmt.Errorf("unsatisfiable constraints for field %s", key)
			}
		}
		if value, exists := field["default"]; exists && ValidateSchema(field, value) != nil {
			return fmt.Errorf("invalid default for field %s", key)
		}
	}
	for key, value := range f.Initial {
		field, exists := properties[key].(map[string]any)
		if !exists || ValidateSchema(field, value) != nil {
			return fmt.Errorf("invalid initial value for field %s", key)
		}
	}
	return nil
}

func (f QuestionForm) AnswerText(values map[string]any) (string, error) {
	if values == nil || ValidateSchema(f.Schema, values) != nil {
		return "", fmt.Errorf("form values do not match the declared schema")
	}
	properties := f.Schema["properties"].(map[string]any)
	keys := append([]string{}, f.FieldOrder...)
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
	}
	var remaining []string
	for key := range properties {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	keys = append(keys, remaining...)
	lines := []string{"对追问的回答：" + f.Title}
	for _, key := range keys {
		field := properties[key].(map[string]any)
		label, _ := field["title"].(string)
		if label == "" {
			label = key
		}
		text := "未填写"
		if value, exists := values[key]; exists {
			if stringValue, ok := value.(string); ok {
				text = stringValue
			} else {
				raw, _ := json.Marshal(value)
				text = string(raw)
			}
		}
		lines = append(lines, label+"："+text)
	}
	return strings.Join(lines, "\n"), nil
}
