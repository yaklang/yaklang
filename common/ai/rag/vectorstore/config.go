package vectorstore

import (
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
	DisableEmbedCollectionInfo bool
	LazyLoadEmbeddingClient    bool

	buildGraphFilter *yakit.VectorDocumentFilter
	buildGraphPolicy string

	// otherOptions []any

	DB *gorm.DB

	MaxChunkSize int
	Overlap      int
	BigTextPlan  string

	CacheSize    int
	PreCacheSize int

	KeyAsUID bool

	TryRebuildHNSWIndex bool
}

func NewCollectionConfig(options ...CollectionConfigFunc) *CollectionConfig {
	defaultConfig := &CollectionConfig{
		ModelName:                  "Qwen3-Embedding-0.6B-Q4_K_M",
		Dimension:                  1024,
		DistanceFuncType:           "cosine",
		MaxNeighbors:               16,
		LayerGenerationFactor:      0.25,
		EfSearch:                   20,
		EfConstruct:                200,
		EnableAutoUpdateGraphInfos: true,
		MaxChunkSize:               defaultMaxChunkSize,
		Overlap:                    defaultChunkOverlap,
		BigTextPlan:                defaultBigTextPlan,
		KeyAsUID:                   false,
		TryRebuildHNSWIndex:        false,
	}

	for _, option := range options {
		option(defaultConfig)
	}
	if defaultConfig.DB == nil {
		defaultConfig.DB = consts.GetGormProfileDatabase()
	}
	return defaultConfig
}

func LoadConfigFromCollectionInfo(collection *schema.VectorStoreCollection, options ...CollectionConfigFunc) *CollectionConfig {
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
		MaxChunkSize:               defaultMaxChunkSize,
		Overlap:                    defaultChunkOverlap,
		BigTextPlan:                defaultBigTextPlan,
		CacheSize:                  10000,
		PreCacheSize:               0,
		KeyAsUID:                   false,
	}
	for _, option := range options {
		option(loadBasicConfig)
	}
	return loadBasicConfig
}

func (c *CollectionConfig) FixEmbeddingClient() error {
	if c.EmbeddingClient != nil {
		return nil
	}
	if c.LazyLoadEmbeddingClient {
		return nil
	}
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
		// 优先尝试使用 AIBalance 免费服务（如果可用）
		aibalanceEmbedder, err := GetAIBalanceFreeEmbeddingService()
		if err == nil && aibalanceEmbedder != nil {
			// 检查模型兼容性
			if c.ModelName != "" && !IsCompatibleEmbeddingModel(c.ModelName, aibalanceEmbedder.GetModelName()) {
				log.Warnf("collection model '%s' is not compatible with AIBalance free model '%s', falling back to local service",
					c.ModelName, aibalanceEmbedder.GetModelName())
			} else {
				log.OnceInfoLog("using AIBalance free embedding service (normalized model: %s, dimension: %d)",
					aibalanceEmbedder.GetModelName(), aibalanceEmbedder.GetModelDimension())
				c.EmbeddingClient = aibalanceEmbedder
				// 更新模型名称为归一化名称（如果为空或兼容）
				if c.ModelName == "" || IsCompatibleEmbeddingModel(c.ModelName, aibalanceEmbedder.GetModelName()) {
					c.ModelName = aibalanceEmbedder.GetModelName()
				}
				return nil
			}
		} else {
			log.Infof("AIBalance free embedding service not available: %v, trying local service", err)
		}

		// 回退到本地嵌入服务
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

type CollectionConfigFunc func(config *CollectionConfig)

func WithKeyAsUID(keyAsUID bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.KeyAsUID = keyAsUID
	}
}

func WithTryRebuildHNSWIndex(tryRebuildHNSWIndex bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.TryRebuildHNSWIndex = tryRebuildHNSWIndex
	}
}

func WithCacheSize(cacheSize int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.CacheSize = cacheSize
	}
}

func WithPreCacheSize(preCacheSize int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.PreCacheSize = preCacheSize
	}
}

func WithMaxChunkSize(maxChunkSize int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.MaxChunkSize = maxChunkSize
	}
}

func WithOverlap(overlap int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.Overlap = overlap
	}
}

func WithBigTextPlan(bigTextPlan string) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.BigTextPlan = bigTextPlan
	}
}

// WithEmbeddingClient 设置embedding客户端
func WithEmbeddingClient(client aispec.EmbeddingCaller) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.EmbeddingClient = client
	}
}

func WithLazyLoadEmbeddingClient() CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.LazyLoadEmbeddingClient = true
	}
}

func WithDescription(description string) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.Description = description
	}
}

func WithForceNew(i ...bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		if len(i) > 0 {
			config.ForceNew = i[0]
		} else {
			config.ForceNew = true
		}
	}
}

// WithEmbeddingModel 设置embedding模型
func WithEmbeddingModel(model string) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.ModelName = model
	}
}

// WithModelDimension 设置模型维度
func WithModelDimension(dimension int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.Dimension = dimension
	}
}

func WithModelName(name string) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.ModelName = name
	}
}

func WithBuildGraphFilter(filter *yakit.VectorDocumentFilter) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.buildGraphFilter = filter
	}
}

func WithBuildGraphPolicy(policy string) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.buildGraphPolicy = policy
	}
}

func WithCosineDistance() CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.DistanceFuncType = "cosine"
	}
}

func WithEnablePQ(enable bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.EnablePQ = enable
	}
}

func WithEnableAutoUpdateGraphInfos(enable bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.EnableAutoUpdateGraphInfos = enable
	}
}

func WithDisableEmbedCollectionInfo(enable bool) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.DisableEmbedCollectionInfo = enable
	}
}

// WithDB 设置数据库
func WithDB(db *gorm.DB) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.DB = db
	}
}

// WithHNSWParameters 批量设置HNSW参数
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) CollectionConfigFunc {
	return func(config *CollectionConfig) {
		config.MaxNeighbors = m
		config.LayerGenerationFactor = ml
		config.EfSearch = efSearch
		config.EfConstruct = efConstruct
	}
}

var IsMockMode = false

// DeleteCollection 删除知识库
func DeleteCollection(db *gorm.DB, name string) error {
	return yakit.DeleteRAGCollection(db, name)
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
