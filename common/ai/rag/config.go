package rag

import (
	"context"
	"io"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type RAGSystemConfig struct {
	CollectionName                       string
	CollectionDescription                string
	CollectionModelName                  string
	CollectionModelDimension             int
	CollectionModelDistanceFuncType      string
	CollectionModelMaxNeighbors          int
	CollectionModelLayerGenerationFactor float64
	CollectionModelEfSearch              int
	CollectionModelEfConstruct           int

	embeddingModel string

	db                     *gorm.DB
	name                   string
	description            string
	knowledgeBaseType      string
	embeddingClient        aispec.EmbeddingCaller
	enableEntityRepository bool
	enableKnowledgeBase    bool
	aiOptions              []aispec.AIConfigOption
	forceNew               bool

	vectorStore      *vectorstore.SQLiteVectorStoreHNSW
	knowledgeBase    *knowledgebase.KnowledgeBase
	entityRepository *entityrepos.EntityRepository

	// Query configuration fields
	ctx                      context.Context
	limit                    int
	collectionLimit          int
	enhance                  []string
	enhanceSearchHandler     enhancesearch.SearchHandler
	systemLoadConfig         []vectorstore.CollectionConfigFunc
	similarityThreshold      float64
	msgCallback              func(*RAGSearchResult)
	logReader                func(reader io.Reader)
	everyQueryResultCallback func(result *vectorstore.ScoredResult)
	onQueryFinish            func([]*vectorstore.ScoredResult)
	concurrent               int
	onlyResults              bool
	collectionNames          []string
	collectionScoreLimit     float64
	queryStatusCallback      func(label string, i any, tags ...string)
	filterCallback           func(key string, getDoc func() *vectorstore.Document) bool
	documentTypes            []string

	// KHop configuration fields
	kHopK              int // k=0表示返回所有路径，k>0表示返回k-hop路径，k>=2
	kHopKMin           int // default 2 (minimum 2-hop paths)
	kHopKMax           int
	kHopLimit          int
	kHopPathDepth      int
	kHopStartFilter    *ypb.EntityFilter
	kHopRagQuery       string
	kHopIsRuntimeBuild bool // 是否只查询当前运行时的关系

	// Document and import/export configuration fields
	documentMetadataKeyValue map[string]any
	documentRawMetadata      map[string]any
	documentType             string
	documentEntityID         string
	documentRelatedEntities  []string
	documentRuntimeID        string
	importExportDB           *gorm.DB
	overwriteExisting        bool
	collectionName           string
	rebuildHNSWIndex         bool
	documentHandler          func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	progressHandler          func(percent float64, message string, messageType string) // 进度回调
	noHNSWGraph              bool
	noMetadata               bool
	noOriginInput            bool
	onlyPQCode               bool
	modelDimension           int
	modelName                string
	cosineDistance           bool

	// m int, ml float64, efSearch, efConstruct int
	hnswM           int
	hnswMl          float64
	hnswEfSearch    int
	hnswEfConstruct int

	lazyLoadEmbeddingClient bool
}

var defaultRAGSystemName = "default"
var defaultRAGSystemDescription = "default description"

func NewRAGSystemConfig(options ...RAGSystemConfigOption) *RAGSystemConfig {
	config := &RAGSystemConfig{
		db:                     consts.GetGormProfileDatabase(),
		name:                   defaultRAGSystemName,
		description:            defaultRAGSystemDescription,
		knowledgeBaseType:      "default",
		enableEntityRepository: false,
		enableKnowledgeBase:    false,
	}
	for _, option := range options {
		option(config)
	}
	return config
}

type RAGSystemConfigOption func(*RAGSystemConfig)

func WithExportNoHNSWIndex(noHNSWGraph bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noHNSWGraph = noHNSWGraph
	}
}

func WithExportOnlyPQCode(onlyPQCode bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.onlyPQCode = onlyPQCode
	}
}

func WithExportNoMetadata(noMetadata bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noMetadata = noMetadata
	}
}

func WithExportOverwriteExisting(overwriteExisting bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.overwriteExisting = overwriteExisting
	}
}

