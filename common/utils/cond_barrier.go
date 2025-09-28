package utils

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
)

/*
CondBarrier 提供了一个强大的条件屏障管理系统，支持分层等待和重入功能。

主要特性：
1. 命名条件管理：使用字符串ID管理不同的屏障条件
2. 分层等待：支持按需等待特定条件完成
3. 多条件等待：可同时等待多个条件
4. 重入功能：类似 sync.WaitGroup，支持多次调用同名屏障
5. 取消支持：可取消所有等待操作
6. 上下文支持：支持超时和取消机制
7. 并发安全：所有操作都是线程安全的

基本用法示例：

	cb := NewCondBarrier()

	// 启动并发任务
	go func(){
		cond1 := cb.CreateBarrier("condition1")
		defer cond1.Done()
		// 执行工作...
	}()

	go func(){
		cond2 := cb.CreateBarrier("condition2")
		defer cond2.Done()
		// 执行工作...
	}()

	// 分层等待
	cb.Wait("condition1")        // 等待 condition1 完成
	cb.Wait("condition1", "condition2") // 等待多个条件
	cb.WaitAll()                // 等待所有条件完成
	cb.Wait()                   // 等待所有（空参数时退化为 WaitAll）

重入功能示例（类似 WaitGroup）：

	cb := NewCondBarrier()
	for i := 0; i < 5; i++ {
		go func() {
			barrier := cb.CreateBarrier("workers") // 重入，自动增加计数
			defer barrier.Done()
			// 执行工作...
		}()
	}
	cb.Wait("workers") // 等待所有5个工作者完成

取消功能示例：

	cb := NewCondBarrier()
	go func() {
		cb.Wait("some_condition") // 这个等待会因为Cancel而立即返回
	}()
	cb.Cancel() // 取消所有等待操作

等待未创建条件：

	cb := NewCondBarrier()
	go func() {
		cb.Wait("future_task") // 等待一个还未创建的屏障
	}()
	// 稍后创建屏障
	go func() {
		barrier := cb.CreateBarrier("future_task")
		defer barrier.Done()
		// 执行工作...
	}()
*/

// Barrier 表示单个条件屏障，支持重入（类似 WaitGroup）
type Barrier struct {
	name    string
	counter int64 // 重入计数器
	done    chan struct{}
	cb      *CondBarrier
	mutex   sync.Mutex
}

func (b *Barrier) safeCloseDone() { // 这里不使用once的原因是有reset行为
	b.mutex.Lock()
	defer b.mutex.Unlock()
	select {
	case <-b.done:
	default:
		close(b.done)
	}
}

func (b *Barrier) safeReset() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.done = make(chan struct{})
}

// Done 减少屏障计数，当计数为0时标记该屏障条件已完成
func (b *Barrier) Done() {
	// 使用原子操作减少计数
	newCount := atomic.AddInt64(&b.counter, -1)

	if newCount < 0 {
		// 防止计数器变成负数
		atomic.StoreInt64(&b.counter, 0)
		return
	}

	if newCount == 0 {
		// 需要完成屏障
		b.safeCloseDone()

		// 更新完成状态
		b.cb.mutex.Lock()
		b.cb.completedBarriers[b.name] = true
		b.cb.mutex.Unlock()
	}
}

// Add 增加屏障计数（重入功能）
func (b *Barrier) Add(delta int) {
	for {
		oldCount := atomic.LoadInt64(&b.counter)
		newCount := oldCount + int64(delta)

		if newCount < 0 {
			newCount = 0
		}

		// 使用 CAS 操作确保原子性
		if atomic.CompareAndSwapInt64(&b.counter, oldCount, newCount) {
			wasZero := oldCount == 0
			shouldComplete := newCount == 0
			shouldReset := wasZero && delta > 0

			if shouldReset {
				// 需要重置状态 - 这里仍需要锁保护，因为涉及到 channel 重新创建
				b.safeReset()

				// 如果之前已经完成，需要重置 completedBarriers 状态
				b.cb.mutex.Lock()
				delete(b.cb.completedBarriers, b.name)
				b.cb.mutex.Unlock()
			}

			if shouldComplete && !shouldReset {
				b.safeCloseDone()

				b.cb.mutex.Lock()
				b.cb.completedBarriers[b.name] = true
				b.cb.mutex.Unlock()
			}

			break
		}
		// CAS 失败，重试
	}
}

