package sharkcli

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func rsvpPresentationFrame(t *testing.T, wire []byte, v6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 64, Protocol: 46, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: 46, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(wire)))
	return b.Bytes()
}

func TestRSVPIntservProgressiveParameterDecimalAndCapturedPrefix(t *testing.T) {
	w := make([]byte, 44)
	copy(w, []byte{0x10, 1, 0, 0, 64, 0, 0, 44, 0, 36, 12, 2, 0, 0, 0, 7, 1, 0, 0, 6, 127, 0, 0, 5})
	for _, offset := range []int{24, 28, 32} {
		binary.BigEndian.PutUint32(w[offset:], 0x45bb8000)
	}
	binary.BigEndian.PutUint32(w[36:], 0)
	binary.BigEndian.PutUint32(w[40:], 1500)
	for _, v6 := range []bool{false, true} {
		raw := replayPIMPresentationPacket(t, rsvpPresentationFrame(t, w, v6))
		require.Len(t, raw.events, 1)
		raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
		budget, ok := rsvpObservedBudget(raw.events[0].Session)
		require.True(t, ok)
		require.Equal(t, 3, budget, "object, service and parameter consume the same shared budget")
		for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
			text := detailLines(d.fields)
			require.Contains(t, text, "Captured network prefix")
			require.Contains(t, text, "Intserv Header")
			require.Contains(t, text, "Global Sender Traffic Specification")
			require.Contains(t, text, "Token Bucket TSpec")
			require.Contains(t, text, "minimum-policed-unit-must-be-positive")
			require.NotContains(t, text, "<nil>")
			require.NotContains(t, text, "YAML dissector incomplete")
			var offsets []int
			for _, f := range d.fields {
				if f.text == "Value: 6000" {
					require.True(t, f.hasRange)
					require.Equal(t, 1, f.source)
					require.Equal(t, 4, f.end-f.start)
					require.Equal(t, uint32(0x45bb8000), binary.BigEndian.Uint32(d.sources[0].Data[f.start:f.end]))
					offsets = append(offsets, f.start)
				}
			}
			require.Equal(t, []int{24, 28, 32}, offsets)
		}
	}
}
func TestRSVPProgressiveNamesSourcesAndAdvertisedSelectors(t *testing.T) {
	w := []byte{0x10, 1, 0, 0, 64, 0, 0, 36, 0, 12, 1, 1, 203, 0, 113, 9, 17, 0, 0, 53, 0, 8, 5, 1, 0, 0, 0x75, 0x30, 0, 8, 5, 1, 0, 0, 0x75, 0x30}
	for _, v6 := range []bool{false, true} {
		raw := replayPIMPresentationPacket(t, rsvpPresentationFrame(t, w, v6))
		require.Len(t, raw.events, 1)
		raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
		row := summarize(raw)
		require.Equal(t, "RSVP", row.Protocol)
		require.NotEqual(t, "udp", strings.ToLower(row.Transport))
		require.NotContains(t, row.ProtocolStack, "UDP")
		require.Contains(t, row.Info, "Path")
		for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
			text := detailLines(d.fields)
			require.Contains(t, text, "Captured network prefix")
			require.Contains(t, text, "Message Type: 1 (Path)")
			require.Contains(t, text, "Class: 1 (SESSION)")
			require.Contains(t, text, "Class: 5 (TIME_VALUES)")
			require.Contains(t, text, "Destination Address: 203.0.113.9")
			require.NotContains(t, text, "Wire Field Ranges")
			require.NotContains(t, text, "YAML dissector incomplete")
			require.NotContains(t, text, "<nil>")
			var offsets []int
			for _, f := range d.fields {
				if f.text == "Refresh Period Milliseconds: 30000" {
					require.True(t, f.hasRange)
					require.Equal(t, 1, f.source)
					require.Equal(t, uint32(30000), binary.BigEndian.Uint32(d.sources[0].Data[f.start:f.end]))
					offsets = append(offsets, f.start)
				}
			}
			require.Equal(t, []int{24, 32}, offsets)
		}
	}
}
func TestRSVPMalformedBodyKeepsCapturedPrefixWithoutInventedFields(t *testing.T) {
	w := []byte{0x10, 1, 0, 0, 64, 0, 0, 16, 0, 12, 1, 1, 203, 0, 113, 9}
	raw := replayPIMPresentationPacket(t, rsvpPresentationFrame(t, w, false))
	require.Len(t, raw.events, 1)
	raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
	for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(d.fields)
		require.Contains(t, text, "Captured network prefix")
		require.Contains(t, text, "Destination Address: 203.0.113.9")
		require.NotContains(t, text, "Protocol ID: 0")
		require.NotContains(t, text, "Destination Port: 0")
		require.NotContains(t, text, "YAML dissector incomplete")
	}
}

func rsvpPresentationCapture(t *testing.T, wire []byte, originalLength int) *capturedPacket {
	t.Helper()
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), CaptureLength: len(wire), Length: originalLength}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	return &capturedPacket{number: 1, reference: events[0].SourceBytes.PacketRefs[0], data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
}

func TestRSVPDiagnosticRowsKeepProgressiveIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		wire         []byte
	}{
		{"common-prefix-complete-parent", "malformed", []byte{0x10}},
		{"wrong-object-length", "malformed", []byte{0x10, 1, 0, 0, 64, 0, 0, 16, 0, 12, 1, 1, 203, 0, 113, 9}},
		{"unselected-Bundle", "context-required", []byte{0x10, 12, 0, 0, 64, 0, 0, 8}},
		{"unselected-version", "context-required", []byte{0x20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := rsvpPresentationFrame(t, tc.wire, false)
			raw := rsvpPresentationCapture(t, frame, len(frame))
			row := summarize(raw)
			require.Equal(t, "RSVP", row.Protocol)
			require.Equal(t, tc.status, row.ProtocolStatus)
			require.Equal(t, 1, countString(row.ProtocolStack, "RSVP"))
			require.Contains(t, row.ProtocolEvidence, "declared IP next-header46")
			require.Contains(t, row.Info, tc.status)
			require.Empty(t, row.ConnectionProtocol)
			require.Zero(t, row.StreamID)
		})
	}
	full := rsvpPresentationFrame(t, []byte{0x10, 1, 0, 0, 64, 0, 0, 28, 0, 12, 1, 1, 203, 0, 113, 9, 17, 0, 0, 53, 0, 8, 5, 1, 0, 0, 0x75, 0x30}, false)
	raw := rsvpPresentationCapture(t, bytes.Clone(full[:50]), len(full))
	row := summarize(raw)
	require.Equal(t, "RSVP", row.Protocol)
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Equal(t, []string{"Ethernet", "IPv4"}, row.Layers)
	require.Equal(t, []string{"Ethernet", "IPv4", "RSVP"}, row.ProtocolStack)
	require.Contains(t, row.Info, "incomplete")
	require.NotContains(t, row.ProtocolStack, "UDP")
	require.Zero(t, row.StreamID)
	for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(d.fields)
		require.Contains(t, text, "Captured network prefix")
		require.Contains(t, text, "Destination Address: 203.0.113.9")
		require.NotContains(t, text, "Destination Port: 0")
	}
	bad := bytes.Clone(full)
	binary.BigEndian.PutUint16(bad[16:18], 0)
	refused := summarize(rsvpPresentationCapture(t, bad, len(bad)))
	require.NotEqual(t, "RSVP", refused.Protocol)
	require.NotContains(t, refused.ProtocolStack, "RSVP")
}
