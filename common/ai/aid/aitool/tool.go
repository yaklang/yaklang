package aitool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	// "github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

type ToolRuntimeConfig struct {
	FeedBacker func(result *ypb.ExecResult) error
	RuntimeID  string
}

// NoRuntimeInvokeCallback 定义工具调用回调函数的签名
type NoRuntimeInvokeCallback func(ctx context.Context, params InvokeParams, stdout io.Writer, stderr io.Writer) (any, error)
type InvokeCallback func(ctx context.Context, params InvokeParams, invokeExConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error)
type Tool struct {
	*mcp.Tool
	// A list of keywords for tool indexing and searching.
	Keywords []string       `json:"keywords,omitempty"`
	Callback InvokeCallback // 添加回调函数字段
}

// ToolOption 定义工具选项函数的类型
type ToolOption func(*Tool)

// PropertyOption 定义属性选项函数的类型
type PropertyOption func(map[string]any)

// New 使用函数选项模式创建一个新的Tool实例
func New(name string, options ...ToolOption) (*Tool, error) {
	tool := newTool(name, options...)

	// 检查是否设置了回调函数
	if tool.Callback == nil {
		return nil, errors.New("WithSimpleCallback is needed, normal ai.Tool should have callback anyway")
	}

	return tool, nil
}

func NewWithoutCallback(name string, opts ...ToolOption) *Tool {
	tool := newTool(name, opts...)
	return tool
}

func NewFromMCPTool(mt *mcp.Tool, opts ...ToolOption) (*Tool, error) {
	tool := &Tool{Tool: mt}
	for _, opt := range opts {
		opt(tool)
	}
	return tool, nil
}

func newTool(name string, options ...ToolOption) *Tool {
	tool := &Tool{
		Tool: mcp.NewTool(name),
	}

	// 应用所有选项
	for _, option := range options {
		option(tool)
	}

	return tool
}

// WithDescription 设置工具的描述信息
func WithDescription(description string) ToolOption {
	return func(t *Tool) {
		t.Description = description
	}
}

// WithDangerousNoNeedTimelineRecorded 设置工具是否需要时间线记录
func WithDangerousNoNeedTimelineRecorded(i bool) ToolOption {
	return func(t *Tool) {
		t.NoNeedTimelineRecorded = i
	}
}

// WithDangerousNoNeedUserReview 设置工具是否不需要用户审核
func WithDangerousNoNeedUserReview(i bool) ToolOption {
	return func(t *Tool) {
		t.NoNeedUserReview = i
	}
}

// WithKeywords 设置工具索引关键词
func WithKeywords(keywords []string) ToolOption {
	return func(t *Tool) {
		t.Keywords = keywords
	}
}

// WithSimpleCallback 设置工具的回调函数
func WithSimpleCallback(callback func(params InvokeParams, stdout io.Writer, stderr io.Writer) (any, error)) ToolOption {
	return func(t *Tool) {
		t.Callback = func(ctx context.Context, params InvokeParams, runtimeConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if callback == nil {
				return nil, errors.New("callback function is nil")
			}
			return callback(params, stdout, stderr)
		}
	}
}

func WithNoRuntimeCallback(callback NoRuntimeInvokeCallback) ToolOption {
	return func(t *Tool) {
		t.Callback = func(ctx context.Context, params InvokeParams, runtimeConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			if callback == nil {
				return nil, errors.New("callback function is nil")
			}
			return callback(ctx, params, stdout, stderr)
		}
	}
}

func WithCallback(callback InvokeCallback) ToolOption {
	return func(t *Tool) {
		t.Callback = callback
	}
}

// WithParam_Description adds a description to a property in the JSON Schema.
// The description should explain the purpose and expected values of the property.
func WithParam_Description(desc string) PropertyOption {
	return func(schema map[string]any) {
		schema["description"] = desc
	}
}

