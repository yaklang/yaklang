package rag

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/asynchelper"

	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
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
	hnsw *hnsw.Graph[string]

	EnableAutoUpdateGraphInfos bool
	ctx                        context.Context
	wg                         sync.WaitGroup
	UIDType                    string

	cacheSize int
}

const (
	Policy_UseDBCanche = "DB_Cache"
	Policy_UseFilter   = "Filter"
	Policy_None        = "None"
)

func LoadSQLiteVectorStoreHNSW(db *gorm.DB, collectionName string, opts ...any) (*SQLiteVectorStoreHNSW, error) {
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return nil, utils.Wrap(err, fmt.Sprintf("query rag collection [%#v]", collectionName))
	}

	if collection == nil {
		return nil, utils.Errorf("rag collection[%v] not existed", collectionName)
	}

	collectionConfig := LoadConfigFromCollectionInfo(collection, opts...)

	if err := collectionConfig.FixEmbeddingClient(); err != nil {
		return nil, utils.Errorf("fix embedding client err: %v", err)
	}

	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: collectionConfig.EnableAutoUpdateGraphInfos,
		collection:                 collection,
		ctx:                        context.Background(),
		embedder:                   collectionConfig.EmbeddingClient,
		cacheSize:                  10000,
	}

	hnswGraph := NewHNSWGraph(collectionName,
		hnsw.WithHNSWParameters[string](collectionConfig.MaxNeighbors, collectionConfig.LayerGenerationFactor, collectionConfig.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc(collectionConfig.DistanceFuncType)),
	)

	log.Infof("start to recover hnsw graph from db, collection name: %s", collectionName)
	switch collectionConfig.buildGraphPolicy {
	case Policy_UseFilter: // 选择性加载子图
		collectionConfig.buildGraphFilter.CollectionUUID = collection.UUID
		log.Info("build graph with filter policy, load existed vectors from db with filter")
		for document := range yakit.YieldVectorDocument(vectorStore.ctx, db, collectionConfig.buildGraphFilter) {
			hnswGraph.Add(hnsw.InputNode[string]{
				Key:   document.DocumentID,
				Value: document.Embedding,
			})
		}
	case Policy_None:
		log.Info("build graph with no policy, skip load existed vectors")
	case Policy_UseDBCanche:
		fallthrough
	default:
		var err error
		var isEmpty bool
		if len(collection.GraphBinary) == 0 {
			// 检测是否存在向量
			var count int64
			db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", collection.ID).Count(&count)
			if count == 0 {
				isEmpty = true
			} else {
				// 检测到旧版向量库，开始迁移 HNSW Graph
				log.Warnf("detect old version vector store, start to migrate to new version")
				err := MigrateHNSWGraph(db, collection)
				if err != nil {
					if errors.Is(err, graphNodesIsEmpty) {
						isEmpty = true
					} else {
						return nil, utils.Errorf("migrate hnsw graph err: %v", err)
					}
				}
			}

		}
		if isEmpty {
			config := collection
			hnswGraph = NewHNSWGraph(collectionName,
				hnsw.WithHNSWParameters[string](config.M, config.Ml, config.EfSearch),
				hnsw.WithDistance[string](hnsw.GetDistanceFunc(config.DistanceFuncType)),
			)
		} else {
			graphBinaryReader := bytes.NewReader(collection.GraphBinary)
			hnswGraph, err = ParseHNSWGraphFromBinary(vectorStore.ctx, collectionName, graphBinaryReader, db, vectorStore.cacheSize, collection.EnablePQMode, &vectorStore.wg)
			if err != nil {
				return nil, utils.Wrap(err, "parse hnsw graph from binary")
			}
		}

		if collection.EnablePQMode {
			if len(collection.CodeBookBinary) != 0 {
				codeBook, err := hnsw.ImportCodebook(bytes.NewReader(collection.CodeBookBinary))
				if err != nil {
					return nil, utils.Errorf("import codebook from binary err: %v", err)
				}
				hnswGraph.SetPQCodebook(codeBook)
				hnswGraph.SetPQQuantizer(pq.NewQuantizer(codeBook))
			}
		}
	}

	vectorStore.hnsw = hnswGraph
	hnswGraph.OnLayersChange = func(layers []*hnsw.Layer[string]) {
		if vectorStore.EnableAutoUpdateGraphInfos {
			vectorStore.UpdateAutoUpdateGraphInfos()
		}
	}

	return vectorStore, nil
}

