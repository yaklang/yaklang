package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func vxlanProfileTestHeader(flags, second byte, policy uint16, vni uint32, last byte) []byte {
	return []byte{flags, second, byte(policy >> 8), byte(policy), byte(vni >> 16), byte(vni >> 8), byte(vni), last}
}

func vxlanProfileTestEthernet(payload []byte, kind layers.EthernetType) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, byte(kind >> 8), byte(kind)}, payload...)
}

func vxlanProfileTestOuter(t testing.TB, payload []byte, source, destination layers.UDPPort, reverse bool) []byte {
	t.Helper()
	src, dst := net.IPv4(198, 51, 100, 1), net.IPv4(198, 51, 100, 2)
	if reverse {
		src, dst = dst, src
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
	udp := &layers.UDP{SrcPort: source, DstPort: destination}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(payload)))
	return bytes.Clone(b.Bytes())
}

func vxlanProfileTestDNS(t testing.TB) []byte {
	return vxlanProfileTestEthernet(pptpV1TestInner(t, false, 0, false, nil), layers.EthernetTypeIPv4)
}

func TestVXLANMandatoryIStopsBeforeEveryInnerDecoder(t *testing.T) {
	inner := vxlanProfileTestDNS(t)
	for _, flags := range []byte{0, 0xf7} {
		packet := gopacket.NewPacket(append(vxlanProfileTestHeader(flags, 0x48, 0x1234, 42, 0xff), inner...), protocolPacketDecoder{decoder: layers.LayerTypeVXLAN}, gopacket.Default)
		var failure *vxlanHeaderError
		require.NotNil(t, packet.ErrorLayer())
		require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
		require.False(t, failure.truncated)
		require.Equal(t, uint32(42), failure.layer.VNI)
		require.Nil(t, packet.Layer(layers.LayerTypeEthernet))
		require.Nil(t, packet.NetworkLayer())
		require.Nil(t, packet.TransportLayer())
	}
}

func TestVXLANAllFixedHeaderPrefixesCannotBorrowOrInventVNI(t *testing.T) {
	header := vxlanProfileTestHeader(8, 0, 0, 42, 0)
	for size := 0; size < 8; size++ {
		packet := gopacket.NewPacket(bytes.Clone(header[:size]), protocolPacketDecoder{decoder: layers.LayerTypeVXLAN}, gopacket.Default)
		var failure *vxlanHeaderError
		require.ErrorAs(t, packet.ErrorLayer().Error(), &failure)
		require.True(t, failure.truncated)
		require.Equal(t, size, failure.available)
		require.Len(t, failure.layer.LayerContents(), size)
		require.Nil(t, packet.Layer(layers.LayerTypeEthernet))
		if size < 7 {
			require.Zero(t, failure.layer.VNI)
		} else {
			require.Equal(t, uint32(42), failure.layer.VNI)
		}
	}
}

func TestVXLANReceiveIgnoresReservedBitsAndGBPUsesActualA(t *testing.T) {
	inner := vxlanProfileTestDNS(t)
	for _, flags := range []byte{8, 9, 10, 12, 24, 40, 72, 136, 255} {
		payload := append(vxlanProfileTestHeader(flags, 0xff, 0xffff, 42, 0xff), inner...)
		packet := gopacket.NewPacket(payload, protocolPacketDecoder{decoder: layers.LayerTypeVXLAN}, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		v := packet.Layer(layers.LayerTypeVXLAN).(*layers.VXLAN)
		require.True(t, selectedVXLAN(v))
		require.Equal(t, uint32(42), v.VNI)
		require.NotNil(t, packet.Layer(layers.LayerTypeIPv4))
		require.NotNil(t, packet.Layer(layers.LayerTypeUDP))
		if flags&0x80 == 0 {
			require.False(t, v.GBPDontLearn)
			require.False(t, v.GBPApplied)
			require.Zero(t, v.GBPGroupPolicyID)
		}
	}
	for _, second := range []byte{0, 8, 0x40, 0x48, 0x80} {
		wire := vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(0x88, second, 0x1234, 42, 0), inner...), 55000, 4789, false)
		// Both the observer and native view report the actual raw flag. Neither
		// Boolean means that this analyzer learned a MAC or enforced a policy.
		for _, decoder := range []gopacket.Decoder{captureLinkDecoder(layers.LinkTypeEthernet), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}} {
			packet := gopacket.NewPacket(wire, decoder, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			v := packet.Layer(layers.LayerTypeVXLAN).(*layers.VXLAN)
			require.True(t, v.GBPExtension)
			require.Equal(t, second&8 != 0, v.GBPApplied)
			require.Equal(t, second&0x40 != 0, v.GBPDontLearn)
			require.Equal(t, uint16(0x1234), v.GBPGroupPolicyID)
		}
	}
}

