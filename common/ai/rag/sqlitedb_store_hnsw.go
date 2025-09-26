package rag

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/ai/rag/config"
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
	buildGraphFilter           *yakit.VectorDocumentFilter
	buildGraphPolicy           string
	ctx                        context.Context
	wg                         sync.WaitGroup
	UIDType                    string

	opts []SQLiteVectorStoreHNSWOption
}

const (
	Policy_UseDBCanche = "DB_Cache"
	Policy_UseFilter   = "Filter"
	Policy_None        = "None"
)

type SQLiteVectorStoreHNSWOption func(*SQLiteVectorStoreHNSW)

func WithBuildGraphPolicy(policy string) SQLiteVectorStoreHNSWOption {
	return func(s *SQLiteVectorStoreHNSW) {
		s.buildGraphPolicy = policy
	}
}

func WithBuildGraphFilter(filter *yakit.VectorDocumentFilter) SQLiteVectorStoreHNSWOption {
	return func(s *SQLiteVectorStoreHNSW) {
		s.buildGraphFilter = filter
	}
}

func WithEnableAutoUpdateGraphInfos(enable bool) SQLiteVectorStoreHNSWOption {
	return func(s *SQLiteVectorStoreHNSW) {
		s.EnableAutoUpdateGraphInfos = enable
	}
}

func WithContext(ctx context.Context) SQLiteVectorStoreHNSWOption {
	return func(s *SQLiteVectorStoreHNSW) {
		s.ctx = ctx
	}
}

func LoadSQLiteVectorStoreHNSW(db *gorm.DB, collectionName string, embedder EmbeddingClient, opts ...SQLiteVectorStoreHNSWOption) (*SQLiteVectorStoreHNSW, error) {
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	if err != nil {
		return nil, utils.Errorf("query rag collection [%#v] err: %v", collectionName, err)
	}

	if collection == nil {
		return nil, utils.Errorf("rag collection[%v] not existed", collectionName)
	}

	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: true,
		embedder:                   embedder,
		collection:                 collection,
		buildGraphPolicy:           Policy_UseDBCanche,
		ctx:                        context.Background(),
		opts:                       opts,
	}

	for _, opt := range opts {
		opt(vectorStore)
	}

	collectionConfig := collection
	hnswGraph := NewHNSWGraph(collectionName,
		hnsw.WithHNSWParameters[string](collectionConfig.M, collectionConfig.Ml, collectionConfig.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc(collectionConfig.DistanceFuncType)),
	)

	log.Infof("start to recover hnsw graph from db, collection name: %s", collectionName)
	switch vectorStore.buildGraphPolicy {
	case Policy_UseFilter: // 选择性加载子图
		vectorStore.buildGraphFilter.CollectionUUID = collection.UUID
		log.Info("build graph with filter policy, load existed vectors from db with filter")
		for document := range yakit.YieldVectorDocument(vectorStore.ctx, db, vectorStore.buildGraphFilter) {
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
			hnswGraph, err = ParseHNSWGraphFromBinary(vectorStore.ctx, collectionName, graphBinaryReader, db, 1000, 1200, collection.EnablePQMode, &vectorStore.wg)
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
	vectorStore.hnsw = hnswGraph

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

// NewSQLiteVectorStore 创建一个新的 SQLite 向量存储
func NewSQLiteVectorStoreHNSW(name string, description string, modelName string, dimension int, embedder EmbeddingClient, db *gorm.DB, options ...config.SQLiteVectorStoreHNSWOption) (*SQLiteVectorStoreHNSW, error) {
	cfg := config.NewSQLiteVectorStoreHNSWConfig()
	for _, option := range options {
		option(cfg)
	}

	vcolsNum := 0
	db.Where("name = ?", name).Count(&vcolsNum)
	var collection *schema.VectorStoreCollection
	var err error
	hnswGraph := NewHNSWGraph(name)
	if vcolsNum == 0 {
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
			EnablePQMode:     cfg.EnablePQMode,
		}
		if err := db.Create(&collection).Error; err != nil {
			return nil, utils.Errorf("创建集合失败: %v", err)
		}
	} else {
		collection, err = yakit.QueryRAGCollectionByName(db, name)
		if err != nil {
			return nil, utils.Errorf("查询集合失败: %v", err)
		}
	}

	vectorStore := &SQLiteVectorStoreHNSW{
		db:                         db,
		EnableAutoUpdateGraphInfos: true,
		embedder:                   embedder,
		collection:                 collection,
		hnsw:                       hnswGraph,
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
	s.mu.Lock()
	defer s.mu.Unlock()
	addStartTime := time.Now()
	defer func() {
		usedTime := time.Since(addStartTime)
		if usedTime > 2*time.Second {
			log.Warnf("adding docs took too long: %v", usedTime)
		} else {
			log.Debugf("adding docs took: %v", usedTime)
		}
	}()

	// 开始事务
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recover from panic when adding docs: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
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

		existingDoc, err := yakit.GetRAGDocumentByCollectionIDAndKey(tx, s.collection.ID, doc.ID)
		schemaDoc := s.toSchemaDocument(doc)
		if existingDoc != nil {
			// 更新现有文档
			existingDoc.Metadata = schemaDoc.Metadata
			existingDoc.Embedding = schemaDoc.Embedding
			existingDoc.Content = schemaDoc.Content
			existingDoc.EntityID = schemaDoc.EntityID
			existingDoc.RelatedEntities = schemaDoc.RelatedEntities
			if err := yakit.UpdateRAGDocument(tx, existingDoc); err != nil {
				tx.Rollback()
				return utils.Errorf("更新文档失败: %v", err)
			}
		} else if err == gorm.ErrRecordNotFound {
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
		nodes[i] = hnsw.MakeInputNodeFromID(doc.ID, hnswspec.LazyNodeID(getLazyNodeUIDByMd5(s.collection.Name, doc.ID)), func(uid hnswspec.LazyNodeID) ([]float32, error) {
			dbDoc, err := getVectorDocumentByLazyNodeID(s.db, uid)
			if err != nil {
				return nil, utils.Wrap(err, "get vector document by lazy node uid")
			}
			return dbDoc.Embedding, nil
		})
	}
	err := tx.Commit().Error
	if err != nil {
		return err
	}

	s.hnsw.Add(nodes...)

	// 提交事务
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
	//log.Infof("start to search with query: %s", query)
	// 生成查询的嵌入向量
	//log.Infof("generated query embedding with dimension: %d", len(queryEmbedding))
	queryEmbedding, err := s.embedder.Embedding(query)
	if err != nil {
		return nil, utils.Errorf("generate embedding vector for %#v: %v", query, err)
	}

	startSearch := time.Now()
	defer func() {
		useTime := time.Since(startSearch)
		if useTime > 2*time.Second {
			log.Warnf("just search without embedding took too long: %v with [%s]", useTime, query)
		} else {
			log.Debugf("search took: %v", useTime)
		}
	}()

	pageSize := 10
	s.mu.RLock()
	defer s.mu.RUnlock()
	//log.Infof("starting search for query with length: %d, page: %d, limit: %d", len(query), page, limit)

	nodesFilterStart := time.Now()

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
	nodesFilterUseTime := time.Since(nodesFilterStart)
	if nodesFilterUseTime > 2*time.Second {
		log.Warnf("nodes filter took too long: %v with query[%s]", nodesFilterUseTime, query)
	} else {
		log.Debugf("nodes filter took: %v", nodesFilterUseTime)
	}

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

// 确保 SQLiteVectorStoreHNSW 实现了 VectorStore 接口
var _ VectorStore = (*SQLiteVectorStoreHNSW)(nil)
