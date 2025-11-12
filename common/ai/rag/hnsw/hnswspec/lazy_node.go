package hnswspec

import (
	"cmp"
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
	return node
}

func (n *LazyLayerNode[K]) GetVector() Vector {
	return n.LoadNode().GetVector()
}

func (n *LazyLayerNode[K]) GetNeighbors() map[K]LayerNode[K] {
	return n.LoadNode().GetNeighbors()
}

func (n *LazyLayerNode[K]) AddNeighbor(neighbor LayerNode[K], m int, distFunc DistanceFunc[K]) {
	n.LoadNode().AddNeighbor(neighbor, m, distFunc)
}

func (n *LazyLayerNode[K]) GetKey() K {
	return n.LoadNode().GetKey()
}
func (n *LazyLayerNode[K]) GetData() any {
	return n.GetUID()
}

func (n *LazyLayerNode[K]) GetPQCodes() ([]byte, bool) {
	return n.LoadNode().GetPQCodes()
}

func (n *LazyLayerNode[K]) IsPQEnabled() bool {
	return n.LoadNode().IsPQEnabled()
}

func (n *LazyLayerNode[K]) Isolate(neighbors map[K]LayerNode[K], m int, distFunc DistanceFunc[K]) {
	n.LoadNode().Isolate(neighbors, m, distFunc)
}

func (n *LazyLayerNode[K]) Replenish(m int, distFunc DistanceFunc[K]) {
	n.LoadNode().Replenish(m, distFunc)
}

func (n *LazyLayerNode[K]) RemoveNeighbor(key K) {
	n.LoadNode().RemoveNeighbor(key)
}

func (n *LazyLayerNode[K]) AddSingleNeighbor(neighbor LayerNode[K]) {
	n.LoadNode().AddSingleNeighbor(neighbor)
}
