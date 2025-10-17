package utils

import (
	"sync"
	"sync/atomic"
)

// Once 是一个可以确保某个操作只执行一次的结构体
// 相比于 sync.Once，它提供了更灵活的接口，支持 DoOr 方法
type Once struct {
	done uint32     // 标记是否已执行，使用原子操作
	m    sync.Mutex // 保护执行过程
}

// NewOnce 创建一个新的 Once 实例
func NewOnce() *Once {
	return &Once{}
}

// Do 执行函数 f，确保 f 只会被执行一次
// 如果已经执行过，则直接返回
// 如果正在执行，其他 goroutine 会阻塞等待执行完成
func (o *Once) Do(f func()) {
	// 快速检查是否已执行（无锁检查）
	if atomic.LoadUint32(&o.done) == 1 {
		return
	}

	// 进入临界区
	o.m.Lock()
	defer o.m.Unlock()

	// 双重检查锁定模式（Double-Checked Locking）
	if o.done == 0 {
		defer atomic.StoreUint32(&o.done, 1)
		f()
	}
}

// DoOr 尝试执行函数 f，如果已经执行过则执行回调函数 fallback
// 如果正在执行，其他 goroutine 会执行回调函数 fallback
// 这与 Do 的区别在于：Do 会阻塞等待，DoOr 会立即执行回调
func (o *Once) DoOr(f func(), fallback func()) {
	// 快速检查是否已执行
	if atomic.LoadUint32(&o.done) == 1 {
		if fallback != nil {
			fallback()
		}
		return
	}

	// 尝试获取锁，不阻塞
	if o.m.TryLock() {
		defer o.m.Unlock()

		// 再次检查（可能在等待锁的过程中被其他 goroutine 执行了）
		if o.done == 0 {
			defer atomic.StoreUint32(&o.done, 1)
			f()
			return
		}

		// 已经执行过了
		if fallback != nil {
			fallback()
		}
	} else {
		// 无法获取锁，说明有其他 goroutine 正在执行，执行回调
		if fallback != nil {
			fallback()
		}
	}
}

// Done 返回是否已经执行过
func (o *Once) Done() bool {
	return atomic.LoadUint32(&o.done) == 1
}

// Reset 重置 Once 状态，使其可以再次执行
// 注意：这个方法不是并发安全的，只应该在确保没有并发访问时使用
func (o *Once) Reset() {
	o.m.Lock()
	defer o.m.Unlock()
	atomic.StoreUint32(&o.done, 0)
}
