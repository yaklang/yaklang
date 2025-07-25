package schema

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// Float32Array 用于存储 float32 数组 (HNSW使用float32向量)
type Float32Array []float32

func (f Float32Array) Value() (driver.Value, error) {
	if f == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(f)
	return string(bytes), err
}

func (f *Float32Array) Scan(value interface{}) error {
	if value == nil {
		*f = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}
	return json.Unmarshal(bytes, f)
}

// IntArray 用于存储整数数组
type IntArray []int

func (i IntArray) Value() (driver.Value, error) {
	if i == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(i)
	return string(bytes), err
}

func (i *IntArray) Scan(value interface{}) error {
	if value == nil {
		*i = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}
	return json.Unmarshal(bytes, i)
}

// StringArray 用于存储字符串数组
type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(s)
	return string(bytes), err
}

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return utils.Errorf("不支持的类型: %T", value)
	}
	return json.Unmarshal(bytes, s)
}

// ===================== HNSW 集合表 =====================

// HNSWCollection HNSW向量集合表
type HNSWCollection struct {
	gorm.Model

	// 基本信息
	Name        string `gorm:"unique_index;index:idx_hnsw_collection_name" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	ModelName   string `json:"model_name"`
	Dimension   int    `json:"dimension"`

	// HNSW 算法参数
	M            int     `json:"m" gorm:"default:16"`                   // 最大邻居数
	Ml           float64 `json:"ml" gorm:"default:0.25"`                // 层生成因子
	EfSearch     int     `json:"ef_search" gorm:"default:20"`           // 搜索时候选节点数
	EfConstruct  int     `json:"ef_construct" gorm:"default:200"`       // 构建时候选节点数
	DistanceFunc string  `json:"distance_func" gorm:"default:'cosine'"` // 距离函数类型

	// 统计信息
	NodeCount  int       `json:"node_count" gorm:"default:0"`  // 节点总数
	LayerCount int       `json:"layer_count" gorm:"default:0"` // 层数
	LastUpdate time.Time `json:"last_update"`                  // 最后更新时间

	// 索引状态
	IndexStatus string `json:"index_status" gorm:"default:'building'"` // building, ready, error
	IndexError  string `gorm:"type:text" json:"index_error,omitempty"` // 索引错误信息

	// 性能配置
	EnableCompaction    bool `json:"enable_compaction" gorm:"default:true"`    // 是否启用压缩
	CompactionThreshold int  `json:"compaction_threshold" gorm:"default:1000"` // 压缩阈值
}

// ===================== HNSW 节点表 =====================

// HNSWNode HNSW图中的节点
type HNSWNode struct {
	gorm.Model

	// 关联信息
	CollectionID uint   `gorm:"index:idx_hnsw_node_collection" json:"collection_id"`
	NodeKey      string `gorm:"index:idx_hnsw_node_key" json:"node_key"`         // 节点唯一标识
	DocumentID   string `gorm:"index:idx_hnsw_node_document" json:"document_id"` // 关联的文档ID

	// 向量数据
	Vector Float32Array `gorm:"type:text" json:"vector"` // 节点向量

	// 层级信息
	MaxLayer int `json:"max_layer"` // 节点存在的最高层级

	// 元数据
	Metadata MetadataMap `gorm:"type:text" json:"metadata,omitempty"`

	// 唯一约束：集合内节点Key唯一
	// 联合索引：集合ID + 节点Key
}

// ===================== HNSW 层级表 =====================

// HNSWLayer HNSW图的层级信息
type HNSWLayer struct {
	gorm.Model

	// 关联信息
	CollectionID uint `gorm:"index:idx_hnsw_layer_collection" json:"collection_id"`
	LayerLevel   int  `gorm:"index:idx_hnsw_layer_level" json:"layer_level"` // 层级编号

	// 层级统计
	NodeCount    int    `json:"node_count"`               // 该层节点数量
	EntryNodeKey string `json:"entry_node_key,omitempty"` // 入口节点Key

	// 连接统计
	AvgConnections float64 `json:"avg_connections"` // 平均连接数
	MaxConnections int     `json:"max_connections"` // 最大连接数
	MinConnections int     `json:"min_connections"` // 最小连接数

	// 唯一约束：集合内层级唯一
}

// ===================== HNSW 邻居连接表 =====================

// HNSWConnection HNSW图中的邻居连接关系
type HNSWConnection struct {
	gorm.Model

	// 关联信息
	CollectionID uint `gorm:"index:idx_hnsw_conn_collection" json:"collection_id"`
	LayerLevel   int  `gorm:"index:idx_hnsw_conn_layer" json:"layer_level"`

	// 连接信息
	FromNodeKey string  `gorm:"index:idx_hnsw_conn_from" json:"from_node_key"` // 源节点
	ToNodeKey   string  `gorm:"index:idx_hnsw_conn_to" json:"to_node_key"`     // 目标节点
	Distance    float32 `json:"distance"`                                      // 节点间距离

	// 连接属性
	ConnectionType string  `json:"connection_type" gorm:"default:'bidirectional'"` // bidirectional, unidirectional
	Weight         float32 `json:"weight" gorm:"default:1.0"`                      // 连接权重
	IsActive       bool    `json:"is_active" gorm:"default:true"`                  // 连接是否激活

	// 联合唯一索引：防止重复连接
}

// ===================== HNSW 文档表 =====================

// HNSWDocument HNSW系统中的文档
type HNSWDocument struct {
	gorm.Model

	// 基本信息
	DocumentID   string `gorm:"unique_index" json:"document_id"`
	CollectionID uint   `gorm:"index:idx_hnsw_doc_collection" json:"collection_id"`

	// 文档内容
	Content  string      `gorm:"type:text" json:"content"`
	Title    string      `json:"title,omitempty"`
	Metadata MetadataMap `gorm:"type:text" json:"metadata,omitempty"`

	// 向量信息
	Vector  Float32Array `gorm:"type:text" json:"vector"`
	NodeKey string       `gorm:"index:idx_hnsw_doc_node" json:"node_key"` // 关联的HNSW节点

	// 处理状态
	ProcessStatus string    `json:"process_status" gorm:"default:'pending'"` // pending, processed, error
	ProcessError  string    `gorm:"type:text" json:"process_error,omitempty"`
	ProcessedAt   time.Time `json:"processed_at,omitempty"`

	// 统计信息
	AccessCount int       `json:"access_count" gorm:"default:0"`
	LastAccess  time.Time `json:"last_access,omitempty"`
}

// ===================== HNSW 搜索日志表 =====================

// HNSWSearchLog HNSW搜索操作日志
type HNSWSearchLog struct {
	gorm.Model

	// 搜索信息
	CollectionID uint         `gorm:"index:idx_hnsw_search_collection" json:"collection_id"`
	QueryVector  Float32Array `gorm:"type:text" json:"query_vector"`
	QueryText    string       `gorm:"type:text" json:"query_text,omitempty"`
	K            int          `json:"k"` // 请求返回的结果数量

	// 搜索参数
	EfSearch   int    `json:"ef_search"`
	SearchMode string `json:"search_mode" gorm:"default:'standard'"` // standard, accurate, fast

	// 搜索结果
	ResultNodeKeys StringArray  `gorm:"type:text" json:"result_node_keys"`
	ResultScores   Float32Array `gorm:"type:text" json:"result_scores"`
	ResultCount    int          `json:"result_count"`

	// 性能指标
	SearchDuration  int64 `json:"search_duration_ns"` // 搜索耗时（纳秒）
	NodesVisited    int   `json:"nodes_visited"`      // 访问的节点数
	LayersTraversed int   `json:"layers_traversed"`   // 遍历的层数
	CacheHits       int   `json:"cache_hits"`         // 缓存命中数

	// 质量指标
	RecallScore    float32 `json:"recall_score,omitempty"`    // 召回率
	PrecisionScore float32 `json:"precision_score,omitempty"` // 精确率

	// 请求信息
	ClientID  string `json:"client_id,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
}