// CondBarrier 条件屏障管理器
type CondBarrier struct {
	ctx               context.Context
	cancel            context.CancelFunc
	mutex             sync.RWMutex
	barriers          map[string]*Barrier
	completedBarriers map[string]bool
	waiters           map[string][]chan struct{} // 等待指定屏障的 channel 列表
	cancelled         bool                       // 是否已取消
}

// NewCondBarrier 创建新的条件屏障管理器
func NewCondBarrier() *CondBarrier {
	ctx, cancel := context.WithCancel(context.Background())
	return &CondBarrier{
		ctx:               ctx,
		cancel:            cancel,
		barriers:          make(map[string]*Barrier),
		completedBarriers: make(map[string]bool),
		waiters:           make(map[string][]chan struct{}),
	}
}

// NewCondBarrierContext 创建带上下文的条件屏障管理器
func NewCondBarrierContext(ctx context.Context) *CondBarrier {
	if ctx == nil {
		ctx = context.Background()
	}
	ctxWithCancel, cancel := context.WithCancel(ctx)
	return &CondBarrier{
		ctx:               ctxWithCancel,
		cancel:            cancel,
		barriers:          make(map[string]*Barrier),
		completedBarriers: make(map[string]bool),
		waiters:           make(map[string][]chan struct{}),
	}
}

// CreateBarrier 创建一个命名的屏障条件
func (cb *CondBarrier) CreateBarrier(name string) *Barrier {
	cb.mutex.Lock()

	// 如果已经取消，返回一个已完成的屏障
	if cb.cancelled {
		barrier := &Barrier{
			name:    name,
			counter: 0,
			done:    make(chan struct{}),
			cb:      cb,
		}
		barrier.safeCloseDone()
		cb.completedBarriers[name] = true
		cb.mutex.Unlock()
		return barrier
	}

	// 如果已经存在同名屏障，增加计数并返回现有的
	if barrier, exists := cb.barriers[name]; exists {
		// 先释放 cb.mutex，完全避免嵌套锁
		cb.mutex.Unlock()
		// 使用原子操作增加计数，避免锁竞争
		atomic.AddInt64(&barrier.counter, 1)
		return barrier
	}

	// 创建新的屏障
	barrier := &Barrier{
		name:    name,
		counter: 1, // 初始计数为1
		done:    make(chan struct{}),
		cb:      cb,
	}

	cb.barriers[name] = barrier

	// 通知等待这个屏障的所有 goroutine
	if waitChannels, exists := cb.waiters[name]; exists {
		for _, ch := range waitChannels {
			close(ch) // 通知等待者屏障已创建
		}
		delete(cb.waiters, name) // 清理等待列表
	}

	cb.mutex.Unlock()
	return barrier
}

// Cancel 取消所有等待，使所有Wait操作立即返回
// 调用 Cancel 后：
// - 所有正在等待的 Wait 操作将立即返回 nil（不返回错误）
// - 新创建的屏障将立即处于完成状态
// - 可以安全地多次调用 Cancel
func (cb *CondBarrier) Cancel() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.cancelled {
		return // 避免重复取消
	}

	cb.cancelled = true

	// 完成所有未完成的屏障
	for name, barrier := range cb.barriers {
		if !cb.completedBarriers[name] {
			// 使用原子操作设置计数器为0，确保屏障完成
			atomic.StoreInt64(&barrier.counter, 0)
			barrier.safeCloseDone()
			cb.completedBarriers[name] = true
		}
	}

	// 通知所有等待未创建屏障的 goroutine
	for name, waitChannels := range cb.waiters {
		for _, ch := range waitChannels {
			select {
			case <-ch:
				// 已经关闭
			default:
				close(ch)
			}
		}
		delete(cb.waiters, name)
	}

	// 取消上下文
	cb.cancel()
}

// Wait 等待指定的屏障条件完成
// 参数：
//
//	names - 要等待的屏障名称列表，如果为空则等待所有屏障
//
// 返回：
//
//	error - 只有在上下文超时/取消时才返回错误，Cancel 操作不会返回错误
//
// 特性：
//   - 支持等待尚未创建的屏障（会等待其被创建并完成）
//   - 支持同时等待多个屏障
//   - 空参数时等同于 WaitAll()
//   - 线程安全，可以并发调用
func (cb *CondBarrier) Wait(names ...string) error {
	// 如果没有指定名称，等待所有屏障
	if len(names) == 0 {
		return cb.WaitAll()
	}

	// 等待指定的屏障
	for _, name := range names {
		if err := cb.waitSingle(name); err != nil {
			return err
		}
	}

	return nil
}

