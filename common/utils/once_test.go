package utils

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestOnceBasicFunctionality 测试基本功能
func TestOnceBasicFunctionality(t *testing.T) {
	once := NewOnce()
	var executed int32

	// 第一次调用应该执行
	once.Do(func() {
		atomic.AddInt32(&executed, 1)
	})

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", atomic.LoadInt32(&executed))
	}

	// 第二次调用不应该执行
	once.Do(func() {
		atomic.AddInt32(&executed, 1)
	})

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", atomic.LoadInt32(&executed))
	}

	// 检查 Done 状态
	if !once.Done() {
		t.Error("Once 应该标记为已执行")
	}
}

// TestOnceConcurrentExecution 测试并发执行，确保只执行一次
func TestOnceConcurrentExecution(t *testing.T) {
	once := NewOnce()
	var executeCount int32
	var executingFlag int32
	var concurrentCount int32

	numGoroutines := 100
	var wg sync.WaitGroup

	// 启动多个 goroutine 并发调用
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			once.Do(func() {
				// 检查是否有其他函数同时执行
				if !atomic.CompareAndSwapInt32(&executingFlag, 0, 1) {
					atomic.AddInt32(&concurrentCount, 1)
					t.Errorf("检测到并发执行：goroutine %d", id)
				}

				atomic.AddInt32(&executeCount, 1)

				// 模拟一些工作
				time.Sleep(50 * time.Millisecond)

				// 释放执行标记
				atomic.StoreInt32(&executingFlag, 0)
			})
		}(i)
	}

	wg.Wait()

	actualExecutions := atomic.LoadInt32(&executeCount)
	concurrentExecutions := atomic.LoadInt32(&concurrentCount)

	if concurrentExecutions > 0 {
		t.Errorf("检测到 %d 次并发执行，应该为0", concurrentExecutions)
	}

	if actualExecutions != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", actualExecutions)
	}

	t.Logf("成功：%d个goroutine中只有1个执行了函数", numGoroutines)
}

// TestOnceDoOr 测试 DoOr 方法
func TestOnceDoOr(t *testing.T) {
	once := NewOnce()
	var mainExecuted, fallbackExecuted int32

	// 第一次调用应该执行主函数
	once.DoOr(func() {
		atomic.StoreInt32(&mainExecuted, 1)
	}, func() {
		atomic.StoreInt32(&fallbackExecuted, 1)
	})

	if atomic.LoadInt32(&mainExecuted) != 1 {
		t.Error("第一次调用应该执行主函数")
	}
	if atomic.LoadInt32(&fallbackExecuted) != 0 {
		t.Error("第一次调用不应该执行回调函数")
	}

	// 第二次调用应该执行回调函数
	once.DoOr(func() {
		atomic.StoreInt32(&mainExecuted, 2)
	}, func() {
		atomic.StoreInt32(&fallbackExecuted, 1)
	})

	if atomic.LoadInt32(&mainExecuted) != 1 {
		t.Error("第二次调用不应该执行主函数")
	}
	if atomic.LoadInt32(&fallbackExecuted) != 1 {
		t.Error("第二次调用应该执行回调函数")
	}
}

// TestOnceConcurrentDoOr 测试并发 DoOr 调用
func TestOnceConcurrentDoOr(t *testing.T) {
	once := NewOnce()
	var mainExecuteCount int32
	var fallbackExecuteCount int32
	var executingMainFlag int32
	var concurrentMainCount int32

	numGoroutines := 50
	var wg sync.WaitGroup

	// 启动多个 goroutine 同时调用 DoOr
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			once.DoOr(func() {
				// 检查是否有其他主函数同时执行
				if !atomic.CompareAndSwapInt32(&executingMainFlag, 0, 1) {
					atomic.AddInt32(&concurrentMainCount, 1)
					t.Errorf("检测到主函数并发执行：goroutine %d", id)
				}

				atomic.AddInt32(&mainExecuteCount, 1)

				// 模拟一些工作
				time.Sleep(30 * time.Millisecond)

				// 释放执行标记
				atomic.StoreInt32(&executingMainFlag, 0)
			}, func() {
				// 回调函数
				atomic.AddInt32(&fallbackExecuteCount, 1)
			})
		}(i)
	}

	wg.Wait()

	actualMainExecutions := atomic.LoadInt32(&mainExecuteCount)
	actualFallbackExecutions := atomic.LoadInt32(&fallbackExecuteCount)
	concurrentMainExecutions := atomic.LoadInt32(&concurrentMainCount)

	if concurrentMainExecutions > 0 {
		t.Errorf("检测到主函数 %d 次并发执行，应该为0", concurrentMainExecutions)
	}

	if actualMainExecutions != 1 {
		t.Errorf("期望主函数执行1次，实际执行 %d 次", actualMainExecutions)
	}

	if actualFallbackExecutions < 1 {
		t.Errorf("期望至少有一些回调函数执行，实际执行 %d 次", actualFallbackExecutions)
	}

	t.Logf("成功：%d个goroutine中1个执行主函数，%d个执行回调函数", numGoroutines, actualFallbackExecutions)
}