func WithExportNoOriginInput(noOriginInput bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noOriginInput = noOriginInput
	}
}

func WithImportRebuildHNSWIndex(rebuildHNSWIndex bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.rebuildHNSWIndex = rebuildHNSWIndex
	}
}

func WithAIServiceName(aiServiceName string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		chatter, err := ai.LoadChater(aiServiceName)
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
			return
		}
		config.aiOptions = append(config.aiOptions, aispec.WithAIServiceName(aiServiceName))
	}
}

func WithExportDocumentHandler(documentHandler func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentHandler = documentHandler
	}
}

func WithExportOnProgressHandler(progressHandler func(percent float64, message string, messageType string)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.progressHandler = progressHandler
	}
}

func WithKnowledgeBaseType(knowledgeBaseType string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.knowledgeBaseType = knowledgeBaseType
	}
}

func WithEnableEntityRepository(enable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enableEntityRepository = enable
	}
}

func WithEnableKnowledgeBase(enable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enableKnowledgeBase = enable
	}
}

func WithKnowledgeBase(knowledgeBase *knowledgebase.KnowledgeBase) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.knowledgeBase = knowledgeBase
	}
}

func WithEntityRepository(entityRepository *entityrepos.EntityRepository) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.entityRepository = entityRepository
	}
}

func WithEmbeddingClient(client aispec.EmbeddingCaller) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.embeddingClient = client
	}
}

func WithDescription(description string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.description = description
	}
}

func WithVectorStore(store *vectorstore.SQLiteVectorStoreHNSW) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.vectorStore = store
	}
}

func WithEmbeddingModel(model string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.embeddingModel = model
	}
}

func WithDB(db *gorm.DB) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.db = db
	}
}
func WithName(name string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.name = name
	}
}

func (config *RAGSystemConfig) ConvertToExportOptions() []vectorstore.RAGExportOptionFunc {
	options := []vectorstore.RAGExportOptionFunc{}
	if config.noHNSWGraph {
		options = append(options, vectorstore.WithNoHNSWGraph(config.noHNSWGraph))
	}
	if config.noMetadata {
		options = append(options, vectorstore.WithNoMetadata(config.noMetadata))
	}
	if config.noOriginInput {
		options = append(options, vectorstore.WithNoOriginInput(config.noOriginInput))
	}
	if config.onlyPQCode {
		options = append(options, vectorstore.WithOnlyPQCode(config.onlyPQCode))
	}
	if config.rebuildHNSWIndex {
		options = append(options, vectorstore.WithRebuildHNSWIndex(config.rebuildHNSWIndex))
	}
	if config.documentHandler != nil {
		options = append(options, vectorstore.WithDocumentHandler(config.documentHandler))
	}
	if config.progressHandler != nil {
		options = append(options, vectorstore.WithProgressHandler(config.progressHandler))
	}
	if config.collectionName != "" {
		options = append(options, vectorstore.WithCollectionName(config.collectionName))
	}
	if config.importExportDB != nil {
		options = append(options, vectorstore.WithImportExportDB(config.importExportDB))
	}
	if config.overwriteExisting {
		options = append(options, vectorstore.WithOverwriteExisting(config.overwriteExisting))
	}
	if config.ctx != nil {
		options = append(options, vectorstore.WithContext(config.ctx))
	}
	if config.db != nil {
		options = append(options, vectorstore.WithImportExportDB(config.db))
	}
	return options
}

func (config *RAGSystemConfig) ConvertToVectorStoreOptions() []vectorstore.CollectionConfigFunc {
	options := []vectorstore.CollectionConfigFunc{}
	if config.embeddingClient != nil {
		options = append(options, vectorstore.WithEmbeddingClient(config.embeddingClient))
	}
	if config.description != "" {
		options = append(options, vectorstore.WithDescription(config.description))
	}
	if config.forceNew {
		options = append(options, vectorstore.WithForceNew(config.forceNew))
	}
	if config.modelDimension > 0 {
		options = append(options, vectorstore.WithModelDimension(config.modelDimension))
	}
	if config.modelName != "" {
		options = append(options, vectorstore.WithModelName(config.modelName))
	}
	if config.cosineDistance {
		options = append(options, vectorstore.WithCosineDistance())
	}
	// if config.hnswM > 0 {
	// 	options = append(options, vectorstore.WithHNSWParameters(config.hnswM))
	// }
	// if config.hnswMl > 0 {
	// 	options = append(options, vectorstore.WithHNSWParameters(config.hnswMl))
	// }
	// if config.hnswEfSearch > 0 {
	// 	options = append(options, vectorstore.WithHNSWParameters(config.hnswEfSearch))
	// }

	return options
}

