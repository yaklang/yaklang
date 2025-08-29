package localmodel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ServiceStatus 服务状态
type ServiceStatus int

const (
	StatusStopped ServiceStatus = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusError
)

// ServiceType 服务类型
type ServiceType string

const (
	ServiceTypeEmbedding ServiceType = "embedding"
	ServiceTypeChat      ServiceType = "aichat"
)

func (s ServiceStatus) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

func (t ServiceType) String() string {
	switch t {
	case ServiceTypeEmbedding:
		return "embedding"
	case ServiceTypeChat:
		return "aichat"
	default:
		return ""
	}
}

// ServiceInfo 服务信息
type ServiceInfo struct {
	Name      string         `json:"name"`
	Type      ServiceType    `json:"type"`
	Status    ServiceStatus  `json:"status"`
	Config    *ServiceConfig `json:"config"`
	ProcessID int            `json:"processID"`

	StartTime  time.Time `json:"startTime"`
	Process    *exec.Cmd `json:"-"`
	LastError  string    `json:"lastError,omitempty"`
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// Manager 本地模型管理器
type Manager struct {
	mutex             sync.RWMutex
	services          map[string]*ServiceInfo
	currentBinaryPath string // 当前二进制文件路径（用于 Detached 模式）
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

// GetManager 获取管理器单例实例
func GetManager() *Manager {
	managerOnce.Do(func() {
		currentBinary, _ := os.Executable() // 获取当前二进制路径，忽略错误
		managerInstance = &Manager{
			services:          make(map[string]*ServiceInfo),
			currentBinaryPath: currentBinary,
		}
	})
	return managerInstance
}

// NewManager 创建新的管理器实例 (已废弃，使用 GetManager 代替)
// Deprecated: Use GetManager() instead
func NewManager() *Manager {
	return GetManager()
}

// SetCurrentBinaryPath 设置当前二进制文件路径（用于 Detached 模式）
func (m *Manager) SetCurrentBinaryPath(path string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.currentBinaryPath = path
}

// GetCurrentBinaryPathFromManager 获取当前设置的二进制文件路径
func (m *Manager) GetCurrentBinaryPathFromManager() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.currentBinaryPath != "" {
		return m.currentBinaryPath
	}
	// 如果没有设置自定义路径，尝试获取当前可执行文件路径
	path, _ := os.Executable()
	return path
}

// IsLocalModelExists 检查本地模型是否存在
func (m *Manager) IsLocalModelExists(modelName string) bool {
	modelPath, err := m.GetLocalModelPath(modelName)
	if err != nil {
		return false
	}

	exists, err := utils.PathExists(modelPath)
	return err == nil && exists
}

// GetLocalModelPath 获取本地模型路径
func (m *Manager) GetLocalModelPath(modelName string) (string, error) {
	// 如果是默认的 Qwen3 模型，直接使用 consts 中的路径
	if modelName == "Qwen3-Embedding-0.6B-Q4_K_M" || modelName == "" {
		return consts.GetQwen3Embedding0_6BQ4_0ModelPath(), nil
	}

	// 其他模型使用原来的逻辑
	return GetModelPath(modelName)
}

// ListLocalModels 列出本地可用的模型
func (m *Manager) ListLocalModels() []string {
	var availableModels []string

	models := GetSupportedModels()
	for _, model := range models {
		if m.IsLocalModelExists(model.Name) {
			availableModels = append(availableModels, model.Name)
		}
	}

	return availableModels
}

// StartService 启动服务的通用方法
func (m *Manager) StartService(address string, options ...Option) error {
	m.refreshServiceListFromProcess()
	// 解析地址
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address format: %v", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %v", err)
	}

	// 创建默认配置
	config := DefaultServiceConfig()
	config.Host = host
	config.Port = int32(port)

	// 应用选项
	for _, option := range options {
		option(config)
	}

	// 检查port和当前服务是否冲突
	var allPorts []int32
	for _, service := range m.services {
		if service.ProcessID == os.Getpid() {
			continue
		}
		allPorts = append(allPorts, service.Config.Port)
	}

	if slices.Contains(allPorts, int32(port)) {
		return fmt.Errorf("port %d is already in use", port)
	}

	serviceType := config.ModelType
	if serviceType == "" {
		serviceType = string(ServiceTypeChat)
	}

	// 如果没有指定模型路径，尝试从模型名称获取
	if config.ModelPath == "" {
		if config.Model != "" {
			// 使用指定的模型
			modelPath, err := m.GetLocalModelPath(config.Model)
			if err != nil {
				return fmt.Errorf("failed to get model path: %v", err)
			}
			config.ModelPath = modelPath
		} else {
			// 没有指定模型，使用默认模型
			switch serviceType {
			case string(ServiceTypeEmbedding):
				config.ModelPath = GetDefaultEmbeddingModelPath()
				config.Model = "Qwen3-Embedding-0.6B-Q4_K_M"
			case string(ServiceTypeChat):
				defaultModel := GetDefaultChatModel()
				if defaultModel == nil {
					return fmt.Errorf("no default chat model available")
				}
				modelPath, err := m.GetLocalModelPath(defaultModel.Name)
				if err != nil {
					return fmt.Errorf("failed to get default chat model path: %v", err)
				}
				config.ModelPath = modelPath
				config.Model = defaultModel.Name
			default:
				return fmt.Errorf("unsupported service type: %v", serviceType)
			}
		}
	}

	// 验证模型路径
	if err := ValidateModelPath(config.ModelPath); err != nil {
		return fmt.Errorf("model validation failed: %v, for model: %v", err, config.ModelPath)
	}

	// 验证模型类型（确保模型类型与服务类型匹配）
	if config.Model != "" {
		if modelConfig, err := FindModelConfig(config.Model); err == nil {
			expectedType := ""
			switch serviceType {
			case string(ServiceTypeEmbedding):
				expectedType = "embedding"
			case string(ServiceTypeChat):
				expectedType = "chat"
			}
			if modelConfig.Type != expectedType {
				return fmt.Errorf("model %s is not a %s model (type: %s)", config.Model, expectedType, modelConfig.Type)
			}
		}
	}

	// 生成服务名称
	serviceName := fmt.Sprintf("%s-%s-%d", serviceType, host, port)

	// 检查服务是否已存在
	m.mutex.Lock()
	if service, exists := m.services[serviceName]; exists {
		if service.Status == StatusRunning || service.Status == StatusStarting {
			m.mutex.Unlock()
			return fmt.Errorf("service already running: %s", serviceName)
		}
	}

	// 创建服务信息
	ctx, cancelFunc := context.WithCancel(context.Background())
	service := &ServiceInfo{
		Name:       serviceName,
		Type:       ServiceType(serviceType),
		Status:     StatusStarting,
		Config:     config,
		StartTime:  time.Now(),
		ctx:        ctx,
		cancelFunc: cancelFunc,
	}

	m.services[serviceName] = service
	m.mutex.Unlock()

	// 异步启动服务
	done := make(chan error, 3)
	go m.startServiceInternal(service, config.LlamaServerPath, done)

	// 等待启动完成或超时
	if err := m.waitForService(service); err != nil {
		return err
	}

	log.Infof("Starting %s service: %s", serviceType, serviceName)
	return nil
}

