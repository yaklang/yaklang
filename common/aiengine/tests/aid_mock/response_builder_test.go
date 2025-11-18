package aidmock

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildDirectlyAnswer 测试直接回答构建器
func TestBuildDirectlyAnswer(t *testing.T) {
	response := BuildDirectlyAnswer(
		"Test answer",
		"Test thought",
		"Test summary",
	)

	// 验证是否为有效的JSON
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// 验证字段
	if obj["@action"] != "object" {
		t.Errorf("Expected @action to be 'object', got %v", obj["@action"])
	}

	nextAction := obj["next_action"].(map[string]interface{})
	if nextAction["type"] != "directly_answer" {
		t.Errorf("Expected type to be 'directly_answer', got %v", nextAction["type"])
	}

	if nextAction["answer_payload"] != "Test answer" {
		t.Errorf("Expected answer_payload to be 'Test answer', got %v", nextAction["answer_payload"])
	}

	if obj["human_readable_thought"] != "Test thought" {
		t.Errorf("Expected thought to be 'Test thought', got %v", obj["human_readable_thought"])
	}

	if obj["cumulative_summary"] != "Test summary" {
		t.Errorf("Expected summary to be 'Test summary', got %v", obj["cumulative_summary"])
	}
}

// TestBuildRequireTool 测试工具请求构建器
func TestBuildRequireTool(t *testing.T) {
	response := BuildRequireTool(
		"test_tool",
		"Need to use test tool",
		"Tool requested",
	)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	nextAction := obj["next_action"].(map[string]interface{})
	if nextAction["type"] != "require_tool" {
		t.Errorf("Expected type to be 'require_tool', got %v", nextAction["type"])
	}

	if nextAction["tool_require_payload"] != "test_tool" {
		t.Errorf("Expected tool_require_payload to be 'test_tool', got %v", nextAction["tool_require_payload"])
	}
}

// TestBuildRequestPlanAndExecution 测试计划执行构建器
func TestBuildRequestPlanAndExecution(t *testing.T) {
	response := BuildRequestPlanAndExecution(
		"Test plan",
		"Planning",
		"Plan created",
	)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	nextAction := obj["next_action"].(map[string]interface{})
	if nextAction["type"] != "request_plan_and_execution" {
		t.Errorf("Expected type to be 'request_plan_and_execution', got %v", nextAction["type"])
	}

	if nextAction["plan_request_payload"] != "Test plan" {
		t.Errorf("Expected plan_request_payload to be 'Test plan', got %v", nextAction["plan_request_payload"])
	}
}

// TestBuildAskForClarification 测试澄清请求构建器
func TestBuildAskForClarification(t *testing.T) {
	response := BuildAskForClarification(
		"What do you prefer?",
		[]string{"option1", "option2"},
		"Need input",
		"Awaiting clarification",
	)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	nextAction := obj["next_action"].(map[string]interface{})
	payload := nextAction["ask_for_clarification_payload"].(map[string]interface{})

	if payload["question"] != "What do you prefer?" {
		t.Errorf("Expected question, got %v", payload["question"])
	}

	options := payload["options"].([]interface{})
	if len(options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(options))
	}
}

// TestBuildCallTool 测试工具调用构建器
func TestBuildCallTool(t *testing.T) {
	params := map[string]interface{}{
		"param1": "value1",
		"param2": 123,
	}

	response := BuildCallTool(params)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if obj["@action"] != "call-tool" {
		t.Errorf("Expected @action to be 'call-tool', got %v", obj["@action"])
	}

	gotParams := obj["params"].(map[string]interface{})
	if gotParams["param1"] != "value1" {
		t.Errorf("Expected param1 to be 'value1', got %v", gotParams["param1"])
	}
}

// TestBuildVerifySatisfaction 测试满意度验证构建器
func TestBuildVerifySatisfaction(t *testing.T) {
	response := BuildVerifySatisfaction(true, "Everything is good")

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if obj["@action"] != "verify-satisfaction" {
		t.Errorf("Expected @action to be 'verify-satisfaction', got %v", obj["@action"])
	}

	if obj["user_satisfied"] != true {
		t.Errorf("Expected user_satisfied to be true, got %v", obj["user_satisfied"])
	}

	if obj["reasoning"] != "Everything is good" {
		t.Errorf("Expected reasoning to be 'Everything is good', got %v", obj["reasoning"])
	}
}

// TestBuildSleepToolParams 测试sleep工具参数构建器
func TestBuildSleepToolParams(t *testing.T) {
	response := BuildSleepToolParams(0.5)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	params := obj["params"].(map[string]interface{})
	if params["seconds"] != 0.5 {
		t.Errorf("Expected seconds to be 0.5, got %v", params["seconds"])
	}
}

// TestBuildRiskToolParams 测试风险工具参数构建器
func TestBuildRiskToolParams(t *testing.T) {
	response := BuildRiskToolParams(
		"http://example.com",
		"Test Risk",
		"high",
		"sqli",
	)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	params := obj["params"].(map[string]interface{})
	if params["target"] != "http://example.com" {
		t.Errorf("Expected target, got %v", params["target"])
	}

	if params["title"] != "Test Risk" {
		t.Errorf("Expected title, got %v", params["title"])
	}

	if params["severity"] != "high" {
		t.Errorf("Expected severity, got %v", params["severity"])
	}

	if params["type"] != "sqli" {
		t.Errorf("Expected type, got %v", params["type"])
	}
}

