package aimem

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSearchMemory_Basic(t *testing.T) {
	sessionID := "search-memory-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 先添加一些测试记忆
	testInputs := []string{
		"Go语言的并发编程使用goroutine和channel",
		"Python适合数据分析和机器学习",
		"JavaScript是前端开发的主要语言",
		"数据库设计需要考虑范式和索引优化",
	}

	for _, input := range testInputs {
		err = memory.HandleMemory(input)
		if err != nil {
			t.Fatalf("handle memory failed for input '%s': %v", input, err)
		}
		// 添加小延迟避免时间戳冲突
		time.Sleep(10 * time.Millisecond)
	}

	// 测试搜索功能
	searchQuery := "编程语言"
	bytesLimit := 1000

	result, err := memory.SearchMemory(searchQuery, bytesLimit)
	if err != nil {
		t.Fatalf("search memory failed: %v", err)
	}

	// 验证搜索结果
	if result == nil {
		t.Fatalf("search result is nil")
	}

	t.Logf("search results for '%s':", searchQuery)
	t.Logf("  found %d memories", len(result.Memories))
	t.Logf("  total content bytes: %d (limit: %d)", result.ContentBytes, bytesLimit)
	t.Logf("  search summary: %s", result.SearchSummary)

	// 验证字节限制
	if result.ContentBytes > bytesLimit {
		t.Errorf("content bytes %d exceeds limit %d", result.ContentBytes, bytesLimit)
	}

	// 验证内容不为空（如果找到了记忆）
	if len(result.Memories) > 0 && result.TotalContent == "" {
		t.Errorf("found memories but total content is empty")
	}

	// 打印搜索到的记忆内容
	for i, mem := range result.Memories {
		t.Logf("  memory %d: %s (tags: %v, relevance scores: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f)",
			i+1, utils.ShrinkString(mem.Content, 50), mem.Tags,
			mem.C_Score, mem.O_Score, mem.R_Score, mem.E_Score, mem.P_Score, mem.A_Score, mem.T_Score)
	}
}

func TestSearchMemory_BytesLimit(t *testing.T) {
	sessionID := "search-bytes-limit-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 如果RAG系统不可用，跳过这个测试
	if memory.rag == nil {
		t.Skip("RAG system not initialized (embedding service unavailable)")
	}

	// 添加一些较长的测试记忆
	longInputs := []string{
		"Go语言的并发编程模型基于CSP（Communicating Sequential Processes）理论，通过goroutine和channel实现轻量级并发。Goroutine是Go语言的轻量级线程，可以在单个线程上运行数千个goroutine。Channel是goroutine之间通信的管道，支持同步和异步通信模式。",
		"Python是一种解释型、面向对象、动态数据类型的高级程序设计语言。Python语法简洁清晰，特色之一是强制用空白符作为语句缩进。Python具有丰富和强大的库，常被称为胶水语言，能够把用其他语言制作的各种模块很轻松地联结在一起。",
		"JavaScript是一种具有函数优先的轻量级、解释型或即时编译型的编程语言。虽然它是作为开发Web页面的脚本语言而出名，但是它也被用到了很多非浏览器环境中。JavaScript基于原型编程、多范式的动态脚本语言，并且支持面向对象、命令式和声明式风格。",
	}

	for _, input := range longInputs {
		err = memory.HandleMemory(input)
		if err != nil {
			t.Fatalf("handle memory failed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 测试不同的字节限制
	testLimits := []int{200, 500, 1000, 2000}

	for _, limit := range testLimits {
		result, err := memory.SearchMemory("编程语言特点", limit)
		if err != nil {
			t.Fatalf("search memory failed with limit %d: %v", limit, err)
		}

		t.Logf("bytes limit %d: found %d memories, actual bytes: %d",
			limit, len(result.Memories), result.ContentBytes)

		// 验证字节限制
		if result.ContentBytes > limit {
			t.Errorf("content bytes %d exceeds limit %d", result.ContentBytes, limit)
		}

		// 验证随着限制增加，内容应该不减少
		if limit > 200 && result.ContentBytes == 0 {
			t.Errorf("expected some content with limit %d, got 0 bytes", limit)
		}
	}
}

func TestSearchMemory_EmptyQuery(t *testing.T) {
	sessionID := "search-empty-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 测试空查询
	result, err := memory.SearchMemory("", 1000)
	if err != nil {
		t.Fatalf("search memory with empty query failed: %v", err)
	}

	// 验证空查询结果
	if result == nil {
		t.Fatalf("search result is nil")
	}

	if len(result.Memories) != 0 {
		t.Errorf("expected no memories for empty query, got %d", len(result.Memories))
	}

	if result.ContentBytes != 0 {
		t.Errorf("expected 0 content bytes for empty query, got %d", result.ContentBytes)
	}

	if result.TotalContent != "" {
		t.Errorf("expected empty total content for empty query, got: %s", result.TotalContent)
	}

	t.Logf("empty query handled correctly: %s", result.SearchSummary)
}

func cleanupEntryTestData(t *testing.T, sessionID string) {
	db := consts.GetGormProjectDatabase()
	if db != nil {
		// 清理测试数据
		if err := db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{}).Error; err != nil {
			t.Logf("cleanup AIMemoryEntity failed: %v", err)
		}
		if err := db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryCollection{}).Error; err != nil {
			t.Logf("cleanup AIMemoryCollection failed: %v", err)
		}
	}
}
