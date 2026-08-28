package vectorstore

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
)

var graphWrapperBenchmarkSink []hnsw.SearchResult[int]

func BenchmarkGraphWrapperSearchOverhead(b *testing.B) {
	vector := make([]float32, 1024)
	for i := range vector {
		vector[i] = float32(i%31) / 31
	}
	graph := hnsw.NewGraph[int](hnsw.WithDeterministicRng[int](42))
	graph.Add(hnsw.MakeInputNode(1, vector))
	wrapper := NewGraphWrapper(graph, "benchmark", "benchmark")

	b.Run("direct", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			graphWrapperBenchmarkSink = graph.SearchWithDistance(vector, 1)
		}
	})
	b.Run("wrapper", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			graphWrapperBenchmarkSink = wrapper.SearchWithDistanceAndFilter(vector, 1, nil)
		}
	})
	b.Run("wrapper_parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = wrapper.SearchWithDistanceAndFilter(vector, 1, nil)
			}
		})
	})
}
