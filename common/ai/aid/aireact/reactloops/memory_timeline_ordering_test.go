package reactloops

import (
	"context"
	"testing"
	"time"

	"path/filepath"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// createTestAIMemory 创建用于测试的AIMemory实例
func createTestAIMemory(t *testing.T, sessionID string) *aimem.AIMemoryTriage {
	// 创建临时数据库
	tmpDir := consts.GetDefaultYakitBaseTempDir()
	dbFile := filepath.Join(tmpDir, uuid.NewString()+".db")

	db, err := gorm.Open("sqlite3", dbFile)
	require.NoError(t, err, "创建测试数据库失败")

	// 自动迁移表结构
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)

	// 设置数据库连接池
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)

	// 创建 mock embedding 客户端
	mockEmbedder := vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		// 简单的 mock：返回固定维度的向量
		return []float32{0.1, 0.2, 0.3}, nil
	})

	// 创建 AI Memory
	memory, err := aimem.NewAIMemory(sessionID,
		aimem.WithInvoker(mock.NewMockInvoker(context.Background())),
		aimem.WithRAGOptions(rag.WithEmbeddingClient(mockEmbedder)),
		aimem.WithDatabase(db),
	)
	require.NoError(t, err, "创建 AI Memory 失败")

	return memory
}

// TestReActLoop_MemorySearchTimelineOrdering 测试 ReActLoop 搜索记忆时的时间排序
// 确保在 reactloop 中搜索记忆时，返回的记忆按时间线排序（从旧到新）
func TestReActLoop_MemorySearchTimelineOrdering(t *testing.T) {
	sessionID := "reactloop-timeline-test-" + uuid.New().String()

	// 创建测试用的 AI Memory
	memory := createTestAIMemory(t, sessionID)
	defer memory.Close()
	defer cleanupMemoryTestData(t, sessionID)

	// 按时间顺序添加记忆
	testMemories := []struct {
		content  string
		waitTime time.Duration
	}{
		{
			content:  "第一个知识点：ReAct 是 Reasoning and Acting 的缩写",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第二个知识点：ReAct 结合了推理和行动",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第三个知识点：ReAct 可以进行多轮对话",
			waitTime: 50 * time.Millisecond,
		},
		{
			content:  "第四个知识点：ReAct 支持工具调用",
			waitTime: 50 * time.Millisecond,
		},
	}

	// 添加记忆并记录时间戳
	var expectedTimestamps []time.Time
	for i, mem := range testMemories {
		beforeAdd := time.Now()
		err := memory.HandleMemory(mem.content)
		require.NoError(t, err, "添加记忆 #%d 失败", i+1)

		expectedTimestamps = append(expectedTimestamps, beforeAdd)
		t.Logf("添加记忆 #%d: %s (时间: %s)",
			i+1,
			mem.content[:30]+"...",
			beforeAdd.Format("15:04:05.000"))

		time.Sleep(mem.waitTime)
	}

	// 创建 MockMemoryTriage 并设置搜索结果
	// 注意：这里我们直接使用真实的 memory，而不是 mock
	// 因为我们要测试真实的搜索和排序逻辑

	// 测试搜索功能
	searchQuery := "ReAct"
	bytesLimit := 5000

	result, err := memory.SearchMemory(searchQuery, bytesLimit)
	require.NoError(t, err, "搜索记忆失败")
	require.NotNil(t, result, "搜索结果不应为 nil")

	t.Logf("搜索返回 %d 条记忆", len(result.Memories))

	// 核心验证1：检查记忆按时间从旧到新排序
	for i := 1; i < len(result.Memories); i++ {
		prevTime := result.Memories[i-1].CreatedAt
		currTime := result.Memories[i].CreatedAt

		assert.True(t,
			prevTime.Before(currTime) || prevTime.Equal(currTime),
			"记忆应该按时间排序：记忆 #%d (时间: %s) 应该早于或等于记忆 #%d (时间: %s)",
			i, prevTime.Format("15:04:05.000"),
			i+1, currTime.Format("15:04:05.000"))

		t.Logf("  记忆 #%d: 时间=%s, 内容=%s",
			i,
			prevTime.Format("15:04:05.000"),
			result.Memories[i-1].Content[:min(len(result.Memories[i-1].Content), 40)]+"...")
	}

	// 核心验证2：检查每条记忆都有时间戳
	for i, mem := range result.Memories {
		assert.False(t, mem.CreatedAt.IsZero(),
			"记忆 #%d 缺少时间戳", i+1)

		t.Logf("  记忆 #%d 时间戳验证通过: %s",
			i+1,
			mem.CreatedAt.Format("2006-01-02 15:04:05.000"))
	}

	log.Infof("✓ ReActLoop 记忆搜索时间排序验证通过：%d 条记忆", len(result.Memories))
}

