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
	// BrowserSessionTracker is set for AI ReAct tool runs to hook browser.Open ids.
	BrowserSessionTracker interface {
		TrackBrowserSession(id string)
	}
}

// NoRuntimeInvokeCallback 定义工具调用回调函数的签名
type NoRuntimeInvokeCallback func(ctx context.Context, params InvokeParams, stdout io.Writer, stderr io.Writer) (any, error)
type InvokeCallback func(ctx context.Context, params InvokeParams, invokeExConfig *ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error)

// MCPClientCloser closes an outbound MCP client kept alive for bridge tool callbacks.
type MCPClientCloser interface {
	Close() error
}

type Tool struct {
	*mcp.Tool
	// A list of keywords for tool indexing and searching.
	Keywords    []string `json:"keywords,omitempty"`
	VerboseName string   `json:"verbose_name,omitempty"`
	// Usage 工具使用说明，在参数生成阶段(第2阶段)才披露给 AI，
	// 包含使用原则、参数建议、关联使用等信息，帮助 AI 更好地使用工具参数。
	Usage    string         `json:"usage,omitempty"`
	Callback InvokeCallback // 添加回调函数字段
	// MCPPendingStub marks a placeholder MCP tool loaded from DB cache before the
	// remote server connection completes. Invokers should wait for a live replacement.
	MCPPendingStub bool `json:"-"`
	// BridgeMCPClient is the live outbound MCP client for tools bridged from an
	// external server. The hosting Yak MCP server closes it on shutdown.
	BridgeMCPClient MCPClientCloser `json:"-"`
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

func WithVerboseName(verboseName string) ToolOption {
	return func(t *Tool) {
		t.VerboseName = verboseName
	}
}

// WithUsage 设置工具的使用说明（在参数生成阶段披露）
func WithUsage(usage string) ToolOption {
	return func(t *Tool) {
		t.Usage = usage
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

// WithMCPPendingStub marks the tool as an MCP cache stub (not yet connected to remote server).
func WithMCPPendingStub(pending bool) ToolOption {
	return func(t *Tool) {
		t.MCPPendingStub = pending
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

// WithParam_Description 为属性添加描述信息（导出名为 jsonschema.description）
// 参数:
//   - desc: 属性描述文本
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.description("the user name")))
// assert str.Contains(schema, "the user name"), "schema should contain the description"
// ```
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

// WithParam_Required 将属性标记为必填（导出名为 jsonschema.required）
// 参数:
//   - required: 是否必填，缺省为 true
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.required(true)))
// assert str.Contains(schema, "required"), "schema should mark name as required"
// ```
func WithParam_Required(required ...bool) PropertyOption {
	return func(schema map[string]any) {
		if len(required) > 0 {
			schema["required"] = required[0]
		} else {
			schema["required"] = true
		}
	}
}

// WithParam_Title 为属性添加便于展示的标题（导出名为 jsonschema.title）
// 参数:
//   - title: 属性标题
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.title("User Name")))
// assert str.Contains(schema, "User Name"), "schema should contain the title"
// ```
func WithParam_Title(title string) PropertyOption {
	return func(schema map[string]any) {
		schema["title"] = title
	}
}

// WithParam_Example 为属性添加示例值（导出名为 jsonschema.example）
// 参数:
//   - i: 示例值
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.example("yak")))
// assert str.Contains(schema, "example"), "schema should contain the example"
// ```
func WithParam_Example(i any) PropertyOption {
	return func(schema map[string]any) {
		schema["example"] = i
	}
}

// WithParam_Raw 向属性写入原始的 JSON Schema 键值（导出名为 jsonschema.raw）
// 参数:
//   - name: 键名
//   - v: 键值
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("status", jsonschema.raw("format", "email")))
// assert str.Contains(schema, "format"), "schema should contain the raw key"
// ```
func WithParam_Raw(name string, v any) PropertyOption {
	return func(m map[string]any) {
		m[name] = v
	}
}

//
// String Property Options
//

// WithParam_Enum 指定属性的可选枚举值列表（导出名为 jsonschema.enum）
// 参数:
//   - values: 允许的取值列表
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("level", jsonschema.enum("low", "high")))
// assert str.Contains(schema, "enum"), "schema should contain enum values"
// ```
func WithParam_Enum(values ...any) PropertyOption {
	return func(schema map[string]any) {
		schema["enum"] = values
	}
}

// WithParam_Const 指定属性的常量取值（导出名为 jsonschema.const）
// 参数:
//   - values: 常量值
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("type", jsonschema.const("user")))
// assert str.Contains(schema, "const"), "schema should contain const value"
// ```
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

