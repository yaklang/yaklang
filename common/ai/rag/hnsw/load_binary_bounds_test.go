package hnsw

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func binaryBoundsSeed(t testing.TB) []byte {
	t.Helper()
	g := NewGraph[string]()
	g.Add(InputNode[string]{Key: "node", Value: []float32{1, 0, 1, 0, 0, 0, 0}})
	r, err := ExportGraphToBinary(g)
	require.NoError(t, err)
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return b
}

func TestMUSTPASS_LoadBinaryRejectsOversizedCounts(t *testing.T) {
	seed := binaryBoundsSeed(t)
	// Seven-byte magic, six fixed32 header fields, then export mode.
	for _, value := range []uint64{math.MaxUint64, math.MaxUint32, 1 << 40} {
		bad := append([]byte(nil), seed[:32]...)
		bad = protowire.AppendVarint(bad, value) // layer count
		_, err := LoadBinary[string](bytes.NewReader(bad))
		require.Error(t, err)
	}
	bad := append([]byte(nil), seed...)
	binary.LittleEndian.PutUint32(bad[15:19], math.MaxUint32) // vector dimensions
	_, err := LoadBinary[string](bytes.NewReader(bad))
	require.Error(t, err)
	for n := 0; n < len(seed); n++ {
		_, err := LoadBinary[string](bytes.NewReader(seed[:n]))
		require.Error(t, err, "accepted truncated graph at byte %d", n)
	}
}

func FuzzLoadBinaryBounded(f *testing.F) {
	f.Add(binaryBoundsSeed(f))
	f.Add([]byte("YAKHNSW"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			t.Skip()
		}
		// Decoding malformed blobs must return an error, not panic or allocate
		// in proportion to forged lengths unrelated to the actual input.
		_, _ = LoadBinary[string](bytes.NewReader(data))
		_, _ = ImportCodebook(bytes.NewReader(data))
	})
}

func TestMUSTPASS_ImportCodebookRejectsOversizedCounts(t *testing.T) {
	for _, values := range [][3]uint64{{math.MaxUint64, 1, 1}, {1, 1 << 40, 1}, {1, 1, 1 << 40}} {
		var data []byte
		for _, v := range values {
			data = protowire.AppendVarint(data, v)
		}
		_, err := ImportCodebook(bytes.NewReader(data))
		require.Error(t, err)
	}
}
