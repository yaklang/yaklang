package rag

import (
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

type SearchResult struct {
	Document           *vectorstore.Document      `json:"document"` // 检索到的文档
	KnowledgeBaseEntry *schema.KnowledgeBaseEntry `json:"knowledgeBaseEntry"`
	Entity             *schema.ERModelEntity      `json:"entity"`
	Score              float64                    `json:"score"` // 相似度得分 (-1 到 1 之间)
}
