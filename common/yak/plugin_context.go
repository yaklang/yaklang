package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/filter"
)

type YakitPluginContext struct {
	PluginName    string
	RuntimeId     string
	Proxy         string
	Ctx           context.Context
	defaultFilter *filter.StringFilter
}

func (y *YakitPluginContext) WithPluginName(id string) *YakitPluginContext {
	y.PluginName = id
	return y
}

func (y *YakitPluginContext) WithProxy(proxy string) *YakitPluginContext {
	y.Proxy = proxy
	return y
}

func (y *YakitPluginContext) WithScanTargetFilter(filter *filter.StringFilter) *YakitPluginContext {
	y.defaultFilter = filter
	return y
}

func (y *YakitPluginContext) WithContext(ctx context.Context) *YakitPluginContext {
	y.Ctx = ctx
	return y
}

var fallbackFilter = filter.NewFilter()

func CreateYakitPluginContext(runtimeId string) *YakitPluginContext {
	return &YakitPluginContext{
		RuntimeId:     runtimeId,
		defaultFilter: fallbackFilter,
	}
}
