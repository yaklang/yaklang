package rag

import (
	"math/rand"
	"sort"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/ai/rag/config"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SQLiteVectorStore 是一个基于 SQLite 的向量存储实现
type SQLiteVectorStoreHNSW struct {
	db         *gorm.DB
	embedder   EmbeddingClient
	mu         sync.RWMutex // 用于并发安全的互斥锁
	collection *schema.VectorStoreCollection
	// 是否自动更新 graph_infos
	EnableAutoUpdateGraphInfos bool
	hnsw                       *hnsw.Graph[string]
}

func LoadSQLiteVectorStoreHNSW(db *gorm.DB, collectionName string, embedder EmbeddingClient) (*SQLiteVectorStoreHNSW, error) {
	var collections []*schema.VectorStoreCollection
	dbErr := db.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Find(&collections)
	if dbErr.Error != nil {
		return nil, utils.Errorf("查询集合失败: %v", dbErr.Error)
	}

	if len(collections) == 0 {
		return nil, utils.Errorf("集合 %s 不存在", collectionName)
	}

	config := collections[0]
	hnswGraph := hnsw.NewGraph(
		hnsw.WithHNSWParameters[string](config.M, config.Ml, config.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc(config.DistanceFuncType)),
		hnsw.WithDeterministicRng[string](0), // 使用固定的随机数生成器，便于调试，不影响结果
	)

	// 尝试恢复HNSW图结构
	layers := ParseLayersInfo(&collections[0].GroupInfos, func(key string) []float32 {
		var doc schema.VectorStoreDocument
		db.Where("document_id = ?", key).First(&doc)
		return []float32(doc.Embedding)
	})

	// 检查是否成功恢复了图结构
	// 如果GroupInfos不为空但layers为nil，可能是不支持的格式
	if layers == nil && len(collections[0].GroupInfos) > 0 {
		// 图信息存在但无法恢复，可能是PQ模式或其他不支持的格式
		log.Warnf("无法从数据库恢复HNSW图结构，将使用空图开始")
	}

	hnswGraph.Layers = layers
	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: true,
		embedder:                   embedder,
		collection:                 collections[0],
		hnsw:                       hnswGraph,
	}
	hnswGraph.OnLayersChange = func(layers []*hnsw.Layer[string]) {
		if vectorStore.EnableAutoUpdateGraphInfos {
			vectorStore.UpdateAutoUpdateGraphInfos()
		}
	}

	return vectorStore, nil
}

func (s *SQLiteVectorStoreHNSW) UpdateAutoUpdateGraphInfos() error {
	graphInfos := ConvertLayersInfoToGraph(s.hnsw.Layers, func(key string, vec []float32) {})
	resDb := s.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Update("group_infos", graphInfos)
	return resDb.Error
}

