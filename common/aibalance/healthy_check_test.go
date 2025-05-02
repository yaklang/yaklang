package aibalance

import (
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// TestHealthCheckManagerConcurrency 测试健康检查管理器在并发环境下的行为
func TestHealthCheckManagerConcurrency(t *testing.T) {
	// 创建负载均衡器
	balancer, err := NewBalancer("")
	if err != nil {
		t.Fatalf("无法创建 Balancer 实例: %v", err)
	}
	defer balancer.Close()

	// 创建健康检查管理器
	manager := NewHealthCheckManager(balancer)
	manager.SetCheckInterval(5 * time.Minute) // 设置为5分钟间隔

	// 测试并发访问 ShouldCheck 和 RecordCheck
	t.Run("Concurrent ShouldCheck and RecordCheck", func(t *testing.T) {
		const providerCount = 100
		const goroutineCount = 10

		var wg sync.WaitGroup
		wg.Add(goroutineCount)

		for i := 0; i < goroutineCount; i++ {
			go func(id int) {
				defer wg.Done()

				for j := 0; j < providerCount; j++ {
					providerID := uint(j)

					// 交替读取和写入
					if id%2 == 0 {
						// 一半协程读取
						_ = manager.ShouldCheck(providerID)
					} else {
						// 一半协程写入
						manager.RecordCheck(providerID)
					}
				}
			}(i)
		}

		wg.Wait()
	})

	// 测试并发保存和获取检查结果
	t.Run("Concurrent SaveHealthCheckResult and GetHealthCheckResult", func(t *testing.T) {
		const providerCount = 50
		const goroutineCount = 10

		var wg sync.WaitGroup
		wg.Add(goroutineCount)

		for i := 0; i < goroutineCount; i++ {
			go func(id int) {
				defer wg.Done()

				for j := 0; j < providerCount; j++ {
					providerID := j

					if id%2 == 0 {
						// 一半协程读取
						_ = manager.GetHealthCheckResult(providerID)
					} else {
						// 一半协程写入
						// 创建包含 gorm.Model 嵌入字段的 schema.AiProvider
						dbProvider := &schema.AiProvider{}
						// 设置 ID 字段 (在 gorm.Model 中)
						dbProvider.Model.ID = uint(providerID)

						result := &HealthCheckResult{
							Provider:     dbProvider,
							IsHealthy:    true,
							ResponseTime: 100,
						}
						manager.SaveHealthCheckResult(result)
					}
				}
			}(i)
		}

		wg.Wait()
	})

	// 测试同时调用 GetAllHealthCheckResults
	t.Run("Concurrent GetAllHealthCheckResults", func(t *testing.T) {
		const goroutineCount = 20

		// 先添加一些结果
		for i := 0; i < 30; i++ {
			// 创建包含 gorm.Model 嵌入字段的 schema.AiProvider
			dbProvider := &schema.AiProvider{}
			// 设置 ID 字段 (在 gorm.Model 中)
			dbProvider.Model.ID = uint(i)

			result := &HealthCheckResult{
				Provider:     dbProvider,
				IsHealthy:    i%2 == 0,
				ResponseTime: int64(100 + i*10),
			}
			manager.SaveHealthCheckResult(result)
		}

		var wg sync.WaitGroup
		wg.Add(goroutineCount)

		for i := 0; i < goroutineCount; i++ {
			go func() {
				defer wg.Done()
				results := manager.GetAllHealthCheckResults()
				if len(results) == 0 {
					t.Errorf("预期应该有结果，但获取到空结果")
				}
			}()
		}

		wg.Wait()
	})
}

// TestHealthCheckSchedulerConcurrency 测试健康检查调度器在并发环境下的行为
func TestHealthCheckSchedulerConcurrency(t *testing.T) {
	// 创建负载均衡器
	balancer, err := NewBalancer("")
	if err != nil {
		t.Fatalf("无法创建 Balancer 实例: %v", err)
	}
	defer balancer.Close()

	// 测试重复启动健康检查调度器
	t.Run("Multiple StartHealthCheckScheduler Calls", func(t *testing.T) {
		var wg sync.WaitGroup
		const callCount = 10

		// 重置 healthCheckSchedulerStarted 以便测试
		healthCheckSchedulerStarted = sync.Once{}

		wg.Add(callCount)

		for i := 0; i < callCount; i++ {
			go func() {
				defer wg.Done()
				// 尝试启动健康检查调度器
				// 即使同时调用多次，也应该只有一次真正执行
				StartHealthCheckScheduler(balancer, 5*time.Minute)
			}()
		}

		wg.Wait()
		// 无法直接验证只启动了一次，但这里测试是否有崩溃或死锁
	})
}

// TestCheckProviderHealthConcurrency 测试对单个提供者进行并发健康检查
func TestCheckProviderHealthConcurrency(t *testing.T) {
	// 创建模拟提供者
	dbProvider := &schema.AiProvider{}
	dbProvider.Model.ID = 1
	dbProvider.WrapperName = "test-provider"
	dbProvider.ModelName = "test-model"
	dbProvider.TypeName = "test-type"
	dbProvider.DomainOrURL = "http://example.com"
	dbProvider.APIKey = "test-key"

	provider := &Provider{
		ModelName:   "test-model",
		TypeName:    "test-type",
		DomainOrURL: "http://example.com",
		APIKey:      "test-key",
		DbProvider:  dbProvider,
	}

	// 并发执行健康检查
	const checkCount = 5
	var wg sync.WaitGroup
	wg.Add(checkCount)

	for i := 0; i < checkCount; i++ {
		go func() {
			defer wg.Done()
			// 这里不测试结果，只测试是否有并发问题
			_, _ = CheckProviderHealth(provider)
		}()
	}

	wg.Wait()
	// 如果没有死锁或崩溃，测试通过
}

// TestRunSingleProviderHealthCheckConcurrency 测试对单个提供者 ID 的并发健康检查
func TestRunSingleProviderHealthCheckConcurrency(t *testing.T) {
	// 注：此测试需要数据库中有有效的提供者，否则会跳过
	// 获取一个有效的提供者 ID
	providers, err := GetAllAiProviders()
	if err != nil || len(providers) == 0 {
		t.Skip("跳过测试：数据库中没有有效的提供者")
		return
	}

	providerID := providers[0].ID

	// 并发执行健康检查
	const checkCount = 3
	results := make(chan error, checkCount)

	var wg sync.WaitGroup
	wg.Add(checkCount)

	for i := 0; i < checkCount; i++ {
		go func() {
			defer wg.Done()
			_, err := RunSingleProviderHealthCheck(providerID)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	// 收集结果
	for err := range results {
		if err != nil {
			t.Errorf("并发健康检查出错: %v", err)
		}
	}
}

// MockCheckAllProvidersHealth 是用于测试的模拟函数，快速返回结果
// 该函数模拟了CheckAllProvidersHealth的行为，但不执行实际的API调用
// 这样可以显著缩短测试时间，将测试重点放在并发安全性上
// 而不是测试实际的健康检查流程（那应该在单独的集成测试中进行）
func MockCheckAllProvidersHealth(manager *HealthCheckManager) []*HealthCheckResult {
	var results []*HealthCheckResult

	// 获取所有提供者
	providers := manager.Balancer.GetProviders()
	if len(providers) == 0 {
		return nil
	}

	// 为每个提供者创建一个模拟的健康检查结果
	for _, provider := range providers {
		result := &HealthCheckResult{
			Provider:     provider.DbProvider,
			IsHealthy:    true,
			ResponseTime: 100, // 模拟100ms响应时间
			Error:        nil,
		}
		results = append(results, result)

		// 可选：模拟将结果保存到健康检查管理器
		manager.SaveHealthCheckResult(result)
	}

	return results
}

// TestCheckAllProvidersHealthConcurrency 测试同时执行多个全局健康检查
func TestCheckAllProvidersHealthConcurrency(t *testing.T) {
	// 创建负载均衡器
	balancer, err := NewBalancer("")
	if err != nil {
		t.Fatalf("无法创建 Balancer 实例: %v", err)
	}
	defer balancer.Close()

	// 创建健康检查管理器
	manager := NewHealthCheckManager(balancer)

	// 并发执行健康检查，使用模拟函数
	const checkCount = 3
	var wg sync.WaitGroup
	wg.Add(checkCount)

	for i := 0; i < checkCount; i++ {
		go func() {
			defer wg.Done()
			// 使用模拟函数代替真实的健康检查
			_ = MockCheckAllProvidersHealth(manager)
		}()
	}

	wg.Wait()
	// 如果没有死锁或崩溃，测试通过
}
