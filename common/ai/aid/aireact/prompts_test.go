package aireact

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type promptGoalTask struct {
	*aicommon.AIStatefulTaskBase
	goal string
}

func (t *promptGoalTask) GetGoal() string {
	if t == nil {
		return ""
	}
	return t.goal
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

	_, _, err = react.GetBasicPromptInfo(nil)
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

func TestPromptManager_DynamicContextWithNonce_RendersPrevUserInputAITag(t *testing.T) {
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

	react.config.SetUserInputHistory([]schema.AIAgentUserInputRecord{
		{Round: 1, Timestamp: time.Date(2026, 3, 26, 10, 0, 0, 0, time.Local), UserInput: "first input"},
		{Round: 2, Timestamp: time.Date(2026, 3, 26, 10, 5, 0, 0, time.Local), UserInput: "second input"},
	})

	nonce := "nonce1234"
	ctx := react.promptManager.DynamicContextWithNonce(nonce)
	if !utils.MatchAllOfSubString(ctx,
		"<|PREV_USER_INPUT_"+nonce+"|>",
		"<|PREV_USER_INPUT_END_"+nonce+"|>",
		"Round 1",
		"Round 2",
		"Time:",
		"first input",
		"second input",
	) {
		t.Fatalf("dynamic context should contain PREV_USER_INPUT AITAG with history. Got:\n%s", ctx)
	}
	if strings.Contains(ctx, "[TRUNCATED_HEAD]") {
		t.Fatalf("dynamic context should not be truncated for small history. Got:\n%s", ctx)
	}
	plainCtx := react.promptManager.DynamicContext()
	if strings.Contains(plainCtx, "PREV_USER_INPUT_") {
		t.Fatalf("plain dynamic context should not contain PREV_USER_INPUT AITAG. Got:\n%s", plainCtx)
	}
}

func TestPromptManager_DynamicContextWithNonce_TruncatesPrevUserInputHistoryTo20K(t *testing.T) {
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

	// Each "word_NNN " produces ~2 tokens. Need total > 20K tokens across both entries.
	var oldParts []string
	for i := 0; i < 15000; i++ {
		oldParts = append(oldParts, fmt.Sprintf("old_%d", i))
	}
	oldPayload := "_old_marker " + strings.Join(oldParts, " ")
	newPayload := strings.Repeat("B", 500) + " _new_marker"
	react.config.SetUserInputHistory([]schema.AIAgentUserInputRecord{
		{Round: 1, Timestamp: time.Date(2026, 3, 26, 10, 0, 0, 0, time.Local), UserInput: oldPayload},
		{Round: 2, Timestamp: time.Date(2026, 3, 26, 10, 5, 0, 0, time.Local), UserInput: newPayload},
	})

	ctx := react.promptManager.DynamicContextWithNonce("cut9876")
	if !strings.Contains(ctx, "[TRUNCATED_HEAD]") {
		t.Fatalf("dynamic context should mark head truncation when history exceeds 20K. Got length=%d", len(ctx))
	}
	if strings.Contains(ctx, oldPayload) || strings.Contains(ctx, "_old_marker") {
		t.Fatalf("oldest head content should be truncated from PREV_USER_INPUT block")
	}
	if !strings.Contains(ctx, "_new_marker") || !strings.Contains(ctx, "Round 2") {
		t.Fatalf("latest history content should be preserved after truncation. Got:\n%s", ctx)
	}
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

func TestGenerateVerificationPrompt_RendersTodoSnapshot(t *testing.T) {
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

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		Reasoning: "任务未完成",
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "create_file", Content: "创建一个 A.md 文件"},
			{Op: "add", ID: "rename_file", Content: "将临时文件改名为最终文件名"},
			{Op: "add", ID: "obsolete_step", Content: "不再需要的临时步骤"},
		},
	})
	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		Reasoning: "阶段推进",
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "done", ID: "rename_file"},
			{Op: "delete", ID: "obsolete_step"},
			{Op: "add", ID: "verify_file", Content: "检查 A.md 是否已写入预期内容"},
		},
	})

	prompt, _, err := react.promptManager.GenerateVerificationPrompt("请创建并验证 A.md", true, "tool executed: create file")
	if err != nil {
		t.Fatalf("Failed to generate verification prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"# TODO:",
		"- [ ]: [id: verify_file]: 检查 A.md 是否已写入预期内容",
		"- [ ]: [id: create_file]: 创建一个 A.md 文件",
		"- [DELETED]: [id: obsolete_step]: 不再需要的临时步骤",
		"- [x]: [id: rename_file]: 将临时文件改名为最终文件名",
		"next_movements 只输出增量",
		"{\"op\": \"doing\", \"id\": \"stable_id\"}",
		"{\"op\": \"add\", \"id\": \"stable_id\", \"content\": \"新增一个短链路 TODO\"}",
		"doing 也必须具体，不得推进模糊 TODO",
		"禁止在 TODO 内容里使用“等”“相关”“若干”“剩余接口”来省略具体对象",
	) {
		t.Fatalf("verification prompt should contain structured TODO snapshot and incremental instructions. Got:\n%s", prompt)
	}
}