// StartEmbeddingService 启动嵌入服务
func (m *Manager) StartEmbeddingService(address string, options ...Option) error {
	return m.StartService(address, append(options, WithModelType("embedding"))...)
}

// StartChatService 启动聊天服务
func (m *Manager) StartChatService(address string, options ...Option) error {
	return m.StartService(address, append(options, WithModelType("aichat"))...)
}

// startServiceInternal 启动服务的内部方法
func (m *Manager) startServiceInternal(service *ServiceInfo, llamaServerPath string, done chan error) {
	doneOnce := new(sync.Once)
	finished := func(err error) {
		doneOnce.Do(func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Service %s panic: %v", service.Name, r)
					m.updateServiceStatus(service.Name, StatusError, fmt.Sprintf("panic: %v", r))
				}
			}()

			if err != nil {
				log.Errorf("Service %s failed to start: %v", service.Name, err)
				m.updateServiceStatus(service.Name, StatusError, err.Error())
			} else {
				log.Infof("Service %s started successfully", service.Name)
				m.updateServiceStatus(service.Name, StatusRunning, "")
			}
			done <- err
			close(done)
		})
	}

	defer func() {
		if r := recover(); r != nil {
			finished(fmt.Errorf("panic: %v", r))
			log.Errorf("Service %s panic: %v", service.Name, r)
			m.updateServiceStatus(service.Name, StatusError, fmt.Sprintf("panic: %v", r))
		} else {
			finished(nil)
		}
	}()

	var cmd *exec.Cmd

	args := m.buildArgs(service.Config)
	log.Infof("Starting command: %s %s", llamaServerPath, strings.Join(args, " "))
	cmd = exec.CommandContext(service.ctx, llamaServerPath, args...)
	var reader, combinedOutput = utils.NewPipe()
	defer func() {
		combinedOutput.Close()
	}()
	var stdout io.Writer = combinedOutput
	var stderr io.Writer = combinedOutput
	if service.Config.Debug {
		stdout = io.MultiWriter(stdout, os.Stdout)
		stderr = io.MultiWriter(stderr, os.Stderr)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// 启动进程
	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start service %s: %v", service.Name, err)
		finished(err)
		m.updateServiceStatus(service.Name, StatusError, err.Error())
		return
	}

	// 更新服务信息
	m.mutex.Lock()
	service.Process = cmd
	service.Status = StatusRunning
	service.ProcessID = cmd.Process.Pid
	m.mutex.Unlock()

	log.Infof("Service %s started with PID: %d", service.Name, cmd.Process.Pid)
	for {
		line, n, err := utils.ReadLineEx(ctxio.NewReader(utils.TimeoutContextSeconds(15), reader))
		if n > 0 && strings.HasPrefix(line, "main: server is listening on http://") && strings.Contains(line, "starting the main loop") {
			log.Infof("Starting main loop for service %s", service.Name)
			finished(nil)
			break
		}
		if err != nil {
			finished(nil)
		}
	}

	// 等待进程结束
	err := cmd.Wait()

	// 清理服务
	m.mutex.Lock()
	if service.Status != StatusStopping {
		if err != nil {
			service.Status = StatusError
			service.LastError = err.Error()
			log.Errorf("Service %s exited with error: %v", service.Name, err)
		} else {
			service.Status = StatusStopped
			log.Infof("Service %s exited normally", service.Name)
		}
	} else {
		service.Status = StatusStopped
		log.Infof("Service %s stopped", service.Name)
	}
	m.mutex.Unlock()
}

