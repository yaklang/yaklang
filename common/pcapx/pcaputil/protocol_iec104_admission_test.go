package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolIEC104RejectsForeignASDULayout(t *testing.T) {
	// The same APCI/ASDU shape as a live 12010 false positive: 91 APDU bytes,
	// private type 229, 29 sequential objects, but 81 bytes cannot hold their
	// IOA plus equally sized elements. Payload contents are immaterial.
	wire := append([]byte{0x68, 91, 0xca, 0x63, 0x80, 0xfd, 229, 0x9d, 0xe6, 0xdb, 0x54, 0x33}, bytes.Repeat([]byte{'?'}, 1308)...)
	require.Equal(t, ProbeReject, probeIEC104(wire, 64).Verdict)
	events, stats, err := binReplay(t, binTestPcap(t, []tcpStep{{seq: 100, data: string(wire)}}, 12010, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "unrecognized", events[0].Status)
	require.Empty(t, events[0].Protocol, "the port does not establish another protocol either")
	require.Zero(t, stats.Malformed)
	require.Zero(t, stats.Decoded)
}

func TestProtocolIEC104PrivateASDUAfterObservedSession(t *testing.T) {
	s := &binIEC104{}
	_, err := s.consume(0, iec104U(7), 127)
	require.NoError(t, err)
	private := iec104I(0, 0, 229, 3, 0, []byte("opaque-private-value"))
	out, err := s.consume(0, private, 127)
	require.NoError(t, err)
	require.Equal(t, "unsupported-asdu-type", out["ASDU"].(map[string]any)["Semantic Status"])
	require.Equal(t, 229, out["TypeID"])
	for _, wire := range [][]byte{iec104I(0, 0, 1, 3, 0, []byte{1}), iec104I(0, 0, 100, 6, 0, []byte{20}), iec104U(7)} {
		require.Equal(t, ProbeAccept, probeIEC104(wire, 64).Verdict)
	}
}
