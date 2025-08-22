package localmodel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
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

func TestIsYakLocalModelCommand(t *testing.T) {
	manager := GetManager()

	testCases := []struct {
		command  string
		expected bool
	}{
		{"/usr/bin/yak localmodel --host 127.0.0.1 --port 8080", true},
		{"./yak localmodel --debug", true},
		{"yak localmodel", true},
		{"/path/to/yaklang localmodel --model test", true},
		{"python main.py", false},
		{"yak help", false},
		{"localmodel", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.command, func(t *testing.T) {
			result := manager.isYakLocalModelCommand(tc.command)
			if result != tc.expected {
				t.Errorf("Command '%s': expected %t, got %t", tc.command, tc.expected, result)
			}
		})
	}
}

func TestParseProcessToService(t *testing.T) {
	manager := GetManager()

	testCases := []struct {
		name     string
		proc     *ProcessInfo
		expected bool // 是否应该返回有效的服务信息
	}{
		{
			name: "valid yak localmodel process",
			proc: &ProcessInfo{
				PID:     12345,
				Command: "/usr/bin/yak localmodel --host 127.0.0.1 --port 8080 --debug",
				Args: []string{
					"/usr/bin/yak", "localmodel",
					"--host", "127.0.0.1",
					"--port", "8080",
					"--debug",
				},
			},
			expected: true,
		},
		{
			name: "invalid process - no localmodel",
			proc: &ProcessInfo{
				PID:     12346,
				Command: "/usr/bin/yak help",
				Args:    []string{"/usr/bin/yak", "help"},
			},
			expected: false,
		},
		{
			name: "invalid process - too few args",
			proc: &ProcessInfo{
				PID:     12347,
				Command: "yak",
				Args:    []string{"yak"},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := manager.parseProcessToService(tc.proc)

			if tc.expected {
				if result == nil {
					t.Errorf("Expected valid service info, got nil")
				} else {
					t.Logf("Service: %s, Host: %s, Port: %d",
						result.Name, result.Config.Host, result.Config.Port)
				}
			} else {
				if result != nil {
					t.Errorf("Expected nil service info, got %+v", result)
				}
			}
		})
	}
}

func TestParsePsOutput(t *testing.T) {
	manager := GetManager()

	// 模拟 ps 命令输出
	psOutput := `  PID  PPID COMMAND
  123   456 /usr/bin/yak localmodel --host 127.0.0.1 --port 8080
  789   123 /usr/bin/python3 script.py
  999   456 ./yak localmodel --debug --port 9090 --host 0.0.0.0`

	processes, err := manager.parsePsOutput(psOutput)
	if err != nil {
		t.Fatalf("Failed to parse ps output: %v", err)
	}

	expectedCount := 2 // 应该找到 2 个 yak localmodel 进程
	if len(processes) != expectedCount {
		t.Errorf("Expected %d processes, got %d", expectedCount, len(processes))
	}

	for _, proc := range processes {
		if !strings.Contains(proc.Command, "yak") || !strings.Contains(proc.Command, "localmodel") {
			t.Errorf("Process should contain yak and localmodel: %s", proc.Command)
		}
	}
}

func TestParseWmicOutput(t *testing.T) {
	manager := GetManager()

	// 模拟 wmic 命令输出
	wmicOutput := `Node,CommandLine,ParentProcessId,ProcessId
LUOLUO,yak  grpc --port 18087 --host 0.0.0.0,54576,58092
LUOLUO,F:\飞书文件\yak.exe localmodel --host 127.0.0.1 --port 8080 --context-size 4096 --model-path "D:\Program Files\Yakit\Yakit\yakit-projects\projects\libs\aimodel\Qwen3-Embedding-0.6B-Q4_K_M.gguf" --cont-batching --batch-size 1024 --threads 8 --debug --timeout 10 --llama-server-path "D:\Program Files\Yakit\Yakit\yakit-projects\projects\libs\llama-server\llama-server.exe",58092,63056`

	processes, err := manager.parseWmicOutput(wmicOutput)
	if err != nil {
		t.Fatalf("Failed to parse wmic output: %v", err)
	}

	expectedCount := 1 // 应该找到 2 个 yak localmodel 进程
	if len(processes) != expectedCount {
		t.Errorf("Expected %d processes, got %d", expectedCount, len(processes))
	}

	for _, proc := range processes {
		if !strings.Contains(proc.Command, "yak") || !strings.Contains(proc.Command, "localmodel") {
			t.Errorf("Process should contain yak and localmodel: %s", proc.Command)
		}
	}
}

