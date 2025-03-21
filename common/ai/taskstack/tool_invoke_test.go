package taskstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestToolInvoke 测试工具调用
func TestToolInvoke(t *testing.T) {
	// 创建一个简单的回调函数
	echoCallback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stdout, "执行工具：%s\n", "echo")
		return params, nil
	}

	// 创建一个带错误的回调函数
	errorCallback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stderr, "工具执行错误\n")
		return nil, errors.New("回调函数错误")
	}

	// 创建一个带参数验证的回调函数
	validationCallback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		// 验证必要参数
		query, ok := params["query"]
		if !ok {
			fmt.Fprintf(stderr, "缺少必要参数 'query'\n")
			return nil, errors.New("缺少必要参数 'query'")
		}

		fmt.Fprintf(stdout, "查询: %s\n", query.(string))
		return map[string]interface{}{
			"result": "查询结果: " + query.(string),
		}, nil
	}

	tests := []struct {
		name           string
		toolBuilder    func() (*Tool, error)
		inputJSON      string
		expectedResult *ToolResult
		expectError    bool
	}{
		{
			name: "基本调用测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("echoTool",
					WithTool_Description("回显工具"),
					WithTool_Callback(echoCallback),
					WithString("message",
						Description("要回显的消息"),
						Required(),
					),
				)
			},
			inputJSON: `{
				"tool": "echoTool",
				"@action": "invoke",
				"params": {
					"message": "Hello, World!"
				}
			}`,
			expectedResult: &ToolResult{
				Success: true,
				Data: map[string]interface{}{
					"message": "Hello, World!",
				},
			},
			expectError: false,
		},
		{
			name: "参数验证失败测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("validationTool",
					WithTool_Description("验证工具"),
					WithTool_Callback(validationCallback),
					WithString("query",
						Description("查询字符串"),
						Required(),
					),
				)
			},
			inputJSON: `{
				"tool": "validationTool",
				"@action": "invoke",
				"params": {}
			}`,
			expectedResult: &ToolResult{
				Success: false,
				Error:   "参数验证失败",
			},
			expectError: true,
		},
		{
			name: "工具名称不匹配测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("nameTool",
					WithTool_Description("名称工具"),
					WithTool_Callback(echoCallback),
				)
			},
			inputJSON: `{
				"tool": "wrongName",
				"@action": "invoke",
				"params": {}
			}`,
			expectedResult: &ToolResult{
				Success: false,
				Error:   "工具名称不匹配",
			},
			expectError: true,
		},
		{
			name: "回调函数错误测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("errorTool",
					WithTool_Description("错误工具"),
					WithTool_Callback(errorCallback),
				)
			},
			inputJSON: `{
				"tool": "errorTool",
				"@action": "invoke",
				"params": {}
			}`,
			expectedResult: &ToolResult{
				Success: false,
				Error:   "工具执行失败: 回调函数错误",
			},
			expectError: true,
		},
		{
			name: "JSON解析错误测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("parseTool",
					WithTool_Description("解析工具"),
					WithTool_Callback(echoCallback),
				)
			},
			inputJSON: `{
				"tool": "parseTool",
				"@action": "invoke",
				"params": {
					"invalid": json
				}
			}`,
			expectedResult: &ToolResult{
				Success: false,
				Error:   "JSON解析错误",
			},
			expectError: true,
		},
		{
			name: "复杂参数测试",
			toolBuilder: func() (*Tool, error) {
				return NewTool("complexTool",
					WithTool_Description("复杂工具"),
					WithTool_Callback(echoCallback),
					WithString("stringParam",
						Description("字符串参数"),
						Required(),
					),
					WithNumber("numberParam",
						Description("数字参数"),
						Default(42),
					),
					WithStringArray("arrayParam",
						Description("数组参数"),
						Required(),
					),
				)
			},
			inputJSON: `{
				"tool": "complexTool",
				"@action": "invoke",
				"params": {
					"stringParam": "test",
					"numberParam": 123,
					"arrayParam": ["item1", "item2"]
				}
			}`,
			expectedResult: &ToolResult{
				Success: true,
				Data: map[string]interface{}{
					"stringParam": "test",
					"numberParam": float64(123),
					"arrayParam":  []interface{}{"item1", "item2"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := tt.toolBuilder()
			if err != nil {
				t.Errorf("创建工具错误: %v", err)
				return
			}

			result, err := tool.InvokeWithJSON(tt.inputJSON)

			// 检查错误
			if tt.expectError && err == nil {
				t.Errorf("期望错误但没有得到")
				return
			} else if !tt.expectError && err != nil {
				t.Errorf("不期望错误但得到了: %v", err)
				return
			}

			// 如果不期望错误，进一步比较结果
			if !tt.expectError {
				// 检查结果的Success字段
				if result.Success != tt.expectedResult.Success {
					t.Errorf("Success = %v, want %v", result.Success, tt.expectedResult.Success)
				}

				// 对于成功的结果，比较Data
				if result.Success {
					// 从ToolExecutionResult中获取实际结果
					execResult, ok := result.Data.(*ToolExecutionResult)
					if !ok {
						t.Errorf("结果类型错误，期望 *ToolExecutionResult")
						return
					}

					// 将结果转为JSON
					resultJSON, _ := json.Marshal(execResult.Result)
					expectedJSON, _ := json.Marshal(tt.expectedResult.Data)

					// 重新解析为通用格式以进行比较
					var resultData, expectedData map[string]interface{}
					json.Unmarshal(resultJSON, &resultData)
					json.Unmarshal(expectedJSON, &expectedData)

					// 比较每个字段
					for key, expectedVal := range expectedData {
						if resultVal, ok := resultData[key]; !ok {
							t.Errorf("结果缺少字段 %s", key)
						} else {
							resultValStr, _ := json.Marshal(resultVal)
							expectedValStr, _ := json.Marshal(expectedVal)
							if string(resultValStr) != string(expectedValStr) {
								t.Errorf("字段 %s 的值 = %v, want %v", key, string(resultValStr), string(expectedValStr))
							}
						}
					}
				}
			}
		})
	}
}

