package thirdparty_bin

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ProcessCallback 进程回调函数类型
type ProcessCallback func(reader io.Reader)

// RunningProcess 运行中的进程信息
type RunningProcess struct {
	Name     string
	Cmd      *exec.Cmd
	Cancel   context.CancelFunc
	Callback ProcessCallback
}

// Manager 二进制文件管理器
type Manager struct {
	// 注册的二进制文件描述符
	registry map[string]*BinaryDescriptor
	// 安装器
	installer Installer
	// 运行中的进程
	runningProcesses map[string]*RunningProcess
	// 读写锁
	mutex sync.RWMutex
}

// NewManager 创建新的二进制文件管理器
func NewManager(downloadDir, installDir string) (*Manager, error) {
	// 确保目录存在
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, utils.Errorf("create download directory failed: %v", err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, utils.Errorf("create install directory failed: %v", err)
	}
	manager := &Manager{
		registry:         make(map[string]*BinaryDescriptor),
		installer:        NewInstaller(installDir, downloadDir),
		runningProcesses: make(map[string]*RunningProcess),
	}
	return manager, nil
}

// Register 注册二进制文件
func (m *Manager) Register(descriptor *BinaryDescriptor) error {
	if descriptor == nil {
		return utils.Error("descriptor cannot be nil")
	}

	if descriptor.Name == "" {
		return utils.Error("binary name cannot be empty")
	}

	if len(descriptor.DownloadInfoMap) == 0 {
		return utils.Error("download URLs cannot be empty")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.registry[descriptor.Name] = descriptor

	return nil
}

// Unregister 取消注册二进制文件
func (m *Manager) Unregister(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.registry[name]; !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	delete(m.registry, name)

	return nil
}

// Install 安装二进制文件
func (m *Manager) Install(name string, options *InstallOptions) error {
	if options == nil {
		options = &InstallOptions{}
	}

	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	// 设置默认上下文
	if options.Context == nil {
		options.Context = context.Background()
	}

	// 安装依赖
	if err := m.InstallDependencies(name, options); err != nil {
		return utils.Errorf("install dependencies failed: %v", err)
	}

	// 使用installer进行安装（包含下载）
	if err := m.installer.Install(descriptor, options); err != nil {
		return utils.Errorf("install failed: %v", err)
	}

	// 确保文件具有执行权限
	installPath := m.installer.GetTargetPath(descriptor)
	if err := EnsureExecutable(installPath); err != nil {
		log.Warnf("set executable permission failed: %v", err)
	}

	log.Infof("binary %s installed successfully", name)
	return nil
}

// Uninstall 卸载二进制文件
func (m *Manager) Uninstall(name string) error {
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	return m.installer.Uninstall(descriptor)
}

// ListRegistered 列出所有注册的二进制文件
func (m *Manager) ListRegistered() []*BinaryDescriptor {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	descriptors := make([]*BinaryDescriptor, 0, len(m.registry))
	for _, descriptor := range m.registry {
		descriptors = append(descriptors, descriptor)
	}

	return descriptors
}

// List 列出所有注册的二进制文件
func (m *Manager) List() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.registry))
	for name := range m.registry {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// GetBinary 获取二进制文件描述符
func (m *Manager) GetBinaryDescriptor(name string) (*BinaryDescriptor, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	descriptor, exists := m.registry[name]
	if !exists {
		return nil, utils.Errorf("binary %s not registered", name)
	}

	// 返回副本以避免并发修改
	descriptorCopy := *descriptor
	return &descriptorCopy, nil
}

// GetBinaryPath 获取二进制文件的安装路径
func (m *Manager) GetBinaryPath(name string) (string, error) {
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return "", utils.Errorf("binary %s not registered", name)
	}

	// 检查是否已安装
	if !m.installer.IsInstalled(descriptor) {
		return "", utils.Errorf("binary %s not installed", name)
	}

	// 返回安装路径
	installPath := m.installer.GetInstallPath(descriptor)
	return installPath, nil
}

// GetStatus 获取二进制文件状态
func (m *Manager) GetStatus(name string) (*BinaryStatus, error) {
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return nil, utils.Errorf("binary %s not registered", name)
	}

	installPath := m.installer.GetInstallPath(descriptor)
	installed := m.installer.IsInstalled(descriptor)

	status := &BinaryStatus{
		Name:             name,
		Installed:        installed,
		AvailableVersion: descriptor.Version,
		NeedsUpdate:      false, // 暂时不支持版本比较
	}

	if installed {
		status.InstallPath = installPath
		status.InstalledVersion = descriptor.Version // 暂时假设安装的就是当前版本
	}

	return status, nil
}

