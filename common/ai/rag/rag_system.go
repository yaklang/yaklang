package rag

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed prompt/gen_questions.txt
var genQuestionsPrompt string

// RAGSystem 表示完整的 RAG 系统
type RAGSystem struct {
	VectorStore      *vectorstore.SQLiteVectorStoreHNSW // 集合管理
	KnowledgeBase    *knowledgebase.KnowledgeBase       // 知识库管理
	EntityRepository *entityrepos.EntityRepository      // 实体仓库管理
	Name             string
	RAGID            string

	config *RAGSystemConfig
	opts   []RAGSystemConfigOption
}

func newDefaultRAGSystem() *RAGSystem {
	return &RAGSystem{}
}

func NewRAGSystem(options ...RAGSystemConfigOption) (*RAGSystem, error) {
	config := NewRAGSystemConfig(options...)

	if utils.IsNil(config.embeddingClient) && !vectorstore.IsMockMode {
		embedder, err := vectorstore.GetAIBalanceFreeEmbeddingService()
		if err != nil {
			return nil, utils.Wrap(err, "aibalance embedder and local embedder all failed")
		}
		config.embeddingClient = embedder

		// 设置归一化的模型名称和维度，确保与本地模型兼容
		if config.modelName == "" {
			config.modelName = embedder.GetModelName() // "Qwen3-Embedding-0.6B"
		}
		if config.modelDimension == 0 {
			config.modelDimension = embedder.GetModelDimension() // 1024
		}
	}

	err := autoMigrateRAGSystem(config.db)
	if err != nil {
		return nil, utils.Wrap(err, "auto migrate rag system failed")
	}
	colInfo, err := loadCollectionInfoByConfig(config)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			colInfo, err = vectorstore.CreateCollectionRecord(config.db, config.Name, config.description, config.ConvertToVectorStoreOptions()...)
			if err != nil {
				return nil, utils.Wrap(err, "failed to create collection record")
			}
		} else {
			return nil, utils.Wrap(err, "failed to load collection info")
		}
	}

	runImported := false
	importFile := func(force bool) error {
		file, err := os.Open(config.importFile)
		if err != nil {
			return utils.Wrap(err, "failed to open aikb file")
		}
		defer file.Close()
		header, err := LoadRAGFileHeader(file)
		if err != nil {
			return utils.Wrap(err, "failed to import rag collection")
		}

		if colInfo.SerialVersionUID != header.Collection.SerialVersionUID || force {
			runImported = true
			log.Infof("collection serialVersionUID mismatch, update collection")
			err = DeleteRAG(config.db, colInfo.Name)
			if err != nil {
				return utils.Wrap(err, "failed to delete rag collection")
			}
			defaultOpts := slices.Clone(options)
			defaultOpts = append(defaultOpts, WithExportOverwriteExisting(true), WithImportFile(""))
			err := ImportRAG(config.importFile, defaultOpts...)
			if err != nil {
				return utils.Wrap(err, "failed to import rag collection")
			}
		}
		return nil
	}
	if config.importFile != "" {
		err := importFile(false)
		if err != nil {
			return nil, err
		}
	}
	ragSystem, err := _newRAGSystem(options...)
	if err != nil {
		if runImported {
			return nil, utils.Errorf("load rag file failed: %v", err)
		} else if config.importFile != "" {
			log.Errorf("load rag system %s failed: %v, try to import file rag file", config.Name, err)
			err = importFile(true)
			if err != nil {
				return nil, utils.Errorf("import rag file failed: %v", err)
			}
			return _newRAGSystem(options...)
		} else {
			return nil, err
		}
	}
	return ragSystem, nil
}

