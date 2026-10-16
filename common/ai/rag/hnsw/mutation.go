package hnsw

import (
	"cmp"
	"fmt"
	"maps"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

// A mutation saves only neighborhoods that are about to change. Inserting a
// node must not copy the entire graph just to support a failed database commit.
type graphMutation[K cmp.Ordered] struct {
	layers   []*Layer[K]
	dirty    map[adjacencyRef[K]]bool
	nodeType InputNodeType
	saved    map[adjacencyRef[K]]savedAdjacency[K]
}

type savedAdjacency[K cmp.Ordered] struct {
	node      hnswspec.LayerNode[K]
	neighbors map[K]hnswspec.LayerNode[K]
}

func (g *Graph[K]) beginMutation() *graphMutation[K] {
	if g.mutation != nil {
		panic("nested hnsw mutation")
	}
	mutation := &graphMutation[K]{
		layers: append([]*Layer[K](nil), g.Layers...),
		dirty:  maps.Clone(g.adjacencyDirty), nodeType: g.nodeType,
		saved: make(map[adjacencyRef[K]]savedAdjacency[K]),
	}
	g.mutation = mutation
	return mutation
}

func (g *Graph[K]) saveAdjacency(layer int, key K) {
	if g.mutation == nil {
		return
	}
	ref := adjacencyRef[K]{layer: layer, key: key}
	if _, saved := g.mutation.saved[ref]; saved {
		return
	}
	var state savedAdjacency[K]
	if layer < len(g.Layers) && g.Layers[layer] != nil {
		state.node = g.Layers[layer].Nodes[key]
		if state.node != nil {
			state.neighbors = maps.Clone(state.node.GetNeighbors())
		}
	}
	g.mutation.saved[ref] = state
}

func (g *Graph[K]) finishMutation(mutation *graphMutation[K], committed bool) {
	g.mutation = nil
	if committed {
		return
	}
	g.Layers, g.adjacencyDirty, g.nodeType = mutation.layers, mutation.dirty, mutation.nodeType
	for ref, state := range mutation.saved {
		if ref.layer >= len(g.Layers) || g.Layers[ref.layer] == nil {
			continue
		}
		if state.node == nil {
			delete(g.Layers[ref.layer].Nodes, ref.key)
			continue
		}
		g.Layers[ref.layer].Nodes[ref.key] = state.node
		neighbors := state.node.GetNeighbors()
		clear(neighbors)
		maps.Copy(neighbors, state.neighbors)
	}
}

// AddWithCommit inserts nodes, then commits their backing rows and topology.
// Errors and panics restore both the graph and its pending adjacency changes.
// Callers must serialize graph access; commit must not reenter the graph.
func (g *Graph[K]) AddWithCommit(nodes []InputNode[K], commit func() error) (err error) {
	mutation := g.beginMutation()
	callback := g.OnLayersChange
	g.OnLayersChange = nil
	defer func() {
		g.OnLayersChange = callback
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("add graph nodes: %v", recovered)
		}
		g.finishMutation(mutation, err == nil)
	}()
	if err = g.add(nodes...); err != nil {
		return err
	}
	if commit != nil {
		return commit()
	}
	if callback != nil {
		callback(g.Layers)
	}
	return nil
}

func (g *Graph[K]) replenish(layer int, node hnswspec.LayerNode[K]) {
	notify := func(changed hnswspec.LayerNode[K]) { g.noteUpsert(layer, changed.GetKey()) }
	if tracked, ok := node.(interface {
		ReplenishWithChange(int, hnswspec.DistanceFunc[K], func(hnswspec.LayerNode[K]))
	}); ok {
		tracked.ReplenishWithChange(g.M, g.nodeDistance, notify)
		return
	}
	// Preserve compatibility with external LayerNode implementations, whose
	// repair method may change any reciprocal neighborhood in the layer.
	for key := range g.Layers[layer].Nodes {
		g.noteUpsert(layer, key)
	}
	node.Replenish(g.M, g.nodeDistance)
}
