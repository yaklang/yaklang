package aibalance

import (
	"fmt"
	"net"
	"strings"
	"testing"
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