// TestNewToolFromJSON 测试从JSON定义创建工具
func TestNewToolFromJSON(t *testing.T) {
	// 工具定义JSON
	toolDefJSON := `{
		"name": "jsonDefinedTool",
		"description": "从JSON定义创建的工具",
		"params": [
			{
				"name": "query",
				"type": "string",
				"description": "查询字符串",
				"required": true
			},
			{
				"name": "limit",
				"type": "number",
				"description": "结果限制",
				"default": 10
			},
			{
				"name": "tags",
				"type": "array",
				"description": "标签列表",
				"items": {
					"type": "string",
					"description": "标签"
				}
			}
		]
	}`

	// 创建一个简单回调
	callback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stdout, "执行JSON定义的工具\n")
		return params, nil
	}

	// 从JSON创建工具
	tool, err := NewToolFromJSON(toolDefJSON, callback)
	if err != nil {
		t.Errorf("从JSON创建工具失败: %v", err)
		return
	}

	// 验证工具基本属性
	if tool.Name != "jsonDefinedTool" {
		t.Errorf("工具名称 = %s, want %s", tool.Name, "jsonDefinedTool")
	}

	if tool.Description != "从JSON定义创建的工具" {
		t.Errorf("工具描述 = %s, want %s", tool.Description, "从JSON定义创建的工具")
	}

	params := tool.Params()
	if len(params) != 3 {
		t.Errorf("参数数量 = %d, want %d", len(params), 3)
		return
	}

	// 验证第一个参数
	query, ok := params["query"].(map[string]any)
	require.True(t, ok, "query 参数不存在")
	require.Equal(t, query["type"], "string", "query 参数类型错误")

	// 验证第二个参数
	limit, ok := params["limit"].(map[string]any)
	require.True(t, ok, "limit 参数不存在")
	require.Equal(t, limit["type"], "number", "limit 参数类型错误")
	require.Equal(t, limit["default"], float64(10), "limit 参数 default 错误")

	// 验证第三个参数（数组类型）
	tags, ok := params["tags"].(map[string]any)
	require.True(t, ok, "tags 参数不存在")
	require.Equal(t, tags["type"], "array", "tags 参数类型错误")
	tagItems, ok := tags["items"].(map[string]any)
	require.True(t, ok, "tags 参数数组项不存在")
	require.Equal(t, tagItems["type"], "string", "tags 参数数组项类型错误")
	require.Equal(t, tagItems["description"], "标签", "tags 参数数组项描述错误")

	// 测试使用从JSON创建的工具进行调用
	inputJSON := `{
		"tool": "jsonDefinedTool",
		"@action": "invoke",
		"params": {
			"query": "test query",
			"tags": ["tag1", "tag2"]
		}
	}`

	result, err := tool.InvokeWithJSON(inputJSON)
	if err != nil {
		t.Errorf("调用工具失败: %v", err)
		return
	}

	if !result.Success {
		t.Errorf("工具调用不成功: %s", result.Error)
	}

	// 验证工具调用结果包含所有参数
	execResult, ok := result.Data.(*ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误, want map[string]interface{}")
		return
	}

	// 验证参数值
	if query, ok := resultData["query"]; !ok || query != "test query" {
		t.Errorf("query 参数 = %v, want %s", query, "test query")
	}

	// limit 应使用默认值
	if limit, ok := resultData["limit"]; !ok || limit != float64(10) {
		t.Errorf("limit 参数 = %v, want %v", limit, float64(10))
	}

	// 验证 tags 数组
	if tags, ok := resultData["tags"]; !ok {
		t.Errorf("tags 参数缺失")
	} else {
		tagsArray, ok := tags.([]interface{})
		if !ok {
			t.Errorf("tags 参数类型错误, want []interface{}")
		} else if len(tagsArray) != 2 || tagsArray[0] != "tag1" || tagsArray[1] != "tag2" {
			t.Errorf("tags 参数值 = %v, want %v", tagsArray, []string{"tag1", "tag2"})
		}
	}
}
