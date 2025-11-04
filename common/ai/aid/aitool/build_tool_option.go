package aitool

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// BuildParamsOptions converts a Tool's InputSchema Properties back into ToolOptions
// This is the reverse operation of WithXxxParam functions
func (t *Tool) BuildParamsOptions() []ToolOption {
	if t == nil || t.Tool == nil || t.InputSchema.Properties == nil {
		return []ToolOption{}
	}

	var options []ToolOption
	requiredMap := make(map[string]bool)

	// Build a map of required parameters for quick lookup
	for _, req := range t.InputSchema.Required {
		requiredMap[req] = true
	}

	// Iterate through properties in order
	t.InputSchema.Properties.ForEach(func(name string, propRaw any) bool {
		prop := utils.InterfaceToGeneralMap(propRaw)
		if len(prop) == 0 {
			log.Warnf("property '%s' is not a valid map, skipping", name)
			return true // continue
		}

		// Build PropertyOptions for this parameter
		var propOpts []PropertyOption

		// Extract common properties using utils.MapGetString
		if desc := utils.MapGetString(prop, "description"); desc != "" {
			propOpts = append(propOpts, WithParam_Description(desc))
		}

		if title := utils.MapGetString(prop, "title"); title != "" {
			propOpts = append(propOpts, WithParam_Title(title))
		}

		// Use MapGetRaw for default and example as they can be any type
		if defaultVal := utils.MapGetRaw(prop, "default"); !utils.IsNil(defaultVal) {
			propOpts = append(propOpts, WithParam_Default(defaultVal))
		}

		if example := utils.MapGetRaw(prop, "example"); !utils.IsNil(example) {
			propOpts = append(propOpts, WithParam_Example(example))
		}

		// Check if required
		if requiredMap[name] {
			propOpts = append(propOpts, WithParam_Required(true))
		}

		// Determine type and create appropriate ToolOption
		typeVal := utils.MapGetString(prop, "type")
		if typeVal == "" {
			log.Warnf("property '%s' has no type field, skipping", name)
			return true // continue
		}

		var toolOpt ToolOption

		switch typeVal {
		case "string":
			// Check for enum
			if enumRaw := utils.MapGetRaw(prop, "enum"); !utils.IsNil(enumRaw) {
				if enumSlice, ok := enumRaw.([]any); ok {
					propOpts = append(propOpts, WithParam_Enum(enumSlice...))
				}
			}

			// Check for const
			if constVal := utils.MapGetRaw(prop, "const"); !utils.IsNil(constVal) {
				propOpts = append(propOpts, WithParam_Const(constVal))
			}

			// Check string constraints
			if maxLength := utils.MapGetInt(prop, "maxLength"); maxLength > 0 {
				propOpts = append(propOpts, WithParam_MaxLength(maxLength))
			}
			if minLength := utils.MapGetInt(prop, "minLength"); minLength > 0 {
				propOpts = append(propOpts, WithParam_MinLength(minLength))
			}
			if pattern := utils.MapGetString(prop, "pattern"); pattern != "" {
				propOpts = append(propOpts, WithParam_Pattern(pattern))
			}

			toolOpt = WithStringParam(name, propOpts...)

		case "integer":
			// Check number constraints using MapGetFloat64
			if max := utils.MapGetFloat64(prop, "maximum"); max != 0 {
				propOpts = append(propOpts, WithParam_Max(max))
			}
			if min := utils.MapGetFloat64(prop, "minimum"); min != 0 {
				propOpts = append(propOpts, WithParam_Min(min))
			}
			if multipleOf := utils.MapGetFloat64(prop, "multipleOf"); multipleOf != 0 {
				propOpts = append(propOpts, WithParam_MultipleOf(multipleOf))
			}

			toolOpt = WithIntegerParam(name, propOpts...)

		case "number":
			// Check number constraints using MapGetFloat64
			if max := utils.MapGetFloat64(prop, "maximum"); max != 0 {
				propOpts = append(propOpts, WithParam_Max(max))
			}
			if min := utils.MapGetFloat64(prop, "minimum"); min != 0 {
				propOpts = append(propOpts, WithParam_Min(min))
			}
			if multipleOf := utils.MapGetFloat64(prop, "multipleOf"); multipleOf != 0 {
				propOpts = append(propOpts, WithParam_MultipleOf(multipleOf))
			}

			toolOpt = WithNumberParam(name, propOpts...)

		case "boolean":
			toolOpt = WithBoolParam(name, propOpts...)

		case "array":
			toolOpt = buildArrayParam(name, prop, propOpts)

		case "object":
			toolOpt = buildStructParam(name, prop, propOpts)

		case "null":
			toolOpt = WithNullParam(name, propOpts...)

		default:
			log.Warnf("unknown type '%s' for property '%s', using raw param", typeVal, name)
			toolOpt = WithRawParam(name, prop, propOpts...)
		}

		if toolOpt != nil {
			options = append(options, toolOpt)
		}

		return true // continue iteration
	})

	return options
}

