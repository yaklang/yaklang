package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestIPVersionRefusalStopsSpeculativeFragmentAndApplicationDisplay(t *testing.T) {
	bad := &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Status: "malformed", ExpertCode: "IPVersionMismatch", Summary: "invalid IPv4 version 6", Session: map[string]any{"Expected IP Version": uint8(4), "Observed IP Version": uint8(6), "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	stale := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Session: map[string]any{"QNAME": "unproved.invalid"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{bad}, {stale, bad}, {bad, stale}} {
		row := packetSummary{Protocol: "Fragment", Source: "192.0.2.1", Destination: "192.0.2.2", SourcePort: 53, DestinationPort: 40000, Transport: "udp", Layers: []string{"Ethernet", "IPv4", "Fragment", "DNS"}, ConnectionProtocol: "DNS", ConnectionStack: []string{"DNS"}, ConnectionEvidence: "stale tuple", StreamID: 7}
		got := presentEvents(events).apply(row)
		require.Equal(t, "IPv4", got.Protocol)
		require.Equal(t, []string{"Ethernet", "IPv4"}, got.ProtocolStack)
		require.Equal(t, "malformed", got.ProtocolStatus)
		require.Equal(t, "invalid network header; transport not admitted", got.ProtocolScope)
		require.Equal(t, "invalid-network-header", got.Visibility)
		require.Equal(t, bad.Summary, got.Info)
		require.Empty(t, got.Source)
		require.Empty(t, got.Destination)
		require.Zero(t, got.SourcePort)
		require.Zero(t, got.DestinationPort)
		require.Empty(t, got.ConnectionProtocol)
		require.Empty(t, got.ConnectionStack)
		require.Zero(t, got.StreamID)
	}
	fields := protocolEventFields([]*pcaputil.ProtocolEvent{bad})
	text := ""
	for _, f := range fields {
		text += f.text + "\n"
	}
	require.Contains(t, text, "Observed IP Version: 6")
	require.Contains(t, text, "Expected IP Version: 4")
}

func TestUnselectedNetworkParentCannotDisplayAConfirmedChild(t *testing.T) {
	parent := &pcaputil.ProtocolEvent{Protocol: "ip", Profile: "network-encapsulation-refusal", Status: "limited", Summary: "Encapsulation profile unavailable; inner identity unconfirmed", Session: map[string]any{"Child Header Identity": "unconfirmed", "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	child := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "decoded", Session: map[string]any{"Negotiated Version": uint16(0x304)}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{parent, child}, {child, parent}} {
		row := presentEvents(events).apply(packetSummary{Protocol: "TLS", Layers: []string{"Ethernet", "IPv4", "EtherIP", "Ethernet", "IPv4", "TCP", "TLS"}, Transport: "tcp", SourcePort: 443, ConnectionProtocol: "TLS", StreamID: 9})
		require.Equal(t, "IPv4", row.Protocol)
		require.Equal(t, []string{"Ethernet", "IPv4"}, row.ProtocolStack)
		require.Equal(t, "limited", row.ProtocolStatus)
		require.Equal(t, "outer network prefix; inner identity unconfirmed", row.ProtocolScope)
		require.Empty(t, row.ConnectionProtocol)
		require.Zero(t, row.StreamID)
		require.Zero(t, row.SourcePort)
	}
}
