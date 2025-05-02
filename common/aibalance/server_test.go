package aibalance

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func TestServeChatCompletions(t *testing.T) {
	// 创建测试配置
	cfg := NewConfig()

	// 添加测试密钥
	key := &KeyConfig{
		Key:           "test-key",
		AllowedModels: []string{"test-model"},
	}
	cfg.Keys.Set("test-key", key)

	// 添加允许的模型
	allowedModels := omap.NewOrderedMap[string, bool](make(map[string]bool))
	allowedModels.Set("test-model", true)
	cfg.KeyAllowedModels.Set("test-key", allowedModels)

	// 添加测试模型
	model := &ModelConfig{
		Name: "test-model",
		Providers: []*Provider{
			{
				ModelName:   "test-model",
				TypeName:    "test-provider",
				DomainOrURL: "http://test.com",
				APIKey:      "test-api-key",
			},
		},
	}
	cfg.Models.Set("test-model", model)

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
	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 20\r\n" +
		"\r\n" +
		"{\"model\":\"test-model\"}"))

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

	// 发送请求到不存在的模型
	chatMsg := aispec.ChatMessage{
		Model: "non-existent-model",
		Messages: []aispec.ChatDetail{
			{
				Role:    "user",
				Content: "test message",
			},
		},
	}
	body, _ := json.Marshal(chatMsg)

	client.Write([]byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Authorization: Bearer test-key\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: " + fmt.Sprint(len(body)) + "\r\n" +
		"\r\n"))
	client.Write(body)

	n, _ = client.Read(buf)
	response = string(buf[:n])
	if !strings.Contains(response, "404 Not Found") {
		t.Fatal("期望 404 Not Found 响应")
	}

	// 测试用例4: 无效的路径
	client, server = net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.Serve(server)
	}()

	// 发送请求到无效路径
	client.Write([]byte("GET /invalid/path HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"\r\n"))

	n, _ = client.Read(buf)
	response = string(buf[:n])
	if !strings.Contains(response, "404 Not Found") {
		t.Fatal("期望 404 Not Found 响应")
	}
}