func TestGenerateVerificationPrompt_RendersAbandonedTodosAfterSatisfied(t *testing.T) {
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

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		Reasoning: "任务未完成",
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "collect_signal", Content: "收集页面回显信号"},
			{Op: "add", ID: "retry_payload", Content: "更换 payload 再次验证"},
		},
	})
	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied:     true,
		Reasoning:     "目标达成",
		NextMovements: []aicommon.VerifyNextMovement{},
	})

	prompt, _, err := react.promptManager.GenerateVerificationPrompt("观察页面回显", false, "已成功得到完整响应")
	if err != nil {
		t.Fatalf("Failed to generate verification prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"- [SKIPPED]: [id: collect_signal]: 收集页面回显信号",
		"- [SKIPPED]: [id: retry_payload]: 更换 payload 再次验证",
		"系统会自动把剩余未关闭 TODO 标记为 `SKIPPED`",
		"`next_movements` 必须输出 `[]`",
	) {
		t.Fatalf("verification prompt should render abandoned TODO items after satisfaction. Got:\n%s", prompt)
	}
}

func TestGenerateVerificationPrompt_TruncatesLongTodoSnapshotButKeepsFocus(t *testing.T) {
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

	for index := 0; index < 80; index++ {
		react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{{
				Op:      "add",
				ID:      fmt.Sprintf("todo_%03d", index),
				Content: strings.Repeat("非常长的待办描述", 30),
			}},
		})
	}
	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{{
			Op:      "add",
			ID:      "active_focus",
			Content: "优先保留这个焦点 TODO",
		}},
	})

	prompt, _, err := react.promptManager.GenerateVerificationPrompt("请继续推进当前焦点任务", true, "tool executed: continue")
	if err != nil {
		t.Fatalf("Failed to generate verification prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"# TODO:",
		"active_focus",
		"TODO history exceeded 10K tokens",
	) {
		t.Fatalf("verification prompt should truncate long TODO history but keep latest focus item. Got:\n%s", prompt)
	}
}

func TestGenerateVerificationPrompt_IncludesCurrentTaskGoal(t *testing.T) {
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

	task := &promptGoalTask{
		AIStatefulTaskBase: aicommon.NewStatefulTaskBase("verify-goal-task", "当前任务输入", context.Background(), react.Emitter, true),
		goal: "待测列表：\n- /api/user/1\n- /api/user/2\n验收标准：\n- 逐条记录执行结果\n- 给出明确结论",
	}
	task.SetName("验证用户接口")
	react.SetCurrentTask(task)

	prompt, _, err := react.promptManager.GenerateVerificationPrompt("请继续推进当前任务", true, "tool executed: continue")
	if err != nil {
		t.Fatalf("Failed to generate verification prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"<|CURRENT_TASK_GOAL_",
		"当前任务名称: 验证用户接口",
		"待测列表：",
		"验收标准：",
	) {
		t.Fatalf("verification prompt should include current task goal. Got:\n%s", prompt)
	}
}

func TestGenerateIntervalReviewPrompt_IncludesConcreteStringGuidance(t *testing.T) {
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

	tool := aitool.NewWithoutCallback(
		"network_diagnose",
		aitool.WithStringParam("target"),
	)

	prompt, err := react.promptManager.GenerateIntervalReviewPromptWithContext(
		tool,
		aitool.InvokeParams{"target": "127.0.0.1"},
		[]byte("partial output"),
		nil,
		time.Unix(0, 0),
		1,
		"expect structured diagnostics",
	)
	if err != nil {
		t.Fatalf("Failed to generate interval review prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"must be concrete natural-language strings",
		"Do not copy schema metadata such as",
		"Example valid response:",
		`"progress_summary": "已完成网络接口、监听端口与路由配置的采集，正在补充协议与监控指标。"`,
	) {
		t.Fatalf("interval review prompt should contain anti-schema guidance and concrete example. Got:\n%s", prompt)
	}
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
		react.AddToTimeline("note", "Previous task summary")
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
