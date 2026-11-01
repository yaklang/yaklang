package pcaputil

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func erspan3TestFrame(t *testing.T, stamp uint32, hardware byte, egress bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	inner := gopacket.NewSerializeBuffer()
	query := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
	require.NoError(t, gopacket.SerializeLayers(inner, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(query)))
	h := make([]byte, 20)
	h[0] = 0x20
	h[3] = 7 // session ID
	binary.BigEndian.PutUint32(h[4:], stamp)
	binary.BigEndian.PutUint16(h[10:], uint16(hardware)<<4|1)
	if egress {
		h[11] |= 8
	}
	binary.BigEndian.PutUint32(h[12:], 5<<26|1) // platform5, source port1
	binary.BigEndian.PutUint32(h[16:], stamp)   // changing platform timestamp
	eth.EthernetType = layers.EthernetTypeIPv6
	outer := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolGRE, SrcIP: net.ParseIP("2001:db8:1::1"), DstIP: net.ParseIP("2001:db8:1::2")}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, eth, outer, &layers.GRE{Protocol: 0x22eb, SeqPresent: true, Seq: 1}, gopacket.Payload(append(h, inner.Bytes()...))))
	return b.Bytes()
}

func TestERSPAN3NativeIngressAndStableSourceDomain(t *testing.T) {
	first := erspan3TestFrame(t, 100, 3, false)
	events := tunnelAdmissionReplay(t, first)
	require.Len(t, events, 1)
	require.Equal(t, "dns", events[0].Protocol)
	require.Equal(t, "decoded", events[0].Status, events[0].Error)
	require.Equal(t, "192.0.2.1:40000", events[0].Source)
	domain := PacketDomain(first, gopacket.CaptureInfo{}, layers.LinkTypeEthernet)
	require.Contains(t, domain.Encapsulation, "/erspan3:7:3:true:5:1")
	require.Equal(t, domain, PacketDomain(erspan3TestFrame(t, 999, 3, true), gopacket.CaptureInfo{}, layers.LinkTypeEthernet), "timestamps and packet direction cannot split one observed source")
	require.NotEqual(t, domain, PacketDomain(erspan3TestFrame(t, 100, 4, false), gopacket.CaptureInfo{}, layers.LinkTypeEthernet), "independent mirror engines retain separate namespaces")
}

func TestERSPAN3RejectsMissingOptionalIdentity(t *testing.T) {
	for _, wire := range [][]byte{make([]byte, 11), append([]byte{0x20}, make([]byte, 10)...), append(append([]byte{0x20}, make([]byte, 10)...), 1)} {
		p := gopacket.NewPacket(wire, captureERSPAN3Decoder{}, gopacket.Default)
		require.NotNil(t, p.ErrorLayer())
		require.Nil(t, p.Layer(layers.LayerTypeEthernet))
	}
}
