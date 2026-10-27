package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func bindNakRuleWire(body []byte) []byte {
	wire := make([]byte, 16+len(body))
	wire[0], wire[2], wire[3], wire[4] = 5, 13, 3, 0x10
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	wire[12] = 2
	copy(wire[16:], body)
	return wire
}

func TestDCERPCBindNakBoundedTrailingAlignment(t *testing.T) {
	// Windows TCP/135 returns this eight-byte body for an unbound request.
	// The last three bytes align the complete PDU; they are not more versions.
	for count := 0; count <= 20; count++ {
		body := []byte{4, 0, byte(count)}
		for i := 0; i < count; i++ {
			body = append(body, 5, byte(i))
		}
		for _, padding := range [][]byte{nil, bytes.Repeat([]byte{0xa5}, (4-len(body)%4)%4)} {
			wire := bindNakRuleWire(append(bytes.Clone(body), padding...))
			root, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
			require.NoError(t, err, "count=%d padding=%d", count, len(padding))
			protocolCorpusRequireValue(t, root, "Reject Reason", uint64(4))
			protocolCorpusRequireValue(t, root, "Num Versions", uint64(count))
			if len(padding) > 0 {
				protocolCorpusRequireValue(t, root, "Trailing Padding", padding)
			}
		}
	}
	for _, body := range [][]byte{
		{4, 0}, {4, 0, 1}, {4, 0, 1, 5}, // Missing count/version octets.
		{4, 0, 1, 5, 0, 0}, {4, 0, 1, 5, 0, 0, 0}, // Partial alignment.
		{4, 0, 1, 5, 0, 0, 0, 0, 0}, // Bytes beyond the aligned envelope.
		{4, 0, 21},
	} {
		_, err := parser.ParseBinary(bytes.NewReader(bindNakRuleWire(body)), "application-layer.dcerpc", "DCERPC")
		require.Error(t, err, "invalid body %x", body)
	}
}

func TestDCERPCBindNakPhysicalTruncation(t *testing.T) {
	wire := bindNakRuleWire([]byte{4, 0, 1, 5, 0, 0, 0, 0})
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinary(bytes.NewReader(wire[:cut]), "application-layer.dcerpc", "DCERPC")
		require.Error(t, err, "cut=%d with unchanged declared fragment length", cut)
	}
}
