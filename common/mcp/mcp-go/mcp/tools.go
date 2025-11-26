package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/samber/lo"
)

// ListToolsRequest is sent from the client to request a list of tools the
// server has.
type ListToolsRequest struct {
	PaginatedRequest
}

// ListToolsResult is the server's response to a tools/list request from the
// client.
type ListToolsResult struct {
	PaginatedResult
	Tools []*Tool `json:"tools"`
}

// CallToolResult is the server's response to a tool call.
//
// Any errors that originate from the tool SHOULD be reported inside the result
// object, with `isError` set to true, _not_ as an MCP protocol-level error
// response. Otherwise, the LLM would not be able to see that an error occurred
// and self-correct.
//
// However, any errors in _finding_ the tool, an error indicating that the
// server does not support tool calls, or any other exceptional conditions,
// should be reported as an MCP error response.
type CallToolResult struct {
	Result
	Content []any `json:"content"` // Can be TextContent, ImageContent, or      EmbeddedResource
	// Whether the tool call ended in an error.
	//
	// If not set, this is assumed to be false (the call was successful).
	IsError bool `json:"isError,omitempty"`
}

// CallToolRequest is used by the client to invoke a tool provided by the server.
type CallToolRequest struct {
	Request
	Params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
		Meta      *struct {
			// If specified, the caller is requesting out-of-band progress
			// notifications for this request (as represented by
			// notifications/progress). The value of this parameter is an
			// opaque token that will be attached to any subsequent
			// notifications. The receiver is not obligated to provide these
			// notifications.
			ProgressToken ProgressToken `json:"progressToken,omitempty"`
		} `json:"_meta,omitempty"`
	} `json:"params"`
}

// ToolListChangedNotification is an optional notification from the server to
// the client, informing it that the list of tools it offers has changed. This may
// be issued by servers without any previous subscription from the client.
type ToolListChangedNotification struct {
	Notification
}

// Tool represents the definition for a tool the client can call.
type Tool struct {
	// The name of the tool.
	Name string `json:"name"`
	// A human-readable description of the tool.
	Description string `json:"description,omitempty"`
	// A JSON Schema object defining the expected parameters for the tool.
	InputSchema ToolInputSchema `json:"inputSchema"`

	// DANGER: 这个值永远不应该暴露给用户，只有内部工具才有资格设置它
	// No Need Timeline Recorded
	NoNeedTimelineRecorded bool `json:"-"`

	// DANGER: 这个值永远不应该暴露给用户，只有内部工具才有资格设置它
	// No Need User-Review
	NoNeedUserReview bool `json:"-"`

	// 用于使用YakScript创建 MCP 工具
	YakScript string `json:"yakScript,omitempty"`
}

type ToolInputSchema struct {
	Type       string                        `json:"type"`
	Properties *omap.OrderedMap[string, any] `json:"properties,omitempty"`
	Required   []string                      `json:"required,omitempty"`
}

func (t *ToolInputSchema) MarshalJSON() ([]byte, error) {
	temp := struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties,omitempty"`
		Required   []string       `json:"required,omitempty"`
	}{
		Type:     t.Type,
		Required: t.Required,
	}

	if t.Properties != nil {
		temp.Properties = t.Properties.GetMap()
	}

	return json.Marshal(temp)
}

// ToolOption is a function that configures a Tool.
// It provides a flexible way to set various properties of a Tool using the functional options pattern.
type ToolOption func(*Tool)

// PropertyOption is a function that configures a property in a Tool's input schema.
// It allows for flexible configuration of JSON Schema properties using the functional options pattern.
type PropertyOption func(map[string]any)

//
// Core Tool Functions
//

// NewTool creates a new Tool with the given name and options.
// The tool will have an object-type input schema with configurable properties.
// Options are applied in order, allowing for flexible tool configuration.
func NewTool(name string, opts ...ToolOption) *Tool {
	tool := Tool{
		Name: name,
		InputSchema: ToolInputSchema{
			Type:       "object",
			Properties: omap.NewEmptyOrderedMap[string, any](),
			Required:   nil, // Will be omitted from JSON if empty
		},
	}

	for _, opt := range opts {
		opt(&tool)
	}

	return &tool
}

// WithDescription adds a description to the Tool.
// The description should provide a clear, human-readable explanation of what the tool does.
func WithDescription(description string) ToolOption {
	return func(t *Tool) {
		t.Description = description
	}
}

// WithRequireTool adds a ATTENTION description to the Tool.
// require tool description means prerequisites for running this tool
func WithRequireTool(tool string) ToolOption {
	return func(t *Tool) {
		t.Description += fmt.Sprintf("<ATTENTION> before call this tool, please call %s tool first </ATTENTION>", tool)
	}
}

