package aireact

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// newMockTaskWithUserInput 创建一个带有用户输入的模拟任务
func newMockTaskWithUserInput(name, userInput string) aicommon.AIStatefulTask {
	task := aicommon.NewStatefulTaskBase(
		name,
		userInput,
		context.Background(),
		nil, // emitter
	)
	return task
}

func TestPromptManagerWithDynamicContextProvider(t *testing.T) {
	// Track if the provider was called
	providerCalled := false
	providerCallCount := 0
	var providerMutex sync.Mutex

	// Create a mock context provider
	mockProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		providerMutex.Lock()
		defer providerMutex.Unlock()

		providerCalled = true
		providerCallCount++

		return fmt.Sprintf("Mock context from provider '%s' at %s", key, time.Now().Format("15:04:05")), nil
	}

	// Create ReAct instance with the dynamic context provider
	react, err := NewTestReAct(
		aicommon.WithDynamicContextProvider("test_provider", mockProvider),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			// Mock AI response for testing
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Test the DynamicContext method
	ctx := react.promptManager.DynamicContext()

	// Verify the provider was called
	providerMutex.Lock()
	called := providerCalled
	callCount := providerCallCount
	providerMutex.Unlock()

	if !called {
		t.Fatal("Dynamic context provider was not called")
	}

	if callCount != 1 {
		t.Fatalf("Expected provider to be called once, but was called %d times", callCount)
	}

	// Verify the context contains expected content
	if ctx == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	if !utils.MatchAllOfSubString(ctx, "Mock context from provider", "test_provider") {
		t.Fatalf("Dynamic context does not contain expected content. Got: %s", ctx)
	}

	t.Logf("Dynamic context: %s", ctx)
}

func TestPromptManager__MultipleProviders(t *testing.T) {
	callCounts := make(map[string]int)
	var countsMutex sync.Mutex

	provider1 := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		countsMutex.Lock()
		callCounts["provider1"]++
		countsMutex.Unlock()
		return "Context from provider 1", nil
	}

	provider2 := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		countsMutex.Lock()
		callCounts["provider2"]++
		countsMutex.Unlock()
		return "Context from provider 2", nil
	}

	// Create ReAct instance with multiple providers
	react, err := NewTestReAct(
		aicommon.WithDynamicContextProvider("provider1", provider1),
		aicommon.WithDynamicContextProvider("provider2", provider2),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Call DynamicContext
	ctx := react.promptManager.DynamicContext()

	// Verify both providers were called
	countsMutex.Lock()
	if callCounts["provider1"] != 1 {
		t.Fatalf("Provider1 should be called once, got %d", callCounts["provider1"])
	}
	if callCounts["provider2"] != 1 {
		t.Fatalf("Provider2 should be called once, got %d", callCounts["provider2"])
	}
	countsMutex.Unlock()

	// Verify context contains content from both providers
	if !utils.MatchAllOfSubString(ctx, "Context from provider 1", "Context from provider 2") {
		t.Fatalf("Dynamic context should contain content from both providers. Got: %s", ctx)
	}

	t.Logf("Dynamic context with multiple providers: %s", ctx)
}

func TestPromptManager__ErrorHandling(t *testing.T) {
	// Provider that returns an error
	errorProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "", fmt.Errorf("mock provider error")
	}

	// Normal provider
	normalProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "Normal context", nil
	}

	react, err := NewTestReAct(
		aicommon.WithDynamicContextProvider("error_provider", errorProvider),
		aicommon.WithDynamicContextProvider("normal_provider", normalProvider),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Call DynamicContext - should not panic even with error
	ctx := react.promptManager.DynamicContext()

	// Verify normal provider content is included
	if !utils.MatchAllOfSubString(ctx, "Normal context") {
		t.Fatalf("Normal provider content should be included despite error. Got: %s", ctx)
	}

	// Verify error is handled gracefully (should contain error message)
	if !utils.MatchAllOfSubString(ctx, "Error getting context") {
		t.Fatalf("Error should be handled gracefully. Got: %s", ctx)
	}

	t.Logf("Dynamic context with error handling: %s", ctx)
}

