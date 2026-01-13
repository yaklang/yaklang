package yak

import (
	"path/filepath"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
)

// 导出的公共函数
var RagExports = map[string]interface{}{
	"Get":           rag.Get,
	"GetCollection": rag.Get,
	"lazyEmbedding": _lazyEmbedding,

	"embeddingHandle": _embeddingHandle,

	"DeleteCollection":       _deleteCollection,
	"DeleteRAG":              _deleteRAG,
	"DeleteKnowledgeBase":    _deleteKnowledgeBase,
	"DeleteAllKnowledgeBase": _deleteAllKnowledgeBase,
	"ListCollection":         _listCollection,
	"ListRAG":                _listRAG,
	"GetCollectionInfo":      _getCollectionInfo,

	"HasCollection": _hasCollection,

	// Query - 统一查询/搜索接口
	"Query":                    _query,
	"queryLimit":               rag.WithRAGLimit,
	"queryType":                rag.WithRAGDocumentType,
	"queryCollection":          rag.WithRAGCollectionName, // 单集合（兼容旧版）
	"queryCollections":         _queryCollections,         // 多集合
	"queryRAGFilename":         _queryRAGFilename,         // 从 RAG 文件导入后搜索
	"queryStatus":              rag.WithRAGQueryStatus,
	"queryEnhance":             rag.WithRAGEnhance,
	"queryCtx":                 rag.WithRAGCtx,
	"queryConcurrent":          rag.WithRAGConcurrent,
	"queryScoreLimit":          rag.WithRAGCollectionScoreLimit,
	"querySimilarityThreshold": rag.WithRAGSimilarityThreshold, // 语义相似度阈值

	// Query enhance type constants - 查询增强类型常量
	"QUERY_ENHANCE_TYPE_BASIC":                vectorstore.BasicPlan,
	"QUERY_ENHANCE_TYPE_HYPOTHETICAL_ANSWER":  vectorstore.EnhancePlanHypotheticalAnswer,
	"QUERY_ENHANCE_TYPE_SPLIT_QUERY":          vectorstore.EnhancePlanSplitQuery,
	"QUERY_ENHANCE_TYPE_GENERALIZE_QUERY":     vectorstore.EnhancePlanGeneralizeQuery,
	"QUERY_ENHANCE_TYPE_EXACT_KEYWORD_SEARCH": vectorstore.EnhancePlanExactKeywordSearch,

	"AddDocument":    _addDocument,
	"DeleteDocument": _deleteDocument,
	"QueryDocuments": _queryDocuments,

	"ragForceNew":         rag.WithForceNew,
	"ragDescription":      rag.WithDescription,
	"ragEmbeddingModel":   rag.WithEmbeddingModel,
	"ragModelDimension":   rag.WithModelDimension,
	"ragCosineDistance":   rag.WithCosineDistance,
	"ragHNSWParameters":   rag.WithHNSWParameters,
	"enableQuestionIndex": rag.WithEnableDocumentQuestionIndex,

	"docMetadata":          rag.WithDocumentMetadataKeyValue,
	"docRawMetadata":       rag.WithDocumentRawMetadata,
	"setSearchMeta":        _setSearchMeta,           // 快捷设置 search_type 和 search_target
	"noPotentialQuestions": rag.NoPotentialQuestions, // 不保存 potential_questions 到元数据
	"NewRagDatabase":       rag.NewVectorStoreDatabase,
	"NewTempRagDatabase":   _newTempRagDatabase,
	"EnableMockMode":       _enableMockMode,

	"ctx":             aiforge.WithAnalyzeContext,    // use for analyzeContext
	"log":             aiforge.WithAnalyzeLog,        // use for analyzeLog
	"statusCard":      aiforge.WithAnalyzeStatusCard, // use for analyzeStatusCard
	"extraPrompt":     aiforge.WithExtraPrompt,       // use for analyzeImage and analyzeImageFile
	"entryLength":     aiforge.RefineWithKnowledgeEntryLength,
	"disableIndex":    aiforge.RefineWithDisableBuildIndex, // disable building index knowledge
	"disableERM":      aiforge.RefineWithDisableERMBuild,   // disable building entity repository model
	"chunkSize":       chunkmaker.WithChunkSize,
	"khopk":           rag.WithKHopK,
	"khopLimit":       rag.WithKHopLimit,
	"khopkMin":        rag.WithKHopKMin,
	"khopkMax":        rag.WithKHopKMax,
	"buildQuery":      rag.WithKHopRagQuery,
	"buildFilter":     rag.WithKHopStartFilter,
	"pathDepth":       rag.WithKHopPathDepth,
	"getEntityFilter": schema.SimpleBuildEntityFilter,

	"BuildCollectionFromFile":   aiforge.BuildKnowledgeFromFile,
	"BuildCollectionFromReader": aiforge.BuildKnowledgeFromReader,
	"BuildCollectionFromRaw":    aiforge.BuildKnowledgeFromBytes,

	"BuildKnowledgeFromEntityRepos": aiforge.BuildKnowledgeFromEntityReposByName,

	"BuildIndexKnowledgeFromFile": BuildIndexKnowledgeFromFile,

	// Search index building functions - for building search indexes for tools/content
	"BuildSearchIndexKnowledge":         BuildSearchIndexKnowledge,
	"BuildSearchIndexKnowledgeFromFile": BuildSearchIndexKnowledgeFromFile,

	"Import":             rag.ImportRAG,
	"db":                 rag.WithDB,
	"importOverwrite":    rag.WithExportOverwriteExisting,
	"importName":         rag.WithRAGCollectionName,
	"importRebuildGraph": rag.WithImportRebuildHNSWIndex,
	"serialVersionUID":   rag.WithRAGSerialVersionUID,
	"documentHandler":    rag.WithExportDocumentHandler,
	"progressHandler":    rag.WithExportOnProgressHandler,
	"aiServiceType":      rag.WithAIServiceType,
	"aiService":          rag.WithAIService,

	"Export":             rag.ExportRAG,
	"noHNSWGraph":        rag.WithExportNoHNSWIndex,
	"noMetadata":         rag.WithExportNoMetadata,
	"noOriginInput":      rag.WithExportNoOriginInput,
	"onlyPQCode":         rag.WithExportOnlyPQCode,
	"noEntityRepository": _noEntityRepository,
	"noKnowledgeBase":    _noKnowledgeBase,
	"ragImportFile":      rag.WithImportFile,

	"Embedding":       _embedding,
	"LocalEmbedding":  _localEmbedding,
	"OnlineEmbedding": _onlineEmbedding,

	// DBQuery - 数据库直接查询接口（快速，不使用语义搜索）
	// 用于去重检查、快速验证等场景
	"DBQueryKnowledge":             _dbQueryKnowledge,             // 查询知识库条目
	"DBQueryUniqueKnowledgeTitles": _dbQueryUniqueKnowledgeTitles, // 获取唯一的知识标题列表（高效去重）
	"DBQueryEntity":                _dbQueryEntity,                // 查询实体
	"DBQueryVectorDocument":        _dbQueryVectorDocument,        // 查询向量文档
	"DBQueryKnowledgeExists":       _dbQueryKnowledgeExists,       // 检查知识条目是否存在且有向量索引
	"DBQueryCountVectorsByEntry":   _dbQueryCountVectorsByEntryID, // 根据 entry_id 计算向量数量

	// DBQuery 选项
	"dbQueryCollection":  _dbQueryCollection,  // 指定集合（单个）
	"dbQueryCollections": _dbQueryCollections, // 指定多个集合
	"dbQueryLimit":       _dbQueryLimit,       // 限制数量
	"dbQueryOffset":      _dbQueryOffset,      // 偏移量
	"dbQueryRAGFilename": _dbQueryRAGFilename, // 从 RAG 文件导入后查询
	"dbQueryDB":          _dbQueryDB,          // 指定数据库连接
	"dbQueryCtx":         _dbQueryCtx,         // 设置上下文
}

