package hnswspec

import (
	"cmp"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
)

// StandardLayerNode 标准HNSW层节点（无PQ优化）
type StandardLayerNode[K cmp.Ordered] struct {
	key       K
	vector    Vector
	neighbors map[K]LayerNode[K]
	normOnce  sync.Once
	norm      float64
}

// NewStandardLayerNode 创建标准层节点
func NewStandardLayerNode[K cmp.Ordered](key K, vector Vector) *StandardLayerNode[K] {
	return &StandardLayerNode[K]{
		key:       key,
		vector:    vector,
		neighbors: make(map[K]LayerNode[K]),
	}
}

func (n *StandardLayerNode[K]) SetKey(key K) {
	n.key = key
}
func (n *StandardLayerNode[K]) GetKey() K {
	return n.key
}

func (n *StandardLayerNode[K]) GetVector() Vector {
	return n.vector
}

func (n *StandardLayerNode[K]) vectorNorm(vector []float32) float64 {
	n.normOnce.Do(func() {
		for _, value := range vector {
			v := float64(value)
			n.norm += v * v
		}
		n.norm = math.Sqrt(n.norm)
	})
	return n.norm
}

// GetVectorNorm returns the cached L2 norm. Node vectors are immutable for the
// lifetime of a graph node (updates replace the node), so computing this once
// removes two full vector passes from every cosine-distance evaluation.
func (n *StandardLayerNode[K]) GetVectorNorm() float64 {
	return n.vectorNorm(n.vector())
}

// GetVectorAndNorm is an optional cosine fast path. It is intentionally not
// part of LayerNode so existing third-party node implementations remain source
// compatible.
func (n *StandardLayerNode[K]) GetVectorAndNorm() ([]float32, float64) {
	vector := n.vector()
	return vector, n.vectorNorm(vector)
}

func (n *StandardLayerNode[K]) GetCosineVectorAndNorm() ([]float32, float64, bool) {
	vector, norm := n.GetVectorAndNorm()
	return vector, norm, false
}

func (n *StandardLayerNode[K]) GetData() any {
	return n.vector
}

func (n *StandardLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	return n.neighbors
}

// HNSWPerformanceStats 收集HNSW性能统计数据
type HNSWPerformanceStats struct {
	DistanceCalculations   int64
	NeighborConnections    int64
	NeighborDisconnections int64
	GraphRestructures      int64
	CascadeUpdates         int64
}

// Global performance tracker
var globalPerformanceStats = &HNSWPerformanceStats{}

// GetGlobalPerformanceStats 获取全局性能统计
func GetGlobalPerformanceStats() *HNSWPerformanceStats {
	return globalPerformanceStats
}

// ResetGlobalPerformanceStats 重置全局性能统计
func ResetGlobalPerformanceStats() {
	atomic.StoreInt64(&globalPerformanceStats.DistanceCalculations, 0)
	atomic.StoreInt64(&globalPerformanceStats.NeighborConnections, 0)
	atomic.StoreInt64(&globalPerformanceStats.NeighborDisconnections, 0)
	atomic.StoreInt64(&globalPerformanceStats.GraphRestructures, 0)
	atomic.StoreInt64(&globalPerformanceStats.CascadeUpdates, 0)
}

// parallelDistanceResultForReplenish Replenish专用的并行距离计算结果
type parallelDistanceResultForReplenish[K cmp.Ordered] struct {
	Index    int
	Node     LayerNode[K]
	Distance float64
}

// parallelDistanceCalculationForReplenish Replenish专用的并行距离计算函数
func parallelDistanceCalculationForReplenish[K cmp.Ordered](
	candidates []LayerNode[K],
	target LayerNode[K],
	distFunc DistanceFunc[K],
) []parallelDistanceResultForReplenish[K] {
	candidateCount := len(candidates)
	if candidateCount == 0 {
		return nil
	}

	// 对于少量候选者，直接串行计算
	if candidateCount < 16 {
		results := make([]parallelDistanceResultForReplenish[K], candidateCount)
		for i, candidate := range candidates {
			dist := distFunc(candidate, target)
			results[i] = parallelDistanceResultForReplenish[K]{
				Index:    i,
				Node:     candidate,
				Distance: dist,
			}
		}
		return results
	}

	// 并行计算
	workerCount := runtime.NumCPU()
	if workerCount > candidateCount {
		workerCount = candidateCount
	}
	if workerCount > 8 {
		workerCount = 8 // 限制最大worker数量
	}

	results := make([]parallelDistanceResultForReplenish[K], candidateCount)
	var wg sync.WaitGroup

	batchSize := (candidateCount + workerCount - 1) / workerCount

	for w := 0; w < workerCount; w++ {
		start := w * batchSize
		end := start + batchSize
		if end > candidateCount {
			end = candidateCount
		}
		if start >= candidateCount {
			break
		}

		wg.Add(1)
		go func(startIdx, endIdx int) {
			defer wg.Done()
			for i := startIdx; i < endIdx; i++ {
				dist := distFunc(candidates[i], target)
				results[i] = parallelDistanceResultForReplenish[K]{
					Index:    i,
					Node:     candidates[i],
					Distance: dist,
				}
			}
		}(start, end)
	}

	wg.Wait()
	return results
}

