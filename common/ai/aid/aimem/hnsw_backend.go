package aimem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// AIMemoryHNSWBackend 管理AIMemory的HNSW索引
type AIMemoryHNSWBackend struct {
	sessionID  string
	db         *gorm.DB
	graph      atomic.Pointer[hnsw.Graph[string]]
	collection *schema.AIMemoryCollection

	// 保存操作的专用锁 - 确保保存操作的原子性
	saveMutex sync.Mutex

	// Coalesce bursts of graph changes into one bounded save worker.
	saveScheduleMutex sync.Mutex
	savePending       bool
	saveDone          chan struct{}
	saveClosed        bool

	// 图操作的全局锁 - 确保所有图操作（Add/Delete/Update/Export）的互斥性
	graphMutex sync.RWMutex

	// 原子操作标志
	rebuilding int32 // 是否正在重建

	// 是否自动保存graph到数据库
	autoSave bool

	// midtermMode selects independent DB tables for midterm archive storage.
	midtermMode bool
}

type HNSWBackendConfig struct {
	autoSave    bool
	sessionID   string
	db          *gorm.DB
	midtermMode bool
}

type HNSWOption func(*HNSWBackendConfig)

func WithHNSWAutoSave(autoSave bool) HNSWOption {
	return func(b *HNSWBackendConfig) {
		b.autoSave = autoSave
	}
}

func WithHNSWDatabase(db *gorm.DB) HNSWOption {
	return func(b *HNSWBackendConfig) {
		b.db = db
	}
}

func WithHNSWSessionID(sessionID string) HNSWOption {
	return func(b *HNSWBackendConfig) {
		b.sessionID = sessionID
	}
}

// WithHNSWMidtermMode configures the backend to use independent midterm archive tables.
func WithHNSWMidtermMode(midtermMode bool) HNSWOption {
	return func(b *HNSWBackendConfig) {
		b.midtermMode = midtermMode
	}
}

