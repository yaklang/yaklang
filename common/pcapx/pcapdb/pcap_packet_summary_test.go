package pcapdb

import (
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestPacketFrameSummaryTCPAndUDP(t *testing.T) {
	payload := []byte("GET /demo HTTP/1.1\r\nHost: example.test\r\n\r\n")
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{1, 2, 3, 4, 5, 6},
		DstMAC:       net.HardwareAddr{6, 5, 4, 3, 2, 1},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version: 4, TTL: 64,
		SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2),
		Protocol: layers.IPProtocolTCP,
	}
	tcp := &layers.TCP{
		SrcPort: 12345, DstPort: 80,
		Seq: 1001, Ack: 2001, ACK: true, PSH: true, Window: 65535,
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(
		buffer,
		gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		eth, ip, tcp, gopacket.Payload(payload),
	))
	packet := packetMetadata(buffer.Bytes(), layers.LinkTypeEthernet)
	require.Equal(t, fmt.Sprintf("[ACK, PSH] Seq=1001 Ack=2001 Win=65535 Len=%d", len(payload)), packet.Summary)
	require.NotContains(t, packet.Summary, "HTTP")

	udpRaw := dnsPacket(t)
	decoded := gopacket.NewPacket(udpRaw, layers.LinkTypeEthernet, gopacket.Default)
	udp, ok := decoded.TransportLayer().(*layers.UDP)
	require.True(t, ok)
	packet = packetMetadata(udpRaw, layers.LinkTypeEthernet)
	require.Equal(t, fmt.Sprintf("SrcPort=%d DstPort=%d Len=%d Payload=%d",
		uint16(udp.SrcPort), uint16(udp.DstPort), udp.Length, len(udp.LayerPayload())), packet.Summary)
	require.NotContains(t, packet.Summary, "DNS")
}

func TestPacketSummaryPersistedAndReturnedByQueries(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)), WithProtocols(false))
	require.NoError(t, err)

	packets, err := db.QueryPackets(QueryLimit(1))
	require.NoError(t, err)
	require.Len(t, packets, 1)
	require.NotEmpty(t, packets[0].Summary)

	page, err := db.QueryPacketsPage(QueryLimit(1))
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, packets[0].Summary, page.Items[0].Summary)
}
