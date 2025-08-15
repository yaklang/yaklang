package rag

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/rag/test"
	"github.com/yaklang/yaklang/common/utils"
)

// getMockRagDataForTest 在当前函数中缓存embedding数据，避免每次都读取文件
func getMockRagDataForTest() (func(text string) ([]float32, error), error) {
	content, err := test.FS.ReadFile("mock_embedding_data.json")
	if err != nil {
		return nil, utils.Errorf("failed to read embedding data: %v", err)
	}
	var embeddingData map[string][]float32
	err = json.Unmarshal(content, &embeddingData)
	if err != nil {
		return nil, utils.Errorf("failed to unmarshal embedding data: %v", err)
	}
	// 返回一个函数，用于获取嵌入数据
	return func(text string) ([]float32, error) {
		embedding, ok := embeddingData[text]
		if !ok {
			return nil, utils.Errorf("text not found: %s", text)
		}
		return embedding, nil
	}, nil
}
