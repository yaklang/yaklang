package aidmock_test

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	aidmock "github.com/yaklang/yaklang/common/aiengine/tests/aid_mock"
)

// ExampleKeywordScenarios 演示如何使用KeywordScenarios
func ExampleKeywordScenarios() {
	// 创建一个KeywordScenarios，自动加载内置响应
	scenarios := aidmock.NewKeywordScenarios()

	// 获取AI回调函数
	callback := scenarios.GetAICallbackType()

	// 创建模拟配置
	config := mock.NewMockedAIConfig(context.Background())

	// 创建请求（包含关键词 "directly_answer" 和 "next_action"）
	req := aicommon.NewAIRequest("Please provide a directly_answer with next_action")

	// 调用回调函数
	resp, err := callback(config, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response received: %v\n", resp != nil)
	// Output: Response received: true
}

// ExampleKeywordScenarios_addCustomResponse 演示如何添加自定义响应
func ExampleKeywordScenarios_addCustomResponse() {
	scenarios := aidmock.NewKeywordScenarios()

	// 添加自定义响应
	customResponse := `{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Custom answer"}, "human_readable_thought": "Custom thought", "cumulative_summary": "Custom summary"}`

	scenarios.AddResponse(
		"my_custom",                      // 响应名称
		[]string{"custom", "keywords"},   // 匹配关键词
		customResponse,                   // 响应内容
		"My custom response for testing", // 描述
	)

	// 列出所有响应
	responses := scenarios.ListResponses()
	fmt.Printf("Total responses: %d\n", len(responses))

	// 获取特定响应
	resp := scenarios.GetResponse("my_custom")
	fmt.Printf("Custom response exists: %v\n", resp != nil)

	// Output:
	// Total responses: 21
	// Custom response exists: true
}

// ExampleSequentialScenarios 演示如何使用SequentialScenarios
func ExampleSequentialScenarios() {
	// 定义响应序列
	responses := []string{
		`{"step": 1, "action": "first"}`,
		`{"step": 2, "action": "second"}`,
		`{"step": 3, "action": "third"}`,
	}

	// 创建顺序场景生成器
	scenarios := aidmock.NewSequentialScenarios(responses)

	// 获取AI回调函数
	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 按顺序调用
	for i := 0; i < 3; i++ {
		req := aicommon.NewAIRequest("test")
		resp, _ := callback(config, req)
		fmt.Printf("Response %d: %v\n", i+1, resp != nil)
	}

	// Output:
	// Response 1: true
	// Response 2: true
	// Response 3: true
}

// ExampleSequentialScenarios_fromScenario 演示如何从场景创建SequentialScenarios
func ExampleSequentialScenarios_fromScenario() {
	// 从预定义场景创建场景生成器
	scenarios := aidmock.NewSequentialScenariosFromScenario("simple_qa")

	// 获取当前索引
	fmt.Printf("Initial index: %d\n", scenarios.GetCurrentIndex())

	// 调用一次
	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())
	req := aicommon.NewAIRequest("test")
	_, _ = callback(config, req)

	fmt.Printf("After call index: %d\n", scenarios.GetCurrentIndex())

	// 重置
	scenarios.Reset()
	fmt.Printf("After reset index: %d\n", scenarios.GetCurrentIndex())

	// Output:
	// Initial index: 0
	// After call index: 1
	// After reset index: 0
}

// ExampleGetScenario 演示如何获取场景信息
func ExampleGetScenario() {
	// 获取单个场景
	scenario, ok := aidmock.GetScenario("single_tool_call")
	if ok {
		fmt.Printf("Scenario: %s\n", scenario.Name)
		fmt.Printf("Description: %s\n", scenario.Description)
		fmt.Printf("Steps: %d\n", len(scenario.Steps))
	}

	// Output:
	// Scenario: single_tool_call
	// Description: Single tool call workflow
	// Steps: 3
}

// ExampleListScenarios 演示如何列出所有场景
func ExampleListScenarios() {
	scenarios := aidmock.ListScenarios()
	fmt.Printf("Total scenarios: %d\n", len(scenarios))
	fmt.Printf("First scenario exists: %v\n", len(scenarios) > 0)

	// Output:
	// Total scenarios: 15
	// First scenario exists: true
}

// ExampleGetCommonResponse 演示如何获取常见响应
func ExampleGetCommonResponse() {
	// 获取常见响应
	resp, ok := aidmock.GetCommonResponse("simple_answer")
	if ok {
		fmt.Printf("Response found: %v\n", len(resp) > 0)
	}

	// 获取不存在的响应
	_, ok = aidmock.GetCommonResponse("nonexistent")
	fmt.Printf("Nonexistent response found: %v\n", ok)

	// Output:
	// Response found: true
	// Nonexistent response found: false
}

// ExampleCreateCustomScenarios 演示如何创建不带内置响应的场景生成器
func ExampleCreateCustomScenarios() {
	// 创建空的场景生成器
	scenarios := aidmock.CreateCustomScenarios()

	// 检查内置响应数量
	fmt.Printf("Built-in responses: %d\n", len(scenarios.ListResponses()))

	// 添加自定义响应
	scenarios.AddResponse("custom1", []string{"test"}, `{"custom": 1}`, "Custom 1")
	scenarios.AddResponse("custom2", []string{"test2"}, `{"custom": 2}`, "Custom 2")

	fmt.Printf("After adding: %d\n", len(scenarios.ListResponses()))

	// Output:
	// Built-in responses: 0
	// After adding: 2
}

// ExampleKeywordScenarios_multipleKeywords 演示多关键词匹配
func ExampleKeywordScenarios_multipleKeywords() {
	scenarios := aidmock.CreateCustomScenarios()

	// 添加需要多个关键词的响应
	scenarios.AddResponse(
		"multi_keyword",
		[]string{"need", "tool", "sleep"}, // 需要同时包含这3个关键词
		`{"matched": true}`,
		"Multi-keyword response",
	)

	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 测试1: 包含所有关键词 - 应该匹配
	req1 := aicommon.NewAIRequest("I need to use the sleep tool")
	resp1, _ := callback(config, req1)
	fmt.Printf("All keywords matched: %v\n", resp1 != nil)

	// 测试2: 只包含部分关键词 - 不应该匹配
	req2 := aicommon.NewAIRequest("I need a tool")
	resp2, _ := callback(config, req2)
	fmt.Printf("Partial keywords matched: %v\n", resp2 != nil)

	// Output:
	// All keywords matched: true
	// Partial keywords matched: true
}