// ConvertToDocumentOptions converts RAGSystemConfig to document options
func (config *RAGSystemConfig) ConvertToDocumentOptions() []vectorstore.DocumentOption {
	options := []vectorstore.DocumentOption{}

	if len(config.documentMetadataKeyValue) > 0 {
		for key, value := range config.documentMetadataKeyValue {
			options = append(options, vectorstore.WithDocumentMetadataKeyValue(key, value))
		}
	}
	if len(config.documentRawMetadata) > 0 {
		options = append(options, vectorstore.WithDocumentRawMetadata(config.documentRawMetadata))
	}
	if config.documentType != "" {
		options = append(options, vectorstore.WithDocumentType(schema.RAGDocumentType(config.documentType)))
	}
	if config.documentEntityID != "" {
		options = append(options, vectorstore.WithDocumentEntityID(config.documentEntityID))
	}
	if len(config.documentRelatedEntities) > 0 {
		options = append(options, vectorstore.WithDocumentRelatedEntities(config.documentRelatedEntities...))
	}
	if config.documentRuntimeID != "" {
		options = append(options, vectorstore.WithDocumentRuntimeID(config.documentRuntimeID))
	}

	return options
}

func (config *RAGSystemConfig) ConvertToRAGQueryOptions() []vectorstore.CollectionQueryOption {
	options := []vectorstore.CollectionQueryOption{}

	if config.ctx != nil {
		options = append(options, vectorstore.WithRAGCtx(config.ctx))
	}
	if config.limit > 0 {
		options = append(options, vectorstore.WithRAGLimit(config.limit))
	}
	if config.collectionLimit > 0 {
		options = append(options, vectorstore.WithRAGCollectionLimit(config.collectionLimit))
	}
	if len(config.enhance) > 0 {
		options = append(options, vectorstore.WithRAGEnhance(config.enhance...))
	}
	if config.enhanceSearchHandler != nil {
		options = append(options, vectorstore.WithRAGEnhanceSearchHandler(config.enhanceSearchHandler))
	}
	if len(config.systemLoadConfig) > 0 {
		options = append(options, vectorstore.WithRAGSystemLoadConfig(config.systemLoadConfig...))
	}
	if config.similarityThreshold > 0 {
		options = append(options, vectorstore.WithRAGSimilarityThreshold(config.similarityThreshold))
	}
	if config.msgCallback != nil {
		options = append(options, vectorstore.WithRAGMsgCallBack(config.msgCallback))
	}
	if config.logReader != nil {
		options = append(options, vectorstore.WithRAGLogReader(config.logReader))
	}
	if config.everyQueryResultCallback != nil {
		options = append(options, vectorstore.WithEveryQueryResultCallback(config.everyQueryResultCallback))
	}
	if config.onQueryFinish != nil {
		options = append(options, vectorstore.WithRAGOnQueryFinish(config.onQueryFinish))
	}
	if config.concurrent > 0 {
		options = append(options, vectorstore.WithRAGConcurrent(config.concurrent))
	}
	if config.onlyResults {
		options = append(options, vectorstore.WithRAGOnlyResults(config.onlyResults))
	}
	if len(config.collectionNames) > 0 {
		options = append(options, vectorstore.WithRAGQueryCollectionNames(config.collectionNames...))
	}
	if config.collectionScoreLimit > 0 {
		options = append(options, vectorstore.WithRAGCollectionScoreLimit(config.collectionScoreLimit))
	}
	if config.queryStatusCallback != nil {
		options = append(options, vectorstore.WithRAGQueryStatus(config.queryStatusCallback))
	}
	if config.filterCallback != nil {
		options = append(options, vectorstore.WithRAGFilter(config.filterCallback))
	}
	if len(config.documentTypes) > 0 {
		options = append(options, vectorstore.WithRAGDocumentType(config.documentTypes...))
	}

	return options
}

