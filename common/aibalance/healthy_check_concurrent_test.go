package aibalance

import (
	"sync"
	"testing"
	"time"
)

// TestHighConcurrentHealthCheck 测试高并发场景下的健康检查性能和稳定性
func TestHighConcurrentHealthCheck(t *testing.T) {
	// 跳过常规测试，只在特定标记下运行
	if testing.Short() {
		t.Skip("跳过高并发测试，使用 -run=TestHighConcurrentHealthCheck 运行此测试")
	}

	// 创建负载均衡器
	balancer, err := NewBalancer("")
	if err != nil {
		t.Fatalf("无法创建 Balancer 实例: %v", err)
	}
	defer balancer.Close()

	// 创建健康检查管理器，设置为5分钟检查间隔
	manager := NewHealthCheckManager(balancer)
	manager.SetCheckInterval(5 * time.Minute)

	// 测试并发创建和获取健康检查结果
	t.Run("High Concurrent ShouldCheck and RecordCheck", func(t *testing.T) {
		// 设置较高的并发量和操作数
		const providerCount = 1000
		const goroutineCount = 100

		// 准备通道用于同步所有协程同时开始
		startSignal := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(goroutineCount)

		// 启动多个协程等待信号
		for i := 0; i < goroutineCount; i++ {
			go func(id int) {
				defer wg.Done()

				// 等待开始信号
				<-startSignal

				// 每个协程处理一部分提供者
				start := (id * providerCount) / goroutineCount
				end := ((id + 1) * providerCount) / goroutineCount

				for j := start; j < end; j++ {
					providerID := uint(j % 50) // 使用有限的提供者ID模拟冲突

					// 随机进行读取或写入操作
					if j%3 == 0 {
						manager.ShouldCheck(providerID)
					} else if j%3 == 1 {
						manager.RecordCheck(providerID)
					} else {
						manager.GetHealthCheckResult(int(providerID))
					}
				}
			}(i)
		}

		// 发送信号，所有协程同时开始
		close(startSignal)

		// 等待所有协程完成
		wg.Wait()
	})

	// 测试多个检查同时进行
	t.Run("High Concurrent Provider Health Checking", func(t *testing.T) {
		// 获取可用的提供者列表
		providers := balancer.GetProviders()
		if len(providers) == 0 {
			t.Skip("跳过测试：没有可用的提供者")
			return
		}

		// 创建测试用提供者（如果没有足够的真实提供者）
		testProviders := make([]*Provider, 0, 10)
		for i := 0; i < 10; i++ {
			if i < len(providers) {
				testProviders = append(testProviders, providers[i])
			} else {
				// 创建一个模拟提供者
				provider := &Provider{
					ModelName:   "test-model",
					TypeName:    "test-type",
					DomainOrURL: "http://example.com",
					APIKey:      "test-key",
				}
				testProviders = append(testProviders, provider)
			}
		}

		// 记录开始时间
		startTime := time.Now()

		// 并发执行健康检查
		const checkCount = 20
		var wg sync.WaitGroup
		wg.Add(checkCount)

		// 创建信号量限制最大并发数
		semaphore := make(chan struct{}, 10)

		for i := 0; i < checkCount; i++ {
			go func(idx int) {
				defer wg.Done()

				// 获取信号量
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				// 选择一个提供者
				provider := testProviders[idx%len(testProviders)]

				// 执行健康检查
				_, err := CheckProviderHealth(provider)
				if err != nil {
					t.Logf("健康检查出错 [%d]: %v", idx, err)
				}
			}(i)
		}

		// 等待所有健康检查完成
		wg.Wait()

		// 计算总耗时
		duration := time.Since(startTime)
		t.Logf("完成 %d 个并发健康检查，总耗时: %v, 平均每个: %v",
			checkCount, duration, duration/time.Duration(checkCount))
	})

	// 测试同时启动多个健康检查调度器
	t.Run("Concurrent Health Check Scheduler", func(t *testing.T) {
		// 重置 healthCheckSchedulerStarted
		healthCheckSchedulerStarted = sync.Once{}

		// 创建多个 balancer 实例
		balancers := make([]*Balancer, 5)
		for i := 0; i < 5; i++ {
			b, err := NewBalancer("")
			if err != nil {
				t.Fatalf("无法创建 Balancer 实例 %d: %v", i, err)
			}
			defer b.Close()
			balancers[i] = b
		}

		// 同时启动多个健康检查调度器
		var wg sync.WaitGroup
		wg.Add(len(balancers))

		for i, b := range balancers {
			go func(idx int, balancer *Balancer) {
				defer wg.Done()
				// 尝试启动健康检查调度器
				StartHealthCheckScheduler(balancer, 5*time.Minute)
				t.Logf("尝试启动健康检查调度器 %d", idx)
			}(i, b)
		}

		wg.Wait()
		t.Log("所有健康检查调度器启动尝试完成")

		// 等待一段时间，确保健康检查有机会运行
		time.Sleep(500 * time.Millisecond)
	})
}