// buildArgs 构建启动参数
func (m *Manager) buildArgs(config *ServiceConfig) []string {
	args := []string{
		"-m", config.ModelPath,
		"--host", config.Host,
		"--port", fmt.Sprintf("%d", config.Port),
		"--ctx-size", fmt.Sprintf("%d", config.ContextSize),
		"--verbose-prompt",
	}

	if config.ModelType == "embedding" {
		args = append(args, "--embedding")
	}

	if config.Pooling != "" {
		args = append(args, "--pooling", config.Pooling)
	}

	// 连续批处理
	if config.ContBatching {
		args = append(args, "--cont-batching")
	}

	// 批处理大小
	if config.BatchSize > 0 {
		args = append(args, "--batch-size", fmt.Sprintf("%d", config.BatchSize))
	}

	// 线程数
	if config.Threads > 0 {
		args = append(args, "--threads", fmt.Sprintf("%d", config.Threads))
	}

	// 添加额外参数
	args = append(args, config.Args...)

	return args
}

// waitForService 等待服务启动完成
func (m *Manager) waitForService(service *ServiceInfo) error {
	address := fmt.Sprintf("%s:%d", service.Config.Host, service.Config.Port)
	timeoutSeconds := service.Config.StartupTimeout.Seconds()

	log.Infof("waiting for service %s to be ready on %s (timeout: %.1fs)", service.Name, address, timeoutSeconds)

	// 使用 utils.WaitConnect 等待连接
	err := utils.WaitConnect(address, timeoutSeconds)
	if err != nil {
		// 检查进程是否还在运行
		if service.Process != nil && service.Process.ProcessState != nil && service.Process.ProcessState.Exited() {
			return fmt.Errorf("process exited during startup: %v", err)
		}
		return fmt.Errorf("service startup timeout after %v: %v", service.Config.StartupTimeout, err)
	}

	log.Infof("service %s is ready on %s", service.Name, address)
	return nil
}

// WaitForEmbeddingService 等待嵌入服务完全启动并可用
func (m *Manager) WaitForEmbeddingService(address string, timeoutSeconds float64) error {
	log.Infof("waiting for embedding service to be ready on %s (timeout: %.1fs)", address, timeoutSeconds)

	err := utils.WaitConnect(address, timeoutSeconds)
	if err != nil {
		return fmt.Errorf("embedding service not ready: %v", err)
	}

	log.Infof("embedding service is ready on %s", address)
	return nil
}

// updateServiceStatus 更新服务状态
func (m *Manager) updateServiceStatus(serviceName string, status ServiceStatus, errorMsg string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if service, exists := m.services[serviceName]; exists {
		service.Status = status
		if errorMsg != "" {
			service.LastError = errorMsg
		}
	}
}

