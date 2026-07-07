package rag

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag/enhancesearch"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gorm.io/gorm"
)

type RAGSystemConfig struct {
	Name                                 string
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
	description            string
	tags                   []string
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
	logReaderWithInfo        func(reader io.Reader, info *vectorstore.SubQueryLogInfo, referenceMaterialCallback func(content string))
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
	rebuildHNSWIndex         bool
	documentHandler          func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	progressHandler          func(percent float64, message string, messageType string) // 进度回调
	noHNSWGraph              bool
	noMetadata               bool
	noOriginInput            bool
	noPotentialQuestions     bool // 不保存 potential_questions 到元数据，节约存储空间
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

	// VectorStore configuration fields
	maxChunkSize               int
	overlap                    int
	bigTextPlan                string
	buildGraphFilter           *yakit.VectorDocumentFilter
	buildGraphPolicy           string
	enablePQ                   bool
	enableAutoUpdateGraphInfos bool

	// EntityRepository configuration fields
	queryTop                   int
	runtimeID                  string
	disableBulkProcess         bool
	vectorStoreOptions         []vectorstore.CollectionConfigFunc
	disableEmbedCollectionInfo bool

	aiService aicommon.AICallbackType

	ragID string

	importFile string

	importKeyAsUID      bool
	serialVersionUID    string
	tryRebuildHNSWIndex bool

	enableDocumentQuestionIndex bool
}

// var defaultRAGSystemName = "default"
var defaultRAGSystemDescription = "default description"

func NewRAGSystemConfig(options ...RAGSystemConfigOption) *RAGSystemConfig {
	config := &RAGSystemConfig{
		ctx: context.Background(),
		db:  consts.GetGormProfileDatabase(),
		// Name:                       defaultRAGSystemName,
		description:                defaultRAGSystemDescription,
		knowledgeBaseType:          "default",
		enableEntityRepository:     true,
		enableKnowledgeBase:        true,
		maxChunkSize:               800,
		overlap:                    100,
		bigTextPlan:                "chunk_text",
		enableAutoUpdateGraphInfos: true,
		importKeyAsUID:             true,
		tryRebuildHNSWIndex:        false,
	}
	for _, option := range options {
		option(config)
	}
	return config
}

type RAGSystemConfigOption func(*RAGSystemConfig)

// noHNSWGraph 导出 RAG 时是否不导出 HNSW 索引（导出名为 rag.noHNSWGraph）
//
// 参数:
//   - noHNSWGraph: 为 true 时导出包不含 HNSW 索引（导入时可重建）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Export("my-rag", "/tmp/my.rag", rag.noHNSWGraph(true))~
// ```
func WithExportNoHNSWIndex(noHNSWGraph bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noHNSWGraph = noHNSWGraph
	}
}

// ragImportFile 指定导入所用的 RAG 文件路径（导出名为 rag.ragImportFile）
//
// 参数:
//   - importFile: RAG 文件路径
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Import("my-rag", rag.ragImportFile("/tmp/my.rag"))~
// ```
func WithImportFile(importFile string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.importFile = importFile
	}
}

// onlyPQCode 导出 RAG 时是否仅导出 PQ 编码（导出名为 rag.onlyPQCode）
//
// 参数:
//   - onlyPQCode: 为 true 时仅导出 PQ 编码以减小体积
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Export("my-rag", "/tmp/my.rag", rag.onlyPQCode(true))~
// ```
func WithExportOnlyPQCode(onlyPQCode bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.onlyPQCode = onlyPQCode
	}
}

// noMetadata 导出 RAG 时是否不导出元数据（导出名为 rag.noMetadata）
//
// 参数:
//   - noMetadata: 为 true 时导出包不含文档元数据
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Export("my-rag", "/tmp/my.rag", rag.noMetadata(true))~
// ```
func WithExportNoMetadata(noMetadata bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noMetadata = noMetadata
	}
}

// importOverwrite 导入 RAG 时是否覆盖已存在的同名集合（导出名为 rag.importOverwrite）
//
// 参数:
//   - overwriteExisting: 为 true 时覆盖已存在的集合
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Import("my-rag", rag.ragImportFile("/tmp/my.rag"), rag.importOverwrite(true))~
// ```
func WithExportOverwriteExisting(overwriteExisting bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.overwriteExisting = overwriteExisting
	}
}

