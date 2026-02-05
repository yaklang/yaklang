package rag

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

type SearchResult struct {
	Document           *vectorstore.Document      `json:"document"` // 检索到的文档
	KnowledgeBaseEntry *schema.KnowledgeBaseEntry `json:"knowledgeBaseEntry"`
	Entity             *schema.ERModelEntity      `json:"entity"`
	Score              float64                    `json:"score"` // 相似度得分 (-1 到 1 之间)
}

func (s *SearchResult) GetContent() string {
	var result bytes.Buffer
	if s.Document != nil {
		result.WriteString(s.Document.Content)
	}
	if s.KnowledgeBaseEntry != nil {
		result.WriteString("\n")
		result.WriteString(s.KnowledgeBaseEntry.KnowledgeTitle + "[" + s.KnowledgeBaseEntry.KnowledgeType + "]")
		result.WriteString("\n")
		result.WriteString(s.KnowledgeBaseEntry.KnowledgeDetails)
	}
	return result.String()
}
