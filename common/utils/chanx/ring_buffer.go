package chanx

import "errors"

var ErrIsEmpty = errors.New("ringbuffer is empty")

type ringBufferCore struct {
	buf         []any
	initialSize int
	size        int
	r           int
	w           int
}

func newRingBufferCore(initialSize int) ringBufferCore {
	if initialSize <= 1 {
		initialSize = 2
	}
	return ringBufferCore{
		buf:         make([]any, initialSize),
		initialSize: initialSize,
		size:        initialSize,
	}
}

func (r *ringBufferCore) read() (any, error) {
	if r.r == r.w {
		return nil, ErrIsEmpty
	}
	v := r.buf[r.r]
	r.buf[r.r] = nil
	r.r++
	if r.r == r.size {
		r.r = 0
	}
	return v, nil
}

func (r *ringBufferCore) peek() any {
	if r.r == r.w {
		panic(ErrIsEmpty.Error())
	}
	return r.buf[r.r]
}

func (r *ringBufferCore) write(v any) {
	r.buf[r.w] = v
	r.w++
	if r.w == r.size {
		r.w = 0
	}
	if r.w == r.r {
		r.grow()
	}
}

func (r *ringBufferCore) grow() {
	var size int
	if r.size < 1024 {
		size = r.size * 2
	} else {
		size = r.size + r.size/4
	}
	buf := make([]any, size)
	copy(buf[0:], r.buf[r.r:])
	copy(buf[r.size-r.r:], r.buf[0:r.r])
	r.r = 0
	r.w = r.size
	r.size = size
	r.buf = buf
}

func (r *ringBufferCore) isEmpty() bool {
	return r.r == r.w
}

func (r *ringBufferCore) capacity() int {
	return r.size
}

func (r *ringBufferCore) length() int {
	if r.r == r.w {
		return 0
	}
	if r.w > r.r {
		return r.w - r.r
	}
	return r.size - r.r + r.w
}

func (r *ringBufferCore) reset() {
	r.r = 0
	r.w = 0
	r.size = r.initialSize
	r.buf = make([]any, r.initialSize)
}

func castRingValue[T any](v any) T {
	if v == nil {
		var zero T
		return zero
	}
	return v.(T)
}

// RingBuffer is a ring buffer for common types.
// It never is full and always grows if it will be full.
// It is not thread-safe(goroutine-safe) so you must use the lock-like synchronization primitive to use it in multiple writers and multiple readers.
type RingBuffer[T any] struct {
	core ringBufferCore
}

func NewRingBuffer[T any](initialSize int) *RingBuffer[T] {
	return &RingBuffer[T]{
		core: newRingBufferCore(initialSize),
	}
}

func (r *RingBuffer[T]) Read() (T, error) {
	v, err := r.core.read()
	if err != nil {
		var zero T
		return zero, err
	}
	return castRingValue[T](v), nil
}

func (r *RingBuffer[T]) Pop() T {
	v, err := r.core.read()
	if err == ErrIsEmpty {
		panic(ErrIsEmpty.Error())
	}
	return castRingValue[T](v)
}

func (r *RingBuffer[T]) Peek() T {
	return castRingValue[T](r.core.peek())
}

func (r *RingBuffer[T]) Write(v T) {
	r.core.write(v)
}

func (r *RingBuffer[T]) IsEmpty() bool {
	return r.core.isEmpty()
}

// Capacity returns the size of the underlying buffer.
func (r *RingBuffer[T]) Capacity() int {
	return r.core.capacity()
}

func (r *RingBuffer[T]) Len() int {
	return r.core.length()
}

func (r *RingBuffer[T]) Reset() {
	r.core.reset()
}
