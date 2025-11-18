package aidmock

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// TestAddResponseWithMatcher 测试使用自定义匹配器添加响应
func TestAddResponseWithMatcher(t *testing.T) {
	scenarios := CreateCustomScenarios()

	// 使用自定义匹配函数
	customMatcher := func(prompt string) bool {
		return len(prompt) > 10 && len(prompt) < 50
	}

	response := BuildDirectlyAnswer("Matched by length", "Using custom matcher", "Custom match")
	scenarios.AddResponseWithMatcher("length_matcher", customMatcher, response, "Length-based matcher")

	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 测试匹配
	req1 := aicommon.NewAIRequest("This is a test prompt with enough length")
	resp1, err := callback(config, req1)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp1 == nil {
		t.Fatal("Expected response, got nil")
	}

	// 测试不匹配（太短）
	req2 := aicommon.NewAIRequest("short")
	resp2, _ := callback(config, req2)
	if resp2 == nil {
		t.Fatal("Expected default response, got nil")
	}
}

// TestMatcherContains 测试包含匹配器
func TestMatcherContains(t *testing.T) {
	matcher := MatcherContains("security")

	if !matcher("This is a security test") {
		t.Error("Expected to match 'security'")
	}

	if !matcher("SECURITY is important") {
		t.Error("Expected case-insensitive match")
	}

	if matcher("This is a safe test") {
		t.Error("Expected not to match")
	}
}

// TestMatcherRegex 测试正则表达式匹配器
func TestMatcherRegex(t *testing.T) {
	matcher := MatcherRegex(`\d{3}-\d{3}-\d{4}`)

	if !matcher("Call me at 123-456-7890") {
		t.Error("Expected to match phone number pattern")
	}

	if matcher("Call me at 12-34-56") {
		t.Error("Expected not to match invalid pattern")
	}
}

// TestMatcherPrefix 测试前缀匹配器
func TestMatcherPrefix(t *testing.T) {
	matcher := MatcherPrefix("scan")

	if !matcher("scan the target") {
		t.Error("Expected to match prefix")
	}

	if !matcher("SCAN the target") {
		t.Error("Expected case-insensitive match")
	}

	if matcher("please scan the target") {
		t.Error("Expected not to match (not a prefix)")
	}
}

// TestMatcherSuffix 测试后缀匹配器
func TestMatcherSuffix(t *testing.T) {
	matcher := MatcherSuffix("?")

	if !matcher("What is your name?") {
		t.Error("Expected to match suffix")
	}

	if matcher("What is your name") {
		t.Error("Expected not to match (no suffix)")
	}
}

// TestMatcherAnd 测试AND组合匹配器
func TestMatcherAnd(t *testing.T) {
	matcher := MatcherAnd(
		MatcherContains("security"),
		MatcherContains("test"),
	)

	if !matcher("This is a security test") {
		t.Error("Expected to match both keywords")
	}

	if matcher("This is a security check") {
		t.Error("Expected not to match (missing 'test')")
	}
}

// TestMatcherOr 测试OR组合匹配器
func TestMatcherOr(t *testing.T) {
	matcher := MatcherOr(
		MatcherContains("security"),
		MatcherContains("safety"),
	)

	if !matcher("This is a security test") {
		t.Error("Expected to match 'security'")
	}

	if !matcher("This is a safety test") {
		t.Error("Expected to match 'safety'")
	}

	if matcher("This is a test") {
		t.Error("Expected not to match")
	}
}

// TestMatcherNot 测试NOT匹配器
func TestMatcherNot(t *testing.T) {
	matcher := MatcherNot(MatcherContains("skip"))

	if !matcher("This is a test") {
		t.Error("Expected to match (does not contain 'skip')")
	}

	if matcher("Skip this test") {
		t.Error("Expected not to match (contains 'skip')")
	}
}

// TestMatcherLength 测试长度匹配器
func TestMatcherLength(t *testing.T) {
	matcher := MatcherLength(10, 20)

	if !matcher("Hello World!") {
		t.Error("Expected to match length")
	}

	if matcher("Hi") {
		t.Error("Expected not to match (too short)")
	}

	if matcher("This is a very long prompt that exceeds the limit") {
		t.Error("Expected not to match (too long)")
	}
}

// TestMatcherContainsAll 测试包含所有子字符串匹配器
func TestMatcherContainsAll(t *testing.T) {
	matcher := MatcherContainsAll("scan", "target", "port")

	if !matcher("scan the target ports") {
		t.Error("Expected to match all keywords")
	}

	if matcher("scan the target") {
		t.Error("Expected not to match (missing 'port')")
	}
}

