package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ReActIF defines the interface for ReAct instances
// This abstraction allows aiengine to use ReAct without directly importing aireact package
type ReActIF interface {
	// SendInputEvent sends an input event to the ReAct instance
	SendInputEvent(event *ypb.AIInputEvent) error

	// Wait waits for all tasks to complete
	Wait()

	// IsFinished checks if all tasks are finished
	IsFinished() bool
}

// NewReActFunc is the function type for creating a new ReAct instance
type NewReActFunc func(opts ...ConfigOption) (ReActIF, error)

var (
	reactMu         sync.RWMutex
	newReActFunc    NewReActFunc
	builtinToolsOpt ConfigOption
)

// RegisterNewReAct registers a function to create new ReAct instances
// This should be called by aireact package's init function
func RegisterNewReAct(f NewReActFunc) {
	reactMu.Lock()
	defer reactMu.Unlock()
	newReActFunc = f
}

// RegisterBuiltinToolsOption registers the WithBuiltinTools option
// This allows aiengine to use builtin tools without importing aireact
func RegisterBuiltinToolsOption(opt ConfigOption) {
	reactMu.Lock()
	defer reactMu.Unlock()
	builtinToolsOpt = opt
}

// NewReAct creates a new ReAct instance using the registered factory function
// Returns error if no factory function is registered
func NewReAct(opts ...ConfigOption) (ReActIF, error) {
	reactMu.RLock()
	f := newReActFunc
	reactMu.RUnlock()

	if f == nil {
		return nil, utils.Error("ReAct factory not registered, please import 'github.com/yaklang/yaklang/common/ai/aid/aireact'")
	}
	return f(opts...)
}

// GetBuiltinToolsOption returns the registered WithBuiltinTools option
func GetBuiltinToolsOption() ConfigOption {
	reactMu.RLock()
	defer reactMu.RUnlock()
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
