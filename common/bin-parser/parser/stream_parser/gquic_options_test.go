package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGQUIC35TypedOptions(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		size int
		typ  string
	}{{"SMHL", 4, "uint32"}, {"CTIM", 8, "uint64"}, {"XLCT", 8, "uint64"}, {"NONP", 32, "raw"}, {"NONC", 32, ""}} {
		for size := 0; size <= tc.size+1; size++ {
			f, err := gquic35TagValue(bytes.Repeat([]byte{1}, size), tc.tag, 0, size, 3)
			if size != tc.size {
				require.Error(t, err)
				continue
			}
			require.NoError(t, err)
			require.Equal(t, tc.typ, f.Type)
			require.Equal(t, true, f.Info["Value Layout Decoded"])
			require.Equal(t, false, f.Info["Value Semantics Validated"])
			if tc.tag == "NONC" {
				require.Len(t, f.Children, 3)
				require.Equal(t, "big", f.Children[0].Endian)
				require.Equal(t, 4, f.Children[0].End)
				require.Equal(t, 12, f.Children[1].End)
				require.Equal(t, 32, f.Children[2].End)
			}
		}
	}
	for _, tag := range []string{"CCS\x00", "CCRT"} {
		for size := 0; size <= 1452; size++ {
			f, err := gquic35TagValue(make([]byte, size), tag, 0, size, 0)
			if size%8 != 0 {
				require.Error(t, err)
				continue
			}
			require.NoError(t, err)
			require.Equal(t, size/8, f.Info["Hash Count"])
			require.Len(t, f.Children, size/8)
			for i, c := range f.Children {
				require.Equal(t, "uint64", c.Type)
				require.Equal(t, i*8, c.Start)
				require.Equal(t, (i+1)*8, c.End)
			}
			if size == 0 {
				require.Equal(t, "raw", f.Type)
			}
		}
	}
}

// Bridge-only control; the public oracle independently builds and checks hashes.
func gquic35TestTypedBridgePacket(invalid bool) []byte {
	header := []byte{0, 1}
	chlo := make([]byte, 1024)
	copy(chlo, "CHLO")
	binary.LittleEndian.PutUint16(chlo[4:], 2)
	copy(chlo[8:], "PAD\x00")
	binary.LittleEndian.PutUint32(chlo[12:], 968)
	copy(chlo[16:], "NONC")
	binary.LittleEndian.PutUint32(chlo[20:], 1000)
	copy(chlo[992:], []byte{1, 2, 3, 4})
	if invalid {
		binary.LittleEndian.PutUint32(chlo[20:], 999)
	}
	body := append([]byte{0xa0, 1, 0, 4}, chlo...)
	hash := gquic35NullHash(header, body)
	return append(append(header, hash[:]...), body...)
}
