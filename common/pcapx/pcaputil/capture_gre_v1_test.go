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

// Authored controls: no public-corpus bytes are embedded in these fixtures.
func pptpV1TestGRE(flags, protocol, declared, call uint16, sequence, ack uint32, payload []byte) []byte {
	w := make([]byte, 4)
	binary.BigEndian.PutUint16(w, flags)
	binary.BigEndian.PutUint16(w[2:], protocol)
	if flags&0xc000 != 0 {
		w = append(w, make([]byte, 4)...)
	}
	if flags&0x2000 != 0 {
		w = binary.BigEndian.AppendUint16(w, declared)
		w = binary.BigEndian.AppendUint16(w, call)
	}
	if flags&0x1000 != 0 {
		w = binary.BigEndian.AppendUint32(w, sequence)
	}
	if flags&0x4000 != 0 {
		w = append(w, make([]byte, 4)...)
	}
	if flags&0x80 != 0 {
		w = binary.BigEndian.AppendUint32(w, ack)
	}
	return append(w, payload...)
}

func pptpV1TestOuter(t testing.TB, gre []byte, reverse bool) []byte {
	t.Helper()
	src, dst := net.IPv4(198, 51, 100, 1), net.IPv4(198, 51, 100, 2)
	if reverse {
		src, dst = dst, src
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolGRE, SrcIP: src, DstIP: dst}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(gre)))
	return bytes.Clone(b.Bytes())
}

func pptpV1TestInner(t testing.TB, tcp bool, sequence uint32, syn bool, payload []byte) []byte {
	t.Helper()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	stack := []gopacket.SerializableLayer{ip}
	if tcp {
		ip.Protocol = layers.IPProtocolTCP
		segment := &layers.TCP{SrcPort: 50000, DstPort: 80, Seq: sequence, SYN: syn, ACK: !syn, PSH: len(payload) != 0, Window: 65535}
		require.NoError(t, segment.SetNetworkLayerForChecksum(ip))
		stack = append(stack, segment, gopacket.Payload(payload))
	} else {
		udp := &layers.UDP{SrcPort: 53000, DstPort: 53}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		stack = append(stack, udp, &layers.DNS{ID: 4321, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("pptp.test"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}})
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, stack...))
	return bytes.Clone(b.Bytes())
}

func pptpV1TestData(t testing.TB, inner []byte, call uint16, sequence uint32) []byte {
	t.Helper()
	payload := append([]byte{0, 0x21}, inner...)
	return pptpV1TestGRE(0x3001, 0x880b, uint16(len(payload)), call, sequence, 0, payload)
}

func TestPPTPV1HeaderSelectsPPPAndBoundsEveryPrefix(t *testing.T) {
	inner := pptpV1TestInner(t, false, 0, false, nil)
	payload := append([]byte{0, 0x21}, inner...)
	for _, flags := range []uint16{0x3001, 0x3081} {
		wire := pptpV1TestGRE(flags, 0x880b, uint16(len(payload)), 42, 3, 7, payload)
		packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LayerTypeGRE}, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		require.NotNil(t, packet.Layer(layers.LayerTypePPP))
		require.NotNil(t, packet.Layer(layers.LayerTypeUDP))
		gre := packet.Layer(layers.LayerTypeGRE).(*layers.GRE)
		require.True(t, selectedPPTPGRE(gre))
		require.Equal(t, uint16(42), uint16(gre.Key))
		for cut := 2; cut < len(wire); cut++ {
			p := gopacket.NewPacket(wire[:cut], protocolPacketDecoder{decoder: layers.LayerTypeGRE}, gopacket.Default)
			require.NotNil(t, p.ErrorLayer(), "cut %d", cut)
			var failure *pptpHeaderError
			require.ErrorAs(t, p.ErrorLayer().Error(), &failure)
			require.Nil(t, p.Layer(layers.LayerTypePPP), "a partial declared payload cannot select PPP")
			require.Nil(t, p.Layer(layers.LayerTypeUDP))
			require.Equal(t, cut, failure.available)
		}
	}
}

func TestPPTPV1PPPProtocolPrefixCannotPanicOrBorrowBytes(t *testing.T) {
	for _, payload := range [][]byte{{0xff}, {0xff, 3}, {0}, {0xff, 3, 0}, {0, 0x20}} {
		wire := pptpV1TestGRE(0x3001, 0x880b, uint16(len(payload)), 42, 0, 0, payload)
		p := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LayerTypeGRE}, gopacket.Default)
		require.NotNil(t, p.ErrorLayer())
		var failure *pptpHeaderError
		require.ErrorAs(t, p.ErrorLayer().Error(), &failure)
		require.Nil(t, p.Layer(layers.LayerTypePPP))
	}
	// One-byte protocol compression is a complete PPP protocol observation.
	payload := append([]byte{0x21}, pptpV1TestInner(t, false, 0, false, nil)...)
	packet := gopacket.NewPacket(pptpV1TestGRE(0x3001, 0x880b, uint16(len(payload)), 42, 0, 0, payload), protocolPacketDecoder{decoder: layers.LayerTypeGRE}, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	require.NotNil(t, packet.Layer(layers.LayerTypeUDP))
}

func TestPPTPV1CallIdentityExcludesPayloadLengthAndRetainsDirection(t *testing.T) {
	var domains []CaptureDomain
	for _, size := range []int{0, 1, 13, 257} {
		inner := pptpV1TestInner(t, true, 1000, false, bytes.Repeat([]byte{'x'}, size))
		wire := pptpV1TestOuter(t, pptpV1TestData(t, inner, 42, uint32(size)), false)
		domain := PacketDomain(wire, gopacket.CaptureInfo{InterfaceIndex: 7}, layers.LinkTypeEthernet)
		require.Equal(t, "/pptp:198.51.100.1>198.51.100.2:42", domain.Encapsulation)
		domains = append(domains, domain)
	}
	for _, domain := range domains {
		require.Equal(t, domains[0], domain)
	}
	inner := pptpV1TestInner(t, true, 1000, true, nil)
	reverse := pptpV1TestOuter(t, pptpV1TestData(t, inner, 42, 0), true)
	otherCall := pptpV1TestOuter(t, pptpV1TestData(t, inner, 43, 0), false)
	require.NotEqual(t, domains[0], PacketDomain(reverse, gopacket.CaptureInfo{InterfaceIndex: 7}, layers.LinkTypeEthernet))
	require.NotEqual(t, domains[0], PacketDomain(otherCall, gopacket.CaptureInfo{InterfaceIndex: 7}, layers.LinkTypeEthernet))
	for _, version := range []uint16{0, 2, 3, 4, 5, 6, 7} {
		// Versions outside v1 keep their existing stateless observed key domain;
		// the native admission gate separately refuses versions two through seven.
		gre := binary.BigEndian.AppendUint16(nil, 0x2000|version)
		gre = binary.BigEndian.AppendUint16(gre, 0x0800)
		gre = binary.BigEndian.AppendUint32(gre, 42)
		gre = append(gre, inner...)
		wire := pptpV1TestOuter(t, gre, false)
		require.Equal(t, "/gre:198.51.100.1~198.51.100.2:42", PacketDomain(wire, gopacket.CaptureInfo{}, layers.LinkTypeEthernet).Encapsulation)
	}
}