func TestFindYakLocalModelProcesses(t *testing.T) {
	manager := GetManager()

	// 这个测试依赖于实际的系统命令，所以我们主要测试函数不会崩溃
	processes, err := manager.findYakLocalModelProcesses()
	if err != nil {
		t.Logf("Failed to find processes (expected on systems without ps/wmic): %v", err)
		return
	}

	t.Logf("Found %d yak localmodel processes", len(processes))
	for _, proc := range processes {
		t.Logf("Process PID %d: %s", proc.PID, proc.Command)
	}
}

func TestCrossPlatformSupport(t *testing.T) {
	manager := GetManager()

	// 测试当前操作系统是否被支持
	supportedOS := []string{"windows", "linux", "darwin"}
	currentOS := runtime.GOOS

	isSupported := false
	for _, os := range supportedOS {
		if os == currentOS {
			isSupported = true
			break
		}
	}

	if !isSupported {
		t.Logf("Current OS %s is not supported", currentOS)

		// 测试不支持的操作系统应该返回错误
		_, err := manager.findYakLocalModelProcesses()
		if err == nil {
			t.Errorf("Expected error for unsupported OS %s", currentOS)
		}
	} else {
		t.Logf("Current OS %s is supported", currentOS)

		// 对于支持的操作系统，函数应该至少不会崩溃
		_, err := manager.findYakLocalModelProcesses()
		if err != nil {
			t.Logf("Process discovery failed (might be normal): %v", err)
		}
	}
}

// ==================== 集成测试 ====================

var mockServerBinary string

// buildMockServer 编译 mock llama-server 二进制文件
func buildMockServer(t *testing.T) string {
	if mockServerBinary != "" {
		return mockServerBinary
	}

	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// mock server 源码路径
	testcmdDir := filepath.Join(wd, "testcmd")

	// 检查源码是否存在
	mainGoPath := filepath.Join(testcmdDir, "main.go")
	if _, err := os.Stat(mainGoPath); os.IsNotExist(err) {
		t.Fatalf("Mock server source not found at: %s", mainGoPath)
	}

	// 二进制文件路径
	binaryName := "mock-llama-server"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(testcmdDir, binaryName)

	// 编译命令
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = testcmdDir

	t.Logf("Building mock server: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build mock server: %v\nOutput: %s", err, string(output))
	}

	// 检查二进制文件是否生成
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("Mock server binary not found after build: %s", binaryPath)
	}

	t.Logf("Mock server built successfully: %s", binaryPath)
	mockServerBinary = binaryPath
	return binaryPath
}

