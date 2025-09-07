package hnsw

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadBinaryRoundTrip(t *testing.T) {
	// 创建一个简单的 Persistent 对象
	p := &Persistent[string]{
		Total:    3,
		Dims:     4,
		M:        16,
		Ml:       0.5,
		EfSearch: 200,
		PQMode:   false,
		Layers: []*PersistentLayer{
			{HNSWLevel: 0, Nodes: []uint32{1, 2, 3}},
			{HNSWLevel: 1, Nodes: []uint32{1, 2}},
		},
		OffsetToKey: []*PersistentNode[string]{
			{Key: "", Code: []float64{0.0, 0.0, 0.0, 0.0}}, // 0 offset - reserve node
			{Key: "a", Code: []float64{1.0, 2.0, 3.0, 4.0}},
			{Key: "b", Code: []float64{5.0, 6.0, 7.0, 8.0}},
			{Key: "c", Code: []float64{9.0, 10.0, 11.0, 12.0}},
		},
		Neighbors: map[uint32][]uint32{
			1: {2, 3},
			2: {1, 3},
			3: {1, 2},
		},
	}

	// 导出到二进制
	ctx := context.Background()
	reader, err := p.ToBinary(ctx)
	require.NoError(t, err)

	// 从二进制加载
	loaded, err := LoadBinary[string](reader)
	require.NoError(t, err)

	// 验证字段
	require.Equal(t, p.Total, loaded.Total)
	require.Equal(t, p.Dims, loaded.Dims)
	require.Equal(t, p.M, loaded.M)
	require.Equal(t, p.Ml, loaded.Ml)
	require.Equal(t, p.EfSearch, loaded.EfSearch)
	require.Equal(t, p.PQMode, loaded.PQMode)

	// 验证层
	require.Len(t, loaded.Layers, len(p.Layers))
	for i := range p.Layers {
		require.Equal(t, p.Layers[i].HNSWLevel, loaded.Layers[i].HNSWLevel)
		require.Equal(t, p.Layers[i].Nodes, loaded.Layers[i].Nodes)
	}

	// 验证节点
	require.Len(t, loaded.OffsetToKey, len(p.OffsetToKey))
	for i := range p.OffsetToKey {
		require.Equal(t, p.OffsetToKey[i].Code, loaded.OffsetToKey[i].Code)
	}

	// 验证邻居
	require.Len(t, loaded.Neighbors, len(p.Neighbors))
	for k, v := range p.Neighbors {
		require.Equal(t, v, loaded.Neighbors[k])
	}
}

func TestLoadBinaryRoundTripPQ(t *testing.T) {
	// 创建一个带 PQ 的 Persistent 对象
	p := &Persistent[string]{
		Total:    2,
		Dims:     4,
		M:        16,
		Ml:       0.5,
		EfSearch: 200,
		PQMode:   true,
		PQCodebook: &PersistentPQCodebook{
			M:              2,
			K:              256,
			SubVectorDim:   2,
			PQCodeByteSize: 2,
			Centroids:      [][][]float64{{{1.0, 2.0}, {3.0, 4.0}}, {{5.0, 6.0}, {7.0, 8.0}}},
		},
		Layers: []*PersistentLayer{
			{HNSWLevel: 0, Nodes: []uint32{1, 2}},
		},
		OffsetToKey: []*PersistentNode[string]{
			{Key: "", Code: []byte{0, 0}}, // 0 offset - reserve node
			{Key: "a", Code: []byte{1, 2}},
			{Key: "b", Code: []byte{3, 4}},
		},
		Neighbors: map[uint32][]uint32{
			1: {2},
			2: {1},
		},
	}

	// 导出到二进制
	ctx := context.Background()
	reader, err := p.ToBinary(ctx)
	require.NoError(t, err)

	// 从二进制加载
	loaded, err := LoadBinary[string](reader)
	require.NoError(t, err)

	// 验证字段
	require.Equal(t, p.Total, loaded.Total)
	require.Equal(t, p.Dims, loaded.Dims)
	require.Equal(t, p.M, loaded.M)
	require.Equal(t, p.Ml, loaded.Ml)
	require.Equal(t, p.EfSearch, loaded.EfSearch)
	require.Equal(t, p.PQMode, loaded.PQMode)

	// 验证 PQ 码本
	require.NotNil(t, loaded.PQCodebook)
	require.Equal(t, p.PQCodebook.M, loaded.PQCodebook.M)
	require.Equal(t, p.PQCodebook.K, loaded.PQCodebook.K)
	require.Equal(t, p.PQCodebook.SubVectorDim, loaded.PQCodebook.SubVectorDim)
	require.Equal(t, p.PQCodebook.PQCodeByteSize, loaded.PQCodebook.PQCodeByteSize)
	require.Equal(t, p.PQCodebook.Centroids, loaded.PQCodebook.Centroids)

	// 验证节点
	require.Len(t, loaded.OffsetToKey, len(p.OffsetToKey))
	for i := range p.OffsetToKey {
		require.Equal(t, p.OffsetToKey[i].Code, loaded.OffsetToKey[i].Code)
	}
}

func TestLoadBinaryEmpty(t *testing.T) {
	// 测试空数据
	empty := bytes.NewReader([]byte(""))
	_, err := LoadBinary[string](empty)
	require.Error(t, err)

	// 测试无效魔数
	invalid := bytes.NewReader([]byte("INVALID"))
	_, err = LoadBinary[string](invalid)
	require.Error(t, err)
}

