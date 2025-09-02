package hnsw

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"

	_ "embed"
)

//go:embed hnswtestdata/bio.json.gzip
var bioGzip []byte

//go:embed hnswtestdata/eco.query.json.gzip
var ecoQueryGzip []byte

//go:embed hnswtestdata/eco.json.gzip
var ecoGzip []byte

type structuredJson struct {
	Index     int64       `json:"index"`
	Embedding [][]float32 `json:"embedding"`
}

func bytes2vecInTest(i []byte) []float32 {
	v1, _ := utils.GzipDeCompress(i)
	var result []*structuredJson
	_ = json.Unmarshal(v1, &result)
	for _, i := range result {
		if len(i.Embedding) > 0 {
			return i.Embedding[0]
		}
	}
	return nil
}

func TestHNSW(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, bytes2vecInTest(bioGzip)),
		MakeNode(2, bytes2vecInTest(ecoGzip)),
	)
	queryVec := bytes2vecInTest(ecoQueryGzip)
	result := g.Search(queryVec, 1)
	resultFlag := result[0].Key == 2
	require.True(t, resultFlag)
}

func TestEmptyGraph(t *testing.T) {
	g := NewGraph[int]()
	queryVec := []float32{1, 2, 3}
	result := g.Search(queryVec, 1)
	require.Empty(t, result)
}

func TestSearchKGreaterThanNodes(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.1, 0.2, 0.7}), // B
		MakeNode(2, []float32{0.9, 0.0, 0.1}), // C
	)
	queryVec := []float32{0.2, 0.2, 0.6} // A
	// Search for 3 nodes when only 2 exist
	result := g.Search(queryVec, 3)
	require.Len(t, result, 2)
	// The first result should be node 1
	require.Equal(t, 1, result[0].Key)
	require.Equal(t, 2, result[1].Key)
}

func TestMoreNodes(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.2, 0.2, 0.6}),   // B - closer to A
		MakeNode(2, []float32{0.25, 0.25, 0.5}), // C - closest to A
		MakeNode(3, []float32{0.9, 0.0, 0.1}),   // D
		MakeNode(4, []float32{0.8, 0.1, 0.1}),   // E
	)
	queryVec := []float32{0.3, 0.3, 0.4} // A
	result := g.Search(queryVec, 2)
	require.Len(t, result, 2)
	require.Equal(t, 2, result[0].Key)
	require.Equal(t, 1, result[1].Key)
}

func BenchmarkAdd(b *testing.B) {
	g := NewGraph[int]()
	nodes := []Node[int]{
		MakeNode(1, bytes2vecInTest(bioGzip)),
		MakeNode(2, bytes2vecInTest(ecoGzip)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Add(nodes...)
	}
}

func TestSearchWithFilter(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.2, 0.2, 0.6}),   // B
		MakeNode(2, []float32{0.25, 0.25, 0.5}), // C - closest to A but will be filtered out
		MakeNode(3, []float32{0.9, 0.0, 0.1}),   // D
		MakeNode(4, []float32{0.8, 0.1, 0.1}),   // E
	)
	queryVec := []float32{0.3, 0.3, 0.4} // A

	// Filter out node 2 (even numbers)
	filter := func(key int, vector Vector) bool {
		return key%2 == 1 // Only odd keys
	}

	result := g.SearchWithFilter(queryVec, 2, filter)
	require.Len(t, result, 2)
	// Should return nodes 1 and 3 (or 1 and 4), but not 2
	for _, node := range result {
		require.True(t, node.Key%2 == 1, "Expected only odd-numbered nodes, got %d", node.Key)
	}
	// The closest odd node should be 1
	require.Equal(t, 1, result[0].Key)
}

func TestSearchWithDistanceAndFilter(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.1, 0.2, 0.7}),   // B
		MakeNode(2, []float32{0.9, 0.0, 0.1}),   // C - will be filtered out
		MakeNode(3, []float32{0.15, 0.25, 0.6}), // D - closest after filter
	)
	queryVec := []float32{0.2, 0.2, 0.6} // A

	// Filter out node 2
	filter := func(key int, vector Vector) bool {
		return key != 2
	}

	result := g.SearchWithDistanceAndFilter(queryVec, 2, filter)
	require.Len(t, result, 2)
	require.NotEqual(t, 2, result[0].Key)
	require.NotEqual(t, 2, result[1].Key)

	// Check that distances are returned
	for _, res := range result {
		require.Greater(t, res.Distance, float64(0))
	}
}

