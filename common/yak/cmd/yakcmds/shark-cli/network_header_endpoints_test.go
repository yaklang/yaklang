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

func endpointVXLANWire(t *testing.T, source, destination string, flags byte, vni byte, inner []byte) []byte {
	t.Helper()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP(source).To4(), DstIP: net.ParseIP(destination).To4()}
	udp := &layers.UDP{SrcPort: 55000, DstPort: 4789}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(append([]byte{flags, 0, 0, 0, 0, 0, vni, 0}, inner...))))
	return buffer.Bytes()
}

func TestObservedOuterTransportDoesNotPublishSpeculativeGTPPhysicalEndpoints(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		outer := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("1.2.3.4"), DstIP: net.ParseIP("4.3.2.1")}
		outerUDP := &layers.UDP{SrcPort: 9999, DstPort: 2152}
		require.NoError(t, outerUDP.SetNetworkLayerForChecksum(outer))
		innerUDP := &layers.UDP{SrcPort: 1234, DstPort: 5678}
		var inner gopacket.SerializableLayer
		if ipv6 {
			ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
			require.NoError(t, innerUDP.SetNetworkLayerForChecksum(ip))
			inner = ip
		} else {
			ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("203.0.113.1"), DstIP: net.ParseIP("203.0.113.2")}
			require.NoError(t, innerUDP.SetNetworkLayerForChecksum(ip))
			inner = ip
		}
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, outer, outerUDP, &layers.GTPv1U{Version: 1, ProtocolType: 1, MessageType: 255, TEID: 7}, inner, innerUDP, gopacket.Payload([]byte("opaque"))))
		wire := buffer.Bytes()
		decoded := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		require.Nil(t, decoded.ErrorLayer())
		network, _ := pcaputil.CaptureTransport(decoded)
		require.NotEqual(t, "1.2.3.4", network.NetworkFlow().Src().String(), "control must exercise an independently decoded inner network")
		view := &packetTransportView{Protocol: "udp", Source: "1.2.3.4", Destination: "4.3.2.1", SourcePort: 9999, DestinationPort: 2152, Header: bytes.Clone(wire[34:42]), HeaderLength: 8, PayloadLength: len(wire) - 42, Layers: []string{"Ethernet", "IPv4", "UDP"}, LayersComplete: true, Info: "observed outer UDP header"}
		raw := &capturedPacket{number: 1, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, transportView: view}
		row := summarize(raw)
		require.Equal(t, "1.2.3.4", row.Source)
		require.Equal(t, "4.3.2.1", row.Destination)
		require.Empty(t, row.PhysicalSource, "identical real physical endpoints are omitted, not replaced with speculative children")
		require.Empty(t, row.PhysicalDestination)
		require.Equal(t, "observed transport header", row.ProtocolScope)
		require.Equal(t, view.Layers, row.ProtocolStack)
		require.Contains(t, row.Layers, "GTPv1U", "decoder inventory is retained separately from selected protocol scope")
		require.Zero(t, row.StreamID)
		require.Empty(t, row.ConnectionProtocol)
		fast := quickDetail(raw)
		for _, f := range fast.fields {
			require.NotContains(t, f.text, "GTP")
			require.NotContains(t, f.text, "203.0.113.")
			require.NotContains(t, f.text, "2001:db8:")
		}
		selected := inspectionPacketLayers(decoded, view)
		require.Len(t, selected, 3)
		require.Equal(t, layers.LayerTypeUDP, selected[2].LayerType())
		require.Greater(t, len(decoded.Layers()), len(selected), "the control retains its raw decoder inventory")
	}
}

func nativeEndpointRefusal(t *testing.T, wire []byte) *capturedPacket {
	return nativeEndpointRefusalOnLink(t, wire, layers.LinkTypeEthernet)
}

func nativeEndpointRefusalOnLink(t *testing.T, wire []byte, link layers.LinkType) *capturedPacket {
	t.Helper()
	ci := gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire), Timestamp: time.Unix(1, 0)}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, link))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	err := pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) }))
	require.ErrorContains(t, err, "I bit is clear")
	require.Len(t, events, 1)
	require.Equal(t, "VXLANHeaderAdmission", events[0].ExpertCode)
	require.Empty(t, events[0].Source, "native parent diagnostics do not invent transport endpoints")
	return &capturedPacket{number: 1, data: wire, link: link, ci: ci, events: events, presentation: presentEvents(events)}
}

