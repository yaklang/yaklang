package taskstack

import (
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// 提供一个测试用的回调函数
func testCallback(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
	return params, nil
}

// TestNewToolWithOptions 测试使用函数选项模式创建工具
func TestNewToolWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		builder  func() (*Tool, error)
		expected *Tool
	}{
		{
			name: "简单工具",
			builder: func() (*Tool, error) {
				return NewTool("simpleTool",
					WithTool_Description("简单工具描述"),
					WithTool_Callback(testCallback))
			},
			expected: &Tool{
				Tool: &mcp.Tool{
					Name:        "simpleTool",
					Description: "简单工具描述",
					InputSchema: mcp.ToolInputSchema{
						Type:       "object",
						Properties: map[string]any{},
					},
				},
			},
		},
		{
			name: "带参数的工具",
			builder: func() (*Tool, error) {
				return NewTool("paramTool",
					WithTool_Description("带参数的工具"),
					WithTool_Callback(testCallback),
					WithTool_StringParam("query",
						WithParam_Description("查询参数"),
						WithParam_Required(),
					),
					WithTool_NumberParam("limit",
						WithParam_Description("限制数量"),
						WithParam_Default(10),
					),
				)
			},
			expected: &Tool{
				Tool: &mcp.Tool{
					Name:        "paramTool",
					Description: "带参数的工具",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "查询参数",
							},
							"limit": map[string]any{
								"type":        "number",
								"description": "限制数量",
								"default":     10,
							},
						},
					},
				},
			},
		},
		{
			name: "带数组参数的工具",
			builder: func() (*Tool, error) {
				return NewTool("arrayTool",
					WithTool_Description("带数组参数的工具"),
					WithTool_Callback(testCallback),
					WithTool_StringArrayParamEx("items",
						[]PropertyOption{
							WithParam_Description("数组参数"),
							WithParam_Required(),
						},
						WithParam_Description("字符串项"),
					),
				)
			},
			expected: &Tool{
				Tool: &mcp.Tool{
					Name:        "arrayTool",
					Description: "带数组参数的工具",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"items": map[string]any{
								"type":        "array",
								"description": "数组参数",
								"items": map[string]any{
									"type":        "string",
									"description": "字符串项",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "嵌套数组参数工具",
			builder: func() (*Tool, error) {
				return NewTool("nestedArrayTool",
					WithTool_Description("嵌套数组参数工具"),
					WithTool_Callback(testCallback),
					WithTool_ArrayParamEx("nestedItems",
						[]PropertyOption{
							WithParam_Description("嵌套数组参数"),
							WithParam_Required(),
						},
						WithTool_ArrayParam("array",
							"number",
							[]PropertyOption{
								WithParam_Description("数组项"),
							},
							WithParam_Default(0),
							WithParam_Description("数字项"),
						),
					),
				)
			},
			expected: &Tool{
				Tool: &mcp.Tool{
					Name:        "nestedArrayTool",
					Description: "嵌套数组参数工具",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"nestedItems": map[string]any{
								"type":        "array",
								"description": "嵌套数组参数",
								"items": map[string]any{
									"type":        "array",
									"description": "数组项",
									"items": map[string]any{
										"type":        "number",
										"description": "数字项",
										"default":     0,
									},
								},
							},
						},
					},
				},
				Callback: testCallback,
			},
		},
		{
			name: "多参数工具",
			builder: func() (*Tool, error) {
				return NewTool("multiParamTool",
					WithTool_Description("多参数工具"),
					WithTool_Callback(testCallback),
					WithTool_StringParam("stringParam",
						WithParam_Description("字符串参数"),
						WithParam_Required(),
					),
					WithTool_NumberParam("numberParam",
						WithParam_Default(42.5),
					),
					WithTool_BoolParam("boolParam",
						WithParam_Description("布尔参数"),
						WithParam_Default(true),
						WithParam_Required(),
					),
				)
			},
			expected: &Tool{
				Tool: &mcp.Tool{
					Name:        "multiParamTool",
					Description: "多参数工具",
					InputSchema: mcp.ToolInputSchema{
						Type: "object",
						Properties: map[string]any{
							"stringParam": map[string]any{
								"type":        "string",
								"description": "字符串参数",
							},
							"numberParam": map[string]any{
								"type":    "number",
								"default": 42.5,
							},
							"boolParam": map[string]any{
								"type":        "boolean",
								"description": "布尔参数",
								"default":     true,
							},
						},
						Required: []string{"stringParam", "boolParam"},
					},
				},
				Callback: testCallback,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := tt.builder()
			if err != nil {
				t.Errorf("创建工具出错: %v", err)
				return
			}

			// 检查工具名称
			if tool.Name != tt.expected.Name {
				t.Errorf("Name = %v, want %v", tool.Name, tt.expected.Name)
			}

			// 检查工具描述
			if tool.Description != tt.expected.Description {
				t.Errorf("Description = %v, want %v", tool.Description, tt.expected.Description)
			}

			// 检查参数数量
			if len(tool.Params()) != len(tt.expected.Params()) {
				t.Errorf("Params length = %v, want %v", len(tool.Params()), len(tt.expected.Params()))
				return
			}

			// 检查回调函数是否设置
			if tool.Callback == nil {
				t.Errorf("Callback is nil, expected non-nil")
			}

			// 检查每个参数
			require.True(t, reflect.DeepEqual(tool.Params(), tt.expected.Params()))
			// for i, param := range tool.Params {
			// 	expectedParam := tt.expected.Params[i]

			// 	if param.Name != expectedParam.Name {
			// 		t.Errorf("Param[%d].Name = %v, want %v", i, param.Name, expectedParam.Name)
			// 	}

			// 	if param.Type != expectedParam.Type {
			// 		t.Errorf("Param[%d].Type = %v, want %v", i, param.Type, expectedParam.Type)
			// 	}

			// 	if param.Description != expectedParam.Description {
			// 		t.Errorf("Param[%d].Description = %v, want %v", i, param.Description, expectedParam.Description)
			// 	}

			// 	if !reflect.DeepEqual(param.Default, expectedParam.Default) {
			// 		t.Errorf("Param[%d].Default = %v, want %v", i, param.Default, expectedParam.Default)
			// 	}

			// 	if param.Required != expectedParam.Required {
			// 		t.Errorf("Param[%d].Required = %v, want %v", i, param.Required, expectedParam.Required)
			// 	}

			// 	// 检查数组项
			// 	if len(param.ArrayItem) != len(expectedParam.ArrayItem) {
			// 		t.Errorf("Param[%d].ArrayItem length = %v, want %v", i, len(param.ArrayItem), len(expectedParam.ArrayItem))
			// 		continue
			// 	}

			// 	for j, item := range param.ArrayItem {
			// 		expectedItem := expectedParam.ArrayItem[j]

			// 		if item.Type != expectedItem.Type {
			// 			t.Errorf("Param[%d].ArrayItem[%d].Type = %v, want %v", i, j, item.Type, expectedItem.Type)
			// 		}

			// 		if item.Description != expectedItem.Description {
			// 			t.Errorf("Param[%d].ArrayItem[%d].Description = %v, want %v", i, j, item.Description, expectedItem.Description)
			// 		}

			// 		if !reflect.DeepEqual(item.Default, expectedItem.Default) {
			// 			t.Errorf("Param[%d].ArrayItem[%d].Default = %v, want %v", i, j, item.Default, expectedItem.Default)
			// 		}

			// 		// 检查嵌套数组项
			// 		if len(item.ArrayItems) != len(expectedItem.ArrayItems) {
			// 			t.Errorf("Param[%d].ArrayItem[%d].ArrayItems length = %v, want %v", i, j, len(item.ArrayItems), len(expectedItem.ArrayItems))
			// 			continue
			// 		}

			// 		for k, nestedItem := range item.ArrayItems {
			// 			expectedNestedItem := expectedItem.ArrayItems[k]

			// 			if nestedItem.Type != expectedNestedItem.Type {
			// 				t.Errorf("Param[%d].ArrayItem[%d].ArrayItems[%d].Type = %v, want %v", i, j, k, nestedItem.Type, expectedNestedItem.Type)
			// 			}

			// 			if nestedItem.Description != expectedNestedItem.Description {
			// 				t.Errorf("Param[%d].ArrayItem[%d].ArrayItems[%d].Description = %v, want %v", i, j, k, nestedItem.Description, expectedNestedItem.Description)
			// 			}

			// 			if !reflect.DeepEqual(nestedItem.Default, expectedNestedItem.Default) {
			// 				t.Errorf("Param[%d].ArrayItem[%d].ArrayItems[%d].Default = %v, want %v", i, j, k, nestedItem.Default, expectedNestedItem.Default)
			// 			}
			// 		}
			// 	}
			// }
		})
	}
}

