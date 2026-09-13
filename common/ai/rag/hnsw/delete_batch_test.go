package hnsw

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMUSTPASS_DeleteBatchTopologyAndRollback(t *testing.T) {
	g := NewGraph[string]()
	rng := rand.New(rand.NewSource(1))
	vectors := make(map[string][]float32)
	var ids []string
	for i := 0; i < 256; i++ {
		id := fmt.Sprint(i)
		v := make([]float32, 16)
		for j := range v {
			v[j] = rng.Float32()
		}
		vectors[id] = v
		g.Add(InputNode[string]{Key: id, Value: v})
		if i%2 == 0 {
			ids = append(ids, id)
		}
	}
	topology := func() []map[string][]string {
		layers := make([]map[string][]string, len(g.Layers))
		for i, layer := range g.Layers {
			layers[i] = make(map[string][]string)
			for key, node := range layer.Nodes {
				keys := make([]string, 0, len(node.GetNeighbors()))
				for neighbor := range node.GetNeighbors() {
					keys = append(keys, neighbor)
				}
				sort.Strings(keys)
				layers[i][key] = keys
			}
		}
		return layers
	}
	before := topology()
	_, err := g.DeleteBatchWithCommit(ids, func() error { return errors.New("disk full") })
	require.ErrorContains(t, err, "disk full")
	require.Equal(t, before, topology(), "rollback must restore every edge and layer")
	require.Panics(t, func() { _, _ = g.DeleteBatchWithCommit(ids, func() error { panic("commit panic") }) })
	require.Equal(t, before, topology())
	calls := 0
	g.OnLayersChange = func([]*Layer[string]) { calls++ }
	require.True(t, g.DeleteBatch(ids...))
	require.Equal(t, 1, calls)
	require.False(t, g.DeleteBatch(ids...))
	require.Equal(t, 1, calls, "missing IDs must not cause another full export")
	for _, layer := range g.Layers {
		for key, node := range layer.Nodes {
			for neighbor := range node.GetNeighbors() {
				_, present := layer.Nodes[neighbor]
				require.True(t, present, "dangling edge %s -> %s", key, neighbor)
			}
		}
	}
	hits := 0
	for key, v := range vectors {
		if !g.Has(key) {
			continue
		}
		results := g.Search(v, 1)
		if len(results) > 0 && results[0].Key == key {
			hits++
		}
	}
	require.GreaterOrEqual(t, hits, 120, "survivors must remain searchable after deleting half the graph")
}
