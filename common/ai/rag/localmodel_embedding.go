package rag

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/ai/localmodel"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// LocalModelEmbedding 基于本地模型的嵌入服务客户端
type LocalModelEmbedding struct {
	model     *localmodel.Model                // 模型配置
	address   string                           // 服务地址，如 "127.0.0.1:11435"
	embedding *embedding.OpenaiEmbeddingClient // 嵌入客户端
}

// 单例相关变量
var (
	embeddingServiceInstance *LocalModelEmbedding
	embeddingServiceOnce     sync.Once
	embeddingServiceMutex    sync.RWMutex
	embeddingServiceError    error
)

// NewLocalModelEmbedding 创建本地模型嵌入客户端
func NewLocalModelEmbedding(model *localmodel.Model, address string) *LocalModelEmbedding {
	if address == "" {
		address = "127.0.0.1:11435" // 默认端口
	}

	// 创建OpenAI兼容的嵌入客户端，配置使用本地服务地址
	embeddingClient := embedding.NewOpenaiEmbeddingClient(
		aispec.WithDomain(address),    // 设置服务地址
		aispec.WithNoHTTPS(true),      // 本地服务使用HTTP
		aispec.WithModel("embedding"), // 设置模型类型
	)

	return &LocalModelEmbedding{
		model:     model,
		address:   address,
		embedding: embeddingClient,
	}
}

