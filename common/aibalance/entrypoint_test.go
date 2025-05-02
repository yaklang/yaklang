package aibalance

import (
	"fmt"
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
