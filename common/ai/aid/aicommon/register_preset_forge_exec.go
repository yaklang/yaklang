package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

type RegisteredForgeExecutor func(name string, ctx context.Context, params any, opts ...ConfigOption) (*ForgeResult, error)

var execRegisteredForgeCallback RegisteredForgeExecutor
var execForgeYakEngineCallback func(context.Context, string, any, ...any) (any, error)

func RegisterForgeYakEngineCallback(f func(forgeName string, i any, iopts ...any) (any, error)) {
	if f == nil {
		RegisterForgeYakEngineCallbackWithContext(nil)
		return
	}
	RegisterForgeYakEngineCallbackWithContext(func(_ context.Context, name string, input any, opts ...any) (any, error) {
		return f(name, input, opts...)
	})
}

// RegisterForgeYakEngineCallbackWithContext carries the invoking runtime's
// context across the Yak gateway. A ConfigOption alone cannot set Agent.ctx,
// which is used to construct the Forge and must retain parent runtime values.
func RegisterForgeYakEngineCallbackWithContext(f func(context.Context, string, any, ...any) (any, error)) {
	registerMutex.Lock()
	defer registerMutex.Unlock()
	execForgeYakEngineCallback = f
}

func RegisterPresetForgeExecuteCallback(f RegisteredForgeExecutor) {
	registerMutex.Lock()
	defer registerMutex.Unlock()

	execRegisteredForgeCallback = f
}

func ExecuteRegisteredForge(name string, ctx context.Context, params any, opts ...ConfigOption) (*ForgeResult, error) {
	if execRegisteredForgeCallback == nil {
		return nil, utils.Error("registered forge execute callback is not registered, check if `common/ai/aiforge` imported???")
	}
	return execRegisteredForgeCallback(name, ctx, params, opts...)
}

func ExecuteForgeFromDB(forgeName string, ctx context.Context, params any, opts ...ConfigOption) (any, error) {
	if execForgeYakEngineCallback == nil {
		return nil, utils.Error("registered forge execute callback is not registered, check if `common/yak` imported???")
	}
	newOpts := []any{
		WithContext(ctx),
	}
	for _, opt := range opts {
		newOpts = append(newOpts, opt)
	}

	return execForgeYakEngineCallback(ctx, forgeName, params, newOpts...)
}
