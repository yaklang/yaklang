package config

import (
	"context"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/utils"
)

// SQLiteVectorStoreHNSWConfig 定义了 SQLite HNSW 向量存储的配置参数
type SQLiteVectorStoreHNSWConfig struct {
	// HNSW 算法参数配置
	M                int     `json:"m"`                  // 最大邻居数，影响图的连接密度
	Ml               float64 `json:"ml"`                 // 层生成因子，控制层级分布
	EfSearch         int     `json:"ef_search"`          // 搜索时的候选节点数
	EfConstruct      int     `json:"ef_construct"`       // 构建时的候选节点数
	DistanceFuncType string  `json:"distance_func_type"` // 距离函数类型（cosine、euclidean等）

	EnablePQMode bool `json:"enable_pq_mode"`

	CacheSize        int             `json:"cache_size"`
	Ctx              context.Context `json:"ctx"`
	BuildGraphPolicy string
}

// SQLiteVectorStoreHNSWOption 定义配置选项函数类型
type SQLiteVectorStoreHNSWOption func(config *SQLiteVectorStoreHNSWConfig)

const (
	Policy_UseDBCanche = "DB_Cache"
	Policy_UseFilter   = "Filter"
	Policy_None        = "None"
)

// NewSQLiteVectorStoreHNSWConfig 返回默认配置
func NewSQLiteVectorStoreHNSWConfig() *SQLiteVectorStoreHNSWConfig {
	return &SQLiteVectorStoreHNSWConfig{
		M:                16,       // 最大邻居数
		Ml:               0.25,     // 层生成因子
		EfSearch:         20,       // 搜索时的候选节点数
		EfConstruct:      200,      // 构建时的候选节点数
		DistanceFuncType: "cosine", // 默认使用余弦距离
		EnablePQMode:     false,    // 默认不启用PQ模式
		CacheSize:        10000,    // 默认缓存10000个节点
		BuildGraphPolicy: Policy_UseDBCanche,
		Ctx:              context.Background(),
	}
}

// ValidateConfig 验证配置参数的有效性
func (c *SQLiteVectorStoreHNSWConfig) ValidateConfig() error {
	if c.M <= 0 {
		return utils.Errorf("最大邻居数必须大于0，当前值: %d", c.M)
	}
	if c.Ml <= 0 || c.Ml > 1 {
		return utils.Errorf("层生成因子必须在0到1之间，当前值: %f", c.Ml)
	}
	if c.EfSearch <= 0 {
		return utils.Errorf("搜索候选节点数必须大于0，当前值: %d", c.EfSearch)
	}
	if c.EfConstruct <= 0 {
		return utils.Errorf("构建候选节点数必须大于0，当前值: %d", c.EfConstruct)
	}
	if c.DistanceFuncType == "" {
		return utils.Errorf("距离函数类型不能为空")
	}

	// 验证距离函数类型
	validDistanceTypes := map[string]bool{
		"cosine":    true,
		"euclidean": true,
		"manhattan": true,
		"dot":       true,
	}
	if !validDistanceTypes[c.DistanceFuncType] {
		return utils.Errorf("不支持的距离函数类型: %s，支持的类型: cosine, euclidean, manhattan, dot", c.DistanceFuncType)
	}

	return nil
}

// WithEnablePQMode 设置是否启用PQ模式
func WithEnablePQMode(enablePQMode bool) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.EnablePQMode = enablePQMode
	}
}

// WithMaxNeighbors 设置最大邻居数 (M 参数)
func WithMaxNeighbors(m int) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.M = m
	}
}

// WithLayerGenerationFactor 设置层生成因子 (Ml 参数)
func WithLayerGenerationFactor(ml float64) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.Ml = ml
	}
}

// WithEfSearch 设置搜索时的候选节点数
func WithEfSearch(efSearch int) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.EfSearch = efSearch
	}
}

// WithEfConstruct 设置构建时的候选节点数
func WithEfConstruct(efConstruct int) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.EfConstruct = efConstruct
	}
}

// WithDistanceFunction 设置距离函数类型
func WithDistanceFunction(distanceFuncType string) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.DistanceFuncType = distanceFuncType
	}
}

// WithCosineDistance 设置使用余弦距离
func WithCosineDistance() SQLiteVectorStoreHNSWOption {
	return WithDistanceFunction("cosine")
}

// WithEuclideanDistance 设置使用欧几里得距离
func WithEuclideanDistance() SQLiteVectorStoreHNSWOption {
	return WithDistanceFunction("euclidean")
}

// WithManhattanDistance 设置使用曼哈顿距离
func WithManhattanDistance() SQLiteVectorStoreHNSWOption {
	return WithDistanceFunction("manhattan")
}

// WithDotDistance 设置使用点积距离
func WithDotDistance() SQLiteVectorStoreHNSWOption {
	return WithDistanceFunction("dot")
}

// WithPQMode 设置使用PQ模式
func WithPQMode() SQLiteVectorStoreHNSWOption {
	return WithEnablePQMode(true)
}

// WithCtx 设置上下文
func WithCtx(ctx context.Context) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.Ctx = ctx
	}
}

// WithBuildGraphPolicy 设置构建图策略
func WithBuildGraphPolicy(policy string) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.BuildGraphPolicy = policy
	}
}

// WithHNSWParameters 批量设置 HNSW 参数
func WithHNSWParameters(m int, ml float64, efSearch, efConstruct int) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.M = m
		config.Ml = ml
		config.EfSearch = efSearch
		config.EfConstruct = efConstruct
	}
}

// WithCacheSize 设置缓存大小
func WithCacheSize(size int) SQLiteVectorStoreHNSWOption {
	return func(config *SQLiteVectorStoreHNSWConfig) {
		config.CacheSize = size
	}
}

// ApplyOptions 应用配置选项到配置对象
func (c *SQLiteVectorStoreHNSWConfig) ApplyOptions(options ...SQLiteVectorStoreHNSWOption) {
	for _, option := range options {
		option(c)
	}
}
