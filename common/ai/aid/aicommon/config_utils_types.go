package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type MemoryTriage interface {
	// SetInvoker 设置AI调用运行时
	SetInvoker(invoker AIInvokeRuntime)

	// AddRawText 添加原始文本，返回提取的记忆实体
	AddRawText(text string) ([]*MemoryEntity, error)

	// SaveMemoryEntities 保存记忆条目到数据库
	SaveMemoryEntities(entities ...*MemoryEntity) error

	SearchBySemantics(query string, limit int) ([]*SearchResult, error)

	SearchByTags(tags []string, matchAll bool, limit int) ([]*MemoryEntity, error)

	// HandleMemory 智能处理输入内容，自动构造记忆、去重并保存
	HandleMemory(i any) error

	// SearchMemory 根据输入内容搜索相关记忆，限制总内容字节数
	SearchMemory(origin any, bytesLimit int) (*SearchMemoryResult, error)

	// SearchMemoryWithoutAI 不使用AI的关键词搜索，直接基于关键词匹配
	SearchMemoryWithoutAI(origin any, bytesLimit int) (*SearchMemoryResult, error)

	Close() error
}

type ForgeQueryConfig struct {
	Filter *ypb.AIForgeFilter
	Paging *ypb.Paging
}

type ForgeQueryOption func(config *ForgeQueryConfig)

func WithForgeQueryFilter(filter *ypb.AIForgeFilter) ForgeQueryOption {
	return func(config *ForgeQueryConfig) {
		config.Filter = filter
	}
}

func WithForgeQueryPaging(paging *ypb.Paging) ForgeQueryOption {
	return func(config *ForgeQueryConfig) {
		config.Paging = paging
	}
}

// WithForgeFilter_Keyword 设置关键词搜索
func WithForgeFilter_Keyword(keyword string) ForgeQueryOption {
	return func(config *ForgeQueryConfig) {
		config.Filter.Keyword = keyword
	}
}

// WithForgeFilter_Limit 设置返回条数限制
func WithForgeFilter_Limit(limit int) ForgeQueryOption {
	return func(config *ForgeQueryConfig) {
		if limit > 0 {
			config.Paging.Limit = int64(limit)
		}
	}
}

func NewForgeQueryConfig(opts ...ForgeQueryOption) *ForgeQueryConfig {
	config := &ForgeQueryConfig{
		Filter: &ypb.AIForgeFilter{},
		Paging: &ypb.Paging{
			Limit: 50,
			Page:  1,
		},
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

type AIForgeFactory interface {
	Query(ctx context.Context, opts ...ForgeQueryOption) ([]*schema.AIForge, error)
	GetAIForge(name string) (*schema.AIForge, error)
	GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error)
	GenerateAIJSONSchemaFromSchemaAIForge(forge *schema.AIForge) (string, error)
}
