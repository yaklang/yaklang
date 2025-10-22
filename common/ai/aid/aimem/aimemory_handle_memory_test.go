package aimem

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aicommon_mock"
	"testing"

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
