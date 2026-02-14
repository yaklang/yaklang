package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

var registerMutex = new(sync.Mutex)

type ForgeResult struct {
	*Action

	Name string
}

type LiteForgeExecuteCallback func(prompt string, opts ...any) (*ForgeResult, error)

var liteforgeExecuteFunc LiteForgeExecuteCallback

func RegisterLiteForgeExecuteCallback(f LiteForgeExecuteCallback) {
	registerMutex.Lock()
	defer registerMutex.Unlock()

	liteforgeExecuteFunc = f
}

func InvokeLiteForge(prompt string, opts ...any) (*ForgeResult, error) {
	if liteforgeExecuteFunc == nil {
		return nil, utils.Error("liteforge execute callback is not registered, check if `common/aiforge` is imported.")
	}
	return liteforgeExecuteFunc(prompt, opts...)
}