// TestReActLoop_MemorySearchWithoutAI_TimelineOrdering 测试无 AI 搜索的时间排序
func TestReActLoop_MemorySearchWithoutAI_TimelineOrdering(t *testing.T) {
	sessionID := "reactloop-no-ai-timeline-test-" + uuid.New().String()

	memory := createTestAIMemory(t, sessionID)
	defer memory.Close()
	defer cleanupMemoryTestData(t, sessionID)

	// 添加测试记忆
	memories := []string{
		"2020年：开始学习 Go 语言基础",
		"2021年：掌握 Go 并发编程",
		"2022年：深入 Go 性能优化",
		"2023年：Go 微服务实战",
		"2024年：Go 云原生开发",
	}

	for i, content := range memories {
		err := memory.HandleMemory(content)
		require.NoError(t, err)
		t.Logf("添加记忆 #%d: %s", i+1, content)
		time.Sleep(40 * time.Millisecond)
	}

	// 使用 SearchMemoryWithoutAI 进行搜索
	result, err := memory.SearchMemoryWithoutAI("Go", 4000)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("无 AI 搜索返回 %d 条记忆", len(result.Memories))

	// 验证时间排序
	for i := 1; i < len(result.Memories); i++ {
		prevTime := result.Memories[i-1].CreatedAt
		currTime := result.Memories[i].CreatedAt

		assert.True(t,
			prevTime.Before(currTime) || prevTime.Equal(currTime),
			"无 AI 搜索的记忆时间排序错误")

		t.Logf("  #%d: %s (时间: %s)",
			i,
			prevTime.Format("15:04:05.000"),
			result.Memories[i-1].Content[:min(len(result.Memories[i-1].Content), 30)]+"...")
	}

	log.Infof("✓ 无 AI 搜索时间排序验证通过")
}

// TestReActLoop_MemoryTimestampPresence 测试 ReActLoop 中记忆包含时间戳
func TestReActLoop_MemoryTimestampPresence(t *testing.T) {
	sessionID := "reactloop-timestamp-test-" + uuid.New().String()

	memory := createTestAIMemory(t, sessionID)
	defer memory.Close()
	defer cleanupMemoryTestData(t, sessionID)

	// 添加记忆
	testData := []string{
		"工具调用：使用 bash 执行命令",
		"工具调用：使用 grep 搜索文件",
		"工具调用：使用 read_file 读取内容",
		"工具调用：使用 write_file 写入数据",
	}

	for _, data := range testData {
		err := memory.HandleMemory(data)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
	}

	// 搜索记忆
	result, err := memory.SearchMemory("工具调用", 3000)
	require.NoError(t, err)

	// 验证每条记忆都有有效的时间戳
	for i, mem := range result.Memories {
		// 时间戳不应为零值
		assert.False(t, mem.CreatedAt.IsZero(),
			"记忆 #%d 缺少时间戳", i+1)

		// 时间戳应该在合理范围内（最近1分钟内创建）
		timeSinceCreation := time.Since(mem.CreatedAt)
		assert.True(t, timeSinceCreation < 1*time.Minute,
			"记忆 #%d 的时间戳不在合理范围内: %v", i+1, timeSinceCreation)

		// 时间戳不能是未来时间
		assert.True(t, timeSinceCreation >= 0,
			"记忆 #%d 的时间戳是未来时间", i+1)

		t.Logf("记忆 #%d: 创建时间=%s, 距今=%v, 内容=%s",
			i+1,
			mem.CreatedAt.Format("2006-01-02 15:04:05.000"),
			timeSinceCreation.Round(time.Millisecond),
			mem.Content[:min(len(mem.Content), 30)]+"...")
	}

	log.Infof("✓ 所有 %d 条记忆都包含有效时间戳", len(result.Memories))
}