func (s *SQLiteVectorStoreHNSW) ConvertToStandardMode() error {
	err := s.fixCollectionEmbeddingData()
	if err != nil {
		return utils.Wrap(err, "fix collection embedding data")
	}

	s.collection.EnablePQMode = false
	err = s.db.Save(s.collection).Error
	if err != nil {
		return utils.Wrap(err, "save collection")
	}
	return nil
}

func (s *SQLiteVectorStoreHNSW) ConvertToPQMode() error {
	var nodeNum int
	if len(s.hnsw.Layers) > 0 && len(s.hnsw.Layers[0].Nodes) > 0 {
		nodeNum = len(s.hnsw.Layers[0].Nodes)
	}
	k := 256
	if nodeNum < k {
		k = nodeNum
	}
	_, err := s.hnsw.TrainPQCodebookFromDataWithCallback(16, k, func(key string, code []byte, vector []float64) (hnswspec.LayerNode[string], error) {
		err := s.db.Model(&schema.VectorStoreDocument{}).Where("document_id = ? and collection_id = ?", key, s.collection.ID).Update("pq_code", code).Error
		if err != nil {
			return nil, utils.Wrap(err, "update pq code")
		}
		return hnswspec.NewRawPQLayerNode(key, code), nil
	})
	if err != nil {
		return utils.Wrap(err, "train pq codebook from data")
	}
	s.collection.EnablePQMode = true
	err = s.db.Save(s.collection).Error
	if err != nil {
		return utils.Wrap(err, "save collection")
	}
	s.UpdateAutoUpdateGraphInfos()
	return nil
}

func (s *SQLiteVectorStoreHNSW) GetArchived() bool {
	var collection schema.VectorStoreCollection
	err := s.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Select("archived").First(&collection).Error
	if err != nil {
		return false
	}
	return collection.Archived
}

func (s *SQLiteVectorStoreHNSW) SetArchived(archived bool) error {
	return s.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Update("archived", archived).Error
}

func (s *SQLiteVectorStoreHNSW) fixCollectionEmbeddingData() error {
	if !s.collection.EnablePQMode {
		return utils.Errorf("collection %s is not in pq mode", s.collection.Name)
	}
	pqQuantizer := s.hnsw.GetPQQuantizer()
	docNum, err := s.Count()
	if err != nil {
		return utils.Wrap(err, "fix collection embedding data")
	}
	for i := 0; i < docNum+1; i++ {
		var doc schema.VectorStoreDocument
		err := s.db.Model(&schema.VectorStoreDocument{}).Where("embedding is null").First(&doc).Error
		if err != nil {
			return utils.Wrap(err, "fix collection embedding data")
		}
		if len(doc.PQCode) == 0 {
			log.Errorf("document %s in collection %s has no pq code", doc.DocumentID, s.collection.Name)
			continue
		}
		decodedVec64, err := pqQuantizer.Decode(doc.PQCode)
		if err != nil {
			return utils.Wrap(err, "fix collection embedding data")
		}
		vec32 := make([]float32, len(decodedVec64))
		for i, v := range decodedVec64 {
			vec32[i] = float32(v)
		}
		doc.Embedding = vec32
		err = s.db.Model(&schema.VectorStoreDocument{}).Where("document_id = ?", doc.DocumentID).Update("embedding", doc.Embedding).Error
		if err != nil {
			return utils.Wrap(err, "fix collection embedding data")
		}
	}
	return nil
}

func (s *SQLiteVectorStoreHNSW) UpdateAutoUpdateGraphInfos() error {
	graphInfos, err := ExportHNSWGraphToBinary(s.hnsw)
	if err != nil {
		if errors.Is(err, graphNodesIsEmpty) {
			graphInfos = nil
		} else {
			return utils.Wrap(err, "export hnsw graph to binary")
		}
	}
	graphInfosBytes, err := io.ReadAll(graphInfos)
	if err != nil {
		return utils.Wrap(err, "read graph infos")
	}
	err = s.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Update("graph_binary", graphInfosBytes).Error
	if err != nil {
		return utils.Wrap(err, "update graph binary")
	}
	if s.collection.EnablePQMode {
		codebook, err := hnsw.ExportCodebook(s.hnsw.GetCodebook())
		if err != nil {
			return utils.Wrap(err, "export codebook")
		}
		codebookBytes, err := io.ReadAll(codebook)
		if err != nil {
			return utils.Wrap(err, "read codebook")
		}
		err = s.db.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Update("code_book_binary", codebookBytes).Error
		if err != nil {
			return utils.Wrap(err, "update codebook")
		}
	}
	return nil
}