// noOriginInput 导出 RAG 时是否不导出原始输入内容（导出名为 rag.noOriginInput）
//
// 参数:
//   - noOriginInput: 为 true 时导出包不含原始输入文本
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Export("my-rag", "/tmp/my.rag", rag.noOriginInput(true))~
// ```
func WithExportNoOriginInput(noOriginInput bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noOriginInput = noOriginInput
	}
}

// importRebuildGraph 导入 RAG 时是否重建 HNSW 索引（导出名为 rag.importRebuildGraph）
//
// 参数:
//   - rebuildHNSWIndex: 为 true 时在导入后重建 HNSW 索引
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Import("my-rag", rag.ragImportFile("/tmp/my.rag"), rag.importRebuildGraph(true))~
// ```
func WithImportRebuildHNSWIndex(rebuildHNSWIndex bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.rebuildHNSWIndex = rebuildHNSWIndex
	}
}

// aiServiceType 按名称与配置指定 RAG 使用的 AI 服务（导出名为 rag.aiServiceType）
//
// 参数:
//   - aiServiceName: AI 服务名称（如 openai、ollama 等）
//   - aiServiceConfig: 可选的 AI 配置项（如 ai.apiKey、ai.model 等）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.aiServiceType("openai", ai.model("gpt-4o-mini")))~
// ```
func WithAIServiceType(aiServiceName string, aiServiceConfig ...aispec.AIConfigOption) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		chatter, err := ai.LoadChater(aiServiceName, aiServiceConfig...)
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
			return
		}
		config.aiService = aicommon.AIChatToAICallbackType(chatter)
	}
}

func (config *RAGSystemConfig) GetAIService() aicommon.AICallbackType {
	return config.aiService
}

// aiService 直接指定 RAG 使用的 AI 回调服务（导出名为 rag.aiService）
//
// 参数:
//   - aiService: AI 回调函数，用于实体抽取、问题生成等增强能力
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// cb = func(config, msg) { return ai.Chat(msg.GetPrompt())~ }
// db = rag.Get("my-rag", rag.aiService(cb))~
// ```
func WithAIService(aiService aicommon.AICallbackType) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.aiService = aiService
	}
}

func WithRAGID(ragID string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.ragID = ragID
	}
}

// documentHandler 导出 RAG 时对每个文档进行处理的回调（导出名为 rag.documentHandler）
//
// 参数:
//   - documentHandler: 处理函数，接收一个文档并返回处理后的文档与错误
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// handler = func(doc) { return doc, nil }
// rag.Export("my-rag", "/tmp/my.rag", rag.documentHandler(handler))~
// ```
func WithExportDocumentHandler(documentHandler func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentHandler = documentHandler
	}
}

// progressHandler 导出 RAG 时的进度回调（导出名为 rag.progressHandler）
//
// 参数:
//   - progressHandler: 进度回调，接收百分比、消息文本与消息类型
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// onProgress = func(percent, message, messageType) { println(percent, message) }
// rag.Export("my-rag", "/tmp/my.rag", rag.progressHandler(onProgress))~
// ```
func WithExportOnProgressHandler(progressHandler func(percent float64, message string, messageType string)) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.progressHandler = progressHandler
	}
}

func WithProgressHandler(progressHandler func(percent float64, message string, messageType string)) RAGSystemConfigOption {
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

// ragDescription 设置 RAG 集合的描述信息（导出名为 rag.ragDescription）
//
// 参数:
//   - description: 集合描述文本
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragDescription("安全知识库"))~
// ```
func WithDescription(description string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.description = description
	}
}

func WithTags(tags ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.tags = tags
	}
}

func WithVectorStore(store *vectorstore.SQLiteVectorStoreHNSW) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.vectorStore = store
	}
}

// ragEmbeddingModel 设置 RAG 使用的 embedding 模型名称（导出名为 rag.ragEmbeddingModel）
//
// 参数:
//   - model: embedding 模型名称（如 text-embedding-3-small）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragEmbeddingModel("text-embedding-3-small"))~
// ```
func WithEmbeddingModel(model string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.embeddingModel = model
	}
}

// db 指定 RAG 使用的数据库连接（导出名为 rag.db）
//
// 参数:
//   - db: 数据库连接对象
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.db(myDatabaseConn))~
// ```
func WithDB(db *gorm.DB) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.db = db
	}
}

