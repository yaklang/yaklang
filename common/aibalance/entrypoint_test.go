package aibalance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEntrypoint(t *testing.T) {
	e := NewEntrypoint()

	// 添加模型和提供者
	modelName := "test-model"
	provider1 := &Provider{
		ModelName:   "gpt-3.5-turbo",
		TypeName:    "openai",
		DomainOrURL: "https://api.openai.com",
		APIKey:      "api-key-1",
	}
	provider2 := &Provider{
		ModelName:   "gpt-3.5-turbo",
		TypeName:    "openai",
		DomainOrURL: "https://api.openai.com",
		APIKey:      "api-key-2",
	}

	e.AddProvider(modelName, provider1)
	e.AddProvider(modelName, provider2)

	// 验证模型入口点是否正确创建
	entry, ok := e.ModelEntries.Get(modelName)
	assert.True(t, ok, "Model entry should exist")
	assert.Equal(t, 2, len(entry.Providers))
	assert.Contains(t, entry.Providers, provider1)
	assert.Contains(t, entry.Providers, provider2)

	// 验证获取单个提供者
	provider := e.PeekProvider(modelName)
	assert.NotNil(t, provider)
	assert.Contains(t, entry.Providers, provider)

	// 验证不存在的模型
	provider = e.PeekProvider("non-existent-model")
	assert.Nil(t, provider)
}

// TestSmartPeekProvider 测试智能选择算法
func TestSmartPeekProvider(t *testing.T) {
	// 创建一个测试用的 Entrypoint
	e := NewEntrypoint()
	modelName := "test-model"

	// 创建5个不同的 Provider，具有不同的请求数和延迟
	providers := make([]*Provider, 5)
	dbProviders := make([]*schema.AiProvider, 5)

	for i := 0; i < 5; i++ {
		providers[i] = &Provider{
			ModelName:   "test-model",
			TypeName:    "test-type",
			DomainOrURL: fmt.Sprintf("http://test-%d.com", i),
			APIKey:      fmt.Sprintf("key-%d", i),
		}

		// 模拟数据库对象
		dbProviders[i] = &schema.AiProvider{
			WrapperName:       "test-wrapper",
			ModelName:         "test-model",
			TypeName:          "test-type",
			DomainOrURL:       fmt.Sprintf("http://test-%d.com", i),
			APIKey:            fmt.Sprintf("key-%d", i),
			NoHTTPS:           false,
			SuccessCount:      int64((i + 1) * 10), // 10, 20, 30, 40, 50
			FailureCount:      int64(i),            // 0, 1, 2, 3, 4
			TotalRequests:     int64((i+1)*10 + i), // 10, 21, 32, 43, 54
			LastRequestTime:   time.Now(),
			LastRequestStatus: i != 2,               // 第3个 Provider 状态为不健康
			LastLatency:       int64((i + 1) * 100), // 100, 200, 300, 400, 500ms
			IsHealthy:         i != 2,               // 第3个 Provider 不健康
			HealthCheckTime:   time.Now(),
		}

		// 设置 Provider 的数据库对象
		providers[i].DbProvider = dbProviders[i]

		// 添加到 Entrypoint
		e.AddProvider(modelName, providers[i])
	}

	// 模拟多次选择，统计选择结果
	selections := make(map[string]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		provider := e.PeekProvider(modelName)
		assert.NotNil(t, provider)

		key := provider.APIKey
		selections[key]++
	}

	// 不健康的 Provider 应该不会被选择
	assert.Equal(t, 0, selections["key-2"], "Unhealthy provider should not be selected")

	// 打印选择结果
	t.Logf("Provider selection distribution after %d iterations:", iterations)
	for key, count := range selections {
		percentage := float64(count) / float64(iterations) * 100
		t.Logf("%s: %d times (%.2f%%)", key, count, percentage)
	}

	// 检查负载均衡 - 请求数少的应该被选择得更多
	// Provider 0 (10次请求) 应该比 Provider 4 (54次请求) 选择概率更高
	assert.Greater(t, selections["key-0"], selections["key-4"], "Provider with fewer requests should be selected more often")

	// 验证延迟因素 - 延迟低的应该优先选择
	// 在负载接近的情况下，Provider 0 (100ms) 应该比 Provider 1 (200ms) 被选择得更多
	assert.GreaterOrEqual(t, selections["key-0"], selections["key-1"], "Provider with lower latency should be preferred when load is similar")
}

