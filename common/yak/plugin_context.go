package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type YakitPluginContext struct {
	yakit.YakitPluginInfo
	Proxy       string
	Ctx         context.Context
	CliApp      *cli.CliApp
	Cancel      context.CancelFunc
	vulFilter   filter.Filterable
	YakitClient *yaklib.YakitClient
}

func (y *YakitPluginContext) WithContextCancel(cancel context.CancelFunc) *YakitPluginContext {
	y.Cancel = cancel
	return y
}

func (y *YakitPluginContext) WithYakitClient(yakitClient *yaklib.YakitClient) *YakitPluginContext {
	if yakitClient == nil {
		return y
	}
	y.YakitClient = yakitClient
	return y
}

func (y *YakitPluginContext) WithPluginName(id string) *YakitPluginContext {
	y.PluginName = id
	return y
}

func (y *YakitPluginContext) WithPluginUUID(uuid string) *YakitPluginContext {
	y.PluginUUID = uuid
	return y
}

func (y *YakitPluginContext) WithProxy(proxy string) *YakitPluginContext {
	y.Proxy = proxy
	return y
}

func (y *YakitPluginContext) WithVulFilter(filter filter.Filterable) *YakitPluginContext {
	y.vulFilter = filter
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
	y := &YakitPluginContext{}
	y.RuntimeId = runtimeId
	return y
}
