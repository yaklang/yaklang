package thirdparty_bin

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	// 确保只初始化一次
	initOnce sync.Once
	// 初始化是否成功
	initSuccess bool
	// 初始化错误
	initError error
	// DefaultManager 全局默认管理器实例
	DefaultManager *Manager
	// ErrPackageNotInitialized 包未初始化错误
	ErrPackageNotInitialized = utils.Error("thirdparty_bin package not initialized")
)

// init 包初始化函数，自动注册内置的二进制工具
func init() {
	initOnce.Do(func() {
		installDir := consts.GetDefaultLibsDir()
		downloadDir := consts.GetDefaultDownloadTempDir()
		var err error
		DefaultManager, err = NewManager(downloadDir, installDir)
		if err != nil {
			log.Errorf("create default binary manager failed: %v", err)
		}
		log.Debugf("Initializing thirdparty_bin package...")

		// 加载并注册内置的二进制工具
		if err := LoadAndRegisterBuiltinBinaries(); err != nil {
			log.Errorf("Failed to load builtin binaries during package initialization: %v", err)
			initError = err
			initSuccess = false
		} else {
			log.Debugf("Package thirdparty_bin initialized successfully")
			initSuccess = true
		}
	})
}

// EnsureInitialized 确保包已经正确初始化
func EnsureInitialized() error {
	if !initSuccess {
		if initError != nil {
			return initError
		}
		return ErrPackageNotInitialized
	}
	return nil
}

// IsInitialized 检查包是否已经初始化
func IsInitialized() bool {
	return initSuccess
}

// GetInitError 获取初始化错误（如果有的话）
func GetInitError() error {
	return initError
}

// ClearRegistry 清理注册表
func ClearRegistry() {
	if DefaultManager != nil {
		DefaultManager.mutex.Lock()
		DefaultManager.registry = make(map[string]*BinaryDescriptor)
		DefaultManager.mutex.Unlock()
	}
}

// GetRegisteredBinaries 获取已注册的二进制工具
func GetRegisteredBinaries() map[string]*BinaryDescriptor {
	if DefaultManager == nil {
		return nil
	}

	DefaultManager.mutex.RLock()
	defer DefaultManager.mutex.RUnlock()

	// 创建副本以避免并发问题
	result := make(map[string]*BinaryDescriptor)
	for name, descriptor := range DefaultManager.registry {
		result[name] = descriptor
	}

	return result
}

// ReinitializeBuiltinBinaries 重新初始化内置二进制工具
// 这个函数可以用于重新加载配置或处理初始化失败的情况
func ReinitializeBuiltinBinaries() error {
	log.Infof("Reinitializing builtin binary tools...")

	// 清理已注册的二进制工具（如果有的话）
	ClearRegistry()

	// 重新加载并注册
	if err := LoadAndRegisterBuiltinBinaries(); err != nil {
		log.Errorf("Failed to reinitialize builtin binaries: %v", err)
		initError = err
		initSuccess = false
		return err
	}

	log.Infof("Successfully reinitialized builtin binary tools")
	initError = nil
	initSuccess = true
	return nil
}

// GetPackageInfo 获取包信息
func GetPackageInfo() map[string]interface{} {
	info := map[string]interface{}{
		"package":              "thirdparty_bin",
		"initialized":          initSuccess,
		"initialization_error": nil,
	}

	if initError != nil {
		info["initialization_error"] = initError.Error()
	}

	// 获取注册的二进制工具数量
	if registeredBinaries := GetRegisteredBinaries(); registeredBinaries != nil {
		info["registered_binaries_count"] = len(registeredBinaries)

		names := make([]string, 0, len(registeredBinaries))
		for name := range registeredBinaries {
			names = append(names, name)
		}
		info["registered_binaries"] = names
	}

	// 获取内置工具信息
	if builtinNames, err := GetBuiltinBinaryNames(); err == nil {
		info["builtin_binaries_count"] = len(builtinNames)
		info["builtin_binaries"] = builtinNames
	}

	return info
}
