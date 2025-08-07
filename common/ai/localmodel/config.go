package localmodel

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

// ModelConfig 模型配置
type ModelConfig struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // embedding, llm, etc.
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadURL"`
	Description string `json:"description"`
	DefaultPort int32  `json:"defaultPort"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Host           string        `json:"host"`
	Port           int32         `json:"port"`
	Model          string        `json:"model"`
	ModelPath      string        `json:"modelPath"`
	ContextSize    int           `json:"contextSize"`
	ContBatching   bool          `json:"contBatching"` // 连续批处理
	BatchSize      int           `json:"batchSize"`    // 批处理大小
	Threads        int           `json:"threads"`      // 线程数
	Detached       bool          `json:"detached"`
	Debug          bool          `json:"debug"`
	StartupTimeout time.Duration `json:"startupTimeout"`
	Args           []string      `json:"args"`
}

// DefaultServiceConfig 返回默认服务配置
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		Host:           "127.0.0.1",
		Port:           8080,
		ContextSize:    4096,
		ContBatching:   true, // 默认开启连续批处理
		BatchSize:      1024, // 默认批处理大小
		Threads:        8,    // 默认线程数
		Detached:       false,
		Debug:          false,
		StartupTimeout: 30 * time.Second,
		Args:           []string{},
	}
}

// GetSupportedModels 获取支持的模型列表
func GetSupportedModels() []*ModelConfig {
	return []*ModelConfig{
		{
			Name:        "Qwen3-Embedding-0.6B-Q4_K_M",
			Type:        "embedding",
			FileName:    "Qwen3-Embedding-0.6B-Q4_K_M.gguf",
			DownloadURL: "https://oss-qn.yaklang.com/gguf/Qwen3-Embedding-0.6B-Q4_K_M.gguf",
			Description: "Qwen3 Embedding 0.6B Q4_K_M - 文本嵌入模型",
			DefaultPort: 8080,
		},
	}
}

// FindModelConfig 查找模型配置
func FindModelConfig(modelName string) (*ModelConfig, error) {
	models := GetSupportedModels()
	for _, model := range models {
		if model.Name == modelName {
			return model, nil
		}
	}
	return nil, fmt.Errorf("unsupported model: %s", modelName)
}

// ValidateModelPath 验证模型路径
func ValidateModelPath(modelPath string) error {
	if modelPath == "" {
		return fmt.Errorf("model path cannot be empty")
	}

	exists, err := utils.PathExists(modelPath)
	if err != nil {
		return fmt.Errorf("failed to check model path: %v", err)
	}

	if !exists {
		return fmt.Errorf("model file does not exist: %s", modelPath)
	}

	return nil
}

// GetModelPath 获取模型文件路径
func GetModelPath(modelName string) (string, error) {
	model, err := FindModelConfig(modelName)
	if err != nil {
		return "", err
	}

	modelPath := consts.GetAIModelFilePath(model.FileName)
	if modelPath == "" {
		return "", fmt.Errorf("model file not found: %s", model.FileName)
	}
	return modelPath, nil
}

// GetLlamaServerPath 获取 llama-server 路径
func GetLlamaServerPath() (string, error) {
	llamaServerPath := consts.GetLlamaServerPath()
	if llamaServerPath == "" {
		return "", fmt.Errorf("llama-server not installed")
	}
	return llamaServerPath, nil
}
