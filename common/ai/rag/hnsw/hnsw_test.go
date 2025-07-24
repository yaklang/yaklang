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