func (config *RAGSystemConfig) ConvertToEntityRepositoryOptions() []entityrepos.RuntimeConfigOption {
	options := []entityrepos.RuntimeConfigOption{}
	return options
}

func (config *RAGSystemConfig) ConvertToKHopOptions() []entityrepos.KHopQueryOption {
	options := []entityrepos.KHopQueryOption{}

	if config.kHopLimit > 0 {
		options = append(options, entityrepos.WithKHopLimit(config.kHopLimit))
	}
	if config.kHopK >= 0 {
		options = append(options, entityrepos.WithKHopK(config.kHopK))
	}
	if config.kHopKMin >= 2 {
		options = append(options, entityrepos.WithKHopKMin(config.kHopKMin))
	}
	if config.kHopKMax > 0 {
		options = append(options, entityrepos.WithKHopKMax(config.kHopKMax))
	}
	if config.kHopPathDepth > 0 {
		options = append(options, entityrepos.WithPathDepth(config.kHopPathDepth))
	}
	if config.kHopStartFilter != nil {
		options = append(options, entityrepos.WithStartEntityFilter(config.kHopStartFilter))
	}
	if config.kHopRagQuery != "" {
		options = append(options, entityrepos.WithRagQuery(config.kHopRagQuery))
	}
	if config.kHopIsRuntimeBuild {
		options = append(options, entityrepos.WithRuntimeBuildOnly(config.kHopIsRuntimeBuild))
	}

	return options
}

func WithAIOptions(options ...aispec.AIConfigOption) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.aiOptions = options
	}
}

// Query configuration options for RAGSystemConfig

// WithRAGCtx sets the context for RAG query operations
func WithRAGCtx(ctx context.Context) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.ctx = ctx
	}
}

// WithRAGLimit sets the maximum number of results to return
func WithRAGLimit(limit int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.limit = limit
	}
}

// WithRAGCollectionLimit sets the maximum number of collections to search
func WithRAGCollectionLimit(collectionLimit int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.collectionLimit = collectionLimit
	}
}

// WithRAGEnhance sets the enhancement strategies to apply
func WithRAGEnhance(enhance ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enhance = enhance
	}
}

// WithRAGEnhanceSearchHandler sets the search handler for query enhancement
func WithRAGEnhanceSearchHandler(handler enhancesearch.SearchHandler) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enhanceSearchHandler = handler
	}
}

// WithRAGSystemLoadConfig sets the system load configuration functions
func WithRAGSystemLoadConfig(loadConfig ...vectorstore.CollectionConfigFunc) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.systemLoadConfig = loadConfig
	}
}

// WithRAGSimilarityThreshold sets the minimum similarity threshold for results
func WithRAGSimilarityThreshold(threshold float64) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.similarityThreshold = threshold
	}
}

// WithRAGMsgCallBack sets the callback function for query messages
func WithRAGMsgCallBack(msgCallBack func(*RAGSearchResult)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.msgCallback = msgCallBack
	}
}

// WithRAGLogReader sets the log reader function for query logging
func WithRAGLogReader(logReader func(reader io.Reader)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.logReader = logReader
	}
}

// WithEveryQueryResultCallback sets the callback function for each query result
func WithEveryQueryResultCallback(callback func(result *vectorstore.ScoredResult)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.everyQueryResultCallback = callback
	}
}

// WithRAGOnQueryFinish sets the callback function called when query finishes
func WithRAGOnQueryFinish(callback func([]*vectorstore.ScoredResult)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.onQueryFinish = callback
	}
}

// WithRAGConcurrent sets the concurrency level for query operations
func WithRAGConcurrent(concurrent int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.concurrent = concurrent
	}
}