func TestSearchWithNilFilter(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.2, 0.2, 0.6}),
		MakeNode(2, []float32{0.25, 0.25, 0.5}),
	)
	queryVec := []float32{0.3, 0.3, 0.4}

	// Nil filter should behave like normal search
	resultWithNilFilter := g.SearchWithFilter(queryVec, 2, nil)
	resultNormal := g.Search(queryVec, 2)

	require.Len(t, resultWithNilFilter, len(resultNormal))
	for i := range resultWithNilFilter {
		require.Equal(t, resultNormal[i].Key, resultWithNilFilter[i].Key)
	}
}

func TestSearchWithFilterNoMatches(t *testing.T) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, []float32{0.2, 0.2, 0.6}),
		MakeNode(2, []float32{0.25, 0.25, 0.5}),
	)
	queryVec := []float32{0.3, 0.3, 0.4}

	// Filter that excludes all nodes
	filter := func(key int, vector Vector) bool {
		return false
	}

	result := g.SearchWithFilter(queryVec, 2, filter)
	require.Empty(t, result)
}

func BenchmarkSearch(b *testing.B) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, bytes2vecInTest(bioGzip)),
		MakeNode(2, bytes2vecInTest(ecoGzip)),
	)
	queryVec := bytes2vecInTest(ecoQueryGzip)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Search(queryVec, 1)
	}
}

func BenchmarkSearchWithFilter(b *testing.B) {
	g := NewGraph[int]()
	g.Add(
		MakeNode(1, bytes2vecInTest(bioGzip)),
		MakeNode(2, bytes2vecInTest(ecoGzip)),
	)
	queryVec := bytes2vecInTest(ecoQueryGzip)
	filter := func(key int, vector Vector) bool {
		return key > 0 // Simple filter that allows all
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.SearchWithFilter(queryVec, 1, filter)
	}
}

func TestDeleteNodeNeighborCleanup(t *testing.T) {
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(1))
	// Add multiple nodes to create a connected graph with varied distances
	g.Add(
		MakeNode(1, []float32{0.1, 0.8, 0.3}), // Node 1 - diverse vector
		MakeNode(2, []float32{0.5, 0.2, 0.7}), // Node 2 - will be deleted, positioned strategically
		MakeNode(3, []float32{0.9, 0.1, 0.2}), // Node 3 - different quadrant
		MakeNode(4, []float32{0.3, 0.6, 0.8}), // Node 4 - close to node 2
		MakeNode(5, []float32{0.7, 0.4, 0.1}), // Node 5 - another diverse position
		MakeNode(6, []float32{0.2, 0.9, 0.5}), // Node 6 - fixed duplicate ID issue
		// MakeNode(4, []float32{0.3, 0.6, 0.8}), // Node 7 - duplicate ID issue
		// MakeNode(4, []float32{0.3, 0.6, 0.9}), // Node 8 - update vector
	)

	// Verify that the graph is properly connected initially
	require.Greater(t, len(g.Layers), 0, "Graph should have at least one layer")

	// Count total connections before deletion
	var totalConnectionsBefore int
	var connectionsToNode2Before int
	for _, layer := range g.Layers {
		for nodeKey, node := range layer.Nodes {
			totalConnectionsBefore += len(node.Neighbors)
			if nodeKey != 2 {
				// Check if this node has node 2 as a neighbor
				if _, hasNode2AsNeighbor := node.Neighbors[2]; hasNode2AsNeighbor {
					connectionsToNode2Before++
				}
			}
		}
	}

	// Verify that node 2 exists before deletion
	var node2ExistsBefore bool
	for _, layer := range g.Layers {
		if _, exists := layer.Nodes[2]; exists {
			node2ExistsBefore = true
			break
		}
	}
	require.True(t, node2ExistsBefore, "Node 2 should exist before deletion")

	// Delete node 2
	deleted := g.Delete(2)
	require.True(t, deleted, "Delete should return true when node exists")

	// Verify that node 2 no longer exists in any layer
	for _, layer := range g.Layers {
		_, exists := layer.Nodes[2]
		require.False(t, exists, "Node 2 should not exist in any layer after deletion")
	}

	// Critical test: Verify that no remaining node has the deleted node (2) as a neighbor
	for _, layer := range g.Layers {
		for nodeKey, node := range layer.Nodes {
			_, hasDeletedNeighbor := node.Neighbors[2]
			require.False(t, hasDeletedNeighbor,
				"Node %d in layer should not have deleted node 2 as neighbor", nodeKey)
		}
	}

	// Verify that the graph is still functional after deletion
	queryVec := []float32{0.4, 0.3, 0.6} // Close to where node 2 was
	result := g.Search(queryVec, 3)
	require.NotEmpty(t, result, "Search should still return results after deletion")

	// Ensure no result contains the deleted node
	for _, res := range result {
		require.NotEqual(t, 2, res.Key, "Search results should not contain deleted node")
	}

	// Verify the remaining nodes are still properly connected
	// (optional check to ensure graph connectivity is maintained)
	remainingNodeCount := 0
	for _, layer := range g.Layers {
		remainingNodeCount += len(layer.Nodes)
	}
	require.Greater(t, remainingNodeCount, 0, "Graph should still have nodes after deletion")
}

