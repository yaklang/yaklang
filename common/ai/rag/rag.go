package rag

import (
	"context"
	"fmt"
	"sort"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// RAGSystem 表示完整的 RAG 系统
type RAGSystem struct {
	Embedder     vectorstore.EmbeddingClient        // 嵌入向量生成器
	VectorStore  *vectorstore.SQLiteVectorStoreHNSW // 向量存储
	BigTextPlan  string                             // 大文本方案
	Concurrent   int                                // 并发数
	MaxChunkSize int                                // 最大块大小
	ChunkOverlap int                                // 块重叠
	Name         string
	UUID         string
}

// NewRAGSystem 创建一个新的 RAG 系统
func NewRAGSystem(embedder vectorstore.EmbeddingClient, store *vectorstore.SQLiteVectorStoreHNSW) *RAGSystem {
	return NewRAGSystemWithName("", embedder, store)
}

func NewRAGSystemWithName(name string, embedder vectorstore.EmbeddingClient, store *vectorstore.SQLiteVectorStoreHNSW) *RAGSystem {
	return &RAGSystem{
		Name:         name,
		Embedder:     embedder,
		VectorStore:  store,
		BigTextPlan:  vectorstore.BigTextPlanChunkText, // 默认使用分块策略
		Concurrent:   10,
		MaxChunkSize: 800,
		ChunkOverlap: 100,
	}
}

// NewRAGSystemWithLocalEmbedding 创建使用本地模型嵌入的 RAG 系统
// 自动启动本地嵌入服务，如果无法启动则报错
func NewRAGSystemWithLocalEmbedding(store *vectorstore.SQLiteVectorStoreHNSW) (*RAGSystem, error) {
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
func NewDefaultRAGSystem(store *vectorstore.SQLiteVectorStoreHNSW) (*RAGSystem, error) {
	return NewRAGSystemWithLocalEmbedding(store)
}

// NewRAGSystemWithOptionalEmbedding 创建 RAG 系统，支持可选的嵌入服务
// 如果 embedder 为 nil，则使用默认的本地嵌入服务
func NewRAGSystemWithOptionalEmbedding(store *vectorstore.SQLiteVectorStoreHNSW, embedder vectorstore.EmbeddingClient) (*RAGSystem, error) {
	if embedder == nil {
		log.Infof("no embedder provided, using default local embedding service")
		return NewRAGSystemWithLocalEmbedding(store)
	}

	log.Infof("using provided embedder for RAG system")
	return NewRAGSystem(embedder, store), nil
}

// SetBigTextPlan 设置大文本处理方案
func (r *RAGSystem) SetBigTextPlan(plan string) {
	r.BigTextPlan = plan
	log.Infof("set big text plan to: %s", plan)
}

// VectorSimilarity 快速计算两个文本的向量相似度
func (r *RAGSystem) VectorSimilarity(text1, text2 string) (float64, error) {
	embeddingData1, err := r.Embedder.Embedding(text1)
	if err != nil {
		return 0, err
	}

	embeddingData2, err := r.Embedder.Embedding(text2)
	if err != nil {
		return 0, err
	}

	return hnsw.CosineSimilarity(embeddingData1, embeddingData2)
}

// averagePooling 对多个嵌入向量进行平均池化
func averagePooling(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// 获取向量维度
	dim := len(embeddings[0])
	if dim == 0 {
		return nil
	}

	// 初始化结果向量
	result := make([]float32, dim)
	validCount := 0

	// 累加所有向量
	for _, embedding := range embeddings {
		if len(embedding) != dim {
			log.Warnf("embedding dimension mismatch: expected %d, got %d", dim, len(embedding))
			continue
		}
		validCount++
		for i, val := range embedding {
			result[i] += val
		}
	}

	// 如果没有有效向量，返回nil
	if validCount == 0 {
		return nil
	}

	// 计算平均值
	count := float32(validCount)
	for i := range result {
		result[i] /= count
	}

	return result
}

func (r *RAGSystem) Has(docId string) bool {
	return r.VectorStore.Has(docId)
}

func (r *RAGSystem) Add(docId string, content string, opts ...vectorstore.DocumentOption) error {
	//log.Infof("adding document with id: %s, content length: %d", docId, len(content))
	doc := &vectorstore.Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	//log.Infof("applying %d document options", len(opts))
	for i, opt := range opts {
		_ = i
		//log.Infof("applying document option %d", i+1)
		opt(doc)
	}
	//log.Infof("document metadata after options: %+v", doc.Metadata)
	return r.addDocuments(doc)
}

func BuildDocument(docId, content string, opts ...vectorstore.DocumentOption) *vectorstore.Document {
	doc := &vectorstore.Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	for _, opt := range opts {
		opt(doc)
	}
	return doc
}

func (r *RAGSystem) AddDocuments(docs ...*vectorstore.Document) error {
	return r.VectorStore.Add(docs...)
}

// AddDocuments 添加文档到 RAG 系统
func (r *RAGSystem) addDocuments(docs ...*vectorstore.Document) error {
	return r.VectorStore.Add(docs...)
}

func (r *RAGSystem) SetArchived(archived bool) error {
	return r.VectorStore.SetArchived(archived)
}

func (r *RAGSystem) GetArchived() bool {
	return r.VectorStore.GetArchived()
}

func (r *RAGSystem) ConvertToStandardMode() error {
	return r.VectorStore.ConvertToStandardMode()
}

// QueryWithPage 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) QueryWithPage(query string, page, limit int) ([]vectorstore.SearchResult, error) {
	return r.VectorStore.QueryWithPage(query, page, limit)
}

func (r *RAGSystem) QueryWithFilter(query string, page, limit int, filter func(key string, getDoc func() *vectorstore.Document) bool) ([]vectorstore.SearchResult, error) {
	return r.VectorStore.QueryWithFilter(query, page, limit, filter)
}

// FuzzRawSearch Sql 文本模糊搜索（非语义）
func (r *RAGSystem) FuzzRawSearch(ctx context.Context, keywords string, limit int) (<-chan vectorstore.SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", keywords, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	return r.VectorStore.FuzzSearch(ctx, keywords, limit)
}

// Query is short for QueryTopN
func (r *RAGSystem) Query(query string, topN int, limits ...float64) ([]vectorstore.SearchResult, error) {
	return r.QueryTopN(query, topN, limits...)
}

// QueryTopN 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) QueryTopN(query string, topN int, limits ...float64) ([]vectorstore.SearchResult, error) {
	return r.VectorStore.QueryTopN(query, topN, limits...)
}