// WithParam_RequireTool adds a ATTENTION description to a property in the JSON Schema.
// require tool description means prerequisites for running this tool
func WithParam_RequireTool(tool string) PropertyOption {
	return func(schema map[string]any) {
		requireToolMessage := fmt.Sprintf("<ATTENTION> before call this tool, please call %s tool first </ATTENTION>", tool)
		if i, ok := schema["description"]; ok {
			schema["description"] = fmt.Sprintf("%s %s", i, requireToolMessage)
		} else {
			schema["description"] = requireToolMessage
		}
	}
}

// WithParam_Default sets the default value for a property.
// This value will be used if the property is not explicitly provided.
func WithParam_Default(desc any) PropertyOption {
	return func(schema map[string]any) {
		schema["default"] = desc
	}
}

// WithParam_Required marks a property as required in the tool's input schema.
// WithParam_Required properties must be provided when using the tool.
func WithParam_Required(required ...bool) PropertyOption {
	return func(schema map[string]any) {
		if len(required) > 0 {
			schema["required"] = required[0]
		} else {
			schema["required"] = true
		}
	}
}

// WithParam_Title adds a display-friendly title to a property in the JSON Schema.
// This title can be used by UI components to show a more readable property name.
func WithParam_Title(title string) PropertyOption {
	return func(schema map[string]any) {
		schema["title"] = title
	}
}

// WithParam_Example adds an example value for a property in the JSON Schema.
func WithParam_Example(i any) PropertyOption {
	return func(schema map[string]any) {
		schema["example"] = i
	}
}

// WithParam_Raw adds a raw JSON schema object to the tool's input schema.
func WithParam_Raw(name string, v any) PropertyOption {
	return func(m map[string]any) {
		m[name] = v
	}
}

//
// String Property Options
//

// WithParam_Enum specifies a list of allowed values for a string property.
// The property value must be one of the specified enum values.
func WithParam_Enum(values ...any) PropertyOption {
	return func(schema map[string]any) {
		schema["enum"] = values
	}
}

// WithParam_Enum specifies a list of allowed values for a string property.
// The property value must be one of the specified enum values.
func WithParam_Const(values ...any) PropertyOption {
	return func(schema map[string]any) {
		schema["const"] = values
	}
}

func WithParam_EnumString(values ...string) PropertyOption {
	return func(schema map[string]any) {
		schema["enum"] = lo.Map(values, func(item string, _ int) any { return item })
	}
}

// WithParam_MaxLength sets the maximum length for a string property.
// The string value must not exceed this length.
func WithParam_MaxLength(max int) PropertyOption {
	return func(schema map[string]any) {
		schema["maxLength"] = max
	}
}

// WithParam_MinLength sets the minimum length for a string property.
// The string value must be at least this length.
func WithParam_MinLength(min int) PropertyOption {
	return func(schema map[string]any) {
		schema["minLength"] = min
	}
}

// WithParam_Pattern sets a regex pattern that a string property must match.
// The string value must conform to the specified regular expression.
func WithParam_Pattern(pattern string) PropertyOption {
	return func(schema map[string]any) {
		schema["pattern"] = pattern
	}
}

//
// Number Property Options
//

// WithParam_Max sets the maximum value for a number property.
// The number value must not exceed this maximum.
func WithParam_Max(max float64) PropertyOption {
	return func(schema map[string]any) {
		schema["maximum"] = max
	}
}

// WithParam_Min sets the minimum value for a number property.
// The number value must not be less than this minimum.
func WithParam_Min(min float64) PropertyOption {
	return func(schema map[string]any) {
		schema["minimum"] = min
	}
}

// WithParam_MultipleOf specifies that a number must be a multiple of the given value.
// The number value must be divisible by this value.
func WithParam_MultipleOf(value float64) PropertyOption {
	return func(schema map[string]any) {
		schema["multipleOf"] = value
	}
}

//
// Property Type Helpers
//

// WithBoolParam adds a boolean property to the tool schema.
// It accepts property options to configure the boolean property's behavior and constraints.
func WithBoolParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "boolean",
	}
	return WithRawParam(name, schema, opts...)
}

