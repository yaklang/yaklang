package aicommon

import (
	"context"
	"time"
)

const (
	aiRetryInitialBackoff = 2 * time.Second
	aiRetryMaxBackoff     = 32 * time.Second

	aiTransportRetryInitialBackoff = 10 * time.Second
	aiTransportRetryMaxBackoff     = 60 * time.Second
)

// aiRetryBackoff returns the delay before the numbered retry. The first retry
// waits two seconds, then the delay doubles until it reaches 32 seconds.
func aiRetryBackoff(retryNumber int64) time.Duration {
	if retryNumber <= 0 {
		return 0
	}
	if retryNumber >= 5 {
		return aiRetryMaxBackoff
	}
	return aiRetryInitialBackoff << (retryNumber - 1)
}

// aiTransportRetryBackoff returns the delay before the numbered retry after a
// transport-class failure (dial/TLS/gateway outage). It starts at ten seconds
// and doubles up to sixty seconds, so sub-minute outage windows observed in
// engine logs can be ridden out within the network retry budget instead of
// burning attempts inside a two-second spread.
func aiTransportRetryBackoff(retryNumber int64) time.Duration {
	if retryNumber <= 0 {
		return 0
	}
	if retryNumber >= 4 {
		return aiTransportRetryMaxBackoff
	}
	return aiTransportRetryInitialBackoff << (retryNumber - 1)
}

type aiRetryWaiter interface {
	waitBeforeAIRetry(context.Context, time.Duration) error
}

type aiRetryWaitFuncContextKey struct{}

type aiRetryWaitFuncContextValue struct {
	wait func(context.Context, time.Duration) error
}

func withAIRetryWaitFuncContext(ctx context.Context, wait func(context.Context, time.Duration) error) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, aiRetryWaitFuncContextKey{}, aiRetryWaitFuncContextValue{wait: wait})
}

func aiRetryWaitFuncFromContext(ctx context.Context) func(context.Context, time.Duration) error {
	if ctx == nil {
		return nil
	}
	value, ok := ctx.Value(aiRetryWaitFuncContextKey{}).(aiRetryWaitFuncContextValue)
	if !ok {
		return nil
	}
	return value.wait
}

// waitBeforeAIRetry keeps retry delays context-aware and gives package tests a
// clock seam, so the production schedule can be verified without sleeping for
// the full backoff duration.
func waitBeforeAIRetry(ctx context.Context, owner any, delay time.Duration) error {
	if waiter, ok := owner.(aiRetryWaiter); ok {
		return waiter.waitBeforeAIRetry(ctx, delay)
	}
	return waitBeforeAIRetryDefault(ctx, delay)
}

func waitBeforeAIRetryDefault(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Config) waitBeforeAIRetry(ctx context.Context, delay time.Duration) error {
	if c.aiRetryWaitFunc != nil {
		return c.aiRetryWaitFunc(ctx, delay)
	}
	return waitBeforeAIRetryDefault(ctx, delay)
}

func (c *Config) waitBeforeNextAIRequestRetry(ctx context.Context, err error, failedAttempt, maxAttempts int) error {
	if failedAttempt >= maxAttempts {
		c.EmitWarning("ai request err: %v, retry auto time: [%v/%v]", err, failedAttempt, maxAttempts)
		return nil
	}
	retryDelay := aiRetryBackoff(int64(failedAttempt))
	c.EmitWarning("ai request err: %v, retry auto time: [%v/%v], retrying in %s", err, failedAttempt, maxAttempts, retryDelay)
	return waitBeforeAIRetry(ctx, c, retryDelay)
}