// StopService 停止指定服务
func (m *Manager) StopService(serviceName string) error {
	m.mutex.Lock()
	service, exists := m.services[serviceName]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("service not found: %s", serviceName)
	}

	if service.Status == StatusStopped || service.Status == StatusStopping {
		m.mutex.Unlock()
		return fmt.Errorf("service already stopped or stopping: %s", serviceName)
	}

	service.Status = StatusStopping
	m.mutex.Unlock()

	log.Infof("Stopping service: %s", serviceName)

	if service.Process == nil {
		// 跨平台的进程终止
		err := m.killDetachedService(service)
		if err != nil {
			log.Warnf("Failed to kill service %s: %v", serviceName, err)
		}
		return nil
	} else {
		// 取消上下文
		if service.cancelFunc != nil {
			service.cancelFunc()
		}

		// 等待进程结束
		if service.Process != nil && service.Process.ProcessState == nil {
			// 先尝试优雅停止
			if err := service.Process.Process.Signal(os.Interrupt); err != nil {
				log.Warnf("Failed to send interrupt signal to service %s: %v", serviceName, err)
			}

			// 等待一段时间
			time.Sleep(3 * time.Second)

			// 如果还没结束，强制杀死
			if service.Process.ProcessState == nil {
				if err := service.Process.Process.Kill(); err != nil {
					log.Warnf("Failed to kill service %s: %v", serviceName, err)
				}
			}
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	service.Status = StatusStopped
	return nil
}

// StopAllServices 停止所有服务
func (m *Manager) StopAllServices() error {
	m.mutex.RLock()
	serviceNames := make([]string, 0, len(m.services))
	for name := range m.services {
		serviceNames = append(serviceNames, name)
	}
	m.mutex.RUnlock()

	var errors []error
	for _, name := range serviceNames {
		if err := m.StopService(name); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some services: %v", errors)
	}

	return nil
}

// ErrServiceNotFound 服务不存在错误
var ErrServiceNotFound = errors.New("service not found")

// GetServiceStatus 获取服务状态
func (m *Manager) GetServiceStatus(serviceName string) (*ServiceInfo, error) {
	m.ListServices()
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	service, exists := m.services[serviceName]
	if !exists {
		return nil, fmt.Errorf("stop service %s failed: %w", serviceName, ErrServiceNotFound)
	}

	// 返回副本以避免并发修改
	return &ServiceInfo{
		Name:      service.Name,
		Status:    service.Status,
		Config:    service.Config,
		StartTime: service.StartTime,
		LastError: service.LastError,
	}, nil
}

// ListServices 列出所有服务
func (m *Manager) ListServices() []*ServiceInfo {
	// 先刷新服务列表，从进程中发现新的服务
	m.refreshServiceListFromProcess()

	// 现在获取完整的服务列表
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	services := make([]*ServiceInfo, 0, len(m.services))
	for _, service := range m.services {
		services = append(services, &ServiceInfo{
			Name:      service.Name,
			Status:    service.Status,
			Config:    service.Config,
			StartTime: service.StartTime,
			LastError: service.LastError,
		})
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

// refreshServiceListFromProcess 从正在运行的进程中刷新服务列表
func (m *Manager) refreshServiceListFromProcess() []*ServiceInfo {
	processes, err := m.findLlamaServerProcesses()
	if err != nil {
		log.Errorf("Failed to find llama-server processes: %v", err)
		return nil
	}

	var services []*ServiceInfo
	for _, proc := range processes {
		serviceInfo := m.parseProcessToService(proc)
		if serviceInfo != nil {
			services = append(services, serviceInfo)
		}
	}

	m.mutex.Lock()
	// 将发现的服务合并到内部服务列表中，通过process id匹配
	idToService := make(map[int]*ServiceInfo)
	for _, service := range m.services {
		idToService[service.ProcessID] = service
	}
	allAliveProcessePid := []int{}
	for _, discoveredService := range services {
		if _, exists := idToService[discoveredService.ProcessID]; !exists {
			allAliveProcessePid = append(allAliveProcessePid, discoveredService.ProcessID)
			m.services[discoveredService.Name] = discoveredService
		}
	}
	// 清理掉已经停止的进程
	for _, service := range m.services {
		if !slices.Contains(allAliveProcessePid, service.ProcessID) {
			delete(m.services, service.Name)
		}
	}
	m.mutex.Unlock()
	return services
}

// parseProcessToService 将进程信息转换为服务信息
func (m *Manager) parseProcessToService(proc *ProcessInfo) *ServiceInfo {
	if len(proc.Args) < 1 {
		return nil
	}

	// 查找 llama-server 在参数中的位置
	llamaServerIndex := -1

	for i, arg := range proc.Args {
		if strings.Contains(arg, "llama-server") {
			llamaServerIndex = i
			break
		}
	}

	if llamaServerIndex == -1 {
		return nil
	}

	// 解析 llama-server 后面的参数
	args := proc.Args[llamaServerIndex+1:]
	config := m.parseArgsToConfig(args)
	if config == nil {
		return nil
	}

	// 生成服务名称
	serviceName := fmt.Sprintf("%s-%s-%d-%d", config.ModelType, config.Model, config.Port, proc.PID)

	return &ServiceInfo{
		Name:      serviceName,
		Status:    StatusRunning, // 进程存在说明正在运行
		Config:    config,
		ProcessID: proc.PID,
	}
}

// parseArgsToConfig 将命令行参数解析为服务配置
func (m *Manager) parseArgsToConfig(args []string) *ServiceConfig {
	// 创建一个空的配置，不使用默认值
	config := &ServiceConfig{
		Host:           "",
		Port:           0,
		Model:          "",
		ModelPath:      "",
		ContextSize:    0,
		ContBatching:   false,
		BatchSize:      0,
		Threads:        0,
		Debug:          false,
		Pooling:        "",
		StartupTimeout: 0,
		Args:           []string{},
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "--host":
			if i+1 < len(args) {
				config.Host = args[i+1]
				i++
			}
		case "--port":
			if i+1 < len(args) {
				if port, err := strconv.Atoi(args[i+1]); err == nil {
					config.Port = int32(port)
				}
				i++
			}
		case "--model":
			if i+1 < len(args) {
				config.Model = args[i+1]
				i++
			}
		case "--model-path", "-m":
			if i+1 < len(args) {
				config.ModelPath = args[i+1]
				i++
			}
		case "--context-size":
			if i+1 < len(args) {
				if size, err := strconv.Atoi(args[i+1]); err == nil {
					config.ContextSize = size
				}
				i++
			}
		case "--batch-size":
			if i+1 < len(args) {
				if size, err := strconv.Atoi(args[i+1]); err == nil {
					config.BatchSize = size
				}
				i++
			}
		case "--threads":
			if i+1 < len(args) {
				if threads, err := strconv.Atoi(args[i+1]); err == nil {
					config.Threads = threads
				}
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if timeout, err := strconv.Atoi(args[i+1]); err == nil {
					config.StartupTimeout = time.Duration(timeout) * time.Second
				}
				i++
			}
		case "--cont-batching":
			config.ContBatching = true
		case "--debug":
			config.Debug = true
		}
	}
	return config
}

/*
使用示例:

manager := localmodel.GetManager()

// 启动嵌入服务
err := manager.StartEmbeddingService(
	"127.0.0.1:8080",
	localmodel.WithEmbeddingModel("Qwen3-Embedding-0.6B-Q4_K_M"),
	localmodel.WithDetached(true),
	localmodel.WithDebug(true),
	localmodel.WithContextSize(4096),
	localmodel.WithThreads(8),
)
if err != nil {
	log.Fatal(err)
}

// 启动聊天服务
err = manager.StartChatService(
	"127.0.0.1:8081",
	localmodel.WithChatModel("Qwen2.5-3B-Instruct-Q4_K_M"),
	localmodel.WithDetached(true),
	localmodel.WithDebug(false),
	localmodel.WithContextSize(8192),
	localmodel.WithThreads(16),
)
if err != nil {
	log.Fatal(err)
}

// 等待服务启动
err = manager.WaitForEmbeddingService("127.0.0.1:8080", 30.0)
if err != nil {
	log.Fatal(err)
}

err = manager.WaitForChatService("127.0.0.1:8081", 30.0)
if err != nil {
	log.Fatal(err)
}

// 便捷函数使用示例
err = localmodel.StartEmbedding("127.0.0.1:8080", localmodel.WithModel("Qwen3-Embedding-0.6B-Q4_K_M"))
if err != nil {
	log.Fatal(err)
}

err = localmodel.StartChat("127.0.0.1:8081", localmodel.WithModel("Qwen2.5-3B-Instruct-Q4_K_M"))
if err != nil {
	log.Fatal(err)
}
*/

// killDetachedService 直接使用进程ID终止服务进程
func (m *Manager) killDetachedService(service *ServiceInfo) error {
	if service.ProcessID <= 0 {
		return fmt.Errorf("invalid process ID: %d", service.ProcessID)
	}

	p, err := os.FindProcess(service.ProcessID)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %v", service.ProcessID, err)
	}
	return p.Kill()
}
