package sharkcli

import (
	"context"
	"encoding/binary"
	"net"
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
	require.NotZero(t, rows[4].TCPFlowID)
	require.Equal(t, rows[4].TCPFlowID, rows[5].TCPFlowID)
	require.NotEqual(t, rows[0].TCPFlowID, rows[4].TCPFlowID, "SYN after an accepted reset starts a new native generation")
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
