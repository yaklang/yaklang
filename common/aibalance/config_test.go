package aibalance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
)

func init() {
	// 初始化数据库
	consts.InitializeYakitDatabase("", "")
}

func TestYamlConfig_ToServerConfig(t *testing.T) {
	// 在测试前先打印一条信息，帮助调试
	t.Log("Starting TestYamlConfig_ToServerConfig")

	config := &YamlConfig{
		Keys: []KeyConfig{
			{
				Key:           "test-key",
				AllowedModels: []string{"test-model"},
			},
		},
		Models: []ModelConfig{
			{
				Name: "test-model",
				Providers: []*ConfigProvider{
					{
						TypeName:    "openai",
						DomainOrURL: "https://api.openai.com",
						APIKey:      "test-key",
						ModelName:   "test-model", // 确保设置了模型名称
					},
				},
			},
		},
	}

	// 打印配置以帮助调试
	t.Logf("Test config: %+v", config)

	serverConfig, err := config.ToServerConfig()
	if err != nil {
		t.Fatalf("ToServerConfig error: %v", err)
	}

	if serverConfig == nil {
		t.Fatalf("serverConfig is nil")
	}

	t.Logf("Server config after conversion: %+v", serverConfig)

	// 检查 Keys 是否为 nil
	if serverConfig.Keys == nil {
		t.Fatalf("serverConfig.Keys is nil")
	}

	// 检查 KeyAllowedModels 是否为 nil
	if serverConfig.KeyAllowedModels == nil {
		t.Fatalf("serverConfig.KeyAllowedModels is nil")
	}

	// 检查 Models 是否为 nil
	if serverConfig.Models == nil {
		t.Fatalf("serverConfig.Models is nil")
	}

	// 检查 Entrypoints 是否为 nil
	if serverConfig.Entrypoints == nil {
		t.Fatalf("serverConfig.Entrypoints is nil")
	}

	// 验证 key 配置
	key, ok := serverConfig.Keys.Get("test-key")
	if !ok {
		t.Fatal("key not found")
	}
	if key.Key != "test-key" {
		t.Errorf("expected key test-key, got %s", key.Key)
	}
	if !key.AllowedModels["test-model"] {
		t.Error("test-model should be allowed")
	}

	// 验证 model 配置
	providers, ok := serverConfig.Models.Get("test-model")
	if !ok {
		t.Fatal("model not found")
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].TypeName != "openai" {
		t.Errorf("expected type openai, got %s", providers[0].TypeName)
	}

	t.Log("Test completed successfully")
}

