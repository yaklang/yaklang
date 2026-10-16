package vectorstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SQLiteVectorStore 是一个基于 SQLite 的向量存储实现
type SQLiteVectorStoreHNSW struct {
	// config and meta information
	config     *CollectionConfig
	collection *schema.VectorStoreCollection

	// vector embedder and graph alg
	embedder EmbeddingClient
	hnsw     *GraphWrapper[string]

	db *gorm.DB

	mu sync.RWMutex // 用于并发安全的互斥锁
}

const (
	Policy_UseDBCanche = "DB_Cache"
	Policy_None        = "None"
)

func initSQLiteVectorStoreHNSW() *SQLiteVectorStoreHNSW { // 初始化 SQLiteVectorStoreHNSW 结构体
	return &SQLiteVectorStoreHNSW{
		mu: sync.RWMutex{},
	}
}

func LoadSQLiteVectorStoreHNSW(db *gorm.DB, collectionName string, opts ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	started := time.Now()
	var metadataCost, embeddingCost, graphCost, countCost, dimensionCost time.Duration
	defer func() {
		if total := time.Since(started); total > time.Second {
			log.Warnf("RAG collection %q load took %s (metadata=%s embedding=%s graph=%s count=%s dimensions=%s)",
				collectionName, total, metadataCost, embeddingCost, graphCost, countCost, dimensionCost)
		}
	}()
	stage := time.Now()
	collection, err := yakit.QueryRAGCollectionByName(db, collectionName)
	metadataCost = time.Since(stage)
	if err != nil {
		return nil, utils.Wrap(err, fmt.Sprintf("query rag collection [%#v]", collectionName))
	}

	if collection == nil {
		return nil, utils.Errorf("rag collection[%v] not existed", collectionName)
	}

	collectionConfig := LoadConfigFromCollectionInfo(collection, opts...)

	stage = time.Now()
	err = collectionConfig.FixEmbeddingClient()
	embeddingCost = time.Since(stage)
	if err != nil {
		return nil, utils.Errorf("fix embedding client err: %v", err)
	}
	vectorStore := initSQLiteVectorStoreHNSW()
	vectorStore.db = db
	vectorStore.config = collectionConfig
	vectorStore.embedder = collectionConfig.EmbeddingClient
	vectorStore.collection = collection
	stage = time.Now()
	graphWrapper, err := GraphWrapperManager.GetGraphWrapper(db, collection, collectionConfig)
	graphCost = time.Since(stage)
	if err != nil {
		log.Errorf("get graph wrapper err: %v", err)
		return nil, utils.Wrap(err, "get graph wrapper")
	}
	vectorStore.hnsw = graphWrapper

	stage = time.Now()
	docCount, err := vectorStore.Count()
	countCost = time.Since(stage)
	if err != nil {
		return nil, utils.Wrap(err, "count documents")
	}
	if docCount > 0 {
		stage = time.Now()
		dims := graphWrapper.graph.Dims()
		dimensionCost = time.Since(stage)
		if dims != collectionConfig.Dimension {
			return nil, utils.Errorf("dimension mismatch: %d != %d, collection name: %s", dims, collectionConfig.Dimension, collectionName)
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

func (s *SQLiteVectorStoreHNSW) GetCollectionInfo() *schema.VectorStoreCollection {
	return s.collection
}

func (s *SQLiteVectorStoreHNSW) GetEmbedder() EmbeddingClient {
	if s.embedder == nil {
		s.config.LazyLoadEmbeddingClient = false
		err := s.config.FixEmbeddingClient()
		if err != nil {
			log.Errorf("fix embedding client err: %v", err)
		}
		s.embedder = s.config.EmbeddingClient
	}
	return s.embedder
}

func pqSubspaceCount(dim int) int {
	if dim <= 0 {
		return 1
	}
	for _, subspaces := range []int{32, 16, 8, 4, 2, 1} {
		if subspaces == 32 && dim < 512 {
			continue
		}
		if dim%subspaces == 0 {
			return subspaces
		}
	}
	return 1
}

func (s *SQLiteVectorStoreHNSW) ConvertToPQMode() error {
	if err := s.requireWriteCollection(); err != nil {
		return err
	}
	done := make(chan struct{})
	var err error
	if !s.hnsw.submit(&graphOp{opType: opTypeWrite, desc: "ConvertToPQMode", fn: func() {
		defer close(done)
		graph := s.hnsw.graph
		originalLayers, originalBook, originalQuantizer := graph.Layers, graph.GetCodebook(), graph.GetPQQuantizer()
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("convert graph to PQ: %v", recovered)
			}
			if err != nil {
				graph.Layers = originalLayers
				graph.SetPQCodebook(originalBook)
				graph.SetPQQuantizer(originalQuantizer)
			}
		}()
		codes := make(map[string][]byte, graph.Len())
		_, err = graph.TrainPQCodebookFromDataWithCallback(pqSubspaceCount(s.collection.Dimension), min(256, graph.Len()),
			func(key string, code []byte, _ []float64) (hnswspec.LayerNode[string], error) {
				codes[key] = code
				return hnswspec.NewRawPQLayerNode(key, code), nil
			})
		if err != nil {
			return
		}
		// Serialize lazy nodes before reserving a database connection. Commit the
		// codebook, mode, codes and snapshot together, including pending deltas.
		var updates map[string]interface{}
		updates, err = graphDatabaseUpdates(s.hnsw)
		if err != nil {
			return
		}
		updates["enable_pq_mode"] = true
		err = utils.GormTransaction(s.db, func(tx *gorm.DB) error {
			for key, code := range codes {
				result := tx.Model(&schema.VectorStoreDocument{}).Where("collection_id = ? AND document_id = ?", s.collection.ID, key).Update("pq_code", code)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					return utils.Errorf("PQ document %s was deleted", key)
				}
			}
			if err := writeGraphDatabaseUpdates(tx, s.collection.UUID, updates); err != nil {
				return err
			}
			return clearGraphDeltas(tx, s.collection.UUID, 0)
		})
		if err == nil {
			s.collection.EnablePQMode = true
			s.collection.GraphBinary = updates["graph_binary"].([]byte)
			s.collection.CodeBookBinary = updates["code_book_binary"].([]byte)
		}
	}}) {
		return errors.New("graph wrapper is closed")
	}
	<-done
	return err
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
	pqQuantizer := s.hnsw.GetQuantizer()
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

// collectionCreateLock 用于保护集合创建操作的并发安全
var collectionCreateLock sync.Mutex

func CreateCollectionRecord(db *gorm.DB, name string, description string, opts ...CollectionConfigFunc) (*schema.VectorStoreCollection, error) {
	cfg := NewCollectionConfig(opts...)

	if cfg.Description != "" {
		description = cfg.Description
	}

	// 使用锁保护创建操作，避免并发时的唯一性约束冲突
	collectionCreateLock.Lock()
	defer collectionCreateLock.Unlock()

	// 先尝试查询是否已存在（使用 Unscoped 包含软删除的记录，避免唯一索引冲突）
	var existingCollection schema.VectorStoreCollection
	err := db.Unscoped().Model(&schema.VectorStoreCollection{}).Where("name = ?", name).First(&existingCollection).Error
	if err == nil {
		// 记录已存在
		if existingCollection.DeletedAt != nil {
			// 如果是软删除的记录，恢复它
			if updateErr := db.Unscoped().Model(&existingCollection).Update("deleted_at", nil).Error; updateErr != nil {
				return nil, utils.Errorf("恢复已删除的集合失败: %v", updateErr)
			}
		}
		return &existingCollection, nil
	}
	if !gorm.IsRecordNotFoundError(err) {
		return nil, utils.Errorf("查询集合失败: %v", err)
	}

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

	return &collection, nil
}

func NewSQLiteVectorStoreHNSWEx(db *gorm.DB, name string, description string, opts ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	cfg := NewCollectionConfig(opts...)

	collection, err := CreateCollectionRecord(db, name, description, opts...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create collection record")
	}

	if err := cfg.FixEmbeddingClient(); err != nil {
		return nil, utils.Errorf("fix embedding client err: %v", err)
	}
	vectorStore := initSQLiteVectorStoreHNSW()
	vectorStore.db = db
	vectorStore.config = cfg
	vectorStore.embedder = cfg.EmbeddingClient
	vectorStore.collection = collection

	graphWrapper, err := GraphWrapperManager.GetGraphWrapper(db, collection, cfg)
	if err != nil {
		log.Errorf("get graph wrapper err: %v", err)
		return nil, err
	}
	vectorStore.hnsw = graphWrapper
	return vectorStore, nil
}

// NewSQLiteVectorStore 创建一个新的 SQLite 向量存储
func NewSQLiteVectorStoreHNSW(name string, description string, modelName string, dimension int, embedder EmbeddingClient, db *gorm.DB, options ...CollectionConfigFunc) (*SQLiteVectorStoreHNSW, error) {
	options = append(options, WithModelName(modelName))
	options = append(options, WithModelDimension(dimension))
	options = append(options, WithEmbeddingClient(embedder))
	return NewSQLiteVectorStoreHNSWEx(db, name, description, options...)
}

func (s *SQLiteVectorStoreHNSW) Remove() error {
	return DeleteCollection(s.db, s.collection.Name)
}

// 将 schema.VectorStoreDocument 转换为 Document
func (s *SQLiteVectorStoreHNSW) toDocument(doc *schema.VectorStoreDocument) *Document {
	return &Document{
		ID:              doc.DocumentID,
		Type:            doc.DocumentType,
		Metadata:        map[string]any(doc.Metadata),
		Embedding:       []float32(doc.Embedding),
		Content:         doc.Content,
		EntityUUID:      doc.EntityID,
		RelatedEntities: utils.PrettifyListFromStringSplitEx(doc.RelatedEntities, ",", "|"),
		RuntimeID:       doc.RuntimeID,
		UID:             doc.UID,
	}
}

// 将 Document 转换为 schema.VectorStoreDocument
func (s *SQLiteVectorStoreHNSW) toSchemaDocument(doc *Document) *schema.VectorStoreDocument {
	return &schema.VectorStoreDocument{
		DocumentID:      doc.ID,
		UID:             GetLazyNodeUIDByMd5(s.collection.Name, doc.ID),
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

func (s *SQLiteVectorStoreHNSW) Has(docId string) bool {
	return s.hnsw.Has(docId)
}

func (s *SQLiteVectorStoreHNSW) requireWriteCollection() error {
	var collection schema.VectorStoreCollection
	err := s.db.Model(&schema.VectorStoreCollection{}).
		Select("id, archived").
		Where("id = ?", s.collection.ID).
		First(&collection).Error
	if err != nil {
		return utils.Wrap(err, "current vector store is unavailable")
	}
	if collection.Archived {
		return utils.Errorf("current vector store is archived, please unarchive first")
	}
	return nil
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

func (s *SQLiteVectorStoreHNSW) AddWithOptions(docId, content string, opts ...DocumentOption) error {
	doc := &Document{
		ID:        docId,
		Content:   content,
		Metadata:  make(map[string]any),
		Embedding: nil,
	}
	for _, opt := range opts {
		opt(doc)
	}
	return s.Add(doc)
}

func (s *SQLiteVectorStoreHNSW) embedDocuments(docs ...*Document) ([]*Document, error) {
	s.embedder = s.GetEmbedder()
	var finalDocs []*Document

	// 为每个文档生成嵌入向量
	for i := range docs {
		if len(docs[i].Content) == 0 {
			log.Errorf("document %s has no content", docs[i].ID)
		}
		//log.Infof("generating embedding for document %s (index %d)", docs[i].ID, i)
		// 首先尝试直接生成嵌入
		embeddingData, err := s.embedder.Embedding(docs[i].Content)
		if err != nil {
			if errors.Is(err, embedding.ErrInputTooLarge) {
				// 如果失败且是由于文本过大，使用BigTextPlan处理
				processedDocs, processErr := processBigText(s.embedder, docs[i], s.config.MaxChunkSize, s.config.Overlap, s.config.BigTextPlan)
				if processErr != nil {
					log.Errorf("failed to process big text for document %s: %v", docs[i].ID, processErr)
					return nil, utils.Errorf("failed to process document %s: %v", docs[i].ID, processErr)
				}

				// 将处理后的文档添加到最终文档列表
				finalDocs = append(finalDocs, processedDocs...)
				continue
			}
			log.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
			return nil, utils.Errorf("failed to generate embedding for document %s: %v", docs[i].ID, err)
		}

		if len(embeddingData) <= 0 {
			log.Errorf("empty embedding generated for document %s", docs[i].ID)
			return nil, utils.Errorf("failed to generate embedding for document (empty embedding) %s", docs[i].ID)
		}

		//log.Infof("successfully generated embedding for document %s, dimension: %d", docs[i].ID, len(embeddingData))
		docs[i].Embedding = embeddingData
		finalDocs = append(finalDocs, docs[i])
	}
	return finalDocs, nil
}

// Add 添加文档到向量存储
func (s *SQLiteVectorStoreHNSW) Add(docs ...*Document) error {
	if err := s.requireWriteCollection(); err != nil {
		return utils.Wrap(err, "require write vector store")
	}
	for _, doc := range docs {
		if doc == nil || doc.ID == "" {
			return utils.Error("document must have an ID")
		}
	}
	docs, err := s.embedDocuments(docs...)
	if err != nil {
		return utils.Wrap(err, "embed documents")
	}
	if len(docs) == 0 {
		return nil
	}
	nodes := make([]hnsw.InputNode[string], len(docs))
	for i, doc := range docs {
		if len(doc.Embedding) == 0 {
			return utils.Errorf("document %s must have an embedding", doc.ID)
		}
		vector := doc.Embedding
		nodes[i] = hnsw.MakeInputNodeFromID(doc.ID, hnswspec.LazyNodeID(doc.ID), func(hnswspec.LazyNodeID) ([]float32, error) { return vector, nil })
	}
	started := time.Now()
	// Serialize graph changes first, then open a short transaction. Documents
	// and the adjacency delta become visible together; a failed commit restores
	// only the neighborhoods touched by this insertion, including replacements.
	duration, err := s.hnsw.addWithCommit(nodes, func() error {
		return utils.GormTransaction(s.db, func(tx *gorm.DB) error {
			var collection schema.VectorStoreCollection
			if err := tx.Select("id, archived").First(&collection, s.collection.ID).Error; err != nil {
				return utils.Wrap(err, "current vector store is unavailable")
			}
			if collection.Archived {
				return utils.Error("current vector store is archived")
			}
			for _, doc := range docs {
				schemaDoc := s.toSchemaDocument(doc)
				if s.hnsw.graph.IsPQEnabled() {
					vector := make([]float64, len(doc.Embedding))
					for i, value := range doc.Embedding {
						vector[i] = float64(value)
					}
					codes, err := s.hnsw.graph.GetPQQuantizer().Encode(vector)
					if err != nil {
						return err
					}
					schemaDoc.PQCode = codes
				}
				existing, err := yakit.GetRAGDocumentByCollectionIDAndKey(tx, s.collection.ID, doc.ID)
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := tx.Create(schemaDoc).Error; err != nil {
						return utils.Wrap(err, "create vector document")
					}
				} else if err != nil {
					return utils.Wrap(err, "query vector document")
				} else {
					existing.Metadata, existing.Embedding, existing.Content = schemaDoc.Metadata, schemaDoc.Embedding, schemaDoc.Content
					existing.EntityID, existing.RelatedEntities = schemaDoc.EntityID, schemaDoc.RelatedEntities
					existing.PQCode, existing.DocumentType = schemaDoc.PQCode, schemaDoc.DocumentType
					if err := yakit.UpdateRAGDocument(tx, existing); err != nil {
						return utils.Wrap(err, "update vector document")
					}
				}
			}
			if s.hnsw.saveGraphDelta != nil {
				return s.hnsw.saveGraphDelta(tx)
			}
			return nil
		})
	})
	if err != nil {
		return utils.Wrap(err, "add graph and documents")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		log.Warnf("HNSW Add took %s for %d documents (graph and transaction: %s)", elapsed, len(docs), duration)
	}
	return nil
}
func (s *SQLiteVectorStoreHNSW) FuzzSearch(ctx context.Context, query string, limit int) (<-chan SearchResult, error) {
	filter := &yakit.VectorDocumentFilter{
		CollectionUUID: s.collection.UUID,
		Keywords:       []string{query},
	}
	var results = chanx.NewUnlimitedChan[SearchResult](ctx, 100)
	go func() {
		defer results.Close()
		for doc := range yakit.SearchVectorStoreDocumentBM25Yield(ctx, s.db, filter, bizhelper.WithYieldModel_Limit(limit)) {
			results.SafeFeed(SearchResult{
				Document: s.toDocument(doc),
				Score:    0,
			})
		}
	}()
	return results.OutputChannel(), nil
}

// RestoreDocumentGraph repairs an existing document's graph node using its
// durable embedding. It never asks the embedding provider to regenerate it.
func (s *SQLiteVectorStoreHNSW) RestoreDocumentGraph(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.requireWriteCollection(); err != nil {
		return err
	}
	doc, found, err := s.Get(id)
	if err != nil {
		return err
	}
	if !found || len(doc.Embedding) == 0 || len(doc.Embedding) != s.collection.Dimension {
		return fmt.Errorf("document %s has no valid persisted embedding", id)
	}
	node := hnsw.MakeInputNodeFromID(id, hnswspec.LazyNodeID(id), func(hnswspec.LazyNodeID) ([]float32, error) {
		return doc.Embedding, nil
	})
	if _, err := s.hnsw.AddWithError(node); err != nil {
		return err
	}
	return s.PersistGraph(ctx)
}

// PersistGraph is an explicit durable barrier for callers maintaining storage
// receipts. Automatic layer-change callbacks only log persistence failures.
// Queue with graph writes so export and persistence see a coherent snapshot.
func (s *SQLiteVectorStoreHNSW) PersistGraph(ctx context.Context) error {
	done := make(chan struct{})
	var err error
	if !s.hnsw.submit(&graphOp{opType: opTypeWrite, desc: "PersistGraph", fn: func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("persist graph: %v", recovered)
			}
		}()
		if err = ctx.Err(); err != nil {
			return
		}
		if err = s.requireWriteCollection(); err != nil {
			return
		}
		if s.hnsw.persistLocked != nil {
			err = s.hnsw.persistLocked(true)
		} else {
			err = updateDatabaseGraphInfoInLock(s.db, s.collection.UUID, s.hnsw)
		}
	}}) {
		return errors.New("graph wrapper is closed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
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
	resultNodes := s.hnsw.SearchWithDistanceAndFilter(queryEmbedding, (page-1)*pageSize+limit, func(key string, vector hnsw.Vector) bool {
		if key == DocumentTypeCollectionInfo {
			return false
		}
		if filter != nil {
			return filter(key, func() *Document {
				doc, err := yakit.GetRAGDocumentByCollectionIDAndKey(s.db, s.collection.ID, key)
				if err != nil {
					return nil
				}
				if doc == nil {
					return nil
				}
				res := s.toDocument(doc)
				return res
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
	return s.DeleteWithTransactionCheck(nil, ids...)
}

// DeleteWithTransactionCheck checks a conditional deletion before graph work,
// then revalidates inside the transaction that removes rows and saves the graph.
// A failed transaction restores the graph too. check must be read-only, use
// the supplied DB, and never re-enter the store or graph.
func (s *SQLiteVectorStoreHNSW) DeleteWithTransactionCheck(check func(tx *gorm.DB) error, ids ...string) error {
	if err := s.requireWriteCollection(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		return nil
	}

	// Small graphs retain synchronous snapshot/rollback behavior. Large graphs
	// commit row deletion with a delta and compact their full snapshot later.
	err := s.hnsw.deleteWithPrecondition(ids, func() error {
		if check != nil {
			return check(s.db)
		}
		return nil
	}, func() error {
		useDelta := s.hnsw.saveGraphDelta != nil && !(s.hnsw.syncGraphSnapshot != nil && s.hnsw.syncGraphSnapshot())
		var updates map[string]interface{}
		if !useDelta {
			var err error
			updates, err = graphDatabaseUpdates(s.hnsw)
			if err != nil {
				return err
			}
		}
		err := utils.GormTransaction(s.db, func(tx *gorm.DB) error {
			if check != nil {
				if err := check(tx); err != nil {
					return err
				}
			}
			const deleteBatchSize = 200
			for start := 0; start < len(ids); start += deleteBatchSize {
				end := min(start+deleteBatchSize, len(ids))
				if err := tx.Model(&schema.VectorStoreDocument{}).
					Where("collection_id = ? AND document_id IN (?)", s.collection.ID, ids[start:end]).
					Unscoped().Delete(&schema.VectorStoreDocument{}).Error; err != nil {
					return utils.Wrap(err, "delete vector documents")
				}
			}
			var remaining int64
			if err := tx.Model(&schema.VectorStoreCollection{}).Where("id = ?", s.collection.ID).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining != 1 {
				return utils.Error("vector collection was deleted")
			}
			if useDelta {
				if err := s.hnsw.saveGraphDelta(tx); err != nil {
					return err
				}
				return nil
			}
			if err := writeGraphDatabaseUpdates(tx, s.collection.UUID, updates); err != nil {
				return err
			}
			return clearGraphDeltas(tx, s.collection.UUID, 0)
		})
		if err == nil && !useDelta {
			s.hnsw.graph.TakeAdjacencyChanges()
			if s.hnsw.graphSnapshotSaved != nil {
				s.hnsw.graphSnapshotSaved()
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	if s.hnsw.scheduleGraphSnapshot != nil {
		s.hnsw.scheduleGraphSnapshot()
	}
	return nil
}

func (s *SQLiteVectorStoreHNSW) GetName() string {
	return s.collection.Name
}

// Get 根据 ID 获取文档
func (s *SQLiteVectorStoreHNSW) Get(id string) (*Document, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, err := yakit.GetRAGDocumentByCollectionIDAndKey(s.db, s.collection.ID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, utils.Wrap(err, "查询文档失败")
	}
	if doc == nil {
		return nil, false, nil
	}

	return s.toDocument(doc), true, nil
}

// List 列出所有文档
func (s *SQLiteVectorStoreHNSW) List() ([]*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var docs []schema.VectorStoreDocument
	if err := s.db.Where("collection_id = ?", s.collection.ID).Where("document_id <> ?", DocumentTypeCollectionInfo).Find(&docs).Error; err != nil {
		return nil, utils.Errorf("查询文档失败: %v", err)
	}

	results := make([]*Document, len(docs))
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

func (s *SQLiteVectorStoreHNSW) SampleDocuments(n int) ([]*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var docs []schema.VectorStoreDocument
	if err := s.db.Where("collection_id = ?", s.collection.ID).
		Where("document_id <> ?", DocumentTypeCollectionInfo).
		Limit(n).Find(&docs).Error; err != nil {
		return nil, utils.Errorf("sample documents failed: %v", err)
	}

	results := make([]*Document, len(docs))
	for i, doc := range docs {
		results[i] = s.toDocument(&doc)
	}
	return results, nil
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
		diagnostics["hnsw_layers_count"] = s.hnsw.GetLayerLength()
		totalNodes := 0
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

func (s *SQLiteVectorStoreHNSW) Clear() error {
	if err := s.requireWriteCollection(); err != nil {
		return utils.Wrap(err, "require write vector store")
	}
	// Only fetch identifiers. Loading every document used to materialize all
	// content, metadata and embedding blobs before a clear, causing avoidable
	// latency and peak-memory growth on large collections.
	var ids []string
	if err := s.db.Model(&schema.VectorStoreDocument{}).
		Where("collection_id = ?", s.collection.ID).
		Where("document_id <> ?", DocumentTypeCollectionInfo).
		Pluck("document_id", &ids).Error; err != nil {
		return utils.Wrap(err, "list vector document ids")
	}
	return s.Delete(ids...)
}

// 确保 SQLiteVectorStoreHNSW 实现了 VectorStore 接口
var _ VectorStore = (*SQLiteVectorStoreHNSW)(nil)
