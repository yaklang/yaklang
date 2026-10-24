package pcaputil

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func pimTestFrame(t *testing.T, ipv6, null, wholeChecksum bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	outer4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: 103, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
	outer6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: 103, SrcIP: net.ParseIP("2001:db8:1::1"), DstIP: net.ParseIP("2001:db8:1::2")}
	inner4 := &layers.IPv4{Version: 4, TTL: 5, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("239.1.2.3")}
	inner6 := &layers.IPv6{Version: 6, HopLimit: 5, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("ff3e::123")}
	var outer, inner gopacket.SerializableLayer = outer4, inner4
	var network gopacket.NetworkLayer = outer4
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	if ipv6 {
		outer, inner, network = outer6, inner6, outer6
		eth.EthernetType = layers.EthernetTypeIPv6
		require.NoError(t, udp.SetNetworkLayerForChecksum(inner6))
	} else {
		require.NoError(t, udp.SetNetworkLayerForChecksum(inner4))
	}
	payload := gopacket.NewSerializeBuffer()
	query := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 0, 0, 1, 0, 1}
	require.NoError(t, gopacket.SerializeLayers(payload, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, inner, udp, gopacket.Payload(query)))
	w := append([]byte{0x21, 0, 0, 0, 0, 0, 0, 0}, payload.Bytes()...)
	if null {
		w[4] |= 0x40
	}
	checksum := w[:8]
	if wholeChecksum {
		checksum = w
	}
	binary.BigEndian.PutUint16(w[2:], pimChecksum(checksum, network))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, outer, gopacket.Payload(w)))
	return b.Bytes()
}

func TestPIMRegisterCarrierIPv4IPv6AndWholeChecksum(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		for _, whole := range []bool{false, true} {
			wire := pimTestFrame(t, ipv6, false, whole)
			packet := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			require.NotNil(t, packet.Layer(layers.LayerTypeUDP))
			domain := PacketDomain(wire, gopacket.CaptureInfo{}, layers.LinkTypeEthernet)
			require.Contains(t, domain.Encapsulation, "/pim-register:")
			events := tunnelAdmissionReplay(t, wire)
			require.Len(t, events, 1)
			require.Equal(t, "dns", events[0].Protocol)
			require.Equal(t, "decoded", events[0].Status, events[0].Error)
			require.Equal(t, domain, events[0].Domain)
		}
	}
}

func TestPIMRegisterDoesNotInventTrafficFromNullOrBadChecksum(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		wire := pimTestFrame(t, ipv6, true, false)
		packet := gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		require.Nil(t, packet.Layer(layers.LayerTypeUDP), "a Null-Register is not application traffic even if its dummy body looks valid")
		require.NotContains(t, PacketDomain(wire, gopacket.CaptureInfo{}, layers.LinkTypeEthernet).Encapsulation, "/pim-register:")
		wire = pimTestFrame(t, ipv6, false, false)
		offset := 14 + 20
		if ipv6 {
			offset = 14 + 40
		}
		wire[offset+2] ^= 1
		packet = gopacket.NewPacket(wire, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		require.NotNil(t, packet.ErrorLayer())
		require.Nil(t, packet.Layer(layers.LayerTypeUDP))
	}
}