// TestMUSTPASS_ServiceLifecycle 测试服务生命周期
func TestMUSTPASS_ServiceLifecycle(t *testing.T) {
	// 编译 mock server
	mockPath := buildMockServer(t)

	// 创建管理器实例
	manager := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: GetDefaultYakBinaryPath(),
	}

	// 使用随机端口避免冲突
	basePort := utils.GetRandomAvailableTCPPort()
	address := fmt.Sprintf("127.0.0.1:%d", basePort)

	t.Logf("Testing service lifecycle on %s", address)

	// 创建临时模型文件
	tmpModelPath := filepath.Join(os.TempDir(), "test-model.gguf")
	tmpFile, err := os.Create(tmpModelPath)
	if err != nil {
		t.Fatalf("Failed to create temp model file: %v", err)
	}
	tmpFile.WriteString("fake model content")
	tmpFile.Close()
	defer os.Remove(tmpModelPath)

	// 启动服务
	err = manager.StartEmbeddingService(
		address,
		WithModel("test-model"),
		WithModelPath(tmpModelPath),
		WithLlamaServerPath(mockPath),
		WithDebug(true),
		WithStartupTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// 等待服务启动
	time.Sleep(2 * time.Second)

	// 检查服务状态
	serviceName := fmt.Sprintf("embedding-127.0.0.1-%d", basePort)
	serviceInfo, err := manager.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get service status: %v", err)
	}

	t.Logf("Service status: %s", serviceInfo.Status)
	if serviceInfo.Status != StatusRunning {
		t.Errorf("Expected service status %s, got %s", StatusRunning, serviceInfo.Status)
	}

	// 测试服务列表
	services := manager.ListServices()
	if len(services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(services))
	}

	// 验证服务可访问性
	err = manager.WaitForEmbeddingService(address, 5.0)
	if err != nil {
		t.Errorf("Service not accessible: %v", err)
	}

	// 停止服务
	err = manager.StopService(serviceName)
	if err != nil {
		t.Fatalf("Failed to stop service: %v", err)
	}

	// 等待服务停止
	time.Sleep(2 * time.Second)

	// 检查服务状态
	serviceInfo, err = manager.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get service status after stop: %v", err)
	}

	if serviceInfo.Status != StatusStopped {
		t.Errorf("Expected service status %s, got %s", StatusStopped, serviceInfo.Status)
	}
}

// TestMUSTPASS_DetachedMode 测试 Detached 模式
func TestMUSTPASS_DetachedMode(t *testing.T) {
	mockPath := buildMockServer(t)
	// 创建管理器实例
	manager := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: GetDefaultYakBinaryPath(),
	}

	// 使用随机端口
	basePort := utils.GetRandomAvailableTCPPort()
	address := fmt.Sprintf("127.0.0.1:%d", basePort)

	t.Logf("Testing detached mode on %s", address)

	// 创建临时模型文件
	tmpModelPath := filepath.Join(os.TempDir(), "test-detached-model.gguf")
	tmpFile, err := os.Create(tmpModelPath)
	if err != nil {
		t.Fatalf("Failed to create temp model file: %v", err)
	}
	tmpFile.WriteString("fake model content")
	tmpFile.Close()
	defer os.Remove(tmpModelPath)

	// 启动 detached 模式服务
	err = manager.StartEmbeddingService(
		address,
		WithModel("test-model"),
		WithModelPath(tmpModelPath),
		WithLlamaServerPath(mockPath),
		WithDetached(true),
		WithDebug(true),
		WithStartupTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to start detached service: %v", err)
	}

	// 等待服务启动
	time.Sleep(3 * time.Second)

	// 检查服务状态
	serviceName := fmt.Sprintf("embedding-127.0.0.1-%d", basePort)
	serviceInfo, err := manager.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get detached service status: %v", err)
	}

	t.Logf("Detached service status: %s", serviceInfo.Status)
	if serviceInfo.Status != StatusRunning {
		t.Errorf("Expected detached service status %s, got %s", StatusRunning, serviceInfo.Status)
	}

	// 测试从进程中刷新服务列表
	discoveredServices := manager.refreshServiceListFromProcess()
	t.Logf("Discovered %d services from processes", len(discoveredServices))

	// 停止服务
	err = manager.StopService(serviceName)
	if err != nil {
		t.Fatalf("Failed to stop detached service: %v", err)
	}

	// 等待服务停止
	time.Sleep(2 * time.Second)

	// 测试服务列表
	services := manager.ListServices()
	if len(services) != 0 {
		t.Errorf("Expected 0 services, got %d", len(services))
	}
}

