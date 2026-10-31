package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func greParentWire(t *testing.T, inner []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolGRE, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	gre := &layers.GRE{Version: 0, Protocol: layers.EthernetTypeIPv4, KeyPresent: true, Key: 42}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gre, gopacket.Payload(inner)))
	return bytes.Clone(b.Bytes())
}

func greParentAdmissionWire(t *testing.T, tcp bool) []byte {
	t.Helper()
	inner := tunnelAdmissionWire(t, "ipip", 42, "198.51.100.1", false, 0, tcp)
	wire := greParentWire(t, inner[14:])
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	gre, ok := packet.Layer(layers.LayerTypeGRE).(*layers.GRE)
	require.True(t, ok, "the control must contain an actual GRE header")
	require.Equal(t, uint8(0), gre.Version)
	require.Equal(t, uint32(42), gre.Key)
	return wire
}

func TestUnsupportedGREVersionCannotSelectInnerNetworkOrTransport(t *testing.T) {
	for version := uint16(2); version < 8; version++ {
		for _, kind := range []string{"UDP", "TCP", "wrong-IP-version", "short-IP-header"} {
			t.Run(fmt.Sprintf("version%d/%s", version, kind), func(t *testing.T) {
				wire := greParentAdmissionWire(t, kind == "TCP")
				flags := binary.BigEndian.Uint16(wire[34:36])
				binary.BigEndian.PutUint16(wire[34:36], flags&^7|version)
				if kind == "wrong-IP-version" {
					wire[42] = wire[42]&15 | 0x60
				}
				if kind == "short-IP-header" {
					wire = bytes.Clone(wire[:46])
				}
				packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
				require.Equal(t, uint8(version), packet.Layer(layers.LayerTypeGRE).(*layers.GRE).Version)
				packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), Length: len(wire), CaptureLength: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 17}}})
				var events []*ProtocolEvent
				a := coreFragmentParser(&events, 1<<20, 16)
				out, consumed := a.networkPacket(packet)
				require.True(t, consumed)
				require.Nil(t, out)
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "network-encapsulation-refusal", e.Profile)
				require.Equal(t, "limited", e.Status)
				require.Equal(t, uint8(version), e.Session["Observed GRE Version"])
				require.Equal(t, "GRE", e.Session["Unselected Carrier"])
				require.Equal(t, []string{"Ethernet", "IPv4"}, e.Session["Observed Header Layers"])
				require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
				require.NotContains(t, e.Session, "Expected IP Version")
				require.NotContains(t, e.Session, "Available Header Bytes")
				require.Equal(t, "/gre:198.51.100.1~198.51.100.2:42", e.Domain.Encapsulation)
				require.Equal(t, []PacketReference{{Number: 1, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Nil(t, a.udpSessions)
				require.Empty(t, a.fragments.sets)
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}

func TestUnsupportedGREParentPrecedesNativeARPObservation(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolGRE, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
	gre := &layers.GRE{Version: 0, Protocol: layers.EthernetTypeARP}
	arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: 1, SourceHwAddress: []byte{2, 0, 0, 0, 0, 1}, SourceProtAddress: []byte{192, 0, 2, 1}, DstHwAddress: make([]byte, 6), DstProtAddress: []byte{192, 0, 2, 2}}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gre, arp))
	wire := bytes.Clone(b.Bytes())
	wire[35] = wire[35]&^7 | 7
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.NotNil(t, packet.Layer(layers.LayerTypeARP), "the stateless decoder exposes an inner ARP before native admission")
	packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), Length: len(wire), CaptureLength: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1}})
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	out, consumed := a.networkPacket(packet)
	require.True(t, consumed)
	require.Nil(t, out)
	require.Len(t, events, 1)
	require.Equal(t, "ip", events[0].Protocol)
	require.Equal(t, "network-encapsulation-refusal", events[0].Profile)
	require.Equal(t, uint8(7), events[0].Session["Observed GRE Version"])
}

func TestUnselectedGeneveParentCannotClaimItsChildGREVersion(t *testing.T) {
	child := greParentAdmissionWire(t, false)
	child[35] = child[35]&^7 | 7
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 3}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 4}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 3), DstIP: net.IPv4(198, 51, 100, 4)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: 6081}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, &layers.Geneve{Protocol: layers.EthernetTypeTransparentEthernetBridging, VNI: 42, OAMPacket: true}, gopacket.Payload(child)))
	wire := bytes.Clone(b.Bytes())
	packet := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	require.NotNil(t, packet.Layer(layers.LayerTypeGRE), "the stateless decoder reaches the speculative child")
	packet.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), Length: len(wire), CaptureLength: len(wire)}, captureEvidence{Ref: PacketReference{Number: 1}})
	var events []*ProtocolEvent
	a := coreFragmentParser(&events, 1<<20, 16)
	out, consumed := a.networkPacket(packet)
	require.True(t, consumed)
	require.Nil(t, out)
	require.Len(t, events, 1)
	require.Equal(t, "network-encapsulation-refusal", events[0].Profile)
	require.NotContains(t, events[0].Session, "Observed GRE Version")
	require.NotContains(t, events[0].Session, "Unselected Carrier")
	require.Equal(t, []string{"Ethernet", "IPv4"}, events[0].Session["Observed Header Layers"])
}

func TestUnsupportedGREOpeningCannotReplaceExistingTCPGeneration(t *testing.T) {
	for _, protocols := range []bool{false, true} {
		for _, unsupported := range []bool{false, true} {
			t.Run(fmt.Sprintf("protocols%t/unsupported%t", protocols, unsupported), func(t *testing.T) {
				warm := greParentAdmissionWire(t, true)
				// A distant new SYN would create another generation if admitted.
				ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
				syn := &layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 1 << 30, SYN: true, Window: 4096}
				require.NoError(t, syn.SetNetworkLayerForChecksum(ip))
				inner := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(inner, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, syn))
				opening := greParentWire(t, inner.Bytes())
				decoded := gopacket.NewPacket(opening, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
				require.Nil(t, decoded.ErrorLayer())
				require.True(t, decoded.Layer(layers.LayerTypeTCP).(*layers.TCP).SYN)
				require.Equal(t, uint32(1<<30), decoded.Layer(layers.LayerTypeTCP).(*layers.TCP).Seq)
				if unsupported {
					opening[35] = opening[35]&^7 | 7
				}
				var capture bytes.Buffer
				writer := pcapgo.NewWriter(&capture)
				require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
				for n, wire := range [][]byte{warm, opening, warm} {
					stamp := int64(100 + n)
					if n == 1 {
						stamp = 10000
					}
					require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(stamp, 0), Length: len(wire), CaptureLength: len(wire)}, wire))
				}
				var mu sync.Mutex
				created := 0
				owners := map[uint64]uint64{}
				opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); created++ }), WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
					mu.Lock()
					defer mu.Unlock()
					for _, ref := range refs {
						owners[ref.Number] = f.Index
					}
				})}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(capture.Bytes()), opts...))
				if unsupported {
					require.Equal(t, 1, created)
					require.Equal(t, map[uint64]uint64{1: 1, 3: 1}, owners)
				} else {
					require.Equal(t, 2, created, "the same distant SYN under an admitted GRE parent is a real opening")
					require.Contains(t, owners, uint64(2))
				}
			})
		}
	}
}
