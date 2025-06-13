package rag

import (
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// MemoryVectorStore 是一个基于内存的向量存储实现
type MemoryVectorStore struct {
	documents map[string]Document // 文档存储，以 ID 为键
	embedder  EmbeddingClient     // 用于生成查询的嵌入向量
	mu        sync.RWMutex        // 用于并发安全的互斥锁
}

// NewMemoryVectorStore 创建一个新的内存向量存储
func NewMemoryVectorStore(embedder EmbeddingClient) *MemoryVectorStore {
	return &MemoryVectorStore{
		documents: make(map[string]Document),
		embedder:  embedder,
	}
}

// Add 添加文档到向量存储
func (m *MemoryVectorStore) Add(docs ...Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, doc := range docs {
		// 确保文档有 ID
		if doc.ID == "" {
			return utils.Errorf("document must have an ID")
		}

		// 确保文档有嵌入向量
		if len(doc.Embedding) == 0 {
			return utils.Errorf("document %s must have an embedding vector", doc.ID)
		}

		// 存储文档
		m.documents[doc.ID] = doc
	}

	return nil
}

// Search 根据查询文本检索相关文档
func (m *MemoryVectorStore) Search(query string, limit int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查是否有文档
	if len(m.documents) == 0 {
		return []SearchResult{}, nil
	}

	// 生成查询的嵌入向量
	queryEmbedding, err := m.embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("failed to generate embedding for query: %v", err)
	}

	// 计算所有文档与查询的相似度
	var results []SearchResult
	for _, doc := range m.documents {
		// 计算余弦相似度
		similarity, err := utils.CosineSimilarity(queryEmbedding, doc.Embedding)
		if err != nil {
			return nil, utils.Errorf("failed to calculate similarity: %v", err)
		}

		// 添加到结果集
		results = append(results, SearchResult{
			Document: doc,
			Score:    similarity,
			// 欧几里得距离可以作为额外的距离度量（可选）
			Distance: 0, // 这里我们只使用余弦相似度
		})
	}

	// 按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制结果数量
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// Delete 根据 ID 删除文档
func (m *MemoryVectorStore) Delete(ids ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, id := range ids {
		delete(m.documents, id)
	}

	return nil
}

// Get 根据 ID 获取文档
func (m *MemoryVectorStore) Get(id string) (Document, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	doc, exists := m.documents[id]
	return doc, exists, nil
}

// List 列出所有文档
func (m *MemoryVectorStore) List() ([]Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	docs := make([]Document, 0, len(m.documents))
	for _, doc := range m.documents {
		docs = append(docs, doc)
	}

	return docs, nil
}

// Count 返回文档总数
func (m *MemoryVectorStore) Count() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.documents), nil
}
