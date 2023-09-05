package yaklib

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func yakitNewAliveHost(target string, opts ...yakit.AliveHostParamsOpt) {
	risk, _ := yakit.NewAliveHost(target, opts...)
	if risk != nil {
		yakitStatusCard("存活主机", fmt.Sprint(addCounter()))
		yakitOutputHelper(risk)
	}
}

func queryAliveHost(runtimeId string) (chan *yakit.AliveHost, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		return nil, utils.Errorf("cannot found database")
	}
	db = db.Model(&yakit.AliveHost{})
	db = bizhelper.ExactQueryString(db, "runtime_id", runtimeId)
	return yakit.YieldAliveHost(db, context.Background()), nil
}

var (
	AliveHostExports = map[string]interface{}{
		"NewAliveHost":   yakitNewAliveHost,
		"QueryAliveHost": queryAliveHost,
	}
)
