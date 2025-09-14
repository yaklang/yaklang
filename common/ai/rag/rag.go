package rag

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// BigTextPlan 常量定义
const (
	// BigTextPlanChunkText 将大文本分割成多个文档分别存储
	BigTextPlanChunkText = "chunkText"

	// BigTextPlanChunkTextAndAvgPooling 将大文本分割后生成多个嵌入向量，然后平均池化成一个文档存储
	BigTextPlanChunkTextAndAvgPooling = "chunkTextAndAvgPooling"

	// DocumentTypeCollectionInfo 表示集合信息
	DocumentTypeCollectionInfo = "__collection_info__"
)

// Document 表示可以被检索的文档
type Document struct {
	ID              string                 `json:"id"`   // 文档唯一标识符
	Type            schema.RAGDocumentType `json:"type"` // 文档类型
	EntityUUID      string                 `json:"entityUUID"`
	RelatedEntities []string               `json:"relatedEntities"`
	Content         string                 `json:"content"`  // 文档内容
	Metadata        schema.MetadataMap     `json:"metadata"` // 文档元数据
	Embedding       []float32              `json:"-"`        // 文档的嵌入向量，不参与 JSON 序列化
	RuntimeID       string                 `json:"runtimeID"`
}

// SearchResult 表示检索结果
type SearchResult struct {
	Document Document `json:"document"` // 检索到的文档
	Score    float64  `json:"score"`    // 相似度得分 (-1 到 1 之间)
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

	SearchWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error)

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
	Embedder     EmbeddingClient // 嵌入向量生成器
	VectorStore  VectorStore     // 向量存储
	BigTextPlan  string          // 大文本方案
	Concurrent   int             // 并发数
	MaxChunkSize int             // 最大块大小
	ChunkOverlap int             // 块重叠
	Name         string
}

// NewRAGSystem 创建一个新的 RAG 系统
func NewRAGSystem(embedder EmbeddingClient, store VectorStore) *RAGSystem {
	return NewRAGSystemWithName("", embedder, store)
}