func BuildIndexKnowledgeFromFile(kbName string, path string, option ...any) error {
	entries, err := aiforge.BuildIndexKnowledgeFromFile(kbName, path, option...)
	if err != nil {
		return err
	}
	for entry := range entries {
		log.Infof("indexed knowledge entry: %s", entry.KnowledgeTitle)
	}
	return nil
}

func _lazyEmbedding(lazy ...bool) rag.RAGSystemConfigOption {
	if len(lazy) > 0 {
		return rag.WithLazyLoadEmbeddingClient(lazy[0])
	}
	return rag.WithLazyLoadEmbeddingClient(true)
}

// BuildSearchIndexKnowledge builds a search index for the given text content.
// It generates 5-10 search questions that users might ask to find this content,
// and stores the original content as the knowledge entry.
//
// Parameters:
//   - kbName: the knowledge base name
//   - text: the content to index (e.g., tool description, usage, parameters)
//   - options: optional configuration (rag options, AI options, etc.)
//
// The function will:
// 1. Use AI to generate 5-10 search questions based on the text
// 2. Store the original text as the knowledge entry
// 3. Set docMetadata with question_index and search_target for each question
//
// Example:
// ```yak
// text = `
// 工具名：端口扫描器
// 目标：扫描目标主机的开放端口
// 用法：指定目标IP和端口范围，工具会返回开放的端口列表
// `
// result = rag.BuildSearchIndexKnowledge("my-tools", text)~
// println("Generated questions:", result.Questions)
// ```
func BuildSearchIndexKnowledge(kbName string, text string, option ...any) (*aiforge.SearchIndexResult, error) {
	result, err := aiforge.BuildSearchIndexKnowledge(kbName, text, option...)
	if err != nil {
		return nil, err
	}
	log.Infof("built search index with %d questions for entry %s", len(result.Questions), result.EntryID)
	for i, q := range result.Questions {
		log.Infof("  Q%d: %s", i+1, q)
	}
	return result, nil
}

