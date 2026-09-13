package vectorstore

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

func BenchmarkGraphWrapperDeleteBatch(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("nodes=%d/delete=100", size), func(b *testing.B) {
			b.ReportAllocs()
			var callbacks int
			for iter := 0; iter < b.N; iter++ {
				b.StopTimer()
				graph := hnsw.NewGraph[string]()
				layer := &hnsw.Layer[string]{Nodes: make(map[string]hnswspec.LayerNode[string], size)}
				nodes := make([]hnswspec.LayerNode[string], size)
				ids := make([]string, 100)
				for i := range nodes {
					key := fmt.Sprint(i)
					vector := []float32{1, float32(i % 101), float32(i % 37), float32(i % 17), 0, 1, 0}
					nodes[i] = hnswspec.NewStandardLayerNode(key, func() []float32 { return vector })
					layer.Nodes[key] = nodes[i]
					if i < len(ids) {
						ids[i] = key
					}
				}
				for i, node := range nodes {
					for offset := 1; offset <= 16; offset++ {
						node.AddSingleNeighbor(nodes[(i+offset)%size])
					}
				}
				graph.Layers = []*hnsw.Layer[string]{layer}
				graph.OnLayersChange = func([]*hnsw.Layer[string]) { callbacks++ }
				wrapper := NewGraphWrapper(graph, "benchmark", "benchmark")
				b.StartTimer()
				if err := wrapper.DeleteWithError(ids...); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				if wrapper.GetSize() != size-100 {
					b.Fatal("wrong survivor count")
				}
				wrapper.Close()
			}
			b.ReportMetric(float64(callbacks)/float64(b.N), "saves/op")
		})
	}
}
