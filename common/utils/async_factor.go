package utils

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/utils/chanx"
)

type AsyncFactory[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	factor func() (T, error)
	buffer *chanx.UnlimitedChan[T]
	wg     sync.WaitGroup
}

const defaultAsyncSize = 200

func NewAsyncFactory[T any](
	CTX context.Context,
	factor func() (T, error),
) *AsyncFactory[T] {
	ctx, cancel := context.WithCancel(CTX)
	ret := &AsyncFactory[T]{
		ctx:    ctx,
		cancel: cancel,
		factor: factor,
		buffer: chanx.NewUnlimitedChan[T](ctx, defaultAsyncSize),
	}

	ret.wg.Add(1)
	go func() {
		defer ret.wg.Done()
		ret.process()
	}()

	return ret
}

func (af *AsyncFactory[T]) Get() (T, error) {
	if item, ok := <-af.buffer.OutputChannel(); ok {
		return item, nil
	} else {
		var zero T
		return zero, Errorf("get async factor item error: %v", ok)
	}
}

func (af *AsyncFactory[T]) process() {
	for {
		select {
		case <-af.ctx.Done():
			return
		default:
			if item, err := af.factor(); err == nil {
				af.buffer.SafeFeed(item)
			}
		}
	}
}

func (af *AsyncFactory[T]) Close() {
	af.cancel()       //  ctx cancel, will finish process
	af.wg.Wait()      // wait process finish
	af.buffer.Close() // then, we can close the chan
}