func WithMaxChunkSize(maxChunkSize int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.maxChunkSize = maxChunkSize
	}
}

func WithOverlap(overlap int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.overlap = overlap
	}
}

func WithBigTextPlan(bigTextPlan string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.bigTextPlan = bigTextPlan
	}
}

func WithBuildGraphFilter(filter *yakit.VectorDocumentFilter) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.buildGraphFilter = filter
	}
}

func WithBuildGraphPolicy(policy string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.buildGraphPolicy = policy
	}
}

func WithEnablePQ(enable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enablePQ = enable
	}
}

func WithEnableAutoUpdateGraphInfos(enable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enableAutoUpdateGraphInfos = enable
	}
}

func WithQueryTop(queryTop int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.queryTop = queryTop
	}
}

func WithRuntimeID(runtimeID string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.runtimeID = runtimeID
	}
}

func WithDisableBulkProcess(disableBulkProcess bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.disableBulkProcess = disableBulkProcess
	}
}

func WithVectorStoreOptions(vectorStoreOptions ...vectorstore.CollectionConfigFunc) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.vectorStoreOptions = vectorStoreOptions
	}
}

func WithName(name string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.Name = name
	}
}

// enableQuestionIndex 是否启用文档潜在问题索引（导出名为 rag.enableQuestionIndex）
//
// 开启后会为文档生成潜在问题并建立索引，提升问答类查询的召回效果。
//
// 参数:
//   - enable: 为 true 时启用文档问题索引
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.enableQuestionIndex(true))~
// ```
func WithEnableDocumentQuestionIndex(enable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.enableDocumentQuestionIndex = enable
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
	if config.serialVersionUID != "" {
		options = append(options, vectorstore.WithSerialVersionUID(config.serialVersionUID))
	}
	if config.Name != "" {
		options = append(options, vectorstore.WithCollectionName(config.Name))
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

	// Embedding configuration
	if config.embeddingClient != nil {
		options = append(options, vectorstore.WithEmbeddingClient(config.embeddingClient))
	}
	if config.lazyLoadEmbeddingClient {
		options = append(options, vectorstore.WithLazyLoadEmbeddingClient())
	}

	// Basic configuration
	if config.description != "" {
		options = append(options, vectorstore.WithDescription(config.description))
	}
	if config.forceNew {
		options = append(options, vectorstore.WithForceNew(config.forceNew))
	}

	// Model configuration
	if config.modelDimension > 0 {
		options = append(options, vectorstore.WithModelDimension(config.modelDimension))
	}
	if config.modelName != "" {
		options = append(options, vectorstore.WithModelName(config.modelName))
	}

	// Distance function
	if config.cosineDistance {
		options = append(options, vectorstore.WithCosineDistance())
	}

	// HNSW parameters
	if config.hnswM > 0 || config.hnswMl > 0 || config.hnswEfSearch > 0 || config.hnswEfConstruct > 0 {
		// Use individual hnsw fields if set, otherwise use defaults
		m := config.hnswM
		if m <= 0 {
			m = 16 // default value
		}
		ml := config.hnswMl
		if ml <= 0 {
			ml = 0.25 // default value
		}
		efSearch := config.hnswEfSearch
		if efSearch <= 0 {
			efSearch = 20 // default value
		}
		efConstruct := config.hnswEfConstruct
		if efConstruct <= 0 {
			efConstruct = 200 // default value
		}
		options = append(options, vectorstore.WithHNSWParameters(m, ml, efSearch, efConstruct))
	}

	// Database
	if config.db != nil {
		options = append(options, vectorstore.WithDB(config.db))
	}

	// Chunk configuration
	if config.maxChunkSize > 0 {
		options = append(options, vectorstore.WithMaxChunkSize(config.maxChunkSize))
	}
	if config.overlap >= 0 {
		options = append(options, vectorstore.WithOverlap(config.overlap))
	}
	if config.bigTextPlan != "" {
		options = append(options, vectorstore.WithBigTextPlan(config.bigTextPlan))
	}

	// Graph building configuration
	if config.buildGraphFilter != nil {
		options = append(options, vectorstore.WithBuildGraphFilter(config.buildGraphFilter))
	}
	if config.buildGraphPolicy != "" {
		options = append(options, vectorstore.WithBuildGraphPolicy(config.buildGraphPolicy))
	}

	// PQ and auto-update configuration
	options = append(options, vectorstore.WithEnablePQ(config.enablePQ))
	options = append(options, vectorstore.WithEnableAutoUpdateGraphInfos(config.enableAutoUpdateGraphInfos))
	options = append(options, vectorstore.WithDisableEmbedCollectionInfo(config.disableEmbedCollectionInfo))
	options = append(options, vectorstore.WithKeyAsUID(config.importKeyAsUID))
	options = append(options, vectorstore.WithTryRebuildHNSWIndex(config.tryRebuildHNSWIndex))
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

// GetDocumentMetadata returns the merged document metadata from both key-value and raw metadata
func (config *RAGSystemConfig) GetDocumentMetadata() map[string]any {
	result := make(map[string]any)

	// First, add raw metadata
	for k, v := range config.documentRawMetadata {
		result[k] = v
	}

	// Then, add key-value metadata (overrides raw if same key)
	for k, v := range config.documentMetadataKeyValue {
		result[k] = v
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// GetNoPotentialQuestions returns whether to exclude potential_questions from metadata
func (config *RAGSystemConfig) GetNoPotentialQuestions() bool {
	return config.noPotentialQuestions
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
	if config.similarityThreshold > 0 {
		options = append(options, vectorstore.WithRAGSimilarityThreshold(config.similarityThreshold))
	}
	if config.msgCallback != nil {
		options = append(options, vectorstore.WithRAGMsgCallBack(config.msgCallback))
	}
	if config.logReaderWithInfo != nil {
		options = append(options, vectorstore.WithRAGLogReaderWithInfo(config.logReaderWithInfo))
	} else if config.logReader != nil {
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
	if config.Name != "" {
		options = append(options, vectorstore.WithRAGCollectionName(config.Name))
	}
	if len(config.documentTypes) > 0 {
		options = append(options, vectorstore.WithRAGDocumentType(config.documentTypes...))
	}

	vectorStoreOptions := config.ConvertToVectorStoreOptions()
	options = append(options, vectorstore.WithRAGSystemLoadConfig(vectorStoreOptions...))
	return options
}

func (config *RAGSystemConfig) ConvertToEntityRepositoryOptions() []entityrepos.RuntimeConfigOption {
	options := []entityrepos.RuntimeConfigOption{}

	if config.similarityThreshold > 0 {
		options = append(options, entityrepos.WithSimilarityThreshold(config.similarityThreshold))
	}
	if config.queryTop > 0 {
		options = append(options, entityrepos.WithQueryTop(config.queryTop))
	}
	if config.runtimeID != "" {
		options = append(options, entityrepos.WithRuntimeID(config.runtimeID))
	}
	if config.disableBulkProcess {
		options = append(options, entityrepos.WithDisableBulkProcess())
	}
	if config.ctx != nil {
		options = append(options, entityrepos.WithContext(config.ctx))
	}
	if len(config.vectorStoreOptions) > 0 {
		options = append(options, entityrepos.WithVectorStoreOptions(config.vectorStoreOptions...))
	}

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

// queryCtx 设置 RAG 查询操作的上下文（导出名为 rag.queryCtx），可用于超时/取消控制
//
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// ctx = context.WithTimeout(context.Background(), 10*time.Second)
// results = rag.Query("my-rag", "关键词", rag.queryCtx(ctx))~
// ```
func WithRAGCtx(ctx context.Context) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.ctx = ctx
	}
}

// queryLimit 设置查询返回结果的最大数量（导出名为 rag.queryLimit）
//
// 参数:
//   - limit: 返回结果数量上限
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryLimit(5))~
// ```
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

// queryEnhance 设置查询增强策略（导出名为 rag.queryEnhance），用于扩展或改写查询以提升召回
//
// 参数:
//   - enhance: 一个或多个增强策略名称
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryEnhance("hyde"))~
// ```
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

// querySimilarityThreshold 设置结果的最小相似度阈值（导出名为 rag.querySimilarityThreshold）
//
// 参数:
//   - threshold: 相似度阈值（0~1），低于该值的结果将被过滤
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.querySimilarityThreshold(0.7))~
// ```
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

// WithRAGLogReaderWithInfo sets the log reader callback with additional sub-query info.
// The callback receives:
// - reader: the log reader for the current sub-query
// - info: information about the current sub-query including method, query, and results
// - referenceMaterialCallback: call this with the reference material content after the reader is consumed
func WithRAGLogReaderWithInfo(f func(reader io.Reader, info *vectorstore.SubQueryLogInfo, referenceMaterialCallback func(content string))) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.logReaderWithInfo = f
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

// queryConcurrent 设置查询操作的并发数（导出名为 rag.queryConcurrent）
//
// 参数:
//   - concurrent: 并发数（同时检索的子查询数量）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryConcurrent(4))~
// ```
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

// queryCollection 指定要查询的集合名称（导出名为 rag.queryCollection，导入时别名 rag.importName）
//
// 参数:
//   - collectionName: 集合名称
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryCollection("my-collection"))~
// ```
func WithRAGCollectionName(collectionName string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.Name = collectionName
	}
}

