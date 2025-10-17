package rag

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type CollectionConfig struct {
	Description string

	// 是否强制创建新的知识库，如果已经存在，会返回错误
	ForceNew bool

	// embedding 配置
	ModelName       string
	Dimension       int
	EmbeddingClient aispec.EmbeddingCaller

	// hnsw 配置
	DistanceFuncType      string
	MaxNeighbors          int
	LayerGenerationFactor float64
	EfSearch              int
	EfConstruct           int

	EnablePQ                   bool
	EnableAutoUpdateGraphInfos bool
	LazyLoadEmbeddingClient    bool

	buildGraphFilter *yakit.VectorDocumentFilter
	buildGraphPolicy string

	otherOptions []any
}

func NewCollectionConfig(options ...any) *CollectionConfig {
	defaultConfig := &CollectionConfig{
		ModelName:                  "Qwen3-Embedding-0.6B-Q4_K_M",
		Dimension:                  1024,
		DistanceFuncType:           "cosine",
		MaxNeighbors:               16,
		LayerGenerationFactor:      0.25,
		EfSearch:                   20,
		EfConstruct:                200,
		EnableAutoUpdateGraphInfos: true,
	}

	for _, option := range options {
		if ragOption, ok := option.(RAGOption); ok {
			ragOption(defaultConfig)
		} else {
			defaultConfig.otherOptions = append(defaultConfig.otherOptions, option)
		}
	}
	return defaultConfig
}

func LoadConfigFromCollectionInfo(collection *schema.VectorStoreCollection, options ...any) *CollectionConfig {
	loadBasicConfig := &CollectionConfig{
		ModelName:                  collection.ModelName,
		Dimension:                  collection.Dimension,
		DistanceFuncType:           collection.DistanceFuncType,
		MaxNeighbors:               collection.M,
		LayerGenerationFactor:      collection.Ml,
		EfSearch:                   collection.EfSearch,
		EfConstruct:                collection.EfConstruct,
		Description:                collection.Description,
		EnablePQ:                   collection.EnablePQMode,
		EnableAutoUpdateGraphInfos: true,
	}
	for _, option := range options {
		if ragOption, ok := option.(RAGOption); ok {
			ragOption(loadBasicConfig)
		} else {
			loadBasicConfig.otherOptions = append(loadBasicConfig.otherOptions, option)
		}
	}
	return loadBasicConfig
}

func (c *CollectionConfig) FixEmbeddingClient() error {
	if IsMockMode {
		// 使用模拟的嵌入服务
		mockRagDataForTest, err := getMockRagDataForTest()
		if err != nil {
			log.Errorf("failed to get mock rag data for test: %v", err)
			return utils.Errorf("failed to get mock rag data for test: %v", err)
		}
		log.Infof("successfully initialized RAG system with mock embedding service")
		c.EmbeddingClient = NewMockEmbedder(mockRagDataForTest)
	} else if c.EmbeddingClient == nil {
		localEmbedder, err := GetLocalEmbeddingService()
		if err != nil {
			log.Errorf("failed to get local embedding service: %v", err)
			return utils.Errorf("failed to initialize local embedding service: %v", err)
		}
		log.Infof("using local embedding service at %s", localEmbedder.GetAddress())
		c.EmbeddingClient = localEmbedder
	}
	return nil
}

type RAGOption func(config *CollectionConfig)

// WithEmbeddingClient 设置embedding客户端
func WithEmbeddingClient(client aispec.EmbeddingCaller) RAGOption {
	return func(config *CollectionConfig) {
		config.EmbeddingClient = client
	}
}

func WithLazyLoadEmbeddingClient() RAGOption {
	return func(config *CollectionConfig) {
		config.LazyLoadEmbeddingClient = true
	}
}

func WithDescription(description string) RAGOption {
	return func(config *CollectionConfig) {
		config.Description = description
	}
}

func WithForceNew(i ...bool) RAGOption {
	return func(config *CollectionConfig) {
		if len(i) > 0 {
			config.ForceNew = i[0]
		} else {
			config.ForceNew = true
		}
	}
}

// WithEmbeddingModel 设置embedding模型
func WithEmbeddingModel(model string) RAGOption {
	return func(config *CollectionConfig) {
		config.ModelName = model
	}
}

// WithModelDimension 设置模型维度
func WithModelDimension(dimension int) RAGOption {
	return func(config *CollectionConfig) {
		config.Dimension = dimension
	}
}

func WithModelName(name string) RAGOption {
	return func(config *CollectionConfig) {
		config.ModelName = name
	}
}

func WithBuildGraphFilter(filter *yakit.VectorDocumentFilter) RAGOption {
	return func(config *CollectionConfig) {
		config.buildGraphFilter = filter
	}
}

