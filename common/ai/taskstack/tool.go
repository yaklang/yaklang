package taskstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// InvokeCallback 定义工具调用回调函数的签名
type InvokeCallback func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error)

type Tool struct {
	Name        string
	Description string
	Params      []*ToolParam
	Callback    InvokeCallback // 添加回调函数字段
}

type ToolParam struct {
	Name        string
	Type        string
	Description string
	Default     any
	Required    bool
	ArrayItem   []*ToolParamValue
}

type ToolParamValue struct {
	Type        string
	Description string
	Default     any
	ArrayItems  []*ToolParamValue
}

// ToolOption 定义工具选项函数的类型
type ToolOption func(*Tool)

// NewTool 使用函数选项模式创建一个新的Tool实例
func NewTool(name string, options ...ToolOption) (*Tool, error) {
	tool := &Tool{
		Name:   name,
		Params: []*ToolParam{},
	}

	// 应用所有选项
	for _, option := range options {
		option(tool)
	}

	// 检查是否设置了回调函数
	if tool.Callback == nil {
		return nil, errors.New("回调函数未设置，请使用 WithTool_Callback 选项设置回调函数")
	}

	return tool, nil
}

// WithTool_Description 设置工具的描述信息
func WithTool_Description(description string) ToolOption {
	return func(t *Tool) {
		t.Description = description
	}
}

// WithTool_Param 添加一个参数到工具
func WithTool_Param(param *ToolParam) ToolOption {
	return func(t *Tool) {
		t.Params = append(t.Params, param)
	}
}

// WithTool_Callback 设置工具的回调函数
func WithTool_Callback(callback InvokeCallback) ToolOption {
	return func(t *Tool) {
		t.Callback = callback
	}
}

// NewToolParam 创建一个新的工具参数
func NewToolParam(name string, paramType string, options ...ToolParamOption) *ToolParam {
	param := &ToolParam{
		Name: name,
		Type: paramType,
	}

	// 应用所有参数选项
	for _, option := range options {
		option(param)
	}

	return param
}

// ToolParamOption 定义参数选项函数的类型
type ToolParamOption func(*ToolParam)

// WithTool_ParamDescription 设置参数的描述信息
func WithTool_ParamDescription(description string) ToolParamOption {
	return func(p *ToolParam) {
		p.Description = description
	}
}

// WithTool_ParamDefault 设置参数的默认值
func WithTool_ParamDefault(defaultValue any) ToolParamOption {
	return func(p *ToolParam) {
		p.Default = defaultValue
	}
}

// WithTool_ParamRequired 设置参数是否必需
func WithTool_ParamRequired(required bool) ToolParamOption {
	return func(p *ToolParam) {
		p.Required = required
	}
}

// WithTool_ArrayItem 设置数组类型参数的元素类型
func WithTool_ArrayItem(arrayItem *ToolParamValue) ToolParamOption {
	return func(p *ToolParam) {
		p.ArrayItem = append(p.ArrayItem, arrayItem)
	}
}

// NewToolParamValue 创建一个参数值
func NewToolParamValue(valueType string, options ...ToolParamValueOption) *ToolParamValue {
	value := &ToolParamValue{
		Type: valueType,
	}

	// 应用所有值选项
	for _, option := range options {
		option(value)
	}

	return value
}

// ToolParamValueOption 定义参数值选项函数的类型
type ToolParamValueOption func(*ToolParamValue)

// WithTool_ValueDescription 设置值的描述信息
func WithTool_ValueDescription(description string) ToolParamValueOption {
	return func(v *ToolParamValue) {
		v.Description = description
	}
}

// WithTool_ValueDefault 设置值的默认值
func WithTool_ValueDefault(defaultValue any) ToolParamValueOption {
	return func(v *ToolParamValue) {
		v.Default = defaultValue
	}
}

// WithTool_ValueArrayItems 设置数组值的元素类型
func WithTool_ValueArrayItems(arrayItems []*ToolParamValue) ToolParamValueOption {
	return func(v *ToolParamValue) {
		v.ArrayItems = arrayItems
	}
}

