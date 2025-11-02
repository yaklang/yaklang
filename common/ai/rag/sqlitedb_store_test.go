package rag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
)

// 测试 SQLiteVectorStore
func TestSQLiteVectorStore(t *testing.T) {
	// 创建模拟嵌入器
	mockEmbed := vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		return []float32{1.0, 0.0, 0.0}, nil
	})

	db := consts.GetGormProfileDatabase()
	// 创建 SQLite 向量存储
	store, err := vectorstore.NewSQLiteVectorStoreHNSW("test_collection", "test", "Qwen3-Embedding-0.6B-Q4_K_M", 1024, mockEmbed, db)
	assert.NoError(t, err)
	defer store.Remove()

	// 准备测试文档
	docs := []*vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Yaklang是一种安全研究编程语言",
			Metadata:  map[string]any{"source": "Yaklang介绍"},
			Embedding: []float32{1.0, 0.0, 0.0},
		},
		{
			ID:        "doc2",
			Content:   "RAG是一种结合检索和生成的AI技术",
			Metadata:  map[string]any{"source": "RAG介绍"},
			Embedding: []float32{0.0, 1.0, 0.0},
		},
	}

	// 添加文档
	err = store.Add(docs...)
	assert.NoError(t, err)

	// 测试计数
	count, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// 测试获取特定文档
	doc, exists, err := store.Get("doc1")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "doc1", doc.ID)

	// 测试搜索
	results, err := store.Search("什么是Yaklang", 1, 5)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "doc1", results[0].Document.ID)     // 第一个结果应该是Yaklang文档
	assert.True(t, results[0].Score > results[1].Score) // Yaklang文档的相似度应该更高

	// 测试删除
	err = store.Delete("doc1")
	assert.NoError(t, err)

	count, err = store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// 测试列出所有文档
	docs, err = store.List()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(docs))
	assert.Equal(t, "doc2", docs[0].ID)
}