func (n *StandardLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
	atomic.AddInt64(&globalPerformanceStats.NeighborConnections, 1)

	if n.neighbors == nil {
		n.neighbors = make(map[K]LayerNode[K], m)
	}

	n.neighbors[neighbor.GetKey()] = neighbor
	if len(n.neighbors) <= m {
		return
	}

	var (
		worstDist = math.Inf(-1)
		worst     LayerNode[K]
	)
	for _, neighborNode := range n.neighbors {
		d := distFunc(neighborNode, n)
		atomic.AddInt64(&globalPerformanceStats.DistanceCalculations, 1)
		if d > worstDist || worst == nil {
			worstDist = d
			worst = neighborNode
		}
	}
	delete(n.neighbors, worst.GetKey())
	atomic.AddInt64(&globalPerformanceStats.NeighborDisconnections, 1)

	// Pruning only updates this outgoing adjacency list. Removing and
	// replenishing the reverse edge recursively walks neighbors-of-neighbors for
	// every insertion and can also discard a useful directed traversal edge.
	// Explicit node deletion still removes incoming edges via Isolate.
	atomic.AddInt64(&globalPerformanceStats.GraphRestructures, 1)
}

func (n *StandardLayerNode[K]) AddSingleNeighbor(neighbor LayerNode[K]) {
	n.neighbors[neighbor.GetKey()] = neighbor
}

func (n *StandardLayerNode[K]) RemoveNeighbor(key K) {
	delete(n.neighbors, key)
}

func (n *StandardLayerNode[K]) Isolate(layerNodes map[K]LayerNode[K], m int, distFunc DistanceFunc[K]) {
	changedNodes := make(map[K]LayerNode[K], m)
	for _, neighbor := range layerNodes {
		if _, ok := neighbor.GetNeighbors()[n.key]; ok {
			neighbor.RemoveNeighbor(n.key)
			changedNodes[neighbor.GetKey()] = neighbor
		}
	}

	for _, neighbor := range changedNodes {
		neighbor.Replenish(m, distFunc)
	}
}

