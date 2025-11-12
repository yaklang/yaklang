package aibalance

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

func init() {
	// 初始化数据库
	consts.InitializeYakitDatabase("", "", "")
}

func TestBalancerBasic(t *testing.T) {
	// 清理测试数据，确保测试环境干净
	db := GetDB()
	// 更精确地清理：不仅要清理 model_name 为 test-model 的记录，还要清理 wrapper_name 为 test-model 的记录
	db.Exec("DELETE FROM ai_providers WHERE model_name = 'test-model' OR wrapper_name = 'test-model'")

	// 创建自定义的 ServerConfig，避免加载数据库中的提供者
	customConfig := func(configData []byte) (*Balancer, error) {
		var ymlConfig YamlConfig
		if err := yaml.Unmarshal(configData, &ymlConfig); err != nil {
			return nil, err
		}

		serverConfig, err := ymlConfig.ToServerConfig()
		if err != nil {
			return nil, err
		}

		// 注意：不调用 LoadProvidersFromDatabase

		ctx, cancel := context.WithCancel(context.Background())
		return &Balancer{
			config: serverConfig,
			ctx:    ctx,
			cancel: cancel,
		}, nil
	}

	config := `
keys:
  - key: test-key
    allowed_models:
      - test-model
models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: https://api.openai.com
        api_key: test-key
    `

	// 创建临时目录
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(config), 0644)
	assert.NoError(t, err, "写入配置文件失败")

	// 使用自定义方法创建 Balancer，避免加载数据库提供者
	balancer, err := customConfig([]byte(config))
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 验证配置是否正确
	assert.NotNil(t, balancer.config, "配置不应为空")
	assert.NotNil(t, balancer.config.Entrypoints, "Entrypoints 不应为空")
	assert.NotNil(t, balancer.config.Models, "Models 不应为空")

	// 验证 API 密钥是否正确
	key, ok := balancer.config.Keys.Get("test-key")
	assert.True(t, ok, "应该找到 API 密钥")
	assert.Equal(t, "test-key", key.Key, "API 密钥应该匹配")

	// 验证模型是否正确
	providers, ok := balancer.config.Models.Get("test-model")
	assert.True(t, ok, "应该找到模型")
	assert.Equal(t, 1, len(providers), "应该有一个提供者")
	assert.Equal(t, "openai", providers[0].TypeName, "提供者类型应该是 openai")

	// 清理
	err = balancer.Close()
	assert.NoError(t, err, "关闭 Balancer 失败")
}

// 测试 Balancer 的 Close 方法
func TestBalancerClose(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// 写入测试配置
	configContent := `
keys:
  - key: test-api-key
    allowed_models:
      - test-model

models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: http://fake-openai-service
        api_key: fake-openai-key
        model_name: gpt-3.5-turbo
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err, "写入测试配置文件失败")

	// 创建 Balancer
	balancer, err := NewBalancer(configPath)
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 在随机端口上运行 Balancer
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 在 goroutine 中运行 Balancer
	go func() {
		err := balancer.RunWithAddr(addr)
		if err != nil {
			// 只有非正常关闭才会报错
			t.Logf("Balancer 运行失败: %v", err)
		}
	}()

	// 等待 Balancer 启动
	time.Sleep(500 * time.Millisecond)

	// 测试连接是否成功
	conn, err := net.Dial("tcp", addr)
	assert.NoError(t, err, "连接到 Balancer 失败")
	assert.NotNil(t, conn, "连接不应为空")
	conn.Close()

	// 关闭 Balancer
	err = balancer.Close()
	assert.NoError(t, err, "关闭 Balancer 失败")

	// 等待关闭完成
	time.Sleep(500 * time.Millisecond)

	// 尝试再次连接，应该失败
	_, err = net.Dial("tcp", addr)
	assert.Error(t, err, "连接应该失败，因为 Balancer 已关闭")
}

// 测试 Balancer 在运行前关闭
func TestBalancerCloseBeforeRun(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// 写入测试配置
	configContent := `
keys:
  - key: test-api-key
    allowed_models:
      - test-model

models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: http://fake-openai-service
        api_key: fake-openai-key
        model_name: gpt-3.5-turbo
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err, "写入测试配置文件失败")

	// 创建 Balancer
	balancer, err := NewBalancer(configPath)
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 直接关闭 Balancer
	err = balancer.Close()
	assert.NoError(t, err, "关闭 Balancer 失败")
}

