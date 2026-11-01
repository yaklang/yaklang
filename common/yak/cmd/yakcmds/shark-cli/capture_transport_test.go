package sharkcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTunnelSummaryAndStreamUseInnerEndpointPair(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	outer := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolGRE, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
	inner := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 443, SYN: true, Seq: 42, Window: 65535}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(inner))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, outer, &layers.GRE{Protocol: layers.EthernetTypeIPv4}, inner, tcp))
	wire := b.Bytes()
	packet := &capturedPacket{number: 1, data: wire, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(wire), Length: len(wire)}}
	row := summarize(packet)
	require.Equal(t, "192.0.2.1", row.Source)
	require.Equal(t, "192.0.2.2", row.Destination)
	require.EqualValues(t, 12345, row.SourcePort)
	require.Equal(t, "tcp", row.Transport)
	store := newStreamStore(8, 4096)
	defer store.finish()
	store.add(packet)
	require.NotZero(t, packet.streamID)
	stream := store.snapshot(packet.streamID)
	require.NotNil(t, stream)
	require.Contains(t, stream.endpoints[0], "192.0.2.")
	for _, side := range stream.endpoints {
		require.NotContains(t, side, "198.51.100.")
	}
}

func TestNativeTCPFlowIDsFollowCoreGenerations(t *testing.T) {
	request := "GET /one HTTP/1.1\r\nHost: test\r\n\r\n"
	reset := tcpPacket(t, 4, 101+uint32(len(request)), "", false, false, false, false)
	// A reset must match the observed receive cursor before tuple reuse is
	// allowed. A different SYN alone must not replace an active connection.
	decoded := gopacket.NewPacket(reset.data, layers.LinkTypeEthernet, gopacket.Default)
	eth := decoded.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	ip := decoded.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp := decoded.Layer(layers.LayerTypeTCP).(*layers.TCP)
	tcp.RST = true
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp))
	reset.data = buffer.Bytes()
	packets := [][]byte{
		tcpPacket(t, 1, 100, "", false, true, false, false).data,
		tcpPacket(t, 2, 101, request, false, false, false, false).data,
		tcpPacket(t, 3, 5000, "", false, true, false, false).data,
		reset.data,
		tcpPacket(t, 5, 5000, "", false, true, false, false).data,
		tcpPacket(t, 6, 5001, "SSH-2.0-audit\r\n", false, false, false, false).data,
	}
	src, err := openOffline(fixture(t, "generations.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	var rows []packetSummary
	for p := range s.packets {
		rows = append(rows, summarize(p))
	}
	require.NoError(t, s.result())
	require.Len(t, rows, 6)
	require.NotZero(t, rows[0].TCPFlowID)
	require.Equal(t, rows[0].TCPFlowID, rows[1].TCPFlowID)
	require.Equal(t, rows[0].TCPFlowID, rows[2].TCPFlowID, "conflicting SYN does not replace an active connection")
	require.Equal(t, rows[0].StreamID, rows[2].StreamID, "a rejected conflicting SYN cannot reset the preview before native association")
	require.NotZero(t, rows[4].TCPFlowID)
	require.Equal(t, rows[4].TCPFlowID, rows[5].TCPFlowID)
	require.NotEqual(t, rows[0].TCPFlowID, rows[4].TCPFlowID, "SYN after an accepted reset starts a new native generation")
}

func TestCaptureBackedPreviewRequiresNativeFlowAssociation(t *testing.T) {
	packet := tcpPacket(t, 1, 100, "GET / HTTP/1.1\r\nHost: test\r\n\r\n", false, false, false, false)
	var err error
	packet.history, err = pcaputil.NewProtocolInspector(8, 4096)
	require.NoError(t, err)
	store := newStreamStore(8, 4096)
	defer store.finish()
	store.add(packet)
	require.Zero(t, packet.streamID, "an observed header alone cannot create a capture-backed preview connection")
	require.Empty(t, store.records)
	packet.tcpFlowID = 17
	store.add(packet)
	require.Zero(t, packet.streamID, "a scalar ID without native creation/data observations cannot create a second reassembler")
}

func TestReassembledDatagramPresentationUsesObservedHeaderPathOnce(t *testing.T) {
	row := packetSummary{Transport: "udp", MessageTransport: "udp", MessageSource: "192.0.2.1:53", Layers: []string{"Ethernet", "IPv4", "Fragment"},
		TransportView: &packetTransportView{Layers: []string{"Ethernet", "IPv4", "UDP"}, LayersComplete: true}}
	presentation := packetPresentation{protocol: "DNS", stack: []string{"DNS"}, status: "decoded"}
	projected := presentation.apply(row)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "DNS"}, projected.ProtocolStack)
	require.Equal(t, row.TransportView.Layers, []string{"Ethernet", "IPv4", "UDP"}, "appending an application layer must not alter the owned network observation")
}

