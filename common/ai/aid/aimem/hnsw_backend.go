package aimem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"sync"
	"sync/atomic"

	"github.com/jinzhu/gorm"
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

	// 图操作的全局锁 - 确保所有图操作（Add/Delete/Update/Export）的互斥性
	graphMutex sync.RWMutex

	// 原子操作标志
	rebuilding int32 // 是否正在重建

	// 是否自动保存graph到数据库
	autoSave bool
}

type HNSWBackendConfig struct {
	autoSave  bool
	sessionID string
	db        *gorm.DB
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
	var collection schema.AIMemoryCollection
	err = db.Where("session_id = ?", sessionID).First(&collection).Error
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
		if err := db.Create(&collection).Error; err != nil {
			return nil, utils.Errorf("create collection failed: %v", err)
		}
	} else if err != nil {
		return nil, utils.Errorf("query collection failed: %v", err)
	}

	backend := &AIMemoryHNSWBackend{
		sessionID:  sessionID,
		db:         db,
		collection: &collection,
		autoSave:   true,
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
	graph.OnLayersChange = func(layers []*hnsw.Layer[string]) {
		if backend.autoSave {
			// 异步保存，使用专用锁确保原子性
			go func() {
				if err := backend.SaveGraph(); err != nil {
					log.Errorf("auto save graph failed: %v", err)
				}
			}()
		}
	}

	return backend, nil
}

// createNewGraph 创建新的HNSW Graph
func (b *AIMemoryHNSWBackend) createNewGraph() *hnsw.Graph[string] {
	return hnsw.NewGraph[string](
		hnsw.WithHNSWParameters[string](b.collection.M, b.collection.Ml, b.collection.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc("cosine")),
	)
}

// loadGraphFromBinary 从二进制数据加载HNSW Graph
func (b *AIMemoryHNSWBackend) loadGraphFromBinary(graphBinary []byte) (*hnsw.Graph[string], error) {
	reader := bytes.NewReader(graphBinary)

	// 创建节点加载函数
	loadNodeFunc := func(key hnswspec.LazyNodeID) (hnswspec.LayerNode[string], error) {
		memoryID, ok := key.(string)
		if !ok {
			return nil, utils.Errorf("invalid key type: %T", key)
		}

		// 从数据库加载记忆实体
		var dbEntity schema.AIMemoryEntity
		if err := b.db.Where("memory_id = ? AND session_id = ?", memoryID, b.sessionID).First(&dbEntity).Error; err != nil {
			return nil, utils.Errorf("load memory entity failed: %v", err)
		}

		// 创建节点
		vector := []float32(dbEntity.CorePactVector)
		return hnswspec.NewStandardLayerNode(memoryID, func() []float32 {
			return vector
		}), nil
	}

	// 加载graph
	graph, err := hnsw.LoadGraphFromBinary(reader, loadNodeFunc,
		hnsw.WithHNSWParameters[string](b.collection.M, b.collection.Ml, b.collection.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc("cosine")),
	)
	if err != nil {
		return nil, utils.Errorf("load graph from binary failed: %v", err)
	}

	return graph, nil
}

// SaveGraph 保存HNSW Graph到数据库
func (b *AIMemoryHNSWBackend) SaveGraph() error {
	// 使用专用锁确保保存操作的原子性
	b.saveMutex.Lock()
	defer b.saveMutex.Unlock()

	// 获取读锁来导出图 - 确保与Add/Delete/Update操作互斥
	b.graphMutex.RLock()
	graph := b.graph.Load()
	if graph == nil {
		b.graphMutex.RUnlock()
		return utils.Errorf("graph is nil")
	}

	// 在读锁保护下导出图
	exportedGraph, err := hnsw.ExportHNSWGraph(graph)
	b.graphMutex.RUnlock() // Export完成后立即释放锁

	if err != nil {
		return utils.Errorf("export graph failed: %v", err)
	}

	exportedGraph.Dims = 7 // 7维向量
	binaryReader, err := exportedGraph.ToBinary(context.Background())
	if err != nil {
		return utils.Errorf("convert to binary failed: %v", err)
	}

	binaryData, err := io.ReadAll(binaryReader)
	if err != nil {
		return utils.Errorf("read binary data failed: %v", err)
	}

	// 原子更新数据库 - 使用事务确保原子性
	return utils.GormTransaction(b.db, func(tx *gorm.DB) error {
		// 使用Update方法避免主键冲突
		return tx.Model(&schema.AIMemoryCollection{}).
			Where("session_id = ?", b.sessionID).
			Update("graph_binary", binaryData).Error
	})
}

// Add 添加记忆实体到HNSW索引
func (b *AIMemoryHNSWBackend) Add(entity *MemoryEntity) error {
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

	deleted := graph.Delete(memoryID)
	if !deleted {
		log.Warnf("memory entity not found in graph: %s", memoryID)
	}

	return nil
}

// Update 更新HNSW索引中的记忆实体
func (b *AIMemoryHNSWBackend) Update(entity *MemoryEntity) error {
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

// Search 使用HNSW索引搜索相似的记忆实体
func (b *AIMemoryHNSWBackend) Search(queryVector []float32, limit int) ([]SearchResultWithDistance, error) {
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
	var dbEntities []schema.AIMemoryEntity
	if err := b.db.Where("memory_id IN (?) AND session_id = ?", memoryIDs, b.sessionID).
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

		entity := &MemoryEntity{
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
	if err := b.db.Where("session_id = ?", b.sessionID).Find(&dbEntities).Error; err != nil {
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

		// 设置回调函数 - 使用专用锁保证保存操作的原子性
		newGraph.OnLayersChange = func(layers []*hnsw.Layer[string]) {
			if b.autoSave {
				// 异步保存，使用专用锁确保原子性
				go func() {
					if err := b.SaveGraph(); err != nil {
						log.Errorf("auto save graph failed: %v", err)
					}
				}()
			}
		}

		newGraph.Add(nodes...)
	}

	// 原子替换graph
	b.graph.Store(newGraph)

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

// Close 关闭后端，保存索引
func (b *AIMemoryHNSWBackend) Close() error {
	return b.SaveGraph()
}

// SearchResultWithDistance 搜索结果（包含距离）
type SearchResultWithDistance struct {
	Entity   *MemoryEntity
	Distance float64
	Score    float64
}
