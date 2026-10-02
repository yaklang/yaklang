package sharkcli

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirstBatchT01Shark(t *testing.T) {
	var input bytes.Buffer
	for _, n := range []uint32{0x0a0d0d0a, 28, 0x1a2b3c4d, 1, 0xffffffff, 0xffffffff, 28, 1, 20, 1, 65535, 20, 6, 32, 0, 0, 0, 32 << 20, 32 << 20, 32} {
		require.NoError(t, binary.Write(&input, binary.LittleEndian, n))
	}
	path := filepath.Join(t.TempDir(), "invalid-caplen.pcapng")
	require.NoError(t, os.WriteFile(path, input.Bytes(), 0600))
	src, err := openOffline(path, "")
	if err == nil {
		defer src.Close()
		_, _, err = src.reader.ReadPacketData()
	}
	require.ErrorContains(t, err, "capture length")
}
