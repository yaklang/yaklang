package aimem

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/log"
)

// TestSearchMemory_TimelineOrdering 测试记忆搜索结果的时间顺序
// 确保返回的记忆按照创建时间从旧到新排序
func TestSearchMemory_TimelineOrdering(t *testing.T) {
	sessionID := "timeline-order-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	require.NoError(t, err)
	defer memory.Close()

	// 按特定顺序添加记忆，确保时间戳递增
	testMemories := []struct {
		content   string
		waitTime  time.Duration // 等待时间，确保时间戳不同
		timestamp time.Time
	}{
		{
			content:  "第一条记忆：学习Go语言的基础语法",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第二条记忆：掌握Go的并发编程模式",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第三条记忆：深入理解Go的内存模型",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第四条记忆：实践Go的微服务开发",
			waitTime: 50 * time.Millisecond,
		},
	}

	// 添加记忆并记录时间戳
	for i := range testMemories {
		beforeAdd := time.Now()
		err = memory.HandleMemory(testMemories[i].content)
		require.NoError(t, err, "添加第%d条记忆失败", i+1)

		testMemories[i].timestamp = beforeAdd
		t.Logf("添加记忆 #%d: %s (时间: %s)",
			i+1,
			testMemories[i].content,
			beforeAdd.Format("15:04:05.000"))

		// 等待确保时间戳不同
		time.Sleep(testMemories[i].waitTime)
	}

	// 搜索所有Go相关的记忆
	searchQuery := "Go语言"
	bytesLimit := 5000 // 足够大的限制，确保能返回所有记忆

	result, err := memory.SearchMemory(searchQuery, bytesLimit)
	require.NoError(t, err, "搜索记忆失败")
	require.NotNil(t, result, "搜索结果不应为nil")

	t.Logf("搜索返回 %d 条记忆", len(result.Memories))

	// 验证：至少应该找到一些记忆
	assert.True(t, len(result.Memories) > 0, "应该至少找到一条记忆")

	// 核心验证：检查记忆是否按时间从旧到新排序
	for i := 1; i < len(result.Memories); i++ {
		prevTime := result.Memories[i-1].CreatedAt
		currTime := result.Memories[i].CreatedAt

		assert.True(t,
			prevTime.Before(currTime) || prevTime.Equal(currTime),
			"记忆 #%d (时间: %s) 应该早于或等于记忆 #%d (时间: %s)",
			i, prevTime.Format("15:04:05.000"),
			i+1, currTime.Format("15:04:05.000"))

		t.Logf("  记忆 #%d: %s (创建时间: %s)",
			i,
			result.Memories[i-1].Content[:30]+"...",
			prevTime.Format("15:04:05.000"))
	}

	if len(result.Memories) > 0 {
		lastIdx := len(result.Memories) - 1
		t.Logf("  记忆 #%d: %s (创建时间: %s)",
			lastIdx+1,
			result.Memories[lastIdx].Content[:30]+"...",
			result.Memories[lastIdx].CreatedAt.Format("15:04:05.000"))
	}

	// 额外验证：检查每条记忆都有时间戳
	for i, mem := range result.Memories {
		assert.False(t, mem.CreatedAt.IsZero(),
			"记忆 #%d 的时间戳不应为零值", i+1)
	}

	log.Infof("✓ 时间排序验证通过：%d 条记忆按照时间线正确排序", len(result.Memories))
}

// TestSearchMemoryWithoutAI_TimelineOrdering 测试无AI搜索的时间排序
func TestSearchMemoryWithoutAI_TimelineOrdering(t *testing.T) {
	sessionID := "timeline-order-no-ai-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	// 创建AI记忆系统
	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	require.NoError(t, err)
	defer memory.Close()

	// 添加测试记忆
	testInputs := []string{
		"2020年的技术栈：JavaScript + React",
		"2021年的技术栈：TypeScript + Vue",
		"2022年的技术栈：Go + Docker",
		"2023年的技术栈：Rust + Kubernetes",
		"2024年的技术栈：Python + AI",
	}

	var timestamps []time.Time
	for i, input := range testInputs {
		beforeAdd := time.Now()
		err = memory.HandleMemory(input)
		require.NoError(t, err, "添加记忆失败: %s", input)

		timestamps = append(timestamps, beforeAdd)
		t.Logf("添加记忆 #%d: %s", i+1, input)
		time.Sleep(30 * time.Millisecond)
	}

	// 使用 SearchMemoryWithoutAI 搜索
	searchQuery := "技术栈"
	bytesLimit := 3000

	result, err := memory.SearchMemoryWithoutAI(searchQuery, bytesLimit)
	require.NoError(t, err, "无AI搜索失败")
	require.NotNil(t, result, "搜索结果不应为nil")

	t.Logf("无AI搜索返回 %d 条记忆", len(result.Memories))

	// 验证时间排序
	if len(result.Memories) > 1 {
		for i := 1; i < len(result.Memories); i++ {
			prevTime := result.Memories[i-1].CreatedAt
			currTime := result.Memories[i].CreatedAt

			assert.True(t,
				prevTime.Before(currTime) || prevTime.Equal(currTime),
				"无AI搜索：记忆应该按时间排序，但第 %d 条 (%s) 晚于第 %d 条 (%s)",
				i, prevTime.Format("15:04:05.000"),
				i+1, currTime.Format("15:04:05.000"))
		}
	}

	log.Infof("✓ 无AI搜索时间排序验证通过")
}

