package vectorstore

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

type NodeOffsetToVectorFunc func(offset uint32) []float32

func ParseHNSWGraphFromBinary(ctx context.Context, collectionName string, graphBinaryReader io.Reader, db *gorm.DB, cacheMinSize int, pqmode bool, wg *sync.WaitGroup) (*hnsw.Graph[string], error) {
	cacheMaxSize := cacheMinSize + 2000
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

	cols, err := yakit.GetRAGDocumentsByCollectionNameAnd(db.Limit(cacheMinSize), collectionName)
	if err != nil {
		return nil, err
	}
	for _, col := range cols {
		uidStr := fmt.Sprint(col.UID)
		cache[uidStr] = []float32(col.Embedding)
	}

	allOpts := getDefaultHNSWGraphOptions(collectionName)
	hnswGraph, err := hnsw.LoadGraphFromBinary(graphBinaryReader, func(key hnswspec.LazyNodeID) (hnswspec.LayerNode[string], error) {
		uidStr := fmt.Sprint(key)

		doc, err := getVectorDocumentByLazyNodeID(db.Select("document_id"), key)
		if err != nil {
			return nil, err
		}
		docId := doc.DocumentID
		var newNode hnswspec.LayerNode[string]
		if pqmode {
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
	if err != nil {
		return nil, err
	}
	return hnswGraph, nil
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

func GetLazyNodeUIDByMd5(collectionName string, key string) []byte {
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
		return GetLazyNodeUIDByMd5(collectionName, key)
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
			return hnswspec.LazyNodeID(GetLazyNodeUIDByMd5(collectionName, node.GetKey())), nil
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
	cacheMinSize := 100000
	cacheMaxSize := cacheMinSize + 2000
	cache := map[hnswspec.LazyNodeID][]float32{}
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
	hnswGraph := NewHNSWGraph(collection.Name)

	// 分页查询向量节点
	pageSize := 1000

	getVectorByID := func(id hnswspec.LazyNodeID) ([]float32, error) {
		idStr := fmt.Sprint(id)
		if node, ok := cache[idStr]; ok {
			return node, nil
		}
		doc, err := getVectorDocumentByLazyNodeID(db, id)
		if err != nil {
			return nil, err
		}
		clearCache()
		cache[idStr] = doc.Embedding
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
			doc.UID = GetLazyNodeUIDByMd5(collection.Name, doc.DocumentID)
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

func NewVectorStoreDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", path)
	if err != nil {
		return db, err
	}
	db = db.AutoMigrate(&schema.KnowledgeBaseEntry{}, &schema.KnowledgeBaseInfo{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})

	return db, nil
}
