package aispec

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

type modelAttemptKey struct{}

// A callback reserves admission before starting its timeout. Its first provider
// uses that reservation; routed fallback providers each acquire their own slot.
// The reservation is settled by the provider, before another fallback can wait.
type modelAttempt struct {
	mu      sync.Mutex
	claimed bool
	done    ModelAttemptDone
	policy  ModelRetryOptions
	number  int
}

func (a *modelAttempt) acquire(ctx context.Context) (ModelAttemptDone, error) {
	a.mu.Lock()
	if !a.claimed {
		a.claimed = true
		a.mu.Unlock()
		return a.done, nil
	}
	a.mu.Unlock()
	return a.policy.acquire(ctx, a.policy.Notify)
}
func (a *modelAttempt) finishUnused(err error) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.claimed {
		return 0
	}
	a.claimed = true
	return finishModelAttempt(a.done, err, a.number)
}
func modelFailureDelay(f *ModelCallFailure, attempt int) time.Duration {
	delay := RetryDelay(attempt, f.RetryAfter, time.Now())
	if f.Decision != nil {
		delay = max(delay, RetryDelay(attempt, f.Decision.RetryAfter, time.Now()))
	}
	return delay
}
func finishModelAttempt(done ModelAttemptDone, err error, attempt int) time.Duration {
	if done == nil {
		return 0
	}
	if err == nil {
		return done(true, false, 0)
	}
	f := ClassifyModelFailure(err, 0, "")
	return done(false, f.CanRetry() && (f.Kind == "http" || f.Kind == "network"), modelFailureDelay(f, attempt))
}

// ModelRetryOptions applies to one logical call. MaxAttempts includes the first
// attempt; zero means one. RetryForever removes this limit for retryable failures.
type ModelRetryOptions struct {
	MaxAttempts int
	Notify      func(string)
	Wait        func(context.Context, time.Duration) error
	acquire     func(context.Context, func(string)) (ModelAttemptDone, error)
}

// RunModelCall owns admission and recovery. Nested model adapters participate in
// the current attempt, so an Agent callback and its provider cannot double-charge
// RPM or multiply retry budgets. Each new attempt inherits caller cancellation.
func RunModelCall[T any](ctx context.Context, policy ModelRetryOptions, call func(context.Context) (T, error)) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if parent, ok := ctx.Value(modelAttemptKey{}).(*modelAttempt); ok {
		var zero T
		done, err := parent.acquire(ctx)
		if err != nil {
			return zero, err
		}
		// Further wrappers around this same provider are part of this admission.
		value, err := call(context.WithValue(ctx, modelAttemptKey{}, true))
		if ctx.Err() != nil {
			if done != nil {
				done(false, false, 0)
			}
			return zero, ctx.Err()
		}
		finishModelAttempt(done, err, parent.number)
		return value, err
	}
	if ctx.Value(modelAttemptKey{}) != nil {
		return call(ctx)
	}
	var zero T
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	if policy.Wait == nil {
		policy.Wait = WaitModelRetry
	}
	if policy.acquire == nil {
		policy.acquire = AcquireModelRequest
	}
	scope := httptrace.GetScope(ctx)
	// A logical call keeps its ID across transport attempts. Agent correction
	// creates another logical call, even when it reuses the checkpoint sequence.
	scope.CallID = uuid.NewString()
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		done, err := policy.acquire(ctx, policy.Notify)
		if err != nil {
			return zero, err
		}
		scope.Attempt = attempt
		reservation := &modelAttempt{done: done, policy: policy, number: attempt}
		attemptCtx := context.WithValue(httptrace.WithScope(ctx, scope), modelAttemptKey{}, reservation)
		value, err := call(attemptCtx)
		if ctx.Err() != nil {
			reservation.finishUnused(&ModelCallFailure{Kind: "configuration", Cause: ctx.Err()})
			return zero, ctx.Err()
		}
		if err == nil {
			reservation.finishUnused(nil)
			return value, nil
		}
		failure := ClassifyModelFailure(err, 0, "")
		failure.Attempt = attempt
		retryable := failure.CanRetry()
		delay := max(modelFailureDelay(failure, attempt), reservation.finishUnused(err))
		forever := consts.IsAIRetryForeverEnabled()
		if !retryable || (!forever && attempt >= policy.MaxAttempts) {
			return value, failure
		}
		if policy.Notify != nil {
			policy.Notify(fmt.Sprintf("模型请求失败，等待 %.1f 秒后进行第 %d 次尝试，可随时停止任务：%v", delay.Seconds(), attempt+1, failure))
		}
		if err := policy.Wait(ctx, delay); err != nil {
			return zero, err
		}
		if forever && !consts.IsAIRetryForeverEnabled() {
			return value, failure
		}
	}
}
