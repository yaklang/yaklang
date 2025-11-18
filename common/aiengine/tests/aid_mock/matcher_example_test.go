package aidmock_test

import (
	"fmt"
	"strings"

	aidmock "github.com/yaklang/yaklang/common/aiengine/tests/aid_mock"
)

// ExampleKeywordScenarios_addResponseWithMatcher 演示如何使用自定义匹配函数
func ExampleKeywordScenarios_addResponseWithMatcher() {
	scenarios := aidmock.CreateCustomScenarios()

	// 使用自定义匹配函数：匹配长度在10-50之间的prompt
	customMatcher := func(prompt string) bool {
		length := len(prompt)
		return length >= 10 && length <= 50
	}

	response := aidmock.BuildDirectlyAnswer(
		"Your prompt length is appropriate",
		"Checking prompt length",
		"Length validated",
	)

	scenarios.AddResponseWithMatcher(
		"length_validator",
		customMatcher,
		response,
		"Validates prompt length",
	)

	fmt.Printf("Response added: %v\n", scenarios.GetResponse("length_validator") != nil)
	// Output: Response added: true
}

// ExampleMatcherContains 演示包含匹配器
func ExampleMatcherContains() {
	scenarios := aidmock.CreateCustomScenarios()

	// 使用便捷的匹配器函数
	matcher := aidmock.MatcherContains("security")

	response := aidmock.BuildRequireTool(
		"security_scanner",
		"Detected security-related request",
		"Security scan initiated",
	)

	scenarios.AddResponseWithMatcher(
		"security_match",
		matcher,
		response,
		"Matches security requests",
	)

	fmt.Printf("Scenarios created: %v\n", len(scenarios.ListResponses()) > 0)
	// Output: Scenarios created: true
}

// ExampleMatcherRegex 演示正则表达式匹配器
func ExampleMatcherRegex() {
	scenarios := aidmock.CreateCustomScenarios()

	// 匹配IP地址格式
	ipMatcher := aidmock.MatcherRegex(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)

	response := aidmock.BuildDirectlyAnswer(
		"IP address detected in your request",
		"Parsing IP address",
		"IP validated",
	)

	scenarios.AddResponseWithMatcher(
		"ip_detector",
		ipMatcher,
		response,
		"Detects IP addresses",
	)

	fmt.Printf("IP matcher added: %v\n", scenarios.GetResponse("ip_detector") != nil)
	// Output: IP matcher added: true
}

// ExampleMatcherAnd 演示AND组合匹配器
func ExampleMatcherAnd() {
	scenarios := aidmock.CreateCustomScenarios()

	// 组合多个条件：必须同时包含 "scan" 和 "port"
	matcher := aidmock.MatcherAnd(
		aidmock.MatcherContains("scan"),
		aidmock.MatcherContains("port"),
	)

	response := aidmock.BuildRequireTool(
		"port_scanner",
		"Initiating port scan",
		"Port scan requested",
	)

	scenarios.AddResponseWithMatcher(
		"port_scan",
		matcher,
		response,
		"Port scanning request",
	)

	fmt.Printf("Combined matcher added: %v\n", scenarios.GetResponse("port_scan") != nil)
	// Output: Combined matcher added: true
}

// ExampleMatcherOr 演示OR组合匹配器
func ExampleMatcherOr() {
	scenarios := aidmock.CreateCustomScenarios()

	// 匹配多个可能的关键词之一
	matcher := aidmock.MatcherOr(
		aidmock.MatcherContains("help"),
		aidmock.MatcherContains("assist"),
		aidmock.MatcherContains("guide"),
	)

	response := aidmock.BuildDirectlyAnswer(
		"How can I assist you?",
		"User requesting help",
		"Help offered",
	)

	scenarios.AddResponseWithMatcher(
		"help_request",
		matcher,
		response,
		"Help/assistance requests",
	)

	fmt.Printf("OR matcher added: %v\n", scenarios.GetResponse("help_request") != nil)
	// Output: OR matcher added: true
}

// ExampleMatcherNot 演示NOT匹配器
func ExampleMatcherNot() {
	scenarios := aidmock.CreateCustomScenarios()

	// 匹配不包含 "skip" 的prompt
	matcher := aidmock.MatcherNot(
		aidmock.MatcherContains("skip"),
	)

	response := aidmock.BuildDirectlyAnswer(
		"Processing your request",
		"Request accepted",
		"Processing",
	)

	scenarios.AddResponseWithMatcher(
		"process_request",
		matcher,
		response,
		"Process non-skip requests",
	)

	fmt.Printf("NOT matcher added: %v\n", scenarios.GetResponse("process_request") != nil)
	// Output: NOT matcher added: true
}

// ExampleMatcherAnd_complex 演示复杂匹配器组合
func ExampleMatcherAnd_complex() {
	scenarios := aidmock.CreateCustomScenarios()

	// 复杂条件：
	// 1. 必须包含 "scan" 或 "test"
	// 2. 必须包含 "target"
	// 3. 长度必须大于15个字符
	matcher := aidmock.MatcherAnd(
		aidmock.MatcherOr(
			aidmock.MatcherContains("scan"),
			aidmock.MatcherContains("test"),
		),
		aidmock.MatcherContains("target"),
		aidmock.MatcherLengthMin(15),
	)

	response := aidmock.BuildRequireTool(
		"vulnerability_scanner",
		"Complex condition matched",
		"Scanner activated",
	)

	scenarios.AddResponseWithMatcher(
		"complex_scan",
		matcher,
		response,
		"Complex scanning scenario",
	)

	fmt.Printf("Complex matcher added: %v\n", scenarios.GetResponse("complex_scan") != nil)
	// Output: Complex matcher added: true
}

