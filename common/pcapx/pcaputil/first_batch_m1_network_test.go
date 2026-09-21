package pcaputil

import (
	"bytes"
	"encoding/binary"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

func m1DNSQuery() []byte {
	return []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0, 0, 1, 0, 1}
}
func m1Serialize(t *testing.T, ls ...gopacket.SerializableLayer) []byte {
	t.Helper()
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ls...))
	return bytes.Clone(b.Bytes())
}
func m1UDP(t *testing.T, w []byte) []byte {
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2), Protocol: layers.IPProtocolUDP}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	return m1Serialize(t, ip, udp, gopacket.Payload(w))
}
func m1Feed(t *testing.T, a *PacketAnalyzer, b []byte, iface int) {
	t.Helper()
	require.NoError(t, a.Feed(b, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(b), Length: len(b), InterfaceIndex: iface}, layers.LinkTypeRaw))
}
func TestFirstBatchT02(t *testing.T) {
	t.Run("ipv4-fragments-domain-and-overlap", func(t *testing.T) {
		var events []*ProtocolEvent
		var stats ProtocolStats
		a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
		require.NoError(t, err)
		raw := m1UDP(t, m1DNSQuery())
		payload := raw[20:]
		frag := func(off int, more bool, b []byte) []byte {
			flags := layers.IPv4Flag(0)
			if more {
				flags = layers.IPv4MoreFragments
			}
			return m1Serialize(t, &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Id: 9, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2), Protocol: layers.IPProtocolUDP, Flags: flags, FragOffset: uint16(off / 8)}, gopacket.Payload(b))
		}
		m1Feed(t, a, frag(16, false, payload[16:]), 1)
		m1Feed(t, a, frag(0, true, payload[:16]), 2)
		require.Empty(t, events)
		m1Feed(t, a, frag(0, true, payload[:16]), 1)
		require.Len(t, events, 1)
		require.Equal(t, "dns", events[0].Protocol)
		require.Equal(t, 1, events[0].Domain.Interface)
		require.Len(t, events[0].SourceBytes.PacketRefs, 2)
		m1Feed(t, a, frag(8, false, payload[8:]), 2)
		require.Equal(t, "FragmentOverlapRejected", events[1].ExpertCode)
		require.NoError(t, a.Close())
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("mixed-pcapng-interface-sections", func(t *testing.T) {
		var b bytes.Buffer
		w, err := pcapgo.NewNgWriter(&b, layers.LinkTypeRaw)
		require.NoError(t, err)
		id, err := w.AddInterface(pcapgo.NgInterface{LinkType: layers.LinkTypeEthernet, SnapLength: 65535})
		require.NoError(t, err)
		raw := m1UDP(t, m1DNSQuery())
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw)}, raw))
		eth := append([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 8, 0}, raw...)
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(2, 0), CaptureLength: len(eth), Length: len(eth), InterfaceIndex: id}, eth))
		require.NoError(t, w.Flush())
		r, err := NewCaptureReader(bytes.NewReader(append(bytes.Clone(b.Bytes()), b.Bytes()...)))
		require.NoError(t, err)
		for i := 0; i < 4; i++ {
			_, ci, err := r.ReadPacketData()
			require.NoError(t, err)
			require.Equal(t, uint32(i/2), evidenceFrom(ci).Ref.Domain.Section)
			require.Equal(t, i%2, ci.InterfaceIndex)
		}
		_, _, err = r.ReadPacketData()
		require.ErrorIs(t, err, io.EOF)
	})
}
func TestFirstBatchT07Native(t *testing.T) {
	for _, file := range []string{"ndpi/ndpi-dns.pcap", "wireshark-tests/wireshark-mdns.pcap"} {
		t.Run(file, func(t *testing.T) {
			b, err := os.ReadFile("../../bin-parser/testdata/protocol-corpus/captures/" + file)
			require.NoError(t, err)
			events, stats, err := binReplay(t, b, 2)
			require.NoError(t, err)
			require.Zero(t, stats.BufferedBytes)
			count := 0
			for _, e := range events {
				if e.Protocol == "dns" || e.Protocol == "mdns" {
					require.Empty(t, e.Error)
					require.NotNil(t, e.Session["DNS"])
					require.NotEmpty(t, e.SourceBytes.PacketRefs)
					count++
				}
			}
			require.Positive(t, count)
		})
	}
	t.Run("compression-window", func(t *testing.T) {
		q := m1DNSQuery()
		binary.BigEndian.PutUint16(q[2:], 0x8180)
		binary.BigEndian.PutUint16(q[6:], 1)
		q = append(q, 0xc0, 12, 0, 5, 0, 1, 0, 0, 0, 30, 0, 1, 0xc0, 12)
		_, err := DecodeDNSMessage(q, 10)
		require.Error(t, err)
	})
}
func TestFirstBatchT08(t *testing.T) {
	for _, file := range []string{"wireshark-dhcp.pcap", "wireshark-icmp-ascii.pcapng"} {
		t.Run(file, func(t *testing.T) {
			b, err := os.ReadFile("../../bin-parser/testdata/protocol-corpus/captures/wireshark-tests/" + file)
			require.NoError(t, err)
			events, stats, err := binReplay(t, b, 1)
			require.NoError(t, err)
			require.Zero(t, stats.BufferedBytes)
			require.NotEmpty(t, events)
			for _, e := range events {
				require.Empty(t, e.Error)
			}
			if file == "wireshark-dhcp.pcap" {
				require.Len(t, events, 4)
				require.Equal(t, "Discover", events[0].Session["Packet Name"])
				require.Equal(t, "Ack", events[3].Session["Packet Name"])
				require.NotZero(t, events[3].ResponseTo)
				require.Equal(t, "unverified-lease", events[3].Session["Observation"])
			}
		})
	}
}