func NewSQLiteVectorStoreHNSWEx(db *gorm.DB, name string, description string, opts ...any) (*SQLiteVectorStoreHNSW, error) {
	cfg := NewCollectionConfig(opts...)

	// 创建集合配置
	collection := schema.VectorStoreCollection{
		Name:             name,
		Description:      description,
		ModelName:        cfg.ModelName,
		Dimension:        cfg.Dimension,
		M:                cfg.MaxNeighbors,
		Ml:               cfg.LayerGenerationFactor,
		EfSearch:         cfg.EfSearch,
		EfConstruct:      cfg.EfConstruct,
		DistanceFuncType: cfg.DistanceFuncType,
	}
	// 创建集合
	res := db.Create(&collection)
	if res.Error != nil {
		return nil, utils.Errorf("创建集合失败: %v", res.Error)
	}

	if err := cfg.FixEmbeddingClient(); err != nil {
		return nil, utils.Errorf("fix embedding client err: %v", err)
	}
	hnswGraph := NewHNSWGraph(name)
	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: true,
		embedder:                   cfg.EmbeddingClient,
		collection:                 &collection,
		hnsw:                       hnswGraph,
		cacheSize:                  10000,
	}

	vectorStore.hnsw.OnLayersChange = func(layers []*hnsw.Layer[string]) {
		if vectorStore.EnableAutoUpdateGraphInfos {
			vectorStore.UpdateAutoUpdateGraphInfos()
		}
	}
	return vectorStore, nil
}

// NewSQLiteVectorStore 创建一个新的 SQLite 向量存储
func NewSQLiteVectorStoreHNSW(name string, description string, modelName string, dimension int, embedder EmbeddingClient, db *gorm.DB, options ...any) (*SQLiteVectorStoreHNSW, error) {
	options = append(options, WithModelName(modelName))
	options = append(options, WithModelDimension(dimension))
	options = append(options, WithEmbeddingClient(embedder))
	return NewSQLiteVectorStoreHNSWEx(db, name, description, options...)
}

func RemoveCollectionHNSW(db *gorm.DB, collectionName string) error {
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		collection, err := yakit.QueryRAGCollectionByName(tx, collectionName)
		if err != nil {
			return err
		}
		if collection == nil {
			return utils.Errorf("集合 %s 不存在", collectionName)
		}

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
		ID:              doc.DocumentID,
		Type:            doc.DocumentType,
		Metadata:        map[string]any(doc.Metadata),
		Embedding:       []float32(doc.Embedding),
		Content:         doc.Content,
		EntityUUID:      doc.EntityID,
		RelatedEntities: utils.PrettifyListFromStringSplitEx(doc.RelatedEntities, ",", "|"),
		RuntimeID:       doc.RuntimeID,
	}
}

// 将 Document 转换为 schema.VectorStoreDocument
func (s *SQLiteVectorStoreHNSW) toSchemaDocument(doc Document) *schema.VectorStoreDocument {
	return &schema.VectorStoreDocument{
		DocumentID:      doc.ID,
		UID:             getLazyNodeUIDByMd5(s.collection.Name, doc.ID),
		DocumentType:    doc.Type,
		CollectionID:    s.collection.ID,
		CollectionUUID:  s.collection.UUID,
		Metadata:        schema.MetadataMap(doc.Metadata),
		Embedding:       schema.FloatArray(doc.Embedding),
		Content:         doc.Content,
		EntityID:        doc.EntityUUID,
		RelatedEntities: strings.Join(doc.RelatedEntities, ","),
		RuntimeID:       doc.RuntimeID,
	}
}

// DeleteEmbeddingData 删除嵌入数据
func (s *SQLiteVectorStoreHNSW) DeleteEmbeddingData() error {
	if !s.collection.EnablePQMode {
		return errors.New("collection is not in pq mode")
	}
	err := s.db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", s.collection.ID).Update("embedding", nil).Error
	if err != nil {
		return utils.Wrap(err, "delete embedding data")
	}
	return nil
}