// ExampleMatcherLength 演示长度匹配器
func ExampleMatcherLength() {
	scenarios := aidmock.CreateCustomScenarios()

	// 匹配短prompt（1-20个字符）
	shortMatcher := aidmock.MatcherLength(1, 20)

	response := aidmock.BuildAskForClarification(
		"Your request is too short. Could you provide more details?",
		[]string{"Provide more context", "Cancel"},
		"Short prompt detected",
		"Requesting clarification",
	)

	scenarios.AddResponseWithMatcher(
		"short_prompt_handler",
		shortMatcher,
		response,
		"Handles short prompts",
	)

	fmt.Printf("Length matcher added: %v\n", scenarios.GetResponse("short_prompt_handler") != nil)
	// Output: Length matcher added: true
}

// ExampleMatcherContainsAll 演示包含所有关键词匹配器
func ExampleMatcherContainsAll() {
	scenarios := aidmock.CreateCustomScenarios()

	// 必须同时包含所有关键词
	matcher := aidmock.MatcherContainsAll("web", "application", "security", "test")

	response := aidmock.BuildRequireTool(
		"web_security_scanner",
		"Initiating comprehensive web security test",
		"Web security scan started",
	)

	scenarios.AddResponseWithMatcher(
		"web_sec_test",
		matcher,
		response,
		"Web application security testing",
	)

	fmt.Printf("ContainsAll matcher added: %v\n", scenarios.GetResponse("web_sec_test") != nil)
	// Output: ContainsAll matcher added: true
}

// ExampleMatcherFunc 演示自定义函数匹配器
func ExampleMatcherFunc() {
	scenarios := aidmock.CreateCustomScenarios()

	// 使用完全自定义的匹配逻辑
	customLogic := aidmock.MatcherFunc(func(prompt string) bool {
		// 自定义逻辑：检查是否是问句且包含特定关键词
		isQuestion := strings.HasSuffix(strings.TrimSpace(prompt), "?")
		hasKeyword := strings.Contains(strings.ToLower(prompt), "vulnerability")
		return isQuestion && hasKeyword
	})

	response := aidmock.BuildDirectlyAnswer(
		"Let me help you understand vulnerabilities",
		"Question about vulnerabilities detected",
		"Educational response provided",
	)

	scenarios.AddResponseWithMatcher(
		"vuln_question",
		customLogic,
		response,
		"Vulnerability-related questions",
	)

	fmt.Printf("Custom logic matcher added: %v\n", scenarios.GetResponse("vuln_question") != nil)
	// Output: Custom logic matcher added: true
}

// ExampleKeywordScenarios_mixedMatchers 演示混合使用关键词和自定义匹配器
func ExampleKeywordScenarios_mixedMatchers() {
	scenarios := aidmock.CreateCustomScenarios()

	// 添加使用关键词的响应
	scenarios.AddResponse(
		"keyword_based",
		[]string{"simple", "request"},
		aidmock.BuildDirectlyAnswer("Simple response", "Keyword match", "Done"),
		"Keyword-based matching",
	)

	// 添加使用自定义匹配器的响应
	scenarios.AddResponseWithMatcher(
		"matcher_based",
		aidmock.MatcherRegex(`urgent|critical|emergency`),
		aidmock.BuildDirectlyAnswer("High priority response", "Urgent match", "Priority"),
		"Matcher-based matching",
	)

	fmt.Printf("Total responses: %d\n", len(scenarios.ListResponses()))
	// Output: Total responses: 2
}

// ExampleMatcherPrefix 演示前缀匹配器
func ExampleMatcherPrefix() {
	scenarios := aidmock.CreateCustomScenarios()

	// 匹配以特定命令开头的prompt
	commandMatcher := aidmock.MatcherPrefix("/scan")

	response := aidmock.BuildRequireTool(
		"scanner",
		"Command detected",
		"Executing scan command",
	)

	scenarios.AddResponseWithMatcher(
		"scan_command",
		commandMatcher,
		response,
		"Scan command handler",
	)

	fmt.Printf("Prefix matcher added: %v\n", scenarios.GetResponse("scan_command") != nil)
	// Output: Prefix matcher added: true
}

// ExampleMatcherEmpty 演示空匹配器
func ExampleMatcherEmpty() {
	scenarios := aidmock.CreateCustomScenarios()

	// 处理空prompt
	emptyMatcher := aidmock.MatcherEmpty()

	response := aidmock.BuildAskForClarification(
		"Your prompt is empty. What would you like to do?",
		[]string{"Start scanning", "Get help", "Exit"},
		"Empty prompt received",
		"Requesting user input",
	)

	scenarios.AddResponseWithMatcher(
		"empty_handler",
		emptyMatcher,
		response,
		"Handles empty prompts",
	)

	fmt.Printf("Empty matcher added: %v\n", scenarios.GetResponse("empty_handler") != nil)
	// Output: Empty matcher added: true
}
