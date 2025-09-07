package hnsw

import (
	"io"
	"math"

	"cmp"

	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
)

func LoadBinary[K cmp.Ordered](r io.Reader) (*Persistent[K], error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, utils.Wrap(err, "read all data")
	}

	if len(data) < 7 || string(data[:7]) != "YAKHNSW" {
		return nil, utils.Error("invalid magic header")
	}

	offset := 7

	consumeUint32 := func() (uint32, error) {
		v, n := protowire.ConsumeFixed32(data[offset:])
		if n < 0 {
			return 0, utils.Errorf("consume fixed32: %d", n)
		}
		offset += n
		return v, nil
	}

	consumeFloat32 := func() (float32, error) {
		v, err := consumeUint32()
		if err != nil {
			return 0, err
		}
		return math.Float32frombits(v), nil
	}

	consumeFloat64 := func() (float64, error) {
		v, n := protowire.ConsumeFixed64(data[offset:])
		if n < 0 {
			return 0, utils.Errorf("consume fixed64: %d", n)
		}
		offset += n
		return math.Float64frombits(v), nil
	}

	consumeVarint := func() (uint64, error) {
		v, n := protowire.ConsumeVarint(data[offset:])
		if n < 0 {
			return 0, utils.Errorf("consume varint: %d", n)
		}
		offset += n
		return v, nil
	}

	consumeBool := func() (bool, error) {
		v, err := consumeVarint()
		if err != nil {
			return false, err
		}
		return v != 0, nil
	}

	// version
	ver, err := consumeUint32()
	if err != nil {
		return nil, utils.Wrap(err, "version")
	}
	if ver != 1 {
		return nil, utils.Errorf("unsupported version: %d", ver)
	}

	// total
	total, err := consumeUint32()
	if err != nil {
		return nil, utils.Wrap(err, "total")
	}

	// dims
	dims, err := consumeUint32()
	if err != nil {
		return nil, utils.Wrap(err, "dims")
	}

	// m
	m, err := consumeUint32()
	if err != nil {
		return nil, utils.Wrap(err, "m")
	}

	// ml
	ml, err := consumeFloat32()
	if err != nil {
		return nil, utils.Wrap(err, "ml")
	}

	// efsearch
	efsearch, err := consumeUint32()
	if err != nil {
		return nil, utils.Wrap(err, "efsearch")
	}

	// pqmode
	pqmode, err := consumeBool()
	if err != nil {
		return nil, utils.Wrap(err, "pqmode")
	}

	p := &Persistent[K]{
		Total:    total,
		Dims:     dims,
		M:        m,
		Ml:       ml,
		EfSearch: efsearch,
		PQMode:   pqmode,
	}

	if pqmode {
		// pq m
		pqm, err := consumeUint32()
		if err != nil {
			return nil, utils.Wrap(err, "pq m")
		}

		// pq k
		pqk, err := consumeUint32()
		if err != nil {
			return nil, utils.Wrap(err, "pq k")
		}

		// sub vector dim
		subdim, err := consumeUint32()
		if err != nil {
			return nil, utils.Wrap(err, "sub vector dim")
		}

		// pq code size
		codesize, err := consumeUint32()
		if err != nil {
			return nil, utils.Wrap(err, "pq code size")
		}

		// len centroids (uint32)
		lenCentroidsUint32, err := consumeUint32()
		if err != nil {
			return nil, utils.Wrap(err, "len centroids (uint32)")
		}
		lenCentroids := uint64(lenCentroidsUint32)

		p.PQCodebook = &PersistentPQCodebook{
			M:              pqm,
			K:              pqk,
			SubVectorDim:   subdim,
			PQCodeByteSize: codesize,
		}

		// consume centroids
		l1len, err := consumeVarint()
		if err != nil {
			return nil, utils.Wrap(err, "centroids l1 len")
		}
		if l1len != lenCentroids {
			return nil, utils.Errorf("centroids len mismatch: %d vs %d", l1len, lenCentroids)
		}

		p.PQCodebook.Centroids = make([][][]float64, l1len)
		for i := uint64(0); i < l1len; i++ {
			l2len, err := consumeVarint()
			if err != nil {
				return nil, utils.Wrap(err, "centroids l2 len")
			}
			l2 := make([][]float64, l2len)
			for j := uint64(0); j < l2len; j++ {
				l3len, err := consumeVarint()
				if err != nil {
					return nil, utils.Wrap(err, "centroids l3 len")
				}
				l3 := make([]float64, l3len)
				for k := uint64(0); k < l3len; k++ {
					f, err := consumeFloat64()
					if err != nil {
						return nil, utils.Wrap(err, "centroids float")
					}
					l3[k] = f
				}
				l2[j] = l3
			}
			p.PQCodebook.Centroids[i] = l2
		}
	}

	// layers
	layersLen, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "layers len")
	}
	p.Layers = make([]*PersistentLayer, layersLen)
	for i := uint64(0); i < layersLen; i++ {
		nodesLen, err := consumeVarint()
		if err != nil {
			return nil, utils.Wrap(err, "layer nodes len")
		}
		nodes := make([]uint32, nodesLen)
		for j := uint64(0); j < nodesLen; j++ {
			node, err := consumeVarint()
			if err != nil {
				return nil, utils.Wrap(err, "layer node")
			}
			nodes[j] = uint32(node)
		}
		p.Layers[i] = &PersistentLayer{
			HNSWLevel: uint32(i),
			Nodes:     nodes,
		}
	}

	// nodes (offset to key codes)
	offsetToKeyLen, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "offset to key len")
	}
	p.OffsetToKey = make([]*PersistentNode[K], offsetToKeyLen)
	var zero K
	for i := uint64(0); i < offsetToKeyLen; i++ {
		// read key
		// key is always string in binary format

		var code any
		if p.PQMode {
			size := int(p.PQCodebook.PQCodeByteSize)
			if offset+size > len(data) {
				return nil, utils.Error("not enough data for pq code")
			}
			tempCode := make([]byte, size)
			copy(tempCode, data[offset:offset+size])
			offset += size
			code = tempCode
		} else {
			vec := make([]float64, p.Dims)
			for j := uint32(0); j < p.Dims; j++ {
				f, err := consumeFloat64()
				if err != nil {
					return nil, utils.Wrap(err, "vector float")
				}
				vec[j] = f
			}
			code = vec
		}
		p.OffsetToKey[i] = &PersistentNode[K]{
			Key:  zero,
			Code: code,
		}
	}

	// neighbors
	neighborsLen, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "neighbors len")
	}
	p.Neighbors = make(map[uint32][]uint32, neighborsLen)
	for i := uint64(0); i < neighborsLen; i++ {
		off, err := consumeVarint()
		if err != nil {
			return nil, utils.Wrap(err, "neighbor offset")
		}
		lenNs, err := consumeVarint()
		if err != nil {
			return nil, utils.Wrap(err, "neighbor len")
		}
		ns := make([]uint32, lenNs)
		for j := uint64(0); j < lenNs; j++ {
			n, err := consumeVarint()
			if err != nil {
				return nil, utils.Wrap(err, "neighbor")
			}
			ns[j] = uint32(n)
		}
		p.Neighbors[uint32(off)] = ns
	}

	if offset != len(data) {
		return nil, utils.Errorf("extra data after parsing: %d bytes", len(data)-offset)
	}

	return p, nil
}