func TestVXLANDestinationSelectionAndOuterBoundsPrecedeHeader(t *testing.T) {
	payload := append(vxlanProfileTestHeader(8, 0, 0, 42, 0), vxlanProfileTestDNS(t)...)
	sourceOnly := vxlanProfileTestOuter(t, payload, 4789, 55001, false)
	packet := gopacket.NewPacket(sourceOnly, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.Nil(t, packet.Layer(layers.LayerTypeVXLAN))
	require.Equal(t, layers.UDPPort(55001), packet.TransportLayer().(*layers.UDP).DstPort)
	observer := gopacket.NewPacket(sourceOnly, captureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.Nil(t, observer.Layer(layers.LayerTypeVXLAN))
	for _, header := range [][]byte{vxlanProfileTestHeader(0, 0, 0, 42, 0), []byte{8}} {
		wire := vxlanProfileTestOuter(t, append(header, vxlanProfileTestDNS(t)...), 55000, 4789, false)
		binary.BigEndian.PutUint16(wire[38:40], uint16(len(wire)-34+1))
		packet = gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		require.Nil(t, packet.Layer(layers.LayerTypeVXLAN), "an unavailable UDP envelope cannot select its inner header")
	}
	custom := layers.UDPPort(34789)
	before := custom.LayerType()
	layers.RegisterUDPPortLayerType(custom, layers.LayerTypeVXLAN)
	defer layers.RegisterUDPPortLayerType(custom, before)
	wire := vxlanProfileTestOuter(t, payload, 55000, custom, false)
	packet = gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	require.NotNil(t, packet.Layer(layers.LayerTypeVXLAN))
}

func TestVXLANGPERequiresCoherentHeaderAndNeverAssumesEthernet(t *testing.T) {
	for _, h := range [][]byte{vxlanProfileTestHeader(8, 0, 0, 42, 0), vxlanProfileTestHeader(12, 0, 1, 42, 0), vxlanProfileTestHeader(12, 0, 2, 42, 0), vxlanProfileTestHeader(12, 0, 3, 42, 0), vxlanProfileTestHeader(15, 0xff, 0xff04, 42, 0xff), vxlanProfileTestHeader(0xcc, 0xff, 0xff7f, 42, 0xff)} {
		wire := vxlanProfileTestOuter(t, append(h, vxlanProfileTestDNS(t)...), 55000, 4790, false)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		_, ok := packet.Layer(layers.LayerTypeVXLAN).(*captureVXLANGPE)
		require.True(t, ok)
		var ethernet, IP int
		for _, layer := range packet.Layers() {
			if layer.LayerType() == layers.LayerTypeEthernet {
				ethernet++
			}
			if _, network := layer.(gopacket.NetworkLayer); network {
				IP++
			}
		}
		require.Equal(t, 1, ethernet)
		require.Equal(t, 1, IP)
	}
	for _, source := range []layers.UDPPort{53, 4789, 55000} {
		wire := vxlanProfileTestOuter(t, append(vxlanProfileTestHeader(12, 0, 3, 42, 0), vxlanProfileTestDNS(t)...), source, 4790, false)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		_, observed := packet.Layer(layers.LayerTypeVXLAN).(*captureVXLANGPE)
		require.True(t, observed, "a source-port application hint cannot override a validated destination header")
	}
	for _, h := range [][]byte{vxlanProfileTestHeader(4, 0, 3, 42, 0), vxlanProfileTestHeader(0x1c, 0, 3, 42, 0), vxlanProfileTestHeader(8, 0, 3, 42, 0), vxlanProfileTestHeader(12, 0, 0, 42, 0), []byte{12, 0, 0, 3, 0, 0, 42}} {
		require.False(t, observedVXLANGPE(h))
		wire := vxlanProfileTestOuter(t, h, 55000, 4790, false)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		require.Nil(t, packet.Layer(layers.LayerTypeVXLAN), "a service port alone cannot confirm a GPE header")
	}
}
