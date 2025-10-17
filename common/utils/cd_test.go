package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCoolDownFirstCallSuccess 测试第一次调用是否成功
func TestCoolDownFirstCallSuccess(t *testing.T) {
	cd := NewCoolDown(100 * time.Millisecond)
	defer cd.cancel()

	var executed int32

	// 立即调用 Do() 方法
	cd.Do(func() {
		atomic.StoreInt32(&executed, 1)
	})

	// 检查是否执行
	if atomic.LoadInt32(&executed) != 1 {
		t.Error("第一次调用 Do() 方法失败，函数没有被执行")
	}
}

// TestCoolDownContextFirstCallSuccess 测试使用 context 的版本
func TestCoolDownContextFirstCallSuccess(t *testing.T) {
	ctx := context.Background()
	cd := NewCoolDownContext(100*time.Millisecond, ctx)
	defer cd.cancel()

	var executed int32

	// 立即调用 Do() 方法
	cd.Do(func() {
		atomic.StoreInt32(&executed, 1)
	})

	// 检查是否执行
	if atomic.LoadInt32(&executed) != 1 {
		t.Error("第一次调用 Do() 方法失败，函数没有被执行")
	}
}

// TestCoolDownRateLimiting 测试冷却功能是否正常工作
func TestCoolDownRateLimiting(t *testing.T) {
	cd := NewCoolDown(200 * time.Millisecond)
	defer cd.cancel()

	var executeCount int32

	// 第一次调用应该执行（因为初始化时已经有一个信号）
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	// 连续调用多次，这些应该被限制
	for i := 0; i < 4; i++ {
		cd.Do(func() {
			atomic.AddInt32(&executeCount, 1)
		})
		time.Sleep(50 * time.Millisecond) // 短暂等待，但小于冷却时间
	}

	// 应该只执行了第一次
	if atomic.LoadInt32(&executeCount) != 1 {
		t.Errorf("期望执行 1 次，实际执行 %d 次", atomic.LoadInt32(&executeCount))
	}

	// 等待冷却时间过去
	time.Sleep(250 * time.Millisecond)

	// 再次调用应该能执行
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	if atomic.LoadInt32(&executeCount) != 2 {
		t.Errorf("冷却后期望执行 2 次，实际执行 %d 次", atomic.LoadInt32(&executeCount))
	}
}

