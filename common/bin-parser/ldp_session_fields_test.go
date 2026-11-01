package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestLDPTCPCanonicalCompleteMessages(t *testing.T) {
	ka := []byte{0, 1, 0, 14, 192, 0, 2, 1, 0, 0, 2, 1, 0, 4, 0, 0, 0, 9}
	init := []byte{0, 1, 0, 32, 192, 0, 2, 1, 0, 0, 2, 0, 0, 22, 0, 0, 0, 1, 5, 0, 0, 14, 0, 1, 0, 90, 0, 0, 0, 0, 192, 0, 2, 2, 0, 0}
	for _, w := range [][]byte{ka, init} {
		m, err := stream_parser.DecodeLDPSessionPDU(w, 128)
		require.NoError(t, err)
		parsed, err := ParseStructured(w, "ldp_session", "LDPSessionFields")
		require.NoError(t, err)
		require.Equal(t, m, parsed["fields"].(map[string]any)["Message"])
		require.NotContains(t, m, "Observed Maximum PDU Length", "stateless entry cannot invent negotiation")
		for cut := 0; cut < len(w); cut++ {
			_, err = ParseStructured(w[:cut], "ldp_session", "LDPSessionFields")
			require.Error(t, err, "cut %d", cut)
		}
	}
	bad := append([]byte(nil), init...)
	bad[25] = 0
	_, err := stream_parser.DecodeLDPSessionPDU(bad, 128)
	require.Error(t, err, "zero KeepAlive proposal is invalid")
	bad = append([]byte(nil), init...)
	binary.BigEndian.PutUint16(bad[20:], 15)
	_, err = stream_parser.DecodeLDPSessionPDU(bad, 128)
	require.Error(t, err, "TLV cannot cross PDU boundary")
}