// Add 添加文档到向量存储
func (s *SQLiteVectorStoreHNSW) Add(docs ...Document) error {
	// 记录锁获取时间
	helper := asynchelper.NewAsyncPerformanceHelper("Vector ADD")
	defer helper.Close()

	helper.MarkNow()
	s.mu.Lock()
	lockAcquireTime := helper.CheckLastMark1Second("lock acquire")
	defer s.mu.Unlock()

	totalStart := helper.MarkNow()
	docCount := len(docs)

	// 分析：当前使用单一大事务，可能导致长时间锁持有
	// 如果事务持续时间过长，建议考虑分批处理或更小的事务粒度
	defer func() {
		helper.CheckMarkAndLog(totalStart, 2*time.Second, fmt.Sprintf("total time(lock acquire: %v, %d docs)", lockAcquireTime, docCount))
	}()

	// 开始事务 - 这是潜在的性能瓶颈点
	helper.SetStatus("db transaction acquire")
	tx := s.db.Begin()
	txInitTime := helper.CheckLastMark1Second("db transaction acquire")
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recover from panic when adding docs: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// 记录文档处理时间
	var totalDbQueryTime, totalDocUpdateTime, totalDocCreateTime time.Duration
	var dbQueryCount, docUpdateCount, docCreateCount int

	for i, doc := range docs {
		helper.SetStatus(fmt.Sprintf("db query id :%s", doc.ID))
		docStart := helper.MarkNow()

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

		// 数据库查询时间 - 这是另一个潜在瓶颈
		queryStartTime := time.Now()
		existingDoc, err := yakit.GetRAGDocumentByCollectionIDAndKey(tx, s.collection.ID, doc.ID)
		queryTime := time.Since(queryStartTime)
		totalDbQueryTime += queryTime
		dbQueryCount++

		schemaDoc := s.toSchemaDocument(doc)
		if existingDoc != nil {
			// 更新现有文档
			updateStartTime := time.Now()
			existingDoc.Metadata = schemaDoc.Metadata
			existingDoc.Embedding = schemaDoc.Embedding
			existingDoc.Content = schemaDoc.Content
			existingDoc.EntityID = schemaDoc.EntityID
			existingDoc.RelatedEntities = schemaDoc.RelatedEntities
			if err := yakit.UpdateRAGDocument(tx, existingDoc); err != nil {
				tx.Rollback()
				return utils.Errorf("更新文档失败: %v", err)
			}
			updateTime := time.Since(updateStartTime)
			totalDocUpdateTime += updateTime
			docUpdateCount++
			helper.CheckMarkAndLog(docStart, time.Second, fmt.Sprintf("doc[%d] %s update (query: %v, update: %v)", i, doc.ID, queryTime, updateTime))
		} else if err == gorm.ErrRecordNotFound {
			// 创建新文档
			createStartTime := time.Now()
			if err := tx.Create(schemaDoc).Error; err != nil {
				tx.Rollback()
				return utils.Errorf("创建文档失败: %v", err)
			}
			createTime := time.Since(createStartTime)
			totalDocCreateTime += createTime
			docCreateCount++

			helper.CheckMarkAndLog(docStart, time.Second, fmt.Sprintf("doc[%d] %s update (query: %v, update: %v)", i, doc.ID, queryTime, createTime))

		} else {
			// 其他错误
			tx.Rollback()
			return utils.Errorf("查询文档失败: %v", err)
		}
	}

	// 记录节点创建时间
	nodeCreationStartTime := time.Now()
	nodes := make([]hnsw.InputNode[string], len(docs))
	for i, doc := range docs {
		helper.SetStatus(fmt.Sprintf("maker node id: %s", doc.ID))
		docVecCache := doc.Embedding
		nodes[i] = hnsw.MakeInputNodeFromID(doc.ID, hnswspec.LazyNodeID(doc.ID), func(uid hnswspec.LazyNodeID) ([]float32, error) {
			return docVecCache, nil
		})
	}
	nodeCreationTime := time.Since(nodeCreationStartTime)

	// 事务提交时间 - 提交可能成为瓶颈，特别是当事务很大时
	helper.SetStatus(fmt.Sprint("db transaction commit"))
	helper.MarkNow()
	err := tx.Commit().Error
	commitTime := helper.CheckLastMark1Second("db transaction commit")
	if err != nil {
		log.Errorf("transaction commit failed: %v", err)
		return err
	}
	transactionDuration := helper.CheckLastMarkAndLog(2*time.Second, "db transaction total")

	// HNSW 添加时间 - 这个操作不在事务中，但可能很耗时
	helper.SetStatus("hnsw add nodes")
	helper.MarkNow()
	s.hnsw.Add(nodes...)
	hnswAddTime := helper.CheckLastMark1Second("hnsw add nodes")

	// 记录详细性能指标
	totalTime := helper.CheckMarkAndLog(totalStart, 2*time.Second, "total time")

	// 计算平均指标
	var avgQueryTime, avgUpdateTime, avgCreateTime time.Duration
	if dbQueryCount > 0 {
		avgQueryTime = totalDbQueryTime / time.Duration(dbQueryCount)
	}
	if docUpdateCount > 0 {
		avgUpdateTime = totalDocUpdateTime / time.Duration(docUpdateCount)
	}
	if docCreateCount > 0 {
		avgCreateTime = totalDocCreateTime / time.Duration(docCreateCount)
	}

	helper.SetStatus("analyzing performance")
	// 性能警告条件 - 包含事务持续时间检查
	shouldWarn := totalTime > 5*time.Second ||
		totalDbQueryTime > time.Second ||
		hnswAddTime > time.Second ||
		transactionDuration > 10*time.Second || // 新增：事务持续时间警告
		(docCount > 10 && avgQueryTime > 500*time.Millisecond)

	if shouldWarn {
		log.Warnf("HNSW Add performance breakdown - Total: %v (%d docs), LockAcquire: %v, TxInit: %v, TransactionDuration: %v, NodeCreation: %v, TxCommit: %v, HNSW: %v",
			totalTime, docCount, lockAcquireTime, txInitTime, transactionDuration, nodeCreationTime, commitTime, hnswAddTime)

		log.Warnf("Database operations summary - TotalDocs: %d, Queries: %d (total: %v, avg: %v), Updates: %d (total: %v, avg: %v), Creates: %d (total: %v, avg: %v)",
			docCount, dbQueryCount, totalDbQueryTime, avgQueryTime, docUpdateCount, totalDocUpdateTime, avgUpdateTime, docCreateCount, totalDocCreateTime, avgCreateTime)

		// 记录性能诊断信息
		s._logPerformanceDiagnosticsNeedLock()

		// 分析可能的性能瓶颈
		if transactionDuration > 30*time.Second {
			log.Warnf("CRITICAL: TRANSACTION DURATION TOO LONG: %v for %d documents - THIS MAY CAUSE SYSTEM-WIDE PERFORMANCE ISSUES", transactionDuration, docCount)
			log.Warnf("RECOMMENDATION: Consider breaking large document batches into smaller chunks or using separate transactions per document")
		}
		if transactionDuration > 60*time.Second {
			log.Errorf("SEVERE: Transaction lasted over 1 minute (%v) - likely blocking other database operations", transactionDuration)
		}
		if avgQueryTime > time.Second {
			log.Warnf("DATABASE QUERY SLOW: average query time %v - possible index or database performance issue", avgQueryTime)
		}
		if hnswAddTime > 5*time.Second {
			log.Warnf("HNSW INDEXING SLOW: %v for %d nodes - possible HNSW algorithm bottleneck", hnswAddTime, docCount)
			log.Warnf("HNSW BOTTLENECK ANALYSIS: Check if collection has too many documents for current M/EfSearch parameters")
		}
		if lockAcquireTime > time.Second {
			log.Warnf("LOCK CONTENTION: lock acquire took %v - possible concurrent access bottleneck", lockAcquireTime)
		}

		// 新增：事务效率分析
		transactionEfficiency := float64(totalDbQueryTime+totalDocUpdateTime+totalDocCreateTime) / float64(transactionDuration)
		if transactionDuration > 5*time.Second && transactionEfficiency < 0.5 {
			log.Warnf("TRANSACTION INEFFICIENCY: Only %.1f%% of transaction time was spent on actual database operations", transactionEfficiency*100)
			log.Warnf("ROOT CAUSE ANALYSIS: The bottleneck is likely in HNSW indexing, not database operations")
		}
	} else {
		// 即使不警告，也记录基本统计信息用于监控
		log.Debugf("HNSW Add completed - Total: %v (%d docs), TxDuration: %v, DB: %v, HNSW: %v",
			totalTime, docCount, transactionDuration, totalDbQueryTime+totalDocUpdateTime+totalDocCreateTime, hnswAddTime)
	}

	return nil
}

