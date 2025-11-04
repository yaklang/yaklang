package rag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// RAGSystem 表示完整的 RAG 系统
type RAGSystem struct {
	VectorStore      *vectorstore.SQLiteVectorStoreHNSW // 集合管理
	KnowledgeBase    *knowledgebase.KnowledgeBase       // 知识库管理
	EntityRepository *entityrepos.EntityRepository      // 实体仓库管理
	Name             string
	RAGID            string

	config *RAGSystemConfig
}

func newDefaultRAGSystem() *RAGSystem {
	return &RAGSystem{}
}

// NewRAGSystem 创建一个新的 RAG 系统
func NewRAGSystem(options ...RAGSystemConfigOption) (*RAGSystem, error) {
	ragConfig := NewRAGSystemConfig(options...)
	ragSystem := newDefaultRAGSystem()
	err := autoMigrateRAGSystem(ragConfig.db)
	if err != nil {
		return nil, utils.Errorf("auto migrate rag system failed: %v", err)
	}
	ragSystem.config = ragConfig

	// 创建collection
	if ragConfig.vectorStore != nil {
		ragSystem.VectorStore = ragConfig.vectorStore
	} else {
		if ragSystem.config.ragID != "" {
			var collection schema.VectorStoreCollection
			err := ragConfig.db.Model(&schema.VectorStoreCollection{}).Where("rag_id = ?", ragSystem.config.ragID).First(&collection).Error
			if err != nil {
				return nil, utils.Errorf("get collection by rag_id %s failed: %v", ragSystem.config.ragID, err)
			}
			collectionMg, err := vectorstore.GetCollection(ragConfig.db, collection.Name, ragConfig.ConvertToVectorStoreOptions()...)
			if err != nil {
				return nil, utils.Errorf("get collection failed: %v", err)
			}
			ragSystem.VectorStore = collectionMg
		} else {
			collection, err := vectorstore.GetCollection(ragConfig.db, ragConfig.Name, ragConfig.ConvertToVectorStoreOptions()...)
			if err != nil {
				return nil, err
			}
			ragSystem.VectorStore = collection
		}
	}

	// 检查ragid
	ragId := ragSystem.VectorStore.GetCollectionInfo().RAGID
	if ragId == "" {
		ragId = uuid.NewString()
		ragSystem.VectorStore.GetCollectionInfo().RAGID = ragId
		err := ragConfig.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", ragSystem.VectorStore.GetCollectionInfo().ID).Update("rag_id", ragId).Error
		if err != nil {
			return nil, utils.Errorf("update rag_id failed: %v", err)
		}
	}
	ragSystem.RAGID = ragId

	// 创建knowledge base
	if ragConfig.enableKnowledgeBase {
		if ragConfig.knowledgeBase != nil {
			ragSystem.KnowledgeBase = ragConfig.knowledgeBase
		} else {
			knowledgeBase, err := knowledgebase.NewKnowledgeBaseWithVectorStore(ragConfig.db, ragConfig.Name, ragConfig.description, ragConfig.knowledgeBaseType, ragSystem.VectorStore)
			if err != nil {
				return nil, err
			}
			info := knowledgeBase.GetKnowledgeBaseInfo()
			if info.RAGID == "" {
				info.RAGID = ragSystem.RAGID
			}
			err = ragConfig.db.Model(&schema.KnowledgeBaseInfo{}).Where("id = ?", info.ID).Update("rag_id", info.RAGID).Error
			if err != nil {
				return nil, utils.Errorf("update rag_id failed: %v", err)
			}
			ragSystem.KnowledgeBase = knowledgeBase
		}
	}

	// 创建entity repository
	if ragConfig.enableEntityRepository {
		if ragConfig.entityRepository != nil {
			ragSystem.EntityRepository = ragConfig.entityRepository
		} else {
			entityRepository, err := entityrepos.GetEntityRepositoryWithVectorStore(ragConfig.db, ragConfig.Name, ragConfig.description, ragSystem.VectorStore)
			if err != nil {
				return nil, err
			}
			info, err := entityRepository.GetInfo()
			if err != nil {
				return nil, err
			}
			if info.RAGID == "" {
				info.RAGID = ragSystem.RAGID
			}
			err = ragConfig.db.Model(&schema.EntityRepository{}).Where("id = ?", info.ID).Update("rag_id", info.RAGID).Error
			if err != nil {
				return nil, utils.Errorf("update rag_id failed: %v", err)
			}
			ragSystem.EntityRepository = entityRepository
		}
	}

	return ragSystem, nil
}

