package localmodel

import (
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

	// Test WithEmbeddingModel
	WithEmbeddingModel("test-model")(config)
	if config.Model != "test-model" {
		t.Errorf("WithEmbeddingModel failed, expected 'test-model', got '%s'", config.Model)
	}

	// Test WithContextSize
	WithContextSize(8192)(config)
	if config.ContextSize != 8192 {
		t.Errorf("WithContextSize failed, expected 8192, got %d", config.ContextSize)
	}

	// Test WithParallelism
	WithParallelism(4)(config)
	if config.Parallelism != 4 {
		t.Errorf("WithParallelism failed, expected 4, got %d", config.Parallelism)
	}

	// Test WithDetached
	WithDetached(true)(config)
	if !config.Detached {
		t.Error("WithDetached failed, expected true")
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
		if model.Name == "Qwen3-Embedding-0.6B-Q8_0" {
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
		t.Error("Qwen3-Embedding-0.6B-Q8_0 model not found in supported models")
	}
}

func TestFindModelConfig(t *testing.T) {
	// Test finding existing model
	model, err := FindModelConfig("Qwen3-Embedding-0.6B-Q8_0")
	if err != nil {
		t.Fatalf("Failed to find model: %v", err)
	}

	if model.Name != "Qwen3-Embedding-0.6B-Q8_0" {
		t.Errorf("Expected model name 'Qwen3-Embedding-0.6B-Q8_0', got '%s'", model.Name)
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
	if !IsModelSupported("Qwen3-Embedding-0.6B-Q8_0") {
		t.Error("Expected Qwen3-Embedding-0.6B-Q8_0 to be supported")
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
		if name == "Qwen3-Embedding-0.6B-Q8_0" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find Qwen3-Embedding-0.6B-Q8_0 in model names")
	}
}

func TestManagerModelAPIs(t *testing.T) {
	manager := GetManager()

	// 测试获取默认模型路径
	defaultPath := manager.GetDefaultEmbeddingModelPath()
	if defaultPath == "" {
		t.Error("Default embedding model path should not be empty")
	}

	// 测试获取本地模型路径
	modelPath, err := manager.GetLocalModelPath("Qwen3-Embedding-0.6B-Q8_0")
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
	exists := manager.IsLocalModelExists("Qwen3-Embedding-0.6B-Q8_0")
	t.Logf("Qwen3 model exists: %t", exists)

	// 测试默认模型可用性
	available := manager.IsDefaultModelAvailable()
	t.Logf("Default model available: %t", available)
}
