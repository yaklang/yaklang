package aicommon

import (
	"context"
)

// ReActLoopIF defines the interface for ReAct Loop operations
// Note: Some methods return interface{} to avoid circular dependencies with reactloops package
type ReActLoopIF interface {
	// Core execution methods
	Execute(taskId string, ctx context.Context, userInput string) error
	ExecuteWithExistedTask(task AIStatefulTask) error

	// Task management
	GetCurrentTask() AIStatefulTask
	SetCurrentTask(t AIStatefulTask)

	// Configuration and context
	GetInvoker() AIInvokeRuntime
	GetEmitter() *Emitter
	GetConfig() AICallerConfigIf
	GetMemoryTriage() MemoryTriage
	GetEnableSelfReflection() bool

	// Variable management
	Set(key string, value any)
	Get(key string) string
	GetVariable(key string) any
	GetStringSlice(key string) []string
	GetInt(key string) int

	// Action management
	RemoveAction(actionType string)
	GetAllActionNames() []string
	NoActions() bool

	// Memory management
	PushMemory(result *SearchMemoryResult)
	GetCurrentMemoriesContent() string

	// User interaction control
	DisallowAskForClarification()
}
