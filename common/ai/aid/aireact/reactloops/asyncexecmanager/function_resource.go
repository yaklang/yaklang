package asyncexecmanager

import (
	"context"
	"fmt"
)

// functionResource adapts existing execution and observation functions without
// storing outputs. Its callbacks are immutable after construction.
type functionResource struct {
	execute func(context.Context) error
	inspect func(context.Context) (Observation, error)
}

// NewResource adapts two functions to Resource and Inspector. It does not start
// work or sample observations. Both functions are required and must satisfy the
// Resource and Inspector contracts. Create one resource per execution; callbacks
// should capture that invocation's state, not a mutable current-task pointer.
func NewResource(
	execute func(context.Context) error,
	inspect func(context.Context) (Observation, error),
) (Resource, error) {
	if execute == nil || inspect == nil {
		return nil, fmt.Errorf("%w: execute and inspect functions are required", ErrInvalidResource)
	}
	return &functionResource{execute: execute, inspect: inspect}, nil
}

func (r *functionResource) Execute(ctx context.Context) error { return r.execute(ctx) }

func (r *functionResource) Inspector() Inspector { return r }

func (r *functionResource) Inspect(ctx context.Context) (Observation, error) {
	return r.inspect(ctx)
}
