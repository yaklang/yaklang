package aireact

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPromptManager_WithDynamicContextProvider(t *testing.T) {
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
	react, err := NewReAct(
		WithDynamicContextProvider("test_provider", mockProvider),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

func TestPromptManager_WithDynamicContextProvider_MultipleProviders(t *testing.T) {
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
	react, err := NewReAct(
		WithDynamicContextProvider("provider1", provider1),
		WithDynamicContextProvider("provider2", provider2),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

func TestPromptManager_WithDynamicContextProvider_ErrorHandling(t *testing.T) {
	// Provider that returns an error
	errorProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "", fmt.Errorf("mock provider error")
	}

	// Normal provider
	normalProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "Normal context", nil
	}

	react, err := NewReAct(
		WithDynamicContextProvider("error_provider", errorProvider),
		WithDynamicContextProvider("normal_provider", normalProvider),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

func TestPromptManager_WithDynamicContextProvider_InPromptGeneration(t *testing.T) {
	providerCalled := false
	var callMutex sync.Mutex

	mockProvider := func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		callMutex.Lock()
		providerCalled = true
		callMutex.Unlock()
		return "Context for prompt generation", nil
	}

	react, err := NewReAct(
		WithDynamicContextProvider("prompt_test", mockProvider),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Test that DynamicContext is called during prompt generation
	_, err = react.promptManager.GenerateLoopPrompt("test query", true, true, 0, 5, nil)
	if err != nil {
		t.Fatalf("Failed to generate loop prompt: %v", err)
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

	_, err = react.promptManager.GenerateDirectlyAnswerPrompt("test query", nil)
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

	react, err := NewReAct(
		WithTracedDynamicContextProvider("traced_provider", mockProvider),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

	react, err := NewReAct(
		WithTracedFileContext("test_file", tempFile.Name()),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

	react, err := NewReAct(
		WithTracedFileContext("non_existent", nonExistentFile),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
	if !utils.MatchAllOfSubString(ctx, "Error getting context", "failed to read file") {
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

	react, err := NewReAct(
		WithDynamicContextProvider("regular", regularProvider),
		WithTracedDynamicContextProvider("traced", tracedProvider),
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
	react, err := NewReAct(
		// Regular dynamic context provider (no tracing)
		WithDynamicContextProvider("system_info", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
			return "System: Linux x86_64", nil
		}),

		// Traced dynamic context provider (tracks changes)
		WithTracedDynamicContextProvider("user_session", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
			return fmt.Sprintf("Session active since %s", time.Now().Format("15:04:05")), nil
		}),

		// Traced file context provider (monitors file changes)
		WithTracedFileContext("config_file", "/etc/config.yaml"),

		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
	react, err := NewReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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

	// 生成包含 Forge 列表的循环提示
	prompt, err := react.promptManager.GenerateLoopPrompt("test query", true, true, 0, 5, nil)
	if err != nil {
		t.Fatalf("Failed to generate loop prompt: %v", err)
	}

	// 记录 Forge 列表内容，用于调试和了解有哪些 Forge
	t.Logf("Prompt Include AI Forge List:\n%s", prompt)

	// 记录生成的完整提示，用于调试
	t.Logf("Generated prompt length: %d", len(prompt))

	// 验证生成的提示包含 Forge 列表相关的文本
	if !utils.MatchAllOfSubString(prompt, "AI Bluprint(AI Forge)") {
		t.Fatal("Generated prompt should contain AI Forge section")
	}

	// 验证生成的提示包含 Forge 列表内容的关键部分
	if !utils.MatchAllOfSubString(prompt, "hostscan") {
		t.Fatal("Generated prompt should contain hostscan forge")
	}

	// 验证生成的提示包含多个 Forge
	if !utils.MatchAllOfSubString(prompt, "xss", "sqlinject") {
		t.Fatal("Generated prompt should contain multiple forges")
	}

	// 验证生成的提示包含 loop.txt 中的 Forge 相关文本
	if !utils.MatchAllOfSubString(prompt, "You have access to AI Forge") {
		t.Fatal("Generated prompt should contain AI Forge introduction text from loop.txt")
	}

	if !utils.MatchAllOfSubString(prompt, "repository of AI blueprints") {
		t.Fatal("Generated prompt should contain AI Forge description from loop.txt")
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
