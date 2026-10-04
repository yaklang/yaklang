package coordinator

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type parentRuntimeKey struct{}
type parentRuntime struct {
	runtime aicommon.AIInvokeRuntime
	task    aicommon.AIStatefulTask
	id      string
}

// WithForgeParent binds an explicitly selected Forge to its calling session.
// The existing outer Forge adapter retains ownership of start/end events.
// It supplies inherited ConfigOptions and its private input/hotpatch channels
// to the Forge factory; this context adds identity, cancellation and storage.
func WithForgeParent(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
		if task != nil {
			ctx = task.GetContext()
		}
	}
	return context.WithValue(ctx, parentRuntimeKey{}, parentRuntime{r, task, id})
}

func NewForgeSession(ctx context.Context, query string, opts ...aicommon.ConfigOption) (*Session, error) {
	// Forge owns a complete invocation, including business result delivery.
	// The caller's detached PLAN preference is only for ordinary planning;
	// inheriting it would return after publication and discard this Forge's
	// delivery handler before any worker runs. Keep its approval live instead.
	opts = append(append([]aicommon.ConfigOption{}, opts...), aicommon.WithEnableDetachedPlan(false))
	if ctx != nil {
		if p, ok := ctx.Value(parentRuntimeKey{}).(parentRuntime); ok {
			s, err := fromRuntime(ctx, p.runtime, p.task, p.id, true, opts...)
			if err != nil {
				return nil, err
			}
			s.query, s.externalLifecycle = query, true
			return s, nil
		}
	}
	return NewSession(ctx, query, opts...)
}
