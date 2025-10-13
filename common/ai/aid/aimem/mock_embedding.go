package aimem

import (
	_ "embed"
	"encoding/json"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed testdata/mock_embedding_data.json
var mockEmbeddingDataJSON []byte

// MockEmbeddingClient mock的embedding客户端，用于测试
type MockEmbeddingClient struct {
	embeddingData map[string][]float32
}

// NewMockEmbeddingClient 创建mock embedding客户端
func NewMockEmbeddingClient() (*MockEmbeddingClient, error) {
	var embeddingData map[string][]float32
	if err := json.Unmarshal(mockEmbeddingDataJSON, &embeddingData); err != nil {
		return nil, utils.Errorf("failed to unmarshal mock embedding data: %v", err)
	}

	log.Infof("loaded %d mock embedding entries", len(embeddingData))
	return &MockEmbeddingClient{
		embeddingData: embeddingData,
	}, nil
}

// Embedding 实现EmbeddingClient接口
func (m *MockEmbeddingClient) Embedding(text string) ([]float32, error) {
	if embedding, ok := m.embeddingData[text]; ok {
		return embedding, nil
	}

	// 如果找不到，返回一个默认的向量
	log.Warnf("text not found in mock data, returning default vector: %s", utils.ShrinkString(text, 50))
	return generateDefaultVector(text), nil
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

// SaveEmbeddingToMockData 将embedding数据保存到mock数据（用于生成测试数据）
func SaveEmbeddingToMockData(text string, embedding []float32) error {
	var embeddingData map[string][]float32
	if err := json.Unmarshal(mockEmbeddingDataJSON, &embeddingData); err != nil {
		embeddingData = make(map[string][]float32)
	}

	embeddingData[text] = embedding

	// 保存回JSON
	data, err := json.MarshalIndent(embeddingData, "", "  ")
	if err != nil {
		return err
	}

	log.Infof("saved embedding for text: %s (dimension: %d)", utils.ShrinkString(text, 50), len(embedding))
	log.Debugf("embedding data to save:\n%s", string(data))

	return nil
}