func assertCapturedEndpointFields(t *testing.T, raw *capturedPacket, source, destination string, sourceRange, destRange [2]int) {
	t.Helper()
	request, err := marshalDecodeRequest(packetDecodeRequest(raw))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
		foundSource, foundDestination := false, false
		for _, f := range got.fields {
			require.NotEqual(t, "DNS", f.text)
			require.NotContains(t, f.text, "example.com")
			if f.text == "Source: "+source || f.text == "Destination: "+destination {
				span := sourceRange
				if strings.HasPrefix(f.text, "Destination:") {
					span, foundDestination = destRange, true
				} else {
					foundSource = true
				}
				require.True(t, f.hasRange)
				require.Zero(t, f.source)
				require.Equal(t, span[0], f.start)
				require.Equal(t, span[1], f.end)
			}
		}
		require.True(t, foundSource)
		require.True(t, foundDestination)
	}
}

func TestNativeParentRefusalKeepsConfirmedOuterEndpointsAcrossInspectors(t *testing.T) {
	inner := dnsPacket(t)
	wire := endpointVXLANWire(t, "198.51.100.1", "198.51.100.2", 0, 42, inner)
	raw := nativeEndpointRefusal(t, wire)
	row := summarize(raw)
	require.Equal(t, "IPv4", row.Protocol)
	require.Equal(t, "198.51.100.1", row.Source)
	require.Equal(t, "198.51.100.2", row.Destination)
	require.Equal(t, row.Source, row.PhysicalSource)
	require.Equal(t, row.Destination, row.PhysicalDestination)
	require.Zero(t, row.SourcePort)
	require.Zero(t, row.DestinationPort)
	require.Empty(t, row.ConnectionProtocol)
	require.Zero(t, row.TCPFlowID)
	require.Zero(t, row.StreamID)
	assertCapturedEndpointFields(t, raw, row.Source, row.Destination, [2]int{26, 30}, [2]int{30, 34})
}

func TestNativeParentEndpointRangesUseRecordedRawAndLoopbackLinkOffsets(t *testing.T) {
	ethernet := endpointVXLANWire(t, "198.51.100.1", "198.51.100.2", 0, 42, dnsPacket(t))
	for _, link := range []layers.LinkType{layers.LinkTypeNull, layers.LinkTypeRaw, layers.LinkTypeIPv4} {
		wire, offset := bytes.Clone(ethernet[14:]), 0
		if link == layers.LinkTypeNull {
			wire, offset = append([]byte{2, 0, 0, 0}, wire...), 4
		}
		raw := nativeEndpointRefusalOnLink(t, wire, link)
		row := summarize(raw)
		require.Equal(t, "198.51.100.1", row.Source)
		require.Equal(t, row.Source, row.PhysicalSource)
		assertCapturedEndpointFields(t, raw, row.Source, row.Destination, [2]int{offset + 12, offset + 16}, [2]int{offset + 16, offset + 20})
	}
}

func TestNativeNestedParentEndpointScopeNeverUsesAnUnadmittedDeepChild(t *testing.T) {
	inner := endpointVXLANWire(t, "198.51.100.1", "198.51.100.2", 0, 42, dnsPacket(t))
	wire := endpointVXLANWire(t, "203.0.113.1", "203.0.113.2", 8, 77, inner)
	raw := nativeEndpointRefusal(t, wire)
	row := summarize(raw)
	require.Equal(t, []string{"Ethernet", "IPv4"}, row.ProtocolStack, "native scope promises the outer network prefix")
	require.Equal(t, "203.0.113.1", row.Source)
	require.Equal(t, "203.0.113.2", row.Destination)
	require.Equal(t, row.Source, row.PhysicalSource)
	require.Equal(t, row.Destination, row.PhysicalDestination)
	assertCapturedEndpointFields(t, raw, row.Source, row.Destination, [2]int{26, 30}, [2]int{30, 34})
	// If native details are unavailable, the independent stateless barrier
	// can still prove the valid outer VXLAN before the inner rejected header.
	raw.events, raw.presentation = nil, packetPresentation{}
	row = summarize(raw)
	require.Equal(t, "198.51.100.1", row.Source)
	require.Equal(t, "203.0.113.1", row.PhysicalSource)
}