// WithIntegerParam adds a integer property to the tool schema.
// It accepts property options to configure the integer property's behavior and constraints.
func WithIntegerParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "integer",
	}
	return WithRawParam(name, schema, opts...)
}

// WithNumberParam adds a number property to the tool schema.
// It accepts property options to configure the number property's behavior and constraints.
func WithNumberParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "number",
	}
	return WithRawParam(name, schema, opts...)
}

// WithStringParam adds a string property to the tool schema.
// It accepts property options to configure the string property's behavior and constraints.
func WithStringParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "string",
	}
	return WithRawParam(name, schema, opts...)
}

// WithStringArrayParam adds a string array property to the tool schema.
// It accepts property options to configure the string-array property's behavior and constraints.
func WithStringArrayParam(name string, opts ...PropertyOption) ToolOption {
	return WithSimpleArrayParam(name, "string", opts...)
}

func WithStringArrayParamEx(name string, opts []PropertyOption, itemsOpt ...PropertyOption) ToolOption {
	return WithArrayParam(name, "string", opts, itemsOpt...)
}

// WithNumberArrayParam adds a number array property to the tool schema.
// It accepts property options to configure the number-array property's behavior and constraints.
func WithNumberArrayParam(name string, opts ...PropertyOption) ToolOption {
	return WithSimpleArrayParam(name, "number", opts...)
}

func WithSimpleArrayParam(name string, itemType string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": itemType,
		},
	}
	return WithRawParam(name, schema, opts...)
}

func WithStructArrayParam(name string, opts []PropertyOption, structOpts []PropertyOption, structMembers ...ToolOption) ToolOption {
	return WithArrayParamEx(
		name, opts,
		WithStructParam("", structOpts, structMembers...),
	)
}

func WithArrayParam(name string, itemType string, opts []PropertyOption, itemsOpt ...PropertyOption) ToolOption {
	itemMap := map[string]any{
		"type": itemType,
	}
	m := WithRawParam(name, itemMap, itemsOpt...)
	return WithArrayParamEx(name, opts, m)
}

func WithArrayParamEx(name string, opts []PropertyOption, itemsOpt ToolOption) ToolOption {
	schema := map[string]any{
		"type": "array",
	}
	temp := newTool("", itemsOpt)
	if temp.InputSchema.Properties != nil {
		temp.InputSchema.Properties.ForEach(func(k string, v any) bool {
			schema["items"] = v
			return false // 只取第一个值
		})
	}
	if len(temp.InputSchema.Required) > 0 {
		schema["required"] = temp.InputSchema.Required

	}
	return WithRawParam(name, schema, opts...)
}

func WithStructParam(name string, opts []PropertyOption, itemsOpt ...ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	temp := newTool("", itemsOpt...)
	if temp.InputSchema.Properties != nil && temp.InputSchema.Properties.Len() > 0 {
		schema["properties"] = temp.InputSchema.Properties
	}
	if len(temp.InputSchema.Required) > 0 {
		schema["required"] = temp.InputSchema.Required
	}
	return WithRawParam(name, schema, opts...)
}

func WithNullParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "null",
	}
	return WithRawParam(name, schema, opts...)
}

// WithOneOfStructParam
func WithOneOfStructParam(name string, opts []PropertyOption, itemsOpt ...[]ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	oneOfArray := make([]any, 0, len(itemsOpt))
	for _, itemOpt := range itemsOpt {
		temp := newTool("", itemOpt...)
		m := map[string]any{
			"properties": temp.InputSchema.Properties,
			"required":   temp.InputSchema.Required,
		}
		oneOfArray = append(oneOfArray, m)
	}
	schema["oneOf"] = oneOfArray
	return WithRawParam(name, schema, opts...)
}

