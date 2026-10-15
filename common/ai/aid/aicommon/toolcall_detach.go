package aicommon

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var ErrToolDetachUnavailable = errors.New("tool detachment requires an owning ReAct loop")
var ErrToolCallSettled = errors.New("tool call has already settled")
var ErrToolReviewDirectAnswer = fmt.Errorf("%w: tool review requested a direct answer", context.Canceled)

// ToolCallDetached is a dispatch receipt, not a ToolResult or execution failure.
// It travels through legacy call signatures without fabricating a successful result.
type ToolCallDetached struct{ Receipt asyncexecmanager.Summary }

func (e *ToolCallDetached) Error() string {
	return "tool execution continues in background: " + e.Receipt.ID
}

func WithToolCaller_AsyncExecutionID(id string) ToolCallerOption {
	return func(t *ToolCaller) {
		t.detachHandler = func() (string, error) { return id, nil }
		t.detached.Store(true)
	}
}
func WithToolCaller_OnDetach(fn func()) ToolCallerOption {
	return func(t *ToolCaller) { t.onDetach = fn }
}
func (t *ToolCaller) Detach() (string, error) {
	if t == nil || t.detachHandler == nil {
		return "", ErrToolDetachUnavailable
	}
	id, err := t.detachHandler()
	if err == nil {
		t.detached.Store(true)
		t.detachOnce.Do(func() {
			if t.onDetach != nil {
				t.onDetach()
			}
		})
	}
	return id, err
}
func (t *ToolCaller) CallToolID() string { return t.callToolId }
func (t *ToolCaller) ResultID() int64    { return t.resultID }

// ToolCallExecution owns the entire original call stack. Detachment changes only
// who waits for it; the plugin, review, writers and cleanup are never restarted.
// finish is invoked exactly once before Done closes, with an immutable ownership
// decision. It must not retain a foreground action/operator.
type ToolCallExecution struct {
	caller   *ToolCaller
	register func(asyncexecmanager.Resource) (asyncexecmanager.Summary, error)
	finish   func(*aitool.ToolResult, bool, error, bool)
	mu       sync.Mutex
	started  bool
	settling bool
	receipt  *asyncexecmanager.Summary
	detached chan struct{}
	done     chan struct{}
	result   *aitool.ToolResult
	directly bool
	err      error
}

func NewToolCallExecution(caller *ToolCaller, register func(asyncexecmanager.Resource) (asyncexecmanager.Summary, error), finish func(*aitool.ToolResult, bool, error, bool)) (*ToolCallExecution, error) {
	if caller == nil || caller.m == nil || caller.ctx == nil || caller.cancel == nil || register == nil {
		return nil, ErrToolDetachUnavailable
	}
	caller.m.Lock()
	defer caller.m.Unlock()
	if caller.asyncResourceBound || !caller.startTime.IsZero() {
		return nil, errors.New("tool caller is already in use")
	}
	caller.asyncResourceBound = true
	caller.waitForCallbackOnCancel = true
	e := &ToolCallExecution{caller: caller, register: register, finish: finish, detached: make(chan struct{}), done: make(chan struct{})}
	caller.detachHandler = e.detach
	return e, nil
}
func (e *ToolCallExecution) Start(run func() (*aitool.ToolResult, bool, error)) {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.mu.Unlock()
	go func() {
		var result *aitool.ToolResult
		var directly bool
		var err error
		defer func() {
			if value := recover(); value != nil {
				err = fmt.Errorf("tool call panic: %v", value)
			}
			e.mu.Lock()
			e.settling = true
			detached := e.receipt != nil
			e.mu.Unlock()
			// Always release waiters, even if owner bookkeeping panics.
			defer func() {
				if value := recover(); value != nil {
					err = fmt.Errorf("tool completion panic: %v", value)
				}
				e.result, e.directly, e.err = result, directly, err
				e.caller.Cancel()
				close(e.done)
			}()
			if e.finish != nil {
				e.finish(result, directly, err, detached)
			}
		}()
		result, directly, err = run()
	}()
}
func (e *ToolCallExecution) detach() (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.receipt != nil {
		return e.receipt.ID, nil
	}
	if e.settling {
		return "", ErrToolCallSettled
	}
	if err := e.caller.ctx.Err(); err != nil {
		return "", err
	}
	receipt, err := e.register(e)
	if err != nil {
		return "", err
	}
	e.receipt = &receipt
	close(e.detached)
	return receipt.ID, nil
}
func (e *ToolCallExecution) Wait() (*aitool.ToolResult, bool, error) {
	select {
	case <-e.done:
	case <-e.detached:
	}
	// A registered execution always belongs to the manager, even when completion
	// raced this waiter. Returning its result here would archive it twice.
	e.mu.Lock()
	receipt := e.receipt
	e.mu.Unlock()
	if receipt != nil {
		return nil, false, &ToolCallDetached{Receipt: *receipt}
	}
	<-e.done
	return e.result, e.directly, e.err
}
func (e *ToolCallExecution) Done() <-chan struct{}                 { return e.done }
func (e *ToolCallExecution) Inspector() asyncexecmanager.Inspector { return e.caller.Inspector() }
func (e *ToolCallExecution) Execute(ctx context.Context) error {
	if ctx == nil {
		return errors.New("tool execution context is required")
	}
	stop := context.AfterFunc(ctx, e.caller.Cancel)
	defer stop()
	if ctx.Err() != nil {
		e.caller.Cancel()
	}
	<-e.done // Adopt must join the original call, including cancellation cleanup.
	if e.err != nil {
		return e.err
	}
	if e.directly {
		return ErrToolReviewDirectAnswer
	}
	if e.result == nil {
		return errors.New("tool call did not produce a result")
	}
	if !e.result.Success {
		return fmt.Errorf("tool call failed: %s", e.result.Error)
	}
	return nil
}

type toolDetachContextKey struct{}
type toolDetachContextValue struct {
	detach   func() (string, error)
	detached bool
}

func withToolDetachContext(ctx context.Context, detach func() (string, error)) context.Context {
	return context.WithValue(ctx, toolDetachContextKey{}, &toolDetachContextValue{detach: detach})
}
func ToolDetachAvailable(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	value, _ := ctx.Value(toolDetachContextKey{}).(*toolDetachContextValue)
	return value != nil && value.detach != nil
}
func toolDetachedDuringReview(ctx context.Context) bool {
	value, _ := ctx.Value(toolDetachContextKey{}).(*toolDetachContextValue)
	return value != nil && value.detached
}
func DetachToolFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrToolDetachUnavailable
	}
	value, _ := ctx.Value(toolDetachContextKey{}).(*toolDetachContextValue)
	if value == nil || value.detach == nil {
		return "", ErrToolDetachUnavailable
	}
	id, err := value.detach()
	if err == nil {
		value.detached = true
	}
	return id, err
}