// TestReActLoop_MemoryTimelineChronological 测试时间线的时间顺序性
// 确保记忆按照发生的时间顺序呈现（时间线特性）
func TestReActLoop_MemoryTimelineChronological(t *testing.T) {
	sessionID := "reactloop-chronological-test-" + uuid.New().String()

	memory := createTestAIMemory(t, sessionID)
	defer memory.Close()
	defer cleanupMemoryTestData(t, sessionID)

	// 模拟一个时间线序列：用户学习过程
	learningTimeline := []string{
		"第1天：了解 ReAct 的基本概念",
		"第2天：学习 ReAct 的推理过程",
		"第3天：实践 ReAct 的工具调用",
		"第4天：掌握 ReAct 的多轮对话",
		"第5天：优化 ReAct 的性能",
	}

	// 按时间顺序添加记忆
	var addedTimes []time.Time
	for i, content := range learningTimeline {
		beforeAdd := time.Now()
		err := memory.HandleMemory(content)
		require.NoError(t, err)

		addedTimes = append(addedTimes, beforeAdd)
		t.Logf("时间线 Day %d: %s (添加时间: %s)",
			i+1,
			content,
			beforeAdd.Format("15:04:05.000"))

		time.Sleep(60 * time.Millisecond) // 确保时间戳有明显差异
	}

	// 搜索学习记录
	result, err := memory.SearchMemory("学习 ReAct", 5000)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("搜索到 %d 条学习记录", len(result.Memories))

	// 验证：记忆应该按照时间线顺序（从第1天到第5天）
	if len(result.Memories) > 1 {
		for i := 1; i < len(result.Memories); i++ {
			prev := result.Memories[i-1]
			curr := result.Memories[i]

			// 当前记忆的时间应该晚于或等于前一条记忆
			assert.True(t,
				prev.CreatedAt.Before(curr.CreatedAt) || prev.CreatedAt.Equal(curr.CreatedAt),
				"时间线顺序错误：第 %d 条记忆 (%s) 应该早于第 %d 条 (%s)",
				i, prev.CreatedAt.Format("15:04:05.000"),
				i+1, curr.CreatedAt.Format("15:04:05.000"))

			t.Logf("  时间线验证 %d -> %d: %s -> %s ✓",
				i, i+1,
				prev.CreatedAt.Format("15:04:05.000"),
				curr.CreatedAt.Format("15:04:05.000"))
		}
	}

	// 额外验证：打印完整的时间线
	t.Log("完整时间线：")
	for _, mem := range result.Memories {
		t.Logf("  [%s] %s",
			mem.CreatedAt.Format("15:04:05.000"),
			mem.Content[:min(len(mem.Content), 50)]+"...")
	}

	log.Infof("✓ 时间线顺序性验证通过：记忆按照时间顺序正确排列")
}

// TestReActLoop_MemoryTimelineMixedQueries 测试混合查询的时间排序
func TestReActLoop_MemoryTimelineMixedQueries(t *testing.T) {
	sessionID := "reactloop-mixed-timeline-test-" + uuid.New().String()

	memory := createTestAIMemory(t, sessionID)
	defer memory.Close()
	defer cleanupMemoryTestData(t, sessionID)

	// 添加混合主题的记忆（模拟实际使用场景）
	mixedMemories := []string{
		"工具A：文件操作工具的使用方法",
		"错误B：遇到了权限问题",
		"工具C：网络请求工具的配置",
		"成功D：解决了权限问题",
		"工具E：数据处理工具的参数",
		"优化F：提升了处理速度",
	}

	for i, content := range mixedMemories {
		err := memory.HandleMemory(content)
		require.NoError(t, err)
		t.Logf("添加混合记忆 #%d: %s", i+1, content)
		time.Sleep(35 * time.Millisecond)
	}

	// 只查询"工具"相关的记忆
	result, err := memory.SearchMemory("工具", 3000)
	require.NoError(t, err)

	t.Logf("混合查询返回 %d 条记忆", len(result.Memories))

	// 验证：即使只返回部分记忆，时间排序仍然正确
	for i := 1; i < len(result.Memories); i++ {
		prev := result.Memories[i-1]
		curr := result.Memories[i]

		assert.True(t,
			prev.CreatedAt.Before(curr.CreatedAt) || prev.CreatedAt.Equal(curr.CreatedAt),
			"混合查询时间排序错误")

		t.Logf("  #%d: %s (时间: %s)",
			i,
			prev.Content[:min(len(prev.Content), 30)]+"...",
			prev.CreatedAt.Format("15:04:05.000"))
	}

	log.Infof("✓ 混合查询时间排序验证通过")
}

// helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// cleanupMemoryTestData 清理测试数据
func cleanupMemoryTestData(t *testing.T, sessionID string) {
	// 这里可以添加清理逻辑，比如删除数据库中的测试数据
	t.Logf("清理测试数据: sessionID=%s", sessionID)
}
