package yakvm

import (
	"context"
	"time"
)

const canceledDeferGracePeriod = 5 * time.Second

type deferContextKey struct{}

// Deferred Yak instructions must still run after the owner is cancelled. Nested
// functions share one cleanup context: they cannot renew its cancellation budget.
// Normal cleanup keeps its existing unlimited lifetime; cancellation starts a
// bounded grace period. Native calls must still cooperate with their own context.
func (v *Frame) enterDeferredExecution() func() {
	original := v.ctx
	if original == nil || original.Value(deferContextKey{}) != nil {
		return func() {}
	}
	cleanup, cancel := context.WithCancel(context.WithoutCancel(original))
	stopOwnerWatch := context.AfterFunc(original, func() {
		timer := time.NewTimer(canceledDeferGracePeriod)
		defer timer.Stop()
		select {
		case <-timer.C:
			cancel()
		case <-cleanup.Done():
		}
	})
	v.ctx = context.WithValue(cleanup, deferContextKey{}, true)
	return func() {
		v.ctx = original
		stopOwnerWatch()
		cancel()
	}
}

type Defer struct {
	Codes []*Code
	Scope *Scope
}
