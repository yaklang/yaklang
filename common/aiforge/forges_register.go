package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

type ForgeExecutor func(context.Context, []*ypb.ExecParamItem, ...aid.Option) (*ForgeResult, error)

var forgeMutex = new(sync.RWMutex)
var forges = make(map[string]ForgeExecutor)

type ForgeResult struct {
	*aid.Action
	Formated any
	Forge    *ForgeBlueprint
}

func RegisterForgeExecutor(i string, f ForgeExecutor) error {
	forgeMutex.Lock()
	if _, ok := forges[i]; ok {
		forgeMutex.Unlock()
		return utils.Errorf("forge %s already registered", i)
	}
	forges[i] = f
	forgeMutex.Unlock()
	return nil
}

func ExecuteForge(
	forgeName string,
	ctx context.Context,
	params []*ypb.ExecParamItem,
	opts ...aid.Option,
) (*ForgeResult, error) {
	forgeMutex.RLock()
	defer forgeMutex.RUnlock()

	if forge, ok := forges[forgeName]; ok {
		return forge(ctx, params, opts...)
	} else {
		return nil, utils.Errorf("forge %s not found", forgeName)
	}
}
