package vectorstore

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"io"
	"sync"
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
						return nil, utils.Wrap(err, "migrate hnsw graph")
					}
					graphBinaryReader := bytes.NewReader(collection.GraphBinary)
					hnswGraph, err = parseHNSWGraphFromBinary(db, collection, collectionConfig, graphBinaryReader)
					if err != nil {
						return nil, utils.Wrap(err, "parse hnsw graph from binary")
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
	wrapper := NewGraphWrapper(hnswGraph)

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
	fn     func()
}

type GraphWrapper[K cmp.Ordered] struct {
	graph            *hnsw.Graph[K]
	operationChannel *chanx.UnlimitedChan[*graphOp]
	mu               sync.RWMutex
}

func NewGraphWrapper[K cmp.Ordered](graph *hnsw.Graph[K]) *GraphWrapper[K] {
	wrapper := &GraphWrapper[K]{
		graph:            graph,
		operationChannel: chanx.NewUnlimitedChan[*graphOp](context.Background(), 10),
	}
	go wrapper.start()
	return wrapper
}

func (gw *GraphWrapper[K]) start() {
	for op := range gw.operationChannel.OutputChannel() {
		switch op.opType {
		case "write":
			gw.mu.Lock()
			safeCall := func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("recovered from panic in graph write operation: %v", r)
					}
					op.fn()
				}()
			}
			safeCall()
			gw.mu.Unlock()
		case "read":
			gw.mu.RLock()
			go func(readFn func()) {
				defer gw.mu.RUnlock()
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("recovered from panic in graph read operation: %v", r)
					}
					op.fn()
				}()
				readFn()
			}(op.fn)
		}
	}
}

func (gw *GraphWrapper[K]) Add(nodes ...hnsw.InputNode[K]) {
	done := make(chan struct{}, 1)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeWrite,
		fn: func() {
			defer close(done)
			gw.graph.Add(nodes...)
		},
	})
	<-done
}

func (gw *GraphWrapper[K]) Delete(uids ...K) {
	done := make(chan struct{}, 1)
	gw.operationChannel.SafeFeed(&graphOp{
		opType: opTypeWrite,
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