func NewHNSWBackendConfig(opts ...HNSWOption) (*HNSWBackendConfig, error) {
	config := &HNSWBackendConfig{
		autoSave: true,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.sessionID == "" {
		config.sessionID = uuid.NewString()
	}

	if config.db == nil {
		config.db = consts.GetGormProjectDatabase()
		if config.db == nil {
			return nil, utils.Errorf("database connection is nil")
		}
	}

	return config, nil
}

// NewAIMemoryHNSWBackend 创建或加载HNSW后端
func NewAIMemoryHNSWBackend(options ...HNSWOption) (*AIMemoryHNSWBackend, error) {
	config, err := NewHNSWBackendConfig(options...)
	if err != nil {
		return nil, err
	}
	db := config.db
	sessionID := config.sessionID

	// 查找或创建collection
	collectionTable := "ai_memory_collections_v1"
	if config.midtermMode {
		collectionTable = "ai_midterm_archive_collections_v1"
	}
	var collection schema.AIMemoryCollection
	err = db.Table(collectionTable).Where("session_id = ?", sessionID).First(&collection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 创建新的collection
		collection = schema.AIMemoryCollection{
			SessionID:   sessionID,
			M:           16,
			Ml:          0.25,
			EfSearch:    20,
			EfConstruct: 200,
			Dimension:   7,
		}
		if err := db.Table(collectionTable).Create(&collection).Error; err != nil {
			return nil, utils.Errorf("create collection failed: %v", err)
		}
	} else if err != nil {
		return nil, utils.Errorf("query collection failed: %v", err)
	}

	backend := &AIMemoryHNSWBackend{
		sessionID:   sessionID,
		db:          db,
		collection:  &collection,
		autoSave:    config.autoSave,
		midtermMode: config.midtermMode,
	}

	// 加载或创建HNSW Graph
	var graph *hnsw.Graph[string]
	if len(collection.GraphBinary) > 0 {
		// 从二进制数据恢复graph
		var err error
		graph, err = backend.loadGraphFromBinary(collection.GraphBinary)
		if err != nil {
			log.Warnf("load graph from binary failed: %v, creating new graph", err)
			graph = backend.createNewGraph()
		}
	} else {
		// 创建新的graph
		graph = backend.createNewGraph()
	}

	// 设置到原子指针
	backend.graph.Store(graph)

	// 设置graph变化回调 - 使用专用锁保证保存操作的原子性
	graph.OnLayersChange = func([]*hnsw.Layer[string]) { backend.scheduleSave() }

	return backend, nil
}

// createNewGraph 创建新的HNSW Graph
func (b *AIMemoryHNSWBackend) createNewGraph() *hnsw.Graph[string] {
	options := []hnsw.GraphOption[string]{
		hnsw.WithHNSWParameters[string](b.collection.M, b.collection.Ml, b.collection.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc("cosine")),
	}
	if b.collection.EfConstruct > 0 {
		options = append(options, hnsw.WithEfConstruction[string](b.collection.EfConstruct))
	}
	return hnsw.NewGraph[string](options...)
}

// loadGraphFromBinary 从二进制数据加载HNSW Graph
func (b *AIMemoryHNSWBackend) loadGraphFromBinary(graphBinary []byte) (*hnsw.Graph[string], error) {
	reader := bytes.NewReader(graphBinary)

	// 创建节点加载函数
	loadNodeFunc := func(_ string, key hnswspec.LazyNodeID) (hnswspec.LayerNode[string], error) {
		memoryID, ok := key.(string)
		if !ok {
			return nil, utils.Errorf("invalid key type: %T", key)
		}

		// 从数据库加载记忆实体
		var dbEntity schema.AIMemoryEntity
		entityTable := "ai_memory_entities_v1"
		if b.midtermMode {
			entityTable = "ai_midterm_archive_entities_v1"
		}
		if err := b.db.Table(entityTable).Where("memory_id = ? AND session_id = ?", memoryID, b.sessionID).First(&dbEntity).Error; err != nil {
			return nil, utils.Errorf("load memory entity failed: %v", err)
		}

		// 创建节点
		vector := []float32(dbEntity.CorePactVector)
		return hnswspec.NewStandardLayerNode(memoryID, func() []float32 {
			return vector
		}), nil
	}

	// 加载graph
	options := []hnsw.GraphOption[string]{
		hnsw.WithHNSWParameters[string](b.collection.M, b.collection.Ml, b.collection.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc("cosine")),
	}
	if b.collection.EfConstruct > 0 {
		options = append(options, hnsw.WithEfConstruction[string](b.collection.EfConstruct))
	}
	graph, err := hnsw.LoadGraphFromBinary(reader, loadNodeFunc, options...)
	if err != nil {
		return nil, utils.Errorf("load graph from binary failed: %v", err)
	}

	return graph, nil
}

// SaveGraph 保存HNSW Graph到数据库
func (b *AIMemoryHNSWBackend) SaveGraph() error {
	b.saveMutex.Lock()
	defer b.saveMutex.Unlock()
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	// A different backend may have cleaned this session since we loaded it.
	// Rebase on the durable survivor graph, retaining only locally added nodes
	// whose authoritative entity rows still exist.
	var current schema.AIMemoryCollection
	if err := b.db.Table(b.collectionTable()).Where("session_id = ?", b.sessionID).First(&current).Error; err != nil {
		return err
	}
	if current.CleanupVersion != b.collection.CleanupVersion {
		if err := b.rebaseAfterCleanup(&current); err != nil {
			return err
		}
	}
	binaryData, err := memoryGraphBinary(b.graph.Load())
	if err != nil {
		return err
	}
	result := b.db.Table(b.collectionTable()).
		Where("session_id = ? AND COALESCE(cleanup_version,0) = ?", b.sessionID, current.CleanupVersion).
		Update("graph_binary", binaryData)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return utils.Error("memory graph changed during cleanup; retry save")
	}
	return nil
}

func (b *AIMemoryHNSWBackend) collectionTable() string {
	if b.midtermMode {
		return "ai_midterm_archive_collections_v1"
	}
	return "ai_memory_collections_v1"
}

func (b *AIMemoryHNSWBackend) entityTable() string {
	if b.midtermMode {
		return "ai_midterm_archive_entities_v1"
	}
	return "ai_memory_entities_v1"
}

