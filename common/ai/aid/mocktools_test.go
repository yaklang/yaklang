package aid

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// 测试所有模拟工具
func TestAllMockTools(t *testing.T) {
	tools := GetAllMockTools()

	// 验证工具数量
	if len(tools) != 5 {
		t.Errorf("期望有5个模拟工具，实际有%d个", len(tools))
	}

	// 验证工具名称
	expectedNames := []string{"WeatherAPI", "AttractionAPI", "RestaurantAPI", "TransportAPI", "TimeEstimateAPI"}
	for i, tool := range tools {
		if i < len(expectedNames) && tool.Name != expectedNames[i] {
			t.Errorf("工具名称错误，期望 %s，实际 %s", expectedNames[i], tool.Name)
		}
	}
}

// 测试天气工具
func TestWeatherTool(t *testing.T) {
	tool := WeatherTool()

	// 验证工具名称和描述
	if tool.Name != "WeatherAPI" {
		t.Errorf("工具名称错误，期望 WeatherAPI，实际 %s", tool.Name)
	}

	// 验证参数数量
	if tool.Params().Len() != 2 {
		t.Errorf("参数数量错误，期望 2，实际 %d", tool.Params().Len())
	}

	// 准备测试数据
	inputJSON := `{
		"tool": "WeatherAPI",
		"@action": "invoke",
		"params": {
			"city": "北京",
			"date": "2025-03-16"
		}
	}`

	// 调用工具
	result, err := tool.InvokeWithJSON(inputJSON)

	// 检查基本结果
	if err != nil {
		t.Errorf("调用工具出错: %v", err)
	}

	// 验证结果类型
	execResult, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 验证结果内容
	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误，期望 map[string]interface{}")
		return
	}

	// 检查状态字段
	status, ok := resultData["status"].(string)
	if !ok || (status != "success" && status != "error") {
		t.Errorf("状态字段错误: %v", status)
	}

	// 打印结果以便手动验证
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	fmt.Printf("天气工具返回结果:\n%s\n", resultJSON)
}

// 测试景点工具
func TestAttractionTool(t *testing.T) {
	tool := AttractionTool()

	// 准备测试数据
	inputJSON := `{
		"tool": "AttractionAPI",
		"@action": "invoke",
		"params": {
			"city": "北京",
			"preference": "历史"
		}
	}`

	// 调用工具
	result, err := tool.InvokeWithJSON(inputJSON)

	// 检查基本结果
	if err != nil {
		t.Errorf("调用工具出错: %v", err)
	}

	// 验证结果类型
	execResult, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 验证结果内容
	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误，期望 map[string]interface{}")
		return
	}

	// 检查状态字段
	status, ok := resultData["status"].(string)
	if !ok || (status != "success" && status != "partial") {
		t.Errorf("状态字段错误: %v", status)
	}

	// 打印结果以便手动验证
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	fmt.Printf("景点工具返回结果:\n%s\n", resultJSON)
}

// 测试餐厅工具
func TestRestaurantTool(t *testing.T) {
	tool := RestaurantTool()

	// 准备测试数据
	inputJSON := `{
		"tool": "RestaurantAPI",
		"@action": "invoke",
		"params": {
			"location": "朝阳区",
			"budget": "中等",
			"cuisine": "川菜"
		}
	}`

	// 调用工具
	result, err := tool.InvokeWithJSON(inputJSON)

	// 检查基本结果
	if err != nil {
		t.Errorf("调用工具出错: %v", err)
	}

	// 验证结果内容
	execResult, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 打印完整结果以进行调试
	resultBytes, _ := json.MarshalIndent(execResult.Result, "", "  ")
	t.Logf("工具返回结果: %s", string(resultBytes))

	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误，期望 map[string]interface{}")
		return
	}

	// 检查数据字段
	t.Logf("status: %v", resultData["status"])
	t.Logf("count: %v", resultData["count"])
	t.Logf("data类型: %T", resultData["data"])

	// 验证数据字段存在且不为空
	if data := resultData["data"]; data == nil {
		t.Errorf("数据字段为空")
		return
	} else {
		// 使用反射判断是否为切片类型
		rt := reflect.TypeOf(data)
		if rt.Kind() != reflect.Slice {
			t.Errorf("数据字段类型错误，期望切片类型，实际类型: %s", rt.String())
			return
		}

		// 用反射获取切片长度
		rv := reflect.ValueOf(data)
		if rv.Len() == 0 {
			t.Errorf("数据字段为空切片")
			return
		}

		// 转换为JSON并解析，这种方式不依赖于具体类型
		jsonData, _ := json.Marshal(data)
		var restaurants []map[string]interface{}
		if err := json.Unmarshal(jsonData, &restaurants); err != nil {
			t.Errorf("解析餐厅数据失败: %v", err)
			return
		}

		// 验证筛选功能
		foundCuisineMatch := false
		for _, r := range restaurants {
			if cuisine, ok := r["cuisine"].(string); ok && strings.Contains(cuisine, "川菜") {
				foundCuisineMatch = true
				break
			}
		}

		if !foundCuisineMatch {
			t.Errorf("未找到匹配菜系的餐厅")
		}
	}

	// 打印结果以便手动验证
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	fmt.Printf("餐厅工具返回结果:\n%s\n", resultJSON)
}