func (s *SQLiteVectorStoreHNSW) FuzzSearch(ctx context.Context, query string, limit int) (<-chan SearchResult, error) {
	filter := &yakit.VectorDocumentFilter{
		CollectionUUID: s.collection.UUID,
		Keywords:       query,
	}
	var results = chanx.NewUnlimitedChan[SearchResult](ctx, 100)
	go func() {
		defer results.Close()
		for doc := range yakit.YieldVectorDocument(ctx, s.db, filter, bizhelper.WithYieldModel_Limit(limit)) {
			results.SafeFeed(SearchResult{
				Document: s.toDocument(doc),
				Score:    0,
			})
		}
	}()
	return results.OutputChannel(), nil
}

// Search 根据查询文本检索相关文档
func (s *SQLiteVectorStoreHNSW) Search(query string, page, limit int) ([]SearchResult, error) {
	return s.SearchWithFilter(query, page, limit, nil)
}

// SearchWithFilter 根据查询文本检索相关文档，并根据过滤函数过滤结果
func (s *SQLiteVectorStoreHNSW) SearchWithFilter(query string, page, limit int, filter func(key string, getDoc func() *Document) bool) ([]SearchResult, error) {
	queryEmbedding, err := s.embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("generate embedding vector for %#v: %v", query, err)
	}

	pageSize := 10
	s.mu.RLock()
	defer s.mu.RUnlock()

	resultNodes := s.hnsw.SearchWithDistanceAndFilter(queryEmbedding, (page-1)*pageSize+limit, func(key string, vector hnsw.Vector) bool {
		if key == DocumentTypeCollectionInfo {
			return false
		}
		if filter != nil {
			return filter(key, func() *Document {
				doc, err := yakit.GetRAGDocumentByID(s.db, s.collection.Name, key)
				if err != nil {
					return nil
				}
				if doc == nil {
					return nil
				}
				res := s.toDocument(doc)
				return &res
			})
		}
		return true
	})

	resultIds := make([]string, len(resultNodes))
	for i, resultNode := range resultNodes {
		resultIds[i] = resultNode.Key
	}
	//log.Infof("hnsw search returned %d candidate documents", len(resultNodes))

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

		err := s.db.Where("collection_id = ? AND document_id IN (?)", s.collection.ID, batchIds).Find(&batchDocs).Error
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

	//log.Infof("calculated similarity scores for %d documents", len(results))

	// 按相似度分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	//log.Infof("sorted results by similarity score")

	if page < 1 {
		page = 1
	}
	if len(results) == 0 {
		return []SearchResult{}, nil
	}
	// 计算分页
	offset := (page - 1) * pageSize
	if offset >= len(results) {
		//log.Infof("page offset %d exceeds total results %d, returning empty", offset, len(results))
		return []SearchResult{}, nil
	}
	if offset+limit > len(results) {
		limit = len(results) - offset
	}
	results = results[offset : offset+limit]
	//log.Infof("returning %d results after pagination (offset: %d)", len(results), offset)
	return results, nil
}

