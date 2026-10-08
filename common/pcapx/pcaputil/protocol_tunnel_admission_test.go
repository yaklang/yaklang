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

// All packets below are authored synthetic controls; they contain no bytes
// copied from third-party traffic. Inner endpoints are intentionally repeated
// across domains so a missing tunnel identity cannot pass accidentally.
func tunnelAdmissionWire(t *testing.T, profile string, vni uint32, outer string, reverse bool, depth int, tcp bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	inner := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	var network gopacket.NetworkLayer = inner
	var ip gopacket.SerializableLayer = inner
	if profile == "6in4" {
		v6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
		network, ip = v6, v6
	}
	var transport gopacket.SerializableLayer
	var application gopacket.SerializableLayer
	if tcp {
		inner.Protocol = layers.IPProtocolTCP
		stream := &layers.TCP{SrcPort: 40000, DstPort: 80, Seq: 100, ACK: true, Window: 4096}
		require.NoError(t, stream.SetNetworkLayerForChecksum(network))
		transport = stream
		application = gopacket.Payload(fmt.Sprintf("GET /%d HTTP/1.1\r\nHost: tunnel.test\r\n\r\n", vni))
	} else {
		udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
		require.NoError(t, udp.SetNetworkLayerForChecksum(network))
		transport = udp
		application = &layers.DNS{ID: 1234, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("tunnel.test"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	}
	stack := []gopacket.SerializableLayer{eth}
	for i := 0; i < depth; i++ {
		src, dst := net.ParseIP(outer), net.ParseIP("198.51.100.2")
		if reverse {
			src, dst = dst, src
		}
		carrier := &layers.IPv4{Version: 4, TTL: 64, SrcIP: src, DstIP: dst, Protocol: layers.IPProtocolIPv4}
		if profile == "6in4" && i == depth-1 {
			carrier.Protocol = layers.IPProtocolIPv6
		}
		if profile == "geneve" {
			carrier.Protocol = layers.IPProtocolUDP
			udp := &layers.UDP{SrcPort: 45000, DstPort: 6081}
			require.NoError(t, udp.SetNetworkLayerForChecksum(carrier))
			stack = append(stack, carrier, udp, &layers.Geneve{Protocol: layers.EthernetTypeTransparentEthernetBridging, VNI: vni}, eth)
		} else {
			stack = append(stack, carrier)
		}
	}
	stack = append(stack, ip, transport, application)
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, stack...))
	return bytes.Clone(b.Bytes())
}

func tunnelAdmissionReplay(t *testing.T, packets ...[]byte) []*ProtocolEvent {
	t.Helper()
	var capture bytes.Buffer
	w := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, raw := range packets {
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i)), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	var events []*ProtocolEvent
	var mu sync.Mutex
	require.NoError(t, ReplayPcap(bytes.NewReader(capture.Bytes()), WithOnProtocolMessage(func(e *ProtocolEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})))
	return events
}

func TestProtocolTunnelAdmission(t *testing.T) {
	for _, profile := range []string{"ipip", "6in4", "geneve"} {
		t.Run(profile, func(t *testing.T) {
			raw := tunnelAdmissionWire(t, profile, 5013, "198.51.100.1", false, 1, false)
			domain := PacketDomain(raw, gopacket.CaptureInfo{InterfaceIndex: 7}, layers.LinkTypeEthernet)
			want := "/" + profile + ":198.51.100.1~198.51.100.2"
			if profile == "geneve" {
				want += ":5013"
			}
			require.Equal(t, CaptureDomain{Interface: 7, Encapsulation: want}, domain)
			reverse := tunnelAdmissionWire(t, profile, 5013, "198.51.100.1", true, 1, false)
			require.Equal(t, domain, PacketDomain(reverse, gopacket.CaptureInfo{InterfaceIndex: 7}, layers.LinkTypeEthernet))
			events := tunnelAdmissionReplay(t, raw)
			require.Len(t, events, 1)
			require.Equal(t, "dns", events[0].Protocol)
			require.Equal(t, "decoded", events[0].Status)
			require.Equal(t, want, events[0].Domain.Encapsulation)
			require.Len(t, events[0].SourceBytes.PacketRefs, 1)
			require.Equal(t, events[0].Domain, events[0].SourceBytes.PacketRefs[0].Domain)
			if profile == "6in4" {
				require.Equal(t, "[2001:db8::1]:40000", events[0].Source)
			} else {
				require.Equal(t, "192.0.2.1:40000", events[0].Source)
			}
		})
	}
}

