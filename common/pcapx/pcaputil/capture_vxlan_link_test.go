package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

type vxlanLinkControl struct {
	name string
	link layers.LinkType
	v6   bool
	loop bool
}

func vxlanSupportedLinkControls() []vxlanLinkControl {
	return []vxlanLinkControl{{"raw-v4", layers.LinkTypeRaw, false, false}, {"raw-v6", layers.LinkTypeRaw, true, false}, {"null-v4", layers.LinkTypeNull, false, true}, {"null-v6", layers.LinkTypeNull, true, true}, {"loop-v4", layers.LinkTypeLoop, false, true}, {"loop-v6", layers.LinkTypeLoop, true, true}, {"IPv4", layers.LinkTypeIPv4, false, false}, {"IPv6", layers.LinkTypeIPv6, true, false}}
}

func vxlanLinkControlWire(t testing.TB, control vxlanLinkControl, payload []byte, source, destination layers.UDPPort) []byte {
	t.Helper()
	var network gopacket.SerializableLayer
	var checksum gopacket.NetworkLayer
	if control.v6 {
		ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
		network, checksum = ip, ip
	} else {
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(198, 51, 100, 1), DstIP: net.IPv4(198, 51, 100, 2)}
		network, checksum = ip, ip
	}
	u := &layers.UDP{SrcPort: source, DstPort: destination}
	require.NoError(t, u.SetNetworkLayerForChecksum(checksum))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, network, u, gopacket.Payload(payload)))
	wire := bytes.Clone(b.Bytes())
	if control.loop {
		family := uint32(layers.ProtocolFamilyIPv4)
		if control.v6 {
			family = uint32(layers.ProtocolFamilyIPv6BSD)
		}
		header := make([]byte, 4)
		if control.link == layers.LinkTypeLoop {
			binary.BigEndian.PutUint32(header, family)
		} else {
			binary.LittleEndian.PutUint32(header, family)
		}
		wire = append(header, wire...)
	}
	return wire
}

func vxlanLinkControlCapture(t testing.TB, control vxlanLinkControl, wire []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	require.NoError(t, w.WriteFileHeader(65535, control.link))
	require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), CaptureLength: len(wire), Length: len(wire)}, wire))
	return out.Bytes()
}

func TestVXLANObserverAndNativeFieldBitsAgreeAcrossSupportedLinks(t *testing.T) {
	for _, control := range vxlanSupportedLinkControls() {
		t.Run(control.name, func(t *testing.T) {
			for _, second := range []byte{8, 0x80, 0x48} {
				body := append(vxlanProfileTestHeader(0x88, second, 0x1234, 42, 0), vxlanProfileTestDNS(t)...)
				wire := vxlanLinkControlWire(t, control, body, 55000, 4789)
				for _, decoder := range []gopacket.Decoder{captureLinkDecoder(control.link), protocolPacketDecoder{decoder: control.link}} {
					packet := gopacket.NewPacket(wire, decoder, gopacket.Default)
					require.Nil(t, packet.ErrorLayer())
					v := packet.Layer(layers.LayerTypeVXLAN).(*layers.VXLAN)
					require.Equal(t, second&8 != 0, v.GBPApplied)
					require.Equal(t, second&0x40 != 0, v.GBPDontLearn)
					require.Equal(t, uint16(0x1234), v.GBPGroupPolicyID)
				}
				var messages []*ProtocolEvent
				var observed bool
				require.NoError(t, ReplayPcap(bytes.NewReader(vxlanLinkControlCapture(t, control, wire)), WithOnProtocolMessage(func(e *ProtocolEvent) { messages = append(messages, e) }), WithEveryPacket(func(packet gopacket.Packet) {
					observed = true
					v := packet.Layer(layers.LayerTypeVXLAN).(*layers.VXLAN)
					require.Equal(t, second&8 != 0, v.GBPApplied)
				})))
				require.True(t, observed)
				require.Len(t, messages, 1)
				require.Equal(t, "dns", messages[0].Protocol)
				require.Equal(t, "decoded", messages[0].Status)
			}
		})
	}
}

func TestVXLANSourcePortAloneCannotSelectTunnelOnSupportedLinks(t *testing.T) {
	for _, control := range vxlanSupportedLinkControls() {
		t.Run(control.name, func(t *testing.T) {
			wire := vxlanLinkControlWire(t, control, append(vxlanProfileTestHeader(8, 0, 0, 42, 0), vxlanProfileTestDNS(t)...), 4789, 55001)
			for _, decoder := range []gopacket.Decoder{captureLinkDecoder(control.link), protocolPacketDecoder{decoder: control.link}} {
				packet := gopacket.NewPacket(wire, decoder, gopacket.Default)
				require.Nil(t, packet.Layer(layers.LayerTypeVXLAN))
				require.Equal(t, layers.UDPPort(55001), packet.TransportLayer().(*layers.UDP).DstPort)
			}
			var messages []*ProtocolEvent
			created := 0
			require.NoError(t, ReplayPcap(bytes.NewReader(vxlanLinkControlCapture(t, control, wire)), WithOnProtocolMessage(func(e *ProtocolEvent) { messages = append(messages, e) }), WithOnTrafficFlowCreated(func(*TrafficFlow) { created++ }), WithEveryPacket(func(packet gopacket.Packet) { require.Nil(t, packet.Layer(layers.LayerTypeVXLAN)) })))
			require.Zero(t, created)
			require.Len(t, messages, 1)
			require.Equal(t, "udp", messages[0].Transport)
			require.Empty(t, messages[0].Protocol)
			require.Empty(t, messages[0].Domain.Encapsulation)
		})
	}
}

func TestVXLANGPEFastAndObservedReplayAgreeAcrossSupportedLinks(t *testing.T) {
	for _, control := range vxlanSupportedLinkControls() {
		for _, next := range []byte{0, 1, 3, 4} {
			flags := byte(8)
			if next != 0 {
				flags |= 4
			}
			wire := vxlanLinkControlWire(t, control, append(vxlanProfileTestHeader(flags, 0, uint16(next), 42, 0), vxlanProfileTestDNS(t)...), 55000, 4790)
			for _, observer := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/next%d/observer%t", control.name, next, observer), func(t *testing.T) {
					var messages []*ProtocolEvent
					var stats TCPReassemblyStats
					opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: false}), WithOnProtocolMessage(func(e *ProtocolEvent) { messages = append(messages, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })}
					if observer {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(vxlanLinkControlCapture(t, control, wire)), opts...))
					require.Len(t, messages, 1)
					require.Equal(t, "VXLANProfileUnselected", messages[0].ExpertCode)
					require.Equal(t, "headers", messages[0].Completeness)
					require.Equal(t, next, messages[0].Session["VXLAN-GPE Next Protocol"])
					require.Zero(t, stats.DecodeErrors)
				})
			}
		}
	}
}