func HasRagSystem(db *gorm.DB, name string) bool {
	collection, err := yakit.GetRAGCollectionInfoByName(db, name)
	if err != nil {
		return false
	}
	if collection == nil {
		return false
	}
	// 集合存在并且有ragid
	return collection.RAGID != ""
}

func LoadRAGSystem(name string, opts ...RAGSystemConfigOption) (*RAGSystem, error) {
	config := NewRAGSystemConfig(opts...)
	if !HasRagSystem(config.db, name) {
		return nil, utils.Errorf("rag collection[%v] not existed", name)
	}

	collection, err := yakit.GetRAGCollectionInfoByName(config.db, name)
	if err != nil {
		return nil, fmt.Errorf("get collection failed: %v", err)
	}

	options := append(opts, WithRAGID(collection.RAGID))
	return NewRAGSystem(options...)
}

func GetRagSystem(name string, opts ...RAGSystemConfigOption) (*RAGSystem, error) {
	defaultOptions := []RAGSystemConfigOption{
		WithName(name),
	}
	config := NewRAGSystemConfig(opts...)
	if HasRagSystem(config.db, name) {
		return LoadRAGSystem(name, opts...)
	} else {
		return NewRAGSystem(append(defaultOptions, opts...)...)
	}
}

// NewRAGSystemWithLocalEmbedding 创建使用本地模型嵌入的 RAG 系统
// 自动启动本地嵌入服务，如果无法启动则报错
func NewRAGSystemWithLocalEmbedding(store *vectorstore.SQLiteVectorStoreHNSW) (*RAGSystem, error) {
	embedder, err := vectorstore.GetLocalEmbeddingService()
	if err != nil {
		return nil, err
	}
	return NewRAGSystem(WithVectorStore(store), WithEmbeddingClient(embedder))
}

// NewRAGSystemWithOptionalEmbedding 创建 RAG 系统，支持可选的嵌入服务
// 如果 embedder 为 nil，则使用默认的本地嵌入服务
func NewRAGSystemWithOptionalEmbedding(store *vectorstore.SQLiteVectorStoreHNSW, embedder vectorstore.EmbeddingClient) (*RAGSystem, error) {
	if embedder == nil {
		log.Infof("no embedder provided, using default local embedding service")
		return NewRAGSystemWithLocalEmbedding(store)
	}

	log.Infof("using provided embedder for RAG system")
	return NewRAGSystem(WithVectorStore(store), WithEmbeddingClient(embedder))
}

// VectorSimilarity 快速计算两个文本的向量相似度
func (r *RAGSystem) VectorSimilarity(text1, text2 string) (float64, error) {
	embedder := r.VectorStore.GetEmbedder()
	embeddingData1, err := embedder.Embedding(text1)
	if err != nil {
		return 0, err
	}
	embeddingData2, err := embedder.Embedding(text2)
	if err != nil {
		return 0, err
	}
	return hnsw.CosineSimilarity(embeddingData1, embeddingData2)
}

func (r *RAGSystem) Has(docId string) bool {
	return r.VectorStore.Has(docId)
}

func (r *RAGSystem) Add(docId string, content string, opts ...RAGSystemConfigOption) error {
	docOpts := NewRAGSystemConfig(opts...).ConvertToDocumentOptions()
	return r.VectorStore.AddWithOptions(docId, content, docOpts...)
}

func (r *RAGSystem) GetEmbedder() vectorstore.EmbeddingClient {
	return r.VectorStore.GetEmbedder()
}

