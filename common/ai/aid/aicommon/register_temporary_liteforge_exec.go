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

// LiteForgeStaticInstruction 是 B 档新增的标记类型，通过 InvokeLiteForge 的 opts 携带系统侧静态指令
// aiforge 端在 _executeLiteForgeTemp 的 type switch 中识别该类型并赋值给 cfg.staticInstruction
// 该机制避免了下游包（如 enhancesearch）反向 import aiforge 造成的循环依赖
// 关键词: aicache, PROMPT_SECTION, StaticInstruction, LiteForgeStaticInstruction, B 档无循环依赖
type LiteForgeStaticInstruction string
