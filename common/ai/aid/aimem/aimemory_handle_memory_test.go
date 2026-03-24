package aimem

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"

	"github.com/google/uuid"
)

func TestHandleMemory_Basic(t *testing.T) {
	sessionID := "handle-memory-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 测试处理新记忆
	testInput := "我正在学习Go语言的并发编程，特别是goroutine和channel的使用"

	err = memory.HandleMemory(testInput)
	if err != nil {
		t.Fatalf("handle memory failed: %v", err)
	}

	// 验证记忆是否被保存
	allMemories, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}

	if len(allMemories) == 0 {
		t.Fatalf("expected memories to be saved, got none")
	}

	t.Logf("successfully handled memory, saved %d entities", len(allMemories))

	// 验证记忆内容
	for i, mem := range allMemories {
		t.Logf("memory %d: %s (tags: %v)", i+1, mem.Content, mem.Tags)
	}
}

func TestHandleMemory_Deduplication(t *testing.T) {
	sessionID := "handle-dedup-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 第一次处理记忆
	testInput1 := "Go语言是Google开发的编程语言"
	err = memory.HandleMemory(testInput1)
	if err != nil {
		t.Fatalf("first handle memory failed: %v", err)
	}

	// 第二次处理类似的记忆（应该被去重）
	testInput2 := "Go语言是由Google公司开发的编程语言"
	err = memory.HandleMemory(testInput2)
	if err != nil {
		t.Fatalf("second handle memory failed: %v", err)
	}

	// 验证去重效果
	allMemories, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}

	t.Logf("after deduplication test, total memories: %d", len(allMemories))

	// 第三次处理完全不同的记忆
	testInput3 := "Python是一种高级编程语言，适合数据分析"
	err = memory.HandleMemory(testInput3)
	if err != nil {
		t.Fatalf("third handle memory failed: %v", err)
	}

	// 验证新记忆被添加
	allMemoriesAfter, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories after third input failed: %v", err)
	}

	if len(allMemoriesAfter) <= len(allMemories) {
		t.Logf("warning: expected more memories after adding different content, but deduplication might have filtered it")
	}

	t.Logf("final memory count: %d", len(allMemoriesAfter))
}

func TestHandleMemory_PromptContainsDurableMemoryRules(t *testing.T) {
	sessionID := "handle-prompt-rules-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	mockInvoker.SetPromptValidator("memory-triage", func(prompt string) bool {
		return strings.Contains(prompt, "Do NOT create memory for one-off events") &&
			strings.Contains(prompt, "Do NOT use pronouns or deictic references") &&
			strings.Contains(prompt, "return an empty memory_entities array")
	})

	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	err = memory.HandleMemory("用户要求后续输出保持结构化并避免记录过程性垃圾记忆")
	if err != nil {
		t.Fatalf("handle memory failed: %v", err)
	}
}

func TestHandleMemory_RejectTransientVisitEvent(t *testing.T) {
	sessionID := "handle-transient-event-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	mockInvoker.SetReturnValue("memory-triage", `{
		"@action": "memory-triage",
		"memory_entities": [
			{
				"content": "用户在 10:20 访问 www.example.com 并查看了一次结果页面",
				"tags": ["browsing", "event"],
				"potential_questions": ["用户在 10:20 访问了什么网址？"],
				"t": 0.9, "a": 0.4, "p": 0.2, "o": 0.9, "e": 0.5, "r": 0.3, "c": 0.2
			}
		]
	}`)

	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	err = memory.HandleMemory("用户在 10:20 访问 www.example.com 并查看了一次结果页面")
	if err != nil {
		t.Fatalf("handle memory failed: %v", err)
	}

	allMemories, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if len(allMemories) != 0 {
		t.Fatalf("expected transient visit event to be rejected, got %d memories", len(allMemories))
	}
}

func TestHandleMemory_RejectAmbiguousPronounMemory(t *testing.T) {
	sessionID := "handle-pronoun-memory-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	mockInvoker.SetReturnValue("memory-triage", `{
		"@action": "memory-triage",
		"memory_entities": [
			{
				"content": "这次需要继续使用这个方案",
				"tags": ["plan"],
				"potential_questions": ["这次要继续用什么方案？"],
				"t": 0.8, "a": 0.8, "p": 0.7, "o": 0.9, "e": 0.5, "r": 0.8, "c": 0.7
			}
		]
	}`)

	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	err = memory.HandleMemory("这次需要继续使用这个方案")
	if err != nil {
		t.Fatalf("handle memory failed: %v", err)
	}

	allMemories, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if len(allMemories) != 0 {
		t.Fatalf("expected ambiguous pronoun memory to be rejected, got %d memories", len(allMemories))
	}
}

func TestHandleMemory_KeepDurableGeneralizedFact(t *testing.T) {
	sessionID := "handle-durable-fact-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	mockInvoker.SetReturnValue("memory-triage", `{
		"@action": "memory-triage",
		"memory_entities": [
			{
				"content": "用户偏好结构化输出，回答应包含小标题与要点列表。",
				"tags": ["format", "preference"],
				"potential_questions": ["用户偏好的回答结构是什么？"],
				"t": 0.9, "a": 0.9, "p": 0.9, "o": 0.9, "e": 0.6, "r": 0.8, "c": 0.8
			}
		]
	}`)

	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	err = memory.HandleMemory("用户偏好结构化输出，回答应包含小标题与要点列表。")
	if err != nil {
		t.Fatalf("handle memory failed: %v", err)
	}

	allMemories, err := memory.ListAllMemories(10)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if len(allMemories) != 1 {
		t.Fatalf("expected durable generalized fact to be kept, got %d memories", len(allMemories))
	}
}