func memoryGraphBinary(graph *hnsw.Graph[string]) ([]byte, error) {
	if graph == nil {
		return nil, utils.Error("graph is nil")
	}
	if graph.IsEmpty() {
		// ExportHNSWGraph rejects empty graphs. Persist the empty state explicitly
		// so deleting the final node cannot leave the previous binary behind.
		return []byte{}, nil
	}
	exported, err := hnsw.ExportHNSWGraph(graph)
	if err != nil {
		return nil, err
	}
	exported.Dims = 7
	reader, err := exported.ToBinary(context.Background())
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

// Called with graphMutex and saveMutex held. Read IDs only, in bounded batches;
// legacy memory content and embeddings can be arbitrarily large.
func (b *AIMemoryHNSWBackend) rebaseAfterCleanup(current *schema.AIMemoryCollection) error {
	graph := b.createNewGraph()
	if len(current.GraphBinary) > 0 {
		var err error
		graph, err = b.loadGraphFromBinary(current.GraphBinary)
		if err != nil {
			return err
		}
	}
	old := b.graph.Load()
	if old != nil && !old.IsEmpty() {
		ids := make([]string, 0, 200)
		merge := func() error {
			var live []schema.AIMemoryEntity
			if err := b.db.Table(b.entityTable()).Select("memory_id").
				Where("session_id = ? AND memory_id IN (?)", b.sessionID, ids).Find(&live).Error; err != nil {
				return err
			}
			for _, entity := range live {
				if !graph.Has(entity.MemoryID) {
					if vector, ok := old.Lookup(entity.MemoryID); ok {
						graph.Add(hnsw.InputNode[string]{Key: entity.MemoryID, Value: vector()})
					}
				}
			}
			ids = ids[:0]
			return nil
		}
		for key := range old.Layers[0].Nodes {
			ids = append(ids, key)
			if len(ids) == cap(ids) {
				if err := merge(); err != nil {
					return err
				}
			}
		}
		if len(ids) > 0 {
			if err := merge(); err != nil {
				return err
			}
		}
	}
	graph.OnLayersChange = func([]*hnsw.Layer[string]) { b.scheduleSave() }
	b.graph.Store(graph)
	b.collection.CleanupVersion = current.CleanupVersion
	return nil
}

// deleteAndSave atomically persists the memory graph, cleanup generation and
// (for automatic cleanup) entity deletion. RAG deletion must succeed first.
func (b *AIMemoryHNSWBackend) deleteAndSave(ctx context.Context, ids []string, deleteEntities bool) error {
	b.saveMutex.Lock()
	defer b.saveMutex.Unlock()
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()
	_, err := b.graph.Load().DeleteBatchWithCommit(ids, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		binaryData, err := memoryGraphBinary(b.graph.Load())
		if err != nil {
			return err
		}
		tx := b.db.BeginTx(ctx, nil)
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		if deleteEntities {
			for start := 0; start < len(ids); start += 200 {
				if err := tx.Table(b.entityTable()).Unscoped().
					Where("session_id = ? AND memory_id IN (?)", b.sessionID, ids[start:min(start+200, len(ids))]).Delete(nil).Error; err != nil {
					return err
				}
			}
		}
		result := tx.Table(b.collectionTable()).
			Where("session_id = ? AND COALESCE(cleanup_version,0) = ?", b.sessionID, b.collection.CleanupVersion).
			Updates(map[string]interface{}{"graph_binary": binaryData, "cleanup_version": gorm.Expr("COALESCE(cleanup_version,0) + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return utils.Error("memory graph changed during cleanup; retry cleanup")
		}
		if err := tx.Commit().Error; err != nil {
			return err
		}
		b.collection.CleanupVersion++
		return nil
	})
	return err
}

// HasMemoryID reports whether the graph already contains the memory id.
func (b *AIMemoryHNSWBackend) HasMemoryID(memoryID string) bool {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return false
	}
	b.graphMutex.RLock()
	defer b.graphMutex.RUnlock()
	graph := b.graph.Load()
	if graph == nil {
		return false
	}
	return graph.Has(memoryID)
}

// ListMemoryIDs returns all node keys currently present in layer 0.
func (b *AIMemoryHNSWBackend) ListMemoryIDs() []string {
	b.graphMutex.RLock()
	defer b.graphMutex.RUnlock()
	graph := b.graph.Load()
	if graph == nil || len(graph.Layers) == 0 || graph.Layers[0] == nil {
		return nil
	}
	ids := make([]string, 0, len(graph.Layers[0].Nodes))
	for key := range graph.Layers[0].Nodes {
		ids = append(ids, string(key))
	}
	return ids
}

// Add 添加记忆实体到HNSW索引
func (b *AIMemoryHNSWBackend) Add(entity *aicommon.MemoryEntity) error {
	// 获取写锁来修改图
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	graph := b.graph.Load()
	if graph == nil {
		return utils.Errorf("graph is nil")
	}

	// 创建输入节点
	node := hnsw.InputNode[string]{
		Key:   entity.Id,
		Value: entity.CorePactVector,
	}

	// 添加到graph
	graph.Add(node)

	return nil
}

// Delete 从HNSW索引中删除记忆实体
func (b *AIMemoryHNSWBackend) Delete(memoryID string) error {
	// 获取写锁来修改图
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	graph := b.graph.Load()
	if graph == nil {
		return utils.Errorf("graph is nil")
	}

	graph.DeleteBatch(memoryID) // Missing nodes are an idempotent delete.

	return nil
}

// Update 更新HNSW索引中的记忆实体
func (b *AIMemoryHNSWBackend) Update(entity *aicommon.MemoryEntity) error {
	// 获取写锁来修改图
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	graph := b.graph.Load()
	if graph == nil {
		return utils.Errorf("graph is nil")
	}

	// 原子更新：删除旧的并添加新的
	graph.Delete(entity.Id)

	node := hnsw.InputNode[string]{
		Key:   entity.Id,
		Value: entity.CorePactVector,
	}
	graph.Add(node)

	return nil
}

func (b *AIMemoryHNSWBackend) searchKeysWithDistance(queryVector []float32, limit int) ([]hnsw.SearchResult[string], error) {
	// 获取读锁来搜索图
	b.graphMutex.RLock()
	graph := b.graph.Load()
	if graph == nil {
		b.graphMutex.RUnlock()
		return nil, utils.Errorf("graph is nil")
	}

	if len(queryVector) != 7 {
		b.graphMutex.RUnlock()
		return nil, utils.Errorf("query vector must be 7 dimensions, got %d", len(queryVector))
	}

	// 使用HNSW搜索
	searchResults := graph.SearchWithDistance(queryVector, limit)
	b.graphMutex.RUnlock() // 搜索完成后立即释放读锁

	if searchResults == nil {
		return []hnsw.SearchResult[string]{}, nil
	}
	return searchResults, nil
}

// Search 使用HNSW索引搜索相似的记忆实体
func (b *AIMemoryHNSWBackend) Search(queryVector []float32, limit int) ([]SearchResultWithDistance, error) {
	searchResults, err := b.searchKeysWithDistance(queryVector, limit)
	if err != nil {
		return nil, err
	}

	// 批量查询数据库以提高性能
	if len(searchResults) == 0 {
		return []SearchResultWithDistance{}, nil
	}

	// 收集所有需要查询的ID
	memoryIDs := make([]string, len(searchResults))
	for i, sr := range searchResults {
		memoryIDs[i] = sr.Key
	}

	// 批量查询数据库
	entityTable := "ai_memory_entities_v1"
	if b.midtermMode {
		entityTable = "ai_midterm_archive_entities_v1"
	}
	var dbEntities []schema.AIMemoryEntity
	if err := b.db.Table(entityTable).Where("memory_id IN (?) AND session_id = ?", memoryIDs, b.sessionID).
		Find(&dbEntities).Error; err != nil {
		return nil, utils.Errorf("batch query memory entities failed: %v", err)
	}

	// 创建ID到实体的映射
	entityMap := make(map[string]*schema.AIMemoryEntity)
	for i := range dbEntities {
		entityMap[dbEntities[i].MemoryID] = &dbEntities[i]
	}

	// 转换结果并保持顺序
	var results []SearchResultWithDistance
	for _, sr := range searchResults {
		dbEntity, exists := entityMap[sr.Key]
		if !exists {
			log.Warnf("memory entity not found in database: %s", sr.Key)
			continue
		}

		entity := &aicommon.MemoryEntity{
			Id:                 dbEntity.MemoryID,
			CreatedAt:          dbEntity.CreatedAt,
			Content:            dbEntity.Content,
			Tags:               []string(dbEntity.Tags),
			PotentialQuestions: []string(dbEntity.PotentialQuestions),
			C_Score:            dbEntity.C_Score,
			O_Score:            dbEntity.O_Score,
			R_Score:            dbEntity.R_Score,
			E_Score:            dbEntity.E_Score,
			P_Score:            dbEntity.P_Score,
			A_Score:            dbEntity.A_Score,
			T_Score:            dbEntity.T_Score,
			CorePactVector:     []float32(dbEntity.CorePactVector),
		}

		results = append(results, SearchResultWithDistance{
			Entity:   entity,
			Distance: sr.Distance,
			Score:    1 - sr.Distance, // 转换为相似度分数
		})
	}

	return results, nil
}

// RebuildIndex 重建HNSW索引（从数据库中的所有记忆实体）
func (b *AIMemoryHNSWBackend) RebuildIndex() error {
	// 使用原子操作防止并发重建
	if !atomic.CompareAndSwapInt32(&b.rebuilding, 0, 1) {
		return utils.Errorf("index rebuild already in progress")
	}
	defer atomic.StoreInt32(&b.rebuilding, 0)

	// 创建新的graph
	newGraph := b.createNewGraph()

	// 从数据库加载所有记忆实体
	var dbEntities []schema.AIMemoryEntity
	if err := b.db.Table(b.entityTable()).Select("memory_id, core_pact_vector").Where("session_id = ?", b.sessionID).Find(&dbEntities).Error; err != nil {
		return utils.Errorf("query memory entities failed: %v", err)
	}

	// 批量添加到graph
	if len(dbEntities) > 0 {
		nodes := make([]hnsw.InputNode[string], 0, len(dbEntities))
		for _, dbEntity := range dbEntities {
			nodes = append(nodes, hnsw.InputNode[string]{
				Key:   dbEntity.MemoryID,
				Value: []float32(dbEntity.CorePactVector),
			})
		}

		newGraph.Add(nodes...)
	}

	// 原子替换graph
	b.graphMutex.Lock()
	newGraph.OnLayersChange = func([]*hnsw.Layer[string]) { b.scheduleSave() }
	b.graph.Store(newGraph)
	b.graphMutex.Unlock()
	b.scheduleSave()

	log.Infof("rebuilt HNSW index for session %s with %d entities", b.sessionID, len(dbEntities))

	return nil
}

// GetStats 获取HNSW索引统计信息
func (b *AIMemoryHNSWBackend) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["session_id"] = b.sessionID
	stats["m"] = b.collection.M
	stats["ml"] = b.collection.Ml
	stats["ef_search"] = b.collection.EfSearch
	stats["ef_construct"] = b.collection.EfConstruct
	stats["dimension"] = b.collection.Dimension
	stats["auto_save"] = b.autoSave
	stats["rebuilding"] = atomic.LoadInt32(&b.rebuilding) == 1

	graph := b.graph.Load()
	if graph != nil {
		stats["layers_count"] = len(graph.Layers)
		totalNodes := 0
		for i, layer := range graph.Layers {
			nodesInLayer := len(layer.Nodes)
			stats[fmt.Sprintf("layer_%d_nodes", i)] = nodesInLayer
			totalNodes += nodesInLayer
		}
		stats["total_nodes"] = totalNodes
	} else {
		stats["graph_status"] = "not_loaded"
	}

	return stats
}

