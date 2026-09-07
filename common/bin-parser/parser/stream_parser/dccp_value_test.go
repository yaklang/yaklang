package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDCCPDataChecksums(t *testing.T) {
	// Independent check vectors: CRC32c("123456789") = 0xe3069283;
	// CRC32c of the empty application-data area is zero (RFC 4340 9.3).
	for _, vector := range []struct {
		payload []byte
		crc     uint32
	}{{nil, 0}, {[]byte("123456789"), 0xe3069283}} {
		data := make([]byte, 28)
		data[4], data[8] = 7, 4 // X=0 DCCP-Data, 12-byte generic header.
		for _, at := range []int{12, 18} {
			data[at], data[at+1] = 44, 6
			binary.BigEndian.PutUint32(data[at+2:], vector.crc)
		}
		data = append(data, vector.payload...)
		count, err := validateDCCPDataChecksums(data)
		require.NoError(t, err)
		require.Equal(t, 2, count)
		for _, at := range []int{14, 20} {
			bad := bytes.Clone(data)
			bad[at] ^= 1
			count, err = validateDCCPDataChecksums(bad)
			require.Zero(t, count)
			require.ErrorContains(t, err, "CRC32c mismatch")
		}
		for end := 0; end < 28; end++ {
			_, err := validateDCCPDataChecksums(data[:end])
			require.Error(t, err)
		}
		// A malformed option prevents later bytes from being interpreted as a CRC.
		data[12], data[13] = 255, 1
		count, err = validateDCCPDataChecksums(data)
		require.NoError(t, err)
		require.Zero(t, count)
	}
}

func TestDCCPDataChecksumConcurrent(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			t.Parallel()
			data := make([]byte, 20)
			data[4], data[8], data[12], data[13] = 5, 4, 44, 6
			for j := 0; j < 50; j++ {
				count, err := validateDCCPDataChecksums(data)
				require.NoError(t, err)
				require.Equal(t, 1, count)
			}
		})
	}
}
