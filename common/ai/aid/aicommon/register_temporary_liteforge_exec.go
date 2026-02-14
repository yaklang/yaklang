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

// InvokeLiteForgeSpeedPriority invokes LiteForge with speed-priority (lightweight) AI model.
// This is a convenience wrapper around InvokeLiteForge that automatically applies WithLiteForgeSpeedFirst().
// The speed priority option is appended after user-provided opts, so if the caller already set
// a callback via WithAICallback, the SpeedPriority callback will be promoted from it.
//
// Example:
//
//	result, err := aicommon.InvokeLiteForgeSpeedPriority(prompt)
//	result, err := aicommon.InvokeLiteForgeSpeedPriority(prompt, aicommon.WithAICallback(cb))
func InvokeLiteForgeSpeedPriority(prompt string, opts ...any) (*ForgeResult, error) {
	opts = append(opts, WithLiteForgeSpeedFirst())
	return InvokeLiteForge(prompt, opts...)
}

// InvokeLiteForgeQualityPriority invokes LiteForge with quality-priority (intelligent) AI model.
// This is a convenience wrapper around InvokeLiteForge that automatically applies WithLiteForgeQualityFirst().
// The quality priority option is appended after user-provided opts, so if the caller already set
// a callback via WithAICallback, the QualityPriority callback will be promoted from it.
//
// Example:
//
//	result, err := aicommon.InvokeLiteForgeQualityPriority(prompt)
//	result, err := aicommon.InvokeLiteForgeQualityPriority(prompt, aicommon.WithAICallback(cb))
func InvokeLiteForgeQualityPriority(prompt string, opts ...any) (*ForgeResult, error) {
	opts = append(opts, WithLiteForgeQualityFirst())
	return InvokeLiteForge(prompt, opts...)
}
