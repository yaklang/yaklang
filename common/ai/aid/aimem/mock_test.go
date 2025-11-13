package aimem

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// MockEmbeddingClient mock的embedding客户端，用于测试
type MockEmbeddingClient struct {
	embeddingData map[string][]float32
}

// NewMockEmbeddingClient 创建mock embedding客户端（使用空的测试数据）
func NewMockEmbeddingClient() (*MockEmbeddingClient, error) {
	// 创建一个空的 embedding 数据映射
	// 对于未知文本，会自动生成默认向量
	embeddingData := make(map[string][]float32)

	log.Infof("created mock embedding client with %d entries", len(embeddingData))
	return &MockEmbeddingClient{
		embeddingData: embeddingData,
	}, nil
}

// NewMockEmbeddingClientFromJSON 创建mock embedding客户端（从JSON数据加载）
func NewMockEmbeddingClientFromJSON(jsonData []byte) (*MockEmbeddingClient, error) {
	var embeddingData map[string][]float32
	if len(jsonData) > 0 {
		if err := json.Unmarshal(jsonData, &embeddingData); err != nil {
			log.Warnf("failed to unmarshal mock embedding data: %v, using empty data", err)
			embeddingData = make(map[string][]float32)
		}
	} else {
		embeddingData = make(map[string][]float32)
	}

	log.Infof("loaded %d mock embedding entries from JSON", len(embeddingData))
	return &MockEmbeddingClient{
		embeddingData: embeddingData,
	}, nil
}

// NewMockEmbeddingClientWithData 创建带有自定义数据的mock embedding客户端
func NewMockEmbeddingClientWithData(data map[string][]float32) *MockEmbeddingClient {
	if data == nil {
		data = make(map[string][]float32)
	}
	return &MockEmbeddingClient{
		embeddingData: data,
	}
}

// Embedding 实现EmbeddingClient接口
func (m *MockEmbeddingClient) Embedding(text string) ([]float32, error) {
	if embedding, ok := m.embeddingData[text]; ok {
		return embedding, nil
	}

	// 如果找不到，返回一个默认的向量
	log.Debugf("text not found in mock data, returning default vector: %s", utils.ShrinkString(text, 50))
	return generateDefaultVector(text), nil
}

// EmbeddingRaw 实现EmbeddingClient接口
func (m *MockEmbeddingClient) EmbeddingRaw(text string) ([][]float32, error) {
	vec, err := m.Embedding(text)
	if err != nil {
		return nil, err
	}
	if vec == nil {
		return nil, nil
	}
	return [][]float32{vec}, nil
}

// generateDefaultVector 为未知文本生成一个简单的默认向量
func generateDefaultVector(text string) []float32 {
	// 基于文本长度和hash生成一个简单的向量
	hash := utils.CalcMd5(text)
	vec := make([]float32, 768) // 假设维度为768

	for i := 0; i < 768; i++ {
		// 使用hash的字节来生成向量值
		vec[i] = float32(hash[i%len(hash)]) / 255.0
	}

	return vec
}
