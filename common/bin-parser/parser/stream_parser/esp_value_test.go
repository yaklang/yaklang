package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestESPHeaderWireOpaqueScopeAndBounds(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0, 0x45, 0, 0, 20}
	m, err := DecodeESPHeaderWire(w, len(w))
	require.NoError(t, err)
	require.Equal(t, uint32(0x12345678), m["SPI"])
	require.Equal(t, uint32(0), m["Sequence"], "ESN low-word zero does not prove replay/wrap")
	require.Equal(t, w[8:], m["Opaque Body"])
	for _, k := range []string{"Algorithm Context Available", "Authentication Verified", "Extended Sequence Reconstructed"} {
		require.Equal(t, false, m[k])
	}
	require.NotContains(t, m, "Ciphertext")
	require.NotContains(t, m, "Next Header")
	w[8] = 0xff
	require.Equal(t, byte(0x45), m["Opaque Body"].([]byte)[0])
	for cut := 0; cut < 8; cut++ {
		part, err := DecodeESPHeaderWire(w[:cut], 64)
		require.Error(t, err)
		require.Equal(t, false, part["Header Complete"])
		require.NotContains(t, part, "Sequence")
		require.NotContains(t, part, "Opaque Body")
		if cut < 4 {
			require.NotContains(t, part, "SPI")
		} else {
			require.Equal(t, uint32(0x12345678), part["SPI"])
		}
	}
	_, err = DecodeESPHeaderWire(w, len(w)-1)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	_, err = DecodeESPHeaderWire(w, 0)
	require.ErrorIs(t, err, ErrObservedWireBudget)
	bad := bytes.Clone(w)
	binary.BigEndian.PutUint32(bad[:4], 0)
	part, err := DecodeESPHeaderWire(bad, 64)
	require.ErrorContains(t, err, "SPI zero")
	require.Equal(t, uint32(0), part["SPI"])
}

func FuzzESPHeaderWireBounds(f *testing.F) {
	for _, seed := range [][]byte{nil, {0}, {0, 0, 0, 1}, {0, 0, 0, 1, 0, 0, 0, 0}, {1, 2, 3, 4, 0xff, 0xff, 0xff, 0xff, 0xff}} {
		f.Add(seed, uint16(1024))
	}
	f.Fuzz(func(t *testing.T, w []byte, n uint16) {
		m, err := DecodeESPHeaderWire(w, int(n))
		if len(w) > int(n) || n == 0 {
			require.ErrorIs(t, err, ErrObservedWireBudget)
			require.Nil(t, m)
			return
		}
		require.Equal(t, len(w) >= 8, m["Header Complete"])
		if len(w) < 8 {
			require.Error(t, err)
			require.NotContains(t, m, "Sequence")
			require.NotContains(t, m, "Opaque Body")
			return
		}
		require.Equal(t, binary.BigEndian.Uint32(w[:4]), m["SPI"])
		require.Equal(t, binary.BigEndian.Uint32(w[4:8]), m["Sequence"])
		require.Equal(t, w[8:], m["Opaque Body"])
	})
}