// WithRAGCollectionNames sets multiple collection names to query
func WithRAGCollectionNames(collectionNames ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.collectionNames = collectionNames
	}
}

// queryScoreLimit 设置集合过滤的分数阈值（导出名为 rag.queryScoreLimit）
//
// 参数:
//   - scoreLimit: 集合分数阈值，低于该值的集合将被跳过
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryScoreLimit(0.5))~
// ```
func WithRAGCollectionScoreLimit(scoreLimit float64) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.collectionScoreLimit = scoreLimit
	}
}

// queryStatus 设置查询状态回调函数（导出名为 rag.queryStatus），用于接收查询过程中的状态信息
//
// 参数:
//   - callback: 状态回调，接收标签、任意数据与可选标签列表
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// onStatus = func(label, data, tags...) { println(label) }
// results = rag.Query("my-rag", "关键词", rag.queryStatus(onStatus))~
// ```
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

// queryType 设置文档类型过滤（导出名为 rag.queryType），仅查询指定类型的文档
//
// 参数:
//   - documentType: 一个或多个文档类型
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// results = rag.Query("my-rag", "关键词", rag.queryType("knowledge"))~
// ```
func WithRAGDocumentType(documentType ...string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentTypes = documentType
	}
}

// Document and import/export configuration functions

