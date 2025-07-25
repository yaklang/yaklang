package rag

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
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
func FilterResults(results []SearchResult, threshold float32) []SearchResult {
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

func ConvertLayersInfoToGraph(layers []*hnsw.Layer[string], saveVectorByKey func(string, []float32)) *schema.GroupInfos {
	groupInfos := make(schema.GroupInfos, 0)

	// 遍历每一层
	for layerLevel, layer := range layers {
		if layer == nil || layer.Nodes == nil {
			continue
		}

		// 遍历该层的每个节点
		for nodeKey, layerNode := range layer.Nodes {
			if layerNode == nil {
				continue
			}

			// 保存节点的向量数据
			if saveVectorByKey != nil {
				saveVectorByKey(nodeKey, layerNode.Value())
			}

			// 收集邻居键
			neighbors := make([]string, 0, len(layerNode.Neighbors))
			for neighborKey := range layerNode.Neighbors {
				neighbors = append(neighbors, neighborKey)
			}

			// 对邻居键排序以确保一致性
			slices.Sort(neighbors)

			// 创建 GroupInfo
			groupInfo := schema.GroupInfo{
				LayerLevel: layerLevel,
				Key:        nodeKey,
				Neighbors:  neighbors,
			}

			groupInfos = append(groupInfos, groupInfo)
		}
	}

	return &groupInfos
}

func ParseLayersInfo(graphInfos *schema.GroupInfos, loadVectorByKey func(string) []float32) []*hnsw.Layer[string] {
	layers := make([]*hnsw.Layer[string], 0)
	layerMap := make(map[int]*hnsw.Layer[string])
	for _, graphInfo := range *graphInfos {
		graphInfo := graphInfo
		if _, ok := layerMap[graphInfo.LayerLevel]; !ok {
			layerMap[graphInfo.LayerLevel] = &hnsw.Layer[string]{
				Nodes: make(map[string]*hnsw.LayerNode[string]),
			}
		}
		neighbors := make(map[string]*hnsw.LayerNode[string])
		for _, neighbor := range graphInfo.Neighbors {
			neighbors[neighbor] = &hnsw.LayerNode[string]{
				Node: hnsw.Node[string]{
					Key:   neighbor,
					Value: func() []float32 { return loadVectorByKey(neighbor) },
				},
			}
		}
		layerMap[graphInfo.LayerLevel].Nodes[graphInfo.Key] = &hnsw.LayerNode[string]{
			Node: hnsw.Node[string]{
				Key:   graphInfo.Key,
				Value: func() []float32 { return loadVectorByKey(graphInfo.Key) },
			},
			Neighbors: neighbors,
		}
	}
	// layerMap 按层数排序
	keys := make([]int, 0)
	for key := range layerMap {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		layers = append(layers, layerMap[key])
	}
	return layers
}
