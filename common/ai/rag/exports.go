package rag

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type KnowledgeBaseConfig struct {
	// embedding 配置
	ModelName string
	Dimension int
	Endpoint  string
	APIKey    string

	// hnsw 配置
	DistanceFuncType      string
	MaxNeighbors          int
	LayerGenerationFactor float64
	EfSearch              int
	EfConstruct           int

	// ai 配置
	AIOptions []aispec.AIConfigOption
}

func NewKnowledgeBaseConfig(options ...any) *KnowledgeBaseConfig {
	defaultConfig := &KnowledgeBaseConfig{
		ModelName:             "Qwen3-Embedding-0.6B-Q8_0",
		Dimension:             1024,
		Endpoint:              "http://127.0.0.1:11434/embeddings",
		APIKey:                "",
		DistanceFuncType:      "cosine",
		MaxNeighbors:          16,
		LayerGenerationFactor: 0.25,
		EfSearch:              20,
		EfConstruct:           200,
	}

	aiOptions := []aispec.AIConfigOption{}
	for _, option := range options {
		if aiOption, ok := option.(aispec.AIConfigOption); ok {
			aiOptions = append(aiOptions, aiOption)
		}
		if ragOption, ok := option.(RAGOption); ok {
			ragOption(defaultConfig)
		}
	}
	defaultConfig.AIOptions = aiOptions

	return defaultConfig
}

type RAGOption func(config *KnowledgeBaseConfig)

// WithEmbeddingEndpoint 设置embedding端点
func WithEmbeddingEndpoint(endpoint string) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.Endpoint = endpoint
	}
}

// WithEmbeddingModel 设置embedding模型
func WithEmbeddingModel(model string) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.ModelName = model
	}
}

// WithEmbeddingAPIKey 设置embedding API密钥
func WithEmbeddingAPIKey(apiKey string) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.APIKey = apiKey
	}
}

// WithModelDimension 设置模型维度
func WithModelDimension(dimension int) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.Dimension = dimension
	}
}

// WithCosineDistance 设置使用余弦距离
func WithCosineDistance() RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.DistanceFuncType = "cosine"
	}
}

// // WithEuclideanDistance 设置使用欧几里得距离
// func WithEuclideanDistance() RAGOption

// // WithManhattanDistance 设置使用曼哈顿距离
// func WithManhattanDistance() RAGOption

// // WithDotDistance 设置使用点积距离
// func WithDotDistance() RAGOption

// WithHNSWParameters 批量设置HNSW参数
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.MaxNeighbors = m
		config.LayerGenerationFactor = ml
		config.EfSearch = efSearch
		config.EfConstruct = efConstruct
	}
}

// KnowledgeBaseIsExists 检查知识库是否存在
func KnowledgeBaseIsExists(name string) bool {
	db := consts.GetGormProfileDatabase()
	collections := []*schema.VectorStoreCollection{}
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Find(&collections)
	return len(collections) > 0
}

// CreateKnowledgeBase 创建知识库
func CreateKnowledgeBase(name string, description string, opts ...any) (*RAGSystem, error) {
	// 创建知识库配置
	cfg := NewKnowledgeBaseConfig(opts...)

	// 创建集合配置
	collection := schema.VectorStoreCollection{
		Name:             name,
		Description:      description,
		ModelName:        cfg.ModelName,
		Dimension:        cfg.Dimension,
		M:                cfg.MaxNeighbors,
		Ml:               cfg.LayerGenerationFactor,
		EfSearch:         cfg.EfSearch,
		EfConstruct:      cfg.EfConstruct,
		DistanceFuncType: cfg.DistanceFuncType,
	}

	// 检查集合是否存在
	if KnowledgeBaseIsExists(name) {
		return nil, utils.Errorf("集合 %s 已存在", name)
	}

	db := consts.GetGormProfileDatabase()
	// 创建集合
	db.Create(&collection)
	return LoadKnowledgeBase(name, cfg.AIOptions...)
}

// LoadKnowledgeBase 加载知识库
func LoadKnowledgeBase(name string, opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	// 创建嵌入客户端适配器
	embedder := embedding.NewOpenaiEmbeddingClient(opts...)
	db := consts.GetGormProfileDatabase()
	// 创建 SQLite 向量存储
	store, err := LoadSQLiteVectorStoreHNSW(db, name, embedder)
	if err != nil {
		return nil, utils.Errorf("创建 SQLite 向量存储失败: %v", err)
	}
	// 创建 RAG 系统
	ragSystem := NewRAGSystem(embedder, store)

	return ragSystem, nil
}

// CreateOrLoadKnowledgeBase 创建或加载知识库
func CreateOrLoadKnowledgeBase(name string, description string, opts ...any) (*RAGSystem, error) {
	cfg := NewKnowledgeBaseConfig(opts...)
	if KnowledgeBaseIsExists(name) {
		return LoadKnowledgeBase(name, cfg.AIOptions...)
	}
	return CreateKnowledgeBase(name, description, opts...)
}

// DeleteKnowledgeBase 删除知识库
func DeleteKnowledgeBase(name string) error {
	db := consts.GetGormProfileDatabase()
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Unscoped().Delete(&schema.VectorStoreCollection{})
	return nil
}