// docMetadata 为文档添加一个元数据键值对（导出名为 rag.docMetadata），可多次调用累加
//
// 参数:
//   - key: 元数据键
//   - value: 元数据值（任意类型）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db.Add("doc-id", "content", rag.docMetadata("source", "manual"))~
// ```
func WithDocumentMetadataKeyValue(key string, value any) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if config.documentMetadataKeyValue == nil {
			config.documentMetadataKeyValue = make(map[string]any)
		}
		config.documentMetadataKeyValue[key] = value
	}
}

// docRawMetadata 直接设置文档的原始元数据 map（导出名为 rag.docRawMetadata）
//
// 参数:
//   - metadata: 元数据键值映射
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db.Add("doc-id", "content", rag.docRawMetadata({"source": "manual", "lang": "zh"}))~
// ```
func WithDocumentRawMetadata(metadata map[string]any) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.documentRawMetadata = metadata
	}
}

// WithNoPotentialQuestions sets whether to exclude potential_questions from metadata to save storage
func WithNoPotentialQuestions(noPQ bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.noPotentialQuestions = noPQ
	}
}

// noPotentialQuestions 返回一个不在元数据中保存潜在问题的配置选项（导出名为 rag.noPotentialQuestions）
//
// 等价于 WithNoPotentialQuestions(true)，可减少存储开销。
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db.Add("doc-id", "content", rag.noPotentialQuestions())~
// ```
func NoPotentialQuestions() RAGSystemConfigOption {
	return WithNoPotentialQuestions(true)
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

// ragModelDimension 设置 embedding 模型的向量维度（导出名为 rag.ragModelDimension）
//
// 参数:
//   - dimension: 向量维度（需与所用 embedding 模型一致）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragModelDimension(1536))~
// ```
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

// ragCosineDistance 使用余弦距离作为向量相似度度量（导出名为 rag.ragCosineDistance）
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragCosineDistance())~
// ```
func WithCosineDistance() RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.cosineDistance = true
	}
}

