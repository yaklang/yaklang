package aitool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// func TestToolParam_GetJSONSchemaString(t *testing.T) {
// 	tests := []struct {
// 		name  string
// 		param *ToolParam
// 		want  map[string]interface{}
// 	}{
// 		{
// 			name: "基本类型",
// 			param: &ToolParam{
// 				Name:        "name",
// 				Type:        "string",
// 				Description: "测试描述",
// 				Default:     "默认值",
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "string",
// 				"description": "测试描述",
// 				"default":     "默认值",
// 			},
// 		},
// 		{
// 			name: "数组类型",
// 			param: &ToolParam{
// 				Name:        "items",
// 				Type:        "array",
// 				Description: "数组参数",
// 				Required:    true,
// 				ArrayItem: []*ToolParamValue{
// 					{
// 						Type:        "string",
// 						Description: "字符串项",
// 						Default:     "默认字符串",
// 					},
// 				},
// 			},
// 			want: map[string]interface{}{
// 				"type":        "array",
// 				"description": "数组参数",
// 				"items": map[string]interface{}{
// 					"type":        "string",
// 					"description": "字符串项",
// 					"default":     "默认字符串",
// 				},
// 			},
// 		},
// 		{
// 			name: "嵌套数组类型",
// 			param: &ToolParam{
// 				Name:        "nestedArray",
// 				Type:        "array",
// 				Description: "嵌套数组",
// 				Required:    true,
// 				ArrayItem: []*ToolParamValue{
// 					{
// 						Type:        "array",
// 						Description: "内部数组",
// 						ArrayItems: []*ToolParamValue{
// 							{
// 								Type:        "number",
// 								Description: "数字项",
// 								Default:     float64(0),
// 							},
// 						},
// 					},
// 				},
// 			},
// 			want: map[string]interface{}{
// 				"type":        "array",
// 				"description": "嵌套数组",
// 				"items": map[string]interface{}{
// 					"type":        "array",
// 					"description": "内部数组",
// 					"items": map[string]interface{}{
// 						"type":        "number",
// 						"description": "数字项",
// 						"default":     float64(0),
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "空描述",
// 			param: &ToolParam{
// 				Name:     "emptyDesc",
// 				Type:     "string",
// 				Default:  "默认值",
// 				Required: true,
// 			},
// 			want: map[string]interface{}{
// 				"type":    "string",
// 				"default": "默认值",
// 			},
// 		},
// 		{
// 			name: "无默认值",
// 			param: &ToolParam{
// 				Name:        "noDefault",
// 				Type:        "string",
// 				Description: "无默认值参数",
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "string",
// 				"description": "无默认值参数",
// 			},
// 		},
// 		{
// 			name: "带特殊字符的描述",
// 			param: &ToolParam{
// 				Name:        "specialChars",
// 				Type:        "string",
// 				Description: "特殊字符：!@#$%^&*()_+{}[]|\"':;?/>.<,~`",
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "string",
// 				"description": "特殊字符：!@#$%^&*()_+{}[]|\"':;?/>.<,~`",
// 			},
// 		},
// 		{
// 			name: "数字类型",
// 			param: &ToolParam{
// 				Name:        "numberParam",
// 				Type:        "number",
// 				Description: "数字参数",
// 				Default:     float64(42),
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "number",
// 				"description": "数字参数",
// 				"default":     float64(42),
// 			},
// 		},
// 		{
// 			name: "布尔类型",
// 			param: &ToolParam{
// 				Name:        "boolParam",
// 				Type:        "boolean",
// 				Description: "布尔参数",
// 				Default:     true,
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "boolean",
// 				"description": "布尔参数",
// 				"default":     true,
// 			},
// 		},
// 		{
// 			name: "空类型",
// 			param: &ToolParam{
// 				Name:        "emptyType",
// 				Description: "空类型参数",
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "",
// 				"description": "空类型参数",
// 			},
// 		},
// 		{
// 			name: "空数组项",
// 			param: &ToolParam{
// 				Name:        "emptyArray",
// 				Type:        "array",
// 				Description: "空数组项参数",
// 				Required:    true,
// 				ArrayItem:   []*ToolParamValue{},
// 			},
// 			want: map[string]interface{}{
// 				"type":        "array",
// 				"description": "空数组项参数",
// 			},
// 		},
// 		{
// 			name: "默认值为零值",
// 			param: &ToolParam{
// 				Name:        "zeroDefault",
// 				Type:        "number",
// 				Description: "零值默认值",
// 				Default:     float64(0),
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "number",
// 				"description": "零值默认值",
// 				"default":     float64(0),
// 			},
// 		},
// 		{
// 			name: "默认值为false",
// 			param: &ToolParam{
// 				Name:        "falseBool",
// 				Type:        "boolean",
// 				Description: "false默认值",
// 				Default:     false,
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "boolean",
// 				"description": "false默认值",
// 				"default":     false,
// 			},
// 		},
// 		{
// 			name: "默认值为空字符串",
// 			param: &ToolParam{
// 				Name:        "emptyString",
// 				Type:        "string",
// 				Description: "空字符串默认值",
// 				Default:     "",
// 				Required:    true,
// 			},
// 			want: map[string]interface{}{
// 				"type":        "string",
// 				"description": "空字符串默认值",
// 				"default":     "",
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := tt.param.GetJSONSchemaString()
// 			fmt.Println(result)
// 			var got map[string]interface{}
// 			if err := json.Unmarshal([]byte(result), &got); err != nil {
// 				t.Errorf("无法解析JSON结果: %v", err)
// 				return
// 			}

// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("GetJSONSchemaString() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

func TestTool_ToJSONSchemaString(t *testing.T) {
	tests := []struct {
		name string
		tool *Tool
		want map[string]interface{}
	}{
		{
			name: "简单工具",
			tool: newTool("testTool",
				WithDescription("测试工具"),
				WithNoRuntimeCallback(testCallback),
				WithStringParam("param1",
					WithParam_Description("字符串参数"),
					WithParam_Required(),
				),
				WithNumberParam("param2",
					WithParam_Description("数字参数"),
					WithParam_Default(42),
				),
			),
			want: map[string]interface{}{
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"type":        "object",
				"description": "测试工具",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "testTool",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "工具的参数",
						"properties": map[string]interface{}{
							"param1": map[string]interface{}{
								"type":        "string",
								"description": "字符串参数",
							},
							"param2": map[string]interface{}{
								"type":        "number",
								"description": "数字参数",
								"default":     float64(42),
							},
						},
						"required": []interface{}{"param1"},
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
		{
			name: "带数组参数的工具",
			tool: newTool("arrayTool",
				WithDescription("带数组的工具"),
				WithNoRuntimeCallback(testCallback),
				WithStringArrayParamEx("stringArray",
					[]PropertyOption{
						WithParam_Description("字符串数组"),
						WithParam_Required(),
					},
					WithParam_Description("数组中的字符串"),
				),
			),
			want: map[string]interface{}{
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"type":        "object",
				"description": "带数组的工具",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "arrayTool",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "工具的参数",
						"properties": map[string]interface{}{
							"stringArray": map[string]interface{}{
								"type":        "array",
								"description": "字符串数组",
								"items": map[string]interface{}{
									"type":        "string",
									"description": "数组中的字符串",
								},
							},
						},
						"required": []interface{}{"stringArray"},
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
		{
			name: "无参数工具",
			tool: newTool("noParamTool",
				WithDescription("无参数工具"),
				WithNoRuntimeCallback(testCallback),
			),
			want: map[string]interface{}{
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"type":        "object",
				"description": "无参数工具",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "noParamTool",
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
		{
			name: "无描述工具",
			tool: newTool("noDescTool",
				WithNoRuntimeCallback(testCallback),
				WithStringParam("param1",
					WithParam_Required(),
				),
			),
			want: map[string]interface{}{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type":    "object",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "noDescTool",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "工具的参数",
						"properties": map[string]interface{}{
							"param1": map[string]interface{}{
								"type": "string",
							},
						},
						"required": []interface{}{"param1"},
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
		{
			name: "多种类型参数工具",
			tool: newTool("multiTypeTool",
				WithDescription("多种类型参数工具"),
				WithNoRuntimeCallback(testCallback),
				WithStringParam("stringParam",
					WithParam_Description("字符串参数"),
					WithParam_Required(),
				),
				WithNumberParam("numberParam",
					WithParam_Description("数字参数"),
					WithParam_Default(42),
				),
				WithBoolParam("booleanParam",
					WithParam_Description("布尔参数"),
					WithParam_Default(true),
					WithParam_Required(),
				),
				WithStructParam("objectParam", []PropertyOption{
					WithParam_Description("对象参数"),
				}),
				WithNullParam("nullParam",
					WithParam_Description("空参数"),
				),
			),
			want: map[string]interface{}{
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"type":        "object",
				"description": "多种类型参数工具",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "multiTypeTool",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "工具的参数",
						"properties": map[string]interface{}{
							"stringParam": map[string]interface{}{
								"type":        "string",
								"description": "字符串参数",
							},
							"numberParam": map[string]interface{}{
								"type":        "number",
								"description": "数字参数",
								"default":     float64(42),
							},
							"booleanParam": map[string]interface{}{
								"type":        "boolean",
								"description": "布尔参数",
								"default":     true,
							},
							"objectParam": map[string]interface{}{
								"type":        "object",
								"description": "对象参数",
							},
							"nullParam": map[string]interface{}{
								"type":        "null",
								"description": "空参数",
							},
						},
						"required": []interface{}{"stringParam", "booleanParam"},
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
		{
			name: "特殊名称工具",
			tool: newTool("special-tool-名称",
				WithDescription("带有特殊字符的工具名称"),
				WithNoRuntimeCallback(testCallback),
				WithStringParam("special_param-名称",
					WithParam_Description("带有特殊字符的参数名称"),
					WithParam_Required(),
				),
			),
			want: map[string]interface{}{
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"type":        "object",
				"description": "带有特殊字符的工具名称",
				"properties": map[string]interface{}{
					"@action": map[string]interface{}{
						"const":       "call-tool",
						"description": "标识当前操作的具体类型",
					},
					"tool": map[string]interface{}{
						"type":        "string",
						"description": "你想要选择的工具名",
						"const":       "special-tool-名称",
					},
					"params": map[string]interface{}{
						"type":        "object",
						"description": "工具的参数",
						"properties": map[string]interface{}{
							"special_param-名称": map[string]interface{}{
								"type":        "string",
								"description": "带有特殊字符的参数名称",
							},
						},
						"required": []interface{}{"special_param-名称"},
					},
				},
				"required":             []interface{}{"tool", "@action", "params"},
				"additionalProperties": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tool.ToJSONSchemaString()
			fmt.Println(result)
			var got map[string]interface{}
			if err := json.Unmarshal([]byte(result), &got); err != nil {
				t.Errorf("无法解析JSON结果: %v", err)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToJSONSchemaString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 测试复杂的嵌套结构
func TestComplexNestedStructures(t *testing.T) {
	complexTool := newTool("complexTool",
		WithDescription("复杂嵌套结构工具"),
		WithNoRuntimeCallback(testCallback),
		WithArrayParamEx("complexParam", []PropertyOption{
			WithParam_Description("复杂参数"),
			WithParam_Required(),
		},
			WithStructParam("objectItem", []PropertyOption{
				WithParam_Description("对象项"),
			}),
		),
	)

	result := complexTool.ToJSONSchemaString()
	fmt.Println(result)
	var parsedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsedSchema); err != nil {
		t.Errorf("无法解析复杂结构的JSON: %v", err)
		return
	}

	// 验证基本结构
	properties, ok := parsedSchema["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("properties 不是期望的类型")
		return
	}

	// 验证@action字段
	action, ok := properties["@action"].(map[string]interface{})
	if !ok {
		t.Errorf("@action 不是期望的类型")
		return
	}
	if action["const"] != "call-tool" {
		t.Errorf("@action const = %v, want 'call-tool'", action["const"])
	}

	// 验证tool字段
	tool, ok := properties["tool"].(map[string]interface{})
	if !ok {
		t.Errorf("tool 不是期望的类型")
		return
	}
	if tool["const"] != "complexTool" {
		t.Errorf("tool const = %v, want 'complexTool'", tool["const"])
	}

	// 验证params字段
	params, ok := properties["params"].(map[string]interface{})
	if !ok {
		t.Errorf("params 不是期望的类型")
		return
	}

	paramsProperties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("params.properties 不是期望的类型")
		return
	}

	complexParamSchema, ok := paramsProperties["complexParam"].(map[string]interface{})
	if !ok {
		t.Errorf("complexParam 不是期望的类型")
		return
	}

	if complexParamSchema["type"] != "array" {
		t.Errorf("complexParam type = %v, want 'array'", complexParamSchema["type"])
	}

	items, ok := complexParamSchema["items"].(map[string]interface{})
	if !ok {
		t.Errorf("items 不是期望的类型")
		return
	}

	if items["type"] != "object" {
		t.Errorf("items type = %v, want 'object'", items["type"])
	}

	// 验证required字段
	required, ok := parsedSchema["required"].([]interface{})
	if !ok {
		t.Errorf("required 不是期望的类型")
		return
	}
	if len(required) < 2 || required[0] != "tool" || required[1] != "@action" {
		t.Errorf("required = %v, want ['tool', '@action']", required)
	}

	// 验证additionalProperties字段
	additionalProps, ok := parsedSchema["additionalProperties"].(bool)
	if !ok || additionalProps != false {
		t.Errorf("additionalProperties = %v, want false", parsedSchema["additionalProperties"])
	}
}

// 测试超深层嵌套结构
func TestDeepNestedStructures(t *testing.T) {
	deepNestedTool := newTool(
		"deepNestedTool",
		WithDescription("深层嵌套结构工具"),
		WithNoRuntimeCallback(testCallback),
		WithArrayParamEx("deepNested", []PropertyOption{
			WithParam_Description("深层嵌套参数"),
		},
			WithArrayParamEx("first", []PropertyOption{
				WithParam_Description("第一层嵌套"),
			},
				WithStringArrayParamEx("second", []PropertyOption{
					WithParam_Description("第二层嵌套"),
				},
					WithParam_Description("最内层参数"),
					WithParam_Default("内层默认值"),
				),
			),
		),
	)
	result := deepNestedTool.ToJSONSchemaString()
	fmt.Println(result)

	// 使用结构化而非反射的方式验证
	var parsedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsedSchema); err != nil {
		t.Errorf("无法解析深层嵌套结构的JSON: %v", err)
		return
	}

	// 顶层验证
	if parsedSchema["type"] != "object" {
		t.Errorf("schema type = %v, want 'object'", parsedSchema["type"])
	}

	if parsedSchema["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Errorf("$schema = %v, want 'http://json-schema.org/draft-07/schema#'", parsedSchema["$schema"])
	}
}

// 测试空工具
func TestEmptyTool(t *testing.T) {
	emptyTool := newTool("")

	result := emptyTool.ToJSONSchemaString()
	fmt.Println(result)

	var parsedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsedSchema); err != nil {
		t.Errorf("无法解析空工具的JSON: %v", err)
		return
	}

	// 验证基本结构
	properties, ok := parsedSchema["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("properties 不是期望的类型")
		return
	}

	// 验证tool字段
	tool, ok := properties["tool"].(map[string]interface{})
	if !ok {
		t.Errorf("tool 不是期望的类型")
		return
	}

	if tool["const"] != "" {
		t.Errorf("空工具名称 tool const = %v, want ''", tool["const"])
	}
}

// 测试极端情况：全部为零值或空值
func TestAllEmptyValues(t *testing.T) {
	emptyValuesTool := newTool("")

	result := emptyValuesTool.ToJSONSchemaString()
	fmt.Println(result)

	var parsedSchema map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsedSchema); err != nil {
		t.Errorf("无法解析全空值工具的JSON: %v", err)
		return
	}

	// 这里主要测试的是是否会出现解析错误或空指针异常
	// 内容验证可以酌情添加
}

// 测试JSON结构合法性
func TestJSONSchemaValidity(t *testing.T) {
	tools := []*Tool{
		newTool("validationTool1",
			WithDescription("验证工具1"),
			WithNoRuntimeCallback(testCallback),
		),
		newTool("validationTool2",
			WithDescription("验证工具2"),
			WithNoRuntimeCallback(testCallback),
			WithStringParam("param1",
				WithParam_Required(),
				WithParam_Description("参数1"),
			),
		),
	}

	for _, tool := range tools {
		result := tool.ToJSONSchemaString()

		// 验证生成的JSON是否符合JSON Schema规范
		schemaValidationURL := "http://json-schema.org/draft-07/schema#"

		// 检查输出中是否包含正确的$schema
		if !strings.Contains(result, schemaValidationURL) {
			t.Errorf("生成的JSON Schema不包含正确的$schema: %s", result)
		}

		// 解析并验证结构
		var parsedSchema map[string]interface{}
		if err := json.Unmarshal([]byte(result), &parsedSchema); err != nil {
			t.Errorf("无法解析JSON Schema: %v", err)
			continue
		}

		// 检查必要字段
		requiredFields := []string{"$schema", "type", "properties", "required"}
		for _, field := range requiredFields {
			if _, exists := parsedSchema[field]; !exists {
				t.Errorf("JSON Schema缺少必要字段: %s", field)
			}
		}
	}
}
