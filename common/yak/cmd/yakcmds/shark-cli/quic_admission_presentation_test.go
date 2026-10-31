package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestQUICDatagramPaddingPreservesHandshakeAndUnprotectedByteScope(t *testing.T) {
	good := &pcaputil.ProtocolEvent{Protocol: "quic", Transport: "udp", FlowID: 1, Status: "decoded", Session: map[string]any{"Authentication Verified": true, "Packet Name": "Initial"}}
	padding := &pcaputil.ProtocolEvent{ID: 2, Protocol: "quic", Transport: "udp", FlowID: 1, Status: "decoded", Completeness: "datagram-padding", Admission: "observed-datagram-padding", Summary: "Datagram padding · offset=1200 · 3 bytes · unprotected UDP bytes", Raw: []byte{0, 0, 0}, Fields: map[string]any{"Datagram Padding": []byte{0, 0, 0}}, Session: map[string]any{"Packet Name": "Datagram padding", "Datagram Offset": 1200, "Datagram Padding Length": uint64(3), "Authentication Verified": false, "Wire Field Ranges": map[string]any{"Datagram Padding": []any{0, 3}}}}
	encoded, err := json.Marshal(padding)
	require.NoError(t, err)
	var ipc pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &ipc))
	for _, e := range []*pcaputil.ProtocolEvent{padding, &ipc} {
		row := presentEvents([]*pcaputil.ProtocolEvent{good, e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
		require.Equal(t, "QUIC", row.Protocol)
		require.Equal(t, "decoded", row.ProtocolStatus)
		require.Equal(t, "mixed packet protection", row.Visibility)
		require.Contains(t, row.Info, "Initial")
		require.Contains(t, row.Info, "unprotected UDP bytes")
		require.Contains(t, row.ProtocolEvidence, "padding is unprotected")
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		found := false
		for _, field := range fields {
			if strings.HasPrefix(field.text, "Datagram Padding:") {
				found = true
				require.True(t, field.hasRange)
				require.Equal(t, bytes.Repeat([]byte{0}, 3), sources[field.source-1].Data[field.start:field.end])
			}
		}
		require.True(t, found)
	}
}

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
	require.Equal(t, tail.Summary, row.Info, "a remainder-only row shows its diagnostic once")
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{good, tail})
	require.NotEmpty(t, fields)
	require.Contains(t, fields[0].text, "QUIC remainder")
	require.LessOrEqual(t, len(fields[0].text), 76, "scope must fit a normal detail pane")
	require.Contains(t, fields[0].text, "authentication not verified")
}
