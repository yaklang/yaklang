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
