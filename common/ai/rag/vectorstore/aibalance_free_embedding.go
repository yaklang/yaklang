package vectorstore

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/asynchelper"
)

// AIBalanceFreeEmbedding 基于 AIBalance 免费服务的嵌入服务客户端
type AIBalanceFreeEmbedding struct {
	embedding *embedding.OpenaiEmbeddingClient // 嵌入客户端
	available bool                             // 服务是否可用
	modelName string                           // 归一化的模型名称
}

// 单例相关变量
var (
	aibalanceFreeInstance   *AIBalanceFreeEmbedding
	aibalanceFreeOnce       sync.Once
	aibalanceFreeMutex      sync.RWMutex
	aibalanceFreeError      error
	aibalanceFreeAvailable  bool
	aibalanceFreeCheckOnce  sync.Once
	aibalanceFreeCheckError error
	aibalanceFreeCheckDone  bool
)

const (
	aibalanceBaseURL         = "https://aibalance.yaklang.com/v1"
	aibalanceDomain          = "aibalance.yaklang.com"
	aibalanceFreeModel       = "embedding-free"
	aibalanceFakAPIKey       = "sk-free-embedding-service" // 免费服务使用假的 API Key
	normalizedModelName      = "Qwen3-Embedding-0.6B"      // 归一化的模型名称
	normalizedModelDimension = 1024                        // 模型维度
)

// NewAIBalanceFreeEmbedder 创建 AIBalance 免费嵌入客户端单例
// 该函数使用 sync.Once 确保只创建一次实例，并在创建时检测服务可用性
func NewAIBalanceFreeEmbedder() (*AIBalanceFreeEmbedding, error) {
	aibalanceFreeOnce.Do(func() {
		log.Infof("initializing aibalance free embedding service singleton")

		// 创建 OpenAI 兼容的嵌入客户端
		embeddingClient := embedding.NewOpenaiEmbeddingClient(
			aispec.WithBaseURL(aibalanceBaseURL),  // 设置服务基础 URL (包含 /v1)
			aispec.WithModel(aibalanceFreeModel),  // 设置模型为 embedding-free
			aispec.WithAPIKey(aibalanceFakAPIKey), // 设置假的 API Key（免费服务不验证）
			aispec.WithTimeout(30),                // 设置 30 秒超时
		)

		if embeddingClient == nil {
			aibalanceFreeError = fmt.Errorf("failed to create embedding client")
			log.Errorf("failed to create aibalance free embedding client")
			return
		}

		instance := &AIBalanceFreeEmbedding{
			embedding: embeddingClient,
			available: false,               // 初始状态为不可用，需要检测
			modelName: normalizedModelName, // 使用归一化的模型名称
		}

		// 检测服务可用性（只检测一次）
		aibalanceFreeCheckOnce.Do(func() {
			log.Infof("checking aibalance free embedding service availability")
			aibalanceFreeCheckDone = false

			// 使用简单的测试文本检测服务
			testText := "test"
			_, err := embeddingClient.Embedding(testText)
			if err != nil {
				aibalanceFreeCheckError = fmt.Errorf("aibalance free embedding service is not available: %w", err)
				aibalanceFreeAvailable = false
				log.Errorf("aibalance free embedding service check failed: %v", err)
			} else {
				aibalanceFreeAvailable = true
				instance.available = true
				log.Infof("aibalance free embedding service is available and working")
			}
			aibalanceFreeCheckDone = true
		})

		// 如果检测失败，设置错误
		if !aibalanceFreeAvailable {
			aibalanceFreeError = aibalanceFreeCheckError
			return
		}

		aibalanceFreeInstance = instance
		log.Infof("aibalance free embedding service singleton initialized successfully")
	})

	if aibalanceFreeError != nil {
		return nil, aibalanceFreeError
	}

	return aibalanceFreeInstance, nil
}

