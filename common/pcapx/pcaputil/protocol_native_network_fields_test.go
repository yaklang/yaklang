package pcaputil

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func networkAliasValues(s DisplayFieldSnapshot) map[string]DisplayFieldValue {
	out := map[string]DisplayFieldValue{}
	for _, v := range s.Values {
		if strings.HasPrefix(v.Name, "arp.") || strings.HasPrefix(v.Name, "icmp.") || strings.HasPrefix(v.Name, "icmpv6.") {
			out[v.Name] = v
		}
	}
	return out
}

func networkAliasReplay(t *testing.T, frames ...[]byte) *ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, frames)), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	return events[0]
}

func TestNativeNetworkAliasesOwnHeaderRangesAndMetadata(t *testing.T) {
	e := networkAliasReplay(t, icmp4UnionFrame(t, 3, 4, [4]byte{0, 0, 5, 0xd4}, nil))
	s := e.DisplayFieldValues()
	v := networkAliasValues(s)
	require.Len(t, v, 3)
	require.Equal(t, uint64(1492), v["icmp.mtu"].Value)
	require.Equal(t, &DisplayFieldRange{Offset: 40, Length: 2}, v["icmp.mtu"].Range)
	require.Equal(t, uint64(3), v["icmp.type"].Value)
	require.NotContains(t, v, "icmp.ident")
	require.NotContains(t, v, "icmp.seq")
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var fake ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &fake))
	require.Empty(t, networkAliasValues(fake.DisplayFieldValues()), "public metadata cannot manufacture native header ownership")
	e.Protocol, e.Status, e.Completeness = "grpc", "decoded", "message"
	e.Source, e.Destination = "203.0.113.1", "203.0.113.2"
	e.ID = 999
	e.Length = 1
	e.SourceBytes.PacketRefs[0].Number = 999
	clear(e.Raw)
	e.Session = map[string]any{"Type": byte(8), "Identifier": uint16(0), "Sequence": uint16(0), "MTU": uint16(1)}
	require.Equal(t, s, e.DisplayFieldValues())
	v["icmp.mtu"].Range.Offset = 0
	s.Source.PacketRefs[0].Number = 0
	fresh := e.DisplayFieldValues()
	require.Equal(t, &DisplayFieldRange{Offset: 40, Length: 2}, networkAliasValues(fresh)["icmp.mtu"].Range)
	require.EqualValues(t, 1, fresh.Source.PacketRefs[0].Number)
}

func TestNativeNetworkPartialAliasesNeverDefaultMissingWords(t *testing.T) {
	for _, n := range []int{0, 1, 3, 4} {
		e := networkAliasReplay(t, icmp6BodyFrame(128, make([]byte, n)))
		v := networkAliasValues(e.DisplayFieldValues())
		if n < 4 {
			require.Len(t, v, 2)
			require.NotContains(t, v, "icmpv6.echo.identifier")
		} else {
			require.Len(t, v, 4)
			require.Equal(t, uint64(0), v["icmpv6.echo.identifier"].Value)
			require.Equal(t, &DisplayFieldRange{Offset: 58, Length: 2}, v["icmpv6.echo.identifier"].Range)
		}
		filter, err := CompileDisplayFilter("icmpv6.echo.identifier == 0")
		require.NoError(t, err)
		require.Equal(t, n == 4, filter.EvaluateEvents([]*ProtocolEvent{e}).Matched)
	}
	e := networkAliasReplay(t, icmp4UnionFrame(t, 14, 0, [4]byte{0x12, 0x34, 0, 7}, []byte{1, 2, 3, 4, 0x80}))
	s := e.DisplayFieldValues()
	v := networkAliasValues(s)
	require.Equal(t, uint64(0x01020304), v["icmp.originate_timestamp"].Value)
	require.Equal(t, &DisplayFieldRange{Offset: 42, Length: 4}, v["icmp.originate_timestamp"].Range)
	require.NotContains(t, v, "icmp.receive_timestamp")
	require.NotContains(t, v, "icmp.transmit_timestamp")
	encoded, err := json.Marshal(s)
	require.NoError(t, err)
	var round DisplayFieldSnapshot
	require.NoError(t, json.Unmarshal(encoded, &round))
	f, err := CompileDisplayFilter("icmp.originate_timestamp == 16909060 and icmp.ident == 4660")
	require.NoError(t, err)
	require.True(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{round}).Matched)
	for _, expr := range []string{"icmp.type == 256", "icmp.mtu == 65536", "icmp.pointer == 256", "icmp.originate_timestamp == 4294967296", "arp.src.proto_ipv4 == 2001:db8::1", "arp.src.proto_ipv4 in 2001:db8::/32", "arp.src.proto_ipv4 in ::ffff:192.0.2.1/80"} {
		_, err := CompileDisplayFilter(expr)
		require.Error(t, err, expr)
	}
}