//
// Common Property Options
//

// Description adds a description to a property in the JSON Schema.
// The description should explain the purpose and expected values of the property.
func Description(desc string) PropertyOption {
	return func(schema map[string]any) {
		schema["description"] = desc
	}
}

// RequireTool adds a ATTENTION description to a property in the JSON Schema.
// require tool description means prerequisites for running this tool
func RequireTool(tool string) PropertyOption {
	return func(schema map[string]any) {
		requireToolMessage := fmt.Sprintf("<ATTENTION> before call this tool, please call %s tool first </ATTENTION>", tool)
		if i, ok := schema["description"]; ok {
			schema["description"] = fmt.Sprintf("%s %s", i, requireToolMessage)
		} else {
			schema["description"] = requireToolMessage
		}
	}
}

// Default sets the default value for a property.
// This value will be used if the property is not explicitly provided.
func Default(desc any) PropertyOption {
	return func(schema map[string]any) {
		schema["default"] = desc
	}
}

// Required marks a property as required in the tool's input schema.
// Required properties must be provided when using the tool.
func Required() PropertyOption {
	return func(schema map[string]any) {
		schema["required"] = true
	}
}

// Title adds a display-friendly title to a property in the JSON Schema.
// This title can be used by UI components to show a more readable property name.
func Title(title string) PropertyOption {
	return func(schema map[string]any) {
		schema["title"] = title
	}
}

//
// String Property Options
//

// Enum specifies a list of allowed values for a string property.
// The property value must be one of the specified enum values.
func Enum(values ...any) PropertyOption {
	return func(schema map[string]any) {
		schema["enum"] = values
	}
}

func EnumString(values ...string) PropertyOption {
	return func(schema map[string]any) {
		schema["enum"] = lo.Map(values, func(item string, _ int) any { return item })
	}
}

// MaxLength sets the maximum length for a string property.
// The string value must not exceed this length.
func MaxLength(max int) PropertyOption {
	return func(schema map[string]any) {
		schema["maxLength"] = max
	}
}

// MinLength sets the minimum length for a string property.
// The string value must be at least this length.
func MinLength(min int) PropertyOption {
	return func(schema map[string]any) {
		schema["minLength"] = min
	}
}

// Pattern sets a regex pattern that a string property must match.
// The string value must conform to the specified regular expression.
func Pattern(pattern string) PropertyOption {
	return func(schema map[string]any) {
		schema["pattern"] = pattern
	}
}

//
// Number Property Options
//

// Max sets the maximum value for a number property.
// The number value must not exceed this maximum.
func Max(max float64) PropertyOption {
	return func(schema map[string]any) {
		schema["maximum"] = max
	}
}

// Min sets the minimum value for a number property.
// The number value must not be less than this minimum.
func Min(min float64) PropertyOption {
	return func(schema map[string]any) {
		schema["minimum"] = min
	}
}

// MultipleOf specifies that a number must be a multiple of the given value.
// The number value must be divisible by this value.
func MultipleOf(value float64) PropertyOption {
	return func(schema map[string]any) {
		schema["multipleOf"] = value
	}
}

//
// Property Type Helpers
//

// WithBool adds a boolean property to the tool schema.
// It accepts property options to configure the boolean property's behavior and constraints.
func WithBool(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "boolean",
	}
	return WithRaw(name, schema, opts...)
}

// WithInteger adds a integer property to the tool schema.
// It accepts property options to configure the integer property's behavior and constraints.
func WithInteger(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "integer",
	}
	return WithRaw(name, schema, opts...)
}

// WithNumber adds a number property to the tool schema.
// It accepts property options to configure the number property's behavior and constraints.
func WithNumber(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "number",
	}
	return WithRaw(name, schema, opts...)
}

// WithString adds a string property to the tool schema.
// It accepts property options to configure the string property's behavior and constraints.
func WithString(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "string",
	}
	return WithRaw(name, schema, opts...)
}

// WithStringArray adds a string array property to the tool schema.
// It accepts property options to configure the string-array property's behavior and constraints.
func WithStringArray(name string, opts ...PropertyOption) ToolOption {
	return WithSimpleArray(name, "string", opts...)
}

// WithNumberArray adds a number array property to the tool schema.
// It accepts property options to configure the number-array property's behavior and constraints.
func WithNumberArray(name string, opts ...PropertyOption) ToolOption {
	return WithSimpleArray(name, "number", opts...)
}

func WithSimpleArray(name string, itemType string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": itemType,
		},
	}
	return WithRaw(name, schema, opts...)
}

