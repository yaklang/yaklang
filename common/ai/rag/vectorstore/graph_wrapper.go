package vectorstore

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

var GraphWrapperManager = NewGraphHNSWManager()

type GraphHNSWManager struct {
	cache map[string]*GraphWrapper[string]
	lock  sync.Mutex
}

func NewGraphHNSWManager() *GraphHNSWManager {
	return &GraphHNSWManager{
		make(map[string]*GraphWrapper[string]),
		sync.Mutex{},
	}
}

func (gm *GraphHNSWManager) ClearCache() {
	gm.lock.Lock()
	wrapperList := make([]*GraphWrapper[string], 0, len(gm.cache))
	for _, wrapper := range gm.cache {
		wrapperList = append(wrapperList, wrapper)
	}
	gm.cache = make(map[string]*GraphWrapper[string])
	gm.lock.Unlock()

	for _, wrapper := range wrapperList {
		wrapper.Close()
	}
}

func graphCacheKey(db *gorm.DB, collection *schema.VectorStoreCollection) string {
	return fmt.Sprintf("%p:%s:%d", db.CommonDB(), collection.UUID, collection.ID)
}

// RemoveFromCache preserves the historical UUID-based API. A UUID can occur in
// more than one database (for example after importing a backup), so every
// matching wrapper is removed rather than leaving a cross-database cache entry.
func (gm *GraphHNSWManager) RemoveFromCache(collectionUUID string) {
	gm.lock.Lock()
	wrapperList := make([]*GraphWrapper[string], 0, 1)
	for cacheKey, wrapper := range gm.cache {
		if wrapper.collectionUUID == collectionUUID {
			wrapperList = append(wrapperList, wrapper)
			delete(gm.cache, cacheKey)
		}
	}
	gm.lock.Unlock()

	for _, wrapper := range wrapperList {
		wrapper.Close()
	}
}

// RemoveCollectionFromCache removes only the wrapper belonging to the given
// database and collection. Deletion paths use this precise variant so an
// unrelated imported collection remains available.
func (gm *GraphHNSWManager) RemoveCollectionFromCache(db *gorm.DB, collection *schema.VectorStoreCollection) {
	gm.lock.Lock()
	cacheKey := graphCacheKey(db, collection)
	wrapper := gm.cache[cacheKey]
	delete(gm.cache, cacheKey)
	gm.lock.Unlock()

	if wrapper != nil {
		wrapper.Close()
	}
}

func (gm *GraphHNSWManager) GetGraphWrapper(db *gorm.DB, collection *schema.VectorStoreCollection, collectionConfig *CollectionConfig) (*GraphWrapper[string], error) {
	gm.lock.Lock()
	defer gm.lock.Unlock()
	cacheKey := graphCacheKey(db, collection)
	wrapper, ok := gm.cache[cacheKey]
	if ok {
		return wrapper, nil

	}
	var collectionCount int64
	if err := db.Model(&schema.VectorStoreCollection{}).Where("id = ?", collection.ID).Count(&collectionCount).Error; err != nil {
		return nil, utils.Wrap(err, "check graph collection exists")
	}
	if collectionCount == 0 {
		return nil, utils.Errorf("collection %s has been deleted", collection.Name)
	}
	wrapper, err := getGraphWrapperFromDB(db, collection, collectionConfig)
	if err != nil {
		return nil, utils.Wrap(err, "get graph wrapper from db")
	}
	gm.cache[cacheKey] = wrapper
	return wrapper, nil
}

