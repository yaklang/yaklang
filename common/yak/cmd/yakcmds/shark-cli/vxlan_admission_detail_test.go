package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func assertVXLANFallbackHasNoChild(t *testing.T, raw *capturedPacket, reason string) {
	t.Helper()
	row := summarize(raw)
	require.Equal(t, "IPv4", row.Protocol)
	require.Contains(t, row.Info, reason)
	require.Equal(t, "limited", row.ProtocolStatus)
	require.Zero(t, row.StreamID)
	require.Zero(t, row.TCPFlowID)
	require.Nil(t, row.MessageDomain)
	require.Empty(t, row.ConnectionProtocol)
	request, err := marshalDecodeRequest(packetDecodeRequest(raw))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
		require.Equal(t, row.Protocol, got.protocol)
		require.Empty(t, got.sources, "all remaining fields refer to the captured frame")
		require.True(t, got.fields[0].hasRange)
		require.Equal(t, len(raw.data), got.fields[0].end)
		var text strings.Builder
		for _, f := range got.fields {
			text.WriteString(f.text + "\n")
			if f.hasRange {
				require.Zero(t, f.source)
				require.GreaterOrEqual(t, f.start, 0)
				require.Greater(t, f.end, f.start)
				require.LessOrEqual(t, f.end, len(raw.data))
			}
		}
		require.Contains(t, text.String(), reason)
		for _, child := range []string{"DNS", "example.com", "Captured VXLAN header", "Inner", "YAML dissector"} {
			require.NotContains(t, text.String(), child)
		}
	}
}

func TestVXLANInspectorWithoutNativeEventsCannotReviveRejectedChild(t *testing.T) {
	for _, link := range []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeNull} {
		t.Run(link.String(), func(t *testing.T) {
			raw := vxlanHeaderDetailPacket(t, link, 0)
			raw.events, raw.presentation = nil, packetPresentation{}
			assertVXLANFallbackHasNoChild(t, raw, "I bit is clear")
		})
	}
}

func TestVXLANHistoryEvictionAndIPCMarkersKeepTheAdmissionBarrier(t *testing.T) {
	raw := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 0)
	ref := pcaputil.PacketReference{Number: raw.number}
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	event := *raw.events[0]
	event.ID, event.SourceBytes.PacketRefs = 1, []pcaputil.PacketReference{ref}
	history.OnEvent(&event)
	history.OnEvent(&pcaputil.ProtocolEvent{ID: 2, Protocol: "dns", Status: "decoded", SourceBytes: pcaputil.ByteSource{PacketRefs: []pcaputil.PacketReference{{Number: 2}}}})
	raw.events, raw.history, raw.reference, raw.eventIDs, raw.physicalRecord = nil, history, ref, []uint64{1}, true
	raw.presentation = packetPresentation{}
	markers := raw.protocolEvents()
	require.NotEmpty(t, markers)
	for _, e := range markers {
		require.Equal(t, string(pcaputil.ErrResourceExceeded), e.ExpertCode)
	}
	assertVXLANFallbackHasNoChild(t, raw, "I bit is clear")
	// This is the actual IPC reduction path, rather than a hand-authored
	// response label. The oversized detail becomes the public limited marker.
	raw.history = nil
	event.Error = strings.Repeat("x", decoderInputLimit)
	raw.events = []*pcaputil.ProtocolEvent{&event}
	request, err := marshalDecodeRequest(packetDecodeRequest(raw))
	require.NoError(t, err)
	require.Contains(t, string(request), "IPC byte/type projection limit")
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	require.Equal(t, "IPv4", worker.protocol)
	for _, f := range worker.fields {
		require.NotContains(t, f.text, "example.com")
		require.NotEqual(t, "DNS", f.text)
	}
}

func TestVXLANInspectorFallbackValidatesAllFixedHeaderPrefixesAndCarrierBounds(t *testing.T) {
	for size := 0; size < 8; size++ {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			wire := udpPacket(t, 4789, []byte{8, 0, 0, 0, 0, 0, 42, 0}[:size])
			raw := &capturedPacket{number: 1, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}}
			assertVXLANFallbackHasNoChild(t, raw, "incomplete fixed header")
		})
	}
	for size := 0; size < 14; size++ {
		wire := udpPacket(t, 4789, append([]byte{8, 0, 0, 0, 0, 0, 42, 0}, make([]byte, size)...))
		raw := &capturedPacket{number: 1, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}}
		assertVXLANFallbackHasNoChild(t, raw, "Ethernet child prefix incomplete")
	}
	raw := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 8)
	raw.events, raw.presentation = nil, packetPresentation{}
	binary.BigEndian.PutUint16(raw.data[38:40], 65535)
	assertVXLANFallbackHasNoChild(t, raw, "UDP carrier bounds unavailable")
}

func TestVXLANInspectorFallbackDoesNotRejectReservedBitsOrSourceOnlyPorts(t *testing.T) {
	raw := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 0xff)
	raw.events, raw.presentation = nil, packetPresentation{}
	require.Nil(t, inspectionVXLANBoundary(gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.Default)))
	require.Equal(t, "DNS", summarize(raw).Protocol)
	// An actual non-tunnel destination owns this datagram, even with source
	// port 4789 and a VXLAN-looking payload with an invalid I bit.
	binary.BigEndian.PutUint16(raw.data[34:36], 4789)
	binary.BigEndian.PutUint16(raw.data[36:38], 55001)
	raw.data[42] = 0
	require.Nil(t, inspectionVXLANBoundary(gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.Default)))
	require.Equal(t, "UDP", summarize(raw).Protocol)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw)} {
		for _, f := range got.fields {
			require.NotContains(t, f.text, "VXLAN header:")
			require.NotContains(t, f.text, "Captured VXLAN header")
			require.NotEqual(t, "DNS", f.text)
		}
	}
}

func TestVXLANInspectorNestedRefusalPreservesAdmittedOuterPathAndInnerEndpoints(t *testing.T) {
	inner := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 0)
	payload := append([]byte{8, 0, 0, 0, 0, 0, 77, 0}, inner.data...)
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	udp := &layers.UDP{SrcPort: 55000, DstPort: 4789}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	wire := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(wire, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(payload)))
	raw := &capturedPacket{number: 1, data: wire.Bytes(), link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(wire.Bytes()), CaptureLength: len(wire.Bytes())}}
	row := summarize(raw)
	require.Equal(t, "IPv4", row.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "VXLAN", "Ethernet", "IPv4"}, row.ProtocolStack)
	require.Equal(t, "192.0.2.1", row.Source)
	require.Equal(t, "192.0.2.2", row.Destination)
	require.Equal(t, "198.51.100.1", row.PhysicalSource)
	require.Equal(t, "198.51.100.2", row.PhysicalDestination)
	request, err := marshalDecodeRequest(packetDecodeRequest(raw))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
		require.Equal(t, row.Protocol, got.protocol)
		found := false
		for _, f := range got.fields {
			require.NotContains(t, f.text, "DNS")
			require.NotContains(t, f.text, "example.com")
			if f.text == "Admitted outer VNI: 77" {
				found = true
				require.True(t, f.hasRange)
				require.Zero(t, f.source)
				require.Equal(t, 46, f.start)
				require.Equal(t, 49, f.end)
			}
		}
		require.True(t, found, "the outer admitted tunnel remains inspectable")
	}
}