// BuildSearchIndexKnowledgeFromFile builds a search index from a file.
// It reads the file content and calls BuildSearchIndexKnowledge.
//
// Parameters:
//   - kbName: the knowledge base name
//   - filename: the path to the file containing the content to index
//   - options: optional configuration (rag options, AI options, etc.)
//
// Example:
// ```yak
// result = rag.BuildSearchIndexKnowledgeFromFile("my-tools", "/path/to/tool-description.txt")~
// println("Generated questions:", result.Questions)
// ```
func BuildSearchIndexKnowledgeFromFile(kbName string, filename string, option ...any) (*aiforge.SearchIndexResult, error) {
	result, err := aiforge.BuildSearchIndexKnowledgeFromFile(kbName, filename, option...)
	if err != nil {
		return nil, err
	}
	log.Infof("built search index from file %s with %d questions", filename, len(result.Questions))
	for i, q := range result.Questions {
		log.Infof("  Q%d: %s", i+1, q)
	}
	return result, nil
}

// _setSearchMeta 快捷设置搜索元数据 (search_type 和 search_target)
// 用于同时设置 search_type 和 search_target 两个元数据字段
//
// Parameters:
//   - searchType: 搜索类型，例如 "AI工具", "Yak插件", "aiforge/模版/技能" 等
//   - searchTarget: 搜索目标，例如插件名称、工具名称等
//
// Example:
// ```yak
// rag.BuildSearchIndexKnowledge("my-tools", text, rag.setSearchMeta("AI工具", "端口扫描器"))
// ```
func _setSearchMeta(searchType string, searchTarget string) rag.RAGSystemConfigOption {
	return func(config *rag.RAGSystemConfig) {
		rag.WithDocumentMetadataKeyValue("search_type", searchType)(config)
		rag.WithDocumentMetadataKeyValue("search_target", searchTarget)(config)
	}
}

// _noEntityRepository 禁用实体仓库
// Example:
// ```
//
//	rag.noEntityRepository()
//
// ```
func _noEntityRepository() rag.RAGSystemConfigOption {
	return rag.WithEnableEntityRepository(false)
}

// _noKnowledgeBase 禁用知识库
// Example:
// ```
//
//	rag.noKnowledgeBase()
//
// ```
func _noKnowledgeBase() rag.RAGSystemConfigOption {
	return rag.WithEnableKnowledgeBase(false)
}

// _deleteCollection 删除指定的 RAG 集合
// Example:
// ```
//
//	err = rag.DeleteCollection("my_collection")
//
// ```
func _deleteCollection(name string) error {
	return rag.DeleteCollection(consts.GetGormProfileDatabase(), name)
}

// _listRAG 列出所有 RAG 系统列表
// Example:
// ```
//
//	ragSystems = rag.ListRAG()
//
// ```
func _listRAG() []string {
	return rag.ListRAGSystemNames(consts.GetGormProfileDatabase())
}

// _deleteRAG 删除指定的 RAG 系统
// Example:
// ```
//
//	err = rag.DeleteRAG("my_rag")
//
// ```
func _deleteRAG(name string) error {
	log.Infof("start to delete RAG system: %s", name)
	return rag.DeleteRAG(consts.GetGormProfileDatabase(), name)
}

