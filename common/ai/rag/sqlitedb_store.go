package rag

import (
	"sort"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SQLiteVectorStore 是一个基于 SQLite 的向量存储实现
type SQLiteVectorStore struct {
	db       *gorm.DB
	embedder EmbeddingClient
	mu       sync.RWMutex // 用于并发安全的互斥锁

	// 集合信息
	collectionName string
	collectionID   uint
}

// NewSQLiteVectorStore 创建一个新的 SQLite 向量存储
func NewSQLiteVectorStore(db *gorm.DB, collectionName string, modelName string, dimension int, embedder EmbeddingClient) (*SQLiteVectorStore, error) {
	// 创建或获取集合
	var collections []*schema.VectorStoreCollection
	dbErr := db.Where("name = ?", collectionName).Find(&collections)
	if dbErr.Error != nil {
		return nil, utils.Errorf("查询集合失败: %v", dbErr.Error)
	}
	var collection *schema.VectorStoreCollection
	if len(collections) == 0 {
		// 创建新集合
		collection = &schema.VectorStoreCollection{
			Name:        collectionName,
			Description: "Created by SQLiteVectorStore",
			ModelName:   modelName,
			Dimension:   dimension,
		}
		if err := db.Create(&collection).Error; err != nil {
			return nil, utils.Errorf("创建集合失败: %v", err)
		}
	} else {
		collection = collections[0]
	}

	return &SQLiteVectorStore{
		db:             db,
		embedder:       embedder,
		collectionName: collectionName,
		collectionID:   collection.ID,
	}, nil
}

func (s *SQLiteVectorStore) Remove() {
	utils.GormTransaction(s.db, func(tx *gorm.DB) error {
		if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", s.collectionID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collectionID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// 将 schema.VectorStoreDocument 转换为 Document
func (s *SQLiteVectorStore) toDocument(doc *schema.VectorStoreDocument) Document {
	return Document{
		ID:        doc.DocumentID,
		Metadata:  map[string]any(doc.Metadata),
		Embedding: []float64(doc.Embedding),
	}
}

// 将 Document 转换为 schema.VectorStoreDocument
func (s *SQLiteVectorStore) toSchemaDocument(doc Document) *schema.VectorStoreDocument {
	return &schema.VectorStoreDocument{
		DocumentID:   doc.ID,
		CollectionID: s.collectionID,
		Metadata:     schema.MetadataMap(doc.Metadata),
		Embedding:    schema.FloatArray(doc.Embedding),
	}
}

// Add 添加文档到向量存储
func (s *SQLiteVectorStore) Add(docs ...Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 开始事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, doc := range docs {
		// 确保文档有 ID
		if doc.ID == "" {
			tx.Rollback()
			return utils.Errorf("文档必须有ID")
		}

		// 确保文档有嵌入向量
		if len(doc.Embedding) == 0 {
			tx.Rollback()
			return utils.Errorf("文档 %s 必须有嵌入向量", doc.ID)
		}

		// 检查文档是否已存在
		var existingDoc schema.VectorStoreDocument
		result := tx.Where("document_id = ?", doc.ID).First(&existingDoc)

		schemaDoc := s.toSchemaDocument(doc)

		if result.Error == nil {
			// 更新现有文档
			existingDoc.Metadata = schemaDoc.Metadata
			existingDoc.Embedding = schemaDoc.Embedding

			if err := tx.Save(&existingDoc).Error; err != nil {
				tx.Rollback()
				return utils.Errorf("更新文档失败: %v", err)
			}
		} else if result.Error == gorm.ErrRecordNotFound {
			// 创建新文档
			if err := tx.Create(schemaDoc).Error; err != nil {
				tx.Rollback()
				return utils.Errorf("创建文档失败: %v", err)
			}
		} else {
			// 其他错误
			tx.Rollback()
			return utils.Errorf("查询文档失败: %v", result.Error)
		}
	}

	// 提交事务
	return tx.Commit().Error
}

// Search 根据查询文本检索相关文档
func (s *SQLiteVectorStore) Search(query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 生成查询的嵌入向量
	queryEmbedding, err := s.embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("为查询生成嵌入向量失败: %v", err)
	}

	// 获取所有文档
	var docs []schema.VectorStoreDocument
	if err := s.db.Where("collection_id = ?", s.collectionID).Find(&docs).Error; err != nil {
		return nil, utils.Errorf("查询文档失败: %v", err)
	}

	if len(docs) == 0 {
		return []SearchResult{}, nil
	}

	// 计算相似度并排序
	var results []SearchResult
	for _, doc := range docs {
		embedding := []float64(doc.Embedding)

		// 计算余弦相似度
		similarity, err := utils.CosineSimilarity(queryEmbedding, embedding)
		if err != nil {
			log.Warnf("计算文档 %s 的相似度失败: %v", doc.DocumentID, err)
			continue
		}

		results = append(results, SearchResult{
			Document: s.toDocument(&doc),
			Score:    similarity,
		})
	}

	// 按相似度降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制结果数量
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// Delete 根据 ID 删除文档
func (s *SQLiteVectorStore) Delete(ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, id := range ids {
		if err := tx.Where("document_id = ?", id).Delete(&schema.VectorStoreDocument{}).Error; err != nil {
			tx.Rollback()
			return utils.Errorf("删除文档 %s 失败: %v", id, err)
		}
	}

	return tx.Commit().Error
}

// Get 根据 ID 获取文档
func (s *SQLiteVectorStore) Get(id string) (Document, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var doc schema.VectorStoreDocument
	result := s.db.Where("document_id = ?", id).First(&doc)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return Document{}, false, nil
		}
		return Document{}, false, utils.Errorf("查询文档失败: %v", result.Error)
	}

	return s.toDocument(&doc), true, nil
}

// List 列出所有文档
func (s *SQLiteVectorStore) List() ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var docs []schema.VectorStoreDocument
	if err := s.db.Where("collection_id = ?", s.collectionID).Find(&docs).Error; err != nil {
		return nil, utils.Errorf("查询文档失败: %v", err)
	}

	results := make([]Document, len(docs))
	for i, doc := range docs {
		results[i] = s.toDocument(&doc)
	}

	return results, nil
}

// Count 返回文档总数
func (s *SQLiteVectorStore) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if err := s.db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", s.collectionID).Count(&count).Error; err != nil {
		return 0, utils.Errorf("计算文档数量失败: %v", err)
	}

	return count, nil
}

// 确保 SQLiteVectorStore 实现了 VectorStore 接口
var _ VectorStore = (*SQLiteVectorStore)(nil)