func WithBuildGraphPolicy(policy string) RAGOption {
	return func(config *CollectionConfig) {
		config.buildGraphPolicy = policy
	}
}

func WithCosineDistance() RAGOption {
	return func(config *CollectionConfig) {
		config.DistanceFuncType = "cosine"
	}
}

// WithHNSWParameters 批量设置HNSW参数
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) RAGOption {
	return func(config *CollectionConfig) {
		config.MaxNeighbors = m
		config.LayerGenerationFactor = ml
		config.EfSearch = efSearch
		config.EfConstruct = efConstruct
	}
}

// CollectionIsExists 检查知识库是否存在
func CollectionIsExists(db *gorm.DB, name string) bool {
	col, err := yakit.QueryRAGCollectionByName(db, name)
	return col != nil && err == nil
}

// CreateCollection 创建RAG集合
func CreateCollection(db *gorm.DB, name string, description string, opts ...any) (*RAGSystem, error) {
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	// 创建RAG配置
	// 检查集合是否存在
	if CollectionIsExists(db, name) {
		return nil, utils.Errorf("集合 %s 已存在", name)
	}

	store, err := NewSQLiteVectorStoreHNSWEx(db, name, description, opts...)
	if err != nil {
		return nil, utils.Errorf("创建SQLite向量存储失败: %v", err)
	}
	ragSystem := NewRAGSystemWithName(name, store.embedder, store)
	if err != nil {
		return nil, utils.Errorf("创建集合失败: %v", err)
	}
	ragSystem.addDocuments(Document{
		ID:      DocumentTypeCollectionInfo,
		Content: fmt.Sprintf("collection_name: %s\ncollection_description: %s", name, description),
		Metadata: map[string]any{
			"collection_name": name,
			"collection_id":   store.collection.ID,
		},
		Embedding: nil,
	})
	return ragSystem, nil
}

var IsMockMode = false

func LoadCollection(db *gorm.DB, name string, opts ...any) (*RAGSystem, error) {
	log.Infof("start to load sqlite vector store for collection %#v", name)
	store, err := LoadSQLiteVectorStoreHNSW(db, name, opts...)
	if err != nil {
		return nil, utils.Errorf("load SQLite vector storage err: %v", err)
	}
	log.Infof("start to create RAG system for collection %#v", name)

	return NewRAGSystemWithName(name, store.embedder, store), nil
}

// CreateOrLoadCollection 创建或加载知识库
func CreateOrLoadCollection(db *gorm.DB, name string, description string, opts ...any) (*RAGSystem, error) {
	if CollectionIsExists(db, name) {
		log.Infof("using default local embedding service for collection '%s'", name)
		return LoadCollection(db, name, opts...)
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
	collectionNames, err := yakit.GetAllRAGCollectionNames(db)
	if err != nil {
		return []string{}
	}
	return collectionNames
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

	// LayerCount        int         // Layer数量
	// LayerNodeCountMap map[int]int // Layer节点数量
	// NodeCount         int         // 节点数量
	// MaxNeighbors      int         // 最大邻居数
	// MinNeighbors      int         // 最小邻居数
	// ConnectionCount   int         // 总连接数
}

// GetCollectionInfo 获取知识库信息
func GetCollectionInfo(db *gorm.DB, name string) (*CollectionInfo, error) {
	var collection schema.VectorStoreCollection
	dbErr := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", name).
		Select("name, description, model_name, dimension, m, ml, ef_search, ef_construct, distance_func_type").
		First(&collection)
	if dbErr.Error != nil {
		return nil, utils.Errorf("获取知识库信息失败: %v", dbErr.Error)
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
	}, nil
}

// AddDocument 添加文档
func AddDocument(db *gorm.DB, knowledgeBaseName, documentName string, document string, metadata map[string]any, opts ...any) error {
	ragSystem, err := LoadCollection(db, knowledgeBaseName, opts...)
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
	ragSystem, err := LoadCollection(db, knowledgeBaseName, opts...)
	if err != nil {
		return utils.Errorf("加载知识库失败: %v", err)
	}
	return ragSystem.DeleteDocuments(documentName)
}

// QueryDocuments 查询文档
func QueryDocuments(db *gorm.DB, knowledgeBaseName, query string, limit int, opts ...any) ([]SearchResult, error) {
	ragSystem, err := LoadCollection(db, knowledgeBaseName, opts...)
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

	config := NewCollectionConfig(i...)
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

func WithDocumentRuntimeID(runtimeID string) DocumentOption {
	return func(document *Document) {
		document.RuntimeID = runtimeID
	}
}
