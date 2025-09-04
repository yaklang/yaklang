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

// TestSyncFunctionality 测试同步功能
func TestSyncFunctionality(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库，使用 mock 嵌入器
	kb, err := NewKnowledgeBase(
		db,
		"sync-test-kb",
		"同步测试知识库",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)

	// 检查初始状态
	status, err := kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.True(t, status.InSync)
	assert.Equal(t, 0, status.DatabaseEntries)
	assert.Equal(t, 0, status.RAGDocuments)

	// 添加知识条目到数据库（不通过知识库接口）
	entry1 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  1,
		KnowledgeTitle:   "测试条目1",
		KnowledgeType:    "Standard",
		ImportanceScore:  8,
		Keywords:         []string{"测试", "同步"},
		KnowledgeDetails: "这是测试条目1的详细内容",
		Summary:          "测试条目1摘要",
	}

	entry2 := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  1,
		KnowledgeTitle:   "测试条目2",
		KnowledgeType:    "Standard",
		ImportanceScore:  7,
		Keywords:         []string{"测试", "功能"},
		KnowledgeDetails: "这是测试条目2的详细内容",
		Summary:          "测试条目2摘要",
	}

	// 直接向数据库添加条目（绕过RAG）
	err = db.Create(entry1).Error
	assert.NoError(t, err)
	err = db.Create(entry2).Error
	assert.NoError(t, err)

	// 检查不同步状态
	status, err = kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.False(t, status.InSync)
	assert.Equal(t, 2, status.DatabaseEntries)
	assert.Equal(t, 0, status.RAGDocuments)

	// 执行同步
	syncResult, err := kb.SyncKnowledgeBaseWithRAG()
	assert.NoError(t, err)
	assert.Equal(t, 2, syncResult.TotalDBEntries)
	assert.Equal(t, 0, syncResult.TotalRAGDocuments)
	assert.Equal(t, 2, len(syncResult.AddedToRAG))
	assert.Equal(t, 0, len(syncResult.DeletedFromRAG))
	assert.Equal(t, 0, len(syncResult.SyncErrors))

	// 检查同步后状态
	status, err = kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.True(t, status.InSync)
	assert.Equal(t, 2, status.DatabaseEntries)
	assert.Equal(t, 2, status.RAGDocuments)

	// 模拟RAG中有但数据库中没有的情况
	// 添加一个额外的文档到RAG
	err = kb.ragSystem.Add("extra-doc", "额外的文档内容")
	assert.NoError(t, err)

	// 检查不同步状态
	status, err = kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.False(t, status.InSync)
	assert.Equal(t, 2, status.DatabaseEntries)
	assert.Equal(t, 3, status.RAGDocuments)

	// 再次同步（应该删除额外的文档）
	syncResult, err = kb.SyncKnowledgeBaseWithRAG()
	assert.NoError(t, err)
	assert.Equal(t, 2, syncResult.TotalDBEntries)
	assert.Equal(t, 3, syncResult.TotalRAGDocuments)
	assert.Equal(t, 0, len(syncResult.AddedToRAG))
	assert.Equal(t, 1, len(syncResult.DeletedFromRAG))
	assert.Equal(t, "extra-doc", syncResult.DeletedFromRAG[0])

	// 检查最终同步状态
	status, err = kb.GetSyncStatus()
	assert.NoError(t, err)
	assert.True(t, status.InSync)
	assert.Equal(t, 2, status.DatabaseEntries)
	assert.Equal(t, 2, status.RAGDocuments)
}

// TestBatchSyncEntries 测试批量同步指定条目
func TestBatchSyncEntries(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库，使用 mock 嵌入器
	kb, err := NewKnowledgeBase(
		db,
		"batch-sync-test-kb",
		"批量同步测试知识库",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)

	// 添加测试条目
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:  1,
			KnowledgeTitle:   "批量测试条目1",
			KnowledgeDetails: "批量测试条目1内容",
		},
		{
			KnowledgeBaseID:  1,
			KnowledgeTitle:   "批量测试条目2",
			KnowledgeDetails: "批量测试条目2内容",
		},
	}

	// 直接添加到数据库
	for _, entry := range entries {
		err = db.Create(entry).Error
		assert.NoError(t, err)
	}

	// 批量同步指定条目
	entryIDs := []string{entries[0].HiddenIndex, entries[1].HiddenIndex}
	syncResult, err := kb.BatchSyncEntries(entryIDs)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(syncResult.AddedToRAG))
	assert.Equal(t, 0, len(syncResult.SyncErrors))

	// 验证同步结果
	count, err := kb.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
}

// TestTransactionOperations 测试事务操作
func TestTransactionOperations(t *testing.T) {
	// 创建临时数据库
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	db, err := rag.NewRagDatabase(path)
	assert.NoError(t, err)
	defer db.Close()

	// 创建知识库，使用 mock 嵌入器
	kb, err := NewKnowledgeBase(
		db,
		"transaction-test-kb",
		"事务测试知识库",
		"test",
		rag.WithEmbeddingModel("mock-model"),
		rag.WithModelDimension(3),
		rag.WithEmbeddingClient(rag.NewMockEmbedder(testEmbedder)),
	)
	assert.NoError(t, err)

	// 测试添加操作（事务）
	entry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:  1,
		KnowledgeTitle:   "事务测试条目",
		KnowledgeType:    "Standard",
		ImportanceScore:  8,
		Keywords:         []string{"事务", "测试"},
		KnowledgeDetails: "这是事务测试条目",
		Summary:          "事务测试摘要",
	}

	err = kb.AddKnowledgeEntry(entry)
	assert.NoError(t, err)

	// 验证添加成功
	count, err := kb.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// 测试更新操作（事务）
	entry.KnowledgeDetails = "更新后的详细内容"
	err = kb.UpdateKnowledgeEntry(entry.HiddenIndex, entry)
	assert.NoError(t, err)

	// 验证更新成功
	count, err = kb.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 1, count) // 数量不变，但内容已更新

	// 测试删除操作（事务）
	err = kb.DeleteKnowledgeEntry(entry.HiddenIndex)
	assert.NoError(t, err)

	// 验证删除成功
	count, err = kb.CountDocuments()
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}
