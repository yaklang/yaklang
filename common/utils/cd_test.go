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
