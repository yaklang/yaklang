package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func versionAdmissionWire(t *testing.T, family int) []byte {
	t.Helper()
	if family == 4 {
		return tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 0, false)
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, &layers.DNS{ID: 1234, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("version.test"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}))
	return bytes.Clone(b.Bytes())
}

func TestInvalidIPVersionCannotAdmitDatagramsOrTunnelContext(t *testing.T) {
	for _, tc := range []struct {
		name, profile, domain string
		family, at, depth     int
	}{
		{"direct-v4", "", "", 4, 14, 0},
		{"direct-v6", "", "", 6, 14, 0},
		{"bad-IPIP-outer", "ipip", "", 4, 14, 1},
		{"bad-IPIP-inner", "ipip", "/ipip:198.51.100.1~198.51.100.2", 4, 34, 1},
		{"bad-6in4-inner", "6in4", "/6in4:198.51.100.1~198.51.100.2", 6, 34, 1},
		{"bad-Geneve-inner", "geneve", "/geneve:198.51.100.1~198.51.100.2:5013", 4, 64, 1},
	} {
		for version := 0; version < 16; version++ {
			if version == tc.family {
				continue
			}
			t.Run(fmt.Sprintf("%s/version%d", tc.name, version), func(t *testing.T) {
				raw := versionAdmissionWire(t, tc.family)
				if tc.profile != "" {
					raw = tunnelAdmissionWire(t, tc.profile, 5013, "198.51.100.1", false, tc.depth, false)
				}
				raw[tc.at] = byte(version<<4) | raw[tc.at]&15
				var events []*ProtocolEvent
				var mu sync.Mutex
				err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{raw})), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					events = append(events, e)
				}))
				require.ErrorContains(t, err, fmt.Sprintf("invalid IPv%d version %d", tc.family, version), "the existing capture decoder error contract stays observable")
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "ip", e.Protocol)
				require.Equal(t, "malformed", e.Status)
				require.Equal(t, "IPVersionMismatch", e.ExpertCode)
				require.Equal(t, tc.domain, e.Domain.Encapsulation, "the invalid IP layer cannot create its own tunnel identity")
				require.Empty(t, e.Source)
				require.Empty(t, e.Destination)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Equal(t, uint8(version), e.Session["Observed IP Version"])
				require.Equal(t, uint8(tc.family), e.Session["Expected IP Version"])
				require.Equal(t, []PacketReference{{Number: 1, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
			})
		}
	}
}

func TestInvalidIPVersionCannotExpireOrQuarantineValidFragments(t *testing.T) {
	for _, name := range []string{"ipv4-options", "ipv6-in-order"} {
		t.Run(name, func(t *testing.T) {
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 16)
			t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
			original := coreIPPackets(t, name)
			domain := CaptureDomain{Interface: 17}
			_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
			require.True(t, consumed)
			buffered := a.buffered.Load()
			raw := bytes.Clone(original[1].Data())
			ip := original[1].NetworkLayer()
			at := len(raw) - len(ip.LayerContents()) - len(ip.LayerPayload())
			raw[at] = (raw[at] & 15) | 0xf0
			bad := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: original[1].Layers()[0].LayerType()}, gopacket.Default)
			bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), Length: len(raw), CaptureLength: len(raw)}, captureEvidence{Ref: PacketReference{Number: 2, Domain: domain}})
			_, consumed = a.networkPacket(bad)
			require.True(t, consumed)
			require.Len(t, events, 1)
			require.Equal(t, "IPVersionMismatch", events[0].ExpertCode)
			require.Equal(t, buffered, a.buffered.Load(), "invalid headers cannot advance fragment clocks or consume a tombstone")
			require.Len(t, a.fragments.sets, 1)
			require.False(t, a.fragments.blocked)
			q, consumed := a.networkPacket(fragmentClockPacket(original[1], domain, 3, time.Unix(101, 0), false))
			require.False(t, consumed)
			require.NotNil(t, q)
			require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(q).refs)
		})
	}
}

func TestInvalidIPVersionCannotCreateTCPOnlyTunnelFlow(t *testing.T) {
	for _, protocols := range []bool{false, true} {
		for _, badOuter := range []bool{false, true} {
			t.Run(fmt.Sprintf("protocols%t/outer%t", protocols, badOuter), func(t *testing.T) {
				raw := tunnelAdmissionWire(t, "ipip", 5013, "198.51.100.1", false, 1, true)
				at := 34
				if badOuter {
					at = 14
				}
				raw[at] = raw[at]&15 | 0x60
				var mu sync.Mutex
				flows, observations := 0, 0
				var stats TCPReassemblyStats
				opts := []CaptureOption{WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowCreated(func(*TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows++ }), WithEveryPacket(func(gopacket.Packet) { mu.Lock(); defer mu.Unlock(); observations++ })}
				if protocols {
					opts = append(opts, WithOnProtocolMessage(func(*ProtocolEvent) {}))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{raw})), opts...), "AllowIncomplete preserves its nonfatal capture policy")
				require.EqualValues(t, 1, stats.DecodeErrors)
				require.Equal(t, 1, observations, "the invalid original frame remains observable")
				require.Zero(t, flows, "tunnel normalization cannot hide an invalid outer version from TCP admission")
			})
		}
	}
}

