package rag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockEmbedder 是一个模拟的嵌入客户端，用于测试
type MockEmbedder struct{}

// Embedding 模拟实现 EmbeddingClient 接口
func (m *MockEmbedder) Embedding(text string) ([]float32, error) {
	// 简单地生成一个固定的向量作为嵌入
	// 在实际测试中，我们可以根据文本内容生成不同的向量
	if text == "Yaklang介绍" || text == "什么是Yaklang" || text == "Yaklang是一种安全研究编程语言" {
		return []float32{1.0, 0.0, 0.0}, nil
	} else if text == "RAG介绍" || text == "什么是RAG" || text == "RAG是一种结合检索和生成的AI技术" {
		return []float32{0.0, 1.0, 0.0}, nil
	} else if text == "AI技术" {
		return []float32{0.5, 0.5, 0.0}, nil
	}
	return []float32{0.0, 0.0, 0.0}, nil
}

// 测试文本分块功能
func TestChunkText(t *testing.T) {
	text := "这是一个测试文本 用于测试文本分块功能 我们需要确保它可以正确地分割成多个块 每个块的大小应该在指定范围内"

	// 测试没有重叠的情况
	chunks := ChunkText(text, 2, 0)
	assert.Equal(t, 2, len(chunks))

	// 测试有重叠的情况
	chunks = ChunkText(text, 2, 1)
	assert.Equal(t, 3, len(chunks))

	// 测试单块情况
	chunks = ChunkText(text, 100, 0)
	assert.Equal(t, 1, len(chunks))
}

// 测试内存向量存储
func TestMemoryVectorStore(t *testing.T) {
	// 创建模拟嵌入器
	mockEmbed := &MockEmbedder{}

	// 创建内存向量存储
	store := NewMemoryVectorStore(mockEmbed)

	// 准备测试文档
	docs := []Document{
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
	err := store.Add(docs...)
	assert.NoError(t, err)

	// 测试计数
	count, err := store.Count()
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

	// 测试获取特定文档
	doc, exists, err := store.Get("doc1")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "Yaklang是一种安全研究编程语言", doc.Content)

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
}

// 测试RAG系统
func TestRAGSystem(t *testing.T) {
	// 创建模拟嵌入器
	mockEmbed := &MockEmbedder{}

	// 创建内存向量存储
	store := NewMemoryVectorStore(mockEmbed)

	// 创建RAG系统
	ragSystem := NewRAGSystem(mockEmbed, store)

	// 准备测试文档
	docs := []Document{
		{
			ID:       "doc1",
			Content:  "Yaklang是一种安全研究编程语言",
			Metadata: map[string]any{"source": "Yaklang介绍"},
		},
		{
			ID:       "doc2",
			Content:  "RAG是一种结合检索和生成的AI技术",
			Metadata: map[string]any{"source": "RAG介绍"},
		},
	}

	// 添加文档到RAG系统
	err := ragSystem.addDocuments(docs...)
	assert.NoError(t, err)

	// 测试查询
	results, err := ragSystem.QueryWithPage("什么是RAG", 1, 5)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "doc2", results[0].Document.ID) // 第一个结果应该是RAG文档

	// 测试生成提示
	prompt := FormatRagPrompt("什么是RAG?", results, "")
	assert.Contains(t, prompt, "RAG是一种结合检索和生成的AI技术")
	assert.Contains(t, prompt, "问题: 什么是RAG?")
}

// 测试TextToDocuments
func TestTextToDocuments(t *testing.T) {
	text := "这是一个长文本 需要被分割成多个文档 这样我们可以测试文本到文档的转换功能"
	metadata := map[string]any{"source": "测试文档"}

	docs := TextToDocuments(text, 2, 0, metadata)

	assert.True(t, len(docs) > 1)
	for _, doc := range docs {
		assert.NotEmpty(t, doc.ID)
		assert.NotEmpty(t, doc.Content)
		assert.Equal(t, "测试文档", doc.Metadata["source"])
		assert.Contains(t, doc.Metadata, "chunk_index")
		assert.Contains(t, doc.Metadata, "total_chunks")
		assert.Contains(t, doc.Metadata, "created_at")
	}
}

// 测试FilterResults
func TestFilterResults(t *testing.T) {
	results := []SearchResult{
		{Score: 0.9},
		{Score: 0.7},
		{Score: 0.5},
		{Score: 0.3},
	}

	filtered := FilterResults(results, 0.6)
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, float32(0.9), filtered[0].Score)
	assert.Equal(t, float32(0.7), filtered[1].Score)
}
