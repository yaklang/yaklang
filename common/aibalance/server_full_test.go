package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 模拟的AI客户端响应
type mockAIResponse struct {
	model        string
	responseText string
}

// TestFullAIBalanceFlow 测试AIBalance的完整流程
func TestFullAIBalanceFlow(t *testing.T) {
	// 创建测试配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// 写入测试配置
	configContent := `
keys:
  - key: test-api-key
    allowed_models:
      - test-model
      - other-model

models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: http://fake-openai-service
        api_key: fake-openai-key
        model_name: gpt-3.5-turbo
  - name: other-model
    providers:
      - type_name: openai
        domain_or_url: http://another-openai-service
        api_key: another-openai-key
        model_name: gpt-4
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err, "写入测试配置文件失败")

	// 解析配置文件
	var yamlConfig YamlConfig
	err = yamlConfig.LoadFromFile(configPath)
	assert.NoError(t, err, "加载配置文件失败")

	// 转换为服务器配置
	serverConfig, err := yamlConfig.ToServerConfig()
	assert.NoError(t, err, "转换服务器配置失败")
	assert.NotNil(t, serverConfig, "服务器配置不应为空")

	// 验证配置是否正确加载
	assert.Equal(t, 1, len(serverConfig.Keys.keys), "应该有1个API密钥")
	assert.Equal(t, 1, len(serverConfig.KeyAllowedModels.allowedModels), "应该有1个API密钥的允许模型")
	assert.Equal(t, 2, len(serverConfig.Models.models), "应该有2个模型")
	assert.Equal(t, 2, len(serverConfig.Entrypoints.providers), "应该有2个服务入口")

	// 设置测试服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0") // 使用随机可用端口
	assert.NoError(t, err, "设置测试服务器失败")
	defer listener.Close()

	// 启动服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // 监听器已关闭
			}
			go serverConfig.Serve(conn)
		}
	}()

	// 获取服务器地址
	serverAddr := listener.Addr().String()
	t.Logf("测试服务器运行在: %s", serverAddr)

	// 测试用例1: 有效的请求 - test-model
	t.Run("ValidRequestForTestModel", func(t *testing.T) {
		// 因为我们使用了假的服务地址，所以预期会失败，但应该返回500而不是400或401等
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model: "test-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "这是一个测试消息",
				},
			},
		}

		// 测试请求
		response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
		// 我们期望服务器能够处理请求
		assert.Contains(t, response, "200 OK", "应返回200 OK")
	})

	// 测试用例2: 有效的请求 - other-model
	t.Run("ValidRequestForOtherModel", func(t *testing.T) {
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model: "other-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "这是另一个测试消息",
				},
			},
		}

		// 测试请求
		response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
		// 我们期望服务器能够处理请求
		assert.Contains(t, response, "200 OK", "应返回200 OK")
	})

	// 测试用例3: 无效的API密钥
	t.Run("InvalidAPIKey", func(t *testing.T) {
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model: "test-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "这是一个测试消息",
				},
			},
		}

		// 测试请求
		response := sendChatRequest(t, serverAddr, "invalid-key", chatMessage)
		assert.Contains(t, response, "401 Unauthorized", "应返回401 Unauthorized")
	})

	// 测试用例4: 模型不存在
	t.Run("ModelNotFound", func(t *testing.T) {
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model: "non-existent-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "这是一个测试消息",
				},
			},
		}

		// 测试请求
		response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
		assert.Contains(t, response, "404 Not Found", "应返回404 Not Found")
	})

	// 测试用例5: 空消息
	t.Run("EmptyMessage", func(t *testing.T) {
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model:    "test-model",
			Messages: []aispec.ChatDetail{},
		}

		// 测试请求
		response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
		assert.Contains(t, response, "400 Bad Request", "应返回400 Bad Request")
	})

	// 测试用例6: 负载均衡功能
	t.Run("LoadBalancing", func(t *testing.T) {
		// 添加多个Provider到同一模型以测试负载均衡
		providers := serverConfig.Models.models["test-model"]

		// 添加第二个提供者
		secondProvider := &Provider{
			ModelName:   "test-model",
			TypeName:    "openai",
			DomainOrURL: "http://second-openai-provider",
			APIKey:      "second-openai-key",
		}
		providers = append(providers, secondProvider)
		serverConfig.Models.models["test-model"] = providers
		serverConfig.Entrypoints.providers["test-model"] = providers

		// 发送多个请求检查负载均衡
		chatMessage := aispec.ChatMessage{
			Model: "test-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "测试负载均衡",
				},
			},
		}

		// 发送几个请求以验证负载均衡
		for i := 0; i < 5; i++ {
			response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
			// 我们期望服务器能够处理请求
			assert.Contains(t, response, "200 OK", "应返回200 OK")
		}
	})

	// 测试用例7: 并发请求
	t.Run("ConcurrentRequests", func(t *testing.T) {
		// 创建测试消息
		chatMessage := aispec.ChatMessage{
			Model: "test-model",
			Messages: []aispec.ChatDetail{
				{
					Role:    "user",
					Content: "并发请求测试",
				},
			},
		}

		// 并发发送多个请求
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				response := sendChatRequest(t, serverAddr, "test-api-key", chatMessage)
				// 我们期望服务器能够处理请求
				assert.Contains(t, response, "200 OK", fmt.Sprintf("并发请求 %d 应返回200 OK", id))
				done <- true
			}(i)
		}

		// 等待所有请求完成
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// 发送聊天请求并返回响应
func sendChatRequest(t *testing.T, serverAddr, apiKey string, message aispec.ChatMessage) string {
	// 创建连接
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("连接服务器失败: %v", err)
	}
	defer conn.Close()

	// 将消息序列化为JSON
	msgBytes, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("序列化消息失败: %v", err)
	}

	// 构建HTTP请求
	request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Authorization: Bearer %s\r\n"+
		"Content-Type: application/json\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n%s",
		serverAddr, apiKey, len(msgBytes), string(msgBytes))

	// 发送请求
	_, err = conn.Write([]byte(request))
	if err != nil {
		t.Fatalf("发送请求失败: %v", err)
	}

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 读取响应
	var buffer bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "timeout") {
				break
			}
			t.Fatalf("读取响应失败: %v", err)
		}
		buffer.Write(buf[:n])

		// 如果响应已经完成，退出循环
		if strings.Contains(buffer.String(), "\r\n\r\n") && n < len(buf) {
			break
		}
	}

	return buffer.String()
}

// TestWithRealConfig 使用实际配置文件进行测试
func TestWithRealConfig(t *testing.T) {
	// 只有在明确指定测试环境变量时才运行这个测试
	if os.Getenv("RUN_REAL_CONFIG_TEST") != "true" {
		t.Skip("跳过真实配置测试，设置环境变量 RUN_REAL_CONFIG_TEST=true 以启用")
	}

	configPath := os.Getenv("TEST_CONFIG_PATH")
	if configPath == "" {
		configPath = "test-config.yaml" // 默认测试配置文件
	}

	// 加载配置
	var yamlConfig YamlConfig
	err := yamlConfig.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// 转换为服务器配置
	serverConfig, err := yamlConfig.ToServerConfig()
	if err != nil {
		t.Fatalf("转换服务器配置失败: %v", err)
	}

	// 设置测试服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("设置测试服务器失败: %v", err)
	}
	defer listener.Close()

	// 启动服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serverConfig.Serve(conn)
		}
	}()

	serverAddr := listener.Addr().String()
	t.Logf("测试服务器运行在: %s", serverAddr)

	// 从环境变量获取要测试的模型和API密钥
	testModel := os.Getenv("TEST_MODEL")
	testAPIKey := os.Getenv("TEST_API_KEY")

	if testModel == "" || testAPIKey == "" {
		t.Fatal("未指定测试模型或API密钥，请设置环境变量 TEST_MODEL 和 TEST_API_KEY")
	}

	// 创建测试消息
	chatMessage := aispec.ChatMessage{
		Model: testModel,
		Messages: []aispec.ChatDetail{
			{
				Role:    "user",
				Content: "这是一个实际配置测试",
			},
		},
	}

	// 发送请求
	response := sendChatRequest(t, serverAddr, testAPIKey, chatMessage)
	t.Logf("收到响应: %s", response)

	// 检查响应
	assert.Contains(t, response, "200 OK", "应返回200 OK")
}