// Delete 根据 ID 删除文档
func (s *SQLiteVectorStoreHNSW) Delete(ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range ids {
		s.hnsw.Delete(id)
	}

	utils.GormTransactionReturnDb(s.db, func(tx *gorm.DB) {
		for _, id := range ids {
			if err := tx.Where("document_id = ?", id).Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
				log.Errorf("删除文档 %s 失败: %v", id, err)
			}
		}
	})

	return nil
}

// Get 根据 ID 获取文档
func (s *SQLiteVectorStoreHNSW) Get(id string) (Document, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, err := yakit.GetRAGDocumentByID(s.db, s.collection.Name, id)
	if err != nil {
		return Document{}, false, utils.Errorf("查询文档失败: %v", err)
	}
	if doc == nil {
		return Document{}, false, nil
	}

	return s.toDocument(doc), true, nil
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

func (s *SQLiteVectorStoreHNSW) UnSafeCount() (int, error) {
	var count int
	if err := s.db.Model(&schema.VectorStoreDocument{}).Where("collection_id = ?", s.collection.ID).Where("document_id <> ?", DocumentTypeCollectionInfo).Count(&count).Error; err != nil {
		return 0, utils.Errorf("计算文档数量失败: %v", err)
	}

	return count, nil
}

func (s *SQLiteVectorStoreHNSW) PerformanceDiagnostics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._performanceDiagnosticsNeedLock()
}

