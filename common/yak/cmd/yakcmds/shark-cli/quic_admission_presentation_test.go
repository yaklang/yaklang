package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestQUICDatagramTailIsPartialAndNeverPresentedAsAuthenticated(t *testing.T) {
	good := &pcaputil.ProtocolEvent{Protocol: "quic", Transport: "udp", FlowID: 1, Status: "decoded", Session: map[string]any{"Authentication Verified": true, "Packet Name": "Initial"}}
	tail := &pcaputil.ProtocolEvent{Protocol: "quic", Transport: "udp", FlowID: 1, Length: 3, Status: "malformed", ExpertCode: "MalformedMessage", Error: "MalformedMessage: invalid coalesced QUIC header", Admission: "unparsed-datagram-remainder", Summary: "Unparsed QUIC datagram remainder · offset=1200 · 3 bytes · authentication not verified"}
	row := presentEvents([]*pcaputil.ProtocolEvent{good, tail}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "QUIC", row.Protocol)
	require.Equal(t, "partial", row.ProtocolStatus)
	require.Equal(t, "mixed datagram outcomes", row.ProtocolScope)
	require.Contains(t, row.ProtocolEvidence, "remainder is not verified")
	require.Contains(t, row.Info, "offset=1200")
	require.Contains(t, row.Info, "authentication not verified")
	require.Contains(t, row.Info[:40], "Unverified QUIC remainder")
	row = presentEvents([]*pcaputil.ProtocolEvent{tail}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "QUIC", row.Protocol)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.NotEqual(t, "authenticated application bytes", row.ProtocolScope)
	require.Contains(t, row.Info, "3 bytes")
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{good, tail})
	require.NotEmpty(t, fields)
	require.Contains(t, fields[0].text, "QUIC remainder")
	require.LessOrEqual(t, len(fields[0].text), 76, "scope must fit a normal detail pane")
	require.Contains(t, fields[0].text, "authentication not verified")
}