// GetAllStatus 获取所有二进制文件的状态
func (m *Manager) GetAllStatus() ([]*BinaryStatus, error) {
	names := m.List()
	statuses := make([]*BinaryStatus, 0, len(names))

	for _, name := range names {
		status, err := m.GetStatus(name)
		if err != nil {
			log.Warnf("get status for %s failed: %v", name, err)
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GetBinaryNamesByTags 根据tags获取二进制文件名称列表
// tags参数为需要匹配的标签列表，binary必须包含所有指定的标签才会被返回
func (m *Manager) GetBinaryNamesByTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []string
	for name, descriptor := range m.registry {
		if m.containsAllTags(descriptor.Tags, tags) {
			result = append(result, name)
		}
	}

	sort.Strings(result)
	return result
}

// GetBinaryNamesByAnyTag 根据tags获取二进制文件名称列表
// tags参数为需要匹配的标签列表，binary只要包含任意一个指定的标签就会被返回
func (m *Manager) GetBinaryNamesByAnyTag(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []string
	for name, descriptor := range m.registry {
		if m.containsAnyTag(descriptor.Tags, tags) {
			result = append(result, name)
		}
	}

	sort.Strings(result)
	return result
}

// containsAllTags 检查binaryTags是否包含requiredTags中的所有标签
func (m *Manager) containsAllTags(binaryTags, requiredTags []string) bool {
	tagSet := make(map[string]bool)
	for _, tag := range binaryTags {
		tagSet[tag] = true
	}

	for _, requiredTag := range requiredTags {
		if !tagSet[requiredTag] {
			return false
		}
	}

	return true
}

// containsAnyTag 检查binaryTags是否包含requiredTags中的任意一个标签
func (m *Manager) containsAnyTag(binaryTags, requiredTags []string) bool {
	tagSet := make(map[string]bool)
	for _, tag := range binaryTags {
		tagSet[tag] = true
	}

	for _, requiredTag := range requiredTags {
		if tagSet[requiredTag] {
			return true
		}
	}

	return false
}

// InstallDependencies 安装依赖
func (m *Manager) InstallDependencies(name string, options *InstallOptions) error {
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	// 递归安装依赖
	for _, dep := range descriptor.Dependencies {
		log.Infof("installing dependency: %s", dep)
		if err := m.Install(dep, options); err != nil {
			return utils.Errorf("install dependency %s failed: %v", dep, err)
		}
	}

	return nil
}

// Start 启动二进制程序
func (m *Manager) Start(ctx context.Context, name string, args []string, callback ProcessCallback) error {
	// 检查是否已注册
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	// 检查是否已安装
	if !m.installer.IsInstalled(descriptor) {
		return utils.Errorf("binary %s not installed", name)
	}

	// 检查是否已在运行
	m.mutex.Lock()
	if _, running := m.runningProcesses[name]; running {
		m.mutex.Unlock()
		return utils.Errorf("binary %s is already running", name)
	}
	m.mutex.Unlock()

	// 获取可执行文件路径
	execPath := m.installer.GetInstallPath(descriptor)

	// 创建带取消功能的上下文
	processCtx, cancel := context.WithCancel(ctx)

	// 创建命令
	cmd := exec.CommandContext(processCtx, execPath, args...)

	// 设置输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return utils.Errorf("create stdout pipe failed: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return utils.Errorf("create stderr pipe failed: %v", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		cancel()
		return utils.Errorf("start process failed: %v", err)
	}

	// 创建运行进程信息
	runningProcess := &RunningProcess{
		Name:     name,
		Cmd:      cmd,
		Cancel:   cancel,
		Callback: callback,
	}

	// 添加到运行列表
	m.mutex.Lock()
	m.runningProcesses[name] = runningProcess
	m.mutex.Unlock()

	// 启动输出处理goroutine
	go func() {
		defer func() {
			// 进程结束时清理
			m.mutex.Lock()
			delete(m.runningProcesses, name)
			m.mutex.Unlock()
			cancel()
		}()

		// 合并stdout和stderr
		if callback != nil {
			// 创建多路复用Reader
			multiReader := io.MultiReader(stdout, stderr)
			callback(multiReader)
		}

		// 等待进程结束
		if err := cmd.Wait(); err != nil {
			log.Warnf("process %s exited with error: %v", name, err)
		} else {
			log.Infof("process %s exited successfully", name)
		}
	}()

	log.Infof("binary %s started with PID %d", name, cmd.Process.Pid)
	return nil
}

// Stop 停止二进制程序
func (m *Manager) Stop(name string) error {
	m.mutex.Lock()
	runningProcess, exists := m.runningProcesses[name]
	if !exists {
		m.mutex.Unlock()
		return utils.Errorf("binary %s is not running", name)
	}
	delete(m.runningProcesses, name)
	m.mutex.Unlock()

	// 取消上下文，这会发送SIGTERM信号
	runningProcess.Cancel()

	// 等待进程结束
	if runningProcess.Cmd.Process != nil {
		if err := runningProcess.Cmd.Wait(); err != nil {
			log.Warnf("process %s stopped with error: %v", name, err)
		}
	}

	log.Infof("binary %s stopped", name)
	return nil
}

// StopAll 停止所有运行中的二进制程序
func (m *Manager) StopAll() {
	m.mutex.RLock()
	runningNames := make([]string, 0, len(m.runningProcesses))
	for name := range m.runningProcesses {
		runningNames = append(runningNames, name)
	}
	m.mutex.RUnlock()

	for _, name := range runningNames {
		if err := m.Stop(name); err != nil {
			log.Warnf("stop %s failed: %v", name, err)
		}
	}
}

// GetRunningBinaries 获取所有运行中的二进制程序名称
func (m *Manager) GetRunningBinaries() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.runningProcesses))
	for name := range m.runningProcesses {
		names = append(names, name)
	}

	return names
}