func TestYamlConfig_LoadFromFile(t *testing.T) {
	// 在测试前先打印一条信息，帮助调试
	t.Log("Starting TestYamlConfig_LoadFromFile")

	// 创建配置文件
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configContent := `
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
        model_name: test-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 打印配置文件内容以帮助调试
	t.Logf("Config file content: %s", configContent)

	// 加载配置
	var config YamlConfig
	if err := config.LoadFromFile(configPath); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	// 打印加载后的配置以帮助调试
	t.Logf("Loaded config: %+v", config)

	// 转换配置
	serverConfig, err := config.ToServerConfig()
	if err != nil {
		t.Fatalf("ToServerConfig error: %v", err)
	}

	if serverConfig == nil {
		t.Fatalf("serverConfig is nil")
	}

	t.Logf("Server config after conversion: %+v", serverConfig)

	// 检查 Keys 是否为 nil
	if serverConfig.Keys == nil {
		t.Fatalf("serverConfig.Keys is nil")
	}

	// 检查 KeyAllowedModels 是否为 nil
	if serverConfig.KeyAllowedModels == nil {
		t.Fatalf("serverConfig.KeyAllowedModels is nil")
	}

	// 检查 Models 是否为 nil
	if serverConfig.Models == nil {
		t.Fatalf("serverConfig.Models is nil")
	}

	// 检查 Entrypoints 是否为 nil
	if serverConfig.Entrypoints == nil {
		t.Fatalf("serverConfig.Entrypoints is nil")
	}

	// 验证 key 配置
	key, ok := serverConfig.Keys.Get("test-key")
	if !ok {
		t.Fatal("key not found")
	}
	if key.Key != "test-key" {
		t.Errorf("expected key test-key, got %s", key.Key)
	}
	if !key.AllowedModels["test-model"] {
		t.Error("test-model should be allowed")
	}

	// 验证 model 配置
	providers, ok := serverConfig.Models.Get("test-model")
	if !ok {
		t.Fatal("model not found")
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].TypeName != "openai" {
		t.Errorf("expected type openai, got %s", providers[0].TypeName)
	}

	t.Log("Test completed successfully")
}

func TestConfigProviderToProvider(t *testing.T) {
	// 创建临时文件用于测试
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "keys.txt")
	// 写入 5 个不同的 key
	err := os.WriteFile(keyFile, []byte("file_key1\nfile_key2\nfile_key3\nfile_key4\nfile_key5\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	// 创建一个包含多种 key 来源的 ConfigProvider
	configProvider := &ConfigProvider{
		ModelName:   "test-model",
		TypeName:    "test-provider",
		DomainOrURL: "http://test.com",
		APIKey:      "direct_key",                     // 1 个 key
		Keys:        []string{"key1", "key2", "key3"}, // 3 个 key
		KeyFile:     keyFile,                          // 5 个 key
		NoHTTPS:     true,
	}

	// 获取所有可能的 key
	expectedKeys := map[string]bool{
		"direct_key": true, // api_key
		"key1":       true, // keys[0]
		"key2":       true, // keys[1]
		"key3":       true, // keys[2]
		"file_key1":  true, // key_file[0]
		"file_key2":  true, // key_file[1]
		"file_key3":  true, // key_file[2]
		"file_key4":  true, // key_file[3]
		"file_key5":  true, // key_file[4]
	}

	// 测试 GetAllKeys 方法
	keys := configProvider.GetAllKeys()
	if len(keys) != len(expectedKeys) {
		t.Errorf("Expected %d keys, got %d", len(expectedKeys), len(keys))
	}

	// 创建一个副本用于跟踪找到的 key
	foundKeys := make(map[string]bool)
	for _, key := range keys {
		if !expectedKeys[key] {
			t.Errorf("Unexpected key: %s", key)
		}
		foundKeys[key] = true
	}

	// 检查是否有缺失的 key
	for key := range expectedKeys {
		if !foundKeys[key] {
			t.Errorf("Missing key: %s", key)
		}
	}

	// 测试 ToProviders 方法
	providers := configProvider.ToProviders()
	if len(providers) != len(keys) {
		t.Errorf("Expected %d providers, got %d", len(keys), len(providers))
	}

	// 验证每个 provider 的属性是否正确
	providerKeys := make(map[string]bool)
	for _, provider := range providers {
		if provider.ModelName != configProvider.ModelName {
			t.Errorf("Expected ModelName %s, got %s", configProvider.ModelName, provider.ModelName)
		}
		if provider.TypeName != configProvider.TypeName {
			t.Errorf("Expected TypeName %s, got %s", configProvider.TypeName, provider.TypeName)
		}
		if provider.DomainOrURL != configProvider.DomainOrURL {
			t.Errorf("Expected DomainOrURL %s, got %s", configProvider.DomainOrURL, provider.DomainOrURL)
		}
		if provider.NoHTTPS != configProvider.NoHTTPS {
			t.Errorf("Expected NoHTTPS %v, got %v", configProvider.NoHTTPS, provider.NoHTTPS)
		}
		// 记录这个 provider 的 APIKey
		providerKeys[provider.APIKey] = true
	}

	// 验证所有预期的 key 都被用于创建 provider
	for key := range expectedKeys {
		if !providerKeys[key] {
			t.Errorf("Key %s was not used to create a provider", key)
		}
	}

	// 验证没有额外的 key 被用于创建 provider
	for key := range providerKeys {
		if !expectedKeys[key] {
			t.Errorf("Unexpected key %s was used to create a provider", key)
		}
	}
}
