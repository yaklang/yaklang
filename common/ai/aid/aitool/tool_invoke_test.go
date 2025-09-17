package aitool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestToolInvoke 测试工具调用
func TestToolInvoke(t *testing.T) {
	// 创建一个简单的回调函数
	echoCallback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stdout, "执行工具：%s\n", "echo")
		return params, nil
	}

	// 创建一个带错误的回调函数
	errorCallback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stderr, "工具执行错误\n")
		return nil, errors.New("回调函数错误")
	}

	// 创建一个带参数验证的回调函数
	validationCallback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
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
				return New("echoTool",
					WithDescription("回显工具"),
					WithSimpleCallback(echoCallback),
					WithStringParam("message",
						WithParam_Description("要回显的消息"),
						WithParam_Required(),
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
				return New("validationTool",
					WithDescription("验证工具"),
					WithSimpleCallback(validationCallback),
					WithStringParam("query",
						WithParam_Description("查询字符串"),
						WithParam_Required(),
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
				return New("nameTool",
					WithDescription("名称工具"),
					WithSimpleCallback(echoCallback),
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
				return New("errorTool",
					WithDescription("错误工具"),
					WithSimpleCallback(errorCallback),
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
				return New("parseTool",
					WithDescription("解析工具"),
					WithSimpleCallback(echoCallback),
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
				return New("complexTool",
					WithDescription("复杂工具"),
					WithSimpleCallback(echoCallback),
					WithStringParam("stringParam",
						WithParam_Description("字符串参数"),
						WithParam_Required(),
					),
					WithNumberParam("numberParam",
						WithParam_Description("数字参数"),
						WithParam_Default(42),
					),
					WithStringArrayParam("arrayParam",
						WithParam_Description("数组参数"),
						WithParam_Required(),
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

func TestToolApplyDefault(t *testing.T) {
	echoCallback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stdout, "执行JSON定义的工具\n")
		return params, nil
	}
	checkWithCallback := func(t *testing.T, toolDefJSON string, params map[string]any, callback func(params map[string]any)) {
		tool, err := NewToolFromJSON(toolDefJSON, echoCallback)
		require.NoError(t, err, "从JSON创建工具失败")

		result, err := tool.InvokeWithParams(params)
		require.NoError(t, err, "工具调用失败")
		require.True(t, result.Success, "工具调用不成功")

		// 验证工具调用结果包含所有参数
		execResult, ok := result.Data.(*ToolExecutionResult)
		require.True(t, ok, "结果类型错误，期望 *ToolExecutionResult")

		resultData, ok := execResult.Result.(InvokeParams)
		require.True(t, ok, "结果数据类型错误, want InvokeParams")
		callback(resultData)
	}

	t.Run("string", func(t *testing.T) {
		toolDefJSON := `{
			"name": "testTool",
			"description": "测试工具",
			"params": [
				{
					"name": "query",
					"type": "string",
					"default": "defaultQuery"
				}
			]
		}`
		checkWithCallback(t, toolDefJSON, map[string]any{}, func(params map[string]any) {
			require.Equal(t, "defaultQuery", params["query"], "默认值测试失败")
		})
	})

	t.Run("object", func(t *testing.T) {
		toolDefJSON := `{
			"name": "testTool",
			"description": "测试工具",
			"params": [
				{
					"name": "o",
					"type": "object",
					"properties": {
						"query": {
							"type": "string",
							"default": "defaultQuery"
						}
					}
				}
			]
		}`
		checkWithCallback(t, toolDefJSON, map[string]any{
			"o": map[string]any{},
		}, func(params map[string]any) {
			o, ok := params["o"].(map[string]any)
			require.True(t, ok, "o 参数不存在")
			require.Equal(t, "defaultQuery", o["query"], "默认值测试失败")
		})
	})

	t.Run("array", func(t *testing.T) {
		toolDefJSON := `{
			"name": "testTool",
			"description": "测试工具",
			"params": [
				{
					"name": "arrayParam",
					"type": "array",
					"default": ["defaultItem"]
				}
			]
		}`
		checkWithCallback(t, toolDefJSON, map[string]any{}, func(params map[string]any) {
			array, ok := params["arrayParam"].([]any)
			require.True(t, ok, "arrayParam 参数不存在")
			require.Equal(t, []any{"defaultItem"}, array, "默认值测试失败")
		})
	})

	t.Run("array_object", func(t *testing.T) {
		toolDefJSON := `{
			"name": "testTool",
			"description": "测试工具",
			"params": [
				{
					"name": "arrayParam",
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"query": {
								"type": "string",
								"default": "defaultItem"
							}
						}
					}
				}
			]
		}`
		checkWithCallback(t, toolDefJSON, map[string]any{
			"arrayParam": []any{map[string]any{}},
		}, func(params map[string]any) {
			array, ok := params["arrayParam"].([]any)
			require.True(t, ok, "arrayParam 参数不存在")
			require.Equal(t, []any{map[string]any{"query": "defaultItem"}}, array, "默认值测试失败")
		})
	})

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
	callback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
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
	if params.Len() != 3 {
		t.Errorf("参数数量 = %d, want %d", params.Len(), 3)
		return
	}

	// 验证第一个参数
	queryVal, ok := params.Get("query")
	require.True(t, ok, "query 参数不存在")
	query, ok := queryVal.(map[string]any)
	require.True(t, ok, "query 参数类型错误")
	require.Equal(t, query["type"], "string", "query 参数类型错误")

	// 验证第二个参数
	limitVal, ok := params.Get("limit")
	require.True(t, ok, "limit 参数不存在")
	limit, ok := limitVal.(map[string]any)
	require.True(t, ok, "limit 参数类型错误")
	require.Equal(t, limit["type"], "number", "limit 参数类型错误")
	require.Equal(t, limit["default"], float64(10), "limit 参数 default 错误")

	// 验证第三个参数（数组类型）
	tagsVal, ok := params.Get("tags")
	require.True(t, ok, "tags 参数不存在")
	tags, ok := tagsVal.(map[string]any)
	require.True(t, ok, "tags 参数类型错误")
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

	resultData, ok := execResult.Result.(InvokeParams)
	if !ok {
		t.Errorf("结果数据类型错误, want InvokeParams")
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

// TestHandleLargeContent 测试大文本内容处理功能
func TestHandleLargeContent(t *testing.T) {
	t.Run("处理小文本内容", func(t *testing.T) {
		// 创建一个小于10KB的文本
		smallContent := strings.Repeat("a", 1024*5) // 5KB
		originalContent := smallContent

		// 调用处理函数
		handleLargeContent(&smallContent, "test", nil)

		// 验证内容没有被修改
		require.Equal(t, originalContent, smallContent, "小于10KB的内容不应被修改")
	})

	t.Run("处理大文本内容", func(t *testing.T) {
		// 创建一个大于10KB的文本
		largeContent := strings.Repeat("b", 1024*15) // 15KB
		originalContent := largeContent

		// 调用处理函数
		handleLargeContent(&largeContent, "test", nil)

		// 验证内容已被截断
		require.NotEqual(t, originalContent, largeContent, "大于10KB的内容应被修改")
		require.Contains(t, largeContent, "saved in file", "应包含文件保存信息")
		require.True(t, len(largeContent) < len(originalContent), "内容应被截断")
	})

	t.Run("测试回调函数", func(t *testing.T) {
		// 创建一个大于10KB的文本
		largeContent := strings.Repeat("c", 1024*15) // 15KB

		// 回调函数验证
		callbackCalled := false
		var savedFilename string

		handleLargeContent(&largeContent, "test", func(filename string) {
			callbackCalled = true
			savedFilename = filename
		})

		// 验证回调被调用
		require.True(t, callbackCalled, "回调函数应被调用")
		require.NotEmpty(t, savedFilename, "文件名不应为空")
	})

	t.Run("测试文件保存功能", func(t *testing.T) {
		// 创建测试内容
		testContent := strings.Repeat("d", 100)

		// 调用文件保存函数
		filename := handleLargeContentToFile(testContent, "test")

		// 验证返回的文件名
		require.NotEmpty(t, filename, "文件名不应为空")
		require.Contains(t, filename, ".test.txt", "文件名应包含内容类型")
	})
}

// TestInvokeWithParamsLargeContent 测试InvokeWithParams处理大内容的功能
func TestInvokeWithParamsLargeContent(t *testing.T) {
	// 创建一个会生成大内容的回调函数
	largeContentCallback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		// 向标准输出写入大于10KB的内容
		largeStdout := strings.Repeat("A", 12*1024)
		fmt.Fprint(stdout, largeStdout)

		// 向标准错误写入大于10KB的内容
		largeStderr := strings.Repeat("B", 11*1024)
		fmt.Fprint(stderr, largeStderr)

		// 返回大JSON结果
		return map[string]interface{}{
			"largeResult": strings.Repeat("C", 13*1024),
		}, nil
	}

	// 创建测试工具
	tool, err := New("largeContentTool",
		WithDescription("生成大内容的工具"),
		WithSimpleCallback(largeContentCallback),
	)
	require.NoError(t, err, "创建工具失败")

	// 调用工具
	result, err := tool.InvokeWithParams(map[string]any{})

	// 验证结果
	require.NoError(t, err, "调用工具失败")
	require.True(t, result.Success, "工具调用不成功")

	// 验证执行结果
	execResult, ok := result.Data.(*ToolExecutionResult)
	require.True(t, ok, "结果类型错误，期望 *ToolExecutionResult")

	// 验证标准输出被截断和保存
	require.Less(t, len(execResult.Stdout), 12*1024, "标准输出应被截断")
	require.Contains(t, execResult.Stdout, "saved in file", "标准输出应包含文件保存信息")

	// 验证标准错误被截断和保存
	require.Less(t, len(execResult.Stderr), 11*1024, "标准错误应被截断")
	require.Contains(t, execResult.Stderr, "saved in file", "标准错误应包含文件保存信息")

	// 验证JSON结果处理
	resultStr, ok := execResult.Result.(string)
	require.True(t, ok, "JSON结果类型错误，应为字符串")
	require.Contains(t, resultStr, "saved in file", "JSON结果应包含文件保存信息")
}