// ===================== HNSW 性能监控表 =====================

// HNSWPerformanceMetric HNSW性能指标记录
type HNSWPerformanceMetric struct {
	gorm.Model

	// 监控目标
	CollectionID uint   `gorm:"index:idx_hnsw_perf_collection" json:"collection_id"`
	MetricType   string `gorm:"index:idx_hnsw_perf_type" json:"metric_type"` // search, insert, delete, rebuild

	// 时间信息
	Timestamp  time.Time `gorm:"index:idx_hnsw_perf_time" json:"timestamp"`
	TimeWindow string    `json:"time_window"` // 1m, 5m, 1h, 1d

	// 性能指标
	OperationCount int64   `json:"operation_count"` // 操作次数
	AvgDuration    float64 `json:"avg_duration_ms"` // 平均耗时(毫秒)
	MaxDuration    float64 `json:"max_duration_ms"` // 最大耗时(毫秒)
	MinDuration    float64 `json:"min_duration_ms"` // 最小耗时(毫秒)
	P95Duration    float64 `json:"p95_duration_ms"` // P95耗时(毫秒)
	P99Duration    float64 `json:"p99_duration_ms"` // P99耗时(毫秒)

	// 资源使用
	MemoryUsage int64   `json:"memory_usage_bytes"` // 内存使用(字节)
	CPUUsage    float64 `json:"cpu_usage_percent"`  // CPU使用率(%)

	// 质量指标
	SuccessRate float64 `json:"success_rate"`         // 成功率
	ErrorRate   float64 `json:"error_rate"`           // 错误率
	AvgRecall   float64 `json:"avg_recall,omitempty"` // 平均召回率
}

// ===================== 索引创建函数 =====================

func CreateHNSWIndexes(db *gorm.DB) error {
	// HNSWNode 索引
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_hnsw_node_unique ON hnsw_nodes(collection_id, node_key)")

	// HNSWConnection 索引
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_hnsw_conn_unique ON hnsw_connections(collection_id, layer_level, from_node_key, to_node_key)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_hnsw_conn_from_layer ON hnsw_connections(from_node_key, layer_level)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_hnsw_conn_to_layer ON hnsw_connections(to_node_key, layer_level)")

	// HNSWLayer 索引
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_hnsw_layer_unique ON hnsw_layers(collection_id, layer_level)")

	// HNSWSearchLog 索引
	db.Exec("CREATE INDEX IF NOT EXISTS idx_hnsw_search_time ON hnsw_search_logs(created_at)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_hnsw_search_duration ON hnsw_search_logs(search_duration_ns)")

	// HNSWPerformanceMetric 索引
	db.Exec("CREATE INDEX IF NOT EXISTS idx_hnsw_perf_time_type ON hnsw_performance_metrics(timestamp, metric_type)")

	return nil
}

// ===================== 数据库表注册 =====================

func init() {
	// 注册HNSW相关表到profile数据库
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE,
		&HNSWCollection{},
		&HNSWNode{},
		&HNSWLayer{},
		&HNSWConnection{},
		&HNSWDocument{},
		&HNSWSearchLog{},
		&HNSWPerformanceMetric{},
	)
}
