package aireact

import (
	"bytes"
	"fmt"
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
