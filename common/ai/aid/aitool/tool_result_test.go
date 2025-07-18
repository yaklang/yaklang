package aitool

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/context"
	"io"
	"strings"
	"testing"
)

// TestToolExecutionResult 测试工具执行结果
func TestToolExecutionResult(t *testing.T) {
	// 创建工具执行结果
	execResult := &ToolExecutionResult{
		Stdout: "标准输出内容",
		Stderr: "标准错误输出内容",
		Result: map[string]interface{}{
			"key": "value",
		},
	}

	// 测试转换为JSON
	jsonStr, err := execResult.ToJSON()
	if err != nil {
		t.Errorf("转换为JSON失败: %v", err)
	}

	// 验证JSON内容
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		t.Errorf("解析JSON失败: %v", err)
	}

	// 验证字段
	if stdout, ok := jsonData["stdout"].(string); !ok || stdout != "标准输出内容" {
		t.Errorf("stdout = %v, want %v", jsonData["stdout"], "标准输出内容")
	}

	if stderr, ok := jsonData["stderr"].(string); !ok || stderr != "标准错误输出内容" {
		t.Errorf("stderr = %v, want %v", jsonData["stderr"], "标准错误输出内容")
	}

	if result, ok := jsonData["result"].(map[string]interface{}); !ok || result["key"] != "value" {
		t.Errorf("result = %v, want %v", jsonData["result"], map[string]interface{}{"key": "value"})
	}

	// 测试JSON Schema生成
	schemaStr := execResult.GetJSONSchemaString()
	if !strings.Contains(schemaStr, "properties") ||
		!strings.Contains(schemaStr, "stdout") ||
		!strings.Contains(schemaStr, "stderr") ||
		!strings.Contains(schemaStr, "result") {
		t.Errorf("JSON Schema 不包含必要字段")
	}

	// 测试结果验证
	valid, errors := ValidateResult(jsonStr)
	if !valid {
		t.Errorf("结果验证失败: %v", errors)
	}

	// 测试无效结果验证
	invalidJSON := `{"stdout": 123, "stderr": "错误", "result": null}`
	valid, errors = ValidateResult(invalidJSON)
	if valid {
		t.Errorf("期望无效结果验证失败，但成功了")
	}
}

// TestExecuteToolWithCapture 测试带捕获的工具执行
func TestExecuteToolWithCapture(t *testing.T) {
	// 创建回调函数
	callback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		// 写入标准输出
		fmt.Fprintf(stdout, "命令: %s\n", params["command"])
		fmt.Fprintf(stdout, "执行成功\n")

		// 写入标准错误
		if warning, ok := params["warning"]; ok && warning.(bool) {
			fmt.Fprintf(stderr, "警告: 这是一个测试警告\n")
		}

		return map[string]interface{}{
			"status": "success",
			"code":   0,
		}, nil
	}

	// 创建工具
	tool, err := New("captureTest",
		WithDescription("测试捕获输出的工具"),
		WithSimpleCallback(callback),
		WithStringParam("command",
			WithParam_Description("要执行的命令"),
			WithParam_Required(),
		),
		WithBoolParam("warning",
			WithParam_Description("是否显示警告"),
			WithParam_Default(false),
		),
	)

	if err != nil {
		t.Errorf("创建工具失败: %v", err)
		return
	}

	// 测试情况1: 无警告
	params1 := map[string]interface{}{
		"command": "test",
		"warning": false,
	}

	result1, err := tool.ExecuteToolWithCapture(context.Background(), params1, &ToolInvokeConfig{})
	if err != nil {
		t.Errorf("执行工具失败: %v", err)
	}

	if !strings.Contains(result1.Stdout, "命令: test") || !strings.Contains(result1.Stdout, "执行成功") {
		t.Errorf("标准输出内容不正确: %s", result1.Stdout)
	}

	if result1.Stderr != "" {
		t.Errorf("标准错误应为空，但得到: %s", result1.Stderr)
	}

	// 测试情况2: 有警告
	params2 := map[string]interface{}{
		"command": "test-warning",
		"warning": true,
	}

	result2, err := tool.ExecuteToolWithCapture(context.Background(), params2, &ToolInvokeConfig{})
	if err != nil {
		t.Errorf("执行工具失败: %v", err)
	}

	if !strings.Contains(result2.Stdout, "命令: test-warning") {
		t.Errorf("标准输出内容不正确: %s", result2.Stdout)
	}

	if !strings.Contains(result2.Stderr, "警告") {
		t.Errorf("标准错误内容不正确: %s", result2.Stderr)
	}

	// 验证结果包含预期数据
	resultData, ok := result2.Result.(map[string]interface{})
	if !ok {
		t.Errorf("结果类型错误")
	} else {
		if status, ok := resultData["status"]; !ok || status != "success" {
			t.Errorf("结果状态不正确: %v", status)
		}

		if code, ok := resultData["code"]; !ok || code != 0 {
			t.Errorf("结果代码不正确: %v", code)
		}
	}
}

// TestToolResultIntegration 测试通过 InvokeWithParams 集成 ToolResult 和 ToolExecutionResult
func TestToolResultIntegration(t *testing.T) {
	// 创建回调函数
	callback := func(params InvokeParams, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		fmt.Fprintf(stdout, "处理参数: %v\n", params)
		return params, nil
	}

	// 创建工具
	tool, err := New("integrationTest",
		WithDescription("集成测试工具"),
		WithSimpleCallback(callback),
		WithStringParam("input",
			WithParam_Description("输入值"),
			WithParam_Required(),
		),
	)

	if err != nil {
		t.Errorf("创建工具失败: %v", err)
		return
	}

	// 调用工具
	params := map[string]interface{}{
		"input": "test-integration",
	}

	result, err := tool.InvokeWithParams(params)
	if err != nil {
		t.Errorf("调用工具失败: %v", err)
	}

	// 验证结果成功
	if !result.Success {
		t.Errorf("调用不成功: %s", result.Error)
		return
	}

	// 验证结果包含 ToolExecutionResult
	execResult, ok := result.Data.(*ToolExecutionResult)
	if !ok {
		t.Errorf("结果类型错误，期望 *ToolExecutionResult")
		return
	}

	// 验证 stdout 被捕获
	if !strings.Contains(execResult.Stdout, "处理参数") {
		t.Errorf("stdout 内容不正确: %s", execResult.Stdout)
	}

	// 验证结果数据
	resultData, ok := execResult.Result.(InvokeParams)
	if !ok {
		t.Errorf("结果数据类型错误")
		return
	}

	if input, ok := resultData["input"]; !ok || input != "test-integration" {
		t.Errorf("结果数据不正确: %v", resultData)
	}
}