func TestFirstBatchT08DHCP6Native(t *testing.T) {
	b, err := os.ReadFile("testdata/protocol-sessions/first-batch-m1/dhcpv6-native.pcap")
	require.NoError(t, err)
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			events, stats, err := binReplay(t, b, workers, WithProtocolDeferred(deferred))
			require.NoError(t, err)
			require.Zero(t, stats.BufferedBytes)
			var types []byte
			paired, nd := 0, 0
			for _, e := range events {
				require.Empty(t, e.Error)
				if e.Protocol == "dhcpv6" {
					types = append(types, e.Session["Message Type"].(byte))
					if e.ResponseTo != 0 {
						paired++
					}
				}
				if e.Protocol == "icmpv6" && e.Session["Target"] != nil {
					nd++
				}
			}
			require.Equal(t, []byte{1, 2, 3, 7, 8, 7}, types)
			require.Equal(t, 3, paired)
			require.Equal(t, 4, nd)
		}
	}
}

func TestFirstBatchT03NativeDecodeAs(t *testing.T) {
	var events []*ProtocolEvent
	a, err := NewPacketAnalyzer(WithProtocolDecodeAs("udp", 4053, "dns"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	raw := m1UDP(t, m1DNSQuery())
	binary.BigEndian.PutUint16(raw[22:24], 4053)
	m1Feed(t, a, raw, 0)
	require.NoError(t, a.Close())
	require.Len(t, events, 1)
	require.Equal(t, "dns", events[0].Protocol)
	require.Equal(t, "explicit-decode-as", events[0].Admission)
	_, err = NewPacketAnalyzer(WithProtocolDecodeAs("udp", 4053, "dns"), WithProtocolDecodeAs("udp", 4053, "dhcp"))
	require.Error(t, err)
	_, err = NewPacketAnalyzer(WithBPFFilter("tcp"), WithOnProtocolMessage(func(*ProtocolEvent) {}))
	require.Error(t, err)
}
func TestFirstBatchT01NameRecords(t *testing.T) {
	var initial bytes.Buffer
	w, err := pcapgo.NewNgWriter(&initial, layers.LinkTypeRaw)
	require.NoError(t, err)
	require.NoError(t, w.Flush())
	for _, valid := range []bool{false, true} {
		name := []byte{192, 0, 2, 1, 'a', 0}
		if !valid {
			name[5] = 'b'
		}
		block := make([]byte, 28)
		binary.LittleEndian.PutUint32(block, 4)
		binary.LittleEndian.PutUint32(block[4:], 28)
		binary.LittleEndian.PutUint16(block[8:], 1)
		binary.LittleEndian.PutUint16(block[10:], uint16(len(name)))
		copy(block[12:], name)
		binary.LittleEndian.PutUint32(block[24:], 28)
		_, err := NewCaptureReader(bytes.NewReader(append(bytes.Clone(initial.Bytes()), block...)))
		if valid {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "bounded name list")
		}
	}
}
func TestFirstBatchT02IPv6AndTunnel(t *testing.T) {
	for _, mode := range []string{"ipv6-fragment", "gre", "vlan"} {
		t.Run(mode, func(t *testing.T) {
			var events []*ProtocolEvent
			var stats ProtocolStats
			a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
			require.NoError(t, err)
			raw := m1UDP(t, m1DNSQuery())
			switch mode {
			case "ipv6-fragment":
				ip := &layers.IPv6{Version: 6, HopLimit: 64, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2"), NextHeader: layers.IPProtocolIPv6Fragment}
				payload := raw[20:]
				for _, part := range []struct {
					off  int
					more bool
					b    []byte
				}{{16, false, payload[16:]}, {0, true, payload[:16]}} {
					hdr := []byte{17, 0, 0, 0, 0, 0, 0, 9}
					v := uint16(part.off)
					if part.more {
						v |= 1
					}
					binary.BigEndian.PutUint16(hdr[2:], v)
					m1Feed(t, a, m1Serialize(t, ip, gopacket.Payload(append(hdr, part.b...))), 0)
				}
			case "gre":
				outer := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2), Protocol: layers.IPProtocolGRE}
				m1Feed(t, a, m1Serialize(t, outer, gopacket.Payload(append([]byte{0, 0, 8, 0}, raw...))), 0)
			case "vlan":
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{0, 1, 2, 3, 4, 6}, EthernetType: layers.EthernetTypeDot1Q}
				b := m1Serialize(t, eth, &layers.Dot1Q{VLANIdentifier: 42, Type: layers.EthernetTypeIPv4}, gopacket.Payload(raw))
				require.NoError(t, a.Feed(b, gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(b), Length: len(b)}, layers.LinkTypeEthernet))
			}
			require.NoError(t, a.Close())
			require.Zero(t, stats.BufferedBytes)
			require.Len(t, events, 1)
			require.Equal(t, "dns", events[0].Protocol)
			require.Empty(t, events[0].Error)
			if mode == "ipv6-fragment" {
				require.Len(t, events[0].SourceBytes.PacketRefs, 2)
			} else {
				require.NotEmpty(t, events[0].Domain.Encapsulation)
			}
		})
	}
}