func TestFinalACKKeepsClosedNativeFlowAndPreview(t *testing.T) {
	request := "GET /audit HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
	packets := [][]byte{
		tcpPacket(t, 1, 100, request, false, false, false, false).data,
		tcpPacket(t, 2, 500, response, true, false, false, false).data,
		tcpPacket(t, 3, 100+uint32(len(request)), "", false, false, true, false).data,
		tcpPacket(t, 4, 500+uint32(len(response)), "", true, false, true, false).data,
		tcpPacket(t, 5, 101+uint32(len(request)), "", false, false, false, false).data,
	}
	src, err := openOffline(fixture(t, "final-ack.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, true)
	var rows []packetSummary
	for p := range s.packets {
		rows = append(rows, summarize(p))
	}
	require.NoError(t, s.result())
	require.Len(t, rows, len(packets))
	require.NotZero(t, rows[0].TCPFlowID)
	require.Equal(t, rows[0].TCPFlowID, rows[4].TCPFlowID)
	require.NotZero(t, rows[0].StreamID)
	require.Equal(t, rows[0].StreamID, rows[4].StreamID, "a final ACK does not create another preview connection")
	require.Equal(t, "TCP", rows[4].Protocol, "connection association is distinct from packet payload identity")
}

func fragmentedGRETCPFrame(t *testing.T, syn bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	outer := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolGRE, SrcIP: net.ParseIP("2001:db8:1::1"), DstIP: net.ParseIP("2001:db8:1::2")}
	inner := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: 100, SYN: syn, ACK: !syn, Window: 65535}
	var payload gopacket.Payload
	if !syn {
		tcp.Seq = 101
		payload = []byte("GET /fragment HTTP/1.1\r\nHost: example.test\r\n\r\n")
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(inner))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, outer, &layers.GRE{Protocol: layers.EthernetTypeIPv4}, inner, tcp, payload))
	return b.Bytes()
}

