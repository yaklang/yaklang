package rag

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ChunkText 将长文本分割成多个小块，以便于处理和嵌入
// 使用rune来分割文本，更好地支持Unicode字符（如中文）
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

	// 如果文本为空，返回空切片
	if text == "" {
		return []string{}
	}

	// 将文本转换为rune切片，以正确处理Unicode字符
	runes := []rune(text)
	textLen := len(runes)

	// 如果文本长度小于等于最大块大小，直接返回原文本
	if textLen <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	for i := 0; i < textLen; i += maxChunkSize - overlap {
		end := i + maxChunkSize
		if end > textLen {
			end = textLen
		}

		// 尝试在合适的位置分割，避免在单词中间分割
		actualEnd := end
		if end < textLen {
			// 向后查找合适的分割点（空格、标点符号等）
			for j := end; j > i && j < textLen && (end-j) < 50; j-- {
				char := runes[j]
				if char == ' ' || char == '\n' || char == '\t' ||
					char == '。' || char == '！' || char == '？' || char == '；' ||
					char == '.' || char == '!' || char == '?' || char == ';' ||
					char == ',' || char == '，' {
					actualEnd = j + 1
					break
				}
			}
		}

		chunk := string(runes[i:actualEnd])
		// 移除首尾空白字符
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		if actualEnd >= textLen {
			break
		}

		// 调整下一次的起始位置
		if actualEnd != end {
			i = actualEnd - (maxChunkSize - overlap)
			if i < 0 {
				i = 0
			}
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
		for k, v := range metadata {
			doc.Metadata[k] = v
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

func ParseLayersInfo(graphInfos *schema.GroupInfos, loadVectorByKey func(string) []float32) []*hnsw.Layer[string] {
	if graphInfos == nil || len(*graphInfos) == 0 {
		return nil
	}

	// 注意：当前只支持恢复标准HNSW图（非PQ优化）
	// 如果需要PQ优化支持，需要额外的序列化信息

	layers := make([]*hnsw.Layer[string], 0)
	layerMap := make(map[int]*hnsw.Layer[string])

	// 第一步：创建所有节点的映射，但不建立邻居关系
	nodeMap := make(map[string]hnswspec.LayerNode[string])

	for _, graphInfo := range *graphInfos {
		if _, ok := layerMap[graphInfo.LayerLevel]; !ok {
			layerMap[graphInfo.LayerLevel] = &hnsw.Layer[string]{
				Nodes: make(map[string]hnswspec.LayerNode[string]),
			}
		}

		// 为每个节点创建StandardLayerNode（非PQ模式）
		if _, exists := nodeMap[graphInfo.Key]; !exists {
			nodeMap[graphInfo.Key] = hnswspec.NewStandardLayerNode(
				graphInfo.Key,
				func() []float32 { return loadVectorByKey(graphInfo.Key) },
			)
		}

		layerMap[graphInfo.LayerLevel].Nodes[graphInfo.Key] = nodeMap[graphInfo.Key]
	}

	// 第二步：建立邻居关系
	for _, graphInfo := range *graphInfos {
		currentNode := nodeMap[graphInfo.Key]
		if currentNode == nil {
			continue
		}

		// 创建邻居节点并建立连接
		for _, neighborKey := range graphInfo.Neighbors {
			if neighborNode, exists := nodeMap[neighborKey]; exists {
				// 这里使用默认的距离函数，实际应该从图配置中获取
				// 但由于我们只是恢复结构，不重新计算邻居，所以使用任意距离函数
				currentNode.AddNeighbor(neighborNode, 16, hnswspec.CosineDistance[string])
			}
		}
	}

	// 按层数排序
	keys := make([]int, 0, len(layerMap))
	for key := range layerMap {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		layers = append(layers, layerMap[key])
	}

	return layers
}

// MockEmbedder 是一个模拟的嵌入客户端，用于测试
type MockEmbedder struct {
	MockEmbedderFunc func(text string) ([]float32, error)
}

func NewMockEmbedder(f func(text string) ([]float32, error)) EmbeddingClient {
	return &MockEmbedder{
		MockEmbedderFunc: f,
	}
}

// Embedding 模拟实现 EmbeddingClient 接口
func (m *MockEmbedder) Embedding(text string) ([]float32, error) {
	return m.MockEmbedderFunc(text)
}

type NodeOffsetToVectorFunc func(offset uint32) []float32

func ParseHNSWGraphFromBinary(collectionName string, graphBinaryReader io.Reader, db *gorm.DB, cacheMinSize int, cacheMaxSize int) (*hnsw.Graph[string], error) {
	cache := map[hnswspec.LazyNodeID]any{}
	clearCache := func() {
		if len(cache) > cacheMaxSize {
			clearNum := len(cache) - cacheMinSize
			clearKeys := []hnswspec.LazyNodeID{}
			for key := range cache {
				clearKeys = append(clearKeys, key)
				clearNum--
				if clearNum <= 0 {
					break
				}
			}
			for _, key := range clearKeys {
				delete(cache, key)
			}
		}
	}
	allOpts := getDefaultHNSWGraphOptions(collectionName)
	return hnsw.LoadGraphFromBinary(graphBinaryReader, func(key hnswspec.LazyNodeID) (hnswspec.LayerNode[string], error) {
		uidStr := fmt.Sprint(key)

		doc, err := getVectorDocumentByLazyNodeID(db.Select("document_id,pq_mode"), key)
		if err != nil {
			return nil, err
		}
		docId := doc.DocumentID
		var newNode hnswspec.LayerNode[string]
		if doc.PQMode {
			newNode = hnswspec.NewLazyRawPQLayerNode(docId, func() ([]byte, error) {
				if node, ok := cache[uidStr]; ok {
					return node.([]byte), nil
				}

				doc, err := getVectorDocumentByLazyNodeID(db.Select("pq_code"), key)
				if err != nil {
					return nil, err
				}
				clearCache()
				cache[uidStr] = doc.PQCode
				return doc.PQCode, nil
			})
		} else {
			newNode = hnswspec.NewStandardLayerNode(docId, func() []float32 {
				if node, ok := cache[uidStr]; ok {
					return node.([]float32)
				}

				doc, err := getVectorDocumentByLazyNodeID(db.Select("embedding"), key)
				if err != nil {
					log.Errorf("get vector document by lazy node id err: %v", err)
					return nil
				}
				clearCache()
				cache[uidStr] = []float32(doc.Embedding)
				return doc.Embedding
			})
		}

		return newNode, nil
	}, allOpts...)
}

func getVectorDocumentByLazyNodeID(db *gorm.DB, id hnswspec.LazyNodeID) (*schema.VectorStoreDocument, error) {
	var doc schema.VectorStoreDocument
	var err error
	switch ret := id.(type) {
	case []byte:
		err = db.Where("uid = ?", ret).First(&doc).Error
	case string:
		err = db.Where("document_id = ?", ret).First(&doc).Error
	case int:
		err = db.Where("id = ?", ret).First(&doc).Error
	case int64:
		err = db.Where("id = ?", ret).First(&doc).Error
	case int32:
		err = db.Where("id = ?", ret).First(&doc).Error
	case uint32:
		err = db.Where("id = ?", ret).First(&doc).Error
	case uint64:
		err = db.Where("id = ?", ret).First(&doc).Error
	}
	return &doc, err
}

func ExportHNSWGraphToBinary(graph *hnsw.Graph[string]) (io.Reader, error) {
	pers, err := hnsw.ExportHNSWGraph(graph)
	if err != nil {
		return nil, err
	}
	pers.Dims = 1024
	return pers.ToBinary(context.Background())
}

const (
	uidTypeMd5        = "md5"
	uidTypeID         = "id"
	uidTypeDocumentID = "document_id"
)

func getLazyNodeUIDByMd5(collectionName string, key string) []byte {
	m := md5.Sum([]byte(collectionName + key))
	return m[:]
}

func getLazyNodeUID(uidType string, collectionName string, data any) hnswspec.LazyNodeID {
	switch uidType {
	case uidTypeMd5:
		key, ok := data.(string)
		if !ok {
			log.Errorf("expected string for key, got %T", data)
			return nil
		}
		return getLazyNodeUIDByMd5(collectionName, key)
	case uidTypeID:
		key := utils.InterfaceToInt(data)
		return hnswspec.LazyNodeID(key)
	case uidTypeDocumentID:
		key, ok := data.(string)
		if !ok {
			log.Errorf("expected string for key, got %T", data)
			return nil
		}
		return hnswspec.LazyNodeID(key)
	}
	return nil
}

var defaultUidType = uidTypeMd5

func getDefaultHNSWGraphOptions(collectionName string) []hnsw.GraphOption[string] {
	return []hnsw.GraphOption[string]{
		hnsw.WithNodeType[string](hnsw.InputNodeTypeLazy),
		hnsw.WithConvertToUIDFunc[string](func(node hnswspec.LayerNode[string]) (hnswspec.LazyNodeID, error) {
			return hnswspec.LazyNodeID(getLazyNodeUIDByMd5(collectionName, node.GetKey())), nil
		}),
		hnsw.WithDeterministicRng[string](0),
	}
}

func NewHNSWGraph(collectionName string, opts ...hnsw.GraphOption[string]) *hnsw.Graph[string] {
	allOpts := getDefaultHNSWGraphOptions(collectionName)
	return hnsw.NewGraph(append(allOpts, opts...)...)
}

var graphNodesIsEmpty = errors.New("hnsw graph nodes is empty")

func MigrateHNSWGraph(db *gorm.DB, collection *schema.VectorStoreCollection) error {
	hnswGraph := NewHNSWGraph(collection.Name)

	// 分页查询向量节点
	pageSize := 1000

	getVectorByID := func(id hnswspec.LazyNodeID) ([]float32, error) {
		doc, err := getVectorDocumentByLazyNodeID(db, id)
		if err != nil {
			return nil, err
		}
		return doc.Embedding, nil
	}

	for page := 1; ; page++ {
		var docs []schema.VectorStoreDocument
		err := db.Where("collection_id = ?", collection.ID).Offset((page - 1) * pageSize).Limit(pageSize).Find(&docs).Error
		if err != nil {
			return utils.Wrap(err, "get docs")
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			doc := doc
			doc.UID = getLazyNodeUIDByMd5(collection.Name, doc.DocumentID)
			db.Save(doc)
			hnswGraph.Add(hnsw.MakeInputNodeFromID(doc.DocumentID, hnswspec.LazyNodeID(doc.UID), func(uid hnswspec.LazyNodeID) ([]float32, error) {
				return getVectorByID(uid)
			}))
		}
	}

	if len(hnswGraph.Layers) == 0 || len(hnswGraph.Layers[0].Nodes) == 0 {
		return graphNodesIsEmpty
	}
	graphBinaryReader, err := ExportHNSWGraphToBinary(hnswGraph)
	if err != nil {
		return utils.Wrap(err, "export hnsw graph to binary")
	}
	binaryBytes, err := io.ReadAll(graphBinaryReader)
	if err != nil {
		return utils.Wrap(err, "read graph binary")
	}
	err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Update("graph_binary", binaryBytes).Error
	if err != nil {
		return utils.Wrap(err, "update graph binary")
	}
	collection.GraphBinary = binaryBytes
	return nil
}
