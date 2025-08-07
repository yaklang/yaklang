package localmodel

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
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

// ServiceInfo 服务信息
type ServiceInfo struct {
	Name       string         `json:"name"`
	Status     ServiceStatus  `json:"status"`
	Config     *ServiceConfig `json:"config"`
	Process    *exec.Cmd      `json:"-"`
	StartTime  time.Time      `json:"startTime"`
	LastError  string         `json:"lastError,omitempty"`
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// Manager 本地模型管理器
type Manager struct {
	mutex    sync.RWMutex
	services map[string]*ServiceInfo
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

// GetManager 获取管理器单例实例
func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			services: make(map[string]*ServiceInfo),
		}
	})
	return managerInstance
}

// NewManager 创建新的管理器实例 (已废弃，使用 GetManager 代替)
// Deprecated: Use GetManager() instead
func NewManager() *Manager {
	return GetManager()
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

// GetDefaultEmbeddingModelPath 获取默认嵌入模型路径
func (m *Manager) GetDefaultEmbeddingModelPath() string {
	return consts.GetQwen3Embedding0_6BQ4_0ModelPath()
}

// IsDefaultModelAvailable 检查默认模型是否可用
func (m *Manager) IsDefaultModelAvailable() bool {
	modelPath := m.GetDefaultEmbeddingModelPath()
	exists, err := utils.PathExists(modelPath)
	return err == nil && exists
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

// StartEmbeddingService 启动嵌入服务
func (m *Manager) StartEmbeddingService(address string, options ...Option) error {
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
			config.ModelPath = m.GetDefaultEmbeddingModelPath()
			config.Model = "Qwen3-Embedding-0.6B-Q4_K_M" // 设置默认模型名称
		}
	}

	// 验证模型路径
	if err := ValidateModelPath(config.ModelPath); err != nil {
		return fmt.Errorf("model validation failed: %v, for model: %v", err, config.ModelPath)
	}

	// 检查 llama-server 是否可用
	llamaServerPath, err := GetLlamaServerPath()
	if err != nil {
		return fmt.Errorf("llama-server not available: %v", err)
	}

	// 生成服务名称
	serviceName := fmt.Sprintf("embedding-%s-%d", host, port)

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
	go m.startService(service, llamaServerPath, done)
	log.Infof("Starting embedding service: %s", serviceName)
	return nil
}

// startService 启动服务的内部方法
func (m *Manager) startService(service *ServiceInfo, llamaServerPath string, done chan error) {
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

	// 构建启动参数
	args := m.buildArgs(service.Config)

	// 创建命令
	log.Infof("Starting command: %s %s", llamaServerPath, strings.Join(args, " "))
	cmd := exec.CommandContext(service.ctx, llamaServerPath, args...)

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

	// 等待启动完成或超时
	if err := m.waitForService(service); err != nil {
		log.Warnf("Service %s startup validation failed: %v", service.Name, err)
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
		"--embedding", // 嵌入模式
		"--verbose-prompt",
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

// GetServiceStatus 获取服务状态
func (m *Manager) GetServiceStatus(serviceName string) (*ServiceInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	service, exists := m.services[serviceName]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", serviceName)
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

	return services
}

/*
使用示例:

manager, err = localmodel.NewManager()
if err != nil {
	return
}

// StartEmbeddingService
err = manager.StartEmbeddingService(
	"127.0.0.1:11434",
	localmodel.WithEmbeddingModel("Qwen3-Embedding-0.6B-Q4_K_M"),
	localmodel.WithDetached(true),
	localmodel.WithDebug(true),
	localmodel.WithModelPath("/tmp/Qwen3-Embedding-0.6B-Q4_K_M.gguf"),
	localmodel.WithContextSize(4096),
	localmodel.WithParallelism(5),
)
if err != nil {
	die(err)
}
*/