func TestProtocolTunnelFlowIsolation(t *testing.T) {
	for _, profile := range []string{"ipip", "geneve"} {
		t.Run(profile, func(t *testing.T) {
			first := tunnelAdmissionWire(t, profile, 5013, "198.51.100.1", false, 1, true)
			outer := "198.51.100.1"
			if profile == "ipip" {
				outer = "198.51.100.3"
			}
			second := tunnelAdmissionWire(t, profile, 5014, outer, false, 1, true)
			events := tunnelAdmissionReplay(t, first, second)
			require.Len(t, events, 2)
			for _, e := range events {
				require.Equal(t, "http", e.Protocol)
				require.Equal(t, "decoded", e.Status)
				require.Equal(t, "192.0.2.1:40000", e.Source)
			}
			require.NotEqual(t, events[0].FlowID, events[1].FlowID)
			require.NotEqual(t, events[0].Domain, events[1].Domain)
		})
	}
}

func TestProtocolTunnelDepthLimit(t *testing.T) {
	for _, profile := range []string{"ipip", "6in4", "geneve"} {
		for _, depth := range []int{4, 5} {
			t.Run(fmt.Sprintf("%s/%d", profile, depth), func(t *testing.T) {
				events := tunnelAdmissionReplay(t, tunnelAdmissionWire(t, profile, 1, "198.51.100.1", false, depth, false))
				require.Len(t, events, 1)
				if depth == 4 {
					require.Equal(t, "dns", events[0].Protocol)
					require.Equal(t, "decoded", events[0].Status)
				} else {
					require.Equal(t, "limited", events[0].Status)
					require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
				}
			})
		}
	}
}

func TestProtocolGeneveProfileLimits(t *testing.T) {
	for _, control := range []struct {
		name   string
		offset int
		value  byte
	}{
		{"version-one", 0, 0x40},
		{"version-two", 0, 0x80},
		{"oam", 1, 0x80},
		{"critical", 1, 0x40},
		{"reserved-flags", 1, 1},
		{"reserved-vni", 7, 1},
	} {
		t.Run(control.name, func(t *testing.T) {
			raw := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
			// Synthetic Ethernet + fixed IPv4 + UDP: Geneve starts at 42.
			raw[42+control.offset] = control.value
			events := tunnelAdmissionReplay(t, raw)
			require.Len(t, events, 1)
			require.Equal(t, "limited", events[0].Status)
			require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
		})
	}
	t.Run("noncritical-option", func(t *testing.T) {
		raw := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
		// Insert a valid four-byte empty option after the Geneve header.
		raw = append(append(bytes.Clone(raw[:50]), 0, 0, 0, 0), raw[50:]...)
		raw[42] = 1
		binary.BigEndian.PutUint16(raw[16:18], uint16(len(raw)-14))
		binary.BigEndian.PutUint16(raw[38:40], uint16(len(raw)-34))
		events := tunnelAdmissionReplay(t, raw)
		require.Len(t, events, 1)
		require.Equal(t, "limited", events[0].Status)
		require.Equal(t, "EncapsulationDepthOrProfile", events[0].ExpertCode)
	})
}

func TestProtocolTunnelCaptureIdentity(t *testing.T) {
	raw := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
	p := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
	ci := gopacket.CaptureInfo{InterfaceIndex: 7, CaptureLength: len(raw), Length: len(raw)}
	ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: 42, Domain: CaptureDomain{Section: 3, Interface: 7}}})
	p.Metadata().CaptureInfo = ci
	a := &binParser{}
	normalized, consumed := a.networkPacket(p)
	require.False(t, consumed)
	require.Equal(t, "192.0.2.1", normalized.NetworkLayer().NetworkFlow().Src().String())
	want := PacketReference{Number: 42, Domain: CaptureDomain{Section: 3, Interface: 7, Encapsulation: "/geneve:198.51.100.1~198.51.100.2:5013"}}
	require.Equal(t, want, evidenceFrom(normalized.Metadata().CaptureInfo).Ref)
	require.Equal(t, want, packetEvidence(normalized).Ref)
}

func TestProtocolTunnelDomainOrder(t *testing.T) {
	raw := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
	// One outer VLAN tag; the innermost IP view must retain this prefix.
	raw = append(append(bytes.Clone(raw[:12]), 0x81, 0, 0, 9, 8, 0), raw[14:]...)
	domain := PacketDomain(raw, gopacket.CaptureInfo{}, layers.LinkTypeEthernet)
	require.Equal(t, "/vlan:9/geneve:198.51.100.1~198.51.100.2:5013", domain.Encapsulation)
	events := tunnelAdmissionReplay(t, raw)
	require.Len(t, events, 1)
	require.Equal(t, "dns", events[0].Protocol)
	require.Equal(t, domain, events[0].Domain)
}
