package reactloops

import (
	"bytes"
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

type LoopActionHandlerOperator struct {
	feedbacks            *bytes.Buffer
	disallowLoopExitOnce *utils.Once
	disallowLoopExit     bool

	terminateOperateOnce *utils.Once

	isContinued  bool
	isTerminated bool
	failedError  error
	isSilence    bool

	task aicommon.AIStatefulTask

	// dynamic async mode: handler can request async mode at runtime
	requestedAsyncMode bool

	// 自我反思相关
	reflectionLevel      ReflectionLevel
	customReflectionData map[string]interface{}
}

func (r *LoopActionHandlerOperator) GetContext() context.Context {
	return r.task.GetContext()
}

func newLoopActionHandlerOperator(task aicommon.AIStatefulTask) *LoopActionHandlerOperator {
	return &LoopActionHandlerOperator{
		feedbacks:            new(bytes.Buffer),
		terminateOperateOnce: utils.NewOnce(),
		disallowLoopExitOnce: utils.NewOnce(),
		task:                 task,
		reflectionLevel:      ReflectionLevel_None, // 默认不反思，由策略决定
		customReflectionData: make(map[string]interface{}),
	}
}

// NewActionHandlerOperator creates a LoopActionHandlerOperator for external use (e.g. action unit tests).
func NewActionHandlerOperator(task aicommon.AIStatefulTask) *LoopActionHandlerOperator {
	return newLoopActionHandlerOperator(task)
}

func (l *LoopActionHandlerOperator) GetTask() aicommon.AIStatefulTask {
	return l.task
}

func (l *LoopActionHandlerOperator) DisallowNextLoopExit() {
	l.disallowLoopExitOnce.Do(func() {
		l.disallowLoopExit = true
	})
}

func (l *LoopActionHandlerOperator) Continue() {
	l.terminateOperateOnce.Do(func() {
		l.isContinued = true
	})
}

func (l *LoopActionHandlerOperator) Exit() {
	l.terminateOperateOnce.Do(func() {
		l.isTerminated = true
	})
}

func (l *LoopActionHandlerOperator) IsTerminated() (bool, error) {
	return l.isTerminated, l.failedError
}

func (l *LoopActionHandlerOperator) IsContinued() bool {
	return l.isContinued
}

func (l *LoopActionHandlerOperator) Fail(i any) {
	l.terminateOperateOnce.Do(func() {
		l.failedError = utils.Error(i)
		l.isTerminated = true
	})
}

func (l *LoopActionHandlerOperator) Feedback(i any) {
	_, _ = l.feedbacks.WriteString(utils.InterfaceToString(i))
	l.feedbacks.WriteRune('\n')
}

func (l *LoopActionHandlerOperator) GetFeedback() *bytes.Buffer {
	return l.feedbacks
}

func (l *LoopActionHandlerOperator) GetDisallowLoopExit() bool {
	return l.disallowLoopExit
}

func (l *LoopActionHandlerOperator) MarkSilence(i ...bool) {
	if len(i) <= 0 {
		l.isSilence = true
		return
	}
	l.isSilence = i[0]
}

// RequestAsyncMode allows a handler to dynamically request async mode at runtime.
// This is used by actions like load_capability that need async behavior conditionally
// (e.g. only when the resolved identifier is a blueprint/forge).
func (l *LoopActionHandlerOperator) RequestAsyncMode() {
	l.requestedAsyncMode = true
}

// IsAsyncModeRequested returns whether the handler has dynamically requested async mode.
func (l *LoopActionHandlerOperator) IsAsyncModeRequested() bool {
	return l.requestedAsyncMode
}

// SetReflectionLevel 设置该 action 执行后的反思级别
// action 可以在执行过程中根据实际情况动态设置反思级别
func (l *LoopActionHandlerOperator) SetReflectionLevel(level ReflectionLevel) {
	l.reflectionLevel = level
}

// GetReflectionLevel 获取当前设置的反思级别
func (l *LoopActionHandlerOperator) GetReflectionLevel() ReflectionLevel {
	return l.reflectionLevel
}

// SetReflectionData 设置自定义反思数据
// action 可以添加额外的上下文信息用于反思分析
func (l *LoopActionHandlerOperator) SetReflectionData(key string, value interface{}) {
	l.customReflectionData[key] = value
}

// GetReflectionData 获取自定义反思数据
func (l *LoopActionHandlerOperator) GetReflectionData() map[string]interface{} {
	return l.customReflectionData
}

// OnPostIterationOperator allows post-iteration callbacks to control loop behavior
// This enables the callback to signal that the loop should end, providing a mechanism
// for external control of the ReAct loop lifecycle.
//
// IMPORTANT: Callbacks registered via WithOnPostIteraction run sequentially in
// registration order. If a callback needs to check the final state of the operator
// (e.g. ShouldIgnoreError()) AFTER all other callbacks have set their flags, it
// should use DeferAfterCallbacks to schedule logic that executes after the entire
// callback chain completes. This solves the ordering problem where a global callback
// might check ShouldIgnoreError() before a loop-specific callback has called IgnoreError().
type OnPostIterationOperator struct {
	shouldEndIteration bool
	endReason          any
	ignoreError        bool   // 忽略错误，静默退出
	deferredFuncs      []func() // 在所有回调完成后执行的延迟函数
}

// newOnPostIterationOperator creates a new OnPostIterationOperator
func newOnPostIterationOperator() *OnPostIterationOperator {
	return &OnPostIterationOperator{
		shouldEndIteration: false,
		endReason:          nil,
		ignoreError:        false,
	}
}

// EndIteration signals that the loop should terminate after this iteration
// Optional reason can be provided for logging/debugging purposes
func (o *OnPostIterationOperator) EndIteration(reason ...any) {
	o.shouldEndIteration = true
	if len(reason) > 0 {
		o.endReason = reason[0]
	}
}

// ShouldEndIteration returns whether the loop should end
func (o *OnPostIterationOperator) ShouldEndIteration() bool {
	return o.shouldEndIteration
}

// GetEndReason returns the reason for ending the iteration
func (o *OnPostIterationOperator) GetEndReason() any {
	return o.endReason
}

// IgnoreError 标记忽略错误，不报错退出
// 用于专注模式在超出迭代次数时的优雅处理
// 调用此方法后，即使循环因为错误（如超出最大迭代次数）而结束，
// 也不会返回错误，而是静默退出
func (o *OnPostIterationOperator) IgnoreError() {
	o.ignoreError = true
}

// ShouldIgnoreError 返回是否应该忽略错误
func (o *OnPostIterationOperator) ShouldIgnoreError() bool {
	return o.ignoreError
}

// DeferAfterCallbacks registers a function to run after ALL OnPostIteration
// callbacks have completed. This is critical for logic that depends on the
// final state of the operator (e.g. checking ShouldIgnoreError()) because
// callbacks run in registration order and a flag like IgnoreError() might
// be set by a later callback.
//
// Typical use: the global EmitReActFail/EmitReActSuccess callback defers its
// emit decision so that loop-specific IgnoreError() calls are respected
// regardless of callback registration order.
func (o *OnPostIterationOperator) DeferAfterCallbacks(fn func()) {
	if fn != nil {
		o.deferredFuncs = append(o.deferredFuncs, fn)
	}
}

// RunDeferredFuncs executes all deferred functions in registration order.
// Called by callOnPostIteration after the main callback chain completes.
func (o *OnPostIterationOperator) RunDeferredFuncs() {
	for _, fn := range o.deferredFuncs {
		fn()
	}
}

// InitTaskOperator allows init task callbacks to control loop behavior
// This enables the callback to:
// - Done(): Signal that initialization completed and loop should exit (early routing)
// - Failed(): Signal initialization failed with an error
// - Continue(): Continue with normal loop execution (default)
// - NextAction(): Specify which actions MUST be used in next iteration
// - RemoveNextAction(): Specify which actions should be DISABLED in next iteration
type InitTaskOperator struct {
	operateOnce *utils.Once

	isDone   bool
	isFailed bool
	failErr  error

	// Action control for next iteration
	nextActionMustUse  []string // Actions that MUST be used
	nextActionDisabled []string // Actions that are DISABLED
}

// newInitTaskOperator creates a new InitTaskOperator
// Default state is Continue (no special behavior)
func newInitTaskOperator() *InitTaskOperator {
	return &InitTaskOperator{
		operateOnce:        utils.NewOnce(),
		isDone:             false,
		isFailed:           false,
		nextActionMustUse:  []string{},
		nextActionDisabled: []string{},
	}
}

// Done signals that initialization is complete and the loop should exit immediately
// This is used for "early routing" scenarios where init directly handles the request
func (o *InitTaskOperator) Done() {
	o.operateOnce.Do(func() {
		o.isDone = true
	})
}

// Failed signals that initialization failed with an error
// The loop will exit with this error
func (o *InitTaskOperator) Failed(err any) {
	o.operateOnce.Do(func() {
		o.isFailed = true
		o.failErr = utils.Error(err)
	})
}

// Continue signals that the loop should continue with normal execution
// This is the default behavior, calling this explicitly is optional
func (o *InitTaskOperator) Continue() {
	// No-op, this is the default behavior
	// Explicitly call this for clarity in init handlers
}

// NextAction specifies which actions MUST be used in the next iteration
// Multiple calls will accumulate the required actions
func (o *InitTaskOperator) NextAction(actions ...string) {
	o.nextActionMustUse = append(o.nextActionMustUse, actions...)
}

// RemoveNextAction specifies which actions should be DISABLED in the next iteration
// These actions will be filtered out from available actions
func (o *InitTaskOperator) RemoveNextAction(actions ...string) {
	o.nextActionDisabled = append(o.nextActionDisabled, actions...)
}

// IsDone returns whether the init handler signaled completion (early exit)
func (o *InitTaskOperator) IsDone() bool {
	return o.isDone
}

// IsFailed returns whether the init handler signaled failure
func (o *InitTaskOperator) IsFailed() (bool, error) {
	return o.isFailed, o.failErr
}

// IsContinued returns whether the init handler wants to continue with normal loop
// This is true when neither Done() nor Failed() was called
func (o *InitTaskOperator) IsContinued() bool {
	return !o.isDone && !o.isFailed
}

// GetNextActionMustUse returns the list of actions that must be used
func (o *InitTaskOperator) GetNextActionMustUse() []string {
	return o.nextActionMustUse
}

// GetNextActionDisabled returns the list of actions that are disabled
func (o *InitTaskOperator) GetNextActionDisabled() []string {
	return o.nextActionDisabled
}

// HasActionConstraints returns true if any action constraints are set
func (o *InitTaskOperator) HasActionConstraints() bool {
	return len(o.nextActionMustUse) > 0 || len(o.nextActionDisabled) > 0
}