func getGraphWrapperFromDB(db *gorm.DB, collection *schema.VectorStoreCollection, collectionConfig *CollectionConfig) (*GraphWrapper[string], error) {
	collectionName := collection.Name
	graphOptions := []hnsw.GraphOption[string]{
		hnsw.WithHNSWParameters[string](collectionConfig.MaxNeighbors, collectionConfig.LayerGenerationFactor, collectionConfig.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc(collectionConfig.DistanceFuncType)),
	}
	if collectionConfig.EfConstruct > 0 {
		graphOptions = append(graphOptions, hnsw.WithEfConstruction[string](collectionConfig.EfConstruct))
	}
	hnswGraph := NewHNSWGraph(collectionName, graphOptions...)

	log.Infof("start to recover hnsw graph from db, collection name: %s", collectionName)
	switch collectionConfig.buildGraphPolicy {
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
			graphOptions := []hnsw.GraphOption[string]{
				hnsw.WithHNSWParameters[string](config.M, config.Ml, config.EfSearch),
				hnsw.WithDistance[string](hnsw.GetDistanceFunc(config.DistanceFuncType)),
			}
			if config.EfConstruct > 0 {
				graphOptions = append(graphOptions, hnsw.WithEfConstruction[string](config.EfConstruct))
			}
			hnswGraph = NewHNSWGraph(collectionName, graphOptions...)
		} else {
			graphBinaryReader := bytes.NewReader(collection.GraphBinary)
			hnswGraph, err = parseHNSWGraphFromBinary(db, collection, collectionConfig, graphBinaryReader)
			if err != nil {
				if collectionConfig.TryRebuildHNSWIndex {
					log.Warnf("load hnsw graph from binary error: %v, try to rebuild hnsw graph, migrate hnsw graph from db", err)
					err := MigrateHNSWGraph(db, collection)
					if err != nil {
						if errors.Is(err, graphNodesIsEmpty) {
							// 知识库没有文档，创建空的 HNSW 图
							graphOptions := []hnsw.GraphOption[string]{
								hnsw.WithHNSWParameters[string](collection.M, collection.Ml, collection.EfSearch),
								hnsw.WithDistance[string](hnsw.GetDistanceFunc(collection.DistanceFuncType)),
							}
							if collection.EfConstruct > 0 {
								graphOptions = append(graphOptions, hnsw.WithEfConstruction[string](collection.EfConstruct))
							}
							hnswGraph = NewHNSWGraph(collection.Name, graphOptions...)
						} else {
							return nil, utils.Wrap(err, "migrate hnsw graph")
						}
					} else {
						graphBinaryReader := bytes.NewReader(collection.GraphBinary)
						hnswGraph, err = parseHNSWGraphFromBinary(db, collection, collectionConfig, graphBinaryReader)
						if err != nil {
							return nil, utils.Wrap(err, "parse hnsw graph from binary")
						}
					}
				} else {
					return nil, utils.Wrap(err, "parse hnsw graph from binary")
				}
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
	wrapper := NewGraphWrapper(hnswGraph, collection.Name, collection.UUID)

	if collectionConfig.EnableAutoUpdateGraphInfos {
		wrapper.setOnLayerChange(func(Layers []*hnsw.Layer[string]) {
			err := updateDatabaseGraphInfoInLock(db, collection.UUID, wrapper)
			if err != nil {
				log.Errorf("update database graph info in lock err: %v", err)
			}
		})
	}
	return wrapper, nil
}

var (
	opTypeRead  = "read"
	opTypeWrite = "write"
)

type graphOp struct {
	opType string // read | write
	desc   string
	params string
	fn     func()
}

type GraphWrapper[K cmp.Ordered] struct {
	graph                *hnsw.Graph[K]
	operationChannel     *chanx.UnlimitedChan[*graphOp]
	mu                   sync.RWMutex
	operationWG          sync.WaitGroup
	closeOnce            sync.Once
	closed               atomic.Bool
	done                 chan struct{}
	singleOpWarnDuration time.Duration
	collectionName       string
	collectionUUID       string
}

func NewGraphWrapper[K cmp.Ordered](graph *hnsw.Graph[K], collectionName, collectionUUID string) *GraphWrapper[K] {
	wrapper := &GraphWrapper[K]{
		graph:                graph,
		operationChannel:     chanx.NewUnlimitedChan[*graphOp](context.Background(), 10),
		done:                 make(chan struct{}),
		singleOpWarnDuration: 3 * time.Second,
		collectionName:       collectionName,
		collectionUUID:       collectionUUID,
	}
	go wrapper.start()
	return wrapper
}

func (gw *GraphWrapper[K]) start() {
	defer func() {
		gw.operationWG.Wait()
		close(gw.done)
	}()
	for op := range gw.operationChannel.OutputChannel() {
		switch op.opType {
		case opTypeWrite:
			gw.mu.Lock()
			gw.executeGraphOpInLock(op)
			gw.mu.Unlock()
		case opTypeRead:
			gw.mu.RLock()
			gw.operationWG.Add(1)
			go func(op *graphOp) {
				defer gw.operationWG.Done()
				defer gw.mu.RUnlock()
				gw.executeGraphOpInLock(op)
			}(op)
		}
	}
}

func (gw *GraphWrapper[K]) submit(op *graphOp) bool {
	if gw.closed.Load() {
		return false
	}
	return gw.operationChannel.SafeFeedWithResult(op)
}

// Close drains all accepted graph operations and stops both queue goroutines.
// It is safe to call more than once and prevents deleted collections from being
// retained forever by the global graph cache.
func (gw *GraphWrapper[K]) Close() {
	gw.closeOnce.Do(func() {
		gw.closed.Store(true)
		gw.operationChannel.Close()
		<-gw.done
	})
}

func (gw *GraphWrapper[K]) executeGraphOpInLock(op *graphOp) {
	warnAfter := gw.singleOpWarnDuration
	if warnAfter <= 0 {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in graph %s operation %q (%s): %v", op.opType, op.desc, gw.describeOp(op), r)
			}
		}()
		op.fn()
		return
	}

	startedAt := time.Now()
	var finished atomic.Bool
	timer := time.AfterFunc(warnAfter, func() {
		if finished.Load() {
			return
		}
		elapsed := time.Since(startedAt)
		log.Errorf("graph %s operation %q (%s) is running longer than %s (elapsed %s)", op.opType, op.desc, gw.describeOp(op), warnAfter, elapsed)
	})

	defer func() {
		finished.Store(true)
		timer.Stop()
		if r := recover(); r != nil {
			log.Errorf("recovered from panic in graph %s operation %q (%s): %v", op.opType, op.desc, gw.describeOp(op), r)
		}
		if elapsed := time.Since(startedAt); elapsed > warnAfter {
			log.Errorf("graph %s operation %q (%s) took %s (> %s)", op.opType, op.desc, gw.describeOp(op), elapsed, warnAfter)
		}
	}()

	op.fn()
}