// TestOnceInitializationPattern 测试初始化模式
func TestOnceInitializationPattern(t *testing.T) {
	once := NewOnce()
	var initialized int32
	var initializerID int32 = -1
	var resourceValue int32
	var accessCount int32
	var concurrentInitCount int32

	numGoroutines := 30
	var wg sync.WaitGroup

	// 模拟多个 goroutine 需要访问一个需要初始化的资源
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			once.DoOr(func() {
				// 初始化函数 - 只有一个 goroutine 能执行
				if !atomic.CompareAndSwapInt32(&initialized, 0, 1) {
					atomic.AddInt32(&concurrentInitCount, 1)
					t.Errorf("检测到重复初始化：goroutine %d", id)
					return
				}

				atomic.StoreInt32(&initializerID, int32(id))

				// 模拟初始化工作
				time.Sleep(20 * time.Millisecond)
				atomic.StoreInt32(&resourceValue, 42)

				t.Logf("资源由 goroutine %d 初始化", id)
			}, func() {
				// 等待初始化完成并使用资源
				for !once.Done() {
					time.Sleep(1 * time.Millisecond)
				}

				// 访问已初始化的资源
				value := atomic.LoadInt32(&resourceValue)
				if value != 42 {
					t.Errorf("goroutine %d: 资源值不正确，期望42，得到%d", id, value)
				}
				atomic.AddInt32(&accessCount, 1)
			})
		}(i)
	}

	wg.Wait()

	// 验证结果
	isInitialized := atomic.LoadInt32(&initialized)
	finalResourceValue := atomic.LoadInt32(&resourceValue)
	totalAccess := atomic.LoadInt32(&accessCount)
	concurrentInits := atomic.LoadInt32(&concurrentInitCount)
	initID := atomic.LoadInt32(&initializerID)

	if concurrentInits > 0 {
		t.Errorf("检测到 %d 次并发初始化，应该为0", concurrentInits)
	}

	if isInitialized != 1 {
		t.Error("资源未正确初始化")
	}

	if finalResourceValue != 42 {
		t.Errorf("资源值不正确，期望42，得到%d", finalResourceValue)
	}

	if totalAccess != int32(numGoroutines-1) {
		t.Errorf("期望 %d 次资源访问，实际 %d 次", numGoroutines-1, totalAccess)
	}

	t.Logf("成功：资源由 goroutine %d 初始化，其他 %d 个goroutine正确访问了资源", initID, totalAccess)
}

// TestOnceReset 测试重置功能
func TestOnceReset(t *testing.T) {
	once := NewOnce()
	var executeCount int32

	// 第一次执行
	once.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	if atomic.LoadInt32(&executeCount) != 1 {
		t.Errorf("第一次执行失败，期望1次，实际 %d 次", atomic.LoadInt32(&executeCount))
	}

	if !once.Done() {
		t.Error("应该标记为已执行")
	}

	// 重置
	once.Reset()

	if once.Done() {
		t.Error("重置后不应该标记为已执行")
	}

	// 再次执行
	once.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	if atomic.LoadInt32(&executeCount) != 2 {
		t.Errorf("重置后再次执行失败，期望2次，实际 %d 次", atomic.LoadInt32(&executeCount))
	}
}

// TestOnceWithSyncOnceComparison 与 sync.Once 对比测试
func TestOnceWithSyncOnceComparison(t *testing.T) {
	// 使用我们的 Once
	ourOnce := NewOnce()
	var ourExecuteCount int32

	// 使用标准库的 sync.Once
	var syncOnce sync.Once
	var syncExecuteCount int32

	numGoroutines := 100
	var wg sync.WaitGroup

	// 测试我们的 Once
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ourOnce.Do(func() {
				atomic.AddInt32(&ourExecuteCount, 1)
			})
		}()
	}

	// 测试标准库的 sync.Once
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			syncOnce.Do(func() {
				atomic.AddInt32(&syncExecuteCount, 1)
			})
		}()
	}

	wg.Wait()

	// 验证结果应该一致
	if atomic.LoadInt32(&ourExecuteCount) != 1 {
		t.Errorf("我们的 Once 执行次数错误，期望1次，实际 %d 次", atomic.LoadInt32(&ourExecuteCount))
	}

	if atomic.LoadInt32(&syncExecuteCount) != 1 {
		t.Errorf("sync.Once 执行次数错误，期望1次，实际 %d 次", atomic.LoadInt32(&syncExecuteCount))
	}

	t.Logf("对比测试成功：我们的 Once 和 sync.Once 都正确执行了1次")
}

// TestOnceDoOrWithNilCallback 测试 DoOr 的 nil 回调
func TestOnceDoOrWithNilCallback(t *testing.T) {
	once := NewOnce()
	var executed int32

	// 第一次调用应该执行主函数
	once.DoOr(func() {
		atomic.AddInt32(&executed, 1)
	}, nil)

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", atomic.LoadInt32(&executed))
	}

	// 第二次调用，回调为 nil，应该不崩溃
	once.DoOr(func() {
		atomic.AddInt32(&executed, 1)
	}, nil)

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", atomic.LoadInt32(&executed))
	}
}

// TestOnceStressTest 压力测试
func TestOnceStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过压力测试")
	}

	once := NewOnce()
	var executeCount int32
	numGoroutines := 1000
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			once.Do(func() {
				atomic.AddInt32(&executeCount, 1)
				time.Sleep(1 * time.Millisecond) // 模拟一些工作
			})
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	if atomic.LoadInt32(&executeCount) != 1 {
		t.Errorf("压力测试失败，期望执行1次，实际执行 %d 次", atomic.LoadInt32(&executeCount))
	}

	t.Logf("压力测试成功：%d个goroutine，耗时 %v，只执行了1次", numGoroutines, duration)
}