func TestExportGraphToBinaryStandard(t *testing.T) {
	// 创建标准模式的图
	graph := NewGraph[string](
		WithM[string](16),
		WithMl[string](0.5),
		WithEfSearch[string](200),
		WithCosineDistance[string](),
	)

	// 添加测试节点
	nodes := []InputNode[string]{
		{Key: "node1", Value: []float32{1.0, 2.0, 3.0, 4.0}},
		{Key: "node2", Value: []float32{5.0, 6.0, 7.0, 8.0}},
		{Key: "node3", Value: []float32{9.0, 10.0, 11.0, 12.0}},
	}
	graph.Add(nodes...)

	// 验证图结构
	require.True(t, len(graph.Layers) > 0)
	require.True(t, len(graph.Layers[0].Nodes) > 0)

	// 导出到 Persistent
	pers, err := ExportHNSWGraph(graph)
	require.NoError(t, err)
	require.NotNil(t, pers)
	require.True(t, pers.Total >= uint32(3)) // 可能包含额外的节点
	require.Equal(t, uint32(4), pers.Dims)
	require.False(t, pers.PQMode)

	// 导出到二进制
	ctx := context.Background()
	reader, err := pers.ToBinary(ctx)
	require.NoError(t, err)

	// 从二进制加载回来
	loaded, err := LoadBinary[string](reader)
	require.NoError(t, err)

	// 验证加载的结果
	require.Equal(t, pers.Total, loaded.Total)
	require.Equal(t, pers.Dims, loaded.Dims)
	require.Equal(t, pers.M, loaded.M)
	require.Equal(t, pers.Ml, loaded.Ml)
	require.Equal(t, pers.EfSearch, loaded.EfSearch)
	require.Equal(t, pers.PQMode, loaded.PQMode)
}

func TestExportGraphToBinaryPQ(t *testing.T) {
	// 创建标准模式的图
	graph := NewGraph[string](
		WithM[string](16),
		WithMl[string](0.5),
		WithEfSearch[string](200),
		WithCosineDistance[string](),
	)

	// 添加测试节点
	nodes := []InputNode[string]{
		{Key: "node1", Value: []float32{1.0, 2.0, 3.0, 4.0}},
		{Key: "node2", Value: []float32{5.0, 6.0, 7.0, 8.0}},
		{Key: "node3", Value: []float32{9.0, 10.0, 11.0, 12.0}},
		{Key: "node4", Value: []float32{2.0, 3.0, 4.0, 5.0}},
		{Key: "node5", Value: []float32{6.0, 7.0, 8.0, 9.0}},
		{Key: "node6", Value: []float32{10.0, 11.0, 12.0, 13.0}},
		{Key: "node7", Value: []float32{3.0, 4.0, 5.0, 6.0}},
		{Key: "node8", Value: []float32{7.0, 8.0, 9.0, 10.0}},
	}
	graph.Add(nodes...)

	// 验证图初始状态
	require.False(t, graph.IsPQEnabled())
	require.True(t, len(graph.Layers) > 0)
	require.True(t, len(graph.Layers[0].Nodes) >= 8)

	// 从现有数据训练PQ码表
	cookbook, err := graph.TrainPQCodebookFromData(2, 4)
	require.NoError(t, err)
	_ = cookbook

	// 验证PQ训练成功
	require.True(t, graph.IsPQEnabled())
	require.NotNil(t, graph.pqCodebook)
	require.NotNil(t, graph.pqQuantizer)

	// 验证所有节点都转换为PQ节点
	convertedCount := 0
	for _, layer := range graph.Layers {
		for _, node := range layer.Nodes {
			if node.IsPQEnabled() {
				convertedCount++
				// 确保PQ codes存在且长度正确
				codes, ok := node.GetPQCodes()
				require.True(t, ok, "PQ node should have codes")
				require.Equal(t, 2, len(codes), "PQ codes should be length 2")
			}
		}
	}
	require.True(t, convertedCount > 0)

	// 导出到 Persistent
	pers, err := ExportHNSWGraph(graph)
	require.NoError(t, err)
	require.NotNil(t, pers)
	require.True(t, pers.PQMode)
	require.NotNil(t, pers.PQCodebook)
	require.Equal(t, uint32(2), pers.PQCodebook.M)
	require.Equal(t, uint32(4), pers.PQCodebook.K)

	// 导出到二进制
	ctx := context.Background()
	reader, err := pers.ToBinary(ctx)
	require.NoError(t, err)

	// 从二进制加载回来
	loaded, err := LoadBinary[string](reader)
	require.NoError(t, err)

	// 验证加载的结果
	require.Equal(t, pers.Total, loaded.Total)
	require.Equal(t, pers.Dims, loaded.Dims)
	require.True(t, loaded.PQMode)
	require.NotNil(t, loaded.PQCodebook)
	require.Equal(t, pers.PQCodebook.M, loaded.PQCodebook.M)
	require.Equal(t, pers.PQCodebook.K, loaded.PQCodebook.K)

	// 验证PQ码表数据完整性
	require.Equal(t, len(pers.PQCodebook.Centroids), len(loaded.PQCodebook.Centroids))
	for i := range pers.PQCodebook.Centroids {
		require.Equal(t, pers.PQCodebook.Centroids[i], loaded.PQCodebook.Centroids[i])
	}
}

func TestExportEmptyGraphToBinary(t *testing.T) {
	// 创建空图
	graph := NewGraph[string](
		WithM[string](16),
		WithMl[string](0.5),
		WithEfSearch[string](200),
		WithCosineDistance[string](),
	)

	// 验证图为空
	require.True(t, len(graph.Layers) == 0 || len(graph.Layers[0].Nodes) == 0)

	// 尝试导出空图应该失败
	_, err := ExportHNSWGraph(graph)
	require.Error(t, err)
	require.Contains(t, err.Error(), "graph is nil")
}