// TestConcurrentPeekProvider 测试并发场景下的 PeekProvider 方法
func TestConcurrentPeekProvider(t *testing.T) {
	// 创建一个测试用的 Entrypoint
	e := NewEntrypoint()
	modelName := "test-model"

	// 创建5个不同的 Provider
	providers := make([]*Provider, 5)
	dbProviders := make([]*schema.AiProvider, 5)

	for i := 0; i < 5; i++ {
		providers[i] = &Provider{
			ModelName:   "test-model",
			TypeName:    "test-type",
			DomainOrURL: fmt.Sprintf("http://test-%d.com", i),
			APIKey:      fmt.Sprintf("key-%d", i),
		}

		// 模拟数据库对象
		dbProviders[i] = &schema.AiProvider{
			WrapperName:       "test-wrapper",
			ModelName:         "test-model",
			TypeName:          "test-type",
			DomainOrURL:       fmt.Sprintf("http://test-%d.com", i),
			APIKey:            fmt.Sprintf("key-%d", i),
			NoHTTPS:           false,
			SuccessCount:      int64(100),
			FailureCount:      int64(0),
			TotalRequests:     int64(100),
			LastRequestTime:   time.Now(),
			LastRequestStatus: true,
			LastLatency:       int64(100),
			IsHealthy:         true,
			HealthCheckTime:   time.Now(),
		}

		// 设置 Provider 的数据库对象
		providers[i].DbProvider = dbProviders[i]

		// 添加到 Entrypoint
		e.AddProvider(modelName, providers[i])
	}

	// 并发测试
	var wg sync.WaitGroup
	var successCount int32
	var errorCount int32

	// 模拟高并发和瞬时并发
	goroutines := 100  // 并发协程数
	iterations := 1000 // 每个协程的迭代次数

	// 同时启动所有协程，模拟瞬时高并发
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				provider := e.PeekProvider(modelName)
				if provider != nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&errorCount, 1)
				}
			}
		}()
	}

	// 在高并发访问的同时，模拟添加和修改操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			newProvider := &Provider{
				ModelName:   "test-model",
				TypeName:    "test-type",
				DomainOrURL: fmt.Sprintf("http://new-test-%d.com", i),
				APIKey:      fmt.Sprintf("new-key-%d", i),
				DbProvider: &schema.AiProvider{
					WrapperName:       "test-wrapper",
					ModelName:         "test-model",
					TypeName:          "test-type",
					DomainOrURL:       fmt.Sprintf("http://new-test-%d.com", i),
					APIKey:            fmt.Sprintf("new-key-%d", i),
					NoHTTPS:           false,
					SuccessCount:      int64(50),
					FailureCount:      int64(0),
					TotalRequests:     int64(50),
					LastRequestTime:   time.Now(),
					LastRequestStatus: true,
					LastLatency:       int64(100),
					IsHealthy:         true,
					HealthCheckTime:   time.Now(),
				},
			}
			e.AddProvider(modelName, newProvider)
			time.Sleep(time.Millisecond * 10) // 小间隔，增加并发复杂度
		}
	}()

	// 等待所有协程完成
	wg.Wait()

	// 验证结果
	expectedTotal := int32(goroutines * iterations)
	actualTotal := successCount + errorCount

	t.Logf("并发测试结果: 预期总调用 %d, 实际成功 %d, 实际错误 %d",
		expectedTotal, successCount, errorCount)

	// 检查是否所有调用都成功执行
	assert.Equal(t, expectedTotal, actualTotal, "所有并发调用应该都被执行")
	// 检查是否所有调用都成功返回了提供者（没有空指针）
	assert.Equal(t, expectedTotal, successCount, "所有并发调用应该都成功返回提供者")
	assert.Equal(t, int32(0), errorCount, "不应该有任何错误")

	// 验证最终的提供者数量
	entry, ok := e.ModelEntries.Get(modelName)
	assert.True(t, ok, "模型入口点应该存在")
	assert.Equal(t, 5+50, len(entry.Providers), "提供者总数应该匹配")
}