// ragHNSWParameters 设置 HNSW 索引参数（导出名为 rag.ragHNSWParameters）
//
// 参数:
//   - m: 每个节点的最大连接数
//   - ml: 层级生成因子
//   - efSearch: 查询时的候选集大小（影响召回与速度）
//   - efConstruct: 构建索引时的候选集大小
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragHNSWParameters(16, 0.25, 64, 200))~
// ```
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.hnswM = m
		config.hnswMl = ml
		config.hnswEfSearch = efSearch
		config.hnswEfConstruct = efConstruct
	}
}

// ragForceNew 是否强制创建新集合（导出名为 rag.ragForceNew），为 true 时会覆盖同名集合
//
// 参数:
//   - force: 为 true 时强制新建集合
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// db = rag.Get("my-rag", rag.ragForceNew(true))~
// ```
func WithForceNew(force bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.forceNew = force
	}
}

// WithEmbedCollectionInfo sets whether to embed collection info
func WithDisableEmbedCollectionInfo(disable bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.disableEmbedCollectionInfo = disable
	}
}

// WithLazyLoadEmbeddingClient sets whether to lazy load embedding client
func WithLazyLoadEmbeddingClient(lazy bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.lazyLoadEmbeddingClient = lazy
	}
}

// KHop configuration functions

// khopk 设置 k-hop 的跳数（导出名为 rag.khopk），k>=2 时返回 k-hop 路径，k=0 返回所有路径
//
// 参数:
//   - k: 跳数，负数会被归一为 0
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.khopk(2))~
// ```
func WithKHopK(k int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if k < 0 {
			k = 0
		}
		config.kHopK = k
	}
}

// khopkMin 设置最小路径长度（导出名为 rag.khopkMin），最小值为 2
//
// 参数:
//   - kMin: 最小路径长度，小于 2 时会被归一为 2
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.khopkMin(2), rag.khopkMax(4))~
// ```
func WithKHopKMin(kMin int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if kMin < 2 {
			kMin = 2
		}
		config.kHopKMin = kMin
	}
}

// khopkMax 设置最大路径长度（导出名为 rag.khopkMax）
//
// 参数:
//   - kMax: 最大路径长度
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.khopkMin(2), rag.khopkMax(4))~
// ```
func WithKHopKMax(kMax int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopKMax = kMax
	}
}

// khopLimit 设置 k-hop 查询返回的路径数量上限（导出名为 rag.khopLimit）
//
// 参数:
//   - k: 路径数量上限，负数会被归一为 0
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.khopLimit(10))~
// ```
func WithKHopLimit(k int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if k < 0 {
			k = 0
		}
		config.kHopLimit = k
	}
}

// pathDepth 设置 k-hop 查询的路径深度（导出名为 rag.pathDepth），至少为 1
//
// 参数:
//   - deep: 路径深度，小于 1 时会被归一为 1
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.pathDepth(3))~
// ```
func WithKHopPathDepth(deep int) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		if deep < 1 { // 至少1层
			deep = 1
		}
		config.kHopPathDepth = deep
	}
}

// buildFilter 设置 k-hop 查询的起始实体过滤条件（导出名为 rag.buildFilter）
//
// 参数:
//   - filter: 实体过滤器，用于确定路径搜索的起点实体
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// filter = rag.getEntityFilter("name", "用户登录")
// paths = rag.KHopQuery("my-rag", rag.buildFilter(filter))~
// ```
func WithKHopStartFilter(filter *ypb.EntityFilter) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.kHopStartFilter = filter
	}
}

// buildQuery 设置 k-hop 查询所用的 RAG 检索语句（导出名为 rag.buildQuery），用于定位起始实体
//
// 参数:
//   - query: 检索关键词或语句
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// paths = rag.KHopQuery("my-rag", rag.buildQuery("用户登录流程"))~
// ```
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

func WithTryRebuildHNSWIndex(tryRebuildHNSWIndex bool) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.tryRebuildHNSWIndex = tryRebuildHNSWIndex
	}
}

// serialVersionUID 设置 RAG 序列化版本标识（导出名为 rag.serialVersionUID），用于导入导出时的兼容性校验
//
// 参数:
//   - serialVersionUID: 序列化版本号字符串
//
// 返回值:
//   - RAG 系统配置选项
//
// Example:
// ```
// rag.Export("my-rag", "/tmp/my.rag", rag.serialVersionUID("v1"))~
// ```
func WithRAGSerialVersionUID(serialVersionUID string) RAGSystemConfigOption {
	return func(config *RAGSystemConfig) {
		config.serialVersionUID = serialVersionUID
	}
}