// _deleteKnowledgeBase 删除指定的知识库及其关联的 RAG 内容
// 包括: RAG 向量库、RAG 集合综述库、知识库条目库、知识库集合、问题索引库等
// Example:
// ```
//
//	err = rag.DeleteKnowledgeBase("my_knowledge_base")
//
// ```
func _deleteKnowledgeBase(name string) error {
	return rag.DeleteRAG(consts.GetGormProfileDatabase(), name)
}

// _deleteAllKnowledgeBase 删除所有知识库及其关联的 RAG 内容
// 清空所有: RAG 向量库、RAG 集合综述库、知识库条目库、知识库集合、问题索引库等
// Example:
// ```
//
//	err = rag.DeleteAllKnowledgeBase()
//
// ```
func _deleteAllKnowledgeBase() error {
	return rag.DeleteAllRAG(consts.GetGormProfileDatabase())
}

// _embeddingHandle 创建自定义嵌入处理器
// Example:
// ```
//
//	embeddingOpt = rag.embeddingHandle((text) => {
//		return [0.1, 0.2, 0.3] // 返回嵌入向量
//	})
//
// ```
func _embeddingHandle(handle func(text string) any) rag.RAGSystemConfigOption {
	embedder := vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		ires := handle(text)
		resSlice, err := utils.InterfaceToSliceInterfaceE(ires)
		if err != nil {
			return nil, err
		}
		float32Slice := lo.Map(resSlice, func(i any, _ int) float32 {
			return float32(utils.InterfaceToFloat64(i))
		})
		return float32Slice, nil
	})
	return rag.WithEmbeddingClient(embedder)
}

// _listCollection 获取所有 RAG 集合列表
// Example:
// ```
//
//	collections = rag.ListCollection()
//
// ```
func _listCollection() []string {
	return rag.ListCollections(consts.GetGormProfileDatabase())
}

// _getCollectionInfo 获取指定集合的详细信息
// Example:
// ```
//
//	info, err = rag.GetCollectionInfo("my_collection")
//
// ```
func _getCollectionInfo(name string) (*vectorstore.CollectionInfo, error) {
	return vectorstore.GetCollectionInfo(consts.GetGormProfileDatabase(), name)
}

// _hasCollection 检查指定集合是否存在
// Example:
// ```
//
//	exists = rag.HasCollection("my_collection")
//
// ```
func _hasCollection(name string) bool {
	return rag.CollectionIsExists(consts.GetGormProfileDatabase(), name)
}

// _addDocument 向指定集合添加文档
// Example:
// ```
//
//	err = rag.AddDocument("my_collection", "doc1", "content", {"key": "value"})
//
// ```
func _addDocument(knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...rag.RAGSystemConfigOption) error {
	return rag.AddDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, document, metadata, opts...)
}

// _deleteDocument 从指定集合删除文档
// Example:
// ```
//
//	err = rag.DeleteDocument("my_collection", "doc1")
//
// ```
func _deleteDocument(knowledgeBaseName, documentName string, opts ...rag.RAGSystemConfigOption) error {
	return rag.DeleteDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, opts...)
}

// _queryDocuments 在指定集合中查询文档
// Example:
// ```
//
//	results, err = rag.QueryDocuments("my_collection", "query", 10)
//
// ```
func _queryDocuments(knowledgeBaseName, query string, limit int, opts ...rag.RAGSystemConfigOption) ([]*rag.SearchResult, error) {
	return rag.QueryDocuments(consts.GetGormProfileDatabase(), knowledgeBaseName, query, limit, opts...)
}

// _newTempRagDatabase 创建临时 RAG 数据库
// Example:
// ```
//
//	db, err = rag.NewTempRagDatabase()
//
// ```
func _newTempRagDatabase() (*gorm.DB, error) {
	path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
	return vectorstore.NewVectorStoreDatabase(path)
}

// _enableMockMode 启用模拟模式
// Example:
// ```
//
//	rag.EnableMockMode()
//
// ```
func _enableMockMode() {
	vectorstore.IsMockMode = true
}

// _embedding 生成文本的嵌入向量
// 使用默认的嵌入服务生成文本的向量表示（优先使用在线服务，回退到本地服务）
// Example:
// ```
//
//	result, err = rag.Embedding("你好")
//	if err != nil {
//	    // handle error
//	}
//	// result is []float32
//
// ```
func _embedding(text string) ([]float32, error) {
	return vectorstore.Embedding(text)
}

