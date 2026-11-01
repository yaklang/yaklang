package sharkcli

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestPPTPAcknowledgmentShowsHeaderIdentityAndSpecificSummary(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolGRE, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	gre := &layers.GRE{Version: 1, Protocol: layers.EthernetTypePPP, KeyPresent: true, Key: 42, AckPresent: true, Ack: 7}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gre))
	wire := bytes.Clone(b.Bytes())
	event := &pcaputil.ProtocolEvent{Protocol: "ip", Transport: "network", Profile: "pptp-acknowledgment", Status: "limited", Completeness: "headers", ExpertCode: "PPTPAcknowledgment", Summary: "PPTP acknowledgment call=42 ack=7; no PPP payload", Session: map[string]any{"Observed GRE Version": uint8(1), "PPTP Call ID": uint16(42), "PPTP Payload Length": uint16(0), "PPTP Acknowledgment Number": uint32(7), "Child Header Identity": "not present", "PPTP Data Path Scope": "one direction; call-control association unavailable"}}
	child := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Summary: "unproved child", Session: map[string]any{"QNAME": "unproved.invalid"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{event}, {event, child}, {child, event}} {
		raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
		row := summarize(raw)
		require.Equal(t, "PPTP", row.Protocol)
		require.Equal(t, event.Summary, row.Info)
		require.Equal(t, []string{"Ethernet", "IPv4", "GRE", "PPTP"}, row.ProtocolStack)
		require.Equal(t, "limited", row.ProtocolStatus)
		require.Equal(t, "acknowledgment-header", row.Visibility)
		require.Contains(t, row.ProtocolScope, "call-control association unavailable")
		require.Empty(t, row.ConnectionProtocol)
		require.Zero(t, row.StreamID)
		request, err := marshalDecodeRequest(decodeRequest{Number: 1, Events: events, Data: wire, Length: len(wire), Link: raw.link})
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
		worker, err := decodeWorkerOutput(output.Bytes(), raw)
		require.NoError(t, err)
		for _, result := range []packetDetail{detail(raw), quickDetail(raw), worker} {
			require.Equal(t, "PPTP", result.protocol)
			var text strings.Builder
			for _, field := range result.fields {
				text.WriteString(field.text + "\n")
			}
			require.Contains(t, text.String(), event.Summary)
			require.Contains(t, text.String(), "PPTP Call ID: 42")
			require.Contains(t, text.String(), "PPTP Acknowledgment Number: 7")
			for _, speculative := range []string{"unproved.invalid", "DNS", "UDP", "YAML dissector"} {
				require.NotContains(t, text.String(), speculative)
			}
		}
	}
}