// TestLoadProvidersFromDatabase 测试从数据库加载恢复 AI 提供者的功能
func TestLoadProvidersFromDatabase(t *testing.T) {
	// 清理测试数据，确保测试环境干净
	db := GetDB()
	db.Exec("DELETE FROM ai_providers WHERE wrapper_name LIKE 'test-load-%'")

	// 创建测试数据：注册两个不同模型的提供者
	model1 := "test-load-model-1"
	model2 := "test-load-model-2"

	// 为模型1注册两个提供者
	provider1, err := RegisterAiProvider(model1, "gpt-3.5-turbo", "openai", "https://api.openai.com", "test-key-1", false)
	assert.NoError(t, err, "注册提供者1失败")
	assert.NotNil(t, provider1, "提供者1不应为空")

	provider2, err := RegisterAiProvider(model1, "gpt-3.5-turbo-16k", "openai", "https://api.openai.com", "test-key-2", false)
	assert.NoError(t, err, "注册提供者2失败")
	assert.NotNil(t, provider2, "提供者2不应为空")

	// 为模型2注册一个提供者
	provider3, err := RegisterAiProvider(model2, "glm-4", "chatglm", "https://open.bigmodel.cn", "test-key-3", false)
	assert.NoError(t, err, "注册提供者3失败")
	assert.NotNil(t, provider3, "提供者3不应为空")

	// 创建一个空的配置
	serverConfig := NewServerConfig()

	// 从数据库加载提供者
	err = LoadProvidersFromDatabase(serverConfig)
	assert.NoError(t, err, "从数据库加载提供者失败")

	// 验证是否正确加载了模型1
	providers1, ok := serverConfig.Models.Get(model1)
	assert.True(t, ok, "应该找到模型1")
	assert.GreaterOrEqual(t, len(providers1), 2, "模型1应该至少有2个提供者")

	// 验证模型1的提供者是否正确
	foundProvider1 := false
	foundProvider2 := false
	for _, p := range providers1 {
		if p.APIKey == "test-key-1" {
			foundProvider1 = true
			assert.Equal(t, "openai", p.TypeName)
			assert.Equal(t, "gpt-3.5-turbo", p.ModelName)
		}
		if p.APIKey == "test-key-2" {
			foundProvider2 = true
			assert.Equal(t, "openai", p.TypeName)
			assert.Equal(t, "gpt-3.5-turbo-16k", p.ModelName)
		}
	}
	assert.True(t, foundProvider1, "应该找到提供者1")
	assert.True(t, foundProvider2, "应该找到提供者2")

	// 验证是否正确加载了模型2
	providers2, ok := serverConfig.Models.Get(model2)
	assert.True(t, ok, "应该找到模型2")
	assert.GreaterOrEqual(t, len(providers2), 1, "模型2应该至少有1个提供者")

	// 验证模型2的提供者是否正确
	foundProvider3 := false
	for _, p := range providers2 {
		if p.APIKey == "test-key-3" {
			foundProvider3 = true
			assert.Equal(t, "chatglm", p.TypeName)
			assert.Equal(t, "glm-4", p.ModelName)
		}
	}
	assert.True(t, foundProvider3, "应该找到提供者3")

	// 测试创建 Balancer 不依赖配置文件，而是从数据库加载
	tempDir := t.TempDir()
	nonExistentConfigPath := filepath.Join(tempDir, "non-existent-config.yaml")

	// 确保文件不存在
	_, err = os.Stat(nonExistentConfigPath)
	assert.True(t, os.IsNotExist(err), "配置文件不应存在")

	// 创建 Balancer，应该自动从数据库加载
	balancer, err := NewBalancer(nonExistentConfigPath)
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 验证是否正确加载了模型1
	providers1, ok = balancer.config.Models.Get(model1)
	assert.True(t, ok, "Balancer 应该加载模型1")
	assert.GreaterOrEqual(t, len(providers1), 2, "模型1应该至少有2个提供者")

	// 验证是否正确加载了模型2
	providers2, ok = balancer.config.Models.Get(model2)
	assert.True(t, ok, "Balancer 应该加载模型2")
	assert.GreaterOrEqual(t, len(providers2), 1, "模型2应该至少有1个提供者")

	// 验证 Entrypoints 是否正确设置
	assert.NotNil(t, balancer.config.Entrypoints, "Entrypoints 不应为空")

	// 验证是否可以从 Entrypoints 获取提供者
	entryProviders1 := balancer.config.Entrypoints.PeekProvider(model1)
	assert.NotNil(t, entryProviders1, "应该能从 Entrypoints 获取模型1的提供者")

	entryProviders2 := balancer.config.Entrypoints.PeekProvider(model2)
	assert.NotNil(t, entryProviders2, "应该能从 Entrypoints 获取模型2的提供者")

	// 清理测试数据
	db.Exec("DELETE FROM ai_providers WHERE wrapper_name LIKE 'test-load-%'")
}
