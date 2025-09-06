package hnsw

import (
	"bytes"
	"context"
	"io"
	"math"

	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/protobuf/encoding/protowire"
)

func (i *Persistent[K]) ToBinary(ctx context.Context) (io.Reader, error) {
	if i.Total <= 0 {
		return nil, utils.Error("cannot export empty graph")
	}

	if i.Dims <= 0 {
		return nil, utils.Error("invalid dimensions")
	}

	if i.M <= 0 {
		return nil, utils.Error("invalid hnsw.M")
	}

	if i.Ml <= 0 || i.Ml > 1 {
		return nil, utils.Error("invalid hnsw.Ml")
	}

	if i.EfSearch <= 0 {
		return nil, utils.Error("invalid hnsw.EfSearch")
	}

	if i.PQMode && i.PQCodebook == nil {
		return nil, utils.Error("pq mode enabled but pq codebook is nil")
	}

	if i.PQMode {
		if i.PQCodebook.K <= 0 || i.PQCodebook.M <= 0 || i.PQCodebook.SubVectorDim <= 0 || len(i.PQCodebook.Centroids) == 0 {
			return nil, utils.Errorf("invalid pq codebook, m:%v k:%v sub_vector_dim:%v centroids-length:%v", i.PQCodebook.M, i.PQCodebook.K, i.PQCodebook.SubVectorDim, len(i.PQCodebook.Centroids))
		}
	}

	if i.OffsetToKey == nil {
		return nil, utils.Error("offset to key mapping is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	var buf = new(bytes.Buffer)
	buf.WriteString("YAKHNSW") // magic header

	// version;
	if err := pbWriteUint32(buf, 1); err != nil {
		return nil, utils.Errorf("write version: %v", err)
	}

	if err := pbWriteUint32(buf, i.Total); err != nil {
		return nil, utils.Errorf("write total: %v", err)
	}
	if err := pbWriteUint32(buf, i.Dims); err != nil {
		return nil, utils.Errorf("write dims: %v", err)
	}
	if err := pbWriteUint32(buf, i.M); err != nil {
		return nil, utils.Errorf("write hnsw m: %v", err)
	}
	if err := pbWriteFloat32(buf, i.Ml); err != nil {
		return nil, utils.Errorf("write hnsw ml: %v", err)
	}
	if err := pbWriteUint32(buf, i.EfSearch); err != nil {
		return nil, utils.Errorf("write hnsw ef search: %v", err)
	}
	if err := pbWriteBool(buf, i.PQMode); err != nil {
		return nil, utils.Errorf("write hnsw pq mode: %v", err)
	}
	if i.PQMode {
		if err := pbWriteUint32(buf, i.PQCodebook.M); err != nil {
			return nil, utils.Errorf("write hnsw pq codebook m: %v", err)
		}
		if err := pbWriteUint32(buf, i.PQCodebook.K); err != nil {
			return nil, utils.Errorf("write hnsw pq codebook k: %v", err)
		}
		if err := pbWriteUint32(buf, i.PQCodebook.SubVectorDim); err != nil {
			return nil, utils.Errorf("write hnsw pq codebook sub vector dim: %v", err)
		}
		if err := pbWriteUint32(buf, i.PQCodebook.PQCodeSize); err != nil {
			return nil, utils.Errorf("write hnsw pq codebook pq code size: %v", err)
		}
		if err := pbWriteUint32(buf, uint32(len(i.PQCodebook.Centroids))); err != nil {
			return nil, utils.Errorf("write hnsw pq codebook centroids length: %v", err)
		}
		var centroids []byte
		pbcb := i.PQCodebook
		centroids = protowire.AppendVarint(centroids, uint64(len(pbcb.Centroids)))
		for _, l2 := range pbcb.Centroids {
			centroids = protowire.AppendVarint(centroids, uint64(len(l2)))
			for _, l3 := range l2 {
				centroids = protowire.AppendVarint(centroids, uint64(len(l3)))
				for _, f := range l3 {
					// 将 float64 的位模式转换为 uint64
					bits := math.Float64bits(f)
					// 使用 AppendFixed64 以固定8字节追加
					centroids = protowire.AppendFixed64(centroids, bits)
				}
			}
		}
		buf.Write(centroids)
	}

	// layers
	pbWriteVarint(buf, uint64(len(i.Layers)))
	for _, layer := range i.Layers {
		pbWriteVarint(buf, uint64(len(layer.Nodes)))
		for _, node := range layer.Nodes {
			pbWriteVarint(buf, uint64(node))
		}
	}

	// nodes
	pbWriteVarint(buf, uint64(len(i.OffsetToKey)))
	for _, node := range i.OffsetToKey {
		switch ret := node.Code.(type) {
		case []byte:
			if !i.PQMode {
				return nil, utils.Errorf("pq mode disabled but node code is []byte")
			}
			if len(ret) != int(i.PQCodebook.PQCodeSize) {
				return nil, utils.Errorf("pq code size mismatch: expected %d, got %d", i.PQCodebook.PQCodeSize, len(ret))
			}
			buf.Write(ret)
		case []float64:
			if i.PQMode {
				return nil, utils.Errorf("pq mode enabled but node code is []float64")
			}
			if len(ret) != int(i.Dims) {
				return nil, utils.Errorf("vector dimension mismatch: expected %d, got %d", i.Dims, len(ret))
			}
			for _, f := range node.Code.([]float64) {
				pbWriteFloat64(buf, f)
			}
		default:
			return nil, utils.Errorf("unsupported node code type: %T", ret)
		}
	}

	// neighbors
	pbWriteVarint(buf, uint64(len(i.Neighbors)))
	for offset, neighbors := range i.Neighbors {
		pbWriteVarint(buf, uint64(offset))
		pbWriteVarint(buf, uint64(len(neighbors)))
		for _, neighbor := range neighbors {
			pbWriteVarint(buf, uint64(neighbor))
		}
	}

	return buf, nil
}