// BuildGraph 从 Persistent 构建 Graph[K]
func (p *Persistent[K]) BuildGraph() (*Graph[K], error) {
	if p.Total <= 0 {
		return nil, utils.Error("cannot build graph from empty persistent")
	}

	if p.Dims <= 0 {
		return nil, utils.Error("invalid dimensions")
	}

	if p.M <= 0 {
		return nil, utils.Error("invalid hnsw.M")
	}

	if p.Ml <= 0 || p.Ml > 1 {
		return nil, utils.Error("invalid hnsw.Ml")
	}

	if p.EfSearch <= 0 {
		return nil, utils.Error("invalid hnsw.EfSearch")
	}

	if p.PQMode && p.PQCodebook == nil {
		return nil, utils.Error("pq mode enabled but pq codebook is nil")
	}

	if p.OffsetToKey == nil {
		return nil, utils.Error("offset to key mapping is nil")
	}

	g := NewGraph[K]()
	g.M = int(p.M)
	g.Ml = float64(p.Ml)
	g.EfSearch = int(p.EfSearch)

	if p.PQMode {
		g.pqCodebook = &pq.Codebook{
			M:            int(p.PQCodebook.M),
			K:            int(p.PQCodebook.K),
			SubVectorDim: int(p.PQCodebook.SubVectorDim),
			Centroids:    p.PQCodebook.Centroids,
		}
		g.pqQuantizer = pq.NewQuantizer(g.pqCodebook)
	}

	// 创建节点映射
	nodes := make(map[uint32]hnswspec.LayerNode[K])
	for offset, node := range p.OffsetToKey {
		if offset == 0 {
			continue // 跳过 0 offset
		}

		key := node.Key
		var vec Vector

		if p.PQMode {
			codes, ok := node.Code.([]byte)
			if !ok {
				return nil, utils.Errorf("expected []byte for pq code, got %T", node.Code)
			}
			if len(codes) != int(p.PQCodebook.PQCodeByteSize) {
				return nil, utils.Errorf("pq code size mismatch: expected %d, got %d", p.PQCodebook.PQCodeByteSize, len(codes))
			}
			// 对于 PQ 模式，我们需要创建一个有效的向量来初始化节点
			// 由于我们只有编码，我们创建一个虚拟向量
			dummyVec := make([]float64, int(p.Dims))
			for i := range dummyVec {
				dummyVec[i] = 0.0 // 使用零向量作为占位符
			}
			dummyVec32 := make([]float32, len(dummyVec))
			for i, v := range dummyVec {
				dummyVec32[i] = float32(v)
			}
			vecFunc := func() []float32 { return dummyVec32 }

			nodeObj, err := hnswspec.NewPQLayerNode(key, vecFunc, g.pqQuantizer)
			if err != nil {
				return nil, utils.Wrap(err, "create pq layer node")
			}
			// 注意：这里我们使用构造函数创建的节点，它的 PQ codes 已经根据输入向量计算
			// 如果需要使用原始的 codes，我们需要在 hnswspec 中添加设置方法
			nodes[uint32(offset)] = nodeObj
		} else {
			vecFloat64, ok := node.Code.([]float64)
			if !ok {
				return nil, utils.Errorf("expected []float64 for vector, got %T", node.Code)
			}
			if len(vecFloat64) != int(p.Dims) {
				return nil, utils.Errorf("vector dimension mismatch: expected %d, got %d", p.Dims, len(vecFloat64))
			}
			vecFloat32 := make([]float32, len(vecFloat64))
			for i, v := range vecFloat64 {
				vecFloat32[i] = float32(v)
			}
			vec = func() []float32 { return vecFloat32 }
			nodes[uint32(offset)] = hnswspec.NewStandardLayerNode(key, vec)
		}
	}

	// 构建层
	g.Layers = make([]*Layer[K], len(p.Layers))
	for level, layer := range p.Layers {
		layerNodes := make(map[K]hnswspec.LayerNode[K])
		for _, offset := range layer.Nodes {
			node, exists := nodes[offset]
			if !exists {
				return nil, utils.Errorf("node offset %d not found", offset)
			}
			key := node.GetKey()
			layerNodes[key] = node
		}
		g.Layers[level] = &Layer[K]{Nodes: layerNodes}
	}

	// 建立邻居连接
	for offset, neighbors := range p.Neighbors {
		node, exists := nodes[offset]
		if !exists {
			return nil, utils.Errorf("node offset %d not found for neighbors", offset)
		}

		for _, neighborOffset := range neighbors {
			neighbor, exists := nodes[neighborOffset]
			if !exists {
				return nil, utils.Errorf("neighbor offset %d not found", neighborOffset)
			}
			node.AddNeighbor(neighbor, g.M, g.nodeDistance)
		}
	}

	return g, nil
}
