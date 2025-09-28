package hnsw

import (
	"cmp"
	"runtime"
	"sync"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

// ParallelDistanceResult 并行距离计算的结果
type ParallelDistanceResult[K cmp.Ordered] struct {
	Index    int
	Node     hnswspec.LayerNode[K]
	Distance float64
}

// ParallelDistanceCalculator 并行距离计算器
type ParallelDistanceCalculator[K cmp.Ordered] struct {
	workerCount int
	taskChan    chan distanceTask[K]
	resultChan  chan ParallelDistanceResult[K]
	wg          sync.WaitGroup
}

type distanceTask[K cmp.Ordered] struct {
	Index      int
	NodeA      hnswspec.LayerNode[K]
	NodeB      hnswspec.LayerNode[K]
	DistFunc   hnswspec.DistanceFunc[K]
	ResultChan chan ParallelDistanceResult[K]
}

// NewParallelDistanceCalculator 创建并行距离计算器
func NewParallelDistanceCalculator[K cmp.Ordered]() *ParallelDistanceCalculator[K] {
	workerCount := runtime.NumCPU()
	if workerCount > 8 {
		workerCount = 8 // 限制最大worker数量，避免过多goroutine开销
	}

	return &ParallelDistanceCalculator[K]{
		workerCount: workerCount,
		taskChan:    make(chan distanceTask[K], workerCount*2),
		resultChan:  make(chan ParallelDistanceResult[K], workerCount*2),
	}
}

// Start 启动并行计算器
func (calc *ParallelDistanceCalculator[K]) Start() {
	for i := 0; i < calc.workerCount; i++ {
		calc.wg.Add(1)
		go calc.worker()
	}
}

// Stop 停止并行计算器
func (calc *ParallelDistanceCalculator[K]) Stop() {
	close(calc.taskChan)
	calc.wg.Wait()
	close(calc.resultChan)
}

// worker goroutine工作函数
func (calc *ParallelDistanceCalculator[K]) worker() {
	defer calc.wg.Done()
	for task := range calc.taskChan {
		dist := task.DistFunc(task.NodeA, task.NodeB)
		result := ParallelDistanceResult[K]{
			Index:    task.Index,
			Node:     task.NodeA,
			Distance: dist,
		}
		task.ResultChan <- result
	}
}

// CalculateDistancesParallel 并行计算多个节点到目标节点的距离
func (calc *ParallelDistanceCalculator[K]) CalculateDistancesParallel(
	nodes []hnswspec.LayerNode[K],
	target hnswspec.LayerNode[K],
	distFunc hnswspec.DistanceFunc[K],
) []ParallelDistanceResult[K] {
	nodeCount := len(nodes)
	if nodeCount == 0 {
		return nil
	}

	// 对于少量节点，直接串行计算更高效
	if nodeCount < calc.workerCount*2 {
		results := make([]ParallelDistanceResult[K], nodeCount)
		for i, node := range nodes {
			dist := distFunc(node, target)
			results[i] = ParallelDistanceResult[K]{
				Index:    i,
				Node:     node,
				Distance: dist,
			}
		}
		return results
	}

	// 并行计算
	resultChan := make(chan ParallelDistanceResult[K], nodeCount)

	// 分发任务
	for i, node := range nodes {
		task := distanceTask[K]{
			Index:      i,
			NodeA:      node,
			NodeB:      target,
			DistFunc:   distFunc,
			ResultChan: resultChan,
		}
		calc.taskChan <- task
	}

	// 收集结果
	results := make([]ParallelDistanceResult[K], nodeCount)
	for i := 0; i < nodeCount; i++ {
		result := <-resultChan
		results[result.Index] = result
	}

	return results
}

// ParallelDistanceCalculation 简化的并行距离计算函数
func ParallelDistanceCalculation[K cmp.Ordered](
	nodes []hnswspec.LayerNode[K],
	target hnswspec.LayerNode[K],
	distFunc hnswspec.DistanceFunc[K],
) []ParallelDistanceResult[K] {
	nodeCount := len(nodes)
	if nodeCount == 0 {
		return nil
	}

	// 对于少量节点（< 16），直接串行计算
	threshold := 16
	if nodeCount < threshold {
		results := make([]ParallelDistanceResult[K], nodeCount)
		for i, node := range nodes {
			dist := distFunc(node, target)
			results[i] = ParallelDistanceResult[K]{
				Index:    i,
				Node:     node,
				Distance: dist,
			}
		}
		return results
	}

	// 并行计算
	workerCount := runtime.NumCPU()
	if workerCount > nodeCount {
		workerCount = nodeCount
	}
	if workerCount > 8 {
		workerCount = 8 // 限制最大worker数量
	}

	results := make([]ParallelDistanceResult[K], nodeCount)
	var wg sync.WaitGroup

	batchSize := (nodeCount + workerCount - 1) / workerCount

	for w := 0; w < workerCount; w++ {
		start := w * batchSize
		end := start + batchSize
		if end > nodeCount {
			end = nodeCount
		}
		if start >= nodeCount {
			break
		}

		wg.Add(1)
		go func(startIdx, endIdx int) {
			defer wg.Done()
			for i := startIdx; i < endIdx; i++ {
				dist := distFunc(nodes[i], target)
				results[i] = ParallelDistanceResult[K]{
					Index:    i,
					Node:     nodes[i],
					Distance: dist,
				}
			}
		}(start, end)
	}

	wg.Wait()
	return results
}
