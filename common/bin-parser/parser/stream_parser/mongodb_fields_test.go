package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
)

func mongoTestLE32(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}

func mongoTestBSONInt32(name string, v uint32) []byte {
	n := append([]byte{0x10}, append([]byte(name), 0)...)
	n = append(n, mongoTestLE32(v)...)
	n = append(n, 0)
	return append(mongoTestLE32(uint32(4+len(n))), n...)
}

func mongoTestOpMsg(req uint32, sections ...[]byte) []byte {
	body := mongoTestLE32(0)
	for _, s := range sections {
		body = append(body, s...)
	}
	total := uint32(16 + len(body))
	w := append(mongoTestLE32(total), mongoTestLE32(req)...)
	w = append(w, mongoTestLE32(0)...)
	w = append(w, mongoTestLE32(2013)...)
	return append(w, body...)
}

func TestMongoDBFieldsOPMsgAndSnappy(t *testing.T) {
	ping := mongoTestOpMsg(1, append([]byte{0}, mongoTestBSONInt32("ping", 1)...))
	_, info, err := decodeMongoDBFields(ping)
	require.NoError(t, err)
	require.Equal(t, "OP_MSG", info["Opcode Name"])
	require.Equal(t, uint64(1), info["Request ID"])
	require.Equal(t, 1, info["Section Count"])

	inner := ping[16:]
	compBody := append(mongoTestLE32(2013), mongoTestLE32(uint32(len(inner)))...)
	compBody = append(compBody, 1)
	compBody = append(compBody, snappy.Encode(nil, inner)...)
	comp := append(mongoTestLE32(uint32(16+len(compBody))), mongoTestLE32(2)...)
	comp = append(comp, mongoTestLE32(0)...)
	comp = append(comp, mongoTestLE32(2012)...)
	comp = append(comp, compBody...)
	_, info, err = decodeMongoDBFields(comp)
	require.NoError(t, err)
	require.Equal(t, "OP_COMPRESSED", info["Opcode Name"])
	require.Equal(t, "snappy", info["Compressor"])
	require.Equal(t, "OP_MSG", info["Inner Opcode Name"])

	_, _, err = decodeMongoDBFields(ping[:10])
	require.Error(t, err)
	_, _, err = decodeMongoDBFields(append([]byte{}, ping[:12]...))
	require.Error(t, err)
}