// DeleteDocuments 删除文档
func (r *RAGSystem) DeleteDocuments(ids ...string) error {
	return r.VectorStore.Delete(ids...)
}

// ClearDocuments 清空所有文档
func (r *RAGSystem) ClearDocuments() error {
	return r.VectorStore.Clear()
}

// GetDocument 获取指定 ID 的文档
func (r *RAGSystem) GetDocument(id string) (*vectorstore.Document, bool, error) {
	return r.VectorStore.Get(id)
}

// ListDocuments 列出所有文档
func (r *RAGSystem) ListDocuments() ([]*vectorstore.Document, error) {
	return r.VectorStore.List()
}

// CountDocuments 获取文档总数
func (r *RAGSystem) CountDocuments() (int, error) {
	return r.VectorStore.Count()
}

func (r *RAGSystem) DeleteEmbeddingData() error {
	return r.VectorStore.DeleteEmbeddingData()
}

func QueryCollection(db *gorm.DB, query string, opts ...aispec.AIConfigOption) ([]*vectorstore.SearchResult, error) {
	log.Infof("searching for collections matching query: %s", query)

	// 1. 首先查找所有集合信息文档
	var collectionDocs []*schema.VectorStoreDocument
	err := db.Model(&schema.VectorStoreDocument{}).Where("document_id = ?", vectorstore.DocumentTypeCollectionInfo).Find(&collectionDocs).Error
	if err != nil {
		return nil, utils.Errorf("failed to query collection documents: %v", err)
	}

	if len(collectionDocs) == 0 {
		log.Warnf("no collections found in database")
		return []*vectorstore.SearchResult{}, nil
	}

	log.Infof("found %d collection info documents", len(collectionDocs))

	// 2. 获取嵌入服务
	embedder, err := GetDefaultEmbedder()
	if err != nil {
		return nil, utils.Errorf("failed to get default embedder: %v", err)
	}

	// 3. 为查询生成嵌入向量
	queryEmbedding, err := embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("failed to generate embedding for query: %v", err)
	}

	// 4. 计算每个集合文档与查询的相似度
	var results []*vectorstore.SearchResult
	for _, doc := range collectionDocs {
		if len(doc.Embedding) == 0 {
			log.Warnf("collection document %s has no embedding, skipping", doc.DocumentID)
			continue
		}

		// 计算余弦相似度
		similarity, err := hnsw.CosineSimilarity(queryEmbedding, []float32(doc.Embedding))
		if err != nil {
			log.Warnf("failed to calculate similarity for collection document %s: %v", doc.DocumentID, err)
			continue
		}

		// 转换为Document结构
		document := &vectorstore.Document{
			ID:        doc.DocumentID,
			Content:   "", // 从metadata中获取集合信息
			Metadata:  map[string]any(doc.Metadata),
			Embedding: []float32(doc.Embedding),
		}

		// 构建集合内容描述
		if collectionName, ok := doc.Metadata["collection_name"].(string); ok {
			collectionID := doc.Metadata["collection_id"]
			document.Content = fmt.Sprintf("collection_name: %s\ncollection_id: %v", collectionName, collectionID)

			// 查找对应的集合详细信息
			var collection *schema.VectorStoreCollection
			if collectionIDInt, ok := collectionID.(float64); ok {
				collection, err = yakit.QueryRAGCollectionByID(db, int64(collectionIDInt))
				if err != nil {
					log.Warnf("failed to query collection by id: %v", err)
					continue
				}
			}
			if collection != nil {
				document.Content = fmt.Sprintf("collection_name: %s\ncollection_description: %s\nmodel_name: %s\ndimension: %d",
					collection.Name, collection.Description, collection.ModelName, collection.Dimension)
			}
		}

		results = append(results, &vectorstore.SearchResult{
			Document: document,
			Score:    similarity,
		})
	}

	// 5. 按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	log.Infof("found %d matching collection results", len(results))
	return results, nil
}

// GetDefaultEmbedder 获取默认的嵌入服务客户端
// 返回本地模型嵌入服务的单例实例
func GetDefaultEmbedder() (vectorstore.EmbeddingClient, error) {
	return GetLocalEmbeddingService()
}

// IsDefaultEmbedderReady 检查默认嵌入服务是否已准备就绪
func IsDefaultEmbedderReady() bool {
	return IsServiceRunning()
}
