package hnsw

import (
	"cmp"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/utils"
)

type PersistentPQCodebook struct {
	M              uint32        `json:"m"` // 1024 维度一般来说选择 32
	K              uint32        `json:"k"` // 子空间聚类中心量
	PQCodeByteSize uint32        `json:"PQCodeByteSize"`
	SubVectorDim   uint32        `json:"sub_vector_dim"`
	Centroids      [][][]float64 `json:"centroids"`
}

type PersistentNode[K cmp.Ordered] struct {
	Key K
	// pq mode is [8]byte to save pq code
	// non-pq mode is []float32 to save full vector
	Code any
}

type PersistentLayer struct {
	HNSWLevel uint32   `json:"level"` // 层级，越到高级，节点越少，查询的时候一般从高级开始查询
	Nodes     []uint32 `json:"nodes"`
}

type Persistent[K cmp.Ordered] struct {
	KeyType     string                `json:"key_type"`
	Total       uint32                `json:"total"`
	Dims        uint32                `json:"dims"` // 向量维度，1024 默认
	M           uint32                `json:"m"`
	Ml          float32               `json:"ml"`
	EfSearch    uint32                `json:"ef_search"`
	PQMode      bool                  `json:"pq_mode"`
	PQCodebook  *PersistentPQCodebook `json:"pq_codebook"`
	Layers      []*PersistentLayer    `json:"layers"`
	OffsetToKey []*PersistentNode[K]  `json:"offset_to_node"`
	Neighbors   map[uint32][]uint32   `json:"neighbors"`
}

func ExportHNSWGraph[K cmp.Ordered](i *Graph[K]) (*Persistent[K], error) {
	if i == nil || len(i.Layers) == 0 || len(i.Layers[0].Nodes) == 0 {
		return nil, utils.Errorf("graph is nil")
	}
	keyType := ""
	for _k := range i.Layers[0].Nodes {
		var k any = _k
		switch k.(type) {
		case string:
			keyType = "string"
		case int:
			keyType = "int64"
		case int64:
			keyType = "int64"
		case uint64:
			keyType = "uint64"
		case uint32:
			keyType = "uint32"
		}
		break
	}

	if keyType == "" {
		return nil, utils.Errorf("unsupported key type, should be string, int/int64, uint64, uint32")
	}

	var total uint32 = 0
	for _, layer := range i.Layers {
		total += uint32(len(layer.Nodes))
	}

	nodeStorage := make([]*PersistentNode[K], 1, total+1)
	// reserve 0 offset with dummy data
	var zeroKey K
	if i.IsPQEnabled() {
		pqCodeSize := i.pqCodebook.M
		nodeStorage[0] = &PersistentNode[K]{Key: zeroKey, Code: make([]byte, pqCodeSize)}
	} else {
		dims := i.Dims()
		dummyVec := make([]float64, dims) // ToBinary expects []float64 for non-PQ mode
		nodeStorage[0] = &PersistentNode[K]{Key: zeroKey, Code: dummyVec}
	}

	pers := &Persistent[K]{
		Total:       total,
		M:           uint32(i.M),
		Ml:          float32(i.Ml),
		EfSearch:    uint32(i.EfSearch),
		Dims:        uint32(i.Dims()),
		OffsetToKey: nodeStorage, // 0 offset is reserved
		Neighbors:   map[uint32][]uint32{},
	}

	if i.IsPQEnabled() {
		pers.PQMode = true
		pers.PQCodebook = &PersistentPQCodebook{
			M:              uint32(i.pqCodebook.M),
			K:              uint32(i.pqCodebook.K),
			SubVectorDim:   uint32(i.pqCodebook.SubVectorDim),
			Centroids:      i.pqCodebook.Centroids,
			PQCodeByteSize: uint32(i.pqCodebook.M),
		}
	}

	var currentOffset uint32 = 0
	k2idx := map[K]uint32{}
	loadOffset := func(i hnswspec.LayerNode[K]) (uint32, error) {
		_, ok := k2idx[i.GetKey()]
		if ok {
			return k2idx[i.GetKey()], nil
		}

		currentOffset++
		k2idx[i.GetKey()] = currentOffset
		if pers.PQMode {
			codes, ok := i.GetPQCodes()
			if !ok {
				return 0, utils.Errorf("node %v does not have PQ codes", i.GetKey())
			}
			if uint32(len(codes)) != pers.PQCodebook.PQCodeByteSize {
				return 0, utils.Errorf("PQ code size mismatch: expected %d, got %d", pers.PQCodebook.PQCodeByteSize, len(codes))
			}
			pers.OffsetToKey = append(pers.OffsetToKey, &PersistentNode[K]{
				Code: codes,
				Key:  i.GetKey(),
			})
		} else {
			result := i.GetVector()()
			if len(result) != int(pers.Dims) {
				return 0, utils.Errorf("vector dimension mismatch: expected %d, got %d", pers.Dims, len(result))
			}
			// Convert []float32 to []float64 for ToBinary compatibility
			float64Vec := make([]float64, len(result))
			for j, v := range result {
				float64Vec[j] = float64(v)
			}
			pers.OffsetToKey = append(pers.OffsetToKey, &PersistentNode[K]{
				Code: float64Vec,
				Key:  i.GetKey(),
			})
		}
		return currentOffset, nil
	}

	pers.Layers = make([]*PersistentLayer, len(i.Layers))
	for level, layer := range i.Layers {
		pl := &PersistentLayer{
			HNSWLevel: uint32(level),
			Nodes:     make([]uint32, 0, len(layer.Nodes)),
		}
		pers.Layers[level] = pl
		for _, node := range layer.Nodes {
			offset, err := loadOffset(node)
			if err != nil {
				return nil, err
			}
			pl.Nodes = append(pl.Nodes, offset)
			ns := node.GetNeighbors()
			nsIdxs := make([]uint32, 0, len(ns))
			for _, neighbor := range ns {
				offsetFromNeighbor, err := loadOffset(neighbor)
				if err != nil {
					return nil, err
				}
				nsIdxs = append(nsIdxs, offsetFromNeighbor)
			}
			pers.Neighbors[offset] = nsIdxs
		}
	}
	return pers, nil
}
