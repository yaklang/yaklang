package thirdparty_bin

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Manager 二进制文件管理器
type Manager struct {
	// 注册的二进制文件描述符
	registry map[string]*BinaryDescriptor
	// 安装器
	installer Installer
	// 读写锁
	mutex sync.RWMutex
}

// NewManager 创建新的二进制文件管理器
func NewManager(installDir string) (*Manager, error) {
	// 获取默认目录
	downloadDir, err := GetDefaultDownloadDir()
	if err != nil {
		return nil, utils.Errorf("get default download directory failed: %v", err)
	}

	// 确保目录存在
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, utils.Errorf("create download directory failed: %v", err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, utils.Errorf("create install directory failed: %v", err)
	}

	manager := &Manager{
		registry:  make(map[string]*BinaryDescriptor),
		installer: NewInstaller(installDir, downloadDir),
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
	installPath := m.installer.GetInstallPath(name)
	if err := EnsureExecutable(installPath); err != nil {
		log.Warnf("set executable permission failed: %v", err)
	}

	log.Infof("binary %s installed successfully", name)
	return nil
}

// Uninstall 卸载二进制文件
func (m *Manager) Uninstall(name string, installPath ...string) error {
	m.mutex.RLock()
	_, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return utils.Errorf("binary %s not registered", name)
	}

	return m.installer.Uninstall(name)
}

// List 列出所有注册的二进制文件
func (m *Manager) List() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.registry))
	for name := range m.registry {
		names = append(names, name)
	}

	return names
}

// GetBinary 获取二进制文件描述符
func (m *Manager) GetBinary(name string) (*BinaryDescriptor, error) {
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

// GetStatus 获取二进制文件状态
func (m *Manager) GetStatus(name string) (*BinaryStatus, error) {
	m.mutex.RLock()
	descriptor, exists := m.registry[name]
	m.mutex.RUnlock()

	if !exists {
		return nil, utils.Errorf("binary %s not registered", name)
	}

	installPath := m.installer.GetInstallPath(name)
	installed := m.installer.IsInstalled(name)

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

// Close 关闭管理器，清理资源
func (m *Manager) Close() error {
	// 获取下载目录并清理临时文件
	downloadDir, err := GetDefaultDownloadDir()
	if err != nil {
		return err
	}

	tempPattern := filepath.Join(downloadDir, "*.tmp")
	return CleanupTempFiles(tempPattern)
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
func Uninstall(name string, installPath ...string) error {
	if DefaultManager == nil {
		return utils.Error("default manager not initialized")
	}
	return DefaultManager.Uninstall(name, installPath...)
}

// List 使用默认管理器列出所有注册的二进制文件
func List() []string {
	if DefaultManager == nil {
		return []string{}
	}
	return DefaultManager.List()
}

// GetStatus 使用默认管理器获取二进制文件状态
func GetStatus(name string) (*BinaryStatus, error) {
	if DefaultManager == nil {
		return nil, utils.Error("default manager not initialized")
	}
	return DefaultManager.GetStatus(name)
}
