package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils/cli"
)

type YakitPluginContext struct {
	PluginName    string
	PluginUUID    string
	RuntimeId     string
	Proxy         string
	Ctx           context.Context
	CliApp        *cli.CliApp
	Cancel        context.CancelFunc
	defaultFilter *filter.StringFilter
}

func (y *YakitPluginContext) WithContextCancel(cancel context.CancelFunc) *YakitPluginContext {
	y.Cancel = cancel
	return y
}

func (y *YakitPluginContext) WithPluginName(id string) *YakitPluginContext {
	y.PluginName = id
	return y
}

func (y *YakitPluginContext) WithProxy(proxy string) *YakitPluginContext {
	y.Proxy = proxy
	return y
}

func (y *YakitPluginContext) WithDefaultFilter(filter *filter.StringFilter) *YakitPluginContext {
	y.defaultFilter = filter
	return y
}

func (y *YakitPluginContext) WithContext(ctx context.Context) *YakitPluginContext {
	y.Ctx = ctx
	return y
}

func (y *YakitPluginContext) WithCliApp(cliApp *cli.CliApp) *YakitPluginContext {
	y.CliApp = cliApp
	return y
}

func CreateYakitPluginContext(runtimeId string) *YakitPluginContext {
	return &YakitPluginContext{
		RuntimeId: runtimeId,
	}
}