// TestMissingCallback 测试缺少回调函数的情况
func TestMissingCallback(t *testing.T) {
	_, err := NewTool("noCallbackTool", WithTool_Description("没有回调的工具"))
	if err == nil {
		t.Errorf("Expected error for missing callback, got nil")
	}
}

// TestToolJSONSchemaGeneration 测试使用函数选项模式创建的工具生成的JSON Schema
func TestToolJSONSchemaGeneration(t *testing.T) {
	tests := []struct {
		name           string
		tool           *Tool
		expectedFields []string
	}{
		{
			name: "简单工具JSON Schema",
			tool: newTool("simpleTool",
				WithTool_Description("简单工具描述"),
				WithTool_Callback(testCallback),
			),
			expectedFields: []string{"$schema", "type", "description", "properties", "required"},
		},
		{
			name: "带参数工具JSON Schema",
			tool: newTool("paramTool",
				WithTool_Description("带参数的工具"),
				WithTool_Callback(testCallback),
				WithTool_StringParam("query",
					WithParam_Description("查询参数"),
					WithParam_Required(),
				),
			),
			expectedFields: []string{"$schema", "type", "description", "properties", "required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 生成JSON Schema
			schemaStr := tt.tool.ToJSONSchemaString()

			// 解析JSON
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
				t.Errorf("无法解析JSON Schema: %v", err)
				return
			}

			// 验证必要字段
			for _, field := range tt.expectedFields {
				if _, exists := schema[field]; !exists {
					t.Errorf("生成的JSON Schema缺少字段: %s", field)
				}
			}

			// 验证工具名称
			properties, ok := schema["properties"].(map[string]interface{})
			if !ok {
				t.Errorf("properties 不是期望的类型")
				return
			}

			tool, ok := properties["tool"].(map[string]interface{})
			if !ok {
				t.Errorf("tool 不是期望的类型")
				return
			}

			if tool["const"] != tt.tool.Name {
				t.Errorf("tool const = %v, want %v", tool["const"], tt.tool.Name)
			}
		})
	}
}