// ListKnowledgeBases 获取所有知识库列表
func ListKnowledgeBases() []string {
	db := consts.GetGormProfileDatabase()
	collections := []*schema.VectorStoreCollection{}
	db.Model(&schema.VectorStoreCollection{}).Find(&collections)
	names := []string{}
	for _, collection := range collections {
		names = append(names, collection.Name)
	}
	return names
}

type KnowledgeBaseInfo struct {
	Name        string
	Description string
	ModelName   string
	Dimension   int

	M                int
	Ml               float64
	EfSearch         int
	EfConstruct      int
	DistanceFuncType string

	LayerCount        int         // Layer数量
	LayerNodeCountMap map[int]int // Layer节点数量
	NodeCount         int         // 节点数量
	MaxNeighbors      int         // 最大邻居数
	MinNeighbors      int         // 最小邻居数
	ConnectionCount   int         // 总连接数
}

// GetKnowledgeBaseInfo 获取知识库信息
func GetKnowledgeBaseInfo(name string) (*KnowledgeBaseInfo, error) {
	db := consts.GetGormProfileDatabase()
	var collections []*schema.VectorStoreCollection
	dbErr := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Find(&collections)
	if dbErr.Error != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", dbErr.Error)
	}
	if len(collections) == 0 {
		return nil, utils.Errorf("知识库 %s 不存在", name)
	}
	collection := collections[0]
	layers := ParseLayersInfo(&collections[0].GroupInfos, func(key string) []float32 {
		var docs []schema.VectorStoreDocument
		db.Where("document_id = ?", key).Find(&docs)
		if len(docs) == 0 {
			return nil
		}
		return []float32(docs[0].Embedding)
	})
	layerNodeCountMap := make(map[int]int)
	nodeCount := 0
	maxNeighbors := 0
	minNeighbors := -1 // 初始化为-1，表示还没有找到任何节点
	connectionCount := 0

	for index, layer := range layers {
		layerNodeCountMap[index] = len(layer.Nodes)
		nodeCount += len(layer.Nodes)

		// 遍历该层的所有节点，统计邻居信息
		for _, node := range layer.Nodes {
			if node == nil {
				continue
			}

			neighborCount := len(node.Neighbors)

			// 更新最大邻居数
			if neighborCount > maxNeighbors {
				maxNeighbors = neighborCount
			}

			// 更新最小邻居数
			if minNeighbors == -1 || neighborCount < minNeighbors {
				minNeighbors = neighborCount
			}

			// 累计连接数（每个邻居关系算作一个连接）
			connectionCount += neighborCount
		}
	}

	// 如果没有任何节点，将最小邻居数设为0
	if minNeighbors == -1 {
		minNeighbors = 0
	}

	return &KnowledgeBaseInfo{
		Name:        collection.Name,
		Description: collection.Description,
		ModelName:   collection.ModelName,
		Dimension:   collection.Dimension,

		M:                collection.M,
		Ml:               collection.Ml,
		EfSearch:         collection.EfSearch,
		EfConstruct:      collection.EfConstruct,
		DistanceFuncType: collection.DistanceFuncType,

		LayerCount:        len(layers),
		LayerNodeCountMap: layerNodeCountMap,
		NodeCount:         nodeCount,
		MaxNeighbors:      maxNeighbors,
		MinNeighbors:      minNeighbors,
		ConnectionCount:   connectionCount,
	}, nil
}

// AddDocument 添加文档
func AddDocument(knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...any) error {
	cfg := NewKnowledgeBaseConfig(opts...)

	ragSystem, err := LoadKnowledgeBase(knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.AddDocuments(Document{
		ID:        documentName,
		Content:   document,
		Metadata:  metadata,
		Embedding: nil,
	})
}

// DeleteDocument 删除文档
func DeleteDocument(knowledgeBaseName, documentName string, opts ...any) error {
	cfg := NewKnowledgeBaseConfig(opts...)
	ragSystem, err := LoadKnowledgeBase(knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.DeleteDocuments(documentName)
}

// QueryDocuments 查询文档
func QueryDocuments(knowledgeBaseName, query string, limit int, opts ...any) ([]SearchResult, error) {
	cfg := NewKnowledgeBaseConfig(opts...)

	ragSystem, err := LoadKnowledgeBase(knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.Query(query, 1, limit)
}

// QueryDocumentsWithAISummary 查询文档并生成摘要
func QueryDocumentsWithAISummary(knowledgeBaseName, query string, limit int, opts ...any) (string, error) {
	// TODO: 实现查询文档并生成摘要
	return "", nil
}

// 导出的公共函数
var Exports = map[string]interface{}{
	"CreateKnowledgeBase":  CreateKnowledgeBase,
	"LoadKnowledgeBase":    LoadKnowledgeBase,
	"DeleteKnowledgeBase":  DeleteKnowledgeBase,
	"ListKnowledgeBases":   ListKnowledgeBases,
	"GetKnowledgeBaseInfo": GetKnowledgeBaseInfo,

	"AddDocument":                 AddDocument,
	"DeleteDocument":              DeleteDocument,
	"QueryDocuments":              QueryDocuments,
	"QueryDocumentsWithAISummary": QueryDocumentsWithAISummary,

	"embeddingEndpoint": WithEmbeddingEndpoint,
	"embeddingModel":    WithEmbeddingModel,
	"embeddingAPIKey":   WithEmbeddingAPIKey,
	"modelDimension":    WithModelDimension,
	"cosineDistance":    WithCosineDistance,
	"hnswParameters":    WithHNSWParameters,
}
