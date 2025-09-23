package hnsw

import (
	"cmp"
	"slices"

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
	ExportMode  byte                  `json:"export_mode"` // pq mode: 1 / string mode: 2 / uid mode: 3
	PQCodebook  *PersistentPQCodebook `json:"pq_codebook"`
	Layers      []*PersistentLayer    `json:"layers"`
	OffsetToKey []*PersistentNode[K]  `json:"offset_to_node"`
	Neighbors   map[uint32][]uint32   `json:"neighbors"`
}

const (
	ExportModePQ       byte = 1
	ExportModeStandard byte = 2
	ExportModeUID      byte = 3
	ExportModeIntUID   byte = 4
	ExportModeStrUID   byte = 5
)

// getKeyType returns the string representation of the key type
func getKeyType[K cmp.Ordered](key K) string {
	var k any = key
	switch k.(type) {
	case string:
		return "string"
	case int:
		return "int64"
	case int64:
		return "int64"
	case uint64:
		return "uint64"
	case uint32:
		return "uint32"
	default:
		return ""
	}
}

func ExportHNSWGraph[K cmp.Ordered](i *Graph[K]) (*Persistent[K], error) {
	if i == nil {
		return nil, utils.Errorf("graph is nil")
	}

	// Handle empty graph case
	if len(i.Layers) == 0 {
		var zeroKey K
		keyType := getKeyType(zeroKey)
		if keyType == "" {
			return nil, utils.Errorf("unsupported key type, should be string, int/int64, uint64, uint32")
		}

		// Create empty persistent graph
		pers := &Persistent[K]{
			KeyType:     keyType,
			Total:       0,
			M:           uint32(i.M),
			Ml:          float32(i.Ml),
			EfSearch:    uint32(i.EfSearch),
			Dims:        uint32(i.Dims()),
			Layers:      []*PersistentLayer{},
			OffsetToKey: []*PersistentNode[K]{},
			Neighbors:   map[uint32][]uint32{},
			ExportMode:  ExportModeStandard, // Default to standard mode for empty graph
		}
		return pers, nil
	}

	if len(i.Layers[0].Nodes) == 0 {
		return nil, utils.Errorf("graph has no nodes")
	}

	keyType := ""
	var exportMode byte
	for _k, val := range i.Layers[0].Nodes {
		keyType = getKeyType(_k)
		switch ret := val.(type) {
		case *hnswspec.PQLayerNode[K]:
			exportMode = ExportModePQ
		case *hnswspec.StandardLayerNode[K]:
			exportMode = ExportModeStandard
		case *hnswspec.LazyLayerNode[K]:
			uid := ret.GetUID()
			switch uid.(type) {
			case string:
				exportMode = ExportModeStrUID
			case []byte:
				exportMode = ExportModeUID
			case int:
				exportMode = ExportModeIntUID
			case int64:
				exportMode = ExportModeIntUID
			case int32:
				exportMode = ExportModeIntUID
			case uint32:
				exportMode = ExportModeIntUID
			case uint64:
				exportMode = ExportModeIntUID
			default:
				return nil, utils.Errorf("unsupported uid type: %T", uid)
			}
		}
		break
	}

	if keyType == "" {
		return nil, utils.Errorf("unsupported key type, should be string, int/int64, uint64, uint32")
	}

	if exportMode == 0 {
		return nil, utils.Errorf("unsupported export mode")
	}

	var total uint32 = 0
	for _, layer := range i.Layers {
		total += uint32(len(layer.Nodes))
	}

	nodeStorage := make([]*PersistentNode[K], 1, total+1)
	// reserve 0 offset with dummy data
	var zeroKey K
	if exportMode == ExportModeUID {
		nodeStorage[0] = &PersistentNode[K]{Key: zeroKey, Code: ""}
	} else {
		if i.IsPQEnabled() {
			pqCodeSize := i.pqCodebook.M
			nodeStorage[0] = &PersistentNode[K]{Key: zeroKey, Code: make([]byte, pqCodeSize)}
		} else {
			dims := i.Dims()
			dummyVec := make([]float64, dims) // ToBinary expects []float64 for non-PQ mode
			nodeStorage[0] = &PersistentNode[K]{Key: zeroKey, Code: dummyVec}
		}
	}

	pers := &Persistent[K]{
		Total:       total,
		M:           uint32(i.M),
		Ml:          float32(i.Ml),
		EfSearch:    uint32(i.EfSearch),
		Dims:        uint32(i.Dims()),
		OffsetToKey: nodeStorage, // 0 offset is reserved
		Neighbors:   map[uint32][]uint32{},
		ExportMode:  exportMode,
	}

	if exportMode == ExportModePQ {
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
		switch pers.ExportMode {
		case ExportModePQ:
			if pers.ExportMode != ExportModePQ {
				return 0, utils.Errorf("pq mode disabled but node data is []byte")
			}
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
		case ExportModeStandard:
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
		case ExportModeUID, ExportModeIntUID, ExportModeStrUID:
			pers.OffsetToKey = append(pers.OffsetToKey, &PersistentNode[K]{
				Code: i.GetData(),
				Key:  i.GetKey(),
			})
		default:
			return 0, utils.Errorf("unsupported node data type: %T", pers.ExportMode)
		}
		return currentOffset, nil
	}

	layerOffset := []uint32{}
	preOffset := uint32(0)
	for _, layer := range i.Layers {
		layerOffset = append(layerOffset, preOffset)
		preOffset += uint32(len(layer.Nodes))
	}

	loadOffsetWithLevel := func(level uint32, node hnswspec.LayerNode[K]) (uint32, error) {
		offset, err := loadOffset(node)
		if err != nil {
			return 0, err
		}
		if level == 0 {
			return offset, nil
		}
		layerOffset := layerOffset[level]
		return offset + layerOffset, nil
	}

	pers.Layers = make([]*PersistentLayer, len(i.Layers))
	for level, layer := range i.Layers {
		pl := &PersistentLayer{
			HNSWLevel: uint32(level),
			Nodes:     make([]uint32, 0, len(layer.Nodes)),
		}
		pers.Layers[level] = pl
		keys := make([]K, 0, len(layer.Nodes))
		for key := range layer.Nodes {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			node := layer.Nodes[key]
			offset, err := loadOffsetWithLevel(uint32(level), node)
			if err != nil {
				return nil, err
			}
			pl.Nodes = append(pl.Nodes, offset)
			ns := node.GetNeighbors()
			nsIdxs := make([]uint32, 0, len(ns))
			for _, neighbor := range ns {
				offsetFromNeighbor, err := loadOffsetWithLevel(uint32(level), neighbor)
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
