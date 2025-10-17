package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

// buildinForges can make basic coordinator use some forge. magic!!!
var buildinForges = new(sync.Map)

type AIDBuildinForgeExecutor func(c context.Context, params []*ypb.ExecParamItem, opts ...Option) (*aicommon.Action, error)

func RegisterAIDBuildinForge(forgeName string, fun AIDBuildinForgeExecutor) error {
	_, ok := buildinForges.Load(forgeName)
	if ok {
		return utils.Errorf("aid buildin forge %s already registered", forgeName)
	}
	buildinForges.Store(forgeName, fun)
	return nil
}

func UnregisterAIDBuildinForge(forgeName string) error {
	_, ok := buildinForges.Load(forgeName)
	if !ok {
		return utils.Errorf("aid buildin forge %s not registered", forgeName)
	}
	buildinForges.Delete(forgeName)
	return nil
}

func ExecuteAIForge(ctx context.Context, forgeName string, params []*ypb.ExecParamItem, opts ...Option) (*aicommon.Action, error) {
	fun, ok := buildinForges.Load(forgeName)
	if !ok {
		return nil, utils.Errorf("aid buildin forge %s not registered", forgeName)
	}
	if t, ok := fun.(AIDBuildinForgeExecutor); !ok {
		return nil, utils.Errorf("aid buildin forge %s not registered", forgeName)
	} else {
		result, err := t(ctx, params, opts...)
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

func IsAIDBuildInForgeExisted(forgeName string) bool {
	_, ok := buildinForges.Load(forgeName)
	return ok
}