// TestDynamicUpdateWithConcurrency 测试在高并发下动态更新提供者状态
func TestDynamicUpdateWithConcurrency(t *testing.T) {
	// 创建一个测试用的 Entrypoint
	e := NewEntrypoint()
	modelName := "test-model"

	// 创建初始提供者
	initialProvider := &Provider{
		ModelName:   "test-model",
		TypeName:    "test-type",
		DomainOrURL: "http://initial.com",
		APIKey:      "initial-key",
		DbProvider: &schema.AiProvider{
			WrapperName:       "test-wrapper",
			ModelName:         "test-model",
			TypeName:          "test-type",
			DomainOrURL:       "http://initial.com",
			APIKey:            "initial-key",
			NoHTTPS:           false,
			SuccessCount:      int64(100),
			FailureCount:      int64(0),
			TotalRequests:     int64(100),
			LastRequestTime:   time.Now(),
			LastRequestStatus: true,
			LastLatency:       int64(100),
			IsHealthy:         true,
			HealthCheckTime:   time.Now(),
		},
	}
	e.AddProvider(modelName, initialProvider)

	// 并发测试
	var wg sync.WaitGroup

	// 并发读取协程
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				provider := e.PeekProvider(modelName)
				assert.NotNil(t, provider)
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// 并发添加新提供者
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				newProvider := &Provider{
					ModelName:   "test-model",
					TypeName:    "test-type",
					DomainOrURL: fmt.Sprintf("http://dynamic-%d-%d.com", index, j),
					APIKey:      fmt.Sprintf("dynamic-key-%d-%d", index, j),
					DbProvider: &schema.AiProvider{
						WrapperName:       "test-wrapper",
						ModelName:         "test-model",
						TypeName:          "test-type",
						DomainOrURL:       fmt.Sprintf("http://dynamic-%d-%d.com", index, j),
						APIKey:            fmt.Sprintf("dynamic-key-%d-%d", index, j),
						NoHTTPS:           false,
						SuccessCount:      int64(0),
						FailureCount:      int64(0),
						TotalRequests:     int64(0),
						LastRequestTime:   time.Now(),
						LastRequestStatus: true,
						LastLatency:       int64(200),
						IsHealthy:         true,
						HealthCheckTime:   time.Now(),
					},
				}
				e.AddProvider(modelName, newProvider)
				time.Sleep(time.Millisecond * 5)
			}
		}(i)
	}

	// 并发更新健康状态
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				provider := e.PeekProvider(modelName)
				if provider != nil && provider.DbProvider != nil {
					// 模拟健康状态变化
					provider.DbProvider.IsHealthy = (j%2 == 0)
					time.Sleep(time.Millisecond * 10)
				}
			}
		}()
	}

	// 等待所有协程完成
	wg.Wait()

	// 验证最终状态
	entry, ok := e.ModelEntries.Get(modelName)
	assert.True(t, ok, "模型入口点应该存在")
	assert.Equal(t, 1+10*5, len(entry.Providers), "提供者总数应该匹配")

	// 验证最终选择是否正常
	for i := 0; i < 100; i++ {
		provider := e.PeekProvider(modelName)
		assert.NotNil(t, provider, "应该能够正常获取提供者")
	}

	t.Log("并发动态更新测试通过，未发现死锁或异常")
}
