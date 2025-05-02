package aibalance

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

func TestServeChatCompletions(t *testing.T) {
	// 创建测试配置
	cfg := NewServerConfig()

	// 添加测试密钥
	key := &Key{
		Key:           "test-key",
		AllowedModels: make(map[string]bool),
	}
	key.AllowedModels["test-model"] = true
	cfg.Keys.keys["test-key"] = key

	// 添加允许的模型
	allowedModels := make(map[string]bool)
	allowedModels["test-model"] = true
	cfg.KeyAllowedModels.allowedModels["test-key"] = allowedModels

	// 添加测试模型
	model := &Provider{
		ModelName:   "test-model",
		TypeName:    "openai",
		DomainOrURL: "http://test.com",
		APIKey:      "test-api-key",
	}
	cfg.Models.models["test-model"] = []*Provider{model}
	cfg.Entrypoints.providers["test-model"] = []*Provider{model}

	// 创建测试连接
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// 测试用例1: 无效的请求体
	go func() {
		cfg.Serve(server)
	}()

	// 发送无效请求
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer test-key\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"))

	// 读取响应
	buf := make([]byte, 1024)
	n, _ := client.Read(buf)
	response := string(buf[:n])
	if !strings.Contains(response, "400 Bad Request") {
		t.Fatal("期望 400 Bad Request 响应")
	}

	// 测试用例2: 未授权的请求
	client, server = net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	// 发送未授权请求
	jsonBody := `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))

	n, _ = client.Read(buf)
	response = string(buf[:n])
	if !strings.Contains(response, "401 Unauthorized") {
		t.Fatal("期望 401 Unauthorized 响应")
	}

	// 测试用例3: 不存在的模型
	client, server = net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	// 发送不存在的模型请求
	jsonBody = `{"model":"non-existent","messages":[{"role":"user","content":"test"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer test-key\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))

	n, _ = client.Read(buf)
	response = string(buf[:n])
	if !strings.Contains(response, "404 Not Found") {
		t.Fatal("期望 404 Not Found 响应")
	}

	// 测试用例4: 有效的请求
	client, server = net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	// 发送有效请求
	jsonBody = `{"model":"test-model","messages":[{"role":"user","content":"test"}]}`
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer test-key\r\n" +
		"Content-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n", len(jsonBody)) +
		"\r\n" +
		jsonBody))

	n, _ = client.Read(buf)
	response = string(buf[:n])
	if !strings.Contains(response, "200 OK") {
		t.Fatal("期望 200 OK 响应")
	}
}

// 模拟 UpdateDbProvider 的线程安全性
func TestServerProviderStatusUpdateConcurrency(t *testing.T) {
	// 模拟提供者
	provider := &Provider{
		ModelName:   "test-model",
		TypeName:    "test-type",
		DomainOrURL: "http://test.com",
		APIKey:      "test-key",
		NoHTTPS:     false,
	}

	// 模拟数据库对象
	dbProvider := &schema.AiProvider{
		WrapperName:     "test-wrapper",
		ModelName:       "test-model",
		TypeName:        "test-type",
		DomainOrURL:     "http://test.com",
		APIKey:          "test-key",
		NoHTTPS:         false,
		SuccessCount:    0,
		FailureCount:    0,
		TotalRequests:   0,
		IsHealthy:       true,
		HealthCheckTime: time.Now(),
	}

	// 设置 provider 的 DbProvider
	provider.DbProvider = dbProvider

	// 模拟多个并发请求
	const requestCount = 100

	// 使用 WaitGroup 等待所有请求完成
	var wg sync.WaitGroup
	wg.Add(requestCount)

	// 启动多个并发请求
	for i := 0; i < requestCount; i++ {
		go func(idx int) {
			defer wg.Done()

			// 模拟请求成功或失败
			success := idx%2 == 0                // 一半成功，一半失败
			latencyMs := int64(100 + idx%10*100) // 100ms - 1000ms

			// 直接调用 UpdateDbProvider 方法
			err := provider.UpdateDbProvider(success, latencyMs)
			if err != nil {
				t.Errorf("Failed to update provider: %v", err)
			}
		}(i)
	}

	// 等待所有请求完成
	wg.Wait()

	// 验证统计数据
	expectedSuccessCount := int64(requestCount / 2)
	expectedFailureCount := int64(requestCount) - expectedSuccessCount

	if dbProvider.TotalRequests != int64(requestCount) {
		t.Errorf("Total requests mismatch: expected %d, got %d", requestCount, dbProvider.TotalRequests)
	}

	if dbProvider.SuccessCount != expectedSuccessCount {
		t.Errorf("Success count mismatch: expected %d, got %d", expectedSuccessCount, dbProvider.SuccessCount)
	}

	if dbProvider.FailureCount != expectedFailureCount {
		t.Errorf("Failure count mismatch: expected %d, got %d", expectedFailureCount, dbProvider.FailureCount)
	}

	// 验证最后一次请求的状态
	t.Logf("Last request status: success=%v, latency=%dms, isHealthy=%v",
		dbProvider.LastRequestStatus, dbProvider.LastLatency, dbProvider.IsHealthy)
}