// GetJSONSchemaString 将ToolParam转换为JSON Schema格式的字符串
func (t *ToolParam) GetJSONSchemaString() string {
	schema := map[string]interface{}{
		"type": t.Type,
	}

	if t.Description != "" {
		schema["description"] = t.Description
	}

	if t.Default != nil {
		schema["default"] = t.Default
	}

	// 处理数组类型
	if t.Type == "array" && len(t.ArrayItem) > 0 {
		items := make(map[string]interface{})

		// 使用第一个ArrayItem元素作为数组项的类型
		firstItem := t.ArrayItem[0]
		items["type"] = firstItem.Type

		if firstItem.Description != "" {
			items["description"] = firstItem.Description
		}

		if firstItem.Default != nil {
			items["default"] = firstItem.Default
		}

		// 如果数组项本身是一个数组
		if firstItem.Type == "array" && len(firstItem.ArrayItems) > 0 {
			nestedItems := make(map[string]interface{})
			nestedFirstItem := firstItem.ArrayItems[0]
			nestedItems["type"] = nestedFirstItem.Type

			if nestedFirstItem.Description != "" {
				nestedItems["description"] = nestedFirstItem.Description
			}

			if nestedFirstItem.Default != nil {
				nestedItems["default"] = nestedFirstItem.Default
			}

			items["items"] = nestedItems
		}

		schema["items"] = items
	}

	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(jsonBytes)
}

// ToJSONSchema 将整个Tool转换为符合JSON Schema Draft-07规范的格式
func (t *Tool) ToJSONSchema() map[string]interface{} {
	// 构建工具参数的properties
	properties := make(map[string]interface{})

	// 添加@action字段
	properties["@action"] = map[string]interface{}{
		"const":       "describe-tool",
		"description": "标识当前操作的具体类型",
	}

	// 添加tool字段
	properties["tool"] = map[string]interface{}{
		"type":        "string",
		"description": "你想要选择的工具名",
		"const":       t.Name,
	}

	// 增加参数字段
	paramProperties := make(map[string]interface{})
	paramRequired := []string{}

	for _, param := range t.Params {
		paramSchema := map[string]interface{}{}

		if param.Type != "" {
			paramSchema["type"] = param.Type
		}

		if param.Description != "" {
			paramSchema["description"] = param.Description
		}

		if param.Default != nil {
			paramSchema["default"] = param.Default
		}

		// 处理数组类型
		if param.Type == "array" && len(param.ArrayItem) > 0 {
			items := make(map[string]interface{})

			// 使用第一个ArrayItem元素作为数组项的类型
			firstItem := param.ArrayItem[0]
			items["type"] = firstItem.Type

			if firstItem.Description != "" {
				items["description"] = firstItem.Description
			}

			if firstItem.Default != nil {
				items["default"] = firstItem.Default
			}

			// 如果数组项本身是一个数组
			if firstItem.Type == "array" && len(firstItem.ArrayItems) > 0 {
				nestedItems := make(map[string]interface{})
				nestedFirstItem := firstItem.ArrayItems[0]
				nestedItems["type"] = nestedFirstItem.Type

				if nestedFirstItem.Description != "" {
					nestedItems["description"] = nestedFirstItem.Description
				}

				if nestedFirstItem.Default != nil {
					nestedItems["default"] = nestedFirstItem.Default
				}

				items["items"] = nestedItems
			}

			paramSchema["items"] = items
		}

		paramProperties[param.Name] = paramSchema

		if param.Required {
			paramRequired = append(paramRequired, param.Name)
		}
	}

	// 将参数添加到params字段
	if len(paramProperties) > 0 {
		paramsSchema := map[string]interface{}{
			"type":        "object",
			"properties":  paramProperties,
			"description": "工具的参数",
		}

		if len(paramRequired) > 0 {
			paramsSchema["required"] = paramRequired
		}

		properties["params"] = paramsSchema
	}

	// 构建最终的JSON Schema
	schema := map[string]interface{}{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"tool", "@action"},
		"additionalProperties": false,
	}

	if t.Description != "" {
		schema["description"] = t.Description
	}

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

/*
示例用法:

```go
// 创建一个简单的工具
simpleTool := NewTool("simpleSearch",
	WithTool_Description("一个简单的搜索工具"))

// 创建一个包含参数的工具
searchTool := NewTool("search",
	WithTool_Description("搜索特定内容的工具"),
	WithTool_Param(
		NewToolParam("query", "string",
			WithTool_ParamDescription("要搜索的查询字符串"),
			WithTool_ParamRequired(true),
		),
	),
	WithTool_Param(
		NewToolParam("limit", "integer",
			WithTool_ParamDescription("返回结果的最大数量"),
			WithTool_ParamDefault(10),
		),
	),
)

// 创建一个包含数组参数的工具
arrayParamTool := NewTool("complexSearch",
	WithTool_Description("高级搜索工具"),
	WithTool_Param(
		NewToolParam("queries", "array",
			WithTool_ParamDescription("要搜索的多个查询"),
			WithTool_ArrayItem(
				NewToolParamValue("string",
					WithTool_ValueDescription("查询字符串"),
				),
			),
		),
	),
)

// 转换为JSON Schema字符串
schemaStr := searchTool.ToJSONSchemaString()
fmt.Println(schemaStr)
```
*/