func WithStructArray(name string, opts []PropertyOption, itemsOpt ...ToolOption) ToolOption {
	items := map[string]any{
		"type": "object",
	}
	schema := map[string]any{
		"type":  "array",
		"items": items,
	}
	temp := NewTool("", itemsOpt...)
	if temp.InputSchema.Properties != nil {
		temp.InputSchema.Properties.ForEach(func(k string, v any) bool {
			items[k] = v
			return true
		})
	}
	items["required"] = temp.InputSchema.Required
	return WithRaw(name, schema, opts...)
}

func WithStruct(name string, opts []PropertyOption, itemsOpt ...ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	temp := NewTool("", itemsOpt...)
	schema["properties"] = temp.InputSchema.Properties
	schema["required"] = temp.InputSchema.Required
	return WithRaw(name, schema, opts...)
}

// WithOneOfStruct
func WithOneOfStruct(name string, opts []PropertyOption, itemsOpt ...[]ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	oneOfArray := make([]any, 0, len(itemsOpt))
	for _, itemOpt := range itemsOpt {
		temp := NewTool("", itemOpt...)
		m := map[string]any{
			"properties": temp.InputSchema.Properties,
			"required":   temp.InputSchema.Required,
		}
		oneOfArray = append(oneOfArray, m)
	}
	schema["oneOf"] = oneOfArray
	return WithRaw(name, schema, opts...)
}

// WithAnyOfStruct
func WithAnyOfStruct(name string, opts []PropertyOption, itemsOpt ...[]ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	anyOfArray := make([]any, 0, len(itemsOpt))
	for _, itemOpt := range itemsOpt {
		temp := NewTool("", itemOpt...)
		m := map[string]any{
			"properties": temp.InputSchema.Properties,
			"required":   temp.InputSchema.Required,
		}
		anyOfArray = append(anyOfArray, m)
	}
	schema["anyOf"] = anyOfArray
	return WithRaw(name, schema, opts...)
}

func WithPaging(name string, fieldNames []string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"page": map[string]any{
				"type": "number",
			},
			"limit": map[string]any{
				"type": "number",
			},
			"order": map[string]any{
				"type": "string",
				"enum": fieldNames,
			},
			"orderby": map[string]any{
				"type": "string",
			},
		},
	}
	return WithRaw(name, schema, opts...)
}

func WithKVPairs(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type": "string",
				},
				"value": map[string]any{
					"type": "string",
				},
			},
		},
	}
	return WithRaw(name, schema, opts...)
}

// WithRaw adds a custom object property to the tool schema.
// It accepts property options to configure the object property's behavior and constraints.
func WithRaw(name string, object map[string]any, opts ...PropertyOption) ToolOption {
	return func(t *Tool) {
		for _, opt := range opts {
			opt(object)
		}

		// Remove required from property schema and add to InputSchema.required
		if required, ok := object["required"].(bool); ok && required {
			delete(object, "required")
			if t.InputSchema.Required == nil {
				t.InputSchema.Required = []string{name}
			} else {
				t.InputSchema.Required = append(t.InputSchema.Required, name)
			}
		}

		t.InputSchema.Properties.Set(name, object)
	}
}

// ToMap converts the ToolInputSchema to a map[string]any.
// Note: This method preserves order using OrderedMap for the result
func (s *ToolInputSchema) ToMap() *omap.OrderedMap[string, any] {
	result := omap.NewEmptyOrderedMap[string, any]()
	result.Set("type", s.Type)

	if s.Properties != nil && s.Properties.Len() > 0 {
		// Create an ordered copy of properties with required field processing
		orderedProps := omap.NewEmptyOrderedMap[string, any]()
		s.Properties.ForEach(func(k string, v any) bool {
			m := utils.InterfaceToGeneralMap(v)
			if _, ok := m["required"]; ok {
				delete(m, "required")
				orderedProps.Set(k, m)
			} else {
				orderedProps.Set(k, v)
			}
			return true
		})
		result.Set("properties", orderedProps)
	}

	if len(s.Required) > 0 {
		required := lo.Map(s.Required, func(item string, _ int) any { return item })
		result.Set("required", required)
	}

	return result
}

func (s *ToolInputSchema) FromMap(m map[string]any) error {
	// type
	typeStr, ok := m["type"].(string)
	if !ok {
		return fmt.Errorf("type is not a string")
	}
	s.Type = typeStr

	// properties
	properties, ok := m["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("properties is not a map[string]any")
	}
	// Convert regular map to OrderedMap
	s.Properties = omap.NewEmptyOrderedMap[string, any]()
	for k, v := range properties {
		s.Properties.Set(k, v)
	}

	// required
	if v, ok := m["required"]; ok {
		s.Required = utils.InterfaceToStringSlice(v)
	}
	return nil
}