// TestMUSTPASS_ProcessDiscovery 测试进程发现功能
func TestMUSTPASS_ProcessDiscovery(t *testing.T) {
	manager := GetManager()

	// 测试跨平台进程发现
	processes, err := manager.findYakLocalModelProcesses()
	if err != nil {
		t.Logf("Process discovery failed (might be normal): %v", err)
	} else {
		t.Logf("Found %d yak localmodel processes", len(processes))
		for _, proc := range processes {
			t.Logf("Process PID %d: %s", proc.PID, proc.Command)
		}
	}

	// 测试服务列表刷新
	services := manager.refreshServiceListFromProcess()
	t.Logf("Refreshed %d services from processes", len(services))

	// 测试参数解析
	testArgs := []string{
		"--host", "192.168.1.100",
		"--port", "9999",
		"--model", "test-model",
		"--debug",
		"--cont-batching",
	}

	config := manager.parseArgsToConfig(testArgs)
	if config == nil {
		t.Error("Failed to parse test arguments")
	} else {
		if config.Host != "192.168.1.100" {
			t.Errorf("Expected host 192.168.1.100, got %s", config.Host)
		}
		if config.Port != 9999 {
			t.Errorf("Expected port 9999, got %d", config.Port)
		}
		if !config.Debug {
			t.Error("Expected debug to be true")
		}
		if !config.ContBatching {
			t.Error("Expected cont batching to be true")
		}
	}
}

// TestMUSTPASS_BinaryPathManagement 测试二进制路径管理
func TestMUSTPASS_BinaryPathManagement(t *testing.T) {
	// 创建管理器实例
	manager := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: "",
	}

	// 测试获取默认二进制路径（应该调用 os.Executable）
	defaultPath := manager.GetCurrentBinaryPathFromManager()
	t.Logf("Default binary path: %s", defaultPath)

	if defaultPath == "" {
		t.Error("Default binary path should not be empty")
	}

	// 测试设置自定义二进制路径
	customPath := "/usr/local/bin/custom-yak"
	manager.SetCurrentBinaryPath(customPath)

	currentPath := manager.GetCurrentBinaryPathFromManager()
	if currentPath != customPath {
		t.Errorf("Expected custom path %s, got %s", customPath, currentPath)
	}

	// 测试清空路径后重新获取默认路径
	manager.SetCurrentBinaryPath("")
	pathAfterClear := manager.GetCurrentBinaryPathFromManager()
	t.Logf("Path after clear: %s", pathAfterClear)

	if pathAfterClear == "" {
		t.Error("Path should fallback to os.Executable result when cleared")
	}

	// 测试在 Detached 模式中的应用
	manager.SetCurrentBinaryPath(customPath)

	// 创建临时模型文件
	tmpModelPath := filepath.Join(os.TempDir(), "test-binary-path-model.gguf")
	tmpFile, err := os.Create(tmpModelPath)
	if err != nil {
		t.Fatalf("Failed to create temp model file: %v", err)
	}
	tmpFile.WriteString("fake model content")
	tmpFile.Close()
	defer os.Remove(tmpModelPath)

	// 测试构建 detached 参数
	config := DefaultServiceConfig()
	config.Host = "127.0.0.1"
	config.Port = 23080
	config.Model = "test-model"
	config.ModelPath = tmpModelPath
	config.Detached = true

	args, err := manager.buildDetachedArgs(config)
	if err != nil {
		t.Fatalf("Failed to build detached args: %v", err)
	}

	t.Logf("Detached args: %v", args)

	// 验证参数包含正确的配置
	if len(args) == 0 || args[0] != "localmodel" {
		t.Error("Detached args should start with 'localmodel'")
	}
}

