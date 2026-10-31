package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNetworkRefusalAppliesToBothInspectorsAndStaleChildEvents(t *testing.T) {
	bad := &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Status: "malformed", ExpertCode: "IPVersionMismatch", Summary: "invalid IPv4 version 6", Session: map[string]any{"Expected IP Version": uint8(4), "Observed IP Version": uint8(6), "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	parent := &pcaputil.ProtocolEvent{Protocol: "ip", Profile: "network-encapsulation-refusal", Status: "limited", Summary: "Encapsulation profile unavailable; inner identity unconfirmed", Session: map[string]any{"Child Header Identity": "unconfirmed", "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	truncated := &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Status: "incomplete", ExpertCode: "IPHeaderTruncated", Summary: "IPv4 header incomplete: 4 of 20 fixed bytes available; transport not admitted", Session: map[string]any{"Expected IP Version": uint8(4), "Observed IP Version": uint8(6), "Available Header Bytes": 4, "Required Header Bytes": 20, "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	child := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Session: map[string]any{"QNAME": "unproved.invalid"}}
	wire := udpPacket(t, 53, []byte("unproved.invalid"))
	for _, refusal := range []*pcaputil.ProtocolEvent{bad, parent, truncated} {
		for _, events := range [][]*pcaputil.ProtocolEvent{{refusal}, {refusal, child}, {child, refusal}} {
			raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
			request, err := marshalDecodeRequest(decodeRequest{Number: raw.number, Events: events, Data: raw.data, Length: len(wire), Link: raw.link})
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
			worker, err := decodeWorkerOutput(output.Bytes(), raw)
			require.NoError(t, err)
			for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
				require.Equal(t, "IPv4", got.protocol)
				require.NotEmpty(t, got.hex, "the original frame stays inspectable")
				require.True(t, got.fields[0].hasRange)
				require.Equal(t, len(wire), got.fields[0].end)
				var text strings.Builder
				for _, f := range got.fields {
					text.WriteString(f.text + "\n")
				}
				require.Contains(t, text.String(), refusal.Summary)
				for _, speculative := range []string{"unproved.invalid", "UDP", "DNS", "Fragment", "YAML dissector"} {
					require.NotContains(t, text.String(), speculative)
				}
			}
		}
	}
}

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

func TestTruncatedIPSummaryKeepsOneSpecificNativeReason(t *testing.T) {
	wire := udpPacket(t, 53, []byte("no transport header captured"))[:18]
	event := &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Status: "incomplete", ExpertCode: "IPHeaderTruncated", Summary: "IPv4 header incomplete: 4 of 20 fixed bytes available; transport not admitted", Session: map[string]any{"Expected IP Version": uint8(4), "Observed IP Version": uint8(4), "Available Header Bytes": 4, "Required Header Bytes": 20, "Observed Header Layers": []string{"Ethernet", "IPv4"}}}
	raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: 100, CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: []*pcaputil.ProtocolEvent{event}, presentation: presentEvents([]*pcaputil.ProtocolEvent{event})}
	row := summarize(raw)
	require.Equal(t, event.Summary+" [truncated capture: 18/100 bytes]", row.Info, "the specific reason replaces the generic marker while actual capture loss stays visible")
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Equal(t, "truncated-network-header", row.Visibility)
	require.Equal(t, "network", row.Transport)
	require.Zero(t, row.SourcePort)
	require.Zero(t, row.DestinationPort)
	require.Empty(t, row.ConnectionProtocol)
	require.Zero(t, row.StreamID)
}

func TestUnselectedGREParentSummaryKeepsSpecificReason(t *testing.T) {
	wire := udpPacket(t, 53, []byte("unselected GRE child"))[:40]
	wire[23] = byte(layers.IPProtocolGRE)
	copy(wire[34:], []byte{0x0f, 0x2f, 0x08, 0x00, 0x46, 0x00})
	packet := gopacket.NewPacket(wire, layers.LinkTypeEthernet, gopacket.Default)
	require.NotNil(t, packet.ErrorLayer(), "the stateless child decode has an error to explain")
	event := &pcaputil.ProtocolEvent{Protocol: "ip", Profile: "network-encapsulation-refusal", Status: "limited", Summary: "GRE version 7 not selected; inner identity unconfirmed", Session: map[string]any{"Child Header Identity": "unconfirmed", "Observed Header Layers": []string{"Ethernet", "IPv4"}, "Unselected Carrier": "GRE", "Observed GRE Version": uint8(7)}}
	for _, originalLength := range []int{len(wire), 100} {
		raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: originalLength, CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: []*pcaputil.ProtocolEvent{event}, presentation: presentEvents([]*pcaputil.ProtocolEvent{event})}
		row := summarize(raw)
		want := event.Summary
		if originalLength > len(wire) {
			want += " [truncated capture: 40/100 bytes]"
		}
		require.Equal(t, want, row.Info)
		require.Equal(t, "IPv4", row.Protocol)
		require.Equal(t, []string{"Ethernet", "IPv4"}, row.ProtocolStack)
		require.Zero(t, row.StreamID)
		require.Empty(t, row.ConnectionProtocol)
		raw.events, raw.presentation = nil, packetPresentation{}
		require.Contains(t, summarize(raw).Info, "[incomplete/undecoded]", "an unexplained stateless decode error must remain visible")
	}
}
