package vectorstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

type ExportVectorStoreDocument struct {
	DocumentID      string                 `json:"document_id"`
	Metadata        map[string]interface{} `json:"metadata"`
	Embedding       []float32              `json:"embedding"`
	PQCode          []byte                 `json:"pq_code"`
	Content         string                 `json:"content"`
	DocumentType    string                 `json:"document_type"`
	EntityID        string                 `json:"entity_id"`
	RelatedEntities string                 `json:"related_entities"`
}

// RAGExportConfig 导入导出统一配置
type RAGExportConfig struct {
	Ctx               context.Context
	DB                *gorm.DB // 数据库（导入时使用）
	NoHNSWIndex       bool     // 是否不包含HNSW索引（导出时使用）
	OnlyPQCode        bool     // 是否只导出PQ编码（导出时使用）
	NoMetadata        bool     // 是否不导出元数据（导出时使用）
	OverwriteExisting bool     // 是否覆盖现有数据（导入时使用）
	NoOriginInput     bool     // 是否不导出原始输入数据（导出时使用）
	RebuildHNSWIndex  bool     // 是否重新构建HNSW索引（导入时使用）

	CollectionName    string // 指定集合名称（导入时使用，可选）
	DocumentHandler   func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	OnProgressHandler func(percent float64, message string, messageType string) // 进度回调

	SerialVersionUID string // 序列化版本号（导入时使用）
}

type RAGExportOptionFunc func(*RAGExportConfig)

func WithSerialVersionUID(version string) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.SerialVersionUID = version
	}
}

// 通用选项
func WithContext(ctx context.Context) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.Ctx = ctx
	}
}

func WithDocumentHandler(handler func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.DocumentHandler = handler
	}
}

func WithProgressHandler(handler func(percent float64, message string, messageType string)) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.OnProgressHandler = handler
	}
}

// RAG 配置选项
func WithNoMetadata(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.NoMetadata = b
	}
}

func WithOnlyPQCode(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.OnlyPQCode = b
	}
}

func WithNoHNSWGraph(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.NoHNSWIndex = b
	}
}

func WithNoOriginInput(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.NoOriginInput = b
	}
}

func WithImportExportDB(db *gorm.DB) RAGExportOptionFunc {
	if db != nil {
		db.AutoMigrate(&schema.VectorStoreCollection{})
		db.AutoMigrate(&schema.VectorStoreDocument{})
	}
	return func(opts *RAGExportConfig) {
		opts.DB = db
	}
}

func WithOverwriteExisting(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.OverwriteExisting = b
	}
}

func WithCollectionName(name string) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.CollectionName = name
	}
}

func WithRebuildHNSWIndex(b bool) RAGExportOptionFunc {
	return func(opts *RAGExportConfig) {
		opts.RebuildHNSWIndex = b
	}
}

func NewRAGConfig(opts ...RAGExportOptionFunc) *RAGExportConfig {
	config := &RAGExportConfig{
		Ctx:               context.Background(),
		NoHNSWIndex:       false,
		RebuildHNSWIndex:  false,
		OverwriteExisting: false,
		SerialVersionUID:  uuid.NewString(),
		DB:                consts.GetGormProfileDatabase(),
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}