func TestAddNodeWithDuplicateID(t *testing.T) {
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(1))
	g.Add(
		MakeNode(1, []float32{0.1, 0.8, 0.3}),
		MakeNode(1, []float32{0.5, 0.2, 0.7}),
	)

	require.Greater(t, len(g.Layers), 0, "Graph should have at least one layer")
	require.Equal(t, 1, len(g.Layers[0].Nodes))
	require.Equal(t, 1, g.Layers[0].Nodes[1].Key)
	require.Equal(t, float32(0.5), g.Layers[0].Nodes[1].Value()[0])
}
func TestDeleteNodeNeighborCleanupWithDuplicateID(t *testing.T) {
	g := NewGraph[int]()
	g.Rng = rand.New(rand.NewSource(1))
	// Add multiple nodes to create a connected graph with varied distances
	g.Add(
		MakeNode(1, []float32{0.1, 0.8, 0.3}), // Node 1 - diverse vector
		MakeNode(2, []float32{0.5, 0.2, 0.7}), // Node 2 - will be deleted, positioned strategically
		MakeNode(3, []float32{0.9, 0.1, 0.2}), // Node 3 - different quadrant
		MakeNode(4, []float32{0.3, 0.6, 0.8}), // Node 4 - close to node 2
		MakeNode(5, []float32{0.7, 0.4, 0.1}), // Node 5 - another diverse position
		MakeNode(6, []float32{0.2, 0.9, 0.5}), // Node 6 - fixed duplicate ID issue
		MakeNode(4, []float32{0.3, 0.6, 0.8}), // Node 7 - duplicate ID issue
		// MakeNode(4, []float32{0.3, 0.6, 0.9}), // Node 8 - update vector
	)

	// Verify that the graph is properly connected initially
	require.Greater(t, len(g.Layers), 0, "Graph should have at least one layer")

	// Count total connections before deletion
	var totalConnectionsBefore int
	var connectionsToNode2Before int
	for _, layer := range g.Layers {
		for nodeKey, node := range layer.Nodes {
			totalConnectionsBefore += len(node.Neighbors)
			if nodeKey != 2 {
				// Check if this node has node 2 as a neighbor
				if _, hasNode2AsNeighbor := node.Neighbors[2]; hasNode2AsNeighbor {
					connectionsToNode2Before++
				}
			}
		}
	}

	// Verify that node 2 exists before deletion
	var node2ExistsBefore bool
	for _, layer := range g.Layers {
		if _, exists := layer.Nodes[2]; exists {
			node2ExistsBefore = true
			break
		}
	}
	require.True(t, node2ExistsBefore, "Node 2 should exist before deletion")

	// Delete node 2
	deleted := g.Delete(2)
	require.True(t, deleted, "Delete should return true when node exists")

	// Verify that node 2 no longer exists in any layer
	for _, layer := range g.Layers {
		_, exists := layer.Nodes[2]
		require.False(t, exists, "Node 2 should not exist in any layer after deletion")
	}

	// Critical test: Verify that no remaining node has the deleted node (2) as a neighbor
	for _, layer := range g.Layers {
		for nodeKey, node := range layer.Nodes {
			_, hasDeletedNeighbor := node.Neighbors[2]
			require.False(t, hasDeletedNeighbor,
				"Node %d in layer should not have deleted node 2 as neighbor", nodeKey)
		}
	}

	// Verify that the graph is still functional after deletion
	queryVec := []float32{0.4, 0.3, 0.6} // Close to where node 2 was
	result := g.Search(queryVec, 3)
	require.NotEmpty(t, result, "Search should still return results after deletion")

	// Ensure no result contains the deleted node
	for _, res := range result {
		require.NotEqual(t, 2, res.Key, "Search results should not contain deleted node")
	}

	// Verify the remaining nodes are still properly connected
	// (optional check to ensure graph connectivity is maintained)
	remainingNodeCount := 0
	for _, layer := range g.Layers {
		remainingNodeCount += len(layer.Nodes)
	}
	require.Greater(t, remainingNodeCount, 0, "Graph should still have nodes after deletion")
}