func BuildDocument(docId, content string, opts ...RAGSystemConfigOption) *vectorstore.Document {
	docOpts := NewRAGSystemConfig(opts...).ConvertToDocumentOptions()
	doc := &vectorstore.Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	for _, opt := range docOpts {
		opt(doc)
	}
	return doc
}

func (r *RAGSystem) AddDocuments(docs ...*vectorstore.Document) error {
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
func (r *RAGSystem) QueryWithPage(query string, page, limit int) ([]*SearchResult, error) {
	res, err := r.VectorStore.QueryWithPage(query, page, limit)
	if err != nil {
		return nil, err
	}
	results := make([]*SearchResult, 0)
	for _, result := range res {
		results = append(results, r.fillSearchResult(&result))
	}
	return results, nil
}

func (r *RAGSystem) QueryWithFilter(query string, page, limit int, filter func(key string, getDoc func() *vectorstore.Document) bool) ([]*SearchResult, error) {
	res, err := r.VectorStore.QueryWithFilter(query, page, limit, filter)
	if err != nil {
		return nil, err
	}
	results := make([]*SearchResult, 0)
	for _, result := range res {
		results = append(results, r.fillSearchResult(&result))
	}
	return results, nil
}

func (r *RAGSystem) fillSearchResult(result *vectorstore.SearchResult) *SearchResult {
	res := &SearchResult{
		Document: result.Document,
		Score:    result.Score,
	}
	doc := result.Document
	dataUUID, ok := doc.Metadata.GetDataUUID()
	if ok {
		entity, err := yakit.GetEntityByIndex(r.config.db, dataUUID)
		if err == nil {
			res.Entity = entity
		}
		knowledgeEntry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(r.config.db, dataUUID)
		if err == nil {
			res.KnowledgeBaseEntry = knowledgeEntry
		}
	}

	return res
}

// FuzzRawSearch Sql 文本模糊搜索（非语义）
func (r *RAGSystem) FuzzRawSearch(ctx context.Context, keywords string, limit int) (<-chan *SearchResult, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("failed to query with page query: %s: %v", keywords, err)
			fmt.Println(utils.ErrorStack(err))
		}
	}()
	res, err := r.VectorStore.FuzzSearch(ctx, keywords, limit)
	if err != nil {
		return nil, err
	}
	outputCh := make(chan *SearchResult)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("failed to query with page query: %s: %v", keywords, err)
				fmt.Println(utils.ErrorStack(err))
			}
		}()
		for result := range res {
			outputCh <- r.fillSearchResult(&result)
		}
		close(outputCh)
	}()
	return outputCh, nil
}

// Query is short for QueryTopN
func (r *RAGSystem) Query(query string, topN int, limits ...float64) ([]*SearchResult, error) {
	return r.QueryTopN(query, topN, limits...)
}

// QueryTopN 根据查询文本检索相关文档并返回结果
func (r *RAGSystem) QueryTopN(query string, topN int, limits ...float64) ([]*SearchResult, error) {
	res, err := r.VectorStore.QueryTopN(query, topN, limits...)
	if err != nil {
		return nil, err
	}
	results := make([]*SearchResult, 0)
	for _, result := range res {
		results = append(results, r.fillSearchResult(&result))
	}
	return results, nil
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

func (r *RAGSystem) AddKnowledgeEntryQuestion(entry *schema.KnowledgeBaseEntry, options ...RAGSystemConfigOption) error {
	docOpts := NewRAGSystemConfig(options...).ConvertToDocumentOptions()
	return r.KnowledgeBase.AddKnowledgeEntryQuestion(entry, docOpts...)
}

func (r *RAGSystem) AddKnowledgeEntry(entry *schema.KnowledgeBaseEntry, options ...RAGSystemConfigOption) error {
	docOpts := NewRAGSystemConfig(options...).ConvertToDocumentOptions()
	return r.KnowledgeBase.AddKnowledgeEntry(entry, docOpts...)
}

func (r *RAGSystem) GetKnowledgeBaseID() int64 {
	return r.KnowledgeBase.GetID()
}