// Embedding 实现 EmbeddingClient 接口，生成文本的嵌入向量
func (l *LocalModelEmbedding) Embedding(text string) ([]float32, error) {
	if l.embedding == nil {
		return nil, fmt.Errorf("embedding client not initialized")
	}

	log.Infof("generating embedding for text of length: %d", len(text))

	// 使用内部的嵌入客户端生成向量
	result, err := l.embedding.Embedding(text)
	if err != nil {
		log.Errorf("failed to generate embedding: %v", err)
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	log.Infof("successfully generated embedding with dimension: %d", len(result))
	return result, nil
}

var (
	ciMockMode = utils.NewAtomicBool()
)

// GetLocalEmbeddingService 获取本地嵌入服务单例
// 使用单例模式，确保只有一个 Embedding 服务实例
func GetLocalEmbeddingService() (*LocalModelEmbedding, error) {
	embeddingServiceOnce.Do(func() {
		embeddingServiceInstance, embeddingServiceError = startEmbeddingServiceInternal()
	})

	if embeddingServiceError != nil {
		return nil, embeddingServiceError
	}

	if embeddingServiceInstance != nil {
		err := utils.WaitConnect(embeddingServiceInstance.address, 30)
		if err != nil {
			return nil, err
		}
	}
	return embeddingServiceInstance, nil
}

// startEmbeddingServiceInternal 内部启动嵌入服务的函数
func startEmbeddingServiceInternal() (*LocalModelEmbedding, error) {
	address := "127.0.0.1:11435"

	// 首先检查服务是否已经在运行
	log.Infof("checking if embedding service is already running on %s", address)
	if testEmbeddingService(address, 3) {
		log.Infof("embedding service is already running and responding on %s, reusing existing service", address)

		// 获取默认模型配置
		model, err := localmodel.FindModelConfig("Qwen3-Embedding-0.6B-Q4_K_M")
		if err != nil {
			log.Warnf("failed to find model config, using default: %v", err)
			// 创建默认模型配置
			model = &localmodel.ModelConfig{
				Name:        "Qwen3-Embedding-0.6B-Q4_K_M",
				Type:        "embedding",
				Description: "Default Qwen3 Embedding Model",
				DefaultPort: 11435,
			}
		}

		// 直接返回使用现有服务的客户端
		return NewLocalModelEmbedding(model, address), nil
	}

	log.Infof("no running embedding service detected, starting new service on %s", address)

	// 获取管理器单例
	manager := localmodel.GetManager()

	modelName := "Qwen3-Embedding-0.6B-Q4_K_M"
	modelPath, err := localmodel.GetModelPath(modelName)
	if err != nil {
		log.Errorf("failed to get model path: %v", err)
		return nil, fmt.Errorf("failed to get model path: %v", err)
	}
	log.Infof("model path: %s", modelPath)

	// 启动嵌入服务，使用端口 11435，开启 Detach
	err = manager.StartEmbeddingService(
		address,
		localmodel.WithModel(modelName), // 使用默认嵌入模型
		localmodel.WithModelType("embedding"),
		localmodel.WithContextSize(4096),
		localmodel.WithContBatching(true),
		localmodel.WithBatchSize(1024),
		localmodel.WithThreads(8),
	)
	if err != nil {
		log.Errorf("failed to start embedding service: %v", err)

		// 如果启动失败，再次检查是否有其他进程已经占用了端口
		if isPortAvailable(address) {
			log.Infof("port %s is available, but service failed to start", address)
		} else {
			log.Infof("port %s is occupied, attempting to use existing service", address)
			// 使用 WaitConnect 等待服务完全启动
			manager := localmodel.GetManager()
			if waitErr := manager.WaitForEmbeddingService(address, 15.0); waitErr == nil {
				if testEmbeddingService(address, 15.0) {
					log.Infof("found working service on %s after startup failure", address)
					// 获取默认模型配置并返回
					model, _ := localmodel.FindModelConfig("Qwen3-Embedding-0.6B-Q4_K_M")
					if model == nil {
						model = &localmodel.ModelConfig{
							Name:        "Qwen3-Embedding-0.6B-Q4_K_M",
							Type:        "embedding",
							Description: "Default Qwen3 Embedding Model",
							DefaultPort: 11435,
						}
					}
					return NewLocalModelEmbedding(model, address), nil
				}
			}
		}

		return nil, fmt.Errorf("failed to start local embedding service: %v", err)
	}

	log.Infof("local embedding service started successfully at %s", address)

	// 获取默认模型配置
	model, err := localmodel.FindModelConfig("Qwen3-Embedding-0.6B-Q4_K_M")
	if err != nil {
		log.Warnf("failed to find model config, using default: %v", err)
		// 创建默认模型配置
		model = &localmodel.ModelConfig{
			Name:        "Qwen3-Embedding-0.6B-Q4_K_M",
			Type:        "embedding",
			Description: "Default Qwen3 Embedding Model",
			DefaultPort: 11435,
		}
	}

	// 创建并返回 LocalModelEmbedding 实例
	return NewLocalModelEmbedding(model, address), nil
}

// StartLocalEmbeddingService 启动本地嵌入服务 (已废弃，使用 GetLocalEmbeddingService 代替)
// Deprecated: Use GetLocalEmbeddingService() instead
func StartLocalEmbeddingService() (*LocalModelEmbedding, error) {
	return GetLocalEmbeddingService()
}

// GetAddress 获取服务地址
func (l *LocalModelEmbedding) GetAddress() string {
	return l.address
}

// GetModel 获取模型配置
func (l *LocalModelEmbedding) GetModel() *localmodel.Model {
	return l.model
}

// IsServiceRunning 检查嵌入服务是否正在运行
func IsServiceRunning() bool {
	embeddingServiceMutex.RLock()
	defer embeddingServiceMutex.RUnlock()
	return embeddingServiceInstance != nil && embeddingServiceError == nil
}

// GetServiceStatus 获取服务状态信息
func GetServiceStatus() (bool, string, error) {
	embeddingServiceMutex.RLock()
	defer embeddingServiceMutex.RUnlock()

	if embeddingServiceInstance == nil {
		if embeddingServiceError != nil {
			return false, "Service failed to start", embeddingServiceError
		}
		return false, "Service not initialized", nil
	}

	return true, "Service running at " + embeddingServiceInstance.address, nil
}

// ResetService 重置服务单例（仅用于测试或特殊情况）
func ResetService() {
	embeddingServiceMutex.Lock()
	defer embeddingServiceMutex.Unlock()

	log.Infof("resetting embedding service singleton")
	embeddingServiceInstance = nil
	embeddingServiceError = nil
	embeddingServiceOnce = sync.Once{}
}

// isPortAvailable 检查端口是否可用
func isPortAvailable(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// testEmbeddingService 测试嵌入服务是否正常工作
func testEmbeddingService(address string, timeoutSeconds float64) bool {
	// 首先使用 WaitConnect 等待端口可用
	manager := localmodel.GetManager()
	err := manager.WaitForEmbeddingService(address, timeoutSeconds) // 等待15秒
	if err != nil {
		log.Infof("embedding service not available on %s: %v", address, err)
		return false
	}

	// 创建临时客户端测试服务
	testClient := embedding.NewOpenaiEmbeddingClient(
		aispec.WithDomain(address),
		aispec.WithNoHTTPS(true),
		aispec.WithModel("embedding"),
	)

	// 测试服务是否能正常生成嵌入
	_, err = testClient.Embedding("test")
	if err != nil {
		log.Infof("embedding service test failed on %s: %v", address, err)
		return false
	}

	log.Infof("embedding service is working correctly on %s", address)
	return true
}

// CleanupRedundantServices 清理多余的llama-server进程
// 只保留一个正常工作的服务
func CleanupRedundantServices() error {
	log.Infof("checking for redundant llama-server processes")

	address := "127.0.0.1:11435"

	// 如果当前有正常工作的服务，就不需要清理
	if testEmbeddingService(address, 10) {
		log.Infof("found working embedding service, no cleanup needed")
		return nil
	}

	log.Warnf("no working embedding service found, redundant processes may exist")
	log.Infof("please manually check and kill redundant llama-server processes using: ps aux | grep llama-server")

	return nil
}

// Embedding 全局嵌入函数，使用单例服务生成文本的嵌入向量
// 如果服务未启动，会自动启动；如果无法启动，则报错
func Embedding(text string) ([]float32, error) {
	service, err := GetLocalEmbeddingService()
	if err != nil {
		log.Errorf("failed to get embedding service: %v", err)
		return nil, fmt.Errorf("failed to get embedding service: %v", err)
	}

	return service.Embedding(text)
}
