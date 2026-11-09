package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AsyncToolCallRuntime prepares a call for its explicit owner without executing
// it. It is optional so AIInvokeRuntime implementations need not add methods.
// The returned cancel function releases preparation on failed admission or owner
// release, including when the manager cancels a pending call before Execute.
type AsyncToolCallRuntime interface {
	PrepareAsyncToolCall(
		task AIStatefulTask, name string, params aitool.InvokeParams, opts ...ToolCallerOption,
	) (asyncexecmanager.Resource, func(), error)
}

// AsyncExecutionManagerProvider lets a session-scoped runtime own background
// executions across multiple task loops. ReAct implements this provider so a
// loop release does not cancel work that was intentionally left in the session.
type AsyncExecutionManagerProvider interface {
	GetAsyncExecutionManager() (*asyncexecmanager.Manager, error)
	PeekAsyncExecutionManager() *asyncexecmanager.Manager
}

// AsyncExecutionTaskTracker associates an admitted execution with its source
// task. Session-owned work survives natural task completion, but a user
// cancellation or aborted task stops the matching executions.
type AsyncExecutionTaskTracker interface {
	TrackAsyncExecutionTask(task AIStatefulTask, executeID string)
}

// AsyncDirectAnswerProvider carries a background-review "answer now" request
// across task loops in the same ReAct session.
type AsyncDirectAnswerProvider interface {
	AsyncDirectAnswerSignal() <-chan struct{}
	RequestAsyncDirectAnswer()
	AsyncDirectAnswerRequested() bool
	ConsumeAsyncDirectAnswer() bool
	// BeginAsyncExecutionAdmission serializes new execution admission with a
	// direct-answer request. The release function must be called after the
	// manager has either admitted the execution or rejected it.
	BeginAsyncExecutionAdmission() (release func(), allowed bool)
}
