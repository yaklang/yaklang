package vectorstore

import (
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

func updateDatabaseGraphInfoInLock(db *gorm.DB, id uint, wrapper *GraphWrapper[string]) error {
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
	err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", id).Update("graph_binary", graphInfosBytes).Error
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
		err = db.Model(&schema.VectorStoreCollection{}).Where("id = ?", id).Update("code_book_binary", codebookBytes).Error
		if err != nil {
			return utils.Wrap(err, "update codebook")
		}
	}
	return nil
}
