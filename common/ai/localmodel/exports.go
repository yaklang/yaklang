package localmodel

// 导出主要的类型和函数，方便外部使用

// 导出的类型
type (
	// Manager 本地模型管理器
	ModelManager = Manager

	// ServiceConfig 服务配置
	Config = ServiceConfig

	// ServiceInfo 服务信息
	Service = ServiceInfo

	// ServiceStatus 服务状态
	Status = ServiceStatus

	// ModelConfig 模型配置
	Model = ModelConfig
)

// 导出的常量
const (
	// 服务状态常量
	Stopped  = StatusStopped
	Starting = StatusStarting
	Running  = StatusRunning
	Stopping = StatusStopping
	Error    = StatusError
)

// 导出的函数
var (
	// GetManager 获取管理器单例
	GetManagerInstance = GetManager

	// NewManager 创建管理器 (已废弃)
	New = NewManager

	// 配置相关
	DefaultConfig  = DefaultServiceConfig
	GetModels      = GetSupportedModels
	FindModel      = FindModelConfig
	ValidateModel  = ValidateModelPath
	GetModel       = GetModelPath
	GetLlamaServer = GetLlamaServerPath
)

// 便捷方法

// GetManagerWithDefaults 获取管理器单例实例
func GetManagerWithDefaults() *Manager {
	return GetManager()
}

// NewManagerWithDefaults 创建带默认配置的管理器 (已废弃)
// Deprecated: Use GetManagerWithDefaults() instead
func NewManagerWithDefaults() *Manager {
	return GetManager()
}

// IsModelSupported 检查模型是否被支持
func IsModelSupported(modelName string) bool {
	_, err := FindModelConfig(modelName)
	return err == nil
}

// GetSupportedModelNames 获取支持的模型名称列表
func GetSupportedModelNames() []string {
	models := GetSupportedModels()
	names := make([]string, len(models))
	for i, model := range models {
		names[i] = model.Name
	}
	return names
}

// 便捷的服务启动函数

// StartEmbedding 启动嵌入服务（便捷函数）
func StartEmbedding(address string, options ...Option) error {
	manager := GetManager()
	return manager.StartEmbeddingService(address, options...)
}

// StartChat 启动聊天服务（便捷函数）
func StartChat(address string, options ...Option) error {
	manager := GetManager()
	return manager.StartChatService(address, options...)
}

// WaitForEmbedding 等待嵌入服务启动（便捷函数）
func WaitForEmbedding(address string, timeoutSeconds float64) error {
	manager := GetManager()
	return manager.WaitForEmbeddingService(address, timeoutSeconds)
}

// StopAllServices 停止所有服务（便捷函数）
func StopAllServices() error {
	manager := GetManager()
	return manager.StopAllServices()
}

// StopService 停止指定服务（便捷函数）
func StopService(serviceName string) error {
	manager := GetManager()
	return manager.StopService(serviceName)
}

// ListServices 列出所有服务（便捷函数）
func ListServices() []*ServiceInfo {
	manager := GetManager()
	return manager.ListServices()
}

// GetServiceStatus 获取服务状态（便捷函数）
func GetServiceStatus(serviceName string) (*ServiceInfo, error) {
	manager := GetManager()
	return manager.GetServiceStatus(serviceName)
}
