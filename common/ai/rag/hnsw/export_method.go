package hnsw

import (
	"cmp"
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
)

func ExportGraphToBinary[K cmp.Ordered](i *Graph[K]) (io.Reader, error) {
	pers, err := ExportHNSWGraph(i)
	if err != nil {
		return nil, err
	}
	return pers.ToBinary(context.Background())
}

func LoadGraphFromBinary[K cmp.Ordered](reader io.Reader, dataLoader func(key hnswspec.LazyNodeID) (hnswspec.LayerNode[K], error)) (*Graph[K], error) {
	pers, err := LoadBinary[K](reader)
	if err != nil {
		return nil, err
	}
	return pers.BuildLazyGraph(dataLoader)
}
