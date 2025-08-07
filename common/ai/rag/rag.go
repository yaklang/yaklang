package rag

import (
	"github.com/yaklang/yaklang/common/utils"
)

// Document 表示可以被检索的文档
type Document struct {
	ID        string         `json:"id"`       // 文档唯一标识符
	Content   string         `json:"content"`  // 文档内容
	Metadata  map[string]any `json:"metadata"` // 文档元数据
	Embedding []float32      `json:"-"`        // 文档的嵌入向量，不参与 JSON 序列化
}

// SearchResult 表示检索结果
type SearchResult struct {
	Document Document `json:"document"` // 检索到的文档
	Score    float32  `json:"score"`    // 相似度得分 (-1 到 1 之间)
}

// EmbeddingClient 接口定义了嵌入向量生成的操作
type EmbeddingClient interface {
	Embedding(text string) ([]float32, error)
}

// VectorStore 接口定义了向量存储的基本操作
type VectorStore interface {
	// Add 添加文档到向量存储
	Add(docs ...Document) error

	// Search 根据查询文本检索相关文档
	Search(query string, page, limit int) ([]SearchResult, error)

	// Delete 根据 ID 删除文档
	Delete(ids ...string) error

	// Get 根据 ID 获取文档
	Get(id string) (Document, bool, error)

	// List 列出所有文档
	List() ([]Document, error)

	// Count 返回文档总数
	Count() (int, error)
}

// RAGSystem 表示完整的 RAG 系统
type RAGSystem struct {
	Embedder    EmbeddingClient // 嵌入向量生成器
	VectorStore VectorStore     // 向量存储
}

// NewRAGSystem 创建一个新的 RAG 系统
func NewRAGSystem(embedder EmbeddingClient, store VectorStore) *RAGSystem {
	return &RAGSystem{
		Embedder:    embedder,
		VectorStore: store,
	}
}

func (r *RAGSystem) Add(docId string, content string, opts ...DocumentOption) error {
	doc := &Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	for _, opt := range opts {
		opt(doc)
	}
	return r.addDocuments(*doc)
}

// AddDocuments 添加文档到 RAG 系统
func (r *RAGSystem) addDocuments(docs ...Document) error {
	// 为每个文档生成嵌入向量
	for i := range docs {
		embedding, err := r.Embedder.Embedding(docs[i].Content)
		if err != nil {
			return utils.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
		}
		docs[i].Embedding = embedding
	}

	// 添加到向量存储
	return r.VectorStore.Add(docs...)
}

// Query 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) Query(query string, page, limit int) ([]SearchResult, error) {
	return r.VectorStore.Search(query, page, limit)
}

// DeleteDocuments 删除文档
func (r *RAGSystem) DeleteDocuments(ids ...string) error {
	return r.VectorStore.Delete(ids...)
}

// ClearDocuments 清空所有文档
func (r *RAGSystem) ClearDocuments() error {
	docs, err := r.ListDocuments()
	if err != nil {
		return err
	}
	ids := []string{}
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	err = r.VectorStore.Delete(ids...)
	if err != nil {
		return err
	}
	return nil
}

// GetDocument 获取指定 ID 的文档
func (r *RAGSystem) GetDocument(id string) (Document, bool, error) {
	return r.VectorStore.Get(id)
}

// ListDocuments 列出所有文档
func (r *RAGSystem) ListDocuments() ([]Document, error) {
	return r.VectorStore.List()
}

// CountDocuments 获取文档总数
func (r *RAGSystem) CountDocuments() (int, error) {
	return r.VectorStore.Count()
}
