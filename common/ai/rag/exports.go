package rag

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// OpenaiEmbeddingAdapter 是对 OpenaiEmbeddingClient 的适配器，确保其实现 EmbeddingClient 接口
type OpenaiEmbeddingAdapter struct {
	client *embedding.OpenaiEmbeddingClient
}

// NewOpenaiEmbeddingAdapter 创建一个新的 OpenAI 嵌入适配器
func NewOpenaiEmbeddingAdapter(opts ...aispec.AIConfigOption) *OpenaiEmbeddingAdapter {
	return &OpenaiEmbeddingAdapter{
		client: embedding.NewOpenaiEmbeddingClient(opts...),
	}
}

// Embedding 实现 EmbeddingClient 接口
func (o *OpenaiEmbeddingAdapter) Embedding(text string) ([]float64, error) {
	return o.client.Embedding(text)
}

// NewDefaultRAGSystem 创建一个默认配置的 RAG 系统
func NewDefaultRAGSystem(opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	// 创建嵌入客户端适配器
	embedder := NewOpenaiEmbeddingAdapter(opts...)

	// 创建内存向量存储
	store := NewMemoryVectorStore(embedder)

	// 创建 RAG 系统
	ragSystem := NewRAGSystem(embedder, store)

	return ragSystem, nil
}

// NewDefaultSQLiteRAGSystem 创建一个基于 SQLite 的 RAG 系统
func NewDefaultSQLiteRAGSystem(db *gorm.DB, collectionName string, modelName string, dimension int, opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	// 创建嵌入客户端适配器
	embedder := NewOpenaiEmbeddingAdapter(opts...)

	// 创建 SQLite 向量存储
	store, err := NewSQLiteVectorStore(db, collectionName, modelName, dimension, embedder)
	if err != nil {
		return nil, utils.Errorf("创建 SQLite 向量存储失败: %v", err)
	}

	// 创建 RAG 系统
	ragSystem := NewRAGSystem(embedder, store)

	return ragSystem, nil
}

// AddText 将文本添加到 RAG 系统中
func AddText(rag *RAGSystem, text string, maxChunkSize int, overlap int, metadata map[string]any) error {
	// 将文本分割成文档
	docs := TextToDocuments(text, maxChunkSize, overlap, metadata)

	// 添加文档到 RAG 系统
	return rag.AddDocuments(docs...)
}

// SearchAndGeneratePrompt 检索相关文档并生成提示
func SearchAndGeneratePrompt(rag *RAGSystem, query string, limit int, threshold float64, promptTemplate string) (string, error) {
	// 检索相关文档
	results, err := rag.Query(query, limit)
	if err != nil {
		return "", utils.Errorf("failed to query documents: %v", err)
	}

	// 根据阈值过滤结果
	filteredResults := FilterResults(results, threshold)

	// 生成提示
	prompt := FormatRagPrompt(query, filteredResults, promptTemplate)

	return prompt, nil
}

func GetAllCollections() ([]*schema.VectorStoreCollection, error) {
	db := consts.GetGormProfileDatabase()
	collections := []*schema.VectorStoreCollection{}
	db.Model(&schema.VectorStoreCollection{}).Find(&collections)
	return collections, nil
}

// 导出的公共函数
var Exports = map[string]interface{}{
	"NewSystem":               NewDefaultRAGSystem,
	"NewSQLiteSystem":         NewDefaultSQLiteRAGSystem,
	"AddText":                 AddText,
	"SearchAndGeneratePrompt": SearchAndGeneratePrompt,
	"ChunkText":               ChunkText,
	"FilterResults":           FilterResults,
}