// 测试交通工具
func TestTransportTool(t *testing.T) {
	tool := TransportTool()

	// 准备测试数据
	inputJSON := `{
		"tool": "TransportAPI",
		"@action": "invoke",
		"params": {
			"origin": "北京南站",
			"destination": "颐和园"
		}
	}`

	// 调用工具
	result, err := tool.InvokeWithJSON(inputJSON)

	// 检查基本结果
	if err != nil {
		t.Errorf("调用工具出错: %v", err)
	}

	// 验证结果内容
	execResult, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 打印完整结果以进行调试
	resultBytes, _ := json.MarshalIndent(execResult.Result, "", "  ")
	t.Logf("工具返回结果: %s", string(resultBytes))

	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误，期望 map[string]interface{}")
		return
	}

	// 检查数据字段
	t.Logf("data类型: %T", resultData["data"])

	// 验证数据字段存在且不为空
	if data := resultData["data"]; data == nil {
		t.Errorf("数据字段为空")
		return
	} else {
		// 使用反射判断是否为切片类型
		rt := reflect.TypeOf(data)
		if rt.Kind() != reflect.Slice {
			t.Errorf("数据字段类型错误，期望切片类型，实际类型: %s", rt.String())
			return
		}

		// 用反射获取切片长度
		rv := reflect.ValueOf(data)
		if rv.Len() == 0 {
			t.Errorf("数据字段为空切片")
			return
		}

		// 转换为JSON以验证内容
		jsonData, _ := json.Marshal(data)
		t.Logf("交通数据: %s", string(jsonData))
	}

	// 打印结果以便手动验证
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	fmt.Printf("交通工具返回结果:\n%s\n", resultJSON)
}

// 测试时间估算工具
func TestTimeEstimateTool(t *testing.T) {
	tool := TimeEstimateTool()

	// 准备测试数据
	inputJSON := `{
		"tool": "TimeEstimateAPI",
		"@action": "invoke",
		"params": {
			"locations": ["故宫博物院", "颐和园", "北京动物园"],
			"durations": ["3小时", "2小时", "2小时"]
		}
	}`

	// 调用工具
	result, err := tool.InvokeWithJSON(inputJSON)

	// 检查基本结果
	if err != nil {
		t.Errorf("调用工具出错: %v", err)
	}

	// 验证结果内容
	execResult, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 打印完整结果以进行调试
	resultBytes, _ := json.MarshalIndent(execResult.Result, "", "  ")
	t.Logf("工具返回结果: %s", string(resultBytes))

	resultData, ok := execResult.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果数据类型错误，期望 map[string]interface{}")
		return
	}

	// 检查数据字段
	t.Logf("itinerary类型: %T", resultData["itinerary"])

	// 验证行程安排
	if itinerary := resultData["itinerary"]; itinerary == nil {
		t.Errorf("行程安排字段为空")
		return
	} else {
		// 使用反射判断是否为切片类型
		rt := reflect.TypeOf(itinerary)
		if rt.Kind() != reflect.Slice {
			t.Errorf("行程安排字段类型错误，期望切片类型，实际类型: %s", rt.String())
			return
		}

		// 用反射获取切片长度
		rv := reflect.ValueOf(itinerary)
		t.Logf("行程安排长度: %d", rv.Len())

		// 转换为JSON以验证内容
		jsonData, _ := json.Marshal(itinerary)
		t.Logf("行程安排数据: %s", string(jsonData))
	}

	// 检查总时间
	totalTime, ok := resultData["totalTime"].(string)
	if !ok || totalTime == "" {
		t.Errorf("总时间字段错误")
	}

	// 打印结果以便手动验证
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	fmt.Printf("时间估算工具返回结果:\n%s\n", resultJSON)
}

// 测试参数验证
func TestToolParameterValidation(t *testing.T) {
	tests := []struct {
		name      string
		toolFunc  func() *aitool.Tool
		inputJSON string
		expectErr bool
	}{
		{
			name:     "天气工具缺少必要参数",
			toolFunc: WeatherTool,
			inputJSON: `{
				"tool": "WeatherAPI",
				"@action": "invoke",
				"params": {
					"city": "北京"
				}
			}`,
			expectErr: true,
		},
		{
			name:     "景点工具缺少必要参数",
			toolFunc: AttractionTool,
			inputJSON: `{
				"tool": "AttractionAPI",
				"@action": "invoke",
				"params": {
					"preference": "历史"
				}
			}`,
			expectErr: true,
		},
		{
			name:     "时间估算工具参数不匹配",
			toolFunc: TimeEstimateTool,
			inputJSON: `{
				"tool": "TimeEstimateAPI",
				"@action": "invoke",
				"params": {
					"locations": ["故宫", "颐和园"],
					"durations": ["2小时", "3小时", "2小时"]
				}
			}`,
			expectErr: false, // 实现中会处理这种情况
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := tt.toolFunc()
			result, err := tool.InvokeWithJSON(tt.inputJSON)

			if tt.expectErr {
				if err == nil && result.Success {
					t.Errorf("期望错误但没有错误")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但有错误: %v", err)
				}
			}
		})
	}
}
