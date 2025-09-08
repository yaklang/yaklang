package rag

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type KnowledgeBaseConfig struct {
	Description string

	// 是否强制创建新的知识库，如果已经存在，会返回错误
	ForceNew bool

	// embedding 配置
	ModelName string
	Dimension int

	// hnsw 配置
	DistanceFuncType      string
	MaxNeighbors          int
	LayerGenerationFactor float64
	EfSearch              int
	EfConstruct           int

	// ai 配置
	AIOptions []aispec.AIConfigOption

	EmbeddingClient aispec.EmbeddingCaller

	LazyLoadEmbeddingClient bool
}

func NewKnowledgeBaseConfig(options ...any) *KnowledgeBaseConfig {
	defaultConfig := &KnowledgeBaseConfig{
		ModelName:             "Qwen3-Embedding-0.6B-Q4_K_M",
		Dimension:             1024,
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

// WithEmbeddingClient 设置embedding客户端
func WithEmbeddingClient(client aispec.EmbeddingCaller) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.EmbeddingClient = client
	}
}

func WithLazyLoadEmbeddingClient() RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.LazyLoadEmbeddingClient = true
	}
}

func WithDescription(description string) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.Description = description
	}
}

func WithForceNew(i ...bool) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		if len(i) > 0 {
			config.ForceNew = i[0]
		} else {
			config.ForceNew = true
		}
	}
}

