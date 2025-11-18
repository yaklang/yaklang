package aidmock_test

import (
	"fmt"

	aidmock "github.com/yaklang/yaklang/common/aiengine/tests/aid_mock"
)

// ExampleBuildDirectlyAnswer 演示如何构建直接回答响应
func ExampleBuildDirectlyAnswer() {
	response := aidmock.BuildDirectlyAnswer(
		"这是答案内容",
		"正在思考如何回答",
		"问题已回答",
	)

	// 验证响应格式
	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleBuildRequireTool 演示如何构建工具请求
func ExampleBuildRequireTool() {
	// 请求使用sleep工具
	response := aidmock.BuildRequireTool(
		"sleep",
		"需要暂停一下",
		"请求sleep工具",
	)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleBuildSleepToolParams 演示如何构建sleep工具参数
func ExampleBuildSleepToolParams() {
	// 构建sleep 0.5秒的参数
	response := aidmock.BuildSleepToolParams(0.5)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleBuildRiskToolParams 演示如何构建风险创建参数
func ExampleBuildRiskToolParams() {
	response := aidmock.BuildRiskToolParams(
		"http://example.com",  // target
		"SQL注入漏洞",          // title
		"high",                 // severity
		"sqli",                 // risk type
	)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleReactResponseBuilder_directlyAnswer 演示使用链式调用构建直接回答
func ExampleReactResponseBuilder_directlyAnswer() {
	response := aidmock.NewReactResponseBuilder().
		WithDirectlyAnswer("自定义答案").
		WithHumanThought("自定义思考过程").
		WithCumulativeSummary("自定义总结").
		Build()

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleReactResponseBuilder_askForClarification 演示构建澄清请求
func ExampleReactResponseBuilder_askForClarification() {
	response := aidmock.NewReactResponseBuilder().
		WithAskForClarification(
			"请选择扫描模式",
			[]string{"快速扫描", "深度扫描", "自定义扫描"},
		).
		WithHumanThought("需要用户选择扫描模式").
		WithCumulativeSummary("等待用户输入").
		Build()

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleCallToolBuilder 演示构建工具调用参数
func ExampleCallToolBuilder() {
	response := aidmock.NewCallToolBuilder().
		WithParam("target", "http://example.com").
		WithParam("port", 8080).
		WithParam("timeout", 30).
		WithParam("verbose", true).
		Build()

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Response valid: %v\n", err == nil)

	// Output: Response valid: true
}

// ExampleVerifySatisfactionBuilder 演示构建满意度验证
func ExampleVerifySatisfactionBuilder() {
	// 正面响应
	positiveResponse := aidmock.NewVerifySatisfactionBuilder().
		WithSatisfied(true).
		WithReasoning("扫描已完成，发现3个漏洞").
		Build()

	// 负面响应
	negativeResponse := aidmock.NewVerifySatisfactionBuilder().
		WithSatisfied(false).
		WithReasoning("需要提供更多目标信息").
		Build()

	err1 := aidmock.ValidateResponse(positiveResponse)
	err2 := aidmock.ValidateResponse(negativeResponse)

	fmt.Printf("Positive valid: %v\n", err1 == nil)
	fmt.Printf("Negative valid: %v\n", err2 == nil)

	// Output:
	// Positive valid: true
	// Negative valid: true
}

// ExampleKeywordScenarios_builderWorkflow 演示完整的工具调用工作流
func ExampleKeywordScenarios_builderWorkflow() {
	// 创建一个自定义场景生成器
	scenarios := aidmock.CreateCustomScenarios()

	// 步骤1: 添加请求工具的响应
	scenarios.AddResponse(
		"request_scan_tool",
		[]string{"scan", "vulnerability"},
		aidmock.BuildRequireTool(
			"vulnerability_scanner",
			"需要使用漏洞扫描工具",
			"请求扫描工具",
		),
		"请求漏洞扫描工具",
	)

	// 步骤2: 添加生成工具参数的响应
	scenarios.AddResponse(
		"generate_scan_params",
		[]string{"parameters", "scan"},
		aidmock.NewCallToolBuilder().
			WithParam("target", "http://test.com").
			WithParam("scan_type", "full").
			WithParam("threads", 10).
			Build(),
		"生成扫描参数",
	)

	// 步骤3: 添加验证满意度的响应
	scenarios.AddResponse(
		"verify_scan_result",
		[]string{"verify", "result"},
		aidmock.BuildVerifySatisfaction(
			true,
			"扫描完成，发现5个漏洞",
		),
		"验证扫描结果",
	)

	fmt.Printf("Registered responses: %d\n", len(scenarios.ListResponses()))

	// Output: Registered responses: 3
}

// ExampleSequentialScenarios_workflow 演示顺序响应工作流
func ExampleSequentialScenarios_workflow() {
	// 使用builder创建完整的工作流序列
	responses := []string{
		// 1. 请求工具
		aidmock.BuildRequireTool(
			"port_scanner",
			"需要扫描端口",
			"请求端口扫描工具",
		),
		// 2. 提供工具参数
		aidmock.NewCallToolBuilder().
			WithParam("target", "192.168.1.1").
			WithParam("port_range", "1-1000").
			Build(),
		// 3. 验证结果
		aidmock.BuildVerifySatisfaction(
			true,
			"端口扫描完成，发现3个开放端口",
		),
	}

	scenarios := aidmock.NewSequentialScenarios(responses)
	fmt.Printf("Workflow steps: %d\n", len(responses))
	fmt.Printf("Current index: %d\n", scenarios.GetCurrentIndex())

	// Output:
	// Workflow steps: 3
	// Current index: 0
}

// ExampleBuildAskForClarification_complex 演示复杂的响应构建
func ExampleBuildAskForClarification_complex() {
	// 构建一个包含多个选项的澄清请求
	response := aidmock.BuildAskForClarification(
		"检测到多个漏洞，请选择处理方式：",
		[]string{
			"生成详细报告",
			"自动修复",
			"添加到待处理列表",
			"忽略",
		},
		"发现多个安全问题，需要用户决定",
		"等待用户选择处理方式",
	)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Complex response valid: %v\n", err == nil)

	// Output: Complex response valid: true
}

// ExamplePrettyPrintResponse 演示格式化打印响应
func ExamplePrettyPrintResponse() {
	response := aidmock.BuildDirectlyAnswer(
		"测试答案",
		"测试思考",
		"测试总结",
	)

	// 格式化打印（实际会包含缩进和换行）
	pretty := aidmock.PrettyPrintResponse(response)
	
	// 验证格式化后仍是有效JSON
	err := aidmock.ValidateResponse(pretty)
	fmt.Printf("Pretty printed response valid: %v\n", err == nil)

	// Output: Pretty printed response valid: true
}

// ExampleBuildCodeToolParams 演示构建代码工具参数
func ExampleBuildCodeToolParams() {
	response := aidmock.BuildCodeToolParams(
		`println("Hello, Yaklang!")`,
		"简单的Hello World程序",
		"yaklang",
	)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Code tool params valid: %v\n", err == nil)

	// Output: Code tool params valid: true
}

// ExampleBuildBlueprintToolParams 演示构建蓝图工具参数
func ExampleBuildBlueprintToolParams() {
	response := aidmock.BuildBlueprintToolParams(
		"web_vulnerability_scanner",
		map[string]interface{}{
			"target":  "http://example.com",
			"depth":   3,
			"threads": 5,
		},
	)

	err := aidmock.ValidateResponse(response)
	fmt.Printf("Blueprint tool params valid: %v\n", err == nil)

	// Output: Blueprint tool params valid: true
}

// ExampleBuildRequireTool_comparison 演示Builder模式 vs 原始JSON
func ExampleBuildRequireTool_comparison() {
	// 使用Builder（推荐方式）
	builderResponse := aidmock.BuildRequireTool(
		"nmap",
		"需要使用nmap扫描",
		"请求nmap工具",
	)

	// 原始JSON方式（不推荐）
	rawJSON := `{"@action":"object","next_action":{"type":"require_tool","tool_require_payload":"nmap"},"human_readable_thought":"需要使用nmap扫描","cumulative_summary":"请求nmap工具"}`

	// 两种方式都有效，但Builder更安全、可读、易维护
	err1 := aidmock.ValidateResponse(builderResponse)
	err2 := aidmock.ValidateResponse(rawJSON)

	fmt.Printf("Builder valid: %v\n", err1 == nil)
	fmt.Printf("Raw JSON valid: %v\n", err2 == nil)
	fmt.Println("Builder is recommended for type safety and maintainability")

	// Output:
	// Builder valid: true
	// Raw JSON valid: true
	// Builder is recommended for type safety and maintainability
}

