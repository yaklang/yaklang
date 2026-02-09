package rag

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/localmodel"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
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
func TextToDocuments(text string, maxChunkSize int, overlap int, metadata map[string]any) []vectorstore.Document {
	chunks := ChunkText(text, maxChunkSize, overlap)
	docs := make([]vectorstore.Document, len(chunks))

	for i, chunk := range chunks {
		// 生成唯一ID
		id := uuid.New().String()

		// 创建文档
		doc := vectorstore.Document{
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

const defaultEmbeddingModelName = "Qwen3-Embedding-0.6B-Q4_K_M"

var (
	embeddingAvailableCache sync.Map // map[string]*embeddingAvailableChecker

	// bounded negative caching to avoid hammering filesystem during startup storms
	embeddingAvailabilityNegativeCacheTTL = 10 * time.Second

	// test seam
	getModelPath = localmodel.GetModelPath
)

type embeddingAvailableChecker struct {
	check func() bool
	close func()
}

func newEmbeddingAvailableChecker(modelName string) *embeddingAvailableChecker {
	cd := utils.NewCoolDown(embeddingAvailabilityNegativeCacheTTL)

	available := utils.NewAtomicBool()

	firstDone := make(chan struct{})
	firstDoneOnce := sync.Once{}

	check := func() bool {
		if available.IsSet() {
			return true
		}

		cd.DoOr(func() {
			_, err := getModelPath(modelName)
			ok := err == nil
			available.SetTo(ok)
			firstDoneOnce.Do(func() { close(firstDone) })
		}, func() {
			select {
			case <-firstDone:
			case <-time.After(10 * time.Second): // if first check hangs, avoid deadlock
			}
		})

		return available.IsSet()
	}

	return &embeddingAvailableChecker{
		check: check,
		close: func() {
			firstDoneOnce.Do(func() { close(firstDone) })
			cd.Close()
		},
	}
}

func clearEmbeddingAvailableCache() {
	embeddingAvailableCache.Range(func(key, value any) bool {
		if checker, ok := value.(*embeddingAvailableChecker); ok && checker != nil && checker.close != nil {
			checker.close()
		}
		embeddingAvailableCache.Delete(key)
		return true
	})
}

func CheckConfigEmbeddingAvailable(opts ...RAGSystemConfigOption) bool {
	config := NewRAGSystemConfig(opts...)
	if config.embeddingClient != nil {
		return true
	}

	modelName := defaultEmbeddingModelName
	if config.modelName != "" {
		modelName = config.modelName
	}

	checkerAny, _ := embeddingAvailableCache.LoadOrStore(modelName, newEmbeddingAvailableChecker(modelName))
	checker := checkerAny.(*embeddingAvailableChecker)
	if checker.check() {
		return true
	}
	return vectorstore.IsAIBalanceFreeServiceAvailable() // fallback to ai balance free service
}

func NewVectorStoreDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", path)
	if err != nil {
		return db, err
	}
	err = autoMigrateRAGSystem(db)
	if err != nil {
		return db, err
	}

	return db, nil
}

func NewTemporaryRAGDB() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	err = autoMigrateRAGSystem(db)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func autoMigrateRAGSystem(db *gorm.DB) error {
	return db.AutoMigrate(
		&schema.KnowledgeBaseEntry{},
		&schema.KnowledgeBaseInfo{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},

		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
		&schema.EntityRepository{},

		&schema.VectorStoreDocument{},
		&schema.VectorStoreCollection{},
	).Error
}

// 创建一个 Mock AI 服务，用于测试和开发环境。
//
// 参数：
// - handle(func(message string) string`): 处理消息的回调函数
//
// 返回值：
// - r1(aicommon.AICallbackType`): Mock AI 服务实例
//
// Example:
// ```go
// // 创建 Mock 服务
// mockAI = ai.MockAIService(fn(msg) {
//  return sprintf("Mock 回复: 收到消息 '%s'", msg)
// })
//
// // 使用 Mock 服务进行测试
// response = mockAI("测试消息")
// println(response)  // 输出: Mock 回复: 收到消息 '测试消息'
// ```
func MockAIService(handle func(message string) string) aicommon.AICallbackType {
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		rspMsg := handle(req.GetPrompt())
		rsp.EmitOutputStream(strings.NewReader(rspMsg))
		rsp.Close()
		return rsp, nil
	}
}

// type ragSystemCoreTables struct {
// 	VectorStore      *schema.VectorStoreCollection
// 	KnowledgeBase    *schema.KnowledgeBaseInfo
// 	EntityRepository *schema.EntityRepository
// }

// func loadRagSystemCoreTables(opts ...RAGSystemConfigOption) (*ragSystemCoreTables, error) {
// 	config := NewRAGSystemConfig(opts...)
// 	coreTables := &ragSystemCoreTables{}

// 	// 加载集合信息
// 	collection, _ := loadCollectionInfoByConfig(config)
// 	if collection == nil {
// 		vectorstore.CreateCollection(config.db, config.Name, config.Description, opts...)

// 	}
// 	coreTables.VectorStore = collection

// 	// 加载知识库信息
// 	knowledgeBase, err := loadKnowledgeBaseInfoByConfig(config)
// 	if err != nil {
// 		return nil, err
// 	}
// 	coreTables.KnowledgeBase = knowledgeBase

// 	// 加载实体仓库信息
// 	entityRepository, err := loadEntityRepositoryInfoByConfig(config)
// 	if err != nil {
// 		return nil, err
// 	}
// 	coreTables.EntityRepository = entityRepository
// 	return coreTables, nil
// }

func loadCollectionInfoByConfig(config *RAGSystemConfig) (*schema.VectorStoreCollection, error) {
	if config.vectorStore != nil {
		return config.vectorStore.GetCollectionInfo(), nil
	} else {
		if config.ragID != "" {
			var collection schema.VectorStoreCollection
			err := config.db.Model(&schema.VectorStoreCollection{}).Where("rag_id = ?", config.ragID).First(&collection).Error
			if err == nil {
				return &collection, nil
			}
		}
		if config.Name != "" {
			collection, _ := yakit.GetRAGCollectionInfoByName(config.db, config.Name)
			if collection != nil {
				return collection, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func loadKnowledgeBaseInfoByConfig(config *RAGSystemConfig) (*schema.KnowledgeBaseInfo, error) {
	if config.knowledgeBase != nil {
		return config.knowledgeBase.GetKnowledgeBaseInfo(), nil
	} else {
		if config.ragID != "" {
			knowledgeBaseInfo, _ := yakit.GetKnowledgeBaseByRAGID(config.db, config.ragID)
			if knowledgeBaseInfo != nil {
				return knowledgeBaseInfo, nil
			}
		}
		if config.Name != "" {
			knowledgeBaseInfo, _ := yakit.GetKnowledgeBaseByName(config.db, config.Name)
			if knowledgeBaseInfo != nil {
				return knowledgeBaseInfo, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func loadEntityRepositoryInfoByConfig(config *RAGSystemConfig) (*schema.EntityRepository, error) {
	if config.entityRepository != nil {
		info, err := config.entityRepository.GetInfo()
		if err != nil {
			return nil, utils.Wrap(err, "get entity repository info failed")
		}
		return info, nil
	} else {
		if config.ragID != "" {
			entityRepositoryInfo, _ := yakit.GetEntityRepositoryByRAGID(config.db, config.ragID)
			if entityRepositoryInfo != nil {
				return entityRepositoryInfo, nil
			}
		}
		if config.Name != "" {
			entityRepositoryInfo, _ := yakit.GetEntityRepositoryByName(config.db, config.Name)
			if entityRepositoryInfo != nil {
				return entityRepositoryInfo, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func NewEmptyMockEmbedding() vectorstore.EmbeddingClient {
	return vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		return nil, nil
	})
}
