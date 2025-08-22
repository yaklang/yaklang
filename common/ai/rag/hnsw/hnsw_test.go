package hnsw

import (
	"encoding/json"
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
		require.Greater(t, res.Distance, float32(0))
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
