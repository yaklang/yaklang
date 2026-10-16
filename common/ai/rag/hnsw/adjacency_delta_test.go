package hnsw

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

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
