package yaklib

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func yakitNewAliveHost(target string, opts ...yakit.AliveHostParamsOpt) {
	risk, _ := yakit.NewAliveHost(target, opts...)
	if risk != nil {
		yakitStatusCard("存活主机", fmt.Sprint(addCounter()))
		yakitOutputHelper(risk)
	}
}

var (
	AliveHostExports = map[string]interface{}{
		"NewAliveHost": yakitNewAliveHost,
		"QueryAliveHost": func(runtimeId string) chan *yakit.AliveHost {
			return yakit.YieldAliveHostRuntimeId(consts.GetGormProjectDatabase(), context.Background(), runtimeId)
		},
	}
)
