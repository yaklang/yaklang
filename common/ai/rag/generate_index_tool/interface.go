package generate_index_tool

import "context"

// IndexableItem 可索引的数据项接口
type IndexableItem interface {
	// GetKey 获取数据的唯一标识符
	GetKey() string

	// GetContent 获取用于生成向量的内容
	GetContent() (string, error)

	// GetMetadata 获取元数据信息
	GetMetadata() map[string]interface{}

	// GetDisplayName 获取显示名称（用于日志等）
	GetDisplayName() string
}

var _ IndexableItem = (*CommonIndexableItem)(nil)

type CommonIndexableItem struct {
	Key         string
	Content     string
	Metadata    map[string]interface{}
	DisplayName string
}

func (c *CommonIndexableItem) GetKey() string {
	return c.Key
}

func (c *CommonIndexableItem) GetContent() (string, error) {
	return c.Content, nil
}

func (c *CommonIndexableItem) GetMetadata() map[string]interface{} {
	return c.Metadata
}

func (c *CommonIndexableItem) GetDisplayName() string {
	return c.DisplayName
}

func NewCommonIndexableItem(key, content string, metadata map[string]interface{}, displayName string) *CommonIndexableItem {
	return &CommonIndexableItem{
		Key:         key,
		Content:     content,
		Metadata:    metadata,
		DisplayName: displayName,
	}
}

// ContentProcessor 内容处理器接口
type ContentProcessor interface {
	// ProcessContent 处理原始内容，返回清洗后的内容
	ProcessContent(ctx context.Context, rawContent string) (string, error)
}

// CacheManager 缓存管理器接口
type CacheManager interface {
	// LoadRawCache 加载原始内容缓存
	LoadRawCache() (map[string]string, error)

	// SaveRawCache 保存原始内容缓存
	SaveRawCache(cache map[string]string) error

	// LoadProcessedCache 加载处理后内容缓存
	LoadProcessedCache() (map[string]string, error)

	// SaveProcessedCache 保存处理后内容缓存
	SaveProcessedCache(cache map[string]string) error

	// Clear 清空所有缓存
	Clear() error
}

// ProgressCallback 进度回调函数
type ProgressCallback func(current, total int, message string)

// IndexOptions 索引选项
type IndexOptions struct {
	// 缓存目录
	CacheDir string

	// 是否强制绕过缓存
	ForceBypassCache bool

	// 是否包含元数据
	IncludeMetadata bool

	// 批处理大小
	BatchSize int

	// 进度回调
	ProgressCallback ProgressCallback

	// 内容处理器
	ContentProcessor ContentProcessor

	// 缓存管理器
	CacheManager CacheManager

	// 并发数
	ConcurrentWorkers int
}

// IndexResult 索引结果
type IndexResult struct {
	// 成功索引的数量
	SuccessCount int

	// 失败的项目
	FailedItems []FailedItem

	// 跳过的项目（已存在且未强制更新）
	SkippedCount int

	// 总耗时
	Duration string
}

// FailedItem 失败的项目
type FailedItem struct {
	Key   string
	Error string
}
