package hnswspec

import (
	"cmp"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
)

// Vector 向量类型定义
type Vector = func() []float32

// LayerNode 图节点接口，支持不同的实现（普通HNSW和PQ优化）
type LayerNode[K cmp.Ordered] interface {
	// GetKey 获取节点键
	GetKey() K

	// GetVector 获取原始向量（用于精确距离计算）
	GetVector() Vector

	// GetNeighbors 获取邻居节点映射
	GetNeighbors() map[K]LayerNode[K]

	// AddNeighbor 添加邻居节点
	AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K])

	// RemoveNeighbor 移除邻居节点
	RemoveNeighbor(key K)

	// Isolate 从图中孤立节点
	Isolate(neighbors map[K]LayerNode[K], m int, distFunc DistanceFunc[K])

	// Replenish 恢复连接性
	Replenish(m int, distFunc DistanceFunc[K])

	// IsPQEnabled 是否启用PQ优化
	IsPQEnabled() bool

	// GetPQCodes 获取PQ编码（仅PQ节点有效）
	GetPQCodes() ([]byte, bool)
}

// DistanceFunc 距离计算函数，基于节点接口
type DistanceFunc[K cmp.Ordered] func(a, b LayerNode[K]) float64

// PQAwareDistanceFunc PQ感知的距离计算函数，可以访问quantizer
type PQAwareDistanceFunc[K cmp.Ordered] func(a, b LayerNode[K], quantizer *pq.Quantizer) float64