// TestSearchMemory_TimelineWithMixedContent 测试混合内容的时间排序
// 添加不同主题的记忆，确保搜索结果中时间排序仍然正确
func TestSearchMemory_TimelineWithMixedContent(t *testing.T) {
	sessionID := "timeline-mixed-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	require.NoError(t, err)
	defer memory.Close()

	// 混合添加不同主题的记忆
	mixedMemories := []string{
		"Go语言的协程很强大",
		"Python适合数据分析",
		"Go的channel是并发的核心",
		"JavaScript的异步编程",
		"Go的垃圾回收机制优化",
		"Rust的所有权系统",
		"Go的性能调优技巧",
	}

	for i, content := range mixedMemories {
		err = memory.HandleMemory(content)
		require.NoError(t, err)
		t.Logf("添加混合记忆 #%d: %s", i+1, content)
		time.Sleep(40 * time.Millisecond)
	}

	// 只搜索Go相关的记忆
	result, err := memory.SearchMemory("Go编程", 4000)
	require.NoError(t, err)

	t.Logf("从混合内容中搜索Go相关记忆，返回 %d 条", len(result.Memories))

	// 验证返回的记忆按时间排序
	for i := 1; i < len(result.Memories); i++ {
		prev := result.Memories[i-1]
		curr := result.Memories[i]

		assert.True(t,
			prev.CreatedAt.Before(curr.CreatedAt) || prev.CreatedAt.Equal(curr.CreatedAt),
			"混合内容搜索：时间排序错误")

		t.Logf("  #%d (时间: %s): %s",
			i,
			prev.CreatedAt.Format("15:04:05.000"),
			prev.Content[:min(len(prev.Content), 40)]+"...")
	}

	if len(result.Memories) > 0 {
		last := result.Memories[len(result.Memories)-1]
		t.Logf("  #%d (时间: %s): %s",
			len(result.Memories),
			last.CreatedAt.Format("15:04:05.000"),
			last.Content[:min(len(last.Content), 40)]+"...")
	}

	log.Infof("✓ 混合内容时间排序验证通过")
}

// TestSearchMemory_TimelineEmptyResult 测试空结果的时间排序
func TestSearchMemory_TimelineEmptyResult(t *testing.T) {
	sessionID := "timeline-empty-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	require.NoError(t, err)
	defer memory.Close()

	// 添加一些记忆
	err = memory.HandleMemory("关于数据库设计的内容")
	require.NoError(t, err)

	// 搜索完全不相关的内容
	result, err := memory.SearchMemory("量子计算机的工作原理", 1000)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 空结果也应该正常处理
	t.Logf("不相关搜索返回 %d 条记忆", len(result.Memories))

	// 即使是空结果，也不应该出错
	assert.NotNil(t, result.Memories)

	log.Infof("✓ 空结果处理正常")
}

// TestSearchMemory_TimestampPresence 测试所有记忆都包含时间戳
func TestSearchMemory_TimestampPresence(t *testing.T) {
	sessionID := "timestamp-presence-test-" + uuid.New().String()
	defer cleanupEntryTestData(t, sessionID)

	memory, err := CreateTestAIMemory(sessionID,
		WithInvoker(mock.NewMockInvoker(context.Background())),
	)
	require.NoError(t, err)
	defer memory.Close()

	// 添加多条记忆
	testData := []string{
		"记忆A：关于系统架构",
		"记忆B：关于性能优化",
		"记忆C：关于代码规范",
		"记忆D：关于测试策略",
	}

	for _, data := range testData {
		err = memory.HandleMemory(data)
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
	}

	// 搜索记忆
	result, err := memory.SearchMemory("记忆", 5000)
	require.NoError(t, err)

	// 验证每条记忆都有有效的时间戳
	for i, mem := range result.Memories {
		assert.False(t, mem.CreatedAt.IsZero(),
			"记忆 #%d 缺少时间戳", i+1)

		// 时间戳应该在合理范围内（最近1分钟内创建）
		timeSinceCreation := time.Since(mem.CreatedAt)
		assert.True(t, timeSinceCreation < 1*time.Minute,
			"记忆 #%d 的时间戳不在合理范围内: %v", i+1, timeSinceCreation)

		assert.True(t, timeSinceCreation >= 0,
			"记忆 #%d 的时间戳不能是未来时间", i+1)

		t.Logf("记忆 #%d: 创建于 %s (距今 %v)",
			i+1,
			mem.CreatedAt.Format("2006-01-02 15:04:05.000"),
			timeSinceCreation.Round(time.Millisecond))
	}

	log.Infof("✓ 所有 %d 条记忆都包含有效时间戳", len(result.Memories))
}

// helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