// scheduleSave never launches one goroutine per node mutation. Close drains
// the current worker before its final synchronous save.
func (b *AIMemoryHNSWBackend) scheduleSave() {
	if !b.autoSave {
		return
	}
	b.saveScheduleMutex.Lock()
	defer b.saveScheduleMutex.Unlock()
	if b.saveClosed {
		return
	}
	b.savePending = true
	if b.saveDone != nil {
		return
	}
	done := make(chan struct{})
	b.saveDone = done
	go func() {
		for {
			b.saveScheduleMutex.Lock()
			if !b.savePending || b.saveClosed {
				b.saveDone = nil
				close(done)
				b.saveScheduleMutex.Unlock()
				return
			}
			b.savePending = false
			b.saveScheduleMutex.Unlock()
			if err := b.SaveGraph(); err != nil {
				log.Errorf("auto save memory graph failed: %v", err)
			}
		}
	}()
}

// Close drains background saves and persists even an empty graph.
func (b *AIMemoryHNSWBackend) Close() error {
	if b.db == nil || b.graph.Load() == nil {
		return nil
	}
	b.saveScheduleMutex.Lock()
	b.saveClosed = true
	done := b.saveDone
	b.saveScheduleMutex.Unlock()
	if done != nil {
		<-done
	}
	return b.SaveGraph()
}

// SearchResultWithDistance 包含距离信息的搜索结果
type SearchResultWithDistance struct {
	Entity   *aicommon.MemoryEntity
	Distance float64
	Score    float64
}