// buildArrayParam builds an array parameter from property schema
func buildArrayParam(name string, prop map[string]any, propOpts []PropertyOption) ToolOption {
	itemsRaw := utils.MapGetRaw(prop, "items")
	if utils.IsNil(itemsRaw) {
		log.Warnf("array property '%s' has no items field", name)
		return WithRawParam(name, prop, propOpts...)
	}

	itemsMap := utils.InterfaceToGeneralMap(itemsRaw)
	if len(itemsMap) == 0 {
		log.Warnf("array property '%s' items is not a valid map", name)
		return WithRawParam(name, prop, propOpts...)
	}

	itemType := utils.MapGetString(itemsMap, "type")
	if itemType == "" {
		log.Warnf("array property '%s' items has no type", name)
		return WithRawParam(name, prop, propOpts...)
	}

	// Build item options
	var itemOpts []PropertyOption
	if desc := utils.MapGetString(itemsMap, "description"); desc != "" {
		itemOpts = append(itemOpts, WithParam_Description(desc))
	}
	if defaultVal := utils.MapGetRaw(itemsMap, "default"); !utils.IsNil(defaultVal) {
		itemOpts = append(itemOpts, WithParam_Default(defaultVal))
	}

	switch itemType {
	case "string":
		// Check for enum on items
		if enumRaw := utils.MapGetRaw(itemsMap, "enum"); !utils.IsNil(enumRaw) {
			if enumSlice, ok := enumRaw.([]any); ok {
				itemOpts = append(itemOpts, WithParam_Enum(enumSlice...))
			}
		}
		return WithStringArrayParamEx(name, propOpts, itemOpts...)

	case "number":
		return WithArrayParam(name, "number", propOpts, itemOpts...)

	case "integer":
		return WithArrayParam(name, "integer", propOpts, itemOpts...)

	case "object":
		// Struct array
		structOpts := buildStructOptionsFromMap(itemsMap)
		return WithStructArrayParam(name, propOpts, nil, structOpts...)

	case "array":
		// Nested array
		nestedOpt := buildArrayParam("", itemsMap, itemOpts)
		return WithArrayParamEx(name, propOpts, nestedOpt)

	default:
		log.Warnf("unknown array item type '%s' for property '%s'", itemType, name)
		return WithRawParam(name, prop, propOpts...)
	}
}

// buildStructParam builds a struct/object parameter from property schema
func buildStructParam(name string, prop map[string]any, propOpts []PropertyOption) ToolOption {
	structOpts := buildStructOptionsFromMap(prop)
	return WithStructParam(name, propOpts, structOpts...)
}

