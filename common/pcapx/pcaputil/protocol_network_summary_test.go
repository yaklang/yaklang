package pcaputil

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestProtocolNetworkSummaryCapturedARP(t *testing.T) {
	for _, op := range []uint16{1, 2} {
		c := NewDefaultConfig()
		var event *ProtocolEvent
		require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { event = e })(c))
		require.NoError(t, c.prepareBinParser())
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeARP}
		arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: op, SourceHwAddress: eth.SrcMAC, SourceProtAddress: net.IP{192, 0, 2, 1}, DstHwAddress: eth.DstMAC, DstProtAddress: net.IP{192, 0, 2, 2}}
		buf := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, eth, arp))
		_, handled := c.binParser.networkPacket(gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default))
		require.True(t, handled)
		require.NotNil(t, event)
		require.Equal(t, "192.0.2.1", event.Source)
		require.Equal(t, "192.0.2.2", event.Destination)
		require.Contains(t, event.Summary, map[uint16]string{1: "ARP request", 2: "ARP reply"}[op])
		require.Contains(t, event.DumpLine(), `op="`+map[uint16]string{1: "1", 2: "2"}[op]+`"`)
		require.Equal(t, "unverified-neighbor", event.Fields["Observation"])
	}
}

func TestProtocolNetworkSummaryCapturedICMP(t *testing.T) {
	c := NewDefaultConfig()
	var event *ProtocolEvent
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { event = e })(c))
	require.NoError(t, c.prepareBinParser())
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(8, 0), Id: 7, Seq: 1}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, icmp, gopacket.Payload("ping")))
	_, handled := c.binParser.networkPacket(gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default))
	require.True(t, handled)
	require.Contains(t, event.Summary, "EchoRequest")
	require.Equal(t, "192.0.2.1", event.Source)
	require.Contains(t, event.DumpLine(), `op="8"`)
}
