package vectorstore

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
)

type EmbeddingClient interface {
	Embedding(text string) ([]float32, error)
	// EmbeddingRaw 返回原始的 embedding 结果，可能包含多个向量
	EmbeddingRaw(text string) ([][]float32, error)
}

// RAG 搜索结果类型常量
const (
	RAGResultTypeMessage   = "message"
	RAGResultEntity        = "entity"
	RAGResultTypeMidResult = "mid_result"
	RAGResultTypeResult    = "result"
	RAGResultTypeError     = "error"
	RAGResultTypeERM       = "erm_analysis"
	RAGResultTypeDotGraph  = "dot_graph"
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
	UID             []byte                 `json:"-"` // 文档全表唯一标识符
}

// SearchResult 表示检索结果
type SearchResult struct {
	Document *Document `json:"document"` // 检索到的文档
	Score    float64   `json:"score"`    // 相似度得分 (-1 到 1 之间)
}

type EmptyEmbedding struct{}

func (e EmptyEmbedding) Embedding(text string) ([]float32, error) {
	var result = make([]float32, 0)
	for i := 0; i < 1024; i++ {
		result = append(result, float32(i))
	}
	return result, nil
}

// VectorStore 接口定义了向量存储的基本操作
type VectorStore interface {
	// Add 添加文档到向量存储
	Add(docs ...*Document) error

	// Search 根据查询文本检索相关文档
	Search(query string, page, limit int) ([]SearchResult, error)

	SearchWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error)

	// 非语义 模糊搜1
	FuzzSearch(ctx context.Context, query string, limit int) (<-chan SearchResult, error)

	// Delete 根据 ID 删除文档
	Delete(ids ...string) error

	// Get 根据 ID 获取文档
	Get(id string) (*Document, bool, error)

	// List 列出所有文档
	List() ([]*Document, error)

	// Count 返回文档总数
	Count() (int, error)
}