// NewRAGSystem 创建一个新的 RAG 系统
func _newRAGSystem(options ...RAGSystemConfigOption) (*RAGSystem, error) {
	ragConfig := NewRAGSystemConfig(options...)
	ragSystem := newDefaultRAGSystem()
	ragSystem.config = ragConfig
	ragSystem.opts = options
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
			knowledgeBaseInfo, err := yakit.GetKnowledgeBaseByRAGID(ragConfig.db, ragSystem.RAGID)
			var knowledgeBaseName string
			if err != nil || knowledgeBaseInfo == nil {
				knowledgeBaseName = ragConfig.Name
			} else {
				knowledgeBaseName = knowledgeBaseInfo.KnowledgeBaseName
			}
			knowledgeBase, err := knowledgebase.NewKnowledgeBaseWithVectorStore(ragConfig.db, knowledgeBaseName, ragConfig.description, ragConfig.knowledgeBaseType, ragConfig.tags, ragSystem.VectorStore)
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
			entityRepositoryInfo, err := yakit.GetEntityRepositoryByRAGID(ragConfig.db, ragSystem.RAGID)
			var entityRepositoryName string
			if err != nil || entityRepositoryInfo == nil {
				entityRepositoryName = ragConfig.Name
			} else {
				entityRepositoryName = entityRepositoryInfo.EntityBaseName
			}
			entityRepository, err := entityrepos.GetEntityRepositoryWithVectorStore(ragConfig.db, entityRepositoryName, ragConfig.description, ragSystem.VectorStore)
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

	defaultOpts := []RAGSystemConfigOption{
		WithName(name),
	}
	options := append(defaultOpts, opts...)
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
	var embedder aispec.EmbeddingCaller
	var err error
	embedder, err = vectorstore.GetLocalEmbeddingService()
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

func (r *RAGSystem) newRagSystemConfig(opts ...RAGSystemConfigOption) *RAGSystemConfig {
	return NewRAGSystemConfig(append(r.opts, opts...)...)
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

func (r *RAGSystem) QueryKnowledge(query string, topN int, limits ...float64) ([]*schema.KnowledgeBaseEntry, error) {
	resCh, err := knowledgebase.Query(r.config.db, query,
		knowledgebase.WithLimit(topN),
		knowledgebase.WithCollectionName(r.KnowledgeBase.GetName()),
		knowledgebase.WithEmbeddingClient(r.GetEmbedder()),
		knowledgebase.WithFilter(func(key string, getDoc func() *vectorstore.Document, knowledgeBaseEntryGetter func() (*schema.KnowledgeBaseEntry, error)) bool {
			return getDoc().Type == schema.RAGDocumentType_Knowledge
		}),
	)
	if err != nil {
		return nil, err
	}
	results := make([]*schema.KnowledgeBaseEntry, 0)
	for result := range resCh {
		if result == nil {
			continue
		}
		if result.Type == "result" {
			entry := result.Data.(*schema.KnowledgeBaseEntry)
			results = append(results, entry)
		}
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

func (r *RAGSystem) CountKnowledge() (int, error) {
	return r.KnowledgeBase.CountKnowledgeEntries()
}

func (r *RAGSystem) DeleteEmbeddingData() error {
	return r.VectorStore.DeleteEmbeddingData()
}

func (r *RAGSystem) AddKnowledgeEntryQuestion(entry *schema.KnowledgeBaseEntry, options ...RAGSystemConfigOption) error {
	config := r.newRagSystemConfig(options...)
	docOpts := config.ConvertToDocumentOptions()
	noPQ := config.GetNoPotentialQuestions()
	return r.KnowledgeBase.AddKnowledgeEntryQuestion(entry, noPQ, docOpts...)
}

func (r *RAGSystem) AddKnowledge(knowledge any, options ...RAGSystemConfigOption) error {
	switch ret := knowledge.(type) {
	case *schema.KnowledgeBaseEntry:
		return r.AddKnowledgeEntry(knowledge.(*schema.KnowledgeBaseEntry), options...)
	case string:
		return r.AddKnowledgeEntry(&schema.KnowledgeBaseEntry{
			KnowledgeBaseID:  r.GetKnowledgeBaseID(),
			KnowledgeType:    "Standard",
			KnowledgeDetails: knowledge.(string),
			HiddenIndex:      uuid.NewString(),
		}, options...)
	case map[string]any:
		return r.AddKnowledgeEntry(&schema.KnowledgeBaseEntry{
			KnowledgeBaseID:    r.GetKnowledgeBaseID(),
			KnowledgeType:      utils.InterfaceToString(ret["knowledge_type"]),
			KnowledgeTitle:     utils.InterfaceToString(ret["title"]),
			KnowledgeDetails:   utils.InterfaceToString(ret["details"]),
			Summary:            utils.InterfaceToString(ret["summary"]),
			Keywords:           utils.InterfaceToStringSlice(ret["keywords"]),
			ImportanceScore:    utils.InterfaceToInt(ret["importance_score"]),
			SourcePage:         utils.InterfaceToInt(ret["source_page"]),
			PotentialQuestions: utils.InterfaceToStringSlice(ret["potential_questions"]),
			HiddenIndex:        uuid.NewString(),
		}, options...)
	default:
		return utils.Errorf("unknown knowledge type: %T", knowledge)
	}
	return nil
}

func (r *RAGSystem) AddKnowledgeEntry(entry *schema.KnowledgeBaseEntry, options ...RAGSystemConfigOption) error {
	if entry.HiddenIndex == "" {
		entry.HiddenIndex = uuid.NewString()
	}

	ragConfig := r.newRagSystemConfig(options...)
	docOpts := ragConfig.ConvertToDocumentOptions()
	err := r.KnowledgeBase.AddKnowledgeEntry(entry, docOpts...)
	if err != nil {
		return utils.Wrap(err, "failed to add knowledge entry")
	}
	if ragConfig.enableDocumentQuestionIndex {
		questionsMap, err := enhancesearch.BuildIndexQuestions([]string{entry.KnowledgeDetails}, ragConfig.GetAIService())
		if err != nil {
			return utils.Wrap(err, "failed to build index questions")
		}
		var questions []string
		for _, qs := range questionsMap {
			questions = append(questions, qs...)
		}
		if len(questions) > 0 {
			entry.HasQuestionIndex = true
			// 注意：这里可能需要更新 entry 到数据库，但这通常在调用 AddKnowledgeEntry 的上层或者由 KnowledgeBase.AddKnowledgeEntry 处理
			// 实际上，KnowledgeBase.AddKnowledgeEntry 已经将 entry 保存到数据库了。
			// 我们需要在这里更新 HasQuestionIndex 字段
			if err := ragConfig.db.Model(entry).Update("has_question_index", true).Error; err != nil {
				log.Errorf("failed to update has_question_index for entry %s: %v", entry.HiddenIndex, err)
			}
		}
		for _, question := range questions {
			questionId := entry.HiddenIndex + "_question_" + utils.CalcSha1(question)
			entry.PotentialQuestions = append(entry.PotentialQuestions, question)
			r.VectorStore.AddWithOptions(questionId, question, append(docOpts,
				vectorstore.WithDocumentQuestionIndex(true),
				vectorstore.WithDocumentMetadataKeyValue(schema.META_Data_UUID, entry.HiddenIndex),
				// vectorstore.WithDocumentMetadataKeyValue(schema.META_Data_Title, question),
				vectorstore.WithDocumentMetadataKeyValue(schema.META_KNOWLEDGE_TITLE, entry.KnowledgeTitle),
			)...)
		}
	}
	return nil
}

func (r *RAGSystem) GenerateQuestionIndexForKnowledge(hiddenIndex string, options ...RAGSystemConfigOption) error {
	ragConfig := r.newRagSystemConfig(options...)
	docOpts := ragConfig.ConvertToDocumentOptions()

	entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(ragConfig.db, hiddenIndex)
	if err != nil {
		return utils.Wrap(err, "failed to get knowledge base entry")
	}

	questionsMap, err := enhancesearch.BuildIndexQuestions([]string{entry.KnowledgeDetails}, ragConfig.GetAIService())
	if err != nil {
		return utils.Wrap(err, "failed to build index questions")
	}
	var questions []string
	for _, qs := range questionsMap {
		questions = append(questions, qs...)
	}
	if len(questions) > 0 {
		entry.HasQuestionIndex = true
		// 注意：这里可能需要更新 entry 到数据库，但这通常在调用 AddKnowledgeEntry 的上层或者由 KnowledgeBase.AddKnowledgeEntry 处理
		// 实际上，KnowledgeBase.AddKnowledgeEntry 已经将 entry 保存到数据库了。
		// 我们需要在这里更新 HasQuestionIndex 字段
		if err := ragConfig.db.Model(entry).Update("has_question_index", true).Error; err != nil {
			log.Errorf("failed to update has_question_index for entry %s: %v", entry.HiddenIndex, err)
		}
	}
	for _, question := range questions {
		questionId := entry.HiddenIndex + "_question_" + utils.CalcSha1(question)
		entry.PotentialQuestions = append(entry.PotentialQuestions, question)
		r.VectorStore.AddWithOptions(questionId, question, append(docOpts,
			vectorstore.WithDocumentQuestionIndex(true),
			vectorstore.WithDocumentMetadataKeyValue(schema.META_Data_UUID, entry.HiddenIndex),
			// vectorstore.WithDocumentMetadataKeyValue(schema.META_Data_Title, question),
			vectorstore.WithDocumentMetadataKeyValue(schema.META_KNOWLEDGE_TITLE, entry.KnowledgeTitle),
		)...)
	}
	return nil
}

func (r *RAGSystem) GetKnowledgeBaseID() int64 {
	return r.KnowledgeBase.GetID()
}

func (r *RAGSystem) GenerateQuestionIndex(options ...RAGSystemConfigOption) error {
	config := r.newRagSystemConfig(options...)
	if config.progressHandler == nil {
		config.progressHandler = func(percent float64, message string, messageType string) {
			log.Infof("GenerateQuestionIndex progress: %f%%, message: %s", percent, message)
		}
	}
	// 1. 遍历知识库的所有项
	page := 1
	limit := 50
	db := config.db

	var totalEntries int
	db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", r.GetKnowledgeBaseID()).Count(&totalEntries)
	config.progressHandler(0, fmt.Sprintf("start to generate question index for %d entries", totalEntries), "info")
	processedCount := 0

	var pendingEntries []*schema.KnowledgeBaseEntry
	processBatch := func(batch []*schema.KnowledgeBaseEntry) {
		if len(batch) == 0 {
			return
		}
		var batchContent []string
		for _, e := range batch {
			batchContent = append(batchContent, e.KnowledgeDetails)
		}

		log.Infof("generating question index for batch of %d entries", len(batch))
		questionsMap, err := enhancesearch.BuildIndexQuestions(batchContent, config.GetAIService())
		if err != nil {
			log.Errorf("failed to build index questions for batch: %v", err)
			return
		}

		docOpts := config.ConvertToDocumentOptions()
		for _, entry := range batch {
			var uniqueQuestions = make(map[string]struct{})
			for snippet, qs := range questionsMap {
				// key 是片段，如果条目内容包含该片段，或者片段包含条目内容，则认为该问题属于该条目
				if strings.Contains(snippet, entry.KnowledgeDetails) {
					for _, q := range qs {
						uniqueQuestions[q] = struct{}{}
					}
				}
			}

			if len(uniqueQuestions) == 0 {
				continue
			}

			for question := range uniqueQuestions {
				questionId := entry.HiddenIndex + "_question_" + utils.CalcSha1(question)
				err := r.VectorStore.AddWithOptions(questionId, question, append(docOpts,
					vectorstore.WithDocumentQuestionIndex(true),
					vectorstore.WithDocumentMetadataKeyValue(schema.META_Data_UUID, entry.HiddenIndex),
					vectorstore.WithDocumentMetadataKeyValue(schema.META_KNOWLEDGE_TITLE, entry.KnowledgeTitle),
				)...)
				if err != nil {
					log.Errorf("failed to add question index %s: %v", questionId, err)
				}
			}

			// 更新 HasQuestionIndex
			if err := db.Model(entry).Update("has_question_index", true).Error; err != nil {
				log.Errorf("failed to update has_question_index for entry %s: %v", entry.HiddenIndex, err)
			}
		}
	}

	for {
		var entries []*schema.KnowledgeBaseEntry
		err := db.Model(&schema.KnowledgeBaseEntry{}).
			Where("knowledge_base_id = ? AND (has_question_index IS NULL OR has_question_index = ?)", r.GetKnowledgeBaseID(), false).
			Limit(limit).
			Offset((page - 1) * limit).
			Find(&entries).Error

		if err != nil {
			return utils.Errorf("failed to get knowledge base entries: %v", err)
		}
		if len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			processedCount++
			percent := float64(processedCount) / float64(totalEntries) * 100
			config.progressHandler(percent, fmt.Sprintf("processing entry: %s", entry.KnowledgeTitle), "info")

			// 加入待处理列表
			pendingEntries = append(pendingEntries, entry)
			if len(pendingEntries) >= 10 {
				processBatch(pendingEntries)
				pendingEntries = nil
			}
		}

		if len(entries) < limit {
			break
		}
		page++
	}
	// 处理剩余的 pendingEntries
	processBatch(pendingEntries)

	config.progressHandler(100, "generate question index finished", "success")
	return nil
}
