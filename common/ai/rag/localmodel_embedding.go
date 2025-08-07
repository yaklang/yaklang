package rag

import "github.com/yaklang/yaklang/common/ai/localmodel"

type LocalModelEmbedding struct {
}

func NewLocalModelEmbedding(model *localmodel.Model) *LocalModelEmbedding {
	return &LocalModelEmbedding{
		model: model,
	}
}
