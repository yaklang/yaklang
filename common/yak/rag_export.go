package yak

import (
	"path/filepath"

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
	"DeleteCollection": func(name string) error {
		return rag.DeleteCollection(consts.GetGormProfileDatabase(), name)
	},
	"ListCollection": func() []string {
		return rag.ListCollections(consts.GetGormProfileDatabase())
	},
	"GetCollectionInfo": func(name string) (*rag.CollectionInfo, error) {
		return rag.GetCollectionInfo(consts.GetGormProfileDatabase(), name)
	},

	"Query":           rag.QueryYakitProfile,
	"queryLimit":      rag.WithRAGLimit,
	"queryType":       rag.WithRAGDocumentType,
	"queryCollection": rag.WithRAGCollectionName,
	"queryStatus":     rag.WithRAGQueryStatus,
	"queryEnhance":    rag.WithRAGEnhance,
	"queryCtx":        rag.WithRAGCtx,
	"queryConcurrent": rag.WithRAGConcurrent,
	"queryScoreLimit": rag.WithRAGCollectionScoreLimit,

	"AddDocument": func(knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...any) error {
		return rag.AddDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, document, metadata, opts...)
	},
	"DeleteDocument": func(knowledgeBaseName, documentName string, opts ...any) error {
		return rag.DeleteDocument(consts.GetGormProfileDatabase(), knowledgeBaseName, documentName, opts...)
	},
	"QueryDocuments": func(knowledgeBaseName, query string, limit int, opts ...any) ([]rag.SearchResult, error) {
		return rag.QueryDocuments(consts.GetGormProfileDatabase(), knowledgeBaseName, query, limit, opts...)
	},
	"QueryDocumentsWithAISummary": func(knowledgeBaseName, query string, limit int, opts ...any) (string, error) {
		return rag.QueryDocumentsWithAISummary(consts.GetGormProfileDatabase(), knowledgeBaseName, query, limit, opts...)
	},

	"ragForceNew":       rag.WithForceNew,
	"ragDescription":    rag.WithDescription,
	"ragEmbeddingModel": rag.WithEmbeddingModel,
	"ragModelDimension": rag.WithModelDimension,
	"ragCosineDistance": rag.WithCosineDistance,
	"ragHNSWParameters": rag.WithHNSWParameters,

	"docMetadata":    rag.WithDocumentMetadataKeyValue,
	"docRawMetadata": rag.WithDocumentRawMetadata,
	"NewRagDatabase": rag.NewRagDatabase,
	"NewTempRagDatabase": func() (*gorm.DB, error) {
		path := filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String())
		return rag.NewRagDatabase(path)
	},
	"EnableMockMode": func() {
		rag.IsMockMode = true
	},

	"ctx":                       aiforge.WithAnalyzeContext,    // use for analyzeContext
	"log":                       aiforge.WithAnalyzeLog,        // use for analyzeLog
	"statusCard":                aiforge.WithAnalyzeStatusCard, // use for analyzeStatusCard
	"extraPrompt":               aiforge.WithExtraPrompt,       // use for analyzeImage and analyzeImageFile
	"entryLength":               aiforge.RefineWithKnowledgeEntryLength,
	"khopk":                     entityrepos.WithKHopK,
	"khopLimit":                 entityrepos.WithKHopLimit,
	"khopkMin":                  entityrepos.WithKHopKMin,
	"khopkMax":                  entityrepos.WithKHopKMax,
	"buildQuery":                entityrepos.WithRagQuery,
	"buildFilter":               entityrepos.WithStartEntityFilter,
	"BuildCollectionFromFile":   aiforge.BuildKnowledgeFromFile,
	"BuildCollectionFromReader": aiforge.BuildKnowledgeFromReader,
	"BuildCollectionFromRaw":    aiforge.BuildKnowledgeFromBytes,

	"BuildKnowledgeFromEntityRepos": aiforge.BuildKnowledgeFromEntityReposByName,
}
