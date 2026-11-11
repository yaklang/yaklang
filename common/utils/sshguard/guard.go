// Package sshguard defines the host authorization contract for Yak SSH exec.
package sshguard

import "context"

type Command struct {
	Target     string `json:"target"`
	Peer       string `json:"peer"`
	User       string `json:"user"`
	Command    string `json:"command"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
}

type Result struct {
	Status    string `json:"status"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Guard authorizes the entire command before any part is sent. A successful
// authorization supplies a callback that receives exactly one observed result.
type Guard func(context.Context, Command) (func(Result), error)

type guardKey struct{}
type guardBinding struct{ guard Guard }

func WithGuard(ctx context.Context, guard Guard) context.Context {
	return context.WithValue(ctx, guardKey{}, &guardBinding{guard})
}

func FromContext(ctx context.Context) (Guard, bool) {
	binding := scope(ctx)
	if binding == nil {
		return nil, false
	}
	return binding.guard, true
}

// SameScope checks whether two contexts were derived from the same host binding.
func SameScope(a, b context.Context) bool {
	left := scope(a)
	return left != nil && left == scope(b)
}

func scope(ctx context.Context) *guardBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(guardKey{}).(*guardBinding)
	return binding
}

type invocationKey struct{}
type invocation struct{ toolCallID, taskID string }

func WithInvocation(ctx context.Context, toolCallID, taskID string) context.Context {
	return context.WithValue(ctx, invocationKey{}, invocation{toolCallID, taskID})
}

func SetInvocation(ctx context.Context, command *Command) {
	id, _ := ctx.Value(invocationKey{}).(invocation)
	command.ToolCallID, command.TaskID = id.toolCallID, id.taskID
}
