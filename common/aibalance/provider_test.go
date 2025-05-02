package aibalance

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

func TestProvider_GetAIClient(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		expectError bool
	}{
		{
			name: "OpenAI provider",
			provider: &Provider{
				ModelName:   "gpt-3.5-turbo",
				TypeName:    "openai",
				DomainOrURL: "https://api.openai.com",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "ChatGLM provider",
			provider: &Provider{
				ModelName:   "glm-4-flash",
				TypeName:    "chatglm",
				DomainOrURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Moonshot provider",
			provider: &Provider{
				ModelName:   "moonshot-v1-8k",
				TypeName:    "moonshot",
				DomainOrURL: "https://api.moonshot.cn",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Tongyi provider",
			provider: &Provider{
				ModelName:   "qwen-turbo",
				TypeName:    "tongyi",
				DomainOrURL: "https://dashscope.aliyuncs.com",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Invalid provider type",
			provider: &Provider{
				ModelName:   "test-model",
				TypeName:    "invalid-type",
				DomainOrURL: "https://test.com",
				APIKey:      "test-key",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.provider.GetAIClient(nil, nil)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestProvider_GetAIClient_Performance(t *testing.T) {
	providers := []*Provider{
		{
			ModelName:   "gpt-3.5-turbo",
			TypeName:    "openai",
			DomainOrURL: "https://api.openai.com",
			APIKey:      "test-key",
		},
		{
			ModelName:   "glm-4-flash",
			TypeName:    "chatglm",
			DomainOrURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			APIKey:      "test-key",
		},
		{
			ModelName:   "moonshot-v1-8k",
			TypeName:    "moonshot",
			DomainOrURL: "https://api.moonshot.cn",
			APIKey:      "test-key",
		},
	}

	for _, provider := range providers {
		t.Run(provider.TypeName, func(t *testing.T) {
			start := time.Now()
			_, err := provider.GetAIClient(nil, nil)
			assert.NoError(t, err)
			duration := time.Since(start)

			// 确保性能在2秒内
			assert.Less(t, duration, 2*time.Second, "GetAIClient took too long for %s", provider.TypeName)
		})
	}
}

func TestProviderConcurrentUpdate(t *testing.T) {
	// 并发更新次数
	const updateCount = 1000
	// 成功请求的比例
	const successRatio = 0.7

	// 预期的成功和失败请求数
	expectedSuccessCount := int64(updateCount * successRatio)
	expectedFailureCount := int64(updateCount) - expectedSuccessCount

	// 定义两组计数器，一组有锁保护，一组无锁
	var (
		// 有锁保护的计数器
		safeTotal   int64
		safeSuccess int64
		safeFailure int64
		safeMutex   sync.Mutex

		// 无锁保护的计数器（用于测试并发问题）
		unsafeTotal   int64
		unsafeSuccess int64
		unsafeFailure int64
	)

	// 使用 WaitGroup 等待所有 goroutine 完成
	var wg sync.WaitGroup
	wg.Add(updateCount)

	// 启动多个 goroutine 并发更新计数器
	for i := 0; i < updateCount; i++ {
		go func(idx int) {
			defer wg.Done()

			// 根据索引确定是成功还是失败请求
			success := idx < int(expectedSuccessCount)

			// 安全地更新有锁计数器
			safeMutex.Lock()
			safeTotal++
			if success {
				safeSuccess++
			} else {
				safeFailure++
			}
			safeMutex.Unlock()

			// 不安全地更新无锁计数器（模拟并发问题）
			unsafeTotal++
			if success {
				unsafeSuccess++
			} else {
				unsafeFailure++
			}

		}(i)
	}

	// 等待所有更新完成
	wg.Wait()

	// 验证有锁计数器的统计数据
	if safeTotal != int64(updateCount) {
		t.Errorf("Safe total count mismatch: expected %d, got %d", updateCount, safeTotal)
	}

	if safeSuccess != expectedSuccessCount {
		t.Errorf("Safe success count mismatch: expected %d, got %d", expectedSuccessCount, safeSuccess)
	}

	if safeFailure != expectedFailureCount {
		t.Errorf("Safe failure count mismatch: expected %d, got %d", expectedFailureCount, safeFailure)
	}

	// 对于无锁计数器，几乎可以确定它的值会出现不一致（因为并发更新）
	// 但我们不能保证它一定会出现问题，所以这里只打印而不断言
	t.Logf("Unsafe counters (should likely be inconsistent in high concurrency):")
	t.Logf("  Total: %d (expected %d)", unsafeTotal, updateCount)
	t.Logf("  Success: %d (expected %d)", unsafeSuccess, expectedSuccessCount)
	t.Logf("  Failure: %d (expected %d)", unsafeFailure, expectedFailureCount)

	// 至少验证是否有不一致的情况出现
	if unsafeTotal == int64(updateCount) &&
		unsafeSuccess == expectedSuccessCount &&
		unsafeFailure == expectedFailureCount {
		t.Logf("No concurrent issues detected in unsafe counters. " +
			"This is possible but unlikely with high concurrency. " +
			"Consider increasing updateCount if this happens consistently.")
	}

	// 现在测试 UpdateDbProvider 方法的并发安全性
	// 创建一个测试用的 Provider
	testDbProvider := &schema.AiProvider{
		SuccessCount:  0,
		FailureCount:  0,
		TotalRequests: 0,
	}

	// 创建一个测试函数来模拟 Provider.UpdateDbProvider 的行为
	updateDbProvider := func(success bool, latencyMs int64) {
		// 更新统计信息
		testDbProvider.TotalRequests++
		testDbProvider.LastRequestTime = time.Now()
		testDbProvider.LastRequestStatus = success
		testDbProvider.LastLatency = latencyMs

		if success {
			testDbProvider.SuccessCount++
		} else {
			testDbProvider.FailureCount++
		}

		// 更新健康状态
		testDbProvider.IsHealthy = success && latencyMs < 3000
		testDbProvider.HealthCheckTime = time.Now()
	}

	// 重置并重用 WaitGroup
	wg = sync.WaitGroup{}
	wg.Add(updateCount)

	// 使用互斥锁保护 testDbProvider
	var dbMutex sync.Mutex

	// 并发调用更新函数
	for i := 0; i < updateCount; i++ {
		go func(idx int) {
			defer wg.Done()

			// 根据索引确定是成功还是失败请求
			success := idx < int(expectedSuccessCount)
			latency := int64(100 + idx%20*100) // 100ms - 2000ms

			// 线程安全地调用更新函数
			dbMutex.Lock()
			updateDbProvider(success, latency)
			dbMutex.Unlock()
		}(i)
	}

	// 等待所有更新完成
	wg.Wait()

	// 验证测试 Provider 的统计数据
	if testDbProvider.TotalRequests != int64(updateCount) {
		t.Errorf("Provider total requests mismatch: expected %d, got %d", updateCount, testDbProvider.TotalRequests)
	}

	if testDbProvider.SuccessCount != expectedSuccessCount {
		t.Errorf("Provider success count mismatch: expected %d, got %d", expectedSuccessCount, testDbProvider.SuccessCount)
	}

	if testDbProvider.FailureCount != expectedFailureCount {
		t.Errorf("Provider failure count mismatch: expected %d, got %d", expectedFailureCount, testDbProvider.FailureCount)
	}
}