// IsRunning 检查指定的二进制程序是否正在运行
func (m *Manager) IsRunning(name string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.runningProcesses[name]
	return exists
}

// GetDownloadInfo 获取下载信息
func (m *Manager) GetDownloadInfo(name string) (*DownloadInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	descriptor, exists := m.registry[name]
	if !exists {
		return nil, utils.Errorf("binary %s not registered", name)
	}

	return m.installer.GetDownloadInfo(descriptor)
}

// GetRunningProcess 获取运行中的进程信息
func (m *Manager) GetRunningProcess(name string) (*RunningProcess, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	process, exists := m.runningProcesses[name]
	if !exists {
		return nil, utils.Errorf("binary %s is not running", name)
	}

	// 返回副本以避免并发修改
	processCopy := *process
	return &processCopy, nil
}

// Close 关闭管理器，清理资源
func (m *Manager) Close() error {
	// 停止所有运行中的进程
	m.StopAll()
	return nil
}

// Register 注册二进制文件到默认管理器
func Register(descriptor *BinaryDescriptor) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Register(descriptor)
}

// Install 使用默认管理器安装二进制文件
func Install(name string, options *InstallOptions) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Install(name, options)
}

// Uninstall 使用默认管理器卸载二进制文件
func Uninstall(name string) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Uninstall(name)
}

// ListRegisteredNames 使用默认管理器列出所有注册的二进制文件
func ListRegisteredNames() []string {
	if DefaultManager == nil {
		return []string{}
	}
	return DefaultManager.List()
}

func ListRegistered() []*BinaryDescriptor {
	if DefaultManager == nil {
		return []*BinaryDescriptor{}
	}
	return DefaultManager.ListRegistered()
}

// GetStatus 使用默认管理器获取二进制文件状态
func GetStatus(name string) (*BinaryStatus, error) {
	if DefaultManager == nil {
		return nil, utils.Error("default manager not initialized")
	}
	return DefaultManager.GetStatus(name)
}

// Start 使用默认管理器启动二进制程序
func Start(ctx context.Context, name string, args []string, callback ProcessCallback) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Start(ctx, name, args, callback)
}

// Stop 使用默认管理器停止二进制程序
func Stop(name string) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Stop(name)
}

// StopAll 使用默认管理器停止所有运行中的二进制程序
func StopAll() {
	if DefaultManager != nil {
		DefaultManager.StopAll()
	}
}

// GetRunningBinaries 使用默认管理器获取所有运行中的二进制程序名称
func GetRunningBinaries() []string {
	if DefaultManager == nil {
		return []string{}
	}
	return DefaultManager.GetRunningBinaries()
}

// IsRunning 使用默认管理器检查指定的二进制程序是否正在运行
func IsRunning(name string) bool {
	if DefaultManager == nil {
		return false
	}
	return DefaultManager.IsRunning(name)
}

// GetRunningProcess 使用默认管理器获取运行中的进程信息
func GetRunningProcess(name string) (*RunningProcess, error) {
	if DefaultManager == nil {
		return nil, utils.Error("default manager not initialized")
	}
	return DefaultManager.GetRunningProcess(name)
}

// GetBinaryPath 使用默认管理器获取二进制文件的安装路径
func GetBinaryPath(name string) (string, error) {
	if DefaultManager == nil {
		return "", utils.Error("default manager not initialized")
	}
	return DefaultManager.GetBinaryPath(name)
}

// GetDownloadInfo 使用默认管理器获取下载信息
func GetDownloadInfo(name string) (*DownloadInfo, error) {
	if DefaultManager == nil {
		return nil, utils.Error("default manager not initialized")
	}
	return DefaultManager.GetDownloadInfo(name)
}

func GetAllStatus() ([]*BinaryStatus, error) {
	if DefaultManager == nil {
		return nil, utils.Error("default manager not initialized")
	}
	return DefaultManager.GetAllStatus()
}

// GetBinaryNamesByTags 使用默认管理器根据tags获取二进制文件名称列表
// tags参数为需要匹配的标签列表，binary必须包含所有指定的标签才会被返回
func GetBinaryNamesByTags(tags []string) []string {
	if DefaultManager == nil {
		return []string{}
	}
	return DefaultManager.GetBinaryNamesByTags(tags)
}

// GetBinaryNamesByAnyTag 使用默认管理器根据tags获取二进制文件名称列表
// tags参数为需要匹配的标签列表，binary只要包含任意一个指定的标签就会被返回
func GetBinaryNamesByAnyTag(tags []string) []string {
	if DefaultManager == nil {
		return []string{}
	}
	return DefaultManager.GetBinaryNamesByAnyTag(tags)
}
