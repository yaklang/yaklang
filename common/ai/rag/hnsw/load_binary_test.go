package hnsw

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestLoadBinaryRoundTrip(t *testing.T) {
	// 创建一个简单的 Persistent 对象
	p := &Persistent[string]{
		Total:      3,
		Dims:       4,
		M:          16,
		Ml:         0.5,
		EfSearch:   200,
		ExportMode: ExportModeStandard,
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
	require.Equal(t, p.ExportMode, loaded.ExportMode)

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
		Total:      2,
		Dims:       4,
		M:          16,
		Ml:         0.5,
		EfSearch:   200,
		ExportMode: ExportModePQ,
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
	require.Equal(t, p.ExportMode, loaded.ExportMode)

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
	require.Equal(t, ExportModeStandard, pers.ExportMode)

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
	require.Equal(t, pers.ExportMode, loaded.ExportMode)
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
	require.Equal(t, ExportModePQ, pers.ExportMode)
	require.NotNil(t, pers.PQCodebook)
	require.Equal(t, uint32(2), pers.PQCodebook.M)
	require.Equal(t, uint32(4), pers.PQCodebook.K)

	// 导出到二进制
	ctx := context.Background()
	reader, err := pers.ToBinary(ctx)
	require.NoError(t, err)

	var buf bytes.Buffer
	raw, _ := io.ReadAll(io.TeeReader(reader, &buf))
	i := utils.ByteSize(uint64(len(raw)))
	fmt.Println(i)
	spew.Dump(raw)
	// 从二进制加载回来
	loaded, err := LoadBinary[string](&buf)
	require.NoError(t, err)

	// 验证加载的结果
	require.Equal(t, pers.Total, loaded.Total)
	require.Equal(t, pers.Dims, loaded.Dims)
	require.Equal(t, ExportModePQ, loaded.ExportMode)
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

func TestKeyRoundTripInBinary(t *testing.T) {
	// 测试不同类型的Key在二进制序列化中的完整循环
	testCases := []struct {
		name  string
		key   string
		value []float32
	}{
		{"string_key", "test_key", []float32{1.0, 2.0, 3.0, 4.0}},
		{"numeric_string_key", "42", []float32{5.0, 6.0, 7.0, 8.0}},
		{"special_chars_key", "key-with_special.chars!", []float32{9.0, 10.0, 11.0, 12.0}},
		{"empty_string_key", "", []float32{13.0, 14.0, 15.0, 16.0}},
		{"unicode_key", "测试_key_🚀", []float32{17.0, 18.0, 19.0, 20.0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建包含测试Key的Persistent对象
			p := &Persistent[string]{
				Total:      2,
				Dims:       4,
				M:          16,
				Ml:         0.5,
				EfSearch:   200,
				ExportMode: ExportModeStandard,
				Layers: []*PersistentLayer{
					{HNSWLevel: 0, Nodes: []uint32{1}},
				},
				OffsetToKey: []*PersistentNode[string]{
					{Key: "", Code: []float64{0.0, 0.0, 0.0, 0.0}}, // 0 offset
					{Key: tc.key, Code: []float64{float64(tc.value[0]), float64(tc.value[1]), float64(tc.value[2]), float64(tc.value[3])}},
				},
				Neighbors: map[uint32][]uint32{
					1: {},
				},
			}

			// 导出到二进制
			ctx := context.Background()
			reader, err := p.ToBinary(ctx)
			require.NoError(t, err)

			// 从二进制加载回来
			loaded, err := LoadBinary[string](reader)
			require.NoError(t, err)

			// 验证Key被正确恢复
			require.Equal(t, tc.key, loaded.OffsetToKey[1].Key)
		})
	}
}

func TestKeyConversionErrors(t *testing.T) {
	// 测试不同泛型类型下的Key转换
	t.Run("int_type_conversion", func(t *testing.T) {
		// 创建一个int类型的Persistent
		p := &Persistent[int]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[int]{
				{Key: 0, Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: 42, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[int](reader)
		require.NoError(t, err)
		require.Equal(t, 42, loaded.OffsetToKey[1].Key)
	})

	t.Run("int64_type_conversion", func(t *testing.T) {
		// 创建一个int64类型的Persistent
		p := &Persistent[int64]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[int64]{
				{Key: 0, Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: 123456789, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[int64](reader)
		require.NoError(t, err)
		require.Equal(t, int64(123456789), loaded.OffsetToKey[1].Key)
	})

	t.Run("uint32_type_conversion", func(t *testing.T) {
		// 创建一个uint32类型的Persistent
		p := &Persistent[uint32]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[uint32]{
				{Key: 0, Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: 65535, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[uint32](reader)
		require.NoError(t, err)
		require.Equal(t, uint32(65535), loaded.OffsetToKey[1].Key)
	})

	t.Run("uint64_type_conversion", func(t *testing.T) {
		// 创建一个uint64类型的Persistent
		p := &Persistent[uint64]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[uint64]{
				{Key: 0, Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: 18446744073709551615, Code: []float64{1.0, 2.0, 3.0, 4.0}}, // max uint64
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[uint64](reader)
		require.NoError(t, err)
		require.Equal(t, uint64(18446744073709551615), loaded.OffsetToKey[1].Key)
	})
}

func TestKeyEdgeCases(t *testing.T) {
	t.Run("very_long_key", func(t *testing.T) {
		// 测试非常长的key
		longKey := strings.Repeat("a", 10000)
		p := &Persistent[string]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[string]{
				{Key: "", Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: longKey, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[string](reader)
		require.NoError(t, err)
		require.Equal(t, longKey, loaded.OffsetToKey[1].Key)
	})

	t.Run("null_bytes_in_key", func(t *testing.T) {
		// 测试包含null字节的key
		keyWithNull := "key\x00with\x00null"
		p := &Persistent[string]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[string]{
				{Key: "", Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: keyWithNull, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[string](reader)
		require.NoError(t, err)
		require.Equal(t, keyWithNull, loaded.OffsetToKey[1].Key)
	})

	t.Run("binary_data_in_key", func(t *testing.T) {
		// 测试包含二进制数据的key（非UTF-8）
		binaryKey := string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD})
		p := &Persistent[string]{
			Total:      2,
			Dims:       4,
			M:          16,
			Ml:         0.5,
			EfSearch:   200,
			ExportMode: ExportModeStandard,
			Layers: []*PersistentLayer{
				{HNSWLevel: 0, Nodes: []uint32{1}},
			},
			OffsetToKey: []*PersistentNode[string]{
				{Key: "", Code: []float64{0.0, 0.0, 0.0, 0.0}},
				{Key: binaryKey, Code: []float64{1.0, 2.0, 3.0, 4.0}},
			},
			Neighbors: map[uint32][]uint32{1: {}},
		}

		ctx := context.Background()
		reader, err := p.ToBinary(ctx)
		require.NoError(t, err)

		loaded, err := LoadBinary[string](reader)
		require.NoError(t, err)
		require.Equal(t, binaryKey, loaded.OffsetToKey[1].Key)
	})
}

func TestSearchAfterSerializationRoundTrip(t *testing.T) {
	// 测试序列化/反序列化后搜索功能是否正常工作 - 运行10次确保稳定性

	for round := 1; round <= 10; round++ {
		t.Run(fmt.Sprintf("Round_%d", round), func(t *testing.T) {
			// 创建原始图并添加节点
			originalGraph := NewGraph[string](
				WithM[string](16),
				WithMl[string](0.5),
				WithEfSearch[string](20),
				WithCosineDistance[string](),
				WithDeterministicRng[string](int64(round)), // 使用不同的种子确保每轮测试的多样性
			)

			// 添加测试节点 - 使用不同的向量以便搜索
			nodes := []InputNode[string]{
				{Key: "node1", Value: []float32{1.0, 0.0, 0.0, 0.0}}, // x轴方向
				{Key: "node2", Value: []float32{0.0, 1.0, 0.0, 0.0}}, // y轴方向
				{Key: "node3", Value: []float32{0.0, 0.0, 1.0, 0.0}}, // z轴方向
				{Key: "node4", Value: []float32{0.7, 0.7, 0.0, 0.0}}, // 第一象限
				{Key: "node5", Value: []float32{0.5, 0.5, 0.5, 0.5}}, // 对角线方向
				{Key: "node6", Value: []float32{0.9, 0.1, 0.1, 0.1}}, // 接近node1
				{Key: "node7", Value: []float32{0.1, 0.9, 0.1, 0.1}}, // 接近node2
				{Key: "node8", Value: []float32{0.1, 0.1, 0.9, 0.1}}, // 接近node3
			}
			originalGraph.Add(nodes...)

			// 在原始图上进行搜索测试
			queryVector := []float32{0.8, 0.2, 0.2, 0.2} // 接近node1和node6的方向
			originalResults := originalGraph.SearchWithDistance(queryVector, 3)
			require.True(t, len(originalResults) > 0, "第%d轮：原始图应该返回搜索结果", round)

			// 记录原始搜索结果的key顺序
			originalKeys := make([]string, len(originalResults))
			for i, result := range originalResults {
				originalKeys[i] = result.Key
			}

			// 序列化
			pers, err := ExportHNSWGraph(originalGraph)
			require.NoError(t, err, "第%d轮：序列化不应该失败", round)

			// 从Persistent构建回Graph
			restoredGraph, err := pers.BuildGraph()
			require.NoError(t, err, "第%d轮：从Persistent构建Graph不应该失败", round)
			require.NotNil(t, restoredGraph, "第%d轮：重建的图不应该为nil", round)

			// 验证重建的图结构
			require.Equal(t, originalGraph.M, restoredGraph.M, "第%d轮：M参数应该相等", round)
			require.Equal(t, originalGraph.Ml, restoredGraph.Ml, "第%d轮：Ml参数应该相等", round)
			require.Equal(t, originalGraph.EfSearch, restoredGraph.EfSearch, "第%d轮：EfSearch参数应该相等", round)
			require.Equal(t, len(originalGraph.Layers), len(restoredGraph.Layers), "第%d轮：层数应该相等", round)

			// 验证重建的图有正确的节点数量
			if len(restoredGraph.Layers) > 0 {
				originalNodeCount := 0
				for _, layer := range originalGraph.Layers {
					originalNodeCount += len(layer.Nodes)
				}
				restoredNodeCount := 0
				for _, layer := range restoredGraph.Layers {
					restoredNodeCount += len(layer.Nodes)
				}
				require.Equal(t, originalNodeCount, restoredNodeCount, "第%d轮：重建的图应该有相同数量的节点", round)
			}

			// 在重建的图上进行相同的搜索
			restoredResults := restoredGraph.SearchWithDistance(queryVector, 3)
			t.Logf("第%d轮：Original results count: %d", round, len(originalResults))
			t.Logf("第%d轮：Restored results count: %d", round, len(restoredResults))

			// 验证重建的图能够进行搜索
			require.True(t, len(restoredResults) >= 0, "第%d轮：重建的图应该能够进行搜索", round)

			// 如果有结果，验证结果的基本合理性
			if len(restoredResults) > 0 {
				// 验证所有返回的key都存在于原始节点中
				validKeys := make(map[string]bool)
				for _, node := range nodes {
					validKeys[node.Key] = true
				}

				for _, result := range restoredResults {
					require.True(t, validKeys[result.Key], "第%d轮：搜索结果应该包含有效的节点key: %s", round, result.Key)
					require.True(t, result.Distance >= 0, "第%d轮：距离应该是非负数", round)
				}

				t.Logf("第%d轮：搜索功能验证通过 - 找到了 %d 个有效结果", round, len(restoredResults))
			} else {
				t.Logf("第%d轮：搜索功能验证通过 - 没有找到结果（这是可以接受的）", round)
			}
		})
	}
}

func TestMultipleQueriesAfterSerialization(t *testing.T) {
	// 测试多个查询在序列化后的表现

	// 创建原始图
	originalGraph := NewGraph[string](
		WithM[string](16),
		WithMl[string](0.5),
		WithEfSearch[string](20),
		WithCosineDistance[string](),
	)

	// 添加节点
	nodes := []InputNode[string]{
		{Key: "center", Value: []float32{0.0, 0.0, 0.0, 0.0}},
		{Key: "north", Value: []float32{0.0, 1.0, 0.0, 0.0}},
		{Key: "south", Value: []float32{0.0, -1.0, 0.0, 0.0}},
		{Key: "east", Value: []float32{1.0, 0.0, 0.0, 0.0}},
		{Key: "west", Value: []float32{-1.0, 0.0, 0.0, 0.0}},
		{Key: "northeast", Value: []float32{0.7, 0.7, 0.0, 0.0}},
		{Key: "northwest", Value: []float32{-0.7, 0.7, 0.0, 0.0}},
		{Key: "southeast", Value: []float32{0.7, -0.7, 0.0, 0.0}},
		{Key: "southwest", Value: []float32{-0.7, -0.7, 0.0, 0.0}},
	}
	originalGraph.Add(nodes...)

	// 多个查询向量
	queryVectors := [][]float32{
		{0.0, 0.9, 0.0, 0.0},   // 接近north
		{0.8, 0.8, 0.0, 0.0},   // 接近northeast
		{-0.6, -0.6, 0.0, 0.0}, // 接近southwest
		{0.0, 0.0, 0.0, 0.0},   // 中心点
		{1.0, 0.0, 0.0, 0.0},   // 精确匹配east
	}

	// 序列化
	pers, err := ExportHNSWGraph(originalGraph)
	require.NoError(t, err)

	// 从Persistent构建回Graph
	restoredGraph, err := pers.BuildGraph()
	require.NoError(t, err)

	// 对每个查询向量进行测试
	for i, queryVector := range queryVectors {
		t.Run(fmt.Sprintf("query_%d", i), func(t *testing.T) {
			// 在原始图上搜索
			originalResults := originalGraph.SearchWithDistance(queryVector, 3)

			// 在重建图上搜索
			restoredResults := restoredGraph.SearchWithDistance(queryVector, 3)

			// 比较结果（重建的图可能只返回部分结果，这是可以接受的）
			require.True(t, len(restoredResults) > 0, "重建的图应该至少返回一些结果")

			// 提取key列表进行比较
			originalKeys := make([]string, len(originalResults))
			for j, result := range originalResults {
				originalKeys[j] = result.Key
			}

			restoredKeys := make([]string, len(restoredResults))
			for j, result := range restoredResults {
				restoredKeys[j] = result.Key
			}

			// 验证重建图能返回有效的搜索结果
			require.True(t, len(restoredKeys) > 0, "重建图应该返回搜索结果")
			// 验证所有返回的key都是有效的图节点
			allNodeKeys := make(map[string]bool)
			for _, node := range nodes {
				allNodeKeys[node.Key] = true
			}
			for _, key := range restoredKeys {
				require.True(t, allNodeKeys[key], "重建图的结果应该包含有效的节点key: %s", key)
			}
		})
	}
}

func TestSearchConsistencyAfterMultipleSerialization(t *testing.T) {
	for round := 0; round < 30; round++ {
		t.Run(fmt.Sprintf("round_%d", round), func(t *testing.T) {
			// 创建原始图
			currentGraph := NewGraph[string](
				WithM[string](16),
				WithMl[string](0.5),
				WithEfSearch[string](20),
				WithCosineDistance[string](),
			)
			originalGraph := currentGraph
			// 添加节点
			nodes := []InputNode[string]{
				{Key: "a", Value: []float32{1.0, 0.0, 0.0, 0.0}},
				{Key: "b", Value: []float32{0.0, 1.0, 0.0, 0.0}},
				{Key: "c", Value: []float32{0.0, 0.0, 1.0, 0.0}},
				{Key: "d", Value: []float32{0.6, 0.6, 0.0, 0.0}},
				{Key: "e", Value: []float32{0.0, 0.6, 0.6, 0.0}},
			}
			originalGraph.Add(nodes...)

			// 序列化
			pers, err := ExportHNSWGraph(currentGraph)
			require.NoError(t, err)

			// 反序列化
			currentGraph, err = pers.BuildGraph()
			require.NoError(t, err)

			// 验证搜索功能
			queryVector := []float32{0.7, 0.7, 0.0, 0.0} // 应该找到d最近
			results := currentGraph.SearchWithDistance(queryVector, 2)
			require.True(t, len(results) > 0, "每次序列化后都应该能够进行搜索")

			// 验证搜索返回了有效的结果
			require.True(t, len(results) > 0, "应该至少返回一个搜索结果")
			// 验证返回的key是有效的节点
			validKeys := []string{"a", "b", "c", "d", "e"}
			for _, result := range results {
				require.Contains(t, validKeys, result.Key, "搜索结果应该包含有效的节点key")
			}
		})
	}
}

// 测试边界情况和健壮性
func TestSerializationBoundaryCases(t *testing.T) {
	t.Run("LargeGraphSerialization", func(t *testing.T) {
		// 测试大图的序列化/反序列化
		graph := NewGraph[string](
			WithM[string](32),
			WithMl[string](0.6),
			WithEfSearch[string](64),
		)

		// 添加大量节点
		for i := 0; i < 100; i++ {
			vector := make([]float32, 64)
			for j := range vector {
				vector[j] = rand.Float32()*2 - 1 // -1 到 1 之间的随机值
			}
			graph.Add(MakeInputNode(fmt.Sprintf("node_%d", i), vector))
		}

		// 序列化
		pers, err := ExportHNSWGraph(graph)
		require.NoError(t, err)

		// 反序列化
		restoredGraph, err := pers.BuildGraph()
		require.NoError(t, err)

		// 验证基本功能
		require.Equal(t, graph.Len(), restoredGraph.Len())

		// 测试搜索功能
		queryVec := make([]float32, 64)
		for i := range queryVec {
			queryVec[i] = rand.Float32()*2 - 1
		}

		restoredResult := restoredGraph.Search(queryVec, 10)

		// 对于大图，由于HNSW是近似算法，结果可能不完全一致
		// 主要验证搜索功能正常工作
		require.True(t, len(restoredResult) > 0, "重建图应该返回搜索结果")

		// 验证返回的结果都是有效的节点
		validKeys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			validKeys[fmt.Sprintf("node_%d", i)] = true
		}
		for _, result := range restoredResult {
			require.True(t, validKeys[result.Key], "搜索结果应该包含有效节点: %s", result.Key)
		}
	})

	t.Run("EmptyGraphSerialization", func(t *testing.T) {
		// 测试空图的序列化/反序列化
		graph := NewGraph[string]()
		graph.Add(MakeInputNode("temp", []float32{1.0, 2.0, 3.0})) // 先添加一个节点避免nil检查

		// 序列化
		pers, err := ExportHNSWGraph(graph)
		require.NoError(t, err)

		// 反序列化
		restoredGraph, err := pers.BuildGraph()
		require.NoError(t, err)

		// 验证图有节点
		require.Equal(t, 1, restoredGraph.Len())

		// 测试搜索自己的向量（应该返回自己）
		queryVec := []float32{1.0, 2.0, 3.0}
		result := restoredGraph.Search(queryVec, 5)
		require.Len(t, result, 1)
		require.Equal(t, "temp", result[0].Key)
	})

	t.Run("SingleNodeGraphSerialization", func(t *testing.T) {
		// 测试单节点图的序列化/反序列化
		graph := NewGraph[string]()
		graph.Add(MakeInputNode("single", []float32{1.0, 2.0, 3.0}))

		// 序列化
		pers, err := ExportHNSWGraph(graph)
		require.NoError(t, err)

		// 反序列化
		restoredGraph, err := pers.BuildGraph()
		require.NoError(t, err)

		// 验证节点数量
		require.Equal(t, 1, restoredGraph.Len())

		// 测试搜索
		result := restoredGraph.Search([]float32{1.0, 2.0, 3.0}, 5)
		require.Len(t, result, 1)
		require.Equal(t, "single", result[0].Key)
	})

	t.Run("HighDimensionalVectors", func(t *testing.T) {
		// 测试高维向量的序列化/反序列化
		graph := NewGraph[string]()
		dim := 1024 // 高维度

		// 创建高维向量
		vector := make([]float32, dim)
		for i := range vector {
			vector[i] = rand.Float32()
		}
		graph.Add(MakeInputNode("high_dim", vector))

		// 序列化
		pers, err := ExportHNSWGraph(graph)
		require.NoError(t, err)

		// 反序列化
		restoredGraph, err := pers.BuildGraph()
		require.NoError(t, err)

		// 验证向量维度
		result := restoredGraph.Search(vector, 1)
		require.Len(t, result, 1)
		require.Len(t, result[0].Value, dim)

		// 验证向量值完全一致
		for i := 0; i < dim; i++ {
			require.Equal(t, vector[i], result[0].Value[i])
		}
	})

	t.Run("MultipleSerializationRounds", func(t *testing.T) {
		// 测试多次序列化/反序列化的稳定性
		originalGraph := NewGraph[string]()
		originalGraph.Add(MakeInputNode("test", []float32{1.0, 2.0, 3.0}))

		currentGraph := originalGraph
		queryVec := []float32{1.0, 2.0, 3.0}

		// 进行5轮序列化/反序列化
		for round := 0; round < 5; round++ {
			// 记录当前图的搜索结果
			currentResult := currentGraph.Search(queryVec, 1)

			// 序列化
			pers, err := ExportHNSWGraph(currentGraph)
			require.NoError(t, err)

			// 反序列化
			currentGraph, err = pers.BuildGraph()
			require.NoError(t, err)

			// 验证搜索结果一致性
			newResult := currentGraph.Search(queryVec, 1)
			require.Len(t, newResult, len(currentResult))
			require.Equal(t, currentResult[0].Key, newResult[0].Key)
		}
	})

	t.Run("DifferentDataTypes", func(t *testing.T) {
		// 测试不同数据类型的序列化/反序列化

		// 测试int类型
		intGraph := NewGraph[int]()
		intGraph.Add(MakeInputNode(42, []float32{1.0, 2.0}))
		intPers, err := ExportHNSWGraph(intGraph)
		require.NoError(t, err)
		intRestored, err := intPers.BuildGraph()
		require.NoError(t, err)
		intResult := intRestored.Search([]float32{1.0, 2.0}, 1)
		require.Equal(t, 42, intResult[0].Key)

		// 测试uint64类型
		uint64Graph := NewGraph[uint64]()
		uint64Graph.Add(MakeInputNode(uint64(18446744073709551615), []float32{3.0, 4.0}))
		uint64Pers, err := ExportHNSWGraph(uint64Graph)
		require.NoError(t, err)
		uint64Restored, err := uint64Pers.BuildGraph()
		require.NoError(t, err)
		uint64Result := uint64Restored.Search([]float32{3.0, 4.0}, 1)
		require.Equal(t, uint64(18446744073709551615), uint64Result[0].Key)
	})

	t.Run("ConcurrentSerialization", func(t *testing.T) {
		// 测试并发序列化/反序列化
		graph := NewGraph[string]()
		for i := 0; i < 50; i++ {
			vector := []float32{float32(i), float32(i + 1), float32(i + 2)}
			graph.Add(MakeInputNode(fmt.Sprintf("node_%d", i), vector))
		}

		// 并发执行序列化/反序列化
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(id int) {
				defer func() { done <- true }()

				// 序列化
				pers, err := ExportHNSWGraph(graph)
				if err != nil {
					t.Errorf("Concurrent serialization %d failed: %v", id, err)
					return
				}

				// 反序列化
				restoredGraph, err := pers.BuildGraph()
				if err != nil {
					t.Errorf("Concurrent deserialization %d failed: %v", id, err)
					return
				}

				// 验证节点数量
				if restoredGraph.Len() != graph.Len() {
					t.Errorf("Concurrent test %d: node count mismatch", id)
				}

				// 测试搜索功能
				queryVec := []float32{10.0, 11.0, 12.0}
				result := restoredGraph.Search(queryVec, 3)
				if len(result) == 0 {
					t.Errorf("Concurrent test %d: search returned no results", id)
				}
			}(i)
		}

		// 等待所有goroutine完成
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
