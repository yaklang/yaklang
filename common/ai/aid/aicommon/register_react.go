package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// NewAIEngineOperatorFunc is the function type for creating a new AIEngineOperator instance
type NewAIEngineOperatorFunc func(opts ...ConfigOption) (AIEngineOperator, error)

var (
	operatorMu                      sync.RWMutex
	newReActAIEngineOperatorFunc    NewAIEngineOperatorFunc
	builtinToolsOpt                 ConfigOption
)

// RegisterReActAIEngineOperator registers a function to create ReAct-based AIEngineOperator instances
// This should be called by aireact package's init function
func RegisterReActAIEngineOperator(f NewAIEngineOperatorFunc) {
	operatorMu.Lock()
	defer operatorMu.Unlock()
	newReActAIEngineOperatorFunc = f
}

// RegisterBuiltinToolsOption registers the WithBuiltinTools option
// This allows aiengine to use builtin tools without importing aireact
func RegisterBuiltinToolsOption(opt ConfigOption) {
	operatorMu.Lock()
	defer operatorMu.Unlock()
	builtinToolsOpt = opt
}

// NewReActAIEngineOperator creates a new ReAct-based AIEngineOperator instance
// Returns error if no factory function is registered
func NewReActAIEngineOperator(opts ...ConfigOption) (AIEngineOperator, error) {
	operatorMu.RLock()
	f := newReActAIEngineOperatorFunc
	operatorMu.RUnlock()

	if f == nil {
		return nil, utils.Error("ReAct AIEngineOperator factory not registered, please import 'github.com/yaklang/yaklang/common/ai/aid/aireact'")
	}
	return f(opts...)
}

// GetBuiltinToolsOption returns the registered WithBuiltinTools option
func GetBuiltinToolsOption() ConfigOption {
	operatorMu.RLock()
	defer operatorMu.RUnlock()
	return builtinToolsOpt
}

// WithBuiltinTools returns the builtin tools option if registered
// This is a convenience function that can be used in aiengine
func WithBuiltinTools() ConfigOption {
	opt := GetBuiltinToolsOption()
	if opt == nil {
		// Return a no-op option if not registered
		return func(c *Config) error { return nil }
	}
	return opt
}