func (n *StandardLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	replenishStart := time.Now()
	defer func() {
		duration := time.Since(replenishStart)
		if duration > 200*time.Millisecond {
			log.Warnf("Replenish slow: node=%v, current_neighbors=%d, target_m=%d, duration=%v",
				n.key, len(n.neighbors), m, duration)
		}
	}()

	if len(n.neighbors) >= m {
		return
	}

	// 收集候选节点（避免在迭代过程中修改map）
	collectCandidatesStart := time.Now()
	candidates := make([]LayerNode[K], 0)
	visited := make(map[K]bool)
	visited[n.key] = true

	// 通过邻居的邻居找到候选连接
	for _, neighbor := range n.neighbors {
		visited[neighbor.GetKey()] = true
		for _, candidate := range neighbor.GetNeighbors() {
			candidateKey := candidate.GetKey()
			if visited[candidateKey] {
				continue // 避免重复和自连接
			}
			visited[candidateKey] = true
			candidates = append(candidates, candidate)
		}
	}
	collectCandidatesDuration := time.Since(collectCandidatesStart)

	// 如果没有足够的候选者，直接返回
	if len(candidates) == 0 {
		return
	}

	// 按距离排序候选者 - 使用并行计算优化
	sortCandidatesStart := time.Now()
	distanceCalls := len(candidates)

	// 创建候选者-距离对，使用并行计算距离
	type candidateWithDist struct {
		candidate LayerNode[K]
		distance  float64
	}

	candidatesWithDist := make([]candidateWithDist, len(candidates))

	// 如果候选者数量足够多，使用并行计算
	if len(candidates) >= 16 { // 并行阈值
		// 使用并行距离计算
		parallelResults := parallelDistanceCalculationForReplenish(candidates, n, distFunc)
		for i, result := range parallelResults {
			candidatesWithDist[i] = candidateWithDist{
				candidate: result.Node,
				distance:  result.Distance,
			}
		}
	} else {
		// 串行计算距离
		for i, candidate := range candidates {
			dist := distFunc(candidate, n)
			candidatesWithDist[i] = candidateWithDist{
				candidate: candidate,
				distance:  dist,
			}
		}
	}

	// 更新性能统计
	atomic.AddInt64(&globalPerformanceStats.DistanceCalculations, int64(distanceCalls))

	// 使用标准库的排序（更高效）
	slices.SortFunc(candidatesWithDist, func(a, b candidateWithDist) int {
		if a.distance < b.distance {
			return -1
		} else if a.distance > b.distance {
			return 1
		}
		return 0
	})

	// 重新组织candidates数组
	for i, cwd := range candidatesWithDist {
		candidates[i] = cwd.candidate
	}

	sortCandidatesDuration := time.Since(sortCandidatesStart)

	// 添加最近的候选者直到达到m个邻居（但避免递归调用AddNeighbor）
	addCandidatesStart := time.Now()
	addedCount := 0
	for _, candidate := range candidates {
		if len(n.neighbors) >= m {
			break
		}
		// 直接添加到neighbors map，避免递归调用AddNeighbor
		n.neighbors[candidate.GetKey()] = candidate
		addedCount++
		atomic.AddInt64(&globalPerformanceStats.NeighborConnections, 1)

		// 确保双向连接：让候选者也添加我们作为邻居
		// 但要小心避免无限递归
		candidateNeighbors := candidate.GetNeighbors()
		if candidateNeighbors != nil && len(candidateNeighbors) < m {
			// 只有在不会超过限制时才添加反向连接
			if _, exists := candidateNeighbors[n.key]; !exists {
				// 直接修改候选者的neighbors map，避免递归
				candidateNeighbors[n.key] = n
			}
		}
	}
	addCandidatesDuration := time.Since(addCandidatesStart)

	totalDuration := time.Since(replenishStart)
	if totalDuration > 1*time.Second {
		log.Warnf("Replenish PERFORMANCE: total=%v, collectCandidates=%v (candidates=%d), sortCandidates=%v (%d distance calls), addCandidates=%v (added=%d)",
			totalDuration, collectCandidatesDuration, len(candidates), sortCandidatesDuration, distanceCalls, addCandidatesDuration, addedCount)
	}
}

func (n *StandardLayerNode[K]) IsPQEnabled() bool {
	return false
}

func (n *StandardLayerNode[K]) GetPQCodes() ([]byte, bool) {
	return nil, false
}

// PQLayerNode PQ优化的HNSW层节点（不存储原始向量数据）
type PQLayerNode[K cmp.Ordered] struct {
	key          K
	pqCodeGetter func() ([]byte, error)
	neighbors    map[K]LayerNode[K]
}

// NewRawPQLayerNode 创建原始PQ编码的层节点
func NewRawPQLayerNode[K cmp.Ordered](key K, pqCodes []byte) *PQLayerNode[K] {
	node := &PQLayerNode[K]{
		key:          key,
		pqCodeGetter: func() ([]byte, error) { return pqCodes, nil },
		neighbors:    make(map[K]LayerNode[K]),
	}
	return node
}

// NewRawPQLayerNode 创建原始PQ编码的层节点
func NewLazyRawPQLayerNode[K cmp.Ordered](key K, pqCodeGetter func() ([]byte, error)) *PQLayerNode[K] {
	node := &PQLayerNode[K]{
		key:          key,
		pqCodeGetter: pqCodeGetter,
		neighbors:    make(map[K]LayerNode[K]),
	}
	return node
}

// NewPQLayerNode 创建PQ优化层节点
func NewPQLayerNode[K cmp.Ordered](key K, vector Vector, quantizer *pq.Quantizer) (*PQLayerNode[K], error) {
	// 对原始向量进行PQ编码
	vec32 := vector()
	vec64 := make([]float64, len(vec32))
	for i, v := range vec32 {
		vec64[i] = float64(v)
	}

	pqCodes, err := quantizer.Encode(vec64)
	if err != nil {
		return nil, err
	}

	node := &PQLayerNode[K]{
		key:          key,
		pqCodeGetter: func() ([]byte, error) { return pqCodes, nil },
		neighbors:    make(map[K]LayerNode[K]),
	}

	return node, nil
}

