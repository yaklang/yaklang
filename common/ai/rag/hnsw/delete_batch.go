package hnsw

import (
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

// DeleteBatch removes all targets before repairing surviving neighborhoods.
// This is essential for lazy vectors: a batch's database rows may already be
// gone, so neither repair nor the change callback may visit another target.
func (h *Graph[K]) DeleteBatch(keys ...K) bool {
	deleted, _ := h.DeleteBatchWithCommit(keys, nil)
	return deleted
}

// DeleteBatchWithCommit runs commit with the updated graph. On error or panic,
// it restores the original topology. The caller must serialize graph access.
// A non-nil commit replaces OnLayersChange and can atomically persist the graph
// with its backing rows; it must not reenter the graph's external lock.
func (h *Graph[K]) DeleteBatchWithCommit(keys []K, commit func() error) (deleted bool, err error) {
	targets := make(map[K]struct{}, len(keys))
	for _, key := range keys {
		targets[key] = struct{}{}
	}
	originalLayers := h.Layers
	type savedNode struct {
		node      hnswspec.LayerNode[K]
		neighbors map[K]hnswspec.LayerNode[K]
	}
	var saved []savedNode
	removed := make([]map[K]hnswspec.LayerNode[K], len(h.Layers))
	committed := false
	if commit != nil {
		// Repair may add reciprocal edges, so save every survivor's adjacency,
		// without loading any vector or PQ code.
		for _, layer := range h.Layers {
			for key, node := range layer.Nodes {
				if _, drop := targets[key]; drop {
					continue
				}
				neighbors := make(map[K]hnswspec.LayerNode[K], len(node.GetNeighbors()))
				for k, n := range node.GetNeighbors() {
					neighbors[k] = n
				}
				saved = append(saved, savedNode{node, neighbors})
			}
		}
		defer func() {
			if committed {
				return
			}
			h.Layers = originalLayers
			for i, nodes := range removed {
				for key, node := range nodes {
					h.Layers[i].Nodes[key] = node
				}
			}
			for _, state := range saved {
				neighbors := state.node.GetNeighbors()
				clear(neighbors)
				for key, node := range state.neighbors {
					neighbors[key] = node
				}
			}
		}()
	}

	for i, layer := range h.Layers {
		for key := range targets {
			if node, ok := layer.Nodes[key]; ok {
				if commit != nil {
					if removed[i] == nil {
						removed[i] = make(map[K]hnswspec.LayerNode[K])
					}
					removed[i][key] = node
				}
				delete(layer.Nodes, key)
				deleted = true
			}
		}
		var affected []hnswspec.LayerNode[K]
		// Scan incoming edges once per layer, rather than once per deleted key.
		for _, node := range layer.Nodes {
			changed := false
			for key := range node.GetNeighbors() {
				if _, drop := targets[key]; drop {
					node.RemoveNeighbor(key)
					changed = true
				}
			}
			if changed {
				affected = append(affected, node)
			}
		}
		// Every target and incoming edge in this layer is gone before repair.
		for _, node := range affected {
			node.Replenish(h.M, h.nodeDistance)
		}
	}
	for len(h.Layers) > 0 && len(h.Layers[len(h.Layers)-1].Nodes) == 0 {
		h.Layers = h.Layers[:len(h.Layers)-1]
	}
	if commit != nil {
		if err := commit(); err != nil {
			return false, err
		}
	} else if deleted && h.OnLayersChange != nil {
		h.OnLayersChange(h.Layers)
	}
	committed = true
	return deleted, nil
}
