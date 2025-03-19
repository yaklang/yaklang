package chanx

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"sync/atomic"
	"time"
)

// UnlimitedChan is an unbounded chan.
// In is used to write without blocking, which supports multiple writers.
// and Out is used to read, which supports multiple readers.
// You can close the in channel if you want.
type UnlimitedChan[T any] struct {
	bufCount int64
	In       chan<- T       // channel for write
	Out      <-chan T       // channel for read
	buffer   *RingBuffer[T] // buffer
	ctx      context.Context
	cancel   context.CancelFunc
}

func (c *UnlimitedChan[T]) SafeFeed(i T) {
	select {
	case c.In <- i:
	case <-time.After(3 * time.Second):
		log.Error("timeout for write in *UnlimitedChan, try to solve it to prevent mem-leak")
	}
}

func (c *UnlimitedChan[T]) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.In != nil {
		close(c.In)
	}
}

// Len returns len of In plus len of Out plus len of buffer.
// It is not accurate and only for your evaluating approximate number of elements in this chan,
// see https://github.com/smallnest/chanx/issues/7.
func (c *UnlimitedChan[T]) Len() int {
	return len(c.In) + c.BufLen() + len(c.Out)
}

// BufLen returns len of the buffer.
// It is not accurate and only for your evaluating approximate number of elements in this chan,
// see https://github.com/smallnest/chanx/issues/7.
func (c *UnlimitedChan[T]) BufLen() int {
	return int(atomic.LoadInt64(&c.bufCount))
}

// NewUnlimitedChan creates the unbounded chan.
// in is used to write without blocking, which supports multiple writers.
// and out is used to read, which supports multiple readers.
// You can close the in channel if you want.
func NewUnlimitedChan[T any](ctx context.Context, initCapacity int) *UnlimitedChan[T] {
	return NewUnlimitedChanSize[T](ctx, initCapacity, initCapacity, initCapacity)
}

func NewUnlimitedChanEx[T any](ctx context.Context, in chan T, out chan T, initBufCapacity int) *UnlimitedChan[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	ch := UnlimitedChan[T]{In: in, Out: out, buffer: NewRingBuffer[T](initBufCapacity), ctx: ctx, cancel: cancel}
	go process(ctx, in, out, &ch)
	return &ch
}

// NewUnlimitedChanSize is like NewUnlimitedChan but you can set initial capacity for In, Out, Buffer.
func NewUnlimitedChanSize[T any](ctx context.Context, initInCapacity, initOutCapacity, initBufCapacity int) *UnlimitedChan[T] {
	in := make(chan T, initInCapacity)
	out := make(chan T, initOutCapacity)
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	ch := UnlimitedChan[T]{In: in, Out: out, buffer: NewRingBuffer[T](initBufCapacity), ctx: ctx, cancel: cancel}

	go process(ctx, in, out, &ch)

	return &ch
}

func process[T any](ctx context.Context, in, out chan T, ch *UnlimitedChan[T]) {
	defer close(out)
	drain := func() {
		for !ch.buffer.IsEmpty() {
			select {
			case out <- ch.buffer.Pop():
				atomic.AddInt64(&ch.bufCount, -1)
			case <-ctx.Done():
				return
			}
		}

		ch.buffer.Reset()
		atomic.StoreInt64(&ch.bufCount, 0)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case val, ok := <-in:
			if !ok { // in is closed
				drain()
				return
			}

			// make sure values' order
			// buffer has some values
			if atomic.LoadInt64(&ch.bufCount) > 0 {
				ch.buffer.Write(val)
				atomic.AddInt64(&ch.bufCount, 1)
			} else {
				// out is not full
				select {
				case out <- val:
					continue
				default:
				}

				// out is full
				ch.buffer.Write(val)
				atomic.AddInt64(&ch.bufCount, 1)
			}

			for !ch.buffer.IsEmpty() {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-in:
					if !ok { // in is closed
						drain()
						return
					}
					ch.buffer.Write(val)
					atomic.AddInt64(&ch.bufCount, 1)

				case out <- ch.buffer.Peek():
					ch.buffer.Pop()
					atomic.AddInt64(&ch.bufCount, -1)
					if ch.buffer.IsEmpty() && ch.buffer.size > ch.buffer.initialSize { // after burst
						ch.buffer.Reset()
						atomic.StoreInt64(&ch.bufCount, 0)
					}
				}
			}
		}
	}
}