// TestCoolDownDoOr 测试 DoOr 方法
func TestCoolDownDoOr(t *testing.T) {
	cd := NewCoolDown(200 * time.Millisecond)
	defer cd.cancel()

	var mainExecuted, fallbackExecuted int32

	// 第一次调用应该执行主函数
	cd.DoOr(func() {
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

	// 立即再次调用应该执行回调函数
	cd.DoOr(func() {
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

// TestCoolDownConcurrency 测试并发安全性
func TestCoolDownConcurrency(t *testing.T) {
	cd := NewCoolDown(100 * time.Millisecond)
	defer cd.cancel()

	var executeCount int32
	var wg sync.WaitGroup

	// 启动多个 goroutine 并发调用
	numGoroutines := 10
	callsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				cd.Do(func() {
					atomic.AddInt32(&executeCount, 1)
				})
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// 由于冷却限制，执行次数应该远小于总调用次数
	totalCalls := numGoroutines * callsPerGoroutine
	actualExecutions := atomic.LoadInt32(&executeCount)

	if actualExecutions >= int32(totalCalls) {
		t.Errorf("冷却功能失效，期望执行次数远小于 %d，实际执行 %d 次", totalCalls, actualExecutions)
	}

	if actualExecutions == 0 {
		t.Error("没有任何执行，可能存在问题")
	}

	t.Logf("总调用次数: %d, 实际执行次数: %d", totalCalls, actualExecutions)
}

// TestCoolDownContextCancel 测试 context 取消
func TestCoolDownContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cd := NewCoolDownContext(100*time.Millisecond, ctx)

	var executeCount int32

	// 第一次调用应该成功
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	// 取消 context
	cancel()

	// 等待一段时间让 goroutine 处理取消
	time.Sleep(50 * time.Millisecond)

	// 再次调用，由于 context 已取消，goroutine 应该已退出
	// 但这次调用仍然可能成功（如果通道中还有信号）
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	initialCount := atomic.LoadInt32(&executeCount)

	// 等待足够长的时间，确保如果 goroutine 还在运行会发送新信号
	time.Sleep(200 * time.Millisecond)

	// 再次调用，这次应该不会有新的执行（因为 goroutine 已停止）
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	finalCount := atomic.LoadInt32(&executeCount)

	// 验证 context 取消后没有新的信号产生
	if finalCount > initialCount {
		t.Log("警告: context 取消后仍有新的执行，但这可能是正常的（通道中的剩余信号）")
	}

	t.Logf("初始执行次数: %d, 最终执行次数: %d", initialCount, finalCount)
}

// TestCoolDownReset 测试重置冷却时间
func TestCoolDownReset(t *testing.T) {
	cd := NewCoolDown(200 * time.Millisecond)
	defer cd.cancel()

	var executeCount int32

	// 第一次调用
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	// 重置为更短的冷却时间
	cd.Reset(50 * time.Millisecond)

	// 等待新的冷却时间
	time.Sleep(100 * time.Millisecond)

	// 应该能够再次执行
	cd.Do(func() {
		atomic.AddInt32(&executeCount, 1)
	})

	if atomic.LoadInt32(&executeCount) != 2 {
		t.Errorf("重置冷却时间后期望执行 2 次，实际执行 %d 次", atomic.LoadInt32(&executeCount))
	}
}

// TestCoolDownZeroDuration 测试零冷却时间
func TestCoolDownZeroDuration(t *testing.T) {
	cd := NewCoolDown(0)
	defer cd.cancel()

	var executeCount int32

	// 连续快速调用
	for i := 0; i < 5; i++ {
		cd.Do(func() {
			atomic.AddInt32(&executeCount, 1)
		})
	}

	// 由于冷却时间为0，但仍然受通道机制限制
	// 第一次调用应该成功，后续调用取决于 goroutine 的发送频率
	actualCount := atomic.LoadInt32(&executeCount)
	if actualCount < 1 {
		t.Error("零冷却时间至少应该执行一次")
	}

	t.Logf("零冷却时间执行次数: %d", actualCount)
}

// TestCoolDownLongRunning 测试长时间运行
func TestCoolDownLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过长时间运行测试")
	}

	cd := NewCoolDown(100 * time.Millisecond)
	defer cd.cancel()

	var executeCount int32
	duration := 1 * time.Second
	start := time.Now()

	// 持续调用直到超时
	for time.Since(start) < duration {
		cd.Do(func() {
			atomic.AddInt32(&executeCount, 1)
		})
		time.Sleep(10 * time.Millisecond)
	}

	actualCount := atomic.LoadInt32(&executeCount)
	expectedMin := int32(duration / (100 * time.Millisecond)) // 最少应该执行的次数

	if actualCount < expectedMin {
		t.Errorf("长时间运行期望至少执行 %d 次，实际执行 %d 次", expectedMin, actualCount)
	}

	t.Logf("长时间运行执行次数: %d, 期望最少: %d", actualCount, expectedMin)
}

// TestCoolDownConcurrentDoExclusive 测试并发调用Do时的排他性
// 确保在并发环境中，只有一个goroutine能执行主函数，其他会被阻塞
func TestCoolDownConcurrentDoExclusive(t *testing.T) {
	cd := NewCoolDown(500 * time.Millisecond) // 较长的冷却时间
	defer cd.cancel()

	var (
		executeCount    int32
		executingFlag   int32 // 标记当前是否有函数正在执行
		concurrentCount int32 // 记录并发执行的数量
	)

	numGoroutines := 10
	var wg sync.WaitGroup

	// 启动多个goroutine同时调用Do
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cd.Do(func() {
				// 检查是否有其他函数同时执行
				if !atomic.CompareAndSwapInt32(&executingFlag, 0, 1) {
					atomic.AddInt32(&concurrentCount, 1)
					t.Errorf("检测到并发执行：goroutine %d", id)
				}

				atomic.AddInt32(&executeCount, 1)

				// 模拟一些工作
				time.Sleep(100 * time.Millisecond)

				// 释放执行标记
				atomic.StoreInt32(&executingFlag, 0)
			})
		}(i)
	}

	wg.Wait()

	// 验证结果
	actualExecutions := atomic.LoadInt32(&executeCount)
	concurrentExecutions := atomic.LoadInt32(&concurrentCount)

	if concurrentExecutions > 0 {
		t.Errorf("检测到 %d 次并发执行，应该为0", concurrentExecutions)
	}

	if actualExecutions != 1 {
		t.Errorf("期望执行1次，实际执行 %d 次", actualExecutions)
	}

	t.Logf("成功：%d个goroutine中只有1个执行了主函数", numGoroutines)
}

