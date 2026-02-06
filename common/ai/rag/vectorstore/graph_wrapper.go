package vectorstore

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
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
	defer gm.lock.Unlock()
	gm.cache = make(map[string]*GraphWrapper[string])
}

func (gm *GraphHNSWManager) RemoveFromCache(collectionUUID string) {
	gm.lock.Lock()
	defer gm.lock.Unlock()
	delete(gm.cache, collectionUUID)
}

func (gm *GraphHNSWManager) GetGraphWrapper(db *gorm.DB, collection *schema.VectorStoreCollection, collectionConfig *CollectionConfig) (*GraphWrapper[string], error) {
	gm.lock.Lock()
	defer gm.lock.Unlock()
	wrapper, ok := gm.cache[collection.UUID]
	if ok {
		return wrapper, nil

	}
	wrapper, err := getGraphWrapperFromDB(db, collection, collectionConfig)
	if err != nil {
		return nil, utils.Wrap(err, "get graph wrapper from db")
	}
	gm.cache[collection.UUID] = wrapper
	return wrapper, nil
}

func getGraphWrapperFromDB(db *gorm.DB, collection *schema.VectorStoreCollection, collectionConfig *CollectionConfig) (*GraphWrapper[string], error) {
	collectionName := collection.Name
	hnswGraph := NewHNSWGraph(collectionName,
		hnsw.WithHNSWParameters[string](collectionConfig.MaxNeighbors, collectionConfig.LayerGenerationFactor, collectionConfig.EfSearch),
		hnsw.WithDistance[string](hnsw.GetDistanceFunc(collectionConfig.DistanceFuncType)),
	)

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
			hnswGraph = NewHNSWGraph(collectionName,
				hnsw.WithHNSWParameters[string](config.M, config.Ml, config.EfSearch),
				hnsw.WithDistance[string](hnsw.GetDistanceFunc(config.DistanceFuncType)),
			)
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
							hnswGraph = NewHNSWGraph(
								collection.Name,
								hnsw.WithHNSWParameters[string](collection.M, collection.Ml, collection.EfSearch),
								hnsw.WithDistance[string](hnsw.GetDistanceFunc(collection.DistanceFuncType)),
							)
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
	singleOpWarnDuration time.Duration
	collectionName       string
	collectionUUID       string
}

func NewGraphWrapper[K cmp.Ordered](graph *hnsw.Graph[K], collectionName, collectionUUID string) *GraphWrapper[K] {
	wrapper := &GraphWrapper[K]{
		graph:                graph,
		operationChannel:     chanx.NewUnlimitedChan[*graphOp](context.Background(), 10),
		singleOpWarnDuration: 3 * time.Second,
		collectionName:       collectionName,
		collectionUUID:       collectionUUID,
	}
	go wrapper.start()
	return wrapper
}

func (gw *GraphWrapper[K]) start() {
	for op := range gw.operationChannel.OutputChannel() {
		switch op.opType {
		case opTypeWrite:
			gw.mu.Lock()
			gw.executeGraphOpInLock(op)
			gw.mu.Unlock()
		case opTypeRead:
			gw.mu.RLock()
			go func(op *graphOp) {
				defer gw.mu.RUnlock()
				gw.executeGraphOpInLock(op)
			}(op)
		}
	}
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
	done := make(chan struct{})
	ticker := time.NewTicker(warnAfter)
	go func() {
		defer ticker.Stop()
		elapsedWarns := 0
		for {
			select {
			case <-ticker.C:
				elapsedWarns++
				elapsed := time.Since(startedAt)
				log.Errorf("graph %s operation %q (%s) is running longer than %s (elapsed %s, warn #%d)", op.opType, op.desc, gw.describeOp(op), warnAfter, elapsed, elapsedWarns)
			case <-done:
				return
			}
		}
	}()

	defer func() {
		close(done)
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

func (gw *GraphWrapper[K]) Add(nodes ...hnsw.InputNode[K]) time.Duration {
	done := make(chan struct{}, 1)
	var pureUseTime time.Duration
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeWrite,
		desc:   "Add",
		params: fmt.Sprintf("nodes_count=%d", len(nodes)),
		fn: func() {
			start := time.Now()
			defer func() {
				pureUseTime = time.Since(start)
			}()
			defer close(done)
			gw.graph.Add(nodes...)
		},
	})
	<-done
	return pureUseTime
}

func (gw *GraphWrapper[K]) Delete(uids ...K) {
	done := make(chan struct{}, 1)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeWrite,
		desc:   "Delete",
		params: fmt.Sprintf("uids=%v", uids),
		fn: func() {
			defer close(done)
			for _, uid := range uids {
				gw.graph.Delete(uid)
			}

		},
	})
	<-done
}

func (gw *GraphWrapper[K]) SearchWithDistanceAndFilter(near []float32, k int, filter hnsw.FilterFunc[K]) []hnsw.SearchResult[K] {
	resultChan := make(chan []hnsw.SearchResult[K], 2)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeRead,
		desc:   "SearchWithDistanceAndFilter",
		params: fmt.Sprintf("k=%d, near_len=%d, has_filter=%t", k, len(near), filter != nil),
		fn: func() {
			results := gw.graph.SearchWithDistanceAndFilter(near, k, filter)
			resultChan <- results
		},
	})
	return <-resultChan
}

func (gw *GraphWrapper[K]) Has(docId K) bool {
	resultChan := make(chan bool, 2)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeRead,
		desc:   "Has",
		params: fmt.Sprintf("doc_id=%v", docId),
		fn: func() {
			resultChan <- gw.graph.Has(docId)
		},
	})
	return <-resultChan
}

func (gw *GraphWrapper[K]) GetSize() int {
	resultChan := make(chan int, 2)
	gw.operationChannel.SafeFeed(&graphOp{
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
	})
	return <-resultChan
}

func (gw *GraphWrapper[K]) GetLayerLength() int {
	resultChan := make(chan int, 2)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeRead,
		desc:   "GetLayerLength",
		params: "none",
		fn: func() {
			resultChan <- len(gw.graph.Layers)
		},
	})
	return <-resultChan
}

func (gw *GraphWrapper[K]) TrainPQCodebookFromDataWithCallback(m, k int, callback func(key K, code []byte, vector []float64) (hnswspec.LayerNode[K], error)) (*pq.Codebook, error) {
	var codebook *pq.Codebook
	var err error
	done := make(chan struct{}, 1)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeWrite,
		desc:   "TrainPQCodebookFromDataWithCallback",
		params: fmt.Sprintf("m=%d,k=%d", m, k),
		fn: func() {
			defer close(done)
			codebook, err = gw.graph.TrainPQCodebookFromDataWithCallback(m, k, callback)
		},
	})
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