// TestMUSTPASS_DetachedModePersistence 测试 Detached 模式的持久性和进程发现
func TestMUSTPASS_DetachedModePersistence(t *testing.T) {
	// 使用随机端口
	basePort := utils.GetRandomAvailableTCPPort()
	address := fmt.Sprintf("127.0.0.1:%d", basePort)
	serviceName := fmt.Sprintf("embedding-127.0.0.1-%d", basePort)

	t.Logf("Testing detached mode persistence on %s", address)

	mockPath := buildMockServer(t)

	// 第一阶段：创建第一个 manager 并启动 detached 服务
	manager1 := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: GetDefaultYakBinaryPath(),
	}

	// 创建临时模型文件
	tmpModelPath := filepath.Join(os.TempDir(), "test-persistence-model.gguf")
	tmpFile, err := os.Create(tmpModelPath)
	if err != nil {
		t.Fatalf("Failed to create temp model file: %v", err)
	}
	tmpFile.WriteString("fake model content")
	tmpFile.Close()
	defer os.Remove(tmpModelPath)

	// 启动 detached 模式服务
	err = manager1.StartEmbeddingService(
		address,
		WithModel("persistence-test-model"),
		WithModelPath(tmpModelPath),
		WithLlamaServerPath(mockPath),
		WithDetached(true),
		WithDebug(true),
		WithStartupTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to start detached service with manager1: %v", err)
	}

	// 等待服务启动
	time.Sleep(3 * time.Second)

	// 验证服务在 manager1 中正在运行
	serviceInfo1, err := manager1.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get service status from manager1: %v", err)
	}

	if serviceInfo1.Status != StatusRunning {
		t.Errorf("Service should be running in manager1, got: %s", serviceInfo1.Status)
	}

	t.Logf("Phase 1: Service started with manager1, status: %s", serviceInfo1.Status)

	// 第二阶段：销毁第一个 manager，创建新的 manager
	manager1 = nil // 模拟 manager 被销毁
	runtime.GC()   // 强制垃圾回收

	// 创建新的 manager2
	manager2 := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: GetDefaultYakBinaryPath(),
	}

	t.Logf("Phase 2: Created new manager2 after destroying manager1")

	// 等待一段时间确保进程稳定
	time.Sleep(2 * time.Second)

	// 第三阶段：使用进程发现功能寻找运行中的 detached 服务
	discoveredServices := manager2.refreshServiceListFromProcess()
	t.Logf("Phase 3: Discovered %d services from processes", len(discoveredServices))

	// 验证是否能通过进程发现找到我们的服务
	var foundService *ServiceInfo
	for _, service := range discoveredServices {
		if service.Config.Port == int32(basePort) && service.Config.Host == "127.0.0.1" {
			foundService = service
			break
		}
	}

	if foundService == nil {
		t.Logf("Warning: Service not found through process discovery. This might be normal if the detached process hasn't fully started.")
	} else {
		t.Logf("Found service through process discovery: %s, Host: %s, Port: %d",
			foundService.Name, foundService.Config.Host, foundService.Config.Port)

		// 验证发现的服务配置
		if foundService.Config.Model != "persistence-test-model" {
			t.Logf("Note: Model name from process discovery: %s (expected: persistence-test-model)",
				foundService.Config.Model)
		}
	}

	// 第四阶段：尝试通过 manager2 控制服务（即使没有通过进程发现找到）

	// 验证 manager2 可以获取服务状态
	serviceInfo2, err := manager2.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get service status from manager2: %v", err)
	}

	t.Logf("Phase 4: Manager2 can access service: %s", serviceInfo2.Name)

	// 第五阶段：测试服务生命周期控制
	// 尝试停止服务
	err = manager2.StopService(serviceName)
	if err != nil {
		t.Fatalf("Failed to stop service with manager2: %v", err)
	}

	// 等待服务停止
	time.Sleep(3 * time.Second)

	// 验证服务已停止
	_, err = manager2.GetServiceStatus(serviceName)
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Failed to get service status after stop: %v", err)
	}
}

