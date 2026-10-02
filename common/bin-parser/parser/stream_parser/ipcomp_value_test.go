package stream_parser

import (
	"bytes"
	"compress/flate"
	"testing"

	"github.com/stretchr/testify/require"
)

func ipCompTestDeflate(t *testing.T, payload []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := flate.NewWriter(&compressed, flate.BestSpeed)
	require.NoError(t, err)
	_, err = writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return compressed.Bytes()
}

func TestIPCompDeflateBoundaries(t *testing.T) {
	payload := bytes.Repeat([]byte("protocol sample "), 100)
	wire := ipCompTestDeflate(t, payload)
	decoded, err := decodeIPCompPayload(2, wire)
	require.NoError(t, err)
	require.True(t, decoded.Decoded)
	require.Equal(t, payload, decoded.Bytes)
	for cut := 0; cut < len(wire); cut++ {
		_, err := decodeIPCompPayload(2, wire[:cut])
		require.Errorf(t, err, "accepted incomplete stream at %d/%d", cut, len(wire))
	}
	_, err = decodeIPCompPayload(2, append(bytes.Clone(wire), 0))
	require.ErrorContains(t, err, "trailing bytes")
	_, err = decodeIPCompPayload(2, make([]byte, 8))
	require.ErrorContains(t, err, "invalid DEFLATE")
	_, err = decodeIPCompPayload(2, ipCompTestDeflate(t, make([]byte, (1<<20)+1)))
	require.ErrorContains(t, err, "exceeds limit")

	opaque, err := decodeIPCompPayload(256, wire)
	require.NoError(t, err)
	require.False(t, opaque.Decoded)
	require.Equal(t, wire, opaque.Bytes)
	opaque.Bytes[0] ^= 0xff
	require.NotEqual(t, wire, opaque.Bytes, "metadata must not alias the caller's input")
}