func TestCapturedNetworkEndpointProjectionRequiresCompleteCorrectIPHeaders(t *testing.T) {
	for _, version := range []int{4, 6} {
		size, source, destination := 20, "198.51.100.1", "198.51.100.2"
		header := make([]byte, 40)
		eth := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}
		if version == 4 {
			header[0], header[8], header[9] = 0x45, 64, 59
			binary.BigEndian.PutUint16(header[2:4], 20)
			copy(header[12:16], net.ParseIP(source).To4())
			copy(header[16:20], net.ParseIP(destination).To4())
		} else {
			size, source, destination = 40, "2001:db8::1", "2001:db8::2"
			eth[12], eth[13] = 0x86, 0xdd
			header[0], header[6], header[7] = 0x60, 59, 64
			copy(header[8:24], net.ParseIP(source).To16())
			copy(header[24:40], net.ParseIP(destination).To16())
		}
		name := "IPv4"
		if version == 6 {
			name = "IPv6"
		}
		for available := 0; available <= size; available++ {
			wire := append(bytes.Clone(eth), header[:available]...)
			packet := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			found := capturedNetworkPrefix(packet, wire, []string{"Ethernet", name})
			if available < size {
				require.Empty(t, found, "a partial fixed header cannot claim a complete IP endpoint pair")
			} else {
				require.Len(t, found, 1)
				require.Equal(t, source, found[0].source)
				require.Equal(t, destination, found[0].destination)
			}
		}
		wire := append(bytes.Clone(eth), header[:size]...)
		wire[14] ^= 0x10
		packet := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		require.Empty(t, capturedNetworkPrefix(packet, wire, []string{"Ethernet", name}), "a declared carrier with a wrong version cannot borrow address-shaped bytes")
	}
}

func TestCapturedNetworkEndpointsCannotCrossRejectedVXLANOrBorrowEnvelopeTail(t *testing.T) {
	inner := endpointVXLANWire(t, "198.51.100.1", "198.51.100.2", 8, 42, dnsPacket(t))
	full := endpointVXLANWire(t, "203.0.113.1", "203.0.113.2", 8, 77, inner)
	names := []string{"Ethernet", "IPv4", "UDP", "VXLAN", "Ethernet", "IPv4"}
	packet := gopacket.NewPacket(full, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	confirmed := capturedNetworkPrefix(packet, full, names)
	require.Len(t, confirmed, 2)
	require.Equal(t, "198.51.100.1", confirmed[1].source)
	for _, mutate := range []func([]byte){
		func(w []byte) { w[42] = 0 },
		func(w []byte) { binary.BigEndian.PutUint16(w[38:40], 65535) },
		func(w []byte) { binary.BigEndian.PutUint16(w[16:18], 20) },
		func(w []byte) {
			binary.BigEndian.PutUint16(w[34:36], 4789)
			binary.BigEndian.PutUint16(w[36:38], 55001)
		},
	} {
		wire := bytes.Clone(full)
		mutate(wire)
		p := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		headers := capturedNetworkPrefix(p, wire, names)
		require.Len(t, headers, 1)
		require.Equal(t, "203.0.113.1", headers[0].source)
	}
}

func TestCapturedNetworkEndpointProjectionStopsAtICMPQuotation(t *testing.T) {
	quoted := endpointVXLANWire(t, "198.51.100.1", "198.51.100.2", 8, 42, dnsPacket(t))[14:]
	wire := udpPacket(t, 4789, append(make([]byte, 8), quoted...))
	wire[23], wire[34] = 1, 3 // Authored ICMP destination-unreachable quotation.
	packet := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	headers := capturedNetworkPrefix(packet, wire, []string{"Ethernet", "IPv4", "ICMPv4", "IPv4"})
	require.Len(t, headers, 1)
	require.Equal(t, "192.0.2.1", headers[0].source)
}

func TestCapturedNetworkEndpointProjectionKeepsIPv4OptionsInsideTheirEnvelope(t *testing.T) {
	base := udpPacket(t, 4789, nil)
	wire := bytes.Clone(base[:34])
	wire = append(wire, 0, 0, 0, 0)
	wire[14], wire[23] = 0x46, 59
	binary.BigEndian.PutUint16(wire[16:18], 24)
	p := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	headers := capturedNetworkPrefix(p, wire, []string{"Ethernet", "IPv4"})
	require.Len(t, headers, 1)
	require.Equal(t, 38, headers[0].end)
	require.Equal(t, "192.0.2.1", headers[0].source)
	wire[14] = 0x44
	p = gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.Empty(t, capturedNetworkPrefix(p, wire, []string{"Ethernet", "IPv4"}), "an undersized IHL must not expose its address-shaped tail")
	row := packetSummary{Source: "unconfirmed child", Destination: "unconfirmed child", PhysicalSource: "unconfirmed child", PhysicalDestination: "unconfirmed child"}
	presentation := packetPresentation{networkRefused: true, nativeSource: "unconfirmed child", nativeDestination: "unconfirmed child"}
	row = applyCapturedNetworkEndpoints(row, &presentation, p, wire, []string{"Ethernet", "IPv4"})
	require.Empty(t, row.Source)
	require.Empty(t, row.PhysicalSource)
	require.Empty(t, presentation.nativeSource)
}
