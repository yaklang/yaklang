package chanx

import (
	"context"
	"sync"
)

type Broadcaster[T any] struct {
	ctx         context.Context
	mu          sync.RWMutex
	baseSize    int
	subscribers map[*UnlimitedChan[T]]struct{}
}

func (b *Broadcaster[T]) Subscribe() *UnlimitedChan[T] {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := NewUnlimitedChan[T](b.ctx, b.baseSize)
	b.subscribers[ch] = struct{}{}
	return ch
}

func (b *Broadcaster[T]) Unsubscribe(ch *UnlimitedChan[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// ch 是 <-chan T 类型，需要类型转换为 chan T 才能作为 map 的 key
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		ch.Close()
	}
}

func (b *Broadcaster[T]) Submit(msg T) {
	b.mu.RLock() // 使用读锁，允许多个广播并发进行
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		ch.SafeFeed(msg)
	}
}

// Close 关闭广播器，并关闭所有订阅者 Channel。
func (b *Broadcaster[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		delete(b.subscribers, ch)
		ch.Close()
	}
}

func NewBroadcastChannel[T any](ctx context.Context, baseSize int) *Broadcaster[T] {
	return &Broadcaster[T]{
		ctx:         ctx,
		mu:          sync.RWMutex{},
		baseSize:    baseSize,
		subscribers: make(map[*UnlimitedChan[T]]struct{}),
	}
}
