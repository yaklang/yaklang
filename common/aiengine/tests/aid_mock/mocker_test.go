package aidmock

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// TestKeywordScenarios_BasicMatch 测试基本的关键词匹配
func TestKeywordScenarios_BasicMatch(t *testing.T) {
	scenarios := NewKeywordScenarios()
	callback := scenarios.GetAICallbackType()

	// 测试匹配 directly_answer
	config := mock.NewMockedAIConfig(context.Background())
	req := aicommon.NewAIRequest("I need a directly_answer with next_action")
	resp, err := callback(config, req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
}

// TestKeywordScenarios_CustomResponse 测试自定义响应
func TestKeywordScenarios_CustomResponse(t *testing.T) {
	scenarios := NewKeywordScenarios()

	// 添加自定义响应
	customResp := `{"@action": "object", "next_action": {"type": "custom_action"}}`
	scenarios.AddResponse("custom_test", []string{"custom", "test"}, customResp, "Custom test response")

	if scenarios.GetResponse("custom_test") == nil {
		t.Fatal("Expected custom response to be added")
	}
}

// TestKeywordScenarios_ListResponses 测试列出所有响应
func TestKeywordScenarios_ListResponses(t *testing.T) {
	scenarios := NewKeywordScenarios()
	responses := scenarios.ListResponses()

	if len(responses) == 0 {
		t.Fatal("Expected at least some built-in responses")
	}

	// 验证包含一些内置响应
	found := false
	for _, name := range responses {
		if name == "directly_answer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected to find 'directly_answer' in built-in responses")
	}
}

// TestSequentialScenarios_Basic 测试顺序响应场景生成器
func TestSequentialScenarios_Basic(t *testing.T) {
	responses := []string{
		`{"step": 1}`,
		`{"step": 2}`,
		`{"step": 3}`,
	}

	scenarios := NewSequentialScenarios(responses)
	if scenarios == nil {
		t.Fatal("Expected scenarios to be created")
	}

	if scenarios.GetCurrentIndex() != 0 {
		t.Fatal("Expected initial index to be 0")
	}
}

// TestSequentialScenarios_Reset 测试重置功能
func TestSequentialScenarios_Reset(t *testing.T) {
	responses := []string{`{"step": 1}`, `{"step": 2}`}
	scenarios := NewSequentialScenarios(responses)
	callback := scenarios.GetAICallbackType()
	config := mock.NewMockedAIConfig(context.Background())

	// 消耗一个响应
	_, _ = callback(config, aicommon.NewAIRequest("test"))

	if scenarios.GetCurrentIndex() != 1 {
		t.Fatalf("Expected index to be 1, got %d", scenarios.GetCurrentIndex())
	}

	// 重置
	scenarios.Reset()

	if scenarios.GetCurrentIndex() != 0 {
		t.Fatalf("Expected index to be 0 after reset, got %d", scenarios.GetCurrentIndex())
	}
}

// TestGetScenario 测试获取场景
func TestGetScenario(t *testing.T) {
	scenario, ok := GetScenario("simple_qa")
	if !ok {
		t.Fatal("Expected to find simple_qa scenario")
	}

	if scenario.Name != "simple_qa" {
		t.Fatalf("Expected name 'simple_qa', got %s", scenario.Name)
	}

	if len(scenario.Steps) == 0 {
		t.Fatal("Expected scenario to have steps")
	}
}

// TestListScenarios 测试列出所有场景
func TestListScenarios(t *testing.T) {
	scenarios := ListScenarios()
	if len(scenarios) == 0 {
		t.Fatal("Expected at least some scenarios")
	}
}

// TestGetCommonResponse 测试获取常见响应
func TestGetCommonResponse(t *testing.T) {
	resp, ok := GetCommonResponse("simple_answer")
	if !ok {
		t.Fatal("Expected to find simple_answer response")
	}

	if !strings.Contains(resp, "directly_answer") {
		t.Fatalf("Expected response to contain 'directly_answer', got: %s", resp)
	}
}

// TestCreateCustomScenarios 测试创建空的自定义场景生成器
func TestCreateCustomScenarios(t *testing.T) {
	scenarios := CreateCustomScenarios()
	responses := scenarios.ListResponses()

	if len(responses) != 0 {
		t.Fatalf("Expected no built-in responses, got %d", len(responses))
	}
}
