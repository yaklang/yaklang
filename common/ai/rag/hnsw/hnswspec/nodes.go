package hnswspec

import (
	"cmp"
	"math"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
)

// StandardLayerNode 标准HNSW层节点（无PQ优化）
type StandardLayerNode[K cmp.Ordered] struct {
	key       K
	vector    Vector
	neighbors map[K]LayerNode[K]
}

// NewStandardLayerNode 创建标准层节点
func NewStandardLayerNode[K cmp.Ordered](key K, vector Vector) *StandardLayerNode[K] {
	return &StandardLayerNode[K]{
		key:       key,
		vector:    vector,
		neighbors: make(map[K]LayerNode[K]),
	}
}

func (n *StandardLayerNode[K]) GetKey() K {
	return n.key
}

func (n *StandardLayerNode[K]) GetVector() Vector {
	return n.vector
}

func (n *StandardLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	return n.neighbors
}

func (n *StandardLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
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
		if d > worstDist || worst == nil {
			worstDist = d
			worst = neighborNode
		}
	}

	delete(n.neighbors, worst.GetKey())
	// 删除反向链接
	worst.RemoveNeighbor(n.key)
	worst.Replenish(m, distFunc)
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

func (n *StandardLayerNode[K]) IsPQEnabled() bool {
	return false
}

func (n *StandardLayerNode[K]) GetPQCodes() ([]byte, bool) {
	return nil, false
}

// PQLayerNode PQ优化的HNSW层节点（不存储原始向量数据）
type PQLayerNode[K cmp.Ordered] struct {
	key       K
	pqCodes   []byte // PQ编码
	neighbors map[K]LayerNode[K]
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
		key:       key,
		pqCodes:   pqCodes,
		neighbors: make(map[K]LayerNode[K]),
	}

	return node, nil
}

func (n *PQLayerNode[K]) GetKey() K {
	return n.key
}

func (n *PQLayerNode[K]) GetVector() Vector {
	// PQ节点不存储原始向量，返回nil或抛出错误
	// 在实际使用中应该使用PQ编码进行距离计算
	return func() []float32 {
		panic("PQ node does not store original vector data. Use PQ codes for distance calculation.")
	}
}

func (n *PQLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	return n.neighbors
}

func (n *PQLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
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
		if d > worstDist || worst == nil {
			worstDist = d
			worst = neighborNode
		}
	}

	delete(n.neighbors, worst.GetKey())
	// 删除反向链接
	worst.RemoveNeighbor(n.key)
	worst.Replenish(m, distFunc)
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
	return n.pqCodes, true
}
