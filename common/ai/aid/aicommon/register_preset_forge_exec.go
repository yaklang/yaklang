package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

type RegisteredForgeExecutor func(name string, ctx context.Context, params any, opts ...ConfigOption) (*ForgeResult, error)

var execRegisteredForgeCallback RegisteredForgeExecutor

func RegisterPresetForgeExecuteCallback(f RegisteredForgeExecutor) {
	registerMutex.Lock()
	defer registerMutex.Unlock()

	execRegisteredForgeCallback = f
}

func ExecuteRegisteredForge(name string, ctx context.Context, params any, opts ...ConfigOption) (*ForgeResult, error) {
	if execRegisteredForgeCallback == nil {
		return nil, utils.Error("registered forge execute callback is not registered, check if `common/aiforge` imported???")
	}
	return execRegisteredForgeCallback(name, ctx, params, opts...)
}
