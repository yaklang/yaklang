package bin_parser

import (
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
)

func TestRDPUDPCanonicalSelectedEnvelope(t *testing.T) {
	w := make([]byte, 1132)
	binary.BigEndian.PutUint32(w, 0xffffffff)
	binary.BigEndian.PutUint16(w[4:], 64)
	binary.BigEndian.PutUint16(w[6:], 1)
	binary.BigEndian.PutUint32(w[8:], 123)
	binary.BigEndian.PutUint16(w[12:], 1132)
	binary.BigEndian.PutUint16(w[14:], 1132)
	f, err := stream_parser.DecodeRDPUDPWire(w, 1)
	require.NoError(t, err)
	require.Equal(t, uint32(123), f.Fields["Initial Sequence"])
	n := protocolCorpusRequireBoundedRuleParse(t, w, "application-layer.rdpudp", "RDPUDP")
	require.NotNil(t, n)
	structured, err := ParseStructured(w, "application-layer.rdpudp", "RDPUDP")
	require.NoError(t, err)
	require.NotEmpty(t, structured["fields"])
	for cut := 0; cut < len(w); cut++ {
		_, err := stream_parser.DecodeRDPUDPWire(w[:cut], 1)
		require.Error(t, err)
	}
	w[len(w)-1] = 1
	_, err = stream_parser.DecodeRDPUDPWire(w, 1)
	require.Error(t, err)
	w[len(w)-1] = 0
	binary.BigEndian.PutUint16(w[6:], 3)
	_, err = stream_parser.DecodeRDPUDPWire(w, 1)
	require.Error(t, err, "SYN and FIN cannot establish a generation")
	// Selected V2 DATA sequence/channel fields are little endian; first and
	// eighth bytes are permuted on wire, without mutating the input.
	v2 := []byte{0xe0, 4, 0xf0, 9, 0, 100, 0, 22, 3, 3, 0, 0}
	v2[0], v2[7] = v2[7], v2[0]
	f, err = stream_parser.DecodeRDPUDPWire(v2, 0x101)
	require.NoError(t, err)
	require.Equal(t, uint16(100), f.ChannelSequence)
	require.Equal(t, []byte{22, 3, 3, 0, 0}, f.Payload)
	require.Equal(t, byte(22), v2[0])
	protocolCorpusRequireBoundedRuleParse(t, v2, "application-layer.rdpudp", "RDPUDP2")
	_, err = stream_parser.DecodeRDPUDPWire(v2, 1)
	require.Error(t, err)
}
