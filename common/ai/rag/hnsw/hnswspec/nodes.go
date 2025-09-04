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

func (n *StandardLayerNode[K]) Isolate(m int, distFunc DistanceFunc[K]) {
	for _, neighbor := range n.neighbors {
		neighbor.RemoveNeighbor(n.key)
	}

	for _, neighbor := range n.neighbors {
		neighbor.Replenish(m, distFunc)
	}
}

func (n *StandardLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	if len(n.neighbors) >= m {
		return
	}

	// 通过添加新邻居恢复连接性
	for _, neighbor := range n.neighbors {
		for key, candidate := range neighbor.GetNeighbors() {
			if _, ok := n.neighbors[key]; ok {
				continue // 不添加重复项
			}
			if candidate.GetKey() == n.key {
				continue
			}
			n.AddNeighbor(candidate, m, distFunc)
			if len(n.neighbors) >= m {
				return
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

func (n *PQLayerNode[K]) Isolate(m int, distFunc DistanceFunc[K]) {
	for _, neighbor := range n.neighbors {
		neighbor.RemoveNeighbor(n.key)
	}

	for _, neighbor := range n.neighbors {
		neighbor.Replenish(m, distFunc)
	}
}

func (n *PQLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	if len(n.neighbors) >= m {
		return
	}

	// 通过添加新邻居恢复连接性
	for _, neighbor := range n.neighbors {
		for key, candidate := range neighbor.GetNeighbors() {
			if _, ok := n.neighbors[key]; ok {
				continue // 不添加重复项
			}
			if candidate.GetKey() == n.key {
				continue
			}
			n.AddNeighbor(candidate, m, distFunc)
			if len(n.neighbors) >= m {
				return
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