func (gw *GraphWrapper[K]) describeOp(op *graphOp) string {
	collectionInfo := gw.collectionName
	switch {
	case collectionInfo != "" && gw.collectionUUID != "":
		collectionInfo = fmt.Sprintf("%s (%s)", collectionInfo, gw.collectionUUID)
	case collectionInfo == "" && gw.collectionUUID != "":
		collectionInfo = gw.collectionUUID
	case collectionInfo == "":
		collectionInfo = "unknown"
	}

	params := op.params
	if params == "" {
		params = "n/a"
	}

	return fmt.Sprintf("collection=%s, params=%s", collectionInfo, params)
}

// Add keeps the original public API. Internal persistence paths use
// AddWithError so a concurrent collection close cannot leave database rows
// without corresponding graph nodes.
func (gw *GraphWrapper[K]) Add(nodes ...hnsw.InputNode[K]) time.Duration {
	duration, _ := gw.AddWithError(nodes...)
	return duration
}

func (gw *GraphWrapper[K]) AddWithError(nodes ...hnsw.InputNode[K]) (time.Duration, error) {
	done := make(chan struct{}, 1)
	var pureUseTime time.Duration
	if !gw.submit(&graphOp{
		opType: opTypeWrite,
		desc:   "Add",
		params: fmt.Sprintf("nodes_count=%d", len(nodes)),
		fn: func() {
			start := time.Now()
			defer close(done)
			gw.graph.Add(nodes...)
			pureUseTime = time.Since(start)
		},
	}) {
		return 0, errors.New("graph wrapper is closed")
	}
	<-done
	return pureUseTime, nil
}

// Delete keeps the original public API. Internal persistence paths use
// DeleteWithError to detect a wrapper closed by collection deletion.
func (gw *GraphWrapper[K]) Delete(uids ...K) {
	_ = gw.DeleteWithError(uids...)
}

func (gw *GraphWrapper[K]) DeleteWithError(uids ...K) error {
	done := make(chan struct{}, 1)
	if !gw.submit(&graphOp{
		opType: opTypeWrite,
		desc:   "Delete",
		params: fmt.Sprintf("uids=%v", uids),
		fn: func() {
			defer close(done)
			for _, uid := range uids {
				gw.graph.Delete(uid)
			}

		},
	}) {
		return errors.New("graph wrapper is closed")
	}
	<-done
	return nil
}

func (gw *GraphWrapper[K]) SearchWithDistanceAndFilter(near []float32, k int, filter hnsw.FilterFunc[K]) []hnsw.SearchResult[K] {
	resultChan := make(chan []hnsw.SearchResult[K], 2)
	if !gw.submit(&graphOp{
		opType: opTypeRead,
		desc:   "SearchWithDistanceAndFilter",
		params: fmt.Sprintf("k=%d, near_len=%d, has_filter=%t", k, len(near), filter != nil),
		fn: func() {
			results := gw.graph.SearchWithDistanceAndFilter(near, k, filter)
			resultChan <- results
		},
	}) {
		return nil
	}
	return <-resultChan
}

