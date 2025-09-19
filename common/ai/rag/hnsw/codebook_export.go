package hnsw

import (
	"bytes"
	"io"
	"math"

	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/utils"
	"google.golang.org/protobuf/encoding/protowire"
)

func ExportCodebook(codebook *pq.Codebook) (io.Reader, error) {
	if codebook == nil {
		return nil, utils.Error("codebook is nil")
	}

	if codebook.M <= 0 || codebook.K <= 0 || codebook.SubVectorDim <= 0 {
		return nil, utils.Errorf("invalid codebook parameters: M=%d, K=%d, SubVectorDim=%d",
			codebook.M, codebook.K, codebook.SubVectorDim)
	}

	if len(codebook.Centroids) != codebook.M {
		return nil, utils.Errorf("centroids length %d does not match M %d",
			len(codebook.Centroids), codebook.M)
	}

	var buf bytes.Buffer

	// Write header: M, K, SubVectorDim
	if err := pbWriteVarint(&buf, uint64(codebook.M)); err != nil {
		return nil, utils.Wrap(err, "write M")
	}
	if err := pbWriteVarint(&buf, uint64(codebook.K)); err != nil {
		return nil, utils.Wrap(err, "write K")
	}
	if err := pbWriteVarint(&buf, uint64(codebook.SubVectorDim)); err != nil {
		return nil, utils.Wrap(err, "write SubVectorDim")
	}

	// Write centroids data
	for m := 0; m < codebook.M; m++ {
		if len(codebook.Centroids[m]) != codebook.K {
			return nil, utils.Errorf("centroids[%d] length %d does not match K %d",
				m, len(codebook.Centroids[m]), codebook.K)
		}

		for k := 0; k < codebook.K; k++ {
			if len(codebook.Centroids[m][k]) != codebook.SubVectorDim {
				return nil, utils.Errorf("centroids[%d][%d] length %d does not match SubVectorDim %d",
					m, k, len(codebook.Centroids[m][k]), codebook.SubVectorDim)
			}

			for d := 0; d < codebook.SubVectorDim; d++ {
				if err := pbWriteFloat64(&buf, codebook.Centroids[m][k][d]); err != nil {
					return nil, utils.Wrapf(err, "write centroids[%d][%d][%d]", m, k, d)
				}
			}
		}
	}

	return &buf, nil
}

func ImportCodebook(reader io.Reader) (*pq.Codebook, error) {
	if reader == nil {
		return nil, utils.Error("reader is nil")
	}

	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read all data")
	}

	offset := 0

	consumeVarint := func() (uint64, error) {
		v, n := protowire.ConsumeVarint(data[offset:])
		if n < 0 {
			return 0, utils.Errorf("consume varint: %d", n)
		}
		offset += n
		return v, nil
	}

	consumeFloat64 := func() (float64, error) {
		v, n := protowire.ConsumeFixed64(data[offset:])
		if n < 0 {
			return 0, utils.Errorf("consume fixed64: %d", n)
		}
		offset += n
		return math.Float64frombits(v), nil
	}

	// Read header
	mVal, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "read M")
	}
	M := int(mVal)

	kVal, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "read K")
	}
	K := int(kVal)

	subVectorDimVal, err := consumeVarint()
	if err != nil {
		return nil, utils.Wrap(err, "read SubVectorDim")
	}
	SubVectorDim := int(subVectorDimVal)

	// Validate parameters
	if M <= 0 || K <= 0 || SubVectorDim <= 0 {
		return nil, utils.Errorf("invalid codebook parameters: M=%d, K=%d, SubVectorDim=%d",
			M, K, SubVectorDim)
	}

	// Initialize centroids
	centroids := make([][][]float64, M)
	for m := 0; m < M; m++ {
		centroids[m] = make([][]float64, K)
		for k := 0; k < K; k++ {
			centroids[m][k] = make([]float64, SubVectorDim)
			for d := 0; d < SubVectorDim; d++ {
				centroids[m][k][d], err = consumeFloat64()
				if err != nil {
					return nil, utils.Wrapf(err, "read centroids[%d][%d][%d]", m, k, d)
				}
			}
		}
	}

	codebook := &pq.Codebook{
		M:            M,
		K:            K,
		SubVectorDim: SubVectorDim,
		Centroids:    centroids,
	}

	return codebook, nil
}
