package localmodel

import (
	"context"
	"fmt"
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
	if modelName == "Qwen3-Embedding-0.6B-Q8_0" || modelName == "" {
		return consts.GetQwen3Embedding0_6BQ8_0ModelPath(), nil
	}

	// 其他模型使用原来的逻辑
	return GetModelPath(modelName)
}

// GetDefaultEmbeddingModelPath 获取默认嵌入模型路径
func (m *Manager) GetDefaultEmbeddingModelPath() string {
	return consts.GetQwen3Embedding0_6BQ8_0ModelPath()
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
			config.Model = "Qwen3-Embedding-0.6B-Q8_0" // 设置默认模型名称
		}
	}

	// 验证模型路径
	if err := ValidateModelPath(config.ModelPath); err != nil {
		return fmt.Errorf("model validation failed: %v", err)
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
	go m.startService(service, llamaServerPath)

	log.Infof("Starting embedding service: %s", serviceName)
	return nil
}

// startService 启动服务的内部方法
func (m *Manager) startService(service *ServiceInfo, llamaServerPath string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Service %s panic: %v", service.Name, r)
			m.updateServiceStatus(service.Name, StatusError, fmt.Sprintf("panic: %v", r))
		}
	}()

	// 构建启动参数
	args := m.buildArgs(service.Config)

	// 创建命令
	log.Infof("Starting command: %s %s", llamaServerPath, strings.Join(args, " "))
	cmd := exec.CommandContext(service.ctx, llamaServerPath, args...)

	// 设置输出
	if service.Config.Debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if !service.Config.Detached {
		// 如果不是分离模式且不是调试模式，仍然显示错误输出
		cmd.Stderr = os.Stderr
	}

	log.Infof("Starting command: %s %s", llamaServerPath, strings.Join(args, " "))

	// 启动进程
	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start service %s: %v", service.Name, err)
		m.updateServiceStatus(service.Name, StatusError, err.Error())
		return
	}

	// 更新服务信息
	m.mutex.Lock()
	service.Process = cmd
	service.Status = StatusRunning
	m.mutex.Unlock()

	log.Infof("Service %s started with PID: %d", service.Name, cmd.Process.Pid)

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
		"--parallel", fmt.Sprintf("%d", config.Parallelism),
		"--embedding", // 嵌入模式
		"--verbose-prompt",
	}

	// 添加额外参数
	args = append(args, config.Args...)

	return args
}

// waitForService 等待服务启动完成
func (m *Manager) waitForService(service *ServiceInfo) error {
	timeout := time.After(service.Config.StartupTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("service startup timeout after %v", service.Config.StartupTimeout)
		case <-ticker.C:
			// 检查端口是否可用
			conn, err := net.DialTimeout("tcp",
				fmt.Sprintf("%s:%d", service.Config.Host, service.Config.Port),
				2*time.Second)
			if err == nil {
				conn.Close()
				log.Infof("Service %s is ready on %s:%d", service.Name, service.Config.Host, service.Config.Port)
				return nil
			}

			// 检查进程是否还在运行
			if service.Process != nil && service.Process.ProcessState != nil {
				return fmt.Errorf("process exited during startup")
			}
		case <-service.ctx.Done():
			return fmt.Errorf("service startup cancelled")
		}
	}
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
	localmodel.WithEmbeddingModel("Qwen3-Embedding-0.6B-Q8_0"),
	localmodel.WithDetached(true),
	localmodel.WithDebug(true),
	localmodel.WithModelPath("/tmp/Qwen3-Embedding-0.6B-Q8_0.gguf"),
	localmodel.WithContextSize(4096),
	localmodel.WithParallelism(5),
)
if err != nil {
	die(err)
}
*/
