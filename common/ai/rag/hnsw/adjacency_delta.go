package hnsw

import (
	"cmp"
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

// AdjacencyChange is one node's neighborhood after a graph mutation.
// A deleted change removes the node from that layer. An upsert replaces that
// node's neighbor list. Replaying the same change twice is a no-op.
type AdjacencyChange[K cmp.Ordered] struct {
	Layer     int
	Key       K
	Neighbors []K
	Deleted   bool
}

type adjacencyRef[K cmp.Ordered] struct {
	layer int
	key   K
}

// EnableAdjacencyTracking records neighborhood edits so callers can persist
// an incremental snapshot instead of the whole graph.
func (g *Graph[K]) EnableAdjacencyTracking() {
	if g == nil {
		return
	}
	g.trackAdjacency = true
	if g.adjacencyDirty == nil {
		g.adjacencyDirty = map[adjacencyRef[K]]bool{}
	}
}

func (g *Graph[K]) noteUpsert(layer int, key K) {
	if g == nil || !g.trackAdjacency {
		return
	}
	if g.adjacencyDirty == nil {
		g.adjacencyDirty = map[adjacencyRef[K]]bool{}
	}
	g.adjacencyDirty[adjacencyRef[K]{layer: layer, key: key}] = false
}

func (g *Graph[K]) noteDelete(layer int, key K) {
	if g == nil || !g.trackAdjacency {
		return
	}
	if g.adjacencyDirty == nil {
		g.adjacencyDirty = map[adjacencyRef[K]]bool{}
	}
	g.adjacencyDirty[adjacencyRef[K]{layer: layer, key: key}] = true
}

// TakeAdjacencyChanges returns and clears neighborhood edits since the last take.
func (g *Graph[K]) TakeAdjacencyChanges() []AdjacencyChange[K] {
	if g == nil || len(g.adjacencyDirty) == 0 {
		return nil
	}
	changes := make([]AdjacencyChange[K], 0, len(g.adjacencyDirty))
	for ref, deleted := range g.adjacencyDirty {
		if deleted || ref.layer < 0 || ref.layer >= len(g.Layers) || g.Layers[ref.layer] == nil {
			changes = append(changes, AdjacencyChange[K]{Layer: ref.layer, Key: ref.key, Deleted: true})
			continue
		}
		node, ok := g.Layers[ref.layer].Nodes[ref.key]
		if !ok || node == nil {
			changes = append(changes, AdjacencyChange[K]{Layer: ref.layer, Key: ref.key, Deleted: true})
			continue
		}
		neighbors := make([]K, 0, len(node.GetNeighbors()))
		for key := range node.GetNeighbors() {
			neighbors = append(neighbors, key)
		}
		sort.Slice(neighbors, func(i, j int) bool { return neighbors[i] < neighbors[j] })
		changes = append(changes, AdjacencyChange[K]{Layer: ref.layer, Key: ref.key, Neighbors: neighbors})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Deleted != changes[j].Deleted {
			return changes[i].Deleted
		}
		if changes[i].Layer != changes[j].Layer {
			return changes[i].Layer < changes[j].Layer
		}
		return changes[i].Key < changes[j].Key
	})
	g.adjacencyDirty = map[adjacencyRef[K]]bool{}
	return changes
}

// RestoreAdjacencyChanges puts previously taken edits back so a failed persist can retry.
func (g *Graph[K]) RestoreAdjacencyChanges(changes []AdjacencyChange[K]) {
	if g == nil || !g.trackAdjacency {
		return
	}
	for _, change := range changes {
		if change.Deleted {
			g.noteDelete(change.Layer, change.Key)
			continue
		}
		g.noteUpsert(change.Layer, change.Key)
	}
}

// ApplyAdjacencyChanges replays an incremental snapshot onto this graph.
// create is used only when an upsert names a node that is not on that layer yet.
func (g *Graph[K]) ApplyAdjacencyChanges(changes []AdjacencyChange[K], create func(K) (hnswspec.LayerNode[K], error)) error {
	if g == nil || len(changes) == 0 {
		return nil
	}
	tracking := g.trackAdjacency
	g.trackAdjacency = false
	defer func() { g.trackAdjacency = tracking }()

	for _, change := range changes {
		if !change.Deleted {
			for len(g.Layers) <= change.Layer {
				g.Layers = append(g.Layers, &Layer[K]{Nodes: map[K]hnswspec.LayerNode[K]{}})
			}
			if _, ok := g.Layers[change.Layer].Nodes[change.Key]; ok {
				continue
			}
			if create == nil {
				return fmt.Errorf("hnsw adjacency delta references missing node %v", change.Key)
			}
			node, err := create(change.Key)
			if err != nil {
				return err
			}
			if g.Layers[change.Layer].Nodes == nil {
				g.Layers[change.Layer].Nodes = map[K]hnswspec.LayerNode[K]{}
			}
			g.Layers[change.Layer].Nodes[change.Key] = node
		}
	}

	for _, change := range changes {
		if change.Layer < 0 || change.Layer >= len(g.Layers) || g.Layers[change.Layer] == nil {
			continue
		}
		layer := g.Layers[change.Layer]
		if change.Deleted {
			delete(layer.Nodes, change.Key)
			for _, node := range layer.Nodes {
				if node != nil {
					node.RemoveNeighbor(change.Key)
				}
			}
			continue
		}
		node := layer.Nodes[change.Key]
		if node == nil {
			continue
		}
		neighbors := node.GetNeighbors()
		for key := range neighbors {
			delete(neighbors, key)
		}
		for _, neighborKey := range change.Neighbors {
			neighbor, ok := layer.Nodes[neighborKey]
			if !ok || neighbor == nil || neighborKey == change.Key {
				continue
			}
			node.AddSingleNeighbor(neighbor)
		}
	}

	for len(g.Layers) > 0 && (g.Layers[len(g.Layers)-1] == nil || len(g.Layers[len(g.Layers)-1].Nodes) == 0) {
		g.Layers = g.Layers[:len(g.Layers)-1]
	}
	return nil
}