// WithAnyOfStructParam
func WithAnyOfStructParam(name string, opts []PropertyOption, itemsOpt ...[]ToolOption) ToolOption {
	schema := map[string]any{
		"type": "object",
	}
	anyOfArray := make([]any, 0, len(itemsOpt))
	for _, itemOpt := range itemsOpt {
		temp := newTool("", itemOpt...)
		m := map[string]any{
			"properties": temp.InputSchema.Properties,
			"required":   temp.InputSchema.Required,
		}
		anyOfArray = append(anyOfArray, m)
	}
	schema["anyOf"] = anyOfArray
	return WithRawParam(name, schema, opts...)
}

func WithPagingParam(name string, fieldNames []string, opts ...PropertyOption) ToolOption {
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
	return WithRawParam(name, schema, opts...)
}

func WithKVPairsParam(name string, opts ...PropertyOption) ToolOption {
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
	return WithRawParam(name, schema, opts...)
}

// WithRawParam adds a custom object property to the tool schema.
// It accepts property options to configure the object property's behavior and constraints.
func WithRawParam(name string, object map[string]any, opts ...PropertyOption) ToolOption {
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

func (t *Tool) GetName() string {
	return t.Name
}

func (t *Tool) GetDescription() string {
	return t.Description
}

func (t *Tool) GetKeywords() []string {
	return t.Keywords
}

func (t *Tool) Params() *omap.OrderedMap[string, any] {
	if t.Tool.InputSchema.Properties == nil {
		return omap.NewEmptyOrderedMap[string, any]()
	}
	// Return a copy of the OrderedMap to preserve order
	result := omap.NewEmptyOrderedMap[string, any]()
	t.Tool.InputSchema.Properties.ForEach(func(k string, v any) bool {
		result.Set(k, v)
		return true
	})
	return result
}

func (t *Tool) ParamsJsonSchemaString() string {
	schema := t.Params()

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(jsonBytes)
}

// ToJSONSchema 将整个Tool转换为符合JSON Schema Draft-07规范的格式
func (t *Tool) ToJSONSchema() *omap.OrderedMap[string, any] {
	// 构建工具参数的properties - 使用 OrderedMap 保证顺序
	properties := omap.NewEmptyOrderedMap[string, any]()

	// 添加@action字段 (第一个)
	properties.Set("@action", map[string]any{
		"const":       "call-tool",
		"description": "标识当前操作的具体类型",
	})

	// 添加tool字段 (第二个)
	properties.Set("tool", map[string]any{
		"type":        "string",
		"description": "你想要选择的工具名",
		"const":       t.Name,
	})

	paramProperties := t.Tool.InputSchema.Properties
	// 将参数添加到params字段 (第三个)
	if paramProperties != nil && paramProperties.Len() > 0 {
		properties.Set("params", map[string]any{
			"type":        "object",
			"description": "工具的参数",
			"properties":  paramProperties,
			"required":    t.InputSchema.Required,
		})
	}

	// 构建最终的JSON Schema - 使用 OrderedMap 保证顺序
	schema := omap.NewEmptyOrderedMap[string, any]()

	// 按照固定顺序添加字段
	schema.Set("$schema", "http://json-schema.org/draft-07/schema#")
	schema.Set("type", "object")

	if t.Description != "" {
		schema.Set("description", t.Description)
	}

	schema.Set("properties", properties)
	schema.Set("required", []string{"tool", "@action", "params"})
	schema.Set("additionalProperties", false)

	return schema
}

// ToJSONSchemaString 将Tool转换为JSON Schema字符串
func (t *Tool) ToJSONSchemaString() string {
	schema := t.ToJSONSchema()

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(jsonBytes)
}

type ToolFactory struct {
	tools []*Tool
}

func NewFactory() *ToolFactory {
	return &ToolFactory{}
}

func (f *ToolFactory) Tools() []*Tool {
	tools := make([]*Tool, 0, len(f.tools))
	for _, tool := range f.tools {
		tools = append(tools, tool)
	}
	return tools
}

func (f *ToolFactory) RegisterTool(toolName string, opts ...ToolOption) error {
	tool, err := New(toolName, opts...)
	if err != nil {
		return utils.Errorf("create tool failed: %v", err)
	}
	f.tools = append(f.tools, tool)
	return nil
}
