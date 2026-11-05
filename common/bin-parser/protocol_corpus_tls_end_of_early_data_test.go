package bin_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestProtocolCorpusTLSEndOfEarlyDataExactLayout(t *testing.T) {
	const rule = "application-layer.tls_end_of_early_data"
	wire := []byte{5, 0, 0, 0}
	n := protocolCorpusRequireBoundedRuleParse(t, wire, rule, "TLSEndOfEarlyData")
	latTestField(t, n, "Handshake Type", "uint8", 0, 8, uint64(5))
	latTestField(t, n, "Handshake Length", "uint32", 8, 32, uint64(0))
	require.Equal(t, wire, NodeToBytes(n))
	_, info, err := stream_parser.DecodeTLSEndOfEarlyDataLayout(wire)
	require.NoError(t, err)
	for _, key := range []string{"Record Authentication Verified", "Early Data Acceptance Validated", "Direction Validated", "Handshake Phase Validated", "Handshake Completion Validated"} {
		require.Equal(t, false, info[key])
	}
	for _, bad := range [][]byte{nil, wire[:1], wire[:2], wire[:3], {5, 0, 0, 1}, {5, 0, 0, 1, 0}, {5, 0, 0, 0, 0}, {14, 0, 0, 0}, {5, 0, 0, 0, 5, 0, 0, 0}} {
		_, _, err := stream_parser.DecodeTLSEndOfEarlyDataLayout(bad)
		require.Error(t, err)
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(bad), rule, "TLSEndOfEarlyData")
		require.Error(t, err)
	}
	_, err = parser.ParseBinary(bytes.NewReader(wire), rule, "TLSEndOfEarlyData")
	require.ErrorContains(t, err, "explicit")
	_, err = parser.GenerateBinary(map[string]any{}, rule, "TLSEndOfEarlyData")
	require.Error(t, err)
}