func TestOuterFragmentCompletesInnerMessageAndEarlierPacketFields(t *testing.T) {
	whole := fragmentedGRETCPFrame(t, false)
	body := whole[54:]
	packets := [][]byte{fragmentedGRETCPFrame(t, true)}
	for at := 0; at < len(body); {
		n := min(48, len(body)-at)
		header := append([]byte(nil), whole[:54]...)
		header[20] = 44
		binary.BigEndian.PutUint16(header[18:], uint16(8+n))
		fh := make([]byte, 8)
		fh[0] = 47
		offset := uint16(at/8) << 3
		if at+n < len(body) {
			offset |= 1
		}
		binary.BigEndian.PutUint16(fh[2:], offset)
		binary.BigEndian.PutUint32(fh[4:], 42)
		packets = append(packets, append(append(header, fh...), body[at:at+n]...))
		at += n
	}
	src, err := openOffline(fixture(t, "fragmented-tunnel.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	var rows []*capturedPacket
	for p := range s.packets {
		rows = append(rows, p)
	}
	require.NoError(t, s.result())
	last := summarize(rows[len(rows)-1])
	require.Equal(t, "HTTP/1.1", last.Protocol)
	require.Equal(t, "192.0.2.1", last.Source)
	require.EqualValues(t, 12345, last.SourcePort)
	require.Equal(t, rows[0].tcpFlowID, last.TCPFlowID)
	require.NotZero(t, last.TCPFlowID)
	require.Contains(t, last.ProtocolStack, "TCP")
	require.Contains(t, last.ProtocolStack, "IPv4")
	require.Equal(t, []string{"Ethernet", "IPv6", "GRE", "IPv4", "TCP", "HTTP/1.1"}, last.ProtocolStack, "the observed header path must not acquire a fabricated duplicate IP/TCP pair")
	events := rows[1].protocolEvents()
	require.NotEmpty(t, events, "an earlier physical fragment can inspect the completed PDU")
	found := false
	for _, e := range events {
		if e.Protocol == "http" {
			found = true
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.NotEmpty(t, fields)
		}
	}
	require.True(t, found)
	require.Contains(t, last.TCPFlowDomain.Encapsulation, "/gre:")
	require.Equal(t, pcaputil.CaptureDomain{}, rows[1].reference.Domain, "raw fragment keeps its original capture domain")
}

func TestFragmentedUnknownApplicationRetainsObservedTransportEndpoints(t *testing.T) {
	whole := fragmentedGRETCPFrame(t, false)
	// Keep a valid full TCP header but remove the recognizable HTTP grammar.
	decoded := gopacket.NewPacket(whole, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	inner, tr := pcaputil.CaptureTransport(decoded)
	tcp := tr.(*layers.TCP)
	start := len(whole) - len(tcp.Payload)
	unknownPayload := bytes.Repeat([]byte{0x9b}, len(tcp.Payload))
	copy(whole[start:], unknownPayload)
	require.NoError(t, tcp.SetNetworkLayerForChecksum(inner))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, tcp, gopacket.Payload(unknownPayload)))
	copy(whole[start-len(tcp.LayerContents()):], buffer.Bytes())
	body := whole[54:]
	var packets [][]byte
	for at := 0; at < len(body); {
		n := min(48, len(body)-at)
		header := append([]byte(nil), whole[:54]...)
		header[20] = 44
		binary.BigEndian.PutUint16(header[18:], uint16(8+n))
		fh := make([]byte, 8)
		fh[0] = 47
		off := uint16(at/8) << 3
		if at+n < len(body) {
			off |= 1
		}
		binary.BigEndian.PutUint16(fh[2:], off)
		binary.BigEndian.PutUint32(fh[4:], 75)
		packets = append(packets, append(append(header, fh...), body[at:at+n]...))
		at += n
	}
	src, err := openOffline(fixture(t, "unknown-fragment.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{}, false)
	var rows []*capturedPacket
	for p := range s.packets {
		rows = append(rows, p)
	}
	require.NoError(t, s.result())
	last := summarize(rows[len(rows)-1])
	require.Equal(t, "TCP", last.Protocol)
	require.Equal(t, "192.0.2.1", last.Source)
	require.Equal(t, "192.0.2.2", last.Destination)
	require.Equal(t, "2001:db8:1::1", last.PhysicalSource)
	require.Equal(t, "2001:db8:1::2", last.PhysicalDestination)
	require.Equal(t, "observed transport header", last.ProtocolScope)
	require.NotNil(t, last.TransportView)
	require.Equal(t, "tcp", last.TransportView.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv6", "GRE", "IPv4", "TCP"}, last.ProtocolStack)
	require.EqualValues(t, 12345, last.SourcePort)
	require.Contains(t, last.Info, "sequence acceptance separate")
	require.True(t, last.TransportView.LayersComplete)
	require.NotZero(t, last.StreamID, "the real reassembled TCP header must also reach the stream page")
	preview := s.streams.snapshot(last.StreamID)
	require.NotNil(t, preview)
	require.EqualValues(t, len(unknownPayload), preview.bytes[0])
	require.Equal(t, unknownPayload, preview.prefix[0])
	fields, sources := transportObservationDetail(last.TransportView)
	require.Len(t, sources, 1)
	found := false
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Source Port:") {
			found = true
			require.True(t, f.hasRange)
			require.Equal(t, []byte{0x30, 0x39}, sources[f.source-1].Data[f.start:f.end])
		}
	}
	require.True(t, found)
	// The Windows UI deep decoder crosses JSON IPC. Keeping this metadata only
	// in the capture process would turn the field page back into Fragment.
	wire, err := marshalDecodeRequest(packetDecodeRequest(rows[len(rows)-1]))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	deep, err := decodeWorkerOutput(output.Bytes(), rows[len(rows)-1])
	require.NoError(t, err)
	require.Equal(t, "TCP", deep.protocol)
	require.Equal(t, "TCP", quickDetail(rows[len(rows)-1]).protocol)
	workerHeader := false
	for _, f := range deep.fields {
		if f.text == "TCP · observed transport header" {
			workerHeader = true
			require.True(t, f.hasRange)
			require.Positive(t, f.source)
			require.Equal(t, last.TransportView.Header, deep.sources[f.source-1].Data[f.start:f.end])
		}
	}
	require.True(t, workerHeader)
	require.Nil(t, summarize(rows[0]).TransportView, "incomplete network data does not invent a transport view")
	for _, event := range rows[len(rows)-1].protocolEvents() {
		require.NotEqual(t, "http", event.Protocol, "a valid TCP header does not identify application bytes")
	}
}
