package hnsw

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

func TestAdjacencyDeltaDeleteThenReinsert(t *testing.T) {
	graph := NewGraph[int]()
	changes := []AdjacencyChange[int]{
		{Key: 1}, {Key: 2, Neighbors: []int{1}},
		{Key: 1, Deleted: true}, {Key: 1, Neighbors: []int{2}},
		{Key: 2, Neighbors: []int{1}},
	}
	create := func(key int) (hnswspec.LayerNode[int], error) {
		return hnswspec.NewStandardLayerNode(key, func() []float32 { return []float32{1, 0} }), nil
	}
	require.NoError(t, graph.ApplyAdjacencyChanges(changes, create))
	require.True(t, graph.Has(1))
	require.Equal(t, map[int]struct{}{2: {}}, neighborKeySet(graph.Layers[0].Nodes[1]))
	require.Equal(t, map[int]struct{}{1: {}}, neighborKeySet(graph.Layers[0].Nodes[2]))
}

func TestAdjacencyDeltaTracksReciprocalRepair(t *testing.T) {
	graph := NewGraph[int](WithM[int](2))
	nodes := make(map[int]hnswspec.LayerNode[int])
	for key := 0; key < 4; key++ {
		vector := []float32{1, float32(key + 1)}
		nodes[key] = hnswspec.NewStandardLayerNode(key, func() []float32 { return vector })
	}
	nodes[1].AddSingleNeighbor(nodes[0])
	nodes[1].AddSingleNeighbor(nodes[2])
	nodes[2].AddSingleNeighbor(nodes[3])
	graph.Layers = []*Layer[int]{{Nodes: nodes}}
	graph.EnableAdjacencyTracking()
	for key := range nodes {
		graph.noteUpsert(0, key)
	}
	initial := graph.TakeAdjacencyChanges()
	replay := NewGraph[int](WithM[int](2))
	create := func(key int) (hnswspec.LayerNode[int], error) {
		vector := []float32{1, float32(key + 1)}
		return hnswspec.NewStandardLayerNode(key, func() []float32 { return vector }), nil
	}
	require.NoError(t, replay.ApplyAdjacencyChanges(initial, create))
	require.True(t, graph.Delete(0))
	require.Contains(t, graph.Layers[0].Nodes[3].GetNeighbors(), 1)
	require.NoError(t, replay.ApplyAdjacencyChanges(graph.TakeAdjacencyChanges(), create))
	assertSameNeighbors(t, graph, replay)
}

func TestAdjacencyDeltaRollbackRestoresTracking(t *testing.T) {
	graph := NewGraph[int](WithDeterministicRng[int](1))
	graph.EnableAdjacencyTracking()
	graph.Add(benchmarkVectors(20, 8, 7)...)
	graph.TakeAdjacencyChanges()
	_, err := graph.DeleteBatchWithCommit([]int{0, 1}, func() error {
		return fmt.Errorf("injected failure")
	})
	require.ErrorContains(t, err, "injected failure")
	require.True(t, graph.Has(0))
	require.Empty(t, graph.TakeAdjacencyChanges(), "rolled back deletions must never be persisted later")
}

func TestAdjacencyDeltaRejectsInvalidLayers(t *testing.T) {
	graph := NewGraph[int]()
	require.Error(t, graph.ApplyAdjacencyChanges([]AdjacencyChange[int]{{Layer: -1, Key: 1}}, nil))
}

func TestFullCollectionSearchIncludesDisconnectedNodes(t *testing.T) {
	graph := NewGraph[int]()
	nodes := make(map[int]hnswspec.LayerNode[int])
	for key, vector := range [][]float32{{1, 0}, {0, 1}, {-1, 0}} {
		nodes[key] = hnswspec.NewStandardLayerNode(key, func() []float32 { return vector })
	}
	nodes[0].AddSingleNeighbor(nodes[1])
	graph.Layers = []*Layer[int]{{Nodes: nodes}}
	results := graph.SearchWithDistance([]float32{1, 0}, 1000)
	require.Len(t, results, 3)
	for key, result := range results {
		require.Equal(t, key, result.Key)
		require.InDelta(t, float64(key), result.Distance, 1e-9)
	}
	filtered := graph.SearchWithDistanceAndFilter([]float32{1, 0}, 3, func(key int, _ Vector) bool { return key == 2 })
	require.Len(t, filtered, 1)
	require.Equal(t, 2, filtered[0].Key)
	require.Empty(t, graph.SearchWithDistance([]float32{1, 0}, 0))
}

func TestAdjacencyDeltaReplayMatchesBuiltGraph(t *testing.T) {
	const count = 40
	source := benchmarkVectors(count, 16, 3)
	vectors := make(map[int][]float32, count)
	original := NewGraph[int](WithDeterministicRng[int](1), WithM[int](8), WithEfConstruction[int](16))
	original.EnableAdjacencyTracking()
	var changes []AdjacencyChange[int]
	for _, node := range source {
		vectors[node.Key] = node.Value
		original.Add(node)
		changes = append(changes, original.TakeAdjacencyChanges()...)
	}
	require.NotEmpty(t, changes)

	replay := NewGraph[int](WithDeterministicRng[int](1), WithM[int](8), WithEfConstruction[int](16))
	create := func(key int) (hnswspec.LayerNode[int], error) {
		vector := vectors[key]
		return hnswspec.NewStandardLayerNode(key, func() []float32 { return vector }), nil
	}
	require.NoError(t, replay.ApplyAdjacencyChanges(changes, create))
	require.Equal(t, original.Len(), replay.Len())
	assertSameNeighbors(t, original, replay)

	require.NoError(t, replay.ApplyAdjacencyChanges(changes, create))
	assertSameNeighbors(t, original, replay)

	query := source[7].Value
	originalHits := original.SearchWithDistance(query, 5)
	replayHits := replay.SearchWithDistance(query, 5)
	require.Equal(t, len(originalHits), len(replayHits))
	for i := range originalHits {
		require.Equal(t, originalHits[i].Key, replayHits[i].Key)
	}

	original.Delete(source[4].Key)
	deleted := original.TakeAdjacencyChanges()
	require.NotEmpty(t, deleted)
	require.NoError(t, replay.ApplyAdjacencyChanges(deleted, create))
	require.False(t, replay.Has(source[4].Key))
	require.Equal(t, original.Len(), replay.Len())
	assertSameNeighbors(t, original, replay)
}

func assertSameNeighbors(t *testing.T, left, right *Graph[int]) {
	t.Helper()
	require.Equal(t, len(left.Layers), len(right.Layers))
	for layer := range left.Layers {
		require.Equal(t, len(left.Layers[layer].Nodes), len(right.Layers[layer].Nodes))
		for key, node := range left.Layers[layer].Nodes {
			other, ok := right.Layers[layer].Nodes[key]
			require.True(t, ok, "layer %d missing %d", layer, key)
			require.Equal(t, neighborKeySet(node), neighborKeySet(other))
		}
	}
}

func neighborKeySet(node hnswspec.LayerNode[int]) map[int]struct{} {
	keys := make(map[int]struct{}, len(node.GetNeighbors()))
	for key := range node.GetNeighbors() {
		keys[key] = struct{}{}
	}
	return keys
}