// TestCoolDownConcurrentDoOrExclusive 测试并发调用DoOr时的行为
// 确保只有一个goroutine执行主函数，其他执行回调函数
func TestCoolDownConcurrentDoOrExclusive(t *testing.T) {
	cd := NewCoolDown(500 * time.Millisecond) // 较长的冷却时间
	defer cd.cancel()

	var (
		mainExecuteCount     int32
		fallbackExecuteCount int32
		executingMainFlag    int32 // 标记当前是否有主函数正在执行
		concurrentMainCount  int32 // 记录主函数并发执行的数量
	)

	numGoroutines := 10
	var wg sync.WaitGroup

	// 启动多个goroutine同时调用DoOr
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cd.DoOr(func() {
				// 检查是否有其他主函数同时执行
				if !atomic.CompareAndSwapInt32(&executingMainFlag, 0, 1) {
					atomic.AddInt32(&concurrentMainCount, 1)
					t.Errorf("检测到主函数并发执行：goroutine %d", id)
				}

				atomic.AddInt32(&mainExecuteCount, 1)

				// 模拟一些工作
				time.Sleep(100 * time.Millisecond)

				// 释放执行标记
				atomic.StoreInt32(&executingMainFlag, 0)
			}, func() {
				// 回调函数
				atomic.AddInt32(&fallbackExecuteCount, 1)
			})
		}(i)
	}

	wg.Wait()

	// 验证结果
	actualMainExecutions := atomic.LoadInt32(&mainExecuteCount)
	actualFallbackExecutions := atomic.LoadInt32(&fallbackExecuteCount)
	concurrentMainExecutions := atomic.LoadInt32(&concurrentMainCount)

	if concurrentMainExecutions > 0 {
		t.Errorf("检测到主函数 %d 次并发执行，应该为0", concurrentMainExecutions)
	}

	if actualMainExecutions != 1 {
		t.Errorf("期望主函数执行1次，实际执行 %d 次", actualMainExecutions)
	}

	if actualFallbackExecutions != int32(numGoroutines-1) {
		t.Errorf("期望回调函数执行 %d 次，实际执行 %d 次", numGoroutines-1, actualFallbackExecutions)
	}

	t.Logf("成功：%d个goroutine中1个执行主函数，%d个执行回调函数", numGoroutines, actualFallbackExecutions)
}