// TestComplexToolCreation 测试创建复杂工具
func TestComplexToolCreation(t *testing.T) {
	// 创建一个复杂工具
	complexTool, err := NewTool("complexTool",
		WithTool_Description("复杂工具"),
		WithTool_Callback(testCallback),
		WithTool_StringParam("simpleParam",
			WithParam_Description("简单参数"),
			WithParam_Required(),
		),
		WithTool_StringArrayParamEx("arrayParam",
			[]PropertyOption{
				WithParam_Description("数组参数"),
				WithParam_Required(),
			},
			WithParam_Description("字符串项"),
		),
		WithTool_ArrayParamEx("nestedItems",
			[]PropertyOption{
				WithParam_Description("嵌套数组参数"),
				WithParam_Required(),
			},
			WithTool_ArrayParam("array",
				"number",
				[]PropertyOption{
					WithParam_Description("数组项"),
				},
				WithParam_Default(0),
			),
		),
	)

	if err != nil {
		t.Errorf("创建复杂工具出错: %v", err)
		return
	}

	// 验证基本属性
	if complexTool.Name != "complexTool" {
		t.Errorf("Name = %v, want %v", complexTool.Name, "complexTool")
	}

	if complexTool.Description != "复杂工具" {
		t.Errorf("Description = %v, want %v", complexTool.Description, "复杂工具")
	}

	if len(complexTool.Params()) != 3 {
		t.Errorf("Params length = %v, want %v", len(complexTool.Params()), 3)
	}

	// 验证JSON Schema生成
	schemaStr := complexTool.ToJSONSchemaString()
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		t.Errorf("无法解析复杂工具的JSON Schema: %v", err)
		return
	}

	// 验证必要字段
	requiredFields := []string{"$schema", "type", "description", "properties", "required"}
	for _, field := range requiredFields {
		if _, exists := schema[field]; !exists {
			t.Errorf("复杂工具的JSON Schema缺少字段: %s", field)
		}
	}
}