// TestBuildCodeToolParams 测试代码工具参数构建器
func TestBuildCodeToolParams(t *testing.T) {
	response := BuildCodeToolParams(
		"println(\"test\")",
		"Test code",
		"yaklang",
	)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	params := obj["params"].(map[string]interface{})
	if params["code"] != "println(\"test\")" {
		t.Errorf("Expected code, got %v", params["code"])
	}

	if params["description"] != "Test code" {
		t.Errorf("Expected description, got %v", params["description"])
	}

	if params["language"] != "yaklang" {
		t.Errorf("Expected language, got %v", params["language"])
	}
}

// TestReactResponseBuilder 测试ReAct响应构建器的链式调用
func TestReactResponseBuilder(t *testing.T) {
	response := NewReactResponseBuilder().
		WithDirectlyAnswer("Custom answer").
		WithHumanThought("Custom thought").
		WithCumulativeSummary("Custom summary").
		Build()

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	nextAction := obj["next_action"].(map[string]interface{})
	if nextAction["answer_payload"] != "Custom answer" {
		t.Errorf("Expected answer_payload, got %v", nextAction["answer_payload"])
	}
}

// TestCallToolBuilder 测试工具调用构建器的链式调用
func TestCallToolBuilder(t *testing.T) {
	response := NewCallToolBuilder().
		WithParam("key1", "value1").
		WithParam("key2", 42).
		WithParam("key3", true).
		Build()

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	params := obj["params"].(map[string]interface{})
	if len(params) != 3 {
		t.Errorf("Expected 3 params, got %d", len(params))
	}

	if params["key1"] != "value1" {
		t.Errorf("Expected key1 value, got %v", params["key1"])
	}
}

// TestVerifySatisfactionBuilder 测试满意度验证构建器的链式调用
func TestVerifySatisfactionBuilder(t *testing.T) {
	response := NewVerifySatisfactionBuilder().
		WithSatisfied(false).
		WithReasoning("Needs improvement").
		Build()

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &obj); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if obj["user_satisfied"] != false {
		t.Errorf("Expected user_satisfied to be false, got %v", obj["user_satisfied"])
	}

	if obj["reasoning"] != "Needs improvement" {
		t.Errorf("Expected reasoning, got %v", obj["reasoning"])
	}
}

// TestValidateResponse 测试响应验证功能
func TestValidateResponse(t *testing.T) {
	// 测试有效响应
	validResponse := BuildDirectlyAnswer("test", "test", "test")
	if err := ValidateResponse(validResponse); err != nil {
		t.Errorf("Expected valid response, got error: %v", err)
	}

	// 测试无效JSON
	if err := ValidateResponse("invalid json"); err == nil {
		t.Error("Expected error for invalid JSON")
	}

	// 测试缺少@action字段
	if err := ValidateResponse(`{"next_action": {}}`); err == nil {
		t.Error("Expected error for missing @action")
	}

	// 测试object action缺少next_action
	if err := ValidateResponse(`{"@action": "object"}`); err == nil {
		t.Error("Expected error for missing next_action")
	}
}

// TestPrettyPrintResponse 测试格式化打印功能
func TestPrettyPrintResponse(t *testing.T) {
	response := BuildDirectlyAnswer("test", "test", "test")
	pretty := PrettyPrintResponse(response)

	// 验证包含换行符（格式化输出）
	if !strings.Contains(pretty, "\n") {
		t.Error("Expected pretty printed response to contain newlines")
	}

	// 验证仍然是有效的JSON
	var obj interface{}
	if err := json.Unmarshal([]byte(pretty), &obj); err != nil {
		t.Errorf("Pretty printed response is not valid JSON: %v", err)
	}
}

// TestBuilderIntegration 测试构建器集成
func TestBuilderIntegration(t *testing.T) {
	// 创建一个完整的工具调用流程
	scenarios := CreateCustomScenarios()

	// 使用builder添加响应
	scenarios.AddResponse(
		"test_workflow",
		[]string{"test", "workflow"},
		BuildRequireTool("test_tool", "Testing tool", "Tool requested"),
		"Test workflow",
	)

	scenarios.AddResponse(
		"test_params",
		[]string{"generate", "params"},
		BuildCallTool(map[string]interface{}{"test_param": "test_value"}),
		"Test params",
	)

	scenarios.AddResponse(
		"test_verify",
		[]string{"verify"},
		BuildVerifySatisfaction(true, "Test completed"),
		"Test verify",
	)

	// 验证响应数量
	if len(scenarios.ListResponses()) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(scenarios.ListResponses()))
	}

	// 验证每个响应都是有效的JSON
	for _, name := range scenarios.ListResponses() {
		resp := scenarios.GetResponse(name)
		if err := ValidateResponse(resp.Response); err != nil {
			t.Errorf("Response %s is invalid: %v", name, err)
		}
	}
}