// waitSingle 等待单个屏障完成
func (cb *CondBarrier) waitSingle(name string) error {
	for {
		cb.mutex.Lock()

		// 如果已经取消，直接返回成功
		if cb.cancelled {
			cb.mutex.Unlock()
			return nil
		}

		// 检查是否已经完成
		if cb.completedBarriers[name] {
			cb.mutex.Unlock()
			return nil
		}

		// 检查屏障是否存在
		if barrier, exists := cb.barriers[name]; exists {
			done := barrier.done
			cb.mutex.Unlock()

			// 等待屏障完成或上下文取消
			select {
			case <-done:
				return nil
			case <-cb.ctx.Done():
				// 检查是否是因为 Cancel 导致的上下文取消
				cb.mutex.RLock()
				cancelled := cb.cancelled
				cb.mutex.RUnlock()
				if cancelled {
					return nil // Cancel 导致的取消不返回错误
				}
				return cb.ctx.Err() // 其他原因的上下文取消返回错误
			}
		}

		// 屏障不存在，创建等待 channel 并注册
		waitCh := make(chan struct{})
		cb.waiters[name] = append(cb.waiters[name], waitCh)
		cb.mutex.Unlock()

		// 等待屏障被创建、取消或上下文结束
		select {
		case <-waitCh:
			// 屏障已创建或被取消，继续循环检查
			continue
		case <-cb.ctx.Done():
			// 清理等待 channel
			cb.mutex.Lock()
			if waitChannels, exists := cb.waiters[name]; exists {
				for i, ch := range waitChannels {
					if ch == waitCh {
						cb.waiters[name] = append(waitChannels[:i], waitChannels[i+1:]...)
						break
					}
				}
				if len(cb.waiters[name]) == 0 {
					delete(cb.waiters, name)
				}
			}
			cancelled := cb.cancelled
			cb.mutex.Unlock()

			if cancelled {
				return nil // Cancel 导致的取消不返回错误
			}
			return cb.ctx.Err()
		}
	}
}

// WaitAll 等待所有已创建的屏障完成
func (cb *CondBarrier) WaitAll() error {
	for {
		cb.mutex.RLock()
		allCompleted := true
		var waitChannels []<-chan struct{}

		// 检查所有屏障是否完成
		for name, barrier := range cb.barriers {
			if !cb.completedBarriers[name] {
				allCompleted = false
				waitChannels = append(waitChannels, barrier.done)
			}
		}

		// 如果所有屏障都已完成，直接返回
		if allCompleted {
			cb.mutex.RUnlock()
			return nil
		}

		cb.mutex.RUnlock()

		// 如果没有要等待的屏障，直接返回
		if len(waitChannels) == 0 {
			return nil
		}

		// 等待任何一个屏障完成，然后重新检查
		if err := cb.waitAnyChannel(waitChannels); err != nil {
			return err
		}
	}
}

