package hnswspec

import (
	"cmp"

	"github.com/yaklang/yaklang/common/log"
)

type LazyNodeID any

type LazyLayerNode[K cmp.Ordered] struct {
	uid          LazyNodeID
	nodeCacheErr error
	nodeGetter   func(uid LazyNodeID) (LayerNode[K], error)
}

var _ LayerNode[string] = (*LazyLayerNode[string])(nil)

func NewLazyLayerNode[K cmp.Ordered](uid LazyNodeID, nodeGetter func(uid LazyNodeID) (LayerNode[K], error)) *LazyLayerNode[K] {
	return &LazyLayerNode[K]{uid: uid, nodeGetter: nodeGetter}
}

func (n *LazyLayerNode[K]) GetUID() LazyNodeID {
	return n.uid
}

func (n *LazyLayerNode[K]) LoadNode() LayerNode[K] {
	node, err := n.nodeGetter(n.GetUID())
	n.nodeCacheErr = err
	if err != nil {
		log.Errorf("LazyLayerNode.LoadNode failed for uid=%v: %v", n.uid, err)
		return nil
	}
	if node == nil {
		log.Errorf("LazyLayerNode.LoadNode returned nil for uid=%v", n.uid)
	}
	return node
}

func (n *LazyLayerNode[K]) GetVector() Vector {
	node := n.LoadNode()
	if node == nil {
		log.Warnf("LazyLayerNode.GetVector: node is nil for uid=%v, returning empty vector", n.uid)
		return func() []float32 { return nil }
	}
	return node.GetVector()
}

func (n *LazyLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	node := n.LoadNode()
	if node == nil {
		log.Warnf("LazyLayerNode.GetNeighbors: node is nil for uid=%v, returning empty map", n.uid)
		return make(map[K]LayerNode[K])
	}
	return node.GetNeighbors()
}

func (n *LazyLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.AddNeighbor: node is nil for uid=%v, skipping", n.uid)
		return
	}
	node.AddNeighbor(neighbor, m, distFunc)
}

func (n *LazyLayerNode[K]) GetKey() K {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.GetKey: node is nil for uid=%v, returning zero value", n.uid)
		var zero K
		return zero
	}
	return node.GetKey()
}

func (n *LazyLayerNode[K]) GetData() any {
	return n.GetUID()
}

func (n *LazyLayerNode[K]) GetPQCodes() ([]byte, bool) {
	node := n.LoadNode()
	if node == nil {
		log.Warnf("LazyLayerNode.GetPQCodes: node is nil for uid=%v, returning false", n.uid)
		return nil, false
	}
	return node.GetPQCodes()
}

func (n *LazyLayerNode[K]) IsPQEnabled() bool {
	node := n.LoadNode()
	if node == nil {
		log.Warnf("LazyLayerNode.IsPQEnabled: node is nil for uid=%v, returning false", n.uid)
		return false
	}
	return node.IsPQEnabled()
}

func (n *LazyLayerNode[K]) Isolate(neighbors map[K]LayerNode[K], m int, distFunc DistanceFunc[K]) {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.Isolate: node is nil for uid=%v, skipping", n.uid)
		return
	}
	node.Isolate(neighbors, m, distFunc)
}

func (n *LazyLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.Replenish: node is nil for uid=%v, skipping", n.uid)
		return
	}
	node.Replenish(m, distFunc)
}

func (n *LazyLayerNode[K]) RemoveNeighbor(key K) {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.RemoveNeighbor: node is nil for uid=%v, skipping", n.uid)
		return
	}
	node.RemoveNeighbor(key)
}

func (n *LazyLayerNode[K]) AddSingleNeighbor(neighbor LayerNode[K]) {
	node := n.LoadNode()
	if node == nil {
		log.Errorf("LazyLayerNode.AddSingleNeighbor: node is nil for uid=%v, skipping", n.uid)
		return
	}
	node.AddSingleNeighbor(neighbor)
}
