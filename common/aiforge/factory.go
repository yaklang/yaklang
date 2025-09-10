package aiforge

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ForgeFactory 负责基于 schema.AIForge 的检索与执行
type ForgeFactory struct{}

// ForgeQueryOption 定义 Query 的可选项
type ForgeQueryOption func(*ypb.AIForgeFilter, *ypb.Paging)

// WithForgeFilter_Keyword 设置关键词搜索
func WithForgeFilter_Keyword(keyword string) ForgeQueryOption {
	return func(filter *ypb.AIForgeFilter, _ *ypb.Paging) {
		filter.Keyword = keyword
	}
}

// WithForgeFilter_Limit 设置返回条数限制
func WithForgeFilter_Limit(limit int) ForgeQueryOption {
	return func(_ *ypb.AIForgeFilter, paging *ypb.Paging) {
		if limit > 0 {
			paging.Limit = int64(limit)
		}
	}
}

// Query 从 Profile 数据库中查询 schema.AIForge 列表
func (ForgeFactory) Query(ctx context.Context, opts ...ForgeQueryOption) ([]*schema.AIForge, error) {
	_ = ctx
	var (
		db     *gorm.DB = consts.GetGormProfileDatabase()
		filter          = &ypb.AIForgeFilter{}
		paging          = &ypb.Paging{Page: 1, Limit: 20}
	)

	for _, opt := range opts {
		opt(filter, paging)
	}

	log.Debugf("ForgeFactory.Query: keyword=%q limit=%d", filter.GetKeyword(), paging.GetLimit())

	_, data, err := yakit.QueryAIForge(db, filter, paging)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (f *ForgeFactory) GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error) {
	result, err := utils.RenderTemplate(`<|AI_BLUEPRINT_{{ .nonce }}_START|>`+
		`{{range .forges}}
* '{{ .ForgeName }}': {{ .Description }}{{ if .ForgeVerboseName }}(Short: {{ .ForgeVerboseName }}){{end}}{{end}}
<|AI_BLUEPRINT_{{ .nonce }}_END|>`, map[string]any{
		"forges": forges,
		"nonce":  utils.RandStringBytes(4),
	})
	return result, err
}

// Execute 透明转发到内置 ExecuteForge
func (ForgeFactory) Execute(ctx context.Context, forgeName string, params []*ypb.ExecParamItem, opts ...aid.Option) (*ForgeResult, error) {
	return ExecuteForge(forgeName, ctx, params, opts...)
}