// buildStructOptionsFromMap builds ToolOptions from an object schema
func buildStructOptionsFromMap(objMap map[string]any) []ToolOption {
	propertiesRaw := utils.MapGetRaw(objMap, "properties")
	if utils.IsNil(propertiesRaw) {
		return []ToolOption{}
	}

	// Properties can be either a map or an OrderedMap
	var structOpts []ToolOption
	requiredMap := make(map[string]bool)

	// Build required map using utils.MapGetStringSlice
	requiredSlice := utils.MapGetStringSlice(objMap, "required")
	for _, req := range requiredSlice {
		requiredMap[req] = true
	}

	// Check if properties is an OrderedMap
	if oMap, ok := propertiesRaw.(*omap.OrderedMap[string, any]); ok {
		// Handle OrderedMap directly
		oMap.ForEach(func(propName string, propValRaw any) bool {
			propVal := utils.InterfaceToGeneralMap(propValRaw)
			if len(propVal) == 0 {
				return true // continue
			}

			structOpt := buildToolOptionFromProperty(propName, propVal, requiredMap[propName])
			if structOpt != nil {
				structOpts = append(structOpts, structOpt)
			}
			return true // continue
		})
	} else {
		// Handle properties as a regular map
		propsMap := utils.InterfaceToGeneralMap(propertiesRaw)
		for propName, propValRaw := range propsMap {
			propVal := utils.InterfaceToGeneralMap(propValRaw)
			if len(propVal) == 0 {
				continue
			}

			structOpt := buildToolOptionFromProperty(propName, propVal, requiredMap[propName])
			if structOpt != nil {
				structOpts = append(structOpts, structOpt)
			}
		}
	}

	return structOpts
}

// buildToolOptionFromProperty builds a single ToolOption from a property definition
func buildToolOptionFromProperty(name string, prop map[string]any, isRequired bool) ToolOption {
	var propOpts []PropertyOption

	// Extract common properties using utils.MapGetString
	if desc := utils.MapGetString(prop, "description"); desc != "" {
		propOpts = append(propOpts, WithParam_Description(desc))
	}

	if title := utils.MapGetString(prop, "title"); title != "" {
		propOpts = append(propOpts, WithParam_Title(title))
	}

	if defaultVal := utils.MapGetRaw(prop, "default"); !utils.IsNil(defaultVal) {
		propOpts = append(propOpts, WithParam_Default(defaultVal))
	}

	if example := utils.MapGetRaw(prop, "example"); !utils.IsNil(example) {
		propOpts = append(propOpts, WithParam_Example(example))
	}

	if isRequired {
		propOpts = append(propOpts, WithParam_Required(true))
	}

	// Determine type
	typeVal := utils.MapGetString(prop, "type")
	if typeVal == "" {
		return WithRawParam(name, prop, propOpts...)
	}

	switch typeVal {
	case "string":
		// Check for enum
		if enumRaw := utils.MapGetRaw(prop, "enum"); !utils.IsNil(enumRaw) {
			if enumSlice, ok := enumRaw.([]any); ok {
				propOpts = append(propOpts, WithParam_Enum(enumSlice...))
			}
		}
		// Check for const
		if constVal := utils.MapGetRaw(prop, "const"); !utils.IsNil(constVal) {
			propOpts = append(propOpts, WithParam_Const(constVal))
		}
		// String constraints
		if maxLength := utils.MapGetInt(prop, "maxLength"); maxLength > 0 {
			propOpts = append(propOpts, WithParam_MaxLength(maxLength))
		}
		if minLength := utils.MapGetInt(prop, "minLength"); minLength > 0 {
			propOpts = append(propOpts, WithParam_MinLength(minLength))
		}
		if pattern := utils.MapGetString(prop, "pattern"); pattern != "" {
			propOpts = append(propOpts, WithParam_Pattern(pattern))
		}
		return WithStringParam(name, propOpts...)

	case "integer":
		// Number constraints using MapGetFloat64
		if max := utils.MapGetFloat64(prop, "maximum"); max != 0 {
			propOpts = append(propOpts, WithParam_Max(max))
		}
		if min := utils.MapGetFloat64(prop, "minimum"); min != 0 {
			propOpts = append(propOpts, WithParam_Min(min))
		}
		if multipleOf := utils.MapGetFloat64(prop, "multipleOf"); multipleOf != 0 {
			propOpts = append(propOpts, WithParam_MultipleOf(multipleOf))
		}
		return WithIntegerParam(name, propOpts...)

	case "number":
		// Number constraints using MapGetFloat64
		if max := utils.MapGetFloat64(prop, "maximum"); max != 0 {
			propOpts = append(propOpts, WithParam_Max(max))
		}
		if min := utils.MapGetFloat64(prop, "minimum"); min != 0 {
			propOpts = append(propOpts, WithParam_Min(min))
		}
		if multipleOf := utils.MapGetFloat64(prop, "multipleOf"); multipleOf != 0 {
			propOpts = append(propOpts, WithParam_MultipleOf(multipleOf))
		}
		return WithNumberParam(name, propOpts...)

	case "boolean":
		return WithBoolParam(name, propOpts...)

	case "array":
		return buildArrayParam(name, prop, propOpts)

	case "object":
		return buildStructParam(name, prop, propOpts)

	case "null":
		return WithNullParam(name, propOpts...)

	default:
		return WithRawParam(name, prop, propOpts...)
	}
}