func (n *PQLayerNode[K]) GetKey() K {
	return n.key
}

func (n *PQLayerNode[K]) GetVector() Vector {
	// PQ节点不存储原始向量，返回空向量并记录警告
	// 在实际使用中应该使用PQ编码进行距离计算
	return func() []float32 {
		log.Warnf("PQLayerNode.GetVector called for key=%v, but PQ node does not store original vector data", n.key)
		return nil
	}
}

func (n *PQLayerNode[K]) GetData() any {
	codes, err := n.pqCodeGetter()
	if err != nil {
		return nil
	}
	return codes
}

func (n *PQLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	return n.neighbors
}

func (n *PQLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
	atomic.AddInt64(&globalPerformanceStats.NeighborConnections, 1)

	if n.neighbors == nil {
		n.neighbors = make(map[K]LayerNode[K], m)
	}

	n.neighbors[neighbor.GetKey()] = neighbor
	if len(n.neighbors) <= m {
		return
	}

	// 找到距离最远的邻居节点
	var (
		worstDist = math.Inf(-1)
		worst     LayerNode[K]
	)
	for _, neighborNode := range n.neighbors {
		d := distFunc(neighborNode, n)
		atomic.AddInt64(&globalPerformanceStats.DistanceCalculations, 1)
		if d > worstDist || worst == nil {
			worstDist = d
			worst = neighborNode
		}
	}

	delete(n.neighbors, worst.GetKey())
	atomic.AddInt64(&globalPerformanceStats.NeighborDisconnections, 1)

	atomic.AddInt64(&globalPerformanceStats.GraphRestructures, 1)
}

func (n *PQLayerNode[K]) RemoveNeighbor(key K) {
	delete(n.neighbors, key)
}

func (n *PQLayerNode[K]) Isolate(layerNodes map[K]LayerNode[K], m int, distFunc DistanceFunc[K]) {
	changedNodes := make(map[K]LayerNode[K], m)
	for _, neighbor := range layerNodes {
		if _, ok := neighbor.GetNeighbors()[n.key]; ok {
			neighbor.RemoveNeighbor(n.key)
			changedNodes[neighbor.GetKey()] = neighbor
		}
	}

	for _, neighbor := range changedNodes {
		neighbor.Replenish(m, distFunc)
	}
}

func (n *PQLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	if len(n.neighbors) >= m {
		return
	}

	// 收集候选节点（避免在迭代过程中修改map）
	candidates := make([]LayerNode[K], 0)
	visited := make(map[K]bool)
	visited[n.key] = true

	// 通过邻居的邻居找到候选连接
	for _, neighbor := range n.neighbors {
		visited[neighbor.GetKey()] = true
		for _, candidate := range neighbor.GetNeighbors() {
			candidateKey := candidate.GetKey()
			if visited[candidateKey] {
				continue // 避免重复和自连接
			}
			visited[candidateKey] = true
			candidates = append(candidates, candidate)
		}
	}

	// 如果没有足够的候选者，直接返回
	if len(candidates) == 0 {
		return
	}

	// 按距离排序候选者
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			distI := distFunc(candidates[i], n)
			distJ := distFunc(candidates[j], n)
			if distI > distJ {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// 添加最近的候选者直到达到m个邻居（但避免递归调用AddNeighbor）
	for _, candidate := range candidates {
		if len(n.neighbors) >= m {
			break
		}
		// 直接添加到neighbors map，避免递归调用AddNeighbor
		n.neighbors[candidate.GetKey()] = candidate

		// 确保双向连接：让候选者也添加我们作为邻居
		// 但要小心避免无限递归
		candidateNeighbors := candidate.GetNeighbors()
		if candidateNeighbors != nil && len(candidateNeighbors) < m {
			// 只有在不会超过限制时才添加反向连接
			if _, exists := candidateNeighbors[n.key]; !exists {
				// 直接修改候选者的neighbors map，避免递归
				candidateNeighbors[n.key] = n
			}
		}
	}
}

func (n *PQLayerNode[K]) IsPQEnabled() bool {
	return true
}

func (n *PQLayerNode[K]) GetPQCodes() ([]byte, bool) {
	codes, err := n.pqCodeGetter()
	if err != nil {
		return nil, false
	}
	return codes, true
}

func (n *PQLayerNode[K]) AddSingleNeighbor(neighbor LayerNode[K]) {
	n.neighbors[neighbor.GetKey()] = neighbor
}
