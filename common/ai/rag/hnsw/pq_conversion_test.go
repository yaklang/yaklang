package hnsw

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

func TestPQConversionPreservesLayerTopology(t *testing.T) {
	graph := NewGraph[int](WithDeterministicRng[int](2), WithM[int](4), WithMl[int](0.5))
	graph.Add(benchmarkVectors(50, 8, 3)...)
	require.Greater(t, len(graph.Layers), 1)
	before := make([]map[int]map[int]struct{}, len(graph.Layers))
	for i, layer := range graph.Layers {
		before[i] = make(map[int]map[int]struct{})
		for key, node := range layer.Nodes {
			before[i][key] = neighborKeySet(node)
		}
	}
	_, err := graph.TrainPQCodebookFromData(2, 4)
	require.NoError(t, err)
	for i, layer := range graph.Layers {
		for key, node := range layer.Nodes {
			require.Equal(t, before[i][key], neighborKeySet(node), "layer %d key %d", i, key)
			for neighborKey, neighbor := range node.GetNeighbors() {
				require.Same(t, layer.Nodes[neighborKey], neighbor)
			}
		}
	}
}

func TestPQConversionCallbackFailureLeavesGraphUnchanged(t *testing.T) {
	graph := NewGraph[int](WithDeterministicRng[int](2))
	graph.Add(benchmarkVectors(20, 8, 3)...)
	before := graph.Layers
	_, err := graph.TrainPQCodebookFromDataWithCallback(2, 4, func(int, []byte, []float64) (hnswspec.LayerNode[int], error) {
		return nil, fmt.Errorf("injected conversion failure")
	})
	require.ErrorContains(t, err, "injected conversion failure")
	require.False(t, graph.IsPQEnabled())
	require.Equal(t, before, graph.Layers)
}

func TestPQBinaryRoundTripPreservesCodesAndDistances(t *testing.T) {
	for _, distance := range []DistanceFunc{CosineDistance, EuclideanDistance} {
		graph := NewGraph[int](WithDeterministicRng[int](2), WithDistance[int](distance))
		vectors := benchmarkVectors(20, 8, 3)
		graph.Add(vectors...)
		_, err := graph.TrainPQCodebookFromData(2, 4)
		require.NoError(t, err)
		persistent, err := ExportHNSWGraph(graph)
		require.NoError(t, err)
		binary, err := persistent.ToBinary(context.Background())
		require.NoError(t, err)
		loaded, err := LoadBinary[int](binary)
		require.NoError(t, err)
		restored, err := loaded.BuildLazyGraph(nil, WithDistance[int](distance))
		require.NoError(t, err)
		for i, layer := range graph.Layers {
			for key, node := range layer.Nodes {
				want, _ := node.GetPQCodes()
				got, ok := restored.Layers[i].Nodes[key].GetPQCodes()
				require.True(t, ok)
				require.Equal(t, want, got)
			}
		}
		left, right := graph.SearchWithDistance(vectors[0].Value, 5), restored.SearchWithDistance(vectors[0].Value, 5)
		require.Len(t, right, len(left))
		for i := range left {
			// Quantized vectors can tie; their relative key order is unspecified.
			require.InDelta(t, left[i].Distance, right[i].Distance, 1e-9)
		}
	}
}