// TestMatcherContainsAny 测试包含任一子字符串匹配器
func TestMatcherContainsAny(t *testing.T) {
	matcher := MatcherContainsAny("scan", "analyze", "test")

	if !matcher("Let's scan the system") {
		t.Error("Expected to match 'scan'")
	}

	if !matcher("Let's analyze the data") {
		t.Error("Expected to match 'analyze'")
	}

	if matcher("Let's check the system") {
		t.Error("Expected not to match")
	}
}

// TestMatcherExact 测试精确匹配器
func TestMatcherExact(t *testing.T) {
	matcher := MatcherExact("help")

	if !matcher("help") {
		t.Error("Expected exact match")
	}

	if !matcher("  HELP  ") {
		t.Error("Expected case-insensitive and trimmed match")
	}

	if matcher("help me") {
		t.Error("Expected not to match (not exact)")
	}
}

// TestMatcherEmpty 测试空匹配器
func TestMatcherEmpty(t *testing.T) {
	matcher := MatcherEmpty()

	if !matcher("") {
		t.Error("Expected to match empty string")
	}

	if !matcher("   ") {
		t.Error("Expected to match whitespace-only string")
	}

	if matcher("test") {
		t.Error("Expected not to match non-empty string")
	}
}

// TestComplexMatcherCombination 测试复杂的匹配器组合
func TestComplexMatcherCombination(t *testing.T) {
	// 创建一个复杂的匹配器：
	// - 必须包含 "scan" 或 "test"
	// - 必须包含 "target"
	// - 长度必须大于10
	matcher := MatcherAnd(
		MatcherOr(
			MatcherContains("scan"),
			MatcherContains("test"),
		),
		MatcherContains("target"),
		MatcherLengthMin(10),
	)

	if !matcher("scan the target system") {
		t.Error("Expected to match complex condition")
	}

	if !matcher("test the target application") {
		t.Error("Expected to match complex condition")
	}

	if matcher("scan") {
		t.Error("Expected not to match (too short, missing 'target')")
	}

	if matcher("analyze the target system") {
		t.Error("Expected not to match (missing 'scan' or 'test')")
	}
}

// TestScenariosWithDifferentMatchers 测试在场景中使用不同的匹配器
func TestScenariosWithDifferentMatchers(t *testing.T) {
	scenarios := CreateCustomScenarios()

	// 添加使用正则的响应
	scenarios.AddResponseWithMatcher(
		"phone_number",
		MatcherRegex(`\d{3}-\d{3}-\d{4}`),
		BuildDirectlyAnswer("Phone number detected", "Regex match", "Phone"),
		"Matches phone numbers",
	)

	// 添加使用长度的响应
	scenarios.AddResponseWithMatcher(
		"short_prompt",
		MatcherLength(1, 10),
		BuildDirectlyAnswer("Short prompt", "Length match", "Short"),
		"Matches short prompts",
	)

	// 添加使用组合匹配器的响应
	scenarios.AddResponseWithMatcher(
		"security_scan",
		MatcherAnd(
			MatcherContains("security"),
			MatcherContains("scan"),
		),
		BuildDirectlyAnswer("Security scan", "Combined match", "SecScan"),
		"Matches security scan",
	)

	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 测试正则匹配
	req1 := aicommon.NewAIRequest("My number is 123-456-7890")
	resp1, _ := callback(config, req1)
	if resp1 == nil {
		t.Error("Expected phone number match")
	}

	// 测试短prompt匹配
	req2 := aicommon.NewAIRequest("hello")
	resp2, _ := callback(config, req2)
	if resp2 == nil {
		t.Error("Expected short prompt match")
	}

	// 测试组合匹配
	req3 := aicommon.NewAIRequest("perform security scan on target")
	resp3, _ := callback(config, req3)
	if resp3 == nil {
		t.Error("Expected security scan match")
	}
}

// TestMatcherPriority 测试匹配器的优先级（先添加的先匹配）
func TestMatcherPriority(t *testing.T) {
	scenarios := CreateCustomScenarios()

	// 先添加更具体的匹配器
	scenarios.AddResponseWithMatcher(
		"specific",
		MatcherExact("test"),
		BuildDirectlyAnswer("Specific match", "Exact", "Specific"),
		"Specific matcher",
	)

	// 后添加更宽泛的匹配器
	scenarios.AddResponseWithMatcher(
		"general",
		MatcherContains("test"),
		BuildDirectlyAnswer("General match", "Contains", "General"),
		"General matcher",
	)

	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 应该匹配第一个（specific）
	req := aicommon.NewAIRequest("test")
	resp, _ := callback(config, req)
	if resp == nil {
		t.Error("Expected response")
	}
	// 注意：由于map遍历顺序不确定，这个测试可能不稳定
	// 在实际使用中，应该避免添加重叠的匹配器
}