// _localEmbedding 使用本地嵌入服务生成文本的向量表示
// 本地服务需要安装本地模型（如 Qwen3-Embedding-0.6B-Q4_K_M）
// Example:
// ```
//
//	result, err = rag.LocalEmbedding("你好")
//	if err != nil {
//	    // handle error - 本地服务不可用
//	}
//	// result is []float32, dimension: 1024
//
// ```
func _localEmbedding(text string) ([]float32, error) {
	service, err := vectorstore.GetLocalEmbeddingService()
	if err != nil {
		return nil, err
	}
	return service.Embedding(text)
}

// _onlineEmbedding 使用在线嵌入服务生成文本的向量表示
// 使用 AIBalance 免费在线服务，无需安装本地模型
// Example:
// ```
//
//	result, err = rag.OnlineEmbedding("你好")
//	if err != nil {
//	    // handle error - 在线服务不可用
//	}
//	// result is []float32, dimension: 1024
//
// ```
func _onlineEmbedding(text string) ([]float32, error) {
	return vectorstore.AIBalanceFreeEmbeddingFunc(text)
}

// _queryCollections 指定查询的多个集合名称
// Example:
// ```
//
//	results = rag.Query("如何使用 MITM 插件?", rag.queryCollections("collection1", "collection2", "collection3"))~
//
// ```
func _queryCollections(names ...string) rag.RAGSystemConfigOption {
	return rag.WithRAGCollectionNames(names...)
}

// _queryRAGFilename 从 RAG 文件导入后查询（自动导入）
// 适合法规条文、技术规范等精确搜索场景
// Example:
// ```
//
//	results = rag.Query("法规第2.3条", rag.queryRAGFilename("/path/to/law.rag"))~
//
// ```
func _queryRAGFilename(filename string) rag.RAGSystemConfigOption {
	// 使用文件名生成集合名
	baseName := filepath.Base(filename)
	tempRagName := "imported_" + utils.CalcSha256(filename)[:8] + "_" + baseName

	db := consts.GetGormProfileDatabase()

	// 检查是否已导入
	if !rag.HasRagSystem(db, tempRagName) {
		// 导入 RAG 文件
		err := rag.ImportRAG(filename,
			rag.WithDB(db),
			rag.WithRAGCollectionName(tempRagName),
			rag.WithExportOverwriteExisting(false),
		)
		if err != nil {
			log.Errorf("failed to import RAG file %s: %v", filename, err)
		} else {
			log.Infof("Imported RAG file %s as collection %s", filename, tempRagName)
		}
	}

	// 返回集合名选项
	return rag.WithRAGCollectionNames(tempRagName)
}

// _query 统一的查询/搜索接口
// 支持多种查询模式:
// 1. 无参数 - 查询所有集合
// 2. queryCollection/queryCollections - 指定集合查询
// 3. queryRAGFilename - 从 RAG 文件导入后查询
//
// Example:
// ```
//
//	// 查询所有集合
//	results = rag.Query("如何使用 XSS 检测?")~
//
//	// 查询指定集合（单个）
//	results = rag.Query("如何使用 MITM 插件?", rag.queryCollection("yaklang-yakscript-plugins"))~
//
//	// 查询多个集合
//	results = rag.Query("XSS 漏洞", rag.queryCollections("plugins", "tools", "docs"))~
//
//	// 从 RAG 文件导入后查询（适合法规条文等精确搜索）
//	results = rag.Query("法规第2.3条", rag.queryRAGFilename("/path/to/law.rag"))~
//
//	// 组合使用
//	results = rag.Query("XSS 漏洞",
//	    rag.queryCollections("plugins"),
//	    rag.queryLimit(20),
//	    rag.querySimilarityThreshold(0.5),
//	    rag.queryEnhance(
//	        rag.QUERY_ENHANCE_TYPE_HYPOTHETICAL_ANSWER,
//	        rag.QUERY_ENHANCE_TYPE_EXACT_KEYWORD_SEARCH,
//	    ),
//	)~
//
// ```
func _query(query string, opts ...rag.RAGSystemConfigOption) (<-chan *rag.RAGSearchResult, error) {
	return rag.QueryYakitProfile(query, opts...)
}