// TestMUSTPASS_DetachedModeProcessTermination 测试进程终止后的服务列表更新
func TestMUSTPASS_DetachedModeProcessTermination(t *testing.T) {
	// 使用随机端口
	basePort := utils.GetRandomAvailableTCPPort()
	address := fmt.Sprintf("127.0.0.1:%d", basePort)
	serviceName := fmt.Sprintf("embedding-127.0.0.1-%d", basePort)

	mockPath := buildMockServer(t)
	t.Logf("Testing process termination detection on %s", address)

	// 创建管理器
	manager := &Manager{
		services:          make(map[string]*ServiceInfo),
		currentBinaryPath: GetDefaultYakBinaryPath(),
	}

	// 创建临时模型文件
	tmpModelPath := filepath.Join(os.TempDir(), "test-termination-model.gguf")
	tmpFile, err := os.Create(tmpModelPath)
	if err != nil {
		t.Fatalf("Failed to create temp model file: %v", err)
	}
	tmpFile.WriteString("fake model content")
	tmpFile.Close()
	defer os.Remove(tmpModelPath)

	// 启动 detached 模式服务
	err = manager.StartEmbeddingService(
		address,
		WithModel("termination-test-model"),
		WithModelPath(tmpModelPath),
		WithDetached(true),
		WithDebug(true),
		WithLlamaServerPath(mockPath),
		WithStartupTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to start detached service: %v", err)
	}

	// 等待服务启动
	time.Sleep(3 * time.Second)

	// 验证服务正在运行
	serviceInfo1, err := manager.GetServiceStatus(serviceName)
	if err != nil {
		t.Fatalf("Failed to get service status: %v", err)
	}

	if serviceInfo1.Status != StatusRunning {
		t.Errorf("Service should be running, got: %s", serviceInfo1.Status)
	}

	t.Logf("Phase 1: Service started, status: %s", serviceInfo1.Status)

	// 获取进程信息，准备手动终止进程
	var targetProcess *ProcessInfo
	processes, err := manager.findYakLocalModelProcesses()
	if err != nil {
		t.Logf("Process discovery failed: %v", err)
	} else {
		for _, proc := range processes {
			if strings.Contains(proc.Command, fmt.Sprintf("--port %d", basePort)) {
				targetProcess = proc
				break
			}
		}
	}

	// 如果找到了进程，手动终止它
	if targetProcess != nil {
		t.Logf("Phase 2: Found target process PID %d, terminating manually", targetProcess.PID)

		// 尝试终止进程
		if runtime.GOOS == "windows" {
			exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", targetProcess.PID)).Run()
		} else {
			exec.Command("kill", "-9", fmt.Sprintf("%d", targetProcess.PID)).Run()
		}

		// 等待进程终止
		time.Sleep(2 * time.Second)

		// 验证进程已经不存在
		processesAfter, err := manager.findYakLocalModelProcesses()
		processFound := false
		if err == nil {
			for _, proc := range processesAfter {
				if proc.PID == targetProcess.PID {
					processFound = true
					break
				}
			}
		}

		if processFound {
			t.Logf("Warning: Process still found after termination attempt")
		} else {
			t.Logf("Phase 3: Process successfully terminated")
		}

		// 刷新服务列表，看是否能检测到进程已终止
		refreshedServices := manager.refreshServiceListFromProcess()
		t.Logf("Phase 4: After process termination, found %d services from process discovery", len(refreshedServices))

		// 检查刷新后的服务列表中是否还包含我们的服务
		serviceStillFound := false
		for _, service := range refreshedServices {
			if service.Config.Port == int32(basePort) && service.Config.Host == "127.0.0.1" {
				serviceStillFound = true
				break
			}
		}

		if serviceStillFound {
			t.Logf("Warning: Service still found in refreshed list after process termination")
		} else {
			t.Logf("Phase 5: Service correctly removed from process discovery after termination")
		}

		// 测试 manager 内部状态是否需要更新
		// 注意：这里的服务信息仍然在 manager 的内存中，但实际进程已终止
		internalServiceInfo, err := manager.GetServiceStatus(serviceName)
		if err == nil {
			t.Logf("Phase 6: Internal service status still shows: %s", internalServiceInfo.Status)
			t.Logf("Note: Manager's internal state may not automatically reflect process termination")
		}
	} else {
		t.Logf("Warning: Could not find target process for manual termination test")
	}

	// 清理：尝试停止服务（如果还在运行）
	manager.StopService(serviceName)
	time.Sleep(2 * time.Second)
}

func TestToUTF8(t *testing.T) {
	gbkBytes := []byte{0xd6, 0xd0, 0xce, 0xc4, 0xba, 0xc3}
	utf8 := toUTF8(gbkBytes)
	fmt.Printf("GBK: %s, UTF-8: %s\n", string(gbkBytes), utf8)

	utf8Bytes := []byte{0xe6, 0x96, 0x87, 0xe6, 0x9c, 0xac}
	gbk := toUTF8(utf8Bytes)
	fmt.Printf("UTF-8: %s, GBK: %s\n", string(utf8Bytes), gbk)
}
