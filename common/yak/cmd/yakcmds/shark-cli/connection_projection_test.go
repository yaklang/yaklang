package sharkcli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestConnectionIdentityRetainsPacketDiagnosticOnRedraw(t *testing.T) {
	for _, tc := range []struct{ status, code, reason string }{
		{"malformed", "MalformedMessage", "ssh-plaintext: Ed25519 Signature Bytes requires 64 bytes"},
		{"limited", "ResourceExceeded", "SSH retained field budget exhausted"},
	} {
		e := &pcaputil.ProtocolEvent{Protocol: "ssh", Transport: "tcp", Status: tc.status, ExpertCode: tc.code, Error: tc.code + ": " + tc.reason}
		row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", packetProtocol: "TCP", payloadBytes: 192, Layers: []string{"Loopback", "IPv4", "TCP"}})
		require.Equal(t, tc.status, row.ProtocolStatus)
		require.Equal(t, "protocol parser diagnostic", row.ProtocolEvidence)
		first := applyApplication(row, "SSH", "validated protocol events", 1)
		require.Equal(t, "SSH", first.Protocol)
		require.Equal(t, tc.status, first.ProtocolStatus)
		require.Equal(t, "connection-associated diagnostic", first.ProtocolScope)
		require.Contains(t, first.Info, tc.reason)
		require.NotContains(t, first.Info, "no complete decoded message")
		require.Equal(t, first, applyApplication(first, "SSH", "validated protocol events", 1))
	}
}

func TestConnectionProjectionRetainsPayloadScopeOnRedraw(t *testing.T) {
	row := packetSummary{Protocol: "TCP", Transport: "tcp", packetProtocol: "TCP", payloadBytes: 100,
		Info: "22 → 47368 TCP header Seq=123", Layers: []string{"Ethernet", "IPv6", "Fragment"},
		TransportView: &packetTransportView{Layers: []string{"Ethernet", "IPv6", "GRE", "Ethernet", "IPv6", "TCP"}}}
	first := applyApplication(row, "SSH", "validated protocol events", 17)
	second := applyApplication(first, "SSH", "validated protocol events", 17)
	require.Equal(t, "SSH", first.Protocol)
	require.Equal(t, "connection-associated bytes", first.ProtocolScope)
	require.Contains(t, first.Info, "no complete decoded message in this packet")
	require.Equal(t, first, second, "redrawing must not erase the distinction between this segment and the observed application")
	changed := applyApplication(first, "TLSv1.3", "validated protocol events", 17)
	require.Equal(t, "TLSv1.3", changed.Protocol)
	require.Contains(t, changed.Info, "TLSv1.3 stream bytes")
	require.NotContains(t, changed.Info, "SSH stream bytes")
}

func TestObservedTransportHeaderIPCRejectsInconsistentByteRanges(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		declared int
		wire     []byte
	}{
		{"tcp", 20, make([]byte, 19)},
		{"tcp", 20, make([]byte, 20)}, // data offset does not describe this header
		{"tcp", 64, make([]byte, 64)},
		{"udp", 8, make([]byte, 7)},
		{"other", 8, make([]byte, 8)},
	} {
		req := decodeRequest{TransportView: &packetTransportView{Protocol: tc.protocol, HeaderLength: tc.declared}, TransportHeader: tc.wire}
		wire, err := marshalDecodeRequest(req)
		require.NoError(t, err)
		var out bytes.Buffer
		require.ErrorContains(t, runDecodeWorker(bytes.NewReader(wire), &out), "invalid observed transport header")
		require.Empty(t, out.Bytes(), "inconsistent header offsets cannot become a field/byte response")
	}
}