func FuzzInvalidIPVersionCannotMutateFragmentState(f *testing.F) {
	f.Add(uint8(4), uint8(6), int64(10000), []byte{0xff, 0, 0xff})
	f.Add(uint8(6), uint8(4), int64(0), []byte{0, 0xff, 1})
	f.Add(uint8(4), uint8(0), int64(1<<50), []byte{1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, family, version uint8, timestamp int64, damaged []byte) {
		if len(damaged) > 128 {
			return
		}
		family = 4 + family&2
		version &= 15
		if version == family {
			return
		}
		var events []*ProtocolEvent
		a := coreFragmentParser(&events, 1<<20, 16)
		t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
		original := coreIPPackets(t, "ipv4-options")
		domain := CaptureDomain{Interface: 17}
		_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 1, time.Unix(100, 0), false))
		require.True(t, consumed)
		buffered := a.buffered.Load()
		raw := versionAdmissionWire(t, int(family))
		for i, b := range damaged {
			if 15+i < len(raw) {
				raw[15+i] = b
			}
		}
		raw[14] = version<<4 | raw[14]&15
		bad := gopacket.NewPacket(raw, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
		bad.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(timestamp, 0), CaptureLength: len(raw), Length: len(raw)}, captureEvidence{Ref: PacketReference{Number: 2, Domain: domain}})
		_, consumed = a.networkPacket(bad)
		require.True(t, consumed)
		require.Len(t, events, 1)
		require.Equal(t, "IPVersionMismatch", events[0].ExpertCode)
		require.Equal(t, version, events[0].Session["Observed IP Version"])
		require.Equal(t, family, events[0].Session["Expected IP Version"])
		require.Equal(t, buffered, a.buffered.Load())
		require.Len(t, a.fragments.sets, 1)
		require.False(t, a.fragments.blocked)
		require.Nil(t, a.udpSessions)
		q, consumed := a.networkPacket(fragmentClockPacket(original[1], domain, 3, time.Unix(101, 0), false))
		require.False(t, consumed)
		require.NotNil(t, q)
		require.ElementsMatch(t, []PacketReference{{Number: 1, Domain: domain}, {Number: 3, Domain: domain}}, packetEvidence(q).refs)
	})
}

func TestUnselectedCarrierCannotConfirmAChildIPVersion(t *testing.T) {
	for _, profile := range []string{"EtherIP-version0", "EtherIP-version3", "IPv4-HOPOPT"} {
		t.Run(profile, func(t *testing.T) {
			inner := versionAdmissionWire(t, 4)
			inner[14] = 0x65
			protocol := layers.IPProtocol(97)
			payload := append([]byte{0, 0}, inner...)
			if profile == "EtherIP-version3" {
				payload[0] = 0x30
			} else if profile == "IPv4-HOPOPT" {
				protocol = 0
				payload = append([]byte{4, 0, 0, 0, 0, 0, 0, 0}, inner[14:]...)
			}
			eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
			ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: protocol, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
			wire := gopacket.NewSerializeBuffer()
			require.NoError(t, gopacket.SerializeLayers(wire, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(payload)))
			var events []*ProtocolEvent
			a := coreFragmentParser(&events, 1<<20, 16)
			t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
			original := coreIPPackets(t, "ipv4-options")
			domain := CaptureDomain{Interface: 17}
			_, consumed := a.networkPacket(fragmentClockPacket(original[0], domain, 10, time.Unix(100, 0), false))
			require.True(t, consumed)
			buffered := a.buffered.Load()
			p := gopacket.NewPacket(wire.Bytes(), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
			p.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(10000, 0), CaptureLength: len(wire.Bytes()), Length: len(wire.Bytes())}, captureEvidence{Ref: PacketReference{Number: 1, Domain: CaptureDomain{Interface: 17}}})
			_, consumed = a.networkPacket(p)
			require.True(t, consumed)
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, "network-encapsulation-refusal", e.Profile)
			require.Equal(t, "limited", e.Status)
			require.Equal(t, "EncapsulationDepthOrProfile", e.ExpertCode)
			require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
			require.Equal(t, []string{"Ethernet", "IPv4"}, e.Session["Observed Header Layers"])
			require.NotContains(t, e.Session, "Observed IP Version")
			require.Equal(t, CaptureDomain{Interface: 17}, e.Domain)
			require.Equal(t, []PacketReference{{Number: 1, Domain: e.Domain}}, e.SourceBytes.PacketRefs)
			require.Zero(t, e.FlowID)
			require.Nil(t, a.udpSessions)
			require.Len(t, a.fragments.sets, 1)
			require.Equal(t, buffered, a.buffered.Load(), "the unselected child cannot advance the domain's fragment clock")
			q, consumed := a.networkPacket(fragmentClockPacket(original[1], domain, 11, time.Unix(101, 0), false))
			require.False(t, consumed)
			require.NotNil(t, q)
		})
	}
}
