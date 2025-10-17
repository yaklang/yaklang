package localmodel

import (
	"fmt"
	"testing"
	"time"
)

func TestGetManager(t *testing.T) {
	manager1 := GetManager()
	manager2 := GetManager()

	if manager1 == nil {
		t.Fatal("Manager is nil")
	}

	if manager1.services == nil {
		t.Fatal("Manager services map is nil")
	}

	// 测试单例模式
	if manager1 != manager2 {
		t.Fatal("GetManager should return the same instance (singleton)")
	}
}

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatal("Manager is nil")
	}

	if manager.services == nil {
		t.Fatal("Manager services map is nil")
	}

	// 测试 NewManager 也返回单例
	manager2 := NewManager()
	if manager != manager2 {
		t.Fatal("NewManager should also return singleton instance")
	}
}

func TestDefaultServiceConfig(t *testing.T) {
	config := DefaultServiceConfig()

	if config.Host != "127.0.0.1" {
		t.Errorf("Expected default host '127.0.0.1', got '%s'", config.Host)
	}

	if config.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", config.Port)
	}

	if config.ContextSize != 4096 {
		t.Errorf("Expected default context size 4096, got %d", config.ContextSize)
	}

	if !config.ContBatching {
		t.Errorf("Expected default cont batching true, got %t", config.ContBatching)
	}

	if config.BatchSize != 1024 {
		t.Errorf("Expected default batch size 1024, got %d", config.BatchSize)
	}

	if config.Threads != 8 {
		t.Errorf("Expected default threads 8, got %d", config.Threads)
	}

	if config.StartupTimeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", config.StartupTimeout)
	}
}

func TestOptions(t *testing.T) {
	config := DefaultServiceConfig()

	// Test WithHost
	WithHost("192.168.1.1")(config)
	if config.Host != "192.168.1.1" {
		t.Errorf("WithHost failed, expected '192.168.1.1', got '%s'", config.Host)
	}

	// Test WithPort
	WithPort(9090)(config)
	if config.Port != 9090 {
		t.Errorf("WithPort failed, expected 9090, got %d", config.Port)
	}

	// Test WithModel
	WithModel("test-model")(config)
	if config.Model != "test-model" {
		t.Errorf("WithModel failed, expected 'test-model', got '%s'", config.Model)
	}

	// Test WithContextSize
	WithContextSize(8192)(config)
	if config.ContextSize != 8192 {
		t.Errorf("WithContextSize failed, expected 8192, got %d", config.ContextSize)
	}

	// Test WithContBatching
	WithContBatching(false)(config)
	if config.ContBatching {
		t.Error("WithContBatching failed, expected false")
	}

	// Test WithBatchSize
	WithBatchSize(2048)(config)
	if config.BatchSize != 2048 {
		t.Errorf("WithBatchSize failed, expected 2048, got %d", config.BatchSize)
	}

	// Test WithThreads
	WithThreads(16)(config)
	if config.Threads != 16 {
		t.Errorf("WithThreads failed, expected 16, got %d", config.Threads)
	}

	// Test WithDebug
	WithDebug(true)(config)
	if !config.Debug {
		t.Error("WithDebug failed, expected true")
	}
}

func TestGetSupportedModels(t *testing.T) {
	models := GetSupportedModels()

	if len(models) == 0 {
		t.Fatal("No supported models found")
	}

	// Check if Qwen3 model exists
	found := false
	for _, model := range models {
		if model.Name == "Qwen3-Embedding-0.6B-Q4_K_M" {
			found = true
			if model.Type != "embedding" {
				t.Errorf("Expected model type 'embedding', got '%s'", model.Type)
			}
			if model.DefaultPort != 8080 {
				t.Errorf("Expected default port 8080, got %d", model.DefaultPort)
			}
			break
		}
	}

	if !found {
		t.Error("Qwen3-Embedding-0.6B-Q4_K_M model not found in supported models")
	}
}

func TestFindModelConfig(t *testing.T) {
	// Test finding existing model
	model, err := FindModelConfig("Qwen3-Embedding-0.6B-Q4_K_M")
	if err != nil {
		t.Fatalf("Failed to find model: %v", err)
	}

	if model.Name != "Qwen3-Embedding-0.6B-Q4_K_M" {
		t.Errorf("Expected model name 'Qwen3-Embedding-0.6B-Q4_K_M', got '%s'", model.Name)
	}

	// Test finding non-existing model
	_, err = FindModelConfig("non-existing-model")
	if err == nil {
		t.Error("Expected error for non-existing model, got nil")
	}
}

func TestServiceStatus(t *testing.T) {
	tests := []struct {
		status   ServiceStatus
		expected string
	}{
		{StatusStopped, "stopped"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusStopping, "stopping"},
		{StatusError, "error"},
	}

	for _, test := range tests {
		if test.status.String() != test.expected {
			t.Errorf("Status %d expected '%s', got '%s'",
				test.status, test.expected, test.status.String())
		}
	}
}

