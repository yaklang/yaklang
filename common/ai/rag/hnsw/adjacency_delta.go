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
	if g == nil {
		return
	}
	g.saveAdjacency(layer, key)
	if !g.trackAdjacency {
		return
	}
	if g.adjacencyDirty == nil {
		g.adjacencyDirty = map[adjacencyRef[K]]bool{}
	}
	g.adjacencyDirty[adjacencyRef[K]{layer: layer, key: key}] = false
}

func (g *Graph[K]) noteDelete(layer int, key K) {
	if g == nil {
		return
	}
	g.saveAdjacency(layer, key)
	if !g.trackAdjacency {
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
	// A caller may supply several mutations at once, including delete/reinsert
	// cycles. Only the last neighborhood for a layer/key is authoritative.
	latest := make(map[adjacencyRef[K]]AdjacencyChange[K], len(changes))
	for _, change := range changes {
		if change.Layer < 0 || change.Layer >= 1<<16 {
			return fmt.Errorf("invalid hnsw adjacency layer %d", change.Layer)
		}
		latest[adjacencyRef[K]{layer: change.Layer, key: change.Key}] = change
	}
	tracking := g.trackAdjacency
	g.trackAdjacency = false
	defer func() { g.trackAdjacency = tracking }()

	deleted := make(map[int]map[K]struct{})
	for ref, change := range latest {
		if change.Deleted {
			if ref.layer < len(g.Layers) && g.Layers[ref.layer] != nil {
				delete(g.Layers[ref.layer].Nodes, ref.key)
				if deleted[ref.layer] == nil {
					deleted[ref.layer] = make(map[K]struct{})
				}
				deleted[ref.layer][ref.key] = struct{}{}
			}
			continue
		}
		for len(g.Layers) <= ref.layer {
			g.Layers = append(g.Layers, &Layer[K]{Nodes: make(map[K]hnswspec.LayerNode[K])})
		}
		if g.Layers[ref.layer] == nil {
			g.Layers[ref.layer] = &Layer[K]{Nodes: make(map[K]hnswspec.LayerNode[K])}
		}
		layer := g.Layers[ref.layer]
		if layer.Nodes[ref.key] != nil {
			continue
		}
		if create == nil {
			return fmt.Errorf("hnsw adjacency delta references missing node %v", ref.key)
		}
		node, err := create(ref.key)
		if err != nil {
			return err
		}
		if node == nil || node.GetKey() != ref.key {
			return fmt.Errorf("invalid hnsw adjacency node %v", ref.key)
		}
		if layer.Nodes == nil {
			layer.Nodes = make(map[K]hnswspec.LayerNode[K])
		}
		layer.Nodes[ref.key] = node
	}
	// Scan each affected layer once, rather than once per deleted node.
	for level, keys := range deleted {
		for _, node := range g.Layers[level].Nodes {
			for key := range node.GetNeighbors() {
				if _, drop := keys[key]; drop {
					node.RemoveNeighbor(key)
				}
			}
		}
	}
	for ref, change := range latest {
		if change.Deleted {
			continue
		}
		layer := g.Layers[ref.layer]
		node := layer.Nodes[ref.key]
		clear(node.GetNeighbors())
		for _, key := range change.Neighbors {
			neighbor := layer.Nodes[key]
			if neighbor == nil || key == ref.key {
				return fmt.Errorf("invalid hnsw adjacency neighbor %v for node %v on layer %d", key, ref.key, ref.layer)
			}
			node.AddSingleNeighbor(neighbor)
		}
	}
	for len(g.Layers) > 0 && (g.Layers[len(g.Layers)-1] == nil || len(g.Layers[len(g.Layers)-1].Nodes) == 0) {
		g.Layers = g.Layers[:len(g.Layers)-1]
	}
	return nil
}
