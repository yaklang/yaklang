package localmodel

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
)

func GetDefaultEmbeddingModelPath() string {
	p, err := thirdparty_bin.GetBinaryPath("model-Qwen3-Embedding-0.6B-Q4")
	if err != nil {
		log.Errorf("failed to get default embedding model path: %v", err)
		return consts.GetQwen3Embedding0_6BQ4_0ModelPath()
	}
	return p
}

func IsDefaultModelAvailable() bool {
	modelPath := GetDefaultEmbeddingModelPath()
	exists, err := utils.PathExists(modelPath)
	return err == nil && exists
}
