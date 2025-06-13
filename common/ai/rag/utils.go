package rag

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ChunkText 将长文本分割成多个小块，以便于处理和嵌入
func ChunkText(text string, maxChunkSize int, overlap int) []string {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000 // 默认块大小
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChunkSize {
		overlap = maxChunkSize / 2
	}

	// 分割文本
	words := strings.Fields(text)
	if len(words) <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	for i := 0; i < len(words); i += maxChunkSize - overlap {
		end := i + maxChunkSize
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		chunks = append(chunks, chunk)
		if end == len(words) {
			break
		}
	}

	return chunks
}

// TextToDocuments 将文本转换为文档对象
func TextToDocuments(text string, maxChunkSize int, overlap int, metadata map[string]any) []Document {
	chunks := ChunkText(text, maxChunkSize, overlap)
	docs := make([]Document, len(chunks))

	for i, chunk := range chunks {
		// 生成唯一ID
		id := uuid.New().String()

		// 创建文档
		doc := Document{
			ID:       id,
			Content:  chunk,
			Metadata: make(map[string]any),
		}

		// 复制元数据
		if metadata != nil {
			for k, v := range metadata {
				doc.Metadata[k] = v
			}
		}

		// 添加额外元数据
		doc.Metadata["chunk_index"] = i
		doc.Metadata["total_chunks"] = len(chunks)
		doc.Metadata["created_at"] = time.Now().Unix()

		docs[i] = doc
	}

	return docs
}

// FormatRagPrompt 格式化 RAG 提示，结合用户问题和检索到的文档
func FormatRagPrompt(query string, results []SearchResult, promptTemplate string) string {
	if promptTemplate == "" {
		promptTemplate = `使用以下信息来回答问题。如果你不知道答案，只需说你不知道，不要试图编造信息。

参考信息:
%s

问题: %s

回答:`
	}

	// 格式化检索到的文档
	var contextBuilder strings.Builder
	for i, result := range results {
		contextBuilder.WriteString(fmt.Sprintf("文档 %d [相关度: %.2f]:\n%s\n\n",
			i+1, result.Score, result.Document.Content))
	}

	// 应用模板
	prompt := fmt.Sprintf(promptTemplate, contextBuilder.String(), query)
	return prompt
}

// FilterResults 根据相似度阈值过滤搜索结果
func FilterResults(results []SearchResult, threshold float64) []SearchResult {
	var filtered []SearchResult
	for _, result := range results {
		if result.Score >= threshold {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// SplitDocumentsByMetadata 根据元数据字段将文档分组
func SplitDocumentsByMetadata(docs []Document, metadataKey string) map[any][]Document {
	groups := make(map[any][]Document)

	for _, doc := range docs {
		value, exists := doc.Metadata[metadataKey]
		if !exists {
			value = nil
		}

		groups[value] = append(groups[value], doc)
	}

	return groups
}
