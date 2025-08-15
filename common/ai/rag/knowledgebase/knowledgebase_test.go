package knowledgebase

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// testEmbedder 测试用的嵌入器函数
func testEmbedder(text string) ([]float32, error) {
	// 简单地生成一个固定的向量作为嵌入
	// 根据文本内容生成不同的向量
	if len(text) > 10 {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if len(text) > 5 {
		return []float32{0.0, 1.0, 0.0}, nil
	}
	return []float32{0.0, 0.0, 1.0}, nil
}

// TestNewKnowledgeBase 测试创建知识库（包含KnowledgeBaseInfo）
func TestNewKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库，使用 mock 嵌入器
	kb, err := NewKnowledgeBase(
		db,
		"test-kb-with-info",
		"测试知识库",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 验证 KnowledgeBaseInfo 被创建
	kbInfo, err := LoadKnowledgeBase(db, "test-kb-with-info")
	assert.NoError(t, err)
	info, err := kbInfo.GetInfo()
	assert.NoError(t, err)
	assert.Equal(t, "test-kb-with-info", info.KnowledgeBaseName)
	assert.Equal(t, "测试知识库", info.KnowledgeBaseDescription)
	assert.Equal(t, "test", info.KnowledgeBaseType)

	// 验证 RAG Collection 被创建
	assert.True(t, rag.CollectionIsExists(db, "test-kb-with-info"))

	// 再次调用 NewKnowledgeBase，应该直接加载而不创建新的
	kb2, err := NewKnowledgeBase(
		db,
		"test-kb-with-info",
		"不应该被使用的描述",
		"不应该被使用的类型",
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb2)

	// 验证数据库中的信息没有被更新
	kbInfo2, err := LoadKnowledgeBase(db, "test-kb-with-info")
	assert.NoError(t, err)
	info2, err := kbInfo2.GetInfo()
	assert.NoError(t, err)
	assert.Equal(t, "测试知识库", info2.KnowledgeBaseDescription) // 应该还是原来的描述
}

// TestCreateKnowledgeBase 测试创建全新知识库
func TestCreateKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建全新知识库
	kb, err := CreateKnowledgeBase(
		db,
		"new-kb",
		"全新知识库",
		"fresh",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb)

	// 验证创建成功
	kbInfo, err := LoadKnowledgeBase(db, "new-kb")
	assert.NoError(t, err)
	assert.Equal(t, "new-kb", kbInfo.name)

	// 再次尝试创建同名知识库，应该失败
	kb2, err := CreateKnowledgeBase(
		db,
		"new-kb",
		"重复知识库",
		"duplicate",
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.Error(t, err)
	assert.Nil(t, kb2)
	assert.Contains(t, err.Error(), "已存在")
}

// TestLoadKnowledgeBase 测试加载知识库
func TestLoadKnowledgeBase(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 先创建一个知识库
	kb1, err := NewKnowledgeBase(
		db,
		"load-test-kb",
		"加载测试知识库",
		"load-test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb1)

	// 加载已存在的知识库
	kb2, err := LoadKnowledgeBase(
		db,
		"load-test-kb",
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)
	assert.NotNil(t, kb2)
	assert.Equal(t, "load-test-kb", kb2.GetName())

	// 尝试加载不存在的知识库
	kb3, err := LoadKnowledgeBase(db, "non-existent-kb")
	assert.Error(t, err)
	assert.Nil(t, kb3)
	assert.Contains(t, err.Error(), "不存在")
}

// TestKnowledgeBaseOperations 测试知识库的基本操作
func TestKnowledgeBaseOperations(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库
	kb, err := NewKnowledgeBase(
		db,
		"ops-test-kb",
		"操作测试知识库",
		"ops-test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)

	// 添加知识条目
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    1, // 假设这是第一个知识库
		KnowledgeTitle:     "测试知识条目",
		KnowledgeType:      "Test",
		ImportanceScore:    8,
		Keywords:           []string{"测试", "知识库"},
		KnowledgeDetails:   "这是一个测试知识条目的详细内容",
		Summary:            "测试条目摘要",
		SourcePage:         1,
		PotentialQuestions: []string{"什么是测试?", "如何使用知识库?"},
	}

	err = kb.AddKnowledgeEntry(entry)
	assert.NoError(t, err)

	// 搜索知识条目
	results, err := kb.SearchKnowledgeEntries("测试", 5)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.Equal(t, "测试知识条目", results[0].KnowledgeTitle)

	// 获取知识条目列表
	entries, err := kb.ListKnowledgeEntries("", 1, 10)
	assert.NoError(t, err)
	assert.True(t, len(entries) > 0)

	// 获取同步状态
	status, err := kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.True(t, status.InSync)
	assert.Equal(t, 1, status.DatabaseEntries)
	assert.Equal(t, 1, status.RAGDocuments)
}