func TestIsModelSupported(t *testing.T) {
	if !IsModelSupported("Qwen3-Embedding-0.6B-Q4_K_M") {
		t.Error("Expected Qwen3-Embedding-0.6B-Q4_K_M to be supported")
	}

	if IsModelSupported("non-existing-model") {
		t.Error("Expected non-existing-model to not be supported")
	}
}

func TestGetSupportedModelNames(t *testing.T) {
	names := GetSupportedModelNames()

	if len(names) == 0 {
		t.Fatal("No model names returned")
	}

	found := false
	for _, name := range names {
		if name == "Qwen3-Embedding-0.6B-Q4_K_M" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find Qwen3-Embedding-0.6B-Q4_K_M in model names")
	}
}

func TestManagerModelAPIs(t *testing.T) {
	manager := GetManager()

	// 测试获取默认模型路径
	defaultPath := GetDefaultEmbeddingModelPath()
	if defaultPath == "" {
		t.Error("Default embedding model path should not be empty")
	}

	// 测试获取本地模型路径
	modelPath, err := manager.GetLocalModelPath("Qwen3-Embedding-0.6B-Q4_K_M")
	if err != nil {
		t.Errorf("Failed to get local model path: %v", err)
	}

	if modelPath != defaultPath {
		t.Error("Qwen3 model path should match default path")
	}

	// 测试空模型名称（应该返回默认路径）
	emptyModelPath, err := manager.GetLocalModelPath("")
	if err != nil {
		t.Errorf("Failed to get default model path with empty name: %v", err)
	}

	if emptyModelPath != defaultPath {
		t.Error("Empty model name should return default path")
	}

	// 测试列出本地模型（这个测试可能会失败，因为模型文件可能不存在）
	localModels := manager.ListLocalModels()
	t.Logf("Local models found: %v", localModels)

	// 测试模型存在性检查
	exists := manager.IsLocalModelExists("Qwen3-Embedding-0.6B-Q4_K_M")
	t.Logf("Qwen3 model exists: %t", exists)

	// 测试默认模型可用性
	available := IsDefaultModelAvailable()
	t.Logf("Default model available: %t", available)
}

func TestRefreshServiceListFromProcess(t *testing.T) {
	manager := GetManager()

	// 测试刷新服务列表
	services := manager.refreshServiceListFromProcess()
	t.Logf("Found %d services from processes", len(services))

	for _, service := range services {
		t.Logf("Service: %s, Status: %s, Host: %s, Port: %d",
			service.Name, service.Status, service.Config.Host, service.Config.Port)
	}
}

func TestParseArgsToConfig(t *testing.T) {
	manager := GetManager()

	testCases := []struct {
		name     string
		args     []string
		expected *ServiceConfig
	}{
		{
			name: "basic configuration",
			args: []string{
				"--host", "127.0.0.1",
				"--port", "8080",
				"--model", "Qwen3-Embedding-0.6B-Q4_K_M",
				"--context-size", "4096",
				"--debug",
				"--cont-batching",
			},
			expected: &ServiceConfig{
				Host:         "127.0.0.1",
				Port:         8080,
				Model:        "Qwen3-Embedding-0.6B-Q4_K_M",
				ContextSize:  4096,
				Debug:        true,
				ContBatching: true,
			},
		},
		{
			name: "minimal configuration",
			args: []string{
				"--host", "0.0.0.0",
				"--port", "9090",
			},
			expected: &ServiceConfig{
				Host: "0.0.0.0",
				Port: 9090,
			},
		},
		{
			name: "invalid configuration - missing port",
			args: []string{
				"--host", "127.0.0.1",
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := manager.parseArgsToConfig(tc.args)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("Expected nil config, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("Expected config, got nil")
			}

			if result.Host != tc.expected.Host {
				t.Errorf("Expected host %s, got %s", tc.expected.Host, result.Host)
			}

			if result.Port != tc.expected.Port {
				t.Errorf("Expected port %d, got %d", tc.expected.Port, result.Port)
			}

			if tc.expected.Model != "" && result.Model != tc.expected.Model {
				t.Errorf("Expected model %s, got %s", tc.expected.Model, result.Model)
			}

			if tc.expected.ContextSize != 0 && result.ContextSize != tc.expected.ContextSize {
				t.Errorf("Expected context size %d, got %d", tc.expected.ContextSize, result.ContextSize)
			}

			if result.Debug != tc.expected.Debug {
				t.Errorf("Expected debug %t, got %t", tc.expected.Debug, result.Debug)
			}

			if result.ContBatching != tc.expected.ContBatching {
				t.Errorf("Expected cont batching %t, got %t", tc.expected.ContBatching, result.ContBatching)
			}
		})
	}
}

func TestToUTF8(t *testing.T) {
	gbkBytes := []byte{0xd6, 0xd0, 0xce, 0xc4, 0xba, 0xc3}
	utf8 := toUTF8(gbkBytes)
	fmt.Printf("GBK: %s, UTF-8: %s\n", string(gbkBytes), utf8)

	utf8Bytes := []byte{0xe6, 0x96, 0x87, 0xe6, 0x9c, 0xac}
	gbk := toUTF8(utf8Bytes)
	fmt.Printf("UTF-8: %s, GBK: %s\n", string(utf8Bytes), gbk)
}
