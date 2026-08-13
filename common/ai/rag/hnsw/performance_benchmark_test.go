package hnsw

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

var benchmarkSearchSink []SearchResult[int]

func TestDefaultGraphConfigCompatibility(t *testing.T) {
	if encodingVersion != 1 {
		t.Fatalf("persistent encoding version changed: got %d, want 1", encodingVersion)
	}
	config := DefaultGraphConfig[int]()
	if config.M != 16 || config.Ml != 0.25 || config.EfSearch != 20 || config.EfConstruction != 200 {
		t.Fatalf("default HNSW parameters changed: M=%d Ml=%v efSearch=%d efConstruction=%d",
			config.M, config.Ml, config.EfSearch, config.EfConstruction)
	}
	if name, ok := distanceFuncToName(config.Distance); !ok || name != "cosine" {
		t.Fatalf("default distance changed: name=%q registered=%t", name, ok)
	}
	if !config.EnableDistanceCache || config.DistanceCacheSize != 1000 {
		t.Fatalf("default distance cache changed: enabled=%t size=%d",
			config.EnableDistanceCache, config.DistanceCacheSize)
	}
}

func benchmarkVectors(count, dims int, seed int64) []InputNode[int] {
	rng := rand.New(rand.NewSource(seed))
	nodes := make([]InputNode[int], count)
	for i := range nodes {
		vector := make([]float32, dims)
		for j := range vector {
			vector[j] = rng.Float32()*2 - 1
		}
		nodes[i] = MakeInputNode(i, vector)
	}
	return nodes
}

func benchmarkQueries(count, dims int, seed int64) [][]float32 {
	rng := rand.New(rand.NewSource(seed))
	queries := make([][]float32, count)
	for i := range queries {
		queries[i] = make([]float32, dims)
		for j := range queries[i] {
			queries[i][j] = rng.Float32()*2 - 1
		}
	}
	return queries
}

func benchmarkExactTopK(nodes []InputNode[int], query []float32, k int) map[int]struct{} {
	type exactResult struct {
		key      int
		distance float64
	}
	results := make([]exactResult, len(nodes))
	queryVector := func() []float32 { return query }
	for i := range nodes {
		results[i] = exactResult{
			key:      nodes[i].Key,
			distance: CosineDistance(nodes[i].ToVector(), queryVector),
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].distance < results[j].distance })
	top := make(map[int]struct{}, k)
	for i := 0; i < k; i++ {
		top[results[i].key] = struct{}{}
	}
	return top
}

func BenchmarkCosineDistanceDimensions(b *testing.B) {
	for _, dims := range []int{128, 768, 1536} {
		b.Run(fmt.Sprintf("dims=%d", dims), func(b *testing.B) {
			nodes := benchmarkVectors(2, dims, 1)
			a, c := nodes[0].ToVector(), nodes[1].ToVector()
			b.ReportAllocs()
			b.SetBytes(int64(dims * 2 * 4))
			b.ResetTimer()
			var distance float64
			for i := 0; i < b.N; i++ {
				distance = CosineDistance(a, c)
			}
			_ = distance
		})
	}
}

func BenchmarkNodeCosineDistanceDimensions(b *testing.B) {
	for _, dims := range []int{128, 256, 384, 768, 1536} {
		b.Run(fmt.Sprintf("dims=%d", dims), func(b *testing.B) {
			nodes := benchmarkVectors(2, dims, 1)
			a := hnswspec.NewStandardLayerNode(nodes[0].Key, nodes[0].ToVector())
			c := hnswspec.NewStandardLayerNode(nodes[1].Key, nodes[1].ToVector())
			// Warm the cached vector norms before timing the search-path distance.
			_ = hnswspec.CosineDistance[int](a, c)
			b.ReportAllocs()
			b.SetBytes(int64(dims * 2 * 4))
			b.ResetTimer()
			var distance float64
			for i := 0; i < b.N; i++ {
				distance = hnswspec.CosineDistance[int](a, c)
			}
			_ = distance
		})
	}
}