// RebuildTool creates a new Tool with the same configuration as the original
// This is useful for testing round-trip conversion
func (t *Tool) RebuildTool() (*Tool, error) {
	if t == nil {
		return nil, utils.Errorf("cannot rebuild nil tool")
	}

	// Get base options
	opts := []ToolOption{
		WithDescription(t.Description),
	}

	// Add keywords if present
	if len(t.Keywords) > 0 {
		opts = append(opts, WithKeywords(t.Keywords))
	}

	// Add callback if present
	if t.Callback != nil {
		opts = append(opts, WithCallback(t.Callback))
	}

	// Add parameter options
	paramOpts := t.BuildParamsOptions()
	opts = append(opts, paramOpts...)

	// Create new tool
	if t.Callback != nil {
		return New(t.Name, opts...)
	}
	return NewWithoutCallback(t.Name, opts...), nil
}

// CompareTools compares two tools and returns a list of differences
// This is useful for debugging round-trip conversion issues
func CompareTools(t1, t2 *Tool) []string {
	var diffs []string

	if t1 == nil && t2 == nil {
		return diffs
	}

	if t1 == nil {
		diffs = append(diffs, "first tool is nil")
		return diffs
	}

	if t2 == nil {
		diffs = append(diffs, "second tool is nil")
		return diffs
	}

	// Compare basic properties
	if t1.Name != t2.Name {
		diffs = append(diffs, fmt.Sprintf("name mismatch: '%s' vs '%s'", t1.Name, t2.Name))
	}

	if t1.Description != t2.Description {
		diffs = append(diffs, fmt.Sprintf("description mismatch: '%s' vs '%s'", t1.Description, t2.Description))
	}

	// Compare parameters count
	p1 := t1.Params()
	p2 := t2.Params()

	if p1.Len() != p2.Len() {
		diffs = append(diffs, fmt.Sprintf("params count mismatch: %d vs %d", p1.Len(), p2.Len()))
	}

	// Compare required fields
	if len(t1.InputSchema.Required) != len(t2.InputSchema.Required) {
		diffs = append(diffs, fmt.Sprintf("required count mismatch: %d vs %d",
			len(t1.InputSchema.Required), len(t2.InputSchema.Required)))
	}

	// Compare each parameter
	p1.ForEach(func(k string, v1 any) bool {
		v2, ok := p2.Get(k)
		if !ok {
			diffs = append(diffs, fmt.Sprintf("parameter '%s' missing in second tool", k))
			return true
		}

		// Deep comparison would go here
		// For now, just check existence
		_ = v1
		_ = v2

		return true
	})

	return diffs
}
