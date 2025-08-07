package rag

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
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

// NewRAGSystemWithLocalEmbedding 创建使用本地模型嵌入的 RAG 系统
// 自动启动本地嵌入服务，如果无法启动则报错
func NewRAGSystemWithLocalEmbedding(store VectorStore) (*RAGSystem, error) {
	log.Infof("creating RAG system with local embedding service")

	// 获取本地嵌入服务单例
	embeddingService, err := GetLocalEmbeddingService()
	if err != nil {
		log.Errorf("failed to get local embedding service: %v", err)
		return nil, utils.Errorf("failed to initialize local embedding service: %v", err)
	}

	log.Infof("successfully initialized RAG system with local embedding at %s", embeddingService.GetAddress())

	return &RAGSystem{
		Embedder:    embeddingService,
		VectorStore: store,
	}, nil
}

// NewDefaultRAGSystem 创建默认的 RAG 系统（使用本地嵌入服务）
// 这是推荐的创建方式，会自动使用本地模型嵌入服务
func NewDefaultRAGSystem(store VectorStore) (*RAGSystem, error) {
	return NewRAGSystemWithLocalEmbedding(store)
}

// NewRAGSystemWithOptionalEmbedding 创建 RAG 系统，支持可选的嵌入服务
// 如果 embedder 为 nil，则使用默认的本地嵌入服务
func NewRAGSystemWithOptionalEmbedding(store VectorStore, embedder EmbeddingClient) (*RAGSystem, error) {
	if embedder == nil {
		log.Infof("no embedder provided, using default local embedding service")
		return NewRAGSystemWithLocalEmbedding(store)
	}

	log.Infof("using provided embedder for RAG system")
	return NewRAGSystem(embedder, store), nil
}

func (r *RAGSystem) Add(docId string, content string, opts ...DocumentOption) error {
	log.Infof("adding document with id: %s, content length: %d", docId, len(content))
	doc := &Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	log.Infof("applying %d document options", len(opts))
	for i, opt := range opts {
		log.Infof("applying document option %d", i+1)
		opt(doc)
	}
	log.Infof("document metadata after options: %+v", doc.Metadata)
	return r.addDocuments(*doc)
}

// AddDocuments 添加文档到 RAG 系统
func (r *RAGSystem) addDocuments(docs ...Document) error {
	log.Infof("adding %d documents to RAG system", len(docs))
	// 为每个文档生成嵌入向量
	for i := range docs {
		log.Infof("generating embedding for document %s (index %d)", docs[i].ID, i)
		embedding, err := r.Embedder.Embedding(docs[i].Content)
		if err != nil {
			log.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
			return utils.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
		}
		if len(embedding) <= 0 {
			log.Errorf("empty embedding generated for document %s", docs[i].ID)
			return utils.Errorf("failed to generate embedding for document (empty embedding) %s", docs[i].ID)
		}
		log.Infof("successfully generated embedding for document %s, dimension: %d", docs[i].ID, len(embedding))
		docs[i].Embedding = embedding
	}

	log.Infof("adding %d documents with embeddings to vector store", len(docs))
	// 添加到向量存储
	err := r.VectorStore.Add(docs...)
	if err != nil {
		log.Errorf("failed to add documents to vector store: %v", err)
		return err
	}
	log.Infof("successfully added %d documents to vector store", len(docs))
	return nil
}

// Query 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) Query(query string, page, limit int) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
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

// GetDefaultEmbedder 获取默认的嵌入服务客户端
// 返回本地模型嵌入服务的单例实例
func GetDefaultEmbedder() (EmbeddingClient, error) {
	return GetLocalEmbeddingService()
}

// IsDefaultEmbedderReady 检查默认嵌入服务是否已准备就绪
func IsDefaultEmbedderReady() bool {
	return IsServiceRunning()
}