// WithParam_MaxLength 设置字符串属性的最大长度（导出名为 jsonschema.maxLength）
// 参数:
//   - max: 最大长度
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.maxLength(20)))
// assert str.Contains(schema, "maxLength"), "schema should contain maxLength"
// ```
func WithParam_MaxLength(max int) PropertyOption {
	return func(schema map[string]any) {
		schema["maxLength"] = max
	}
}

// WithParam_MinLength 设置字符串属性的最小长度（导出名为 jsonschema.minLength）
// 参数:
//   - min: 最小长度
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name", jsonschema.minLength(2)))
// assert str.Contains(schema, "minLength"), "schema should contain minLength"
// ```
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

// WithParam_Max 设置数值属性的最大值（导出名为 jsonschema.max）
// 参数:
//   - max: 最大值
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramNumber("age", jsonschema.max(120)))
// assert str.Contains(schema, "maximum"), "schema should contain maximum"
// ```
func WithParam_Max(max float64) PropertyOption {
	return func(schema map[string]any) {
		schema["maximum"] = max
	}
}

// WithParam_Min 设置数值属性的最小值（导出名为 jsonschema.min）
// 参数:
//   - min: 最小值
//
// 返回值:
//   - 属性可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramNumber("age", jsonschema.min(0)))
// assert str.Contains(schema, "minimum"), "schema should contain minimum"
// ```
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

// WithBoolParam 向 schema 添加一个布尔类型属性（导出名为 jsonschema.paramBool）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项，如 jsonschema.description / jsonschema.required
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramBool("enabled"))
// assert str.Contains(schema, "boolean"), "schema should contain a boolean property"
// ```
func WithBoolParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "boolean",
	}
	return WithRawParam(name, schema, opts...)
}

// WithIntegerParam 向 schema 添加一个整数类型属性（导出名为 jsonschema.paramInt）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项，如 jsonschema.description / jsonschema.required
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramInt("age"))
// assert str.Contains(schema, "integer"), "schema should contain an integer property"
// ```
func WithIntegerParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "integer",
	}
	return WithRawParam(name, schema, opts...)
}

// WithNumberParam 向 schema 添加一个数值类型属性（导出名为 jsonschema.paramNumber）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项，如 jsonschema.min / jsonschema.max
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramNumber("score"))
// assert str.Contains(schema, "number"), "schema should contain a number property"
// ```
func WithNumberParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "number",
	}
	return WithRawParam(name, schema, opts...)
}

// WithStringParam 向 schema 添加一个字符串类型属性（导出名为 jsonschema.paramString）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项，如 jsonschema.description / jsonschema.enum
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramString("name"))
// assert str.Contains(schema, "string"), "schema should contain a string property"
// ```
func WithStringParam(name string, opts ...PropertyOption) ToolOption {
	schema := map[string]any{
		"type": "string",
	}
	return WithRawParam(name, schema, opts...)
}

// WithStringArrayParam 向 schema 添加一个字符串数组类型属性（导出名为 jsonschema.paramStringArray）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramStringArray("tags"))
// assert str.Contains(schema, "array"), "schema should contain a string array property"
// ```
func WithStringArrayParam(name string, opts ...PropertyOption) ToolOption {
	return WithSimpleArrayParam(name, "string", opts...)
}

func WithStringArrayParamEx(name string, opts []PropertyOption, itemsOpt ...PropertyOption) ToolOption {
	return WithArrayParam(name, "string", opts, itemsOpt...)
}

