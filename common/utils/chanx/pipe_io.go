package chanx

import "context"

// PipeIO is the channel abstraction used by pipeline.Pipe for bounded and
// unbounded buffering strategies.
type PipeIO[T any] interface {
	OutputChannel() <-chan T
	SafeFeed(T)
	Close()
}

// LimitedChan is a fixed-capacity channel wrapper implementing PipeIO.
type LimitedChan[T any] struct {
	ctx context.Context
	ch  chan T
}

func NewLimitedChan[T any](ctx context.Context, bufSize int) *LimitedChan[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	if bufSize < 0 {
		bufSize = 0
	}
	return &LimitedChan[T]{
		ctx: ctx,
		ch:  make(chan T, bufSize),
	}
}

func (c *LimitedChan[T]) OutputChannel() <-chan T {
	return c.ch
}

func (c *LimitedChan[T]) SafeFeed(item T) {
	defer func() {
		_ = recover()
	}()
	select {
	case <-c.ctx.Done():
	case c.ch <- item:
	}
}

func (c *LimitedChan[T]) Close() {
	close(c.ch)
}

var (
	_ PipeIO[int] = (*UnlimitedChan[int])(nil)
	_ PipeIO[int] = (*LimitedChan[int])(nil)
)