// WaitContext 等待指定的屏障条件完成，支持传入的上下文
// 参数：
//
//	ctx - 传入的上下文，用于额外的取消/超时控制
//	names - 要等待的屏障名称列表，如果为空则等待所有屏障
//
// 返回：
//
//	error - 在上下文超时/取消时返回错误，Cancel 操作不会返回错误
//
// 特性：
//   - 同时受到 CondBarrier 自身上下文和传入上下文的管控
//   - 支持等待尚未创建的屏障
//   - 支持同时等待多个屏障
//   - 空参数时等同于 WaitAllContext(ctx)
func (cb *CondBarrier) WaitContext(ctx context.Context, names ...string) error {
	// 如果没有指定名称，等待所有屏障
	if len(names) == 0 {
		return cb.WaitAllContext(ctx)
	}

	// 等待指定的屏障
	for _, name := range names {
		if err := cb.waitSingleContext(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

// WaitAllContext 等待所有已创建的屏障完成，支持传入的上下文
func (cb *CondBarrier) WaitAllContext(ctx context.Context) error {
	combinedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 创建一个 goroutine 来监听 CondBarrier 的上下文
	go func() {
		select {
		case <-cb.ctx.Done():
			cancel()
		case <-combinedCtx.Done():
		}
	}()

	for {
		cb.mutex.RLock()
		allCompleted := true
		var waitChannels []<-chan struct{}

		// 检查所有屏障是否完成
		for name, barrier := range cb.barriers {
			if !cb.completedBarriers[name] {
				allCompleted = false
				waitChannels = append(waitChannels, barrier.done)
			}
		}

		// 如果所有屏障都已完成，直接返回
		if allCompleted {
			cb.mutex.RUnlock()
			return nil
		}

		cb.mutex.RUnlock()

		// 如果没有要等待的屏障，直接返回
		if len(waitChannels) == 0 {
			return nil
		}

		// 等待任何一个屏障完成，然后重新检查
		if err := cb.waitAnyChannelContext(combinedCtx, waitChannels); err != nil {
			return err
		}
	}
}

// waitSingleContext 等待单个屏障完成，支持传入的上下文
func (cb *CondBarrier) waitSingleContext(ctx context.Context, name string) error {
	combinedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 创建一个 goroutine 来监听 CondBarrier 的上下文
	go func() {
		select {
		case <-cb.ctx.Done():
			cancel()
		case <-combinedCtx.Done():
		}
	}()

	for {
		cb.mutex.Lock()

		// 如果已经取消，直接返回成功
		if cb.cancelled {
			cb.mutex.Unlock()
			return nil
		}

		// 检查是否已经完成
		if cb.completedBarriers[name] {
			cb.mutex.Unlock()
			return nil
		}

		// 检查屏障是否存在
		if barrier, exists := cb.barriers[name]; exists {
			done := barrier.done
			cb.mutex.Unlock()

			// 等待屏障完成或上下文取消
			select {
			case <-done:
				return nil
			case <-combinedCtx.Done():
				// 检查是否是因为 Cancel 导致的上下文取消
				cb.mutex.RLock()
				cancelled := cb.cancelled
				cb.mutex.RUnlock()
				if cancelled {
					return nil // Cancel 导致的取消不返回错误
				}
				return combinedCtx.Err() // 返回传入上下文的错误
			}
		}

		// 屏障不存在，创建等待 channel 并注册
		waitCh := make(chan struct{})
		cb.waiters[name] = append(cb.waiters[name], waitCh)
		cb.mutex.Unlock()

		// 等待屏障被创建、取消或上下文结束
		select {
		case <-waitCh:
			// 屏障已创建或被取消，继续循环检查
			continue
		case <-combinedCtx.Done():
			// 清理等待 channel
			cb.mutex.Lock()
			if waitChannels, exists := cb.waiters[name]; exists {
				for i, ch := range waitChannels {
					if ch == waitCh {
						cb.waiters[name] = append(waitChannels[:i], waitChannels[i+1:]...)
						break
					}
				}
				if len(cb.waiters[name]) == 0 {
					delete(cb.waiters, name)
				}
			}
			cancelled := cb.cancelled
			cb.mutex.Unlock()

			if cancelled {
				return nil // Cancel 导致的取消不返回错误
			}
			return combinedCtx.Err()
		}
	}
}

// waitAnyChannelContext 等待任何一个 channel 完成，支持传入的上下文
func (cb *CondBarrier) waitAnyChannelContext(ctx context.Context, channels []<-chan struct{}) error {
	if len(channels) == 0 {
		return nil
	}

	combinedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 创建一个 goroutine 来监听 CondBarrier 的上下文
	go func() {
		select {
		case <-cb.ctx.Done():
			cancel()
		case <-combinedCtx.Done():
		}
	}()

	// 创建一个 select case 来等待任何一个 channel
	cases := make([]reflect.SelectCase, len(channels)+1)

	// 添加组合上下文取消的 case
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(combinedCtx.Done()),
	}

	// 添加所有屏障的 case
	for i, ch := range channels {
		cases[i+1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		}
	}

	chosen, _, _ := reflect.Select(cases)

	// 如果选中的是上下文取消
	if chosen == 0 {
		// 检查是否是因为 CondBarrier 的 Cancel 导致的
		cb.mutex.RLock()
		cancelled := cb.cancelled
		cb.mutex.RUnlock()
		if cancelled {
			return nil // Cancel 导致的取消不返回错误
		}
		return combinedCtx.Err() // 返回传入上下文的错误
	}

	return nil
}

// waitAnyChannel 等待任何一个 channel 完成
func (cb *CondBarrier) waitAnyChannel(channels []<-chan struct{}) error {
	if len(channels) == 0 {
		return nil
	}

	// 创建一个 select case 来等待任何一个 channel
	cases := make([]reflect.SelectCase, len(channels)+1)

	// 添加上下文取消的 case
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(cb.ctx.Done()),
	}

	// 添加所有屏障的 case
	for i, ch := range channels {
		cases[i+1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		}
	}

	chosen, _, _ := reflect.Select(cases)

	// 如果选中的是上下文取消
	if chosen == 0 {
		return cb.ctx.Err()
	}

	return nil
}