// WithNumberArrayParam 向 schema 添加一个数值数组类型属性（导出名为 jsonschema.paramNumberArray）
// 参数:
//   - name: 属性名
//   - opts: 属性可选项
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramNumberArray("scores"))
// assert str.Contains(schema, "array"), "schema should contain a number array property"
// ```
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
	// temp 在构造期生成一次 (单线程), 之后只读不写; 它持有嵌套 properties.
	temp := newTool("", itemsOpt...)

	// Save the nested required array (required fields inside this struct)
	var nestedRequired []string
	if len(temp.InputSchema.Required) > 0 {
		nestedRequired = temp.InputSchema.Required
	}

	// Create a ToolOption that applies PropertyOptions and preserves nested required
	return func(t *Tool) {
		// 关键: schema map 必须在闭包内部新建, 不能在构造期捕获后复用. 否则同一个
		// ToolOption 被并发复用应用时 (如 reactloops.buildSchema 每次重新应用
		// action.Options), 多个 goroutine 会并发写同一个 schema map, 触发
		// "fatal error: concurrent map writes". 嵌套 properties 只读引用, 并发安全.
		// 关键词: WithStructParam 并发安全, schema map 闭包内新建, concurrent map writes 修复
		schema := map[string]any{
			"type": "object",
		}
		if temp.InputSchema.Properties != nil && temp.InputSchema.Properties.Len() > 0 {
			schema["properties"] = temp.InputSchema.Properties
		}

		// Apply PropertyOptions to the schema
		for _, opt := range opts {
			opt(schema)
		}

		// Restore nested required array if it was set
		// This must happen AFTER applying PropertyOptions to avoid being overwritten
		if len(nestedRequired) > 0 {
			schema["required"] = nestedRequired
		}

		// Handle top-level required (whether this struct parameter itself is required).
		if required, ok := schema["required"].(bool); ok {
			delete(schema, "required")
			if required {
				if t.InputSchema.Required == nil {
					t.InputSchema.Required = []string{name}
				} else {
					t.InputSchema.Required = append(t.InputSchema.Required, name)
				}
			}
			// Re-add nested required array if it exists
			if len(nestedRequired) > 0 {
				schema["required"] = nestedRequired
			}
		}

		t.InputSchema.Properties.Set(name, schema)
	}
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

// WithKVPairsParam 向 schema 添加一个键值对数组类型属性（导出名为 jsonschema.paramKeyValuePairsArray）
// 适合表达 HTTP headers、查询参数等 key/value 列表结构
// 参数:
//   - name: 属性名
//   - opts: 属性可选项
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramKeyValuePairsArray("headers"))
// assert str.Contains(schema, "array"), "schema should contain a kv-pairs array property"
// ```
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

// WithRawParam 以原始 schema 对象的方式向 schema 添加一个属性（导出名为 jsonschema.paramRaw）
// 参数:
//   - name: 属性名
//   - object: 该属性的原始 JSON Schema 对象
//   - opts: 属性可选项
//
// 返回值:
//   - schema 构建可选项
//
// Example:
// ```
// schema = jsonschema.Object(jsonschema.paramRaw("ip", {"type": "string", "format": "ipv4"}))
// assert str.Contains(schema, "ipv4"), "schema should contain the raw object property"
// ```
func WithRawParam(name string, object map[string]any, opts ...PropertyOption) ToolOption {
	return func(t *Tool) {
		// 关键: 绝对不能直接修改传入/闭包捕获的 object map. 同一个 ToolOption 闭包会被
		// 复用并发应用 (例如 reactloops.buildSchema 缓存了 action.Options, 每次生成
		// prompt 都重新应用一遍; agent 存在并发 loop 时会被多 goroutine 同时执行).
		// 直接写 object 既会让多个 tool 共享同一个 map, 又会与其他 goroutine 的写/序列化
		// 冲突, 触发 "fatal error: concurrent map writes". 这里对 object 做一次浅拷贝,
		// 所有 opt 写入副本, 保证闭包可重入且并发安全.
		// 关键词: WithRawParam 并发安全, schema map 浅拷贝, concurrent map writes 修复
		merged := make(map[string]any, len(object)+1)
		for k, v := range object {
			merged[k] = v
		}
		for _, opt := range opts {
			opt(merged)
		}

		// Handle required field - can be bool (for simple params) or []string (for nested structs).
		// Bool required is internal metadata only; it must not appear in exported JSON Schema
		// (JSON Schema "required" is an array of property names on objects, not a per-field bool).
		if requiredVal, exists := merged["required"]; exists {
			if required, ok := requiredVal.(bool); ok {
				delete(merged, "required")
				if required {
					if t.InputSchema.Required == nil {
						t.InputSchema.Required = []string{name}
					} else {
						t.InputSchema.Required = append(t.InputSchema.Required, name)
					}
				}
			}
			// If it's a []string (nested struct), keep it in the schema
			// It will be handled by buildStructOptionsFromMap during rebuild
		}

		t.InputSchema.Properties.Set(name, merged)
	}
}

func (t *Tool) GetName() string {
	return t.Name
}

func (t *Tool) GetDescription() string {
	return t.Description
}

func (t *Tool) GetVerboseName() string {
	return t.VerboseName
}

func (t *Tool) GetKeywords() []string {
	return t.Keywords
}

func (t *Tool) GetUsage() string {
	return t.Usage
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
