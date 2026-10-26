package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3ConnectionLayoutFaultRemainsVisibleAfterAnIndividuallyDecodedFrame(t *testing.T) {
	fault := map[string]any{"Error Name": "H3_FRAME_ERROR", "Error Code": uint64(0x106), "Error Scope": "connection", "First PDU ID": uint64(1)}
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "decoded", Session: map[string]any{"HTTP3 Connection Error": fault}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "connection-violation-observed", row.ProtocolStatus)
	require.Equal(t, "connection protocol violation observed", row.ProtocolScope)
	require.Contains(t, row.ProtocolEvidence, "peer closure not inferred")
	require.Contains(t, row.Info[:45], "H3_FRAME_ERROR")
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{e})
	require.GreaterOrEqual(t, len(fields), 2)
	require.Contains(t, fields[0].text, "H3_FRAME_ERROR")
	require.LessOrEqual(t, len(fields[0].text), 76)
	require.Contains(t, fields[1].text, "do not certify")
	var live packetPresentation
	live.add(e)
	live.add(&pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Status: "decoded"})
	row = live.apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "connection-violation-observed", row.ProtocolStatus, "incremental publication uses the same scope as batch rendering")
	require.Equal(t, 1, strings.Count(row.Info, "H3_FRAME_ERROR · connection"))
	e.Status, e.Error = "malformed", "HTTP3 H3_FRAME_ERROR"
	row = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "malformed", row.ProtocolStatus, "a new connection violation is not normalized to decoded")
}