// NewSQLiteVectorStore 创建一个新的 SQLite 向量存储
func NewSQLiteVectorStoreHNSW(name string, description string, modelName string, dimension int, embedder EmbeddingClient, db *gorm.DB, options ...config.SQLiteVectorStoreHNSWOption) (*SQLiteVectorStoreHNSW, error) {
	cfg := config.NewSQLiteVectorStoreHNSWConfig()
	for _, option := range options {
		option(cfg)
	}

	// 创建或获取集合
	var collections []*schema.VectorStoreCollection
	dbErr := db.Where("name = ?", name).Find(&collections)
	if dbErr.Error != nil {
		return nil, utils.Errorf("查询集合失败: %v", dbErr.Error)
	}
	var collection *schema.VectorStoreCollection
	if len(collections) == 0 {
		// 创建新集合
		collection = &schema.VectorStoreCollection{
			Name:             name,
			Description:      description,
			ModelName:        modelName,
			Dimension:        dimension,
			M:                cfg.M,
			Ml:               cfg.Ml,
			EfSearch:         cfg.EfSearch,
			EfConstruct:      cfg.EfConstruct,
			DistanceFuncType: cfg.DistanceFuncType,
		}
		if err := db.Create(&collection).Error; err != nil {
			return nil, utils.Errorf("创建集合失败: %v", err)
		}
	} else {
		collection = collections[0]
	}

	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: true,
		embedder:                   embedder,
		collection:                 collection,
		hnsw:                       hnsw.NewGraph(hnsw.WithRng[string](rand.New(rand.NewSource(0)))), // 使用固定的随机数生成器，便于调试，不影响结果
	}
	vectorStore.hnsw.OnLayersChange = func(layers []*hnsw.Layer[string]) {
		if vectorStore.EnableAutoUpdateGraphInfos {
			vectorStore.UpdateAutoUpdateGraphInfos()
		}
	}
	return vectorStore, nil
}
func RemoveCollectionHNSW(db *gorm.DB, collectionName string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		var collections []schema.VectorStoreCollection
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Find(&collections).Error; err != nil {
			return err
		}
		if len(collections) == 0 {
			return utils.Errorf("集合 %s 不存在", collectionName)
		}
		collection := collections[0]

		if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
			return err
		}
		return nil
	})
}
func (s *SQLiteVectorStoreHNSW) Remove() error {
	collectionName := s.collection.Name
	return utils.GormTransaction(s.db, func(tx *gorm.DB) error {
		var collections []schema.VectorStoreCollection
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("name = ?", collectionName).Find(&collections).Error; err != nil {
			return err
		}
		if len(collections) == 0 {
			return utils.Errorf("集合 %s 不存在", collectionName)
		}
		collection := collections[0]

		if err := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Unscoped().Delete(&schema.VectorStoreCollection{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// 将 schema.VectorStoreDocument 转换为 Document
func (s *SQLiteVectorStoreHNSW) toDocument(doc *schema.VectorStoreDocument) Document {
	return Document{
		ID:        doc.DocumentID,
		Metadata:  map[string]any(doc.Metadata),
		Embedding: []float32(doc.Embedding),
		Content:   doc.Content,
	}
}

// 将 Document 转换为 schema.VectorStoreDocument
func (s *SQLiteVectorStoreHNSW) toSchemaDocument(doc Document) *schema.VectorStoreDocument {
	return &schema.VectorStoreDocument{
		DocumentID:   doc.ID,
		CollectionID: s.collection.ID,
		Metadata:     schema.MetadataMap(doc.Metadata),
		Embedding:    schema.FloatArray(doc.Embedding),
		Content:      doc.Content,
	}
}

// Add 添加文档到向量存储
func (s *SQLiteVectorStoreHNSW) Add(docs ...Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 开始事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recover from panic when adding docs: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	var updateIds []string
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
		result := tx.Where("document_id = ? and collection_id = ?", doc.ID, s.collection.ID).First(&existingDoc)

		schemaDoc := s.toSchemaDocument(doc)

		if result.Error == nil {
			// 更新现有文档
			existingDoc.Metadata = schemaDoc.Metadata
			existingDoc.Embedding = schemaDoc.Embedding
			existingDoc.Content = schemaDoc.Content

			if err := tx.Save(&existingDoc).Error; err != nil {
				tx.Rollback()
				return utils.Errorf("更新文档失败: %v", err)
			}
			updateIds = append(updateIds, doc.ID)
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

	nodes := make([]hnsw.InputNode[string], len(docs))
	for i, doc := range docs {
		nodes[i] = hnsw.InputNode[string]{
			Key:   doc.ID,
			Value: doc.Embedding,
		}
	}
	err := tx.Commit().Error
	if err != nil {
		return err
	}
	for _, id := range updateIds {
		s.hnsw.Delete(id)
	}
	s.hnsw.Add(nodes...)

	// 提交事务
	return nil
}

// Search 根据查询文本检索相关文档
func (s *SQLiteVectorStoreHNSW) Search(query string, page, limit int) ([]SearchResult, error) {
	return s.SearchWithFilter(query, page, limit, nil)
}

// SearchWithFilter 根据查询文本检索相关文档，并根据过滤函数过滤结果
func (s *SQLiteVectorStoreHNSW) SearchWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error) {
	pageSize := 10
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Infof("starting search for query with length: %d, page: %d, limit: %d", len(query), page, limit)

	// 生成查询的嵌入向量
	queryEmbedding, err := s.embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("为查询生成嵌入向量失败: %v", err)
	}
	log.Infof("generated query embedding with dimension: %d", len(queryEmbedding))

	resultNodes := s.hnsw.SearchWithDistanceAndFilter(queryEmbedding, (page-1)*pageSize+limit, func(key string, vector hnsw.Vector) bool {
		if filter != nil {
			return filter(key, func() *Document {
				var docs []*schema.VectorStoreDocument
				s.db.Where("document_id = ?", key).Find(&docs)
				if len(docs) == 0 {
					return nil
				}
				doc := docs[0]
				res := s.toDocument(doc)
				return &res
			})
		}
		return true
	})
	resultIds := make([]string, len(resultNodes))
	for i, result := range resultNodes {
		resultIds[i] = result.Key
	}
	log.Infof("hnsw search returned %d candidate documents", len(resultNodes))

	// 分批查询文档 (10个一组)
	batchSize := 10
	var allDocs []schema.VectorStoreDocument

	for i := 0; i < len(resultIds); i += batchSize {
		end := i + batchSize
		if end > len(resultIds) {
			end = len(resultIds)
		}

		batchIds := resultIds[i:end]
		var batchDocs []schema.VectorStoreDocument

		err := s.db.Where("document_id IN (?) AND collection_id = ?", batchIds, s.collection.ID).Find(&batchDocs).Error
		if err != nil {
			return nil, utils.Errorf("批量查询文档失败: %v", err)
		}

		allDocs = append(allDocs, batchDocs...)
	}

	// 创建文档ID到文档的映射，以便快速查找
	docMap := make(map[string]*schema.VectorStoreDocument)
	for i := range allDocs {
		docMap[allDocs[i].DocumentID] = &allDocs[i]
	}

	// 根据resultNodes的顺序和距离构建SearchResult
	var results []SearchResult
	for _, resultNode := range resultNodes {
		if doc, exists := docMap[resultNode.Key]; exists {
			results = append(results, SearchResult{
				Document: s.toDocument(doc),
				Score:    1 - resultNode.Distance,
			})
		}
	}

	log.Infof("calculated similarity scores for %d documents", len(results))

	// 按相似度分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	log.Infof("sorted results by similarity score")

	if page < 1 {
		page = 1
	}
	if len(results) == 0 {
		return []SearchResult{}, nil
	}
	// 计算分页
	offset := (page - 1) * pageSize
	if offset >= len(results) {
		log.Infof("page offset %d exceeds total results %d, returning empty", offset, len(results))
		return []SearchResult{}, nil
	}
	if offset+limit > len(results) {
		limit = len(results) - offset
	}
	results = results[offset : offset+limit]
	log.Infof("returning %d results after pagination (offset: %d)", len(results), offset)
	return results, nil
}

// Delete 根据 ID 删除文档
func (s *SQLiteVectorStoreHNSW) Delete(ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	utils.GormTransactionReturnDb(s.db, func(tx *gorm.DB) {
		for _, id := range ids {
			if err := tx.Where("document_id = ?", id).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
				log.Errorf("删除文档 %s 失败: %v", id, err)
			}
		}
	})

	for _, id := range ids {
		s.hnsw.Delete(id)
	}

	return nil
}

// Get 根据 ID 获取文档
func (s *SQLiteVectorStoreHNSW) Get(id string) (Document, bool, error) {
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
func (s *SQLiteVectorStoreHNSW) List() ([]Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var docs []schema.VectorStoreDocument
	if err := s.db.Where("collection_id = ?", s.collection.ID).Where("document_id <> ?", DocumentTypeCollectionInfo).Find(&docs).Error; err != nil {
		return nil, utils.Errorf("查询文档失败: %v", err)
	}

	results := make([]Document, len(docs))
	for i, doc := range docs {
		results[i] = s.toDocument(&doc)
	}

	return results, nil
}

// Count 返回文档总数
func (s *SQLiteVectorStoreHNSW) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if err := s.db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", s.collection.ID).Where("document_id <> ?", DocumentTypeCollectionInfo).Count(&count).Error; err != nil {
		return 0, utils.Errorf("计算文档数量失败: %v", err)
	}

	return count, nil
}

// 确保 SQLiteVectorStoreHNSW 实现了 VectorStore 接口
var _ VectorStore = (*SQLiteVectorStoreHNSW)(nil)