// WithEmbeddingModel 设置embedding模型
func WithEmbeddingModel(model string) RAGOption {
	return func(config *KnowledgeBaseConfig) {
		config.ModelName = model
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

// CollectionIsExists 检查知识库是否存在
func CollectionIsExists(db *gorm.DB, name string) bool {
	collections := []*schema.VectorStoreCollection{}
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Find(&collections)
	return len(collections) > 0
}

// CreateCollection 创建知识库
func CreateCollection(db *gorm.DB, name string, description string, opts ...any) (*RAGSystem, error) {
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
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
	if CollectionIsExists(db, name) {
		return nil, utils.Errorf("集合 %s 已存在", name)
	}

	// 创建集合
	res := db.Create(&collection)
	if res.Error != nil {
		return nil, utils.Errorf("创建集合失败: %v", res.Error)
	}
	var ragSystem *RAGSystem
	var err error
	if cfg.EmbeddingClient != nil {
		ragSystem, err = LoadCollectionWithEmbeddingClient(db, name, cfg.EmbeddingClient, cfg.AIOptions...)
	} else {
		ragSystem, err = LoadCollection(db, name, cfg.AIOptions...)
	}
	if err != nil {
		return nil, utils.Errorf("创建集合失败: %v", err)
	}
	ragSystem.addDocuments(Document{
		ID:      DocumentTypeCollectionInfo,
		Content: fmt.Sprintf("collection_name: %s\ncollection_description: %s", name, description),
		Metadata: map[string]any{
			"collection_name": name,
			"collection_id":   collection.ID,
		},
		Embedding: nil,
	})
	return ragSystem, nil
}

func LoadCollectionWithEmbeddingClient(db *gorm.DB, name string, client aispec.EmbeddingCaller, opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	// 创建 SQLite 向量存储
	log.Infof("start to load sqlite vector store for collection %#v", name)
	store, err := LoadSQLiteVectorStoreHNSW(db, name, client)
	if err != nil {
		return nil, utils.Errorf("load SQLite vector storage err: %v", err)
	}
	// 创建 RAG 系统
	log.Infof("start to create RAG system for collection %#v", name)
	ragSystem := NewRAGSystem(client, store)

	return ragSystem, nil
}

var IsMockMode = false

// LoadCollection 加载知识库
func LoadCollection(db *gorm.DB, name string, opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	log.Infof("loading collection '%s' with local embedding service", name)

	// 使用本地嵌入服务
	var embeddingService EmbeddingClient
	if IsMockMode {
		// 使用模拟的嵌入服务
		mockRagDataForTest, err := getMockRagDataForTest()
		if err != nil {
			log.Errorf("failed to get mock rag data for test: %v", err)
			return nil, utils.Errorf("failed to get mock rag data for test: %v", err)
		}
		embeddingService = NewMockEmbedder(mockRagDataForTest)
		log.Infof("successfully initialized RAG system with mock embedding service")
	} else {
		localEmbedder, err := GetLocalEmbeddingService()
		if err != nil {
			log.Errorf("failed to get local embedding service: %v", err)
			return nil, utils.Errorf("failed to initialize local embedding service: %v", err)
		}

		log.Infof("using local embedding service at %s for collection '%s'", localEmbedder.GetAddress(), name)
		embeddingService = localEmbedder
	}

	return LoadCollectionWithEmbeddingClient(db, name, embeddingService, opts...)
}

// LoadCollectionWithCustomEmbedding 使用自定义嵌入服务加载知识库
func LoadCollectionWithCustomEmbedding(db *gorm.DB, name string, opts ...aispec.AIConfigOption) (*RAGSystem, error) {
	log.Infof("loading collection '%s' with custom embedding client", name)

	// 创建自定义嵌入客户端适配器
	embedder := embedding.NewOpenaiEmbeddingClient(opts...)
	return LoadCollectionWithEmbeddingClient(db, name, embedder, opts...)
}

// CreateOrLoadCollection 创建或加载知识库
func CreateOrLoadCollection(db *gorm.DB, name string, description string, opts ...any) (*RAGSystem, error) {
	cfg := NewKnowledgeBaseConfig(opts...)
	if CollectionIsExists(db, name) {
		log.Infof("collection '%s' exists, loading it", name)
		if cfg.EmbeddingClient != nil {
			log.Infof("using provided embedding client for collection '%s'", name)
			return LoadCollectionWithEmbeddingClient(db, name, cfg.EmbeddingClient, cfg.AIOptions...)
		}
		log.Infof("using default local embedding service for collection '%s'", name)
		return LoadCollection(db, name, cfg.AIOptions...)
	} else {
		log.Infof("collection '%s' does not exist, creating it", name)
		return CreateCollection(db, name, description, opts...)
	}
}

// DeleteCollection 删除知识库
func DeleteCollection(db *gorm.DB, name string) error {
	db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Unscoped().Delete(&schema.VectorStoreCollection{})
	return nil
}

// ListCollections 获取所有知识库列表
func ListCollections(db *gorm.DB) []string {
	collections := []*schema.VectorStoreCollection{}
	db.Model(&schema.VectorStoreCollection{}).Find(&collections)
	names := []string{}
	for _, collection := range collections {
		names = append(names, collection.Name)
	}
	return names
}

type CollectionInfo struct {
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

// GetCollectionInfo 获取知识库信息
func GetCollectionInfo(db *gorm.DB, name string) (*CollectionInfo, error) {
	var collections []*schema.VectorStoreCollection
	dbErr := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).Find(&collections)
	if dbErr.Error != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", dbErr.Error)
	}
	if len(collections) == 0 {
		return nil, utils.Errorf("知识库 %s 不存在", name)
	}
	collection := collections[0]
	// 暂时不支持从数据库恢复HNSW图结构
	// 当前返回基本信息
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

	// 如果layers为nil（不支持恢复），则只提供基本信息
	if layers == nil {
		// 获取文档数量作为基本信息
		var docCount int64
		db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&docCount)
		nodeCount = int(docCount)
	} else {
		for index, layer := range layers {
			layerNodeCountMap[index] = len(layer.Nodes)
			nodeCount += len(layer.Nodes)

			// 遍历该层的所有节点，统计邻居信息
			for _, node := range layer.Nodes {
				if node == nil {
					continue
				}

				neighborCount := len(node.GetNeighbors())

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
	}

	// 如果没有任何节点，将最小邻居数设为0
	if minNeighbors == -1 {
		minNeighbors = 0
	}

	return &CollectionInfo{
		Name:        collection.Name,
		Description: collection.Description,
		ModelName:   collection.ModelName,
		Dimension:   collection.Dimension,

		M:                collection.M,
		Ml:               collection.Ml,
		EfSearch:         collection.EfSearch,
		EfConstruct:      collection.EfConstruct,
		DistanceFuncType: collection.DistanceFuncType,

		LayerCount: func() int {
			if layers == nil {
				return 0
			} else {
				return len(layers)
			}
		}(),
		LayerNodeCountMap: layerNodeCountMap,
		NodeCount:         nodeCount,
		MaxNeighbors:      maxNeighbors,
		MinNeighbors:      minNeighbors,
		ConnectionCount:   connectionCount,
	}, nil
}

// AddDocument 添加文档
func AddDocument(db *gorm.DB, knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...any) error {
	cfg := NewKnowledgeBaseConfig(opts...)

	ragSystem, err := LoadCollection(db, knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.addDocuments(Document{
		ID:        documentName,
		Content:   document,
		Metadata:  metadata,
		Embedding: nil,
	})
}

// DeleteDocument 删除文档
func DeleteDocument(db *gorm.DB, knowledgeBaseName, documentName string, opts ...any) error {
	cfg := NewKnowledgeBaseConfig(opts...)
	ragSystem, err := LoadCollection(db, knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.DeleteDocuments(documentName)
}

// QueryDocuments 查询文档
func QueryDocuments(db *gorm.DB, knowledgeBaseName, query string, limit int, opts ...any) ([]SearchResult, error) {
	cfg := NewKnowledgeBaseConfig(opts...)

	ragSystem, err := LoadCollection(db, knowledgeBaseName, cfg.AIOptions...)
	if err != nil {
		return nil, utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.QueryWithPage(query, 1, limit)
}

// QueryDocumentsWithAISummary 查询文档并生成摘要
func QueryDocumentsWithAISummary(db *gorm.DB, knowledgeBaseName, query string, limit int, opts ...any) (string, error) {
	// TODO: 实现查询文档并生成摘要
	return "", nil
}

func NewRagDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", path)
	if err != nil {
		return db, err
	}
	db = db.AutoMigrate(&schema.KnowledgeBaseEntry{}, &schema.KnowledgeBaseInfo{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	return db, nil
}

func Get(name string, i ...any) (*RAGSystem, error) {
	log.Infof("getting RAG collection '%s' with local embedding service", name)

	config := NewKnowledgeBaseConfig(i)
	if config.ForceNew {
		log.Infof("force creating new RAG collection for name: %s", name)
		return CreateCollection(consts.GetGormProfileDatabase(), name, config.Description, i...)
	}

	// load existed first
	log.Infof("attempting to load existing RAG collection '%s'", name)
	ragSystem, err := LoadCollection(consts.GetGormProfileDatabase(), name)
	if err != nil {
		log.Errorf("failed to load existing RAG collection '%s': %v, creating new one", name, err)
		return CreateCollection(consts.GetGormProfileDatabase(), name, config.Description, i...)
	}

	log.Infof("successfully loaded RAG collection '%s'", name)
	return ragSystem, nil
}

type DocumentOption func(document *Document)

func WithDocumentMetadataKeyValue(key string, value any) DocumentOption {
	return func(document *Document) {
		if utils.IsNil(document.Metadata) {
			document.Metadata = make(map[string]any)
		}
		document.Metadata[key] = value
	}
}

func WithDocumentRawMetadata(i map[string]any) DocumentOption {
	return func(document *Document) {
		document.Metadata = i
		if utils.IsNil(document.Metadata) {
			document.Metadata = make(map[string]any)
		}
	}
}

func WithDocumentType(i schema.RAGDocumentType) DocumentOption {
	return func(document *Document) {
		document.Type = i
	}
}

func WithDocumentEntityID(entityUUID string) DocumentOption {
	return func(document *Document) {
		document.EntityUUID = entityUUID
	}
}

func WithDocumentRelatedEntities(uuids ...string) DocumentOption {
	return func(document *Document) {
		document.RelatedEntities = uuids
	}
}
