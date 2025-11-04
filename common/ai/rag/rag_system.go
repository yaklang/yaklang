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
	ragSystem.config = ragConfig

	// 创建collection
	if ragConfig.vectorStore != nil {
		ragSystem.VectorStore = ragConfig.vectorStore
	} else {
		collection, err := vectorstore.GetCollection(ragConfig.db, ragConfig.name, ragConfig.ConvertToVectorStoreOptions()...)
		if err != nil {
			return nil, err
		}
		ragSystem.VectorStore = collection
	}

	// 检查rag_id
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
			knowledgeBase, err := knowledgebase.NewKnowledgeBase(ragConfig.db, ragConfig.name, ragConfig.description, ragConfig.knowledgeBaseType, ragConfig.ConvertToVectorStoreOptions()...)
			if err != nil {
				return nil, err
			}
			info := knowledgeBase.GetKnowledgeBaseInfo()
			if info.RAGID == "" {
				info.RAGID = uuid.NewString()
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
			entityRepository, err := entityrepos.GetEntityRepositoryByName(ragConfig.db, ragConfig.name)
			if err != nil {
				return nil, err
			}
			info, err := entityRepository.GetInfo()
			if err != nil {
				return nil, err
			}
			if info.RAGID == "" {
				info.RAGID = uuid.NewString()
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
	// 集合存在并且有rag_id
	return collection.RAGID != ""
}

func GetRagSystem(name string, opts ...RAGSystemConfigOption) (*RAGSystem, error) {
	defaultOptions := []RAGSystemConfigOption{
		WithName(name),
	}
	config := NewRAGSystemConfig(opts...)
	if HasRagSystem(config.db, name) {
		return NewRAGSystem(append(defaultOptions, opts...)...)
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