func TestNativeNetworkIPv6MappedTargetAndReassembledRanges(t *testing.T) {
	body := make([]byte, 20)
	copy(body[4:], net.ParseIP("::ffff:192.0.2.1").To16())
	e := networkAliasReplay(t, icmp6BodyFrame(137, body))
	v := networkAliasValues(e.DisplayFieldValues())
	require.Equal(t, "::ffff:192.0.2.1", v["icmpv6.nd.rd.target_address"].Value)
	require.Equal(t, &DisplayFieldRange{Offset: 62, Length: 16}, v["icmpv6.nd.rd.target_address"].Range)
	require.NotContains(t, v, "icmpv6.rd.na.destination_address")
	f, err := CompileDisplayFilter("icmpv6.nd.rd.target_address == ::ffff:192.0.2.1")
	require.NoError(t, err)
	require.True(t, f.EvaluateEvents([]*ProtocolEvent{e}).Matched)
	frame := icmp4UnionFrame(t, 8, 0, [4]byte{0x12, 0x34, 0x56, 0x78}, make([]byte, 16))
	// Serialization may append Ethernet padding; it is not part of the
	// original IPv4 datagram supplied to the authored fragment builder.
	frame = frame[:14+(int(frame[16])<<8|int(frame[17]))]
	parts := icmpFragmentFrames(t, frame, 8, false)
	e = networkAliasReplay(t, parts...)
	require.Equal(t, "reassembled", e.SourceBytes.Kind)
	require.Equal(t, frame[14:], e.Raw)
	v = networkAliasValues(e.DisplayFieldValues())
	require.Equal(t, &DisplayFieldRange{Offset: 24, Length: 2}, v["icmp.ident"].Range, "PDU ranges must not invent an Ethernet offset in reassembled bytes")
}

func TestNativeARPAddressAliasesRequireDeclaredWidths(t *testing.T) {
	for _, tc := range []struct {
		hlen, plen byte
		proto      layers.EthernetType
	}{{6, 4, layers.EthernetTypeIPv4}, {4, 4, layers.EthernetTypeIPv4}, {6, 16, layers.EthernetTypeIPv4}, {6, 4, layers.EthernetTypeIPv6}} {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeARP}
		arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: tc.proto, HwAddressSize: tc.hlen, ProtAddressSize: tc.plen, Operation: 1, SourceHwAddress: bytes.Repeat([]byte{1}, int(tc.hlen)), DstHwAddress: make([]byte, tc.hlen), SourceProtAddress: bytes.Repeat([]byte{2}, int(tc.plen)), DstProtAddress: bytes.Repeat([]byte{3}, int(tc.plen))}
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true}, eth, arp))
		e := networkAliasReplay(t, bytes.Clone(b.Bytes()))
		v := networkAliasValues(e.DisplayFieldValues())
		require.Equal(t, uint64(1), v["arp.opcode"].Value)
		_, mac := v["arp.src.hw_mac"]
		require.Equal(t, tc.hlen == 6, mac)
		_, ip := v["arp.src.proto_ipv4"]
		require.Equal(t, tc.proto == layers.EthernetTypeIPv4 && tc.plen == 4, ip)
	}
}