func BenchmarkHNSWSearchScale(b *testing.B) {
	for _, tc := range []struct {
		count int
		dims  int
	}{
		{count: 1_000, dims: 128},
		{count: 1_000, dims: 768},
	} {
		b.Run(fmt.Sprintf("n=%d/dims=%d", tc.count, tc.dims), func(b *testing.B) {
			nodes := benchmarkVectors(tc.count, tc.dims, 42)
			queries := benchmarkQueries(32, tc.dims, 84)
			graph := NewGraph[int](WithDeterministicRng[int](42))
			graph.Add(nodes...)

			const k = 10
			exact := make([]map[int]struct{}, len(queries))
			for i := range queries {
				exact[i] = benchmarkExactTopK(nodes, queries[i], k)
			}
			for _, efSearch := range []int{20, 64, 128} {
				b.Run(fmt.Sprintf("ef=%d", efSearch), func(b *testing.B) {
					graph.EfSearch = efSearch
					matches := 0
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						queryIndex := i % len(queries)
						benchmarkSearchSink = graph.SearchWithDistance(queries[queryIndex], k)
						for _, result := range benchmarkSearchSink {
							if _, ok := exact[queryIndex][result.Key]; ok {
								matches++
							}
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(matches)/float64(b.N*k), "recall@10")
				})
			}
		})
	}
}

func BenchmarkHNSWBuild1K128(b *testing.B) {
	nodes := benchmarkVectors(1_000, 128, 42)
	for _, efConstruction := range []int{32, 200} {
		b.Run(fmt.Sprintf("efConstruction=%d", efConstruction), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				graph := NewGraph[int](
					WithDeterministicRng[int](42),
					WithEfConstruction[int](efConstruction),
				)
				graph.Add(nodes...)
				if graph.Len() != len(nodes) {
					b.Fatalf("built %d nodes, want %d", graph.Len(), len(nodes))
				}
			}
		})
	}
}

func TestHNSWRecallRegression(t *testing.T) {
	const (
		count = 500
		dims  = 64
		k     = 10
	)
	nodes := benchmarkVectors(count, dims, 42)
	queries := benchmarkQueries(16, dims, 84)
	graph := NewGraph[int](
		WithDeterministicRng[int](42),
		WithEfSearch[int](64),
		WithEfConstruction[int](200),
	)
	graph.Add(nodes...)

	matches := 0
	for _, query := range queries {
		exact := benchmarkExactTopK(nodes, query, k)
		for _, result := range graph.SearchWithDistance(query, k) {
			if _, ok := exact[result.Key]; ok {
				matches++
			}
		}
	}
	recall := float64(matches) / float64(len(queries)*k)
	if recall < 0.9 {
		t.Fatalf("recall@10 = %.3f, want at least 0.9", recall)
	}
}

func TestConcurrentSearchWorkspacePool(t *testing.T) {
	nodes := benchmarkVectors(300, 1024, 42)
	queries := benchmarkQueries(16, 1024, 84)
	graph := NewGraph[int](
		WithDeterministicRng[int](42),
		WithEfSearch[int](64),
		WithEfConstruction[int](64),
	)
	graph.Add(nodes...)

	var wait sync.WaitGroup
	errors := make(chan string, 20)
	for worker := 0; worker < 20; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for iteration := 0; iteration < 20; iteration++ {
				results := graph.SearchWithDistance(queries[(worker+iteration)%len(queries)], 10)
				if len(results) != 10 {
					errors <- fmt.Sprintf("worker %d: got %d results", worker, len(results))
					return
				}
				for i := 1; i < len(results); i++ {
					if results[i].Distance < results[i-1].Distance {
						errors <- fmt.Sprintf("worker %d: results are not distance-sorted", worker)
						return
					}
				}
			}
		}(worker)
	}
	wait.Wait()
	close(errors)
	for message := range errors {
		t.Error(message)
	}
}