// WithRAGOnlyResults sets whether to return only results without metadata
func WithRAGOnlyResults(onlyResults bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.onlyResults = onlyResults
	}
}

// WithRAGCollectionName sets the specific collection name to query
func WithRAGCollectionName(collectionName string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.name = collectionName
	}
}

// WithRAGCollectionNames sets multiple collection names to query
func WithRAGCollectionNames(collectionNames ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.collectionNames = collectionNames
	}
}

// WithRAGCollectionScoreLimit sets the score limit for collection filtering
func WithRAGCollectionScoreLimit(scoreLimit float64) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.collectionScoreLimit = scoreLimit
	}
}

// WithRAGQueryStatus sets the query status callback function
func WithRAGQueryStatus(callback func(label string, i any, tags ...string)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.queryStatusCallback = callback
	}
}

// WithRAGFilter sets the result filtering function
func WithRAGFilter(filter func(key string, getDoc func() *vectorstore.Document) bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.filterCallback = filter
	}
}

// WithRAGDocumentType sets the document type filter
func WithRAGDocumentType(documentType ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentTypes = documentType
	}
}

// Document and import/export configuration functions

// WithDocumentMetadataKeyValue sets document metadata key-value pairs
func WithDocumentMetadataKeyValue(key string, value any) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if config.documentMetadataKeyValue == nil {
			config.documentMetadataKeyValue = make(map[string]any)
		}
		config.documentMetadataKeyValue[key] = value
	}
}

// WithDocumentRawMetadata sets raw document metadata
func WithDocumentRawMetadata(metadata map[string]any) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentRawMetadata = metadata
	}
}

// WithDocumentType sets the document type
func WithDocumentType(docType string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentType = docType
	}
}

// WithDocumentEntityID sets the document entity ID
func WithDocumentEntityID(entityID string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentEntityID = entityID
	}
}

// WithDocumentRelatedEntities sets related entities for the document
func WithDocumentRelatedEntities(entities ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentRelatedEntities = entities
	}
}

// WithDocumentRuntimeID sets the document runtime ID
func WithDocumentRuntimeID(runtimeID string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentRuntimeID = runtimeID
	}
}

// WithModelDimension sets the model dimension
func WithModelDimension(dimension int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.modelDimension = dimension
	}
}

// WithModelName sets the model name
func WithModelName(name string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.modelName = name
	}
}

// WithCosineDistance sets whether to use cosine distance
func WithCosineDistance() RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.cosineDistance = true
	}
}

// WithHNSWParameters sets HNSW parameters
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.hnswM = m
		config.hnswMl = ml
		config.hnswEfSearch = efSearch
		config.hnswEfConstruct = efConstruct
	}
}

// WithForceNew sets whether to force creation of new collection
func WithForceNew(force bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.forceNew = force
	}
}

// WithLazyLoadEmbeddingClient sets whether to lazy load embedding client
func WithLazyLoadEmbeddingClient(lazy bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.lazyLoadEmbeddingClient = lazy
	}
}

// KHop configuration functions

// WithKHopK 设置k-hop的跳数，k>=2时返回k-hop路径，k=0返回所有路径
func WithKHopK(k int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if k < 0 {
			k = 0
		}
		config.kHopK = k
	}
}

// WithKHopKMin 设置最小路径长度，最小值为2
func WithKHopKMin(kMin int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if kMin < 2 {
			kMin = 2
		}
		config.kHopKMin = kMin
	}
}

// WithKHopKMax 设置最大路径长度，最小值为2
func WithKHopKMax(kMax int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopKMax = kMax
	}
}

func WithKHopLimit(k int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if k < 0 {
			k = 0
		}
		config.kHopLimit = k
	}
}

func WithKHopPathDepth(deep int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if deep < 1 { // 至少1层
			deep = 1
		}
		config.kHopPathDepth = deep
	}
}

func WithKHopStartFilter(filter *ypb.EntityFilter) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopStartFilter = filter
	}
}

func WithKHopRagQuery(query string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopRagQuery = query
	}
}

func WithKHopRuntimeBuildOnly(isRuntime bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopIsRuntimeBuild = isRuntime
	}
}
