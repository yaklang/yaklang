package yak

import (
	"path/filepath"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
)

// 导出的公共函数
var RagExports = map[string]interface{}{
	"GetCollection": rag.Get,

	"embeddingHandle": _embeddingHandle,

	"DeleteCollection":  _deleteCollection,
	"ListCollection":    _listCollection,
	"GetCollectionInfo": _getCollectionInfo,

	"HasCollection": _hasCollection,

	"Query":           rag.QueryYakitProfile,
	"queryLimit":      rag.WithRAGLimit,
	"queryType":       rag.WithRAGDocumentType,
	"queryCollection": rag.WithRAGCollectionName,
	"queryStatus":     rag.WithRAGQueryStatus,
	"queryEnhance":    rag.WithRAGEnhance,
	"queryCtx":        rag.WithRAGCtx,
	"queryConcurrent": rag.WithRAGConcurrent,
	"queryScoreLimit": rag.WithRAGCollectionScoreLimit,

	"AddDocument":                 _addDocument,
	"DeleteDocument":              _deleteDocument,
	"QueryDocuments":              _queryDocuments,
	"QueryDocumentsWithAISummary": _queryDocumentsWithAISummary,

	"ragForceNew":       rag.WithForceNew,
	"ragDescription":    rag.WithDescription,
	"ragEmbeddingModel": rag.WithEmbeddingModel,
	"ragModelDimension": rag.WithModelDimension,
	"ragCosineDistance": rag.WithCosineDistance,
	"ragHNSWParameters": rag.WithHNSWParameters,

	"docMetadata":        rag.WithDocumentMetadataKeyValue,
	"docRawMetadata":     rag.WithDocumentRawMetadata,
	"NewRagDatabase":     rag.NewRagDatabase,
	"NewTempRagDatabase": _newTempRagDatabase,
	"EnableMockMode":     _enableMockMode,

	"ctx":             aiforge.WithAnalyzeContext,    // use for analyzeContext
	"log":             aiforge.WithAnalyzeLog,        // use for analyzeLog
	"statusCard":      aiforge.WithAnalyzeStatusCard, // use for analyzeStatusCard
	"extraPrompt":     aiforge.WithExtraPrompt,       // use for analyzeImage and analyzeImageFile
	"entryLength":     aiforge.RefineWithKnowledgeEntryLength,
	"chunkSize":       chunkmaker.WithChunkSize,
	"khopk":           entityrepos.WithKHopK,
	"khopLimit":       entityrepos.WithKHopLimit,
	"khopkMin":        entityrepos.WithKHopKMin,
	"khopkMax":        entityrepos.WithKHopKMax,
	"buildQuery":      entityrepos.WithRagQuery,
	"buildFilter":     entityrepos.WithStartEntityFilter,
	"pathDepth":       entityrepos.WithPathDepth,
	"getEntityFilter": schema.SimpleBuildEntityFilter,

	"BuildCollectionFromFile":   aiforge.BuildKnowledgeFromFile,
	"BuildCollectionFromReader": aiforge.BuildKnowledgeFromReader,
	"BuildCollectionFromRaw":    aiforge.BuildKnowledgeFromBytes,

	"BuildKnowledgeFromEntityRepos": aiforge.BuildKnowledgeFromEntityReposByName,

	"BuildIndexKnowledgeFromFile": aiforge.BuildIndexKnowledgeFromFile,

	"Import":          rag.ImportRAGFromFile,
	"db":              rag.WithImportExportDB,
	"importOverwrite": rag.WithOverwriteExisting,
	"importName":      rag.WithCollectionName,
	"documentHandler": rag.WithDocumentHandler,
	"progressHandler": rag.WithProgressHandler,

	"Export":        rag.ExportRAGToFile,
	"noHNSWGraph":   rag.WithNoHNSWGraph,
	"noMetadata":    rag.WithNoMetadata,
	"noOriginInput": rag.WithNoOriginInput,
	"onlyPQCode":    rag.WithOnlyPQCode,
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

// _embeddingHandle 创建自定义嵌入处理器
// Example:
// ```
//
//	embeddingOpt = rag.embeddingHandle((text) => {
//		return [0.1, 0.2, 0.3] // 返回嵌入向量
//	})
//
// ```
func _embeddingHandle(handle func(text string) any) rag.RAGOption {
	embedder := rag.NewMockEmbedder(func(text string) ([]float32, error) {
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
func _getCollectionInfo(name string) (*rag.CollectionInfo, error) {
	return rag.GetCollectionInfo(consts.GetGormProfileDatabase(), name)
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
func _addDocument(knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...any) error {
	return rag.AddDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, document, metadata, opts...)
}

// _deleteDocument 从指定集合删除文档
// Example:
// ```
//
//	err = rag.DeleteDocument("my_collection", "doc1")
//
// ```
func _deleteDocument(knowledgeBaseName, documentName string, opts ...any) error {
	return rag.DeleteDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, opts...)
}

// _queryDocuments 在指定集合中查询文档
// Example:
// ```
//
//	results, err = rag.QueryDocuments("my_collection", "query", 10)
//
// ```
func _queryDocuments(knowledgeBaseName, query string, limit int, opts ...any) ([]rag.SearchResult, error) {
	return rag.QueryDocuments(consts.GetGormProfileDatabase(), knowledgeBaseName, query, limit, opts...)
}

// _queryDocumentsWithAISummary 在指定集合中查询文档并生成 AI 摘要
// Example:
// ```
//
//	summary, err = rag.QueryDocumentsWithAISummary("my_collection", "query", 10)
//
// ```
func _queryDocumentsWithAISummary(knowledgeBaseName, query string, limit int, opts ...any) (string, error) {
	return rag.QueryDocumentsWithAISummary(consts.GetGormProfileDatabase(), knowledgeBaseName, query, limit, opts...)
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
	return rag.NewRagDatabase(path)
}

// _enableMockMode 启用模拟模式
// Example:
// ```
//
//	rag.EnableMockMode()
//
// ```
func _enableMockMode() {
	rag.IsMockMode = true
}
