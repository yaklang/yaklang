package vectorstore

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var defaultMaxChunkSize = 800
var defaultChunkOverlap = 100
var defaultBigTextPlan = BigTextPlanChunkText

func processBigText(embeddingsClient EmbeddingClient, doc *Document, maxChunkSize int, overlap int, bigTextPlan string) ([]*Document, error) {
	log.Infof("processing big text for document %s using plan: %s", doc.ID, bigTextPlan)

	// 根据元数据调整分块参数
	if chunkSize, ok := doc.Metadata["chunk_size"].(int); ok && chunkSize > 0 {
		maxChunkSize = chunkSize
	}
	if chunkOverlap, ok := doc.Metadata["chunk_overlap"].(int); ok && chunkOverlap >= 0 {
		overlap = chunkOverlap
	}

	// 分割文本
	chunks := ChunkText(doc.Content, maxChunkSize, overlap)
	if len(chunks) == 0 {
		return nil, utils.Errorf("failed to chunk text for document %s", doc.ID)
	}

	log.Infof("split document %s into %d chunks", doc.ID, len(chunks))

	switch bigTextPlan {
	case BigTextPlanChunkText:
		return processChunkText(embeddingsClient, doc, chunks)
	case BigTextPlanChunkTextAndAvgPooling:
		return processChunkTextAndAvgPooling(embeddingsClient, doc, chunks)
	default:
		log.Warnf("unknown big text plan: %s, using default chunkText", bigTextPlan)
		return processChunkText(embeddingsClient, doc, chunks)
	}
}

// processChunkText 将文本分割成多个文档分别存储
func processChunkText(embeddingsClient EmbeddingClient, originalDoc *Document, chunks []string) ([]*Document, error) {
	log.Infof("processing document %s with chunkText strategy, creating %d documents", originalDoc.ID, len(chunks))

	if len(chunks) == 0 {
		return []*Document{}, nil
	}

	// 并行处理结果结构
	type chunkResult struct {
		index int
		doc   *Document
		err   error
	}

	// 创建结果channel和sync.WaitGroup
	resultChan := make(chan chunkResult, len(chunks))
	var wg sync.WaitGroup

	// 并行处理每个chunk
	for i, chunk := range chunks {
		wg.Add(1)
		go func(index int, chunkText string) {
			defer wg.Done()

			// 为每个分块生成嵌入
			embedding, err := embeddingsClient.Embedding(chunkText)
			if err != nil {
				log.Errorf("failed to generate embedding for chunk %d of document %s: %v", index, originalDoc.ID, err)
				resultChan <- chunkResult{
					index: index,
					err:   utils.Errorf("failed to generate embedding for chunk %d: %v", index, err),
				}
				return
			}

			if len(embedding) == 0 {
				log.Warnf("empty embedding for chunk %d of document %s, skipping", index, originalDoc.ID)
				resultChan <- chunkResult{
					index: index,
					err:   nil, // 空embedding不是错误，只是跳过
				}
				return
			}

			// 创建新文档
			chunkDoc := &Document{
				ID:        fmt.Sprintf("%s_chunk_%d", originalDoc.ID, index),
				Content:   chunkText,
				Metadata:  make(map[string]any),
				Embedding: embedding,
			}

			// 复制原始文档的元数据
			for k, v := range originalDoc.Metadata {
				chunkDoc.Metadata[k] = v
			}

			// 添加分块特有的元数据
			chunkDoc.Metadata["original_doc_id"] = originalDoc.ID
			chunkDoc.Metadata["chunk_index"] = index
			chunkDoc.Metadata["total_chunks"] = len(chunks)
			chunkDoc.Metadata["is_chunk"] = true

			resultChan <- chunkResult{
				index: index,
				doc:   chunkDoc,
				err:   nil,
			}
		}(i, chunk)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	results := make([]chunkResult, 0, len(chunks))
	for result := range resultChan {
		results = append(results, result)
	}

	// 检查错误
	var firstError error
	validDocs := make([]*Document, 0, len(chunks))

	// 按原始索引顺序排序结果
	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	for _, result := range results {
		if result.err != nil {
			if firstError == nil {
				firstError = result.err
			}
			continue
		}

		// 只有非空embedding的文档才添加到结果中
		if len(result.doc.Embedding) > 0 {
			validDocs = append(validDocs, result.doc)
		}
	}

	// 如果有错误，返回第一个错误
	if firstError != nil {
		return nil, firstError
	}

	log.Infof("successfully created %d chunk documents for %s", len(validDocs), originalDoc.ID)
	return validDocs, nil
}

// processChunkTextAndAvgPooling 将文本分割后生成多个嵌入向量，然后平均池化成一个文档存储
func processChunkTextAndAvgPooling(embeddingsClient EmbeddingClient, originalDoc *Document, chunks []string) ([]*Document, error) {
	log.Infof("processing document %s with chunkTextAndAvgPooling strategy", originalDoc.ID)

	var embeddings [][]float32
	var combinedContent string

	// 为每个分块生成嵌入
	for i, chunk := range chunks {
		embedding, err := embeddingsClient.Embedding(chunk)
		if err != nil {
			log.Errorf("failed to generate embedding for chunk %d of document %s: %v", i, originalDoc.ID, err)
			return nil, utils.Errorf("failed to generate embedding for chunk %d: %v", i, err)
		}

		if len(embedding) == 0 {
			log.Warnf("empty embedding for chunk %d of document %s, skipping", i, originalDoc.ID)
			continue
		}

		embeddings = append(embeddings, embedding)
		if i == 0 {
			combinedContent = chunk
		} else {
			combinedContent += " " + chunk
		}
	}

	if len(embeddings) == 0 {
		return nil, utils.Errorf("no valid embeddings generated for document %s", originalDoc.ID)
	}

	// 对所有嵌入向量进行平均池化
	avgEmbedding := averagePooling(embeddings)
	if avgEmbedding == nil {
		return nil, utils.Errorf("failed to compute average pooling for document %s", originalDoc.ID)
	}

	// 创建合并后的文档
	pooledDoc := &Document{
		ID:        originalDoc.ID,
		Content:   combinedContent,
		Metadata:  make(map[string]any),
		Embedding: avgEmbedding,
	}

	// 复制原始文档的元数据
	for k, v := range originalDoc.Metadata {
		pooledDoc.Metadata[k] = v
	}

	// 添加池化特有的元数据
	pooledDoc.Metadata["is_pooled"] = true
	pooledDoc.Metadata["pooled_chunks"] = len(embeddings)
	pooledDoc.Metadata["pooling_method"] = "average"

	log.Infof("successfully created pooled document for %s from %d chunks", originalDoc.ID, len(embeddings))
	return []*Document{pooledDoc}, nil
}

// averagePooling 对多个嵌入向量进行平均池化
func averagePooling(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// 获取向量维度
	dim := len(embeddings[0])
	if dim == 0 {
		return nil
	}

	// 初始化结果向量
	result := make([]float32, dim)
	validCount := 0

	// 累加所有向量
	for _, embedding := range embeddings {
		if len(embedding) != dim {
			log.Warnf("embedding dimension mismatch: expected %d, got %d", dim, len(embedding))
			continue
		}
		validCount++
		for i, val := range embedding {
			result[i] += val
		}
	}

	// 如果没有有效向量，返回nil
	if validCount == 0 {
		return nil
	}

	// 计算平均值
	count := float32(validCount)
	for i := range result {
		result[i] /= count
	}

	return result
}