// Embedding 实现 EmbeddingClient 接口，生成文本的嵌入向量
func (a *AIBalanceFreeEmbedding) Embedding(text string) ([]float32, error) {
	al := asynchelper.NewAsyncPerformanceHelper("aibalance free embedding")
	defer al.Close()

	if a.embedding == nil {
		return nil, fmt.Errorf("embedding client not initialized")
	}

	if !a.available {
		return nil, fmt.Errorf("aibalance free embedding service is not available")
	}

	// 使用内部的嵌入客户端生成向量
	result, err := a.embedding.Embedding(text)
	if err != nil {
		log.Errorf("failed to generate embedding from aibalance free service: %v", err)
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return result, nil
}

// EmbeddingRaw 实现 EmbeddingClient 接口，返回原始的 embedding 结果
func (a *AIBalanceFreeEmbedding) EmbeddingRaw(text string) ([][]float32, error) {
	al := asynchelper.NewAsyncPerformanceHelper("aibalance free embedding raw")
	defer al.Close()

	if a.embedding == nil {
		return nil, fmt.Errorf("embedding client not initialized")
	}

	if !a.available {
		return nil, fmt.Errorf("aibalance free embedding service is not available")
	}

	// 使用内部的嵌入客户端生成向量（可能返回多个向量）
	result, err := a.embedding.EmbeddingRaw(text)
	if err != nil {
		log.Errorf("failed to generate embedding from aibalance free service: %v", err)
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return result, nil
}

// IsAvailable 返回服务是否可用
func (a *AIBalanceFreeEmbedding) IsAvailable() bool {
	return a.available
}

// GetServiceInfo 返回服务信息
func (a *AIBalanceFreeEmbedding) GetServiceInfo() (domain string, model string, available bool) {
	return aibalanceDomain, aibalanceFreeModel, a.available
}

// GetModelName 返回归一化的模型名称
// 这个方法返回的名称应该与本地 Qwen3-Embedding-0.6B 模型保持一致
func (a *AIBalanceFreeEmbedding) GetModelName() string {
	return a.modelName
}

// GetModelDimension 返回模型的嵌入向量维度
func (a *AIBalanceFreeEmbedding) GetModelDimension() int {
	return normalizedModelDimension
}

// GetAIBalanceFreeEmbeddingService 获取 AIBalance 免费嵌入服务单例
// 这是一个便捷函数，直接返回单例实例
func GetAIBalanceFreeEmbeddingService() (*AIBalanceFreeEmbedding, error) {
	return NewAIBalanceFreeEmbedder()
}

// IsAIBalanceFreeServiceAvailable 检查 AIBalance 免费服务是否可用
// 如果服务尚未初始化，会先尝试初始化
func IsAIBalanceFreeServiceAvailable() bool {
	aibalanceFreeMutex.RLock()
	if aibalanceFreeCheckDone {
		available := aibalanceFreeAvailable
		aibalanceFreeMutex.RUnlock()
		return available
	}
	aibalanceFreeMutex.RUnlock()

	// 尝试初始化服务
	_, err := NewAIBalanceFreeEmbedder()
	return err == nil
}

// ResetAIBalanceFreeService 重置服务单例（仅用于测试或特殊情况）
func ResetAIBalanceFreeService() {
	aibalanceFreeMutex.Lock()
	defer aibalanceFreeMutex.Unlock()

	log.Infof("resetting aibalance free embedding service singleton")
	aibalanceFreeInstance = nil
	aibalanceFreeError = nil
	aibalanceFreeAvailable = false
	aibalanceFreeCheckError = nil
	aibalanceFreeCheckDone = false
	aibalanceFreeOnce = sync.Once{}
	aibalanceFreeCheckOnce = sync.Once{}
}

// AIBalanceFreeEmbeddingFunc 全局嵌入函数，使用 AIBalance 免费服务生成文本的嵌入向量
func AIBalanceFreeEmbeddingFunc(text string) ([]float32, error) {
	service, err := GetAIBalanceFreeEmbeddingService()
	if err != nil {
		log.Errorf("failed to get aibalance free embedding service: %v", err)
		return nil, fmt.Errorf("failed to get aibalance free embedding service: %v", err)
	}

	return service.Embedding(text)
}

// NormalizeEmbeddingModelName 归一化 embedding 模型名称
// 将各种变体的模型名称统一为标准名称
// 例如：
// - "Qwen3-Embedding-0.6B-Q4_K_M" -> "Qwen3-Embedding-0.6B"
// - "Qwen3-Embedding-0.6B" -> "Qwen3-Embedding-0.6B"
// - "embedding-free" -> "Qwen3-Embedding-0.6B"
func NormalizeEmbeddingModelName(modelName string) string {
	// 归一化逻辑：
	// 1. embedding-free 是 AIBalance 免费服务，归一化为 Qwen3-Embedding-0.6B
	// 2. 所有 Qwen3-Embedding-0.6B 的变体（Q4_K_M等）都归一化为基础名称

	switch {
	case modelName == "embedding-free":
		return normalizedModelName
	case modelName == "Qwen3-Embedding-0.6B-Q4_K_M":
		return normalizedModelName
	case modelName == "Qwen3-Embedding-0.6B":
		return normalizedModelName
	default:
		// 如果不在已知列表中，返回原始名称
		return modelName
	}
}

// IsCompatibleEmbeddingModel 检查两个模型名称是否兼容
// 兼容的模型具有相同的嵌入维度和归一化名称
func IsCompatibleEmbeddingModel(modelName1, modelName2 string) bool {
	return NormalizeEmbeddingModelName(modelName1) == NormalizeEmbeddingModelName(modelName2)
}

// Verify interface implementation at compile time
var _ EmbeddingClient = (*AIBalanceFreeEmbedding)(nil)