// _performanceDiagnosticsNeedLock 返回性能诊断信息 (需要外部锁)
func (s *SQLiteVectorStoreHNSW) _performanceDiagnosticsNeedLock() map[string]interface{} {
	diagnostics := make(map[string]interface{})

	// 基本集合信息
	diagnostics["collection_name"] = s.collection.Name
	diagnostics["collection_id"] = s.collection.ID
	diagnostics["model_name"] = s.collection.ModelName
	diagnostics["dimension"] = s.collection.Dimension

	// HNSW配置
	diagnostics["hnsw_m"] = s.collection.M
	diagnostics["hnsw_ml"] = s.collection.Ml
	diagnostics["hnsw_ef_search"] = s.collection.EfSearch
	diagnostics["hnsw_ef_construct"] = s.collection.EfConstruct
	diagnostics["hnsw_distance_func"] = s.collection.DistanceFuncType
	diagnostics["hnsw_enable_pq"] = s.collection.EnablePQMode

	// 文档统计
	docCount, err := s.UnSafeCount()
	if err != nil {
		diagnostics["document_count_error"] = err.Error()
	} else {
		diagnostics["document_count"] = docCount
	}

	// HNSW图状态
	if s.hnsw != nil {
		diagnostics["hnsw_layers_count"] = len(s.hnsw.Layers)
		totalNodes := 0
		for i, layer := range s.hnsw.Layers {
			nodesInLayer := len(layer.Nodes)
			diagnostics[fmt.Sprintf("layer_%d_nodes", i)] = nodesInLayer
			totalNodes += nodesInLayer
		}
		diagnostics["hnsw_total_nodes"] = totalNodes

		// 计算理论复杂度
		if docCount > 0 {
			diagnostics["estimated_search_complexity"] = fmt.Sprintf("O(%d * %d)", s.collection.EfSearch, docCount)
			diagnostics["estimated_construction_complexity"] = fmt.Sprintf("O(%d * %d * %d)", s.collection.M, s.collection.EfConstruct, docCount)
		}
	} else {
		diagnostics["hnsw_status"] = "not_initialized"
	}

	return diagnostics
}

func (s *SQLiteVectorStoreHNSW) LogPerformanceDiagnostics() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s._logPerformanceDiagnosticsNeedLock()
}

// _logPerformanceDiagnosticsNeedLock 记录性能诊断信息
func (s *SQLiteVectorStoreHNSW) _logPerformanceDiagnosticsNeedLock() {
	diagnostics := s._performanceDiagnosticsNeedLock()

	log.Infof("=== HNSW Performance Diagnostics for Collection: %s ===", diagnostics["collection_name"])
	log.Infof("Documents: %v", diagnostics["document_count"])
	log.Infof("HNSW Config - M:%v, ML:%v, EfSearch:%v, EfConstruct:%v, Distance:%v, PQ:%v",
		diagnostics["hnsw_m"], diagnostics["hnsw_ml"], diagnostics["hnsw_ef_search"],
		diagnostics["hnsw_ef_construct"], diagnostics["hnsw_distance_func"], diagnostics["hnsw_enable_pq"])
	log.Infof("HNSW Status - Layers:%v, TotalNodes:%v", diagnostics["hnsw_layers_count"], diagnostics["hnsw_total_nodes"])

	if complexity, ok := diagnostics["estimated_search_complexity"]; ok {
		log.Infof("Estimated Complexity - Search:%v, Construction:%v", complexity, diagnostics["estimated_construction_complexity"])
	}

	// 性能建议
	docCount := 0
	if count, ok := diagnostics["document_count"]; ok {
		docCount = count.(int)
	}

	if docCount > 10000 {
		log.Warnf("PERFORMANCE WARNING: Collection has %d documents - consider increasing M and EfSearch parameters", docCount)
	}
	if docCount > 50000 {
		log.Errorf("CRITICAL PERFORMANCE: Collection has %d documents - HNSW performance will degrade significantly", docCount)
	}
}

// 确保 SQLiteVectorStoreHNSW 实现了 VectorStore 接口
var _ VectorStore = (*SQLiteVectorStoreHNSW)(nil)