func TestPromptManager__InPromptGeneration(t *testing.T) {
	providerCalled := false
	var callMutex sync.Mutex

	mockProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		callMutex.Lock()
		providerCalled = true
		callMutex.Unlock()
		return "Context for prompt generation", nil
	}

	react, err := NewTestReAct(
		aicommon.WithDynamicContextProvider("prompt_test", mockProvider),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	_, _, err = react.GetBasicPromptInfo(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	callMutex.Lock()
	if !providerCalled {
		t.Fatal("Dynamic context provider should be called during prompt generation")
	}
	callMutex.Unlock()

	// Test other prompt generation methods
	callMutex.Lock()
	providerCalled = false
	callMutex.Unlock()

	_, _, err = react.promptManager.GenerateDirectlyAnswerPrompt("test query", nil)
	if err != nil {
		t.Fatalf("Failed to generate directly answer prompt: %v", err)
	}

	callMutex.Lock()
	if !providerCalled {
		t.Fatal("Dynamic context provider should be called during directly answer prompt generation")
	}
	callMutex.Unlock()

	t.Log("Dynamic context provider correctly called during prompt generation")
}

func TestPromptManager_WithTracedDynamicContextProvider(t *testing.T) {
	callCount := 0
	var countMutex sync.Mutex

	mockProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		countMutex.Lock()
		callCount++
		countMutex.Unlock()
		return fmt.Sprintf("Traced content call #%d at %s", callCount, time.Now().Format("15:04:05")), nil
	}

	react, err := NewTestReAct(
		aicommon.WithTracedDynamicContextProvider("traced_provider", mockProvider),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// First call - should not have diff
	ctx1 := react.promptManager.DynamicContext()
	if ctx1 == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	if !utils.MatchAllOfSubString(ctx1, "Traced content call #1", "traced_provider") {
		t.Fatalf("First call context does not contain expected content. Got: %s", ctx1)
	}

	// Second call - should have diff
	ctx2 := react.promptManager.DynamicContext()
	if ctx2 == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	if !utils.MatchAllOfSubString(ctx2, "Traced content call #2", "traced_provider") {
		t.Fatalf("Second call context does not contain expected content. Got: %s", ctx2)
	}

	// Second call should contain diff information
	if !utils.MatchAllOfSubString(ctx2, "CHANGES_DIFF") {
		t.Fatalf("Second call should contain diff information. Got: %s", ctx2)
	}

	t.Logf("First call context: %s", ctx1)
	t.Logf("Second call context: %s", ctx2)
}

func TestPromptManager_WithTracedFileContext(t *testing.T) {
	// Create a temporary file for testing
	tempFile, err := os.CreateTemp("", "traced_file_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name()) // Clean up

	// Write initial content
	initialContent := "Initial file content for testing"
	if _, err := tempFile.WriteString(initialContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	react, err := NewTestReAct(
		aicommon.WithTracedFileContext("test_file", tempFile.Name()),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// First call - should read initial content
	ctx1 := react.promptManager.DynamicContext()
	if ctx1 == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	if !utils.MatchAllOfSubString(ctx1, "test_file", initialContent) {
		t.Fatalf("First call should contain initial file content. Got: %s", ctx1)
	}

	// Modify file content
	updatedContent := "Updated file content for testing"
	if err := os.WriteFile(tempFile.Name(), []byte(updatedContent), 0644); err != nil {
		t.Fatalf("Failed to update temp file: %v", err)
	}

	// Second call - should detect changes and show diff
	ctx2 := react.promptManager.DynamicContext()
	if ctx2 == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	if !utils.MatchAllOfSubString(ctx2, "test_file", updatedContent) {
		t.Fatalf("Second call should contain updated file content. Got: %s", ctx2)
	}

	// Second call should contain diff information
	if !utils.MatchAllOfSubString(ctx2, "CHANGES_DIFF") {
		t.Fatalf("Second call should contain diff information. Got: %s", ctx2)
	}

	t.Logf("First call context: %s", ctx1)
	t.Logf("Second call context: %s", ctx2)
}

func TestPromptManager_WithTracedFileContext_FileNotExist(t *testing.T) {
	nonExistentFile := "/tmp/non_existent_file_12345.txt"

	react, err := NewTestReAct(
		aicommon.WithTracedFileContext("non_existent", nonExistentFile),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	ctx := react.promptManager.DynamicContext()
	if ctx == "" {
		t.Fatal("Dynamic context should not be empty")
	}

	// Should contain error message for non-existent file
	if !utils.MatchAllOfSubString(ctx, "Error getting context", "does not exist") {
		t.Fatalf("Context should contain error message for non-existent file. Got: %s", ctx)
	}

	t.Logf("Context with file error: %s", ctx)
}

func TestPromptManager_WithMixedContextProviders(t *testing.T) {
	regularCallCount := 0
	tracedCallCount := 0
	var regularMutex, tracedMutex sync.Mutex

	regularProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		regularMutex.Lock()
		regularCallCount++
		regularMutex.Unlock()
		return "Regular provider content", nil
	}

	tracedProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		tracedMutex.Lock()
		tracedCallCount++
		tracedMutex.Unlock()
		return fmt.Sprintf("Traced provider content #%d", tracedCallCount), nil
	}

	react, err := NewTestReAct(
		aicommon.WithDynamicContextProvider("regular", regularProvider),
		aicommon.WithTracedDynamicContextProvider("traced", tracedProvider),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// First call
	ctx1 := react.promptManager.DynamicContext()
	if !utils.MatchAllOfSubString(ctx1, "Regular provider content", "Traced provider content #1") {
		t.Fatalf("First call should contain content from both providers. Got: %s", ctx1)
	}

	// Second call
	ctx2 := react.promptManager.DynamicContext()
	if !utils.MatchAllOfSubString(ctx2, "Regular provider content", "Traced provider content #2") {
		t.Fatalf("Second call should contain content from both providers. Got: %s", ctx2)
	}

	// Second call should contain diff for traced provider but not for regular provider
	if !utils.MatchAllOfSubString(ctx2, "CHANGES_DIFF") {
		t.Fatalf("Second call should contain diff information for traced provider. Got: %s", ctx2)
	}

	// Check call counts
	regularMutex.Lock()
	if regularCallCount != 2 {
		t.Fatalf("Regular provider should be called twice, got %d", regularCallCount)
	}
	regularMutex.Unlock()

	tracedMutex.Lock()
	if tracedCallCount != 2 {
		t.Fatalf("Traced provider should be called twice, got %d", tracedCallCount)
	}
	tracedMutex.Unlock()

	t.Logf("First call context: %s", ctx1)
	t.Logf("Second call context: %s", ctx2)
}

// Example usage of the new traced context providers
func TestExample_WithTracedDynamicContextProvider(t *testing.T) {
	// This example shows how to use the new traced context provider features

	// Create a ReAct instance with traced providers
	react, err := NewTestReAct(
		// Regular dynamic context provider (no tracing)
		aicommon.WithDynamicContextProvider("system_info", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
			return "System: Linux x86_64", nil
		}),

		// Traced dynamic context provider (tracks changes)
		aicommon.WithTracedDynamicContextProvider("user_session", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
			return fmt.Sprintf("Session active since %s", time.Now().Format("15:04:05")), nil
		}),

		// Traced file context provider (monitors file changes)
		aicommon.WithTracedFileContext("config_file", "/etc/config.yaml"),

		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "Example completed"}, "cumulative_summary": "Example summary", "human_readable_thought": "Example thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	if err != nil {
		fmt.Printf("Failed to create ReAct instance: %v\n", err)
		return
	}

	// First call - no diff information
	_ = react.promptManager.DynamicContext()
	fmt.Printf("First call includes system info and initial session time\n")

	// Wait a moment to ensure different timestamps
	time.Sleep(100 * time.Millisecond)

	// Second call - will include diff for traced providers
	_ = react.promptManager.DynamicContext()
	fmt.Printf("Second call includes changes for traced providers\n")

	// Output: First call includes system info and initial session time
	// Output: Second call includes changes for traced providers
}

// TestPromptManager_AIForgeList 测试 AIForgeList 功能
// 该测试验证：
// 1. AIForgeList 能够正确获取内置的 Forge 列表
// 2. 生成的循环提示包含 Prompt loop.txt 中的内容
// 3. 特别验证 hostscan 作为内置 aiforge 的代表
func TestPromptManager_AIForgeList(t *testing.T) {
	// 创建一个基本的 ReAct 实例来测试 AIForgeList 功能
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// 获取可用的 AI Forge 列表
	forgeList := react.promptManager.GetAvailableAIForgeBlueprints()

	// 验证 Forge 列表不为空
	if forgeList == "" {
		t.Fatal("AI Forge List should not be empty")
	}

	// 专门测试 hostscan Forge（作为内置 aiforge 的代表）
	if !utils.MatchAllOfSubString(forgeList, "hostscan") {
		t.Fatal("AI Forge list should contain hostscan forge")
	}

	// 验证 hostscan 的描述信息
	if !utils.MatchAllOfSubString(forgeList, "主机体检") {
		t.Fatal("AI Forge list should contain hostscan verbose name '主机体检'")
	}

	// 验证 hostscan 的功能描述
	if !utils.MatchAllOfSubString(forgeList, "专业的主机体检AI助手") {
		t.Fatal("AI Forge list should contain hostscan description")
	}

	t.Logf("Successfully verified AI Forge List contains hostscan forge")
}

// TestPromptManager_GenerateAIBlueprintForgeParamsPrompt 测试 GenerateAIBlueprintForgeParamsPrompt 方法
func TestPromptManager_GenerateAIBlueprintForgeParamsPrompt(t *testing.T) {
	// 创建一个基本的 ReAct 实例来测试 GenerateAIBlueprintForgeParamsPrompt 方法
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// 创建测试用的 AIForge 实例
	testAIForge := &schema.AIForge{
		ForgeName:   "test-forge",
		Description: "Test AI Forge for unit testing",
	}

	// 测试用例：基本功能测试
	t.Run("BasicFunctionality", func(t *testing.T) {
		// 定义一个简单的 schema
		schema := `{
			"type": "object",
			"properties": {
				"host": {
					"type": "string",
					"description": "目标主机地址"
				},
				"port": {
					"type": "integer",
					"description": "目标端口号"
				}
			},
			"required": ["host"]
		}`

		// 调用 GenerateAIBlueprintForgeParamsPrompt 方法
		prompt, err := react.promptManager.GenerateAIBlueprintForgeParamsPrompt(testAIForge, schema)
		if err != nil {
			t.Fatalf("Failed to generate AI blueprint forge params prompt: %v", err)
		}

		// 验证生成的内容不为空
		if prompt == "" {
			t.Fatal("Generated prompt should not be empty")
		}

		// 验证包含预期的模板内容
		if !utils.MatchAllOfSubString(prompt, "AI Blueprint Parameter Generation") {
			t.Fatal("Generated prompt should contain AI Blueprint Parameter Generation section")
		}

		if !utils.MatchAllOfSubString(prompt, "Blueprint Schema") {
			t.Fatal("Generated prompt should contain Blueprint Schema section")
		}

		if !utils.MatchAllOfSubString(prompt, "call-ai-blueprint") {
			t.Fatal("Generated prompt should contain call-ai-blueprint action")
		}

		// 验证包含传入的 schema
		if !utils.MatchAllOfSubString(prompt, "目标主机地址") {
			t.Fatal("Generated prompt should contain schema description")
		}

		// 验证包含 AIForge 的信息
		if !utils.MatchAllOfSubString(prompt, "test-forge") {
			t.Fatal("Generated prompt should contain forge name")
		}

		if !utils.MatchAllOfSubString(prompt, "Test AI Forge for unit testing") {
			t.Fatal("Generated prompt should contain forge description")
		}

		t.Logf("Generated AI Blueprint Forge Params Prompt:\n%s", prompt)
	})

	// 测试用例：空 schema 测试
	t.Run("EmptySchema", func(t *testing.T) {
		prompt, err := react.promptManager.GenerateAIBlueprintForgeParamsPrompt(testAIForge, "")
		if err != nil {
			t.Fatalf("Failed to generate prompt with empty schema: %v", err)
		}

		if prompt == "" {
			t.Fatal("Generated prompt with empty schema should not be empty")
		}

		// 验证仍然包含基本模板内容
		if !utils.MatchAllOfSubString(prompt, "AI Blueprint Parameter Generation") {
			t.Fatal("Generated prompt should contain AI Blueprint Parameter Generation section even with empty schema")
		}

		// 验证包含 AIForge 的信息
		if !utils.MatchAllOfSubString(prompt, "test-forge") {
			t.Fatal("Generated prompt should contain forge name even with empty schema")
		}
	})

	// 测试用例：上下文信息集成测试
	t.Run("ContextIntegration", func(t *testing.T) {
		// 设置一些上下文信息
		react.cumulativeSummary = "Previous task summary"
		react.currentIteration = 2
		react.config.MaxIterationCount = 10

		schema := `{"type": "object", "properties": {"test": {"type": "string"}}}`
		prompt, err := react.promptManager.GenerateAIBlueprintForgeParamsPrompt(testAIForge, schema)
		if err != nil {
			t.Fatalf("Failed to generate prompt with context: %v", err)
		}

		// 验证包含上下文信息
		if !utils.MatchAllOfSubString(prompt, "Previous task summary") {
			t.Fatal("Generated prompt should contain cumulative summary")
		}

		if !utils.MatchAllOfSubString(prompt, "2/10") {
			t.Fatal("Generated prompt should contain iteration information")
		}

		// 验证包含 AIForge 的信息
		if !utils.MatchAllOfSubString(prompt, "test-forge") {
			t.Fatal("Generated prompt should contain forge name in context test")
		}

		t.Logf("Generated prompt with context:\n%s", prompt)
	})

	// 测试用例：不同的 AIForge 实例
	t.Run("DifferentAIForge", func(t *testing.T) {
		differentAIForge := &schema.AIForge{
			ForgeName:   "hostscan-forge",
			Description: "专业的主机体检AI助手",
		}

		schema := `{"type": "object", "properties": {"target": {"type": "string"}}}`
		prompt, err := react.promptManager.GenerateAIBlueprintForgeParamsPrompt(differentAIForge, schema)
		if err != nil {
			t.Fatalf("Failed to generate prompt with different AIForge: %v", err)
		}

		// 验证包含正确的 AIForge 信息
		if !utils.MatchAllOfSubString(prompt, "hostscan-forge") {
			t.Fatal("Generated prompt should contain the different forge name")
		}

		if !utils.MatchAllOfSubString(prompt, "专业的主机体检AI助手") {
			t.Fatal("Generated prompt should contain the different forge description")
		}

		t.Logf("Generated prompt with different AIForge:\n%s", prompt)
	})
}

// TestRecommendedToolsAndForgesMatchingUserInput 测试根据用户输入推荐 tools 和 forges
func TestRecommendedToolsAndForgesMatchingUserInput(t *testing.T) {
	flag := ksuid.New().String()
	t.Run("测试 AIForge Tags 匹配", func(t *testing.T) {
		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			// 添加一些测试 forges
			aicommon.WithForges(
				&schema.AIForge{
					ForgeName:        "network_scanner",
					ForgeVerboseName: "网络扫描器",
					Description:      "扫描网络拓扑",
					Tags:             "网络,扫描,拓扑",
				},
				&schema.AIForge{
					ForgeName:        "port_scan_expert",
					ForgeVerboseName: "端口扫描专家",
					Description:      "专业的端口扫描工具",
					Tags:             flag,
				},
				&schema.AIForge{
					ForgeName:        "code_review_assistant",
					ForgeVerboseName: "代码审查助手",
					Description:      "帮助审查代码质量",
					Tags:             "代码,审查,质量",
				},
			),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task", flag))
		// 创建一个包含 "扫描" 关键词的用户输入，确保能匹配 port_scan_expert
		userInput := flag

		// 创建模拟任务
		task := newMockTaskWithUserInput("test-task", userInput)
		react.SetCurrentTask(task)

		// 获取基本提示信息，这会触发推荐逻辑
		_, forgeSlice := loop.GetRecommendedToolsAndForges()
		if err != nil {
			t.Fatalf("获取基本提示信息失败: %v", err)
		}
		// 验证 ForgesToUse 长度大于等于 3
		if len(forgeSlice) < 3 {
			t.Errorf("ForgesToUse 长度应该大于等于 3，实际: %d", len(forgeSlice))
		}

		t.Logf("推荐的 Forge 数量: %d", len(forgeSlice))

		// 验证第一个 forge 应该是 port_scan_expert（因为名称和 tag 都匹配）
		if len(forgeSlice) > 0 {
			firstForgeName := forgeSlice[0].ForgeName
			t.Logf("第一个 Forge: %s", firstForgeName)

			if firstForgeName != "port_scan_expert" {
				t.Errorf("第一个 Forge 应该是 port_scan_expert，但实际是: %s", firstForgeName)
			}

			// 打印所有推荐的 forges 以便调试
			for i, forge := range forgeSlice {
				t.Logf("  [%d] %s (%s) - Tags: %s", i, forge.ForgeName, forge.ForgeVerboseName, forge.Tags)
			}
		}

	})
	t.Run("测试 AIForge Tags 无输入时的匹配", func(t *testing.T) {
		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			// 添加一些测试 forges
			aicommon.WithForges(
				&schema.AIForge{
					ForgeName:        "network_scanner",
					ForgeVerboseName: "网络扫描器",
					Description:      "扫描网络拓扑",
					Tags:             "网络,扫描,拓扑",
				},
				&schema.AIForge{
					ForgeName:        "port_scan_expert",
					ForgeVerboseName: "端口扫描专家",
					Description:      "专业的端口扫描工具",
					Tags:             flag,
				},
				&schema.AIForge{
					ForgeName:        "code_review_assistant",
					ForgeVerboseName: "代码审查助手",
					Description:      "帮助审查代码质量",
					Tags:             "代码,审查,质量",
				},
			),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 获取基本提示信息，这会触发推荐逻辑
		_, forgeSlice := loop.GetRecommendedToolsAndForges()
		if err != nil {
			t.Fatalf("获取基本提示信息失败: %v", err)
		}
		// 验证 ForgesToUse 长度大于等于 3
		if len(forgeSlice) < 3 {
			t.Errorf("ForgesToUse 长度应该大于等于 3，实际: %d", len(forgeSlice))
		}

		t.Logf("推荐的 Forge 数量: %d", len(forgeSlice))

		// 验证第一个 forge 应该是 port_scan_expert（因为名称和 tag 都匹配）
		if len(forgeSlice) > 0 {
			firstForgeName := forgeSlice[0].ForgeName
			t.Logf("第一个 Forge: %s", firstForgeName)

			if firstForgeName != "network_scanner" {
				t.Errorf("第一个 Forge 应该是 port_scan_expert，但实际是: %s", firstForgeName)
			}

			// 打印所有推荐的 forges 以便调试
			for i, forge := range forgeSlice {
				t.Logf("  [%d] %s (%s) - Tags: %s", i, forge.ForgeName, forge.ForgeVerboseName, forge.Tags)
			}
		}

	})

	t.Run("测试_Forge_回退机制_没有匹配时返回所有", func(t *testing.T) {
		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			aicommon.WithForges(
				&schema.AIForge{
					ForgeName:        "test_forge_1",
					ForgeVerboseName: "测试Forge1",
					Description:      "第一个测试forge",
					Tags:             "测试,forge",
				},
				&schema.AIForge{
					ForgeName:        "test_forge_2",
					ForgeVerboseName: "测试Forge2",
					Description:      "第二个测试forge",
					Tags:             "测试,工具",
				},
				&schema.AIForge{
					ForgeName:        "test_forge_3",
					ForgeVerboseName: "测试Forge3",
					Description:      "第三个测试forge",
					Tags:             "扫描,安全",
				},
			),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop-fallback", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 使用一个完全不匹配的用户输入
		userInput := uuid.New().String()
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-fallback", userInput))

		// 获取推荐的 tools 和 forges
		_, forgeSlice := loop.GetRecommendedToolsAndForges()

		// 验证返回了 forges（没有匹配时应该回退到返回所有可用的）
		if len(forgeSlice) == 0 {
			t.Errorf("没有匹配时应该返回所有可用的 forges，实际返回: 0")
		}

		// 验证包含所有测试 forges（测试的 3 个应该在前面）
		forgeNames := make(map[string]bool)
		for _, forge := range forgeSlice {
			forgeNames[forge.ForgeName] = true
		}

		expectedForges := []string{"test_forge_1", "test_forge_2", "test_forge_3"}
		for _, expected := range expectedForges {
			if !forgeNames[expected] {
				t.Errorf("应该包含 forge: %s", expected)
			}
		}

		// 验证测试 forges 在前面位置（因为是通过 WithForges 添加的）
		foundCount := 0
		for i := 0; i < 3 && i < len(forgeSlice); i++ {
			for _, expected := range expectedForges {
				if forgeSlice[i].ForgeName == expected {
					foundCount++
					break
				}
			}
		}

		if foundCount < 3 {
			t.Fatalf("警告: 前 3 个位置只找到 %d 个测试 forges", foundCount)
		}

		t.Logf("用户输入: %s", userInput)
		t.Logf("返回的 forge 数量: %d", len(forgeSlice))
		for i, forge := range forgeSlice {
			t.Logf("  [%d] %s (%s)", i, forge.ForgeName, forge.ForgeVerboseName)
		}
	})

	t.Run("测试工具数量限制", func(t *testing.T) {
		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop-limit", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 设置自定义的工具数量限制为 10
		loop.Set("max_tools_limit", 10)

		// 创建一个通用的用户输入
		userInput := "帮我分析一下"
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-limit", userInput))

		// 获取推荐的工具
		toolSlice, _ := loop.GetRecommendedToolsAndForges()

		toolCount := len(toolSlice)
		t.Logf("推荐的工具数量: %d", toolCount)

		// 验证工具数量不超过 10（我们设置的限制）
		if toolCount > 10 {
			t.Errorf("工具数量 %d 超过了限制 10", toolCount)
		}

		// 验证优先工具在前面
		if toolCount > 0 {
			firstToolName := toolSlice[0].Name
			t.Logf("第一个工具: %s", firstToolName)

			// 检查前几个工具是否包含优先工具
			priorityTools := []string{"tools_search", "aiforge_search", "now", "bash", "read_file"}
			foundPriority := false
			checkCount := 5
			if toolCount < checkCount {
				checkCount = toolCount
			}
			for i := 0; i < checkCount; i++ {
				for _, priorityName := range priorityTools {
					if toolSlice[i].Name == priorityName {
						foundPriority = true
						t.Logf("找到优先工具: %s 在位置 %d", toolSlice[i].Name, i)
						break
					}
				}
			}

			if !foundPriority {
				t.Fatalf("警告: 前 5 个工具中没有找到优先工具")
			}
		}

		t.Logf("用户输入: %s", userInput)
		t.Logf("验证工具数量限制为 10 个，且优先工具排在前面")
	})

	t.Run("测试工具匹配和数量限制", func(t *testing.T) {
		// 创建一些测试工具
		testTools := []*aitool.Tool{
			aitool.NewWithoutCallback("file_scanner",
				aitool.WithVerboseName("文件扫描器"),
				aitool.WithKeywords([]string{"扫描", "文件"}),
				aitool.WithDescription("扫描文件系统"),
			),
			aitool.NewWithoutCallback("network_analyzer",
				aitool.WithVerboseName("网络分析器"),
				aitool.WithKeywords([]string{"网络", "分析"}),
				aitool.WithDescription("分析网络流量"),
			),
			aitool.NewWithoutCallback(flag+"_tool",
				aitool.WithVerboseName("特殊工具"),
				aitool.WithKeywords([]string{flag}),
				aitool.WithDescription("特殊功能工具"),
			),
		}

		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			aicommon.WithTools(testTools...),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop-tool-match", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 设置工具数量限制为 5
		loop.Set("max_tools_limit", 5)

		// 使用包含特殊标记的用户输入
		userInput := flag
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-tool-match", userInput))

		// 获取推荐的工具
		toolSlice, _ := loop.GetRecommendedToolsAndForges()

		t.Logf("推荐的工具数量: %d", len(toolSlice))

		// 验证工具数量不超过限制
		if len(toolSlice) > 5 {
			t.Errorf("工具数量 %d 超过了限制 5", len(toolSlice))
		}

		// 验证匹配的工具在前面
		if len(toolSlice) > 0 {
			found := false
			for i, tool := range toolSlice {
				t.Logf("  [%d] %s (%s)", i, tool.Name, tool.VerboseName)
				if tool.Name == flag+"_tool" {
					found = true
					if i > 3 {
						t.Fatalf("警告: 匹配的工具位置较靠后: %d", i)
					} else {
						t.Logf("匹配的工具在前面位置: %d", i)
					}
				}
			}

			if !found {
				t.Fatalf("警告: 未找到匹配的工具")
			}
		}

		t.Logf("用户输入: %s", userInput)
		t.Logf("验证工具匹配和数量限制")
	})

	t.Run("测试Forge数量限制", func(t *testing.T) {
		flag := ksuid.New().String()
		// 创建多个测试 forges
		var testForges []*schema.AIForge
		for i := 1; i <= 10; i++ {
			testForges = append(testForges, &schema.AIForge{
				ForgeName:        fmt.Sprintf("test_forge_%d", i),
				ForgeVerboseName: fmt.Sprintf("测试Forge%d", i),
				Description:      fmt.Sprintf("第%d个测试forge", i),
				Tags:             flag,
			})
		}

		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			aicommon.WithForges(testForges...),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop-forge-limit", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 设置 forge 数量限制为 5
		loop.Set("max_forges_limit", 5)

		// 创建一个通用的用户输入
		userInput := flag
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-forge-limit", userInput))

		// 获取推荐的 forges
		_, forgeSlice := loop.GetRecommendedToolsAndForges()

		t.Logf("推荐的 forge 数量: %d", len(forgeSlice))

		// 验证 forge 数量不超过 5
		if len(forgeSlice) > 5 {
			t.Errorf("Forge 数量 %d 超过了限制 5", len(forgeSlice))
		}

		for i, forge := range forgeSlice {
			t.Logf("  [%d] %s (%s)", i, forge.ForgeName, forge.ForgeVerboseName)
		}

		t.Logf("用户输入: %s", userInput)
		t.Logf("验证 forge 数量限制为 5 个")
	})

	t.Run("测试多次匹配优先级_新匹配的在最前面", func(t *testing.T) {
		flag1 := ksuid.New().String()
		flag2 := ksuid.New().String()

		react, err := NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
			// 创建多个测试 forges，使用不同的 tags
			aicommon.WithForges(
				&schema.AIForge{
					ForgeName:        "forge_group_a_1",
					ForgeVerboseName: "组A的Forge1",
					Description:      "属于组A",
					Tags:             flag1,
				},
				&schema.AIForge{
					ForgeName:        "forge_group_a_2",
					ForgeVerboseName: "组A的Forge2",
					Description:      "属于组A",
					Tags:             flag1,
				},
				&schema.AIForge{
					ForgeName:        "forge_group_b_1",
					ForgeVerboseName: "组B的Forge1",
					Description:      "属于组B",
					Tags:             flag2,
				},
				&schema.AIForge{
					ForgeName:        "forge_group_b_2",
					ForgeVerboseName: "组B的Forge2",
					Description:      "属于组B",
					Tags:             flag2,
				},
				&schema.AIForge{
					ForgeName:        "forge_unmatched",
					ForgeVerboseName: "不匹配的Forge",
					Description:      "不会被匹配",
					Tags:             "other,tags",
				},
			),
		)
		if err != nil {
			t.Fatalf("创建 ReAct 实例失败: %v", err)
		}

		loop, err := reactloops.NewReActLoop("test-loop-priority", react)
		if err != nil {
			t.Fatalf("创建 ReActLoop 实例失败: %v", err)
		}

		// 第一次匹配：使用 flag1，应该匹配 forge_group_a_1 和 forge_group_a_2
		t.Logf("=== 第一次匹配 ===")
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-1", flag1))
		_, forgeSlice1 := loop.GetRecommendedToolsAndForges()

		t.Logf("第一次匹配结果（匹配 flag1=%s）:", flag1)
		for i, forge := range forgeSlice1 {
			t.Logf("  [%d] %s (%s) - Tags: %s", i, forge.ForgeName, forge.ForgeVerboseName, forge.Tags)
		}

		// 验证第一次匹配：组A的 forges 应该在前面
		if len(forgeSlice1) < 2 {
			t.Fatalf("第一次匹配应该至少返回 2 个 forges")
		}

		groupACount := 0
		for i := 0; i < 2 && i < len(forgeSlice1); i++ {
			if forgeSlice1[i].ForgeName == "forge_group_a_1" || forgeSlice1[i].ForgeName == "forge_group_a_2" {
				groupACount++
			}
		}

		if groupACount != 2 {
			t.Errorf("第一次匹配：前 2 个应该都是组A的 forges，实际找到 %d 个", groupACount)
		}

		// 第二次匹配：使用 flag2，应该匹配 forge_group_b_1 和 forge_group_b_2
		t.Logf("\n=== 第二次匹配 ===")
		loop.SetCurrentTask(newMockTaskWithUserInput("test-task-2", flag2))
		_, forgeSlice2 := loop.GetRecommendedToolsAndForges()

		t.Logf("第二次匹配结果（匹配 flag2=%s）:", flag2)
		for i, forge := range forgeSlice2 {
			t.Logf("  [%d] %s (%s) - Tags: %s", i, forge.ForgeName, forge.ForgeVerboseName, forge.Tags)
		}

		// 验证第二次匹配：
		// 1. 组B的 forges 应该在最前面（新匹配的）
		// 2. 组A的 forges 应该在后面（上次匹配的）
		if len(forgeSlice2) < 4 {
			t.Fatalf("第二次匹配应该至少返回 4 个 forges（2个组B + 2个组A）")
		}

		// 验证前 2 个是组B的 forges
		groupBInFirst2 := 0
		for i := 0; i < 2; i++ {
			if forgeSlice2[i].ForgeName == "forge_group_b_1" || forgeSlice2[i].ForgeName == "forge_group_b_2" {
				groupBInFirst2++
			}
		}

		if groupBInFirst2 != 2 {
			t.Errorf("第二次匹配：前 2 个应该都是组B的 forges（新匹配的），实际找到 %d 个", groupBInFirst2)
			t.Errorf("前 2 个 forges: [0]=%s, [1]=%s", forgeSlice2[0].ForgeName, forgeSlice2[1].ForgeName)
		}

		// 验证第 3、4 个位置包含组A的 forges（上次匹配的）
		groupAIn3And4 := 0
		for i := 2; i < 4 && i < len(forgeSlice2); i++ {
			if forgeSlice2[i].ForgeName == "forge_group_a_1" || forgeSlice2[i].ForgeName == "forge_group_a_2" {
				groupAIn3And4++
			}
		}

		if groupAIn3And4 != 2 {
			t.Fatalf("警告: 第 3、4 个位置应该是组A的 forges（上次匹配的），实际找到 %d 个", groupAIn3And4)
			if len(forgeSlice2) >= 4 {
				t.Logf("第 3、4 个 forges: [2]=%s, [3]=%s", forgeSlice2[2].ForgeName, forgeSlice2[3].ForgeName)
			}
		}

		t.Logf("\n验证完成：新匹配的 forges 排在最前面，上次匹配的排在后面")
	})
}