func NewRAGSystemWithName(name string, embedder EmbeddingClient, store VectorStore) *RAGSystem {
	return &RAGSystem{
		Name:         name,
		Embedder:     embedder,
		VectorStore:  store,
		BigTextPlan:  BigTextPlanChunkText, // 默认使用分块策略
		Concurrent:   10,
		MaxChunkSize: 800,
		ChunkOverlap: 100,
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

func (r *RAGSystem) Add(docId string, content string, opts ...DocumentOption) error {
	//log.Infof("adding document with id: %s, content length: %d", docId, len(content))
	doc := &Document{
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
	return r.addDocuments(*doc)
}

// AddDocuments 添加文档到 RAG 系统
func (r *RAGSystem) addDocuments(docs ...Document) error {
	//log.Infof("adding %d documents to RAG system", len(docs))

	var finalDocs []Document

	// 为每个文档生成嵌入向量
	for i := range docs {
		//log.Infof("generating embedding for document %s (index %d)", docs[i].ID, i)
		// 首先尝试直接生成嵌入
		embeddingData, err := r.Embedder.Embedding(docs[i].Content)
		if err != nil {
			if errors.Is(err, embedding.ErrInputTooLarge) {
				// 如果失败且是由于文本过大，使用BigTextPlan处理
				processedDocs, processErr := r.processBigText(docs[i])
				if processErr != nil {
					log.Errorf("failed to process big text for document %s: %v", docs[i].ID, processErr)
					return utils.Errorf("failed to process document %s: %v", docs[i].ID, processErr)
				}

				// 将处理后的文档添加到最终文档列表
				finalDocs = append(finalDocs, processedDocs...)
				continue
			}
			log.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
			return utils.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
		}

		if len(embeddingData) <= 0 {
			log.Errorf("empty embedding generated for document %s", docs[i].ID)
			return utils.Errorf("failed to generate embedding for document (empty embedding) %s", docs[i].ID)
		}

		//log.Infof("successfully generated embedding for document %s, dimension: %d", docs[i].ID, len(embeddingData))
		docs[i].Embedding = embeddingData
		finalDocs = append(finalDocs, docs[i])
	}

	//log.Infof("adding %d processed documents with embeddings to vector store", len(finalDocs))
	// 添加到向量存储
	err := r.VectorStore.Add(finalDocs...)
	if err != nil {
		log.Errorf("failed to add documents to vector store: %v", err)
		return err
	}
	//log.Infof("successfully added %d documents to vector store", len(finalDocs))
	return nil
}

// processBigText 处理大文本，根据BigTextPlan策略进行不同的处理
func (r *RAGSystem) processBigText(doc Document) ([]Document, error) {
	log.Infof("processing big text for document %s using plan: %s", doc.ID, r.BigTextPlan)

	// 设置合理的分块参数
	maxChunkSize := r.MaxChunkSize // 默认块大小（rune计算）
	overlap := r.ChunkOverlap      // 默认重叠

	// 根据元数据调整分块参数
	if chunkSize, ok := doc.Metadata["chunk_size"].(int); ok && chunkSize > 0 {
		maxChunkSize = chunkSize
	}
	if chunkOverlap, ok := doc.Metadata["chunk_overlap"].(int); ok && chunkOverlap >= 0 {
		overlap = chunkOverlap
	}

	// 分割文本
	chunks := ChunkText(doc.Content, maxChunkSize, overlap)
	if len(chunks) == 0 {
		return nil, utils.Errorf("failed to chunk text for document %s", doc.ID)
	}

	log.Infof("split document %s into %d chunks", doc.ID, len(chunks))

	switch r.BigTextPlan {
	case BigTextPlanChunkText:
		return r.processChunkText(doc, chunks)
	case BigTextPlanChunkTextAndAvgPooling:
		return r.processChunkTextAndAvgPooling(doc, chunks)
	default:
		log.Warnf("unknown big text plan: %s, using default chunkText", r.BigTextPlan)
		return r.processChunkText(doc, chunks)
	}
}

// processChunkText 将文本分割成多个文档分别存储
func (r *RAGSystem) processChunkText(originalDoc Document, chunks []string) ([]Document, error) {
	log.Infof("processing document %s with chunkText strategy, creating %d documents", originalDoc.ID, len(chunks))

	if len(chunks) == 0 {
		return []Document{}, nil
	}

	// 并行处理结果结构
	type chunkResult struct {
		index int
		doc   Document
		err   error
	}

	// 创建结果channel和sync.WaitGroup
	resultChan := make(chan chunkResult, len(chunks))
	var wg sync.WaitGroup

	// 并行处理每个chunk
	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, chunkText string) {
			defer wg.Done()

			// 为每个分块生成嵌入
			embedding, err := r.Embedder.Embedding(chunkText)
			if err != nil {
				log.Errorf("failed to generate embedding for chunk %d of document %s: %v", index, originalDoc.ID, err)
				resultChan <- chunkResult{
					index: index,
					err:   utils.Errorf("failed to generate embedding for chunk %d: %v", index, err),
				}
				return
			}

			if len(embedding) == 0 {
				log.Warnf("empty embedding for chunk %d of document %s, skipping", index, originalDoc.ID)
				resultChan <- chunkResult{
					index: index,
					err:   nil, // 空embedding不是错误，只是跳过
				}
				return
			}

			// 创建新文档
			chunkDoc := Document{
				ID:        fmt.Sprintf("%s_chunk_%d", originalDoc.ID, index),
				Content:   chunkText,
				Metadata:  make(map[string]any),
				Embedding: embedding,
			}

			// 复制原始文档的元数据
			for k, v := range originalDoc.Metadata {
				chunkDoc.Metadata[k] = v
			}

			// 添加分块特有的元数据
			chunkDoc.Metadata["original_doc_id"] = originalDoc.ID
			chunkDoc.Metadata["chunk_index"] = index
			chunkDoc.Metadata["total_chunks"] = len(chunks)
			chunkDoc.Metadata["is_chunk"] = true

			resultChan <- chunkResult{
				index: index,
				doc:   chunkDoc,
				err:   nil,
			}
		}(i, chunk)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	results := make([]chunkResult, 0, len(chunks))
	for result := range resultChan {
		results = append(results, result)
	}

	// 检查错误
	var firstError error
	validDocs := make([]Document, 0, len(chunks))

	// 按原始索引顺序排序结果
	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	for _, result := range results {
		if result.err != nil {
			if firstError == nil {
				firstError = result.err
			}
			continue
		}

		// 只有非空embedding的文档才添加到结果中
		if len(result.doc.Embedding) > 0 {
			validDocs = append(validDocs, result.doc)
		}
	}

	// 如果有错误，返回第一个错误
	if firstError != nil {
		return nil, firstError
	}

	log.Infof("successfully created %d chunk documents for %s", len(validDocs), originalDoc.ID)
	return validDocs, nil
}

// processChunkTextAndAvgPooling 将文本分割后生成多个嵌入向量，然后平均池化成一个文档存储
func (r *RAGSystem) processChunkTextAndAvgPooling(originalDoc Document, chunks []string) ([]Document, error) {
	log.Infof("processing document %s with chunkTextAndAvgPooling strategy", originalDoc.ID)

	var embeddings [][]float32
	var combinedContent string

	// 为每个分块生成嵌入
	for i, chunk := range chunks {
		embedding, err := r.Embedder.Embedding(chunk)
		if err != nil {
			log.Errorf("failed to generate embedding for chunk %d of document %s: %v", i, originalDoc.ID, err)
			return nil, utils.Errorf("failed to generate embedding for chunk %d: %v", i, err)
		}

		if len(embedding) == 0 {
			log.Warnf("empty embedding for chunk %d of document %s, skipping", i, originalDoc.ID)
			continue
		}

		embeddings = append(embeddings, embedding)
		if i == 0 {
			combinedContent = chunk
		} else {
			combinedContent += " " + chunk
		}
	}

	if len(embeddings) == 0 {
		return nil, utils.Errorf("no valid embeddings generated for document %s", originalDoc.ID)
	}

	// 对所有嵌入向量进行平均池化
	avgEmbedding := averagePooling(embeddings)
	if avgEmbedding == nil {
		return nil, utils.Errorf("failed to compute average pooling for document %s", originalDoc.ID)
	}

	// 创建合并后的文档
	pooledDoc := Document{
		ID:        originalDoc.ID,
		Content:   combinedContent,
		Metadata:  make(map[string]any),
		Embedding: avgEmbedding,
	}

	// 复制原始文档的元数据
	for k, v := range originalDoc.Metadata {
		pooledDoc.Metadata[k] = v
	}

	// 添加池化特有的元数据
	pooledDoc.Metadata["is_pooled"] = true
	pooledDoc.Metadata["pooled_chunks"] = len(embeddings)
	pooledDoc.Metadata["pooling_method"] = "average"

	log.Infof("successfully created pooled document for %s from %d chunks", originalDoc.ID, len(embeddings))
	return []Document{pooledDoc}, nil
}

// QueryWithPage 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) QueryWithPage(query string, page, limit int) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	return r.VectorStore.Search(query, page, limit)
}

func (r *RAGSystem) QueryWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	results, err := r.VectorStore.SearchWithFilter(query, page, limit, func(key string, getDoc func() *Document) bool {
		if filter != nil {
			return filter(key, getDoc)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Query is short for QueryTopN
func (r *RAGSystem) Query(query string, topN int, limits ...float64) ([]SearchResult, error) {
	return r.QueryTopN(query, topN, limits...)
}

// QueryTopN 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) QueryTopN(query string, topN int, limits ...float64) ([]SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query top_n %s: %v", query, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	if topN <= 0 {
		topN = 20
	}

	var page = 1
	var limit float64 = -1
	if len(limits) > 0 {
		limit = limits[0]
	}

	if limit >= 1 {
		topN = utils.Max(topN, int(limit))
		log.Warnf("limit should be less than 1, got %f, using -1 instead, use topN: %v (Max(topN, int(limit:%v)))", limit, topN, limit)
		limit = -1
	}

	log.Infof("start to search in vector storage with query: %#v", query)
	results, err := r.VectorStore.Search(query, page, topN)
	if err != nil {
		return nil, err
	}

	var filteredResults []SearchResult
	for _, result := range results {
		if limit < 0 || result.Score >= limit {
			filteredResults = append(filteredResults, result)
		}
	}

	return filteredResults, nil
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

func QueryCollection(db *gorm.DB, query string, opts ...aispec.AIConfigOption) ([]*SearchResult, error) {
	log.Infof("searching for collections matching query: %s", query)

	// 1. 首先查找所有集合信息文档
	var collectionDocs []*schema.VectorStoreDocument
	err := db.Model(&schema.VectorStoreDocument{}).Where("document_id = ?", DocumentTypeCollectionInfo).Find(&collectionDocs).Error
	if err != nil {
		return nil, utils.Errorf("failed to query collection documents: %v", err)
	}

	if len(collectionDocs) == 0 {
		log.Warnf("no collections found in database")
		return []*SearchResult{}, nil
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
	var results []*SearchResult
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
		document := Document{
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
			var collections []*schema.VectorStoreCollection
			if collectionIDInt, ok := collectionID.(float64); ok {
				db.Model(&schema.VectorStoreCollection{}).Where("id = ?", uint(collectionIDInt)).Find(&collections)
			}
			if len(collections) > 0 {
				collection := collections[0]
				document.Content = fmt.Sprintf("collection_name: %s\ncollection_description: %s\nmodel_name: %s\ndimension: %d",
					collection.Name, collection.Description, collection.ModelName, collection.Dimension)
			}
		}

		results = append(results, &SearchResult{
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
func GetDefaultEmbedder() (EmbeddingClient, error) {
	return GetLocalEmbeddingService()
}

// IsDefaultEmbedderReady 检查默认嵌入服务是否已准备就绪
func IsDefaultEmbedderReady() bool {
	return IsServiceRunning()
}