func (gw *GraphWrapper[K]) Has(docId K) bool {
	resultChan := make(chan bool, 2)
	if !gw.submit(&graphOp{
		opType: opTypeRead,
		desc:   "Has",
		params: fmt.Sprintf("doc_id=%v", docId),
		fn: func() {
			resultChan <- gw.graph.Has(docId)
		},
	}) {
		return false
	}
	return <-resultChan
}

func (gw *GraphWrapper[K]) GetSize() int {
	resultChan := make(chan int, 2)
	if !gw.submit(&graphOp{
		opType: opTypeRead,
		desc:   "GetSize",
		params: "none",
		fn: func() {
			var nodeNum int
			if len(gw.graph.Layers) > 0 && len(gw.graph.Layers[0].Nodes) > 0 {
				nodeNum = len(gw.graph.Layers[0].Nodes)
			}
			resultChan <- nodeNum
		},
	}) {
		return 0
	}
	return <-resultChan
}

func (gw *GraphWrapper[K]) GetLayerLength() int {
	resultChan := make(chan int, 2)
	if !gw.submit(&graphOp{
		opType: opTypeRead,
		desc:   "GetLayerLength",
		params: "none",
		fn: func() {
			resultChan <- len(gw.graph.Layers)
		},
	}) {
		return 0
	}
	return <-resultChan
}

func (gw *GraphWrapper[K]) TrainPQCodebookFromDataWithCallback(m, k int, callback func(key K, code []byte, vector []float64) (hnswspec.LayerNode[K], error)) (*pq.Codebook, error) {
	var codebook *pq.Codebook
	var err error
	done := make(chan struct{}, 1)
	if !gw.submit(&graphOp{
		opType: opTypeWrite,
		desc:   "TrainPQCodebookFromDataWithCallback",
		params: fmt.Sprintf("m=%d,k=%d", m, k),
		fn: func() {
			defer close(done)
			codebook, err = gw.graph.TrainPQCodebookFromDataWithCallback(m, k, callback)
		},
	}) {
		return nil, errors.New("graph wrapper is closed")
	}
	<-done
	return codebook, err
}

func (gw *GraphWrapper[K]) GetCodeBook() *pq.Codebook {
	return gw.graph.GetCodebook()
}

func (gw *GraphWrapper[K]) IsPQEnabled() bool {
	return gw.graph.IsPQEnabled()
}

func (gw *GraphWrapper[K]) GetQuantizer() *pq.Quantizer {
	return gw.graph.GetPQQuantizer()
}

// exportHNSWGraphToBinaryInLock exports the HNSW graph to binary format under a lock.
func (gw *GraphWrapper[K]) exportHNSWGraphToBinaryInLock() (io.Reader, error) {
	if gw.graph.IsEmpty() {
		return nil, graphNodesIsEmpty
	}
	pers, err := hnsw.ExportHNSWGraph(gw.graph)
	if err != nil {
		return nil, err
	}
	pers.Dims = 1024
	return pers.ToBinary(context.Background())
}

func (gw *GraphWrapper[K]) setOnLayerChange(handler func(Layers []*hnsw.Layer[K])) {
	gw.graph.OnLayersChange = handler
}

func updateDatabaseGraphInfoInLock(db *gorm.DB, uuid string, wrapper *GraphWrapper[string]) error {
	var graphInfosBytes []byte
	graphInfos, err := wrapper.exportHNSWGraphToBinaryInLock()
	if err != nil {
		if errors.Is(err, graphNodesIsEmpty) {
			// HNSW graph is empty, set graph_binary to empty bytes
			graphInfosBytes = []byte{}
		} else {
			return utils.Wrap(err, "export hnsw graph to binary")
		}
	} else {
		graphInfosBytes, err = io.ReadAll(graphInfos)
		if err != nil {
			return utils.Wrap(err, "read graph infos")
		}
	}
	err = db.Model(&schema.VectorStoreCollection{}).Where("uuid = ?", uuid).Update("graph_binary", graphInfosBytes).Error
	if err != nil {
		return utils.Wrap(err, "update graph binary")
	}
	if wrapper.IsPQEnabled() {
		codebook, err := hnsw.ExportCodebook(wrapper.GetCodeBook())
		if err != nil {
			return utils.Wrap(err, "export codebook")
		}
		codebookBytes, err := io.ReadAll(codebook)
		if err != nil {
			return utils.Wrap(err, "read codebook")
		}
		err = db.Model(&schema.VectorStoreCollection{}).Where("uuid = ?", uuid).Update("code_book_binary", codebookBytes).Error
		if err != nil {
			return utils.Wrap(err, "update codebook")
		}
	}
	return nil
}