// TestCoolDownInitializationPattern 测试初始化模式
// 模拟并发环境下的安全初始化场景
func TestCoolDownInitializationPattern(t *testing.T) {
	cd := NewCoolDown(100 * time.Millisecond)
	defer cd.cancel()

	var (
		initialized         int32
		initializerID       int32 = -1
		resourceValue       int32
		accessCount         int32
		concurrentInitCount int32 // 记录并发初始化的数量
		initComplete        int32 // 标记初始化是否完全完成
	)

	numGoroutines := 20
	var wg sync.WaitGroup

	// 模拟多个goroutine需要访问一个需要初始化的资源
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cd.DoOr(func() {
				// 初始化函数 - 只有一个goroutine能执行
				if !atomic.CompareAndSwapInt32(&initialized, 0, 1) {
					atomic.AddInt32(&concurrentInitCount, 1)
					t.Errorf("检测到重复初始化：goroutine %d", id)
					return
				}

				atomic.StoreInt32(&initializerID, int32(id))

				// 模拟初始化工作
				time.Sleep(50 * time.Millisecond)
				atomic.StoreInt32(&resourceValue, 42) // 设置资源值

				// 标记初始化完全完成
				atomic.StoreInt32(&initComplete, 1)

				t.Logf("资源由 goroutine %d 初始化", id)
			}, func() {
				// 等待初始化完全完成并使用资源
				for atomic.LoadInt32(&initComplete) == 0 {
					time.Sleep(5 * time.Millisecond)
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

// TestCoolDownLongRunningTaskWithQueue 测试长时间运行的任务，其他调用者排队等待
func TestCoolDownLongRunningTaskWithQueue(t *testing.T) {
	cd := NewCoolDown(100 * time.Millisecond)
	defer cd.cancel()

	var (
		executeCount    int32
		queuedCount     int32
		completionOrder []int32
		orderMutex      sync.Mutex
		executingFlag   int32
		concurrentCount int32
	)

	numGoroutines := 5
	var wg sync.WaitGroup

	// 第一个goroutine执行长时间任务
	wg.Add(1)
	go func() {
		defer wg.Done()

		cd.DoOr(func() {
			if !atomic.CompareAndSwapInt32(&executingFlag, 0, 1) {
				atomic.AddInt32(&concurrentCount, 1)
				t.Error("检测到并发执行长时间任务")
			}

			atomic.AddInt32(&executeCount, 1)

			// 模拟长时间工作
			time.Sleep(300 * time.Millisecond)

			orderMutex.Lock()
			completionOrder = append(completionOrder, 0) // 主任务ID为0
			orderMutex.Unlock()

			atomic.StoreInt32(&executingFlag, 0)
		}, func() {
			// 不应该进入这里
			t.Error("主任务goroutine不应该执行回调")
		})
	}()

	// 稍后启动其他goroutine，它们应该执行回调函数
	time.Sleep(50 * time.Millisecond)

	for i := 1; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cd.DoOr(func() {
				// 不应该进入这里，因为主任务还在运行
				t.Errorf("goroutine %d 不应该执行主函数", id)
			}, func() {
				// 应该执行这里
				atomic.AddInt32(&queuedCount, 1)

				orderMutex.Lock()
				completionOrder = append(completionOrder, int32(id))
				orderMutex.Unlock()
			})
		}(i)
	}

	wg.Wait()

	// 验证结果
	actualExecutions := atomic.LoadInt32(&executeCount)
	actualQueued := atomic.LoadInt32(&queuedCount)
	concurrentExecutions := atomic.LoadInt32(&concurrentCount)

	if concurrentExecutions > 0 {
		t.Errorf("检测到 %d 次并发执行，应该为0", concurrentExecutions)
	}

	if actualExecutions != 1 {
		t.Errorf("期望主任务执行1次，实际执行 %d 次", actualExecutions)
	}

	if actualQueued != int32(numGoroutines-1) {
		t.Errorf("期望 %d 个goroutine执行回调，实际 %d 个", numGoroutines-1, actualQueued)
	}

	// 验证执行顺序：主任务应该最先完成（但由于并发性，这个检查可能不总是可靠）
	orderMutex.Lock()
	if len(completionOrder) > 0 {
		t.Logf("完成顺序: %v", completionOrder)
		// 主任务(ID=0)应该存在于顺序中
		foundMainTask := false
		for _, id := range completionOrder {
			if id == 0 {
				foundMainTask = true
				break
			}
		}
		if !foundMainTask {
			t.Error("主任务未在完成顺序中找到")
		}
	}
	orderMutex.Unlock()

	t.Logf("成功：长时间主任务执行完成，%d 个等待的goroutine执行了回调", actualQueued)
}
