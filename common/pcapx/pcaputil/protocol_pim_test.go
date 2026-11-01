package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func pimControlFrame(t *testing.T, body []byte, v6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{1, 0, 0x5e, 0, 0, 13}, EthernetType: layers.EthernetTypeIPv4}
	var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 1, Protocol: 103, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("224.0.0.13")}
	context := stream_parser.PIMWireContext{Source: netip.MustParseAddr("192.0.2.1"), Destination: netip.MustParseAddr("224.0.0.13")}
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip = &layers.IPv6{Version: 6, HopLimit: 1, NextHeader: 103, SrcIP: net.ParseIP("fe80::1"), DstIP: net.ParseIP("ff02::d")}
		context = stream_parser.PIMWireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::d")}
	}
	w := bytes.Clone(body)
	w[2], w[3] = 0, 0
	binary.BigEndian.PutUint16(w[2:], stream_parser.PIMChecksum(w, context, len(w)))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(w)))
	return b.Bytes()
}

func pimReplay(t *testing.T, w []byte) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, c.prepareBinParser())
	p := gopacket.NewPacket(w, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, handled := c.binParser.networkPacket(p)
	require.True(t, handled)
	c.binParser.closeUDPSessions()
	return events
}

func TestPIMNativeProfileIsExplicitAndBounded(t *testing.T) {
	var profiles []ProtocolProfile
	for _, p := range NativeProtocolProfiles() {
		if p.Protocol == "pim" {
			profiles = append(profiles, p)
		}
	}
	require.Len(t, profiles, 1)
	require.Equal(t, "network", profiles[0].Transport)
	require.Contains(t, profiles[0].Framing, "PIMv2")
	require.Contains(t, profiles[0].Context, "no transport identity")
}

func TestPIMHelloOwnedFieldsChecksumsAndNoNeighborInheritance(t *testing.T) {
	hello := []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105, 0, 19, 0, 4, 0, 0, 0, 7, 0, 20, 0, 4, 1, 2, 3, 4}
	for _, v6 := range []bool{false, true} {
		w := pimControlFrame(t, hello, v6)
		es := pimReplay(t, w)
		require.Len(t, es, 1)
		e := es[0]
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Equal(t, "pim", e.Protocol)
		require.Zero(t, e.FlowID)
		require.Equal(t, true, e.Session["Checksum Verified"])
		require.Equal(t, []any{uint64(16909060)}, e.DisplayFields()["pim.generation_id"])
		w[len(w)-1] ^= 1
		require.Equal(t, "malformed", pimReplay(t, w)[0].Status)
		// Same sender, entirely new GenID and no DR priority: no old option
		// can be inherited. A Hello isn't a transport connection.
		second := []byte{0x20, 0, 0, 0, 0, 20, 0, 4, 5, 6, 7, 8}
		es = pimReplay(t, pimControlFrame(t, second, v6))
		require.Zero(t, es[0].FlowID)
		require.Empty(t, es[0].DisplayFields()["pim.dr_priority"])
	}
}

func TestPIMLimitKeepsObservedLengthWithoutRetainingBody(t *testing.T) {
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	a.budget.MaxFrameBytes = 8
	w := pimControlFrame(t, []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}, false)
	p := gopacket.NewPacket(w, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	_, handled := a.networkPacket(p)
	require.True(t, handled)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "limited", e.Status)
	require.Equal(t, 10, e.Length)
	require.Empty(t, e.Raw)
	require.Empty(t, e.Session)
	require.Equal(t, uint64(10), a.limited.Load())
	require.Equal(t, uint64(10), a.messageBytes.Load())
	filter, err := CompileDisplayFilter("not pim.dr_priority")
	require.NoError(t, err)
	require.False(t, filter.EvaluateEvents(events).Complete, "unretained body is not proved absence")
	a.closeUDPSessions()
}

func TestPIMJoinPruneNestedSourcesAndBudget(t *testing.T) {
	w := []byte{0x23, 0, 0, 0, 1, 0, 192, 0, 2, 2, 0, 1, 0, 210, 1, 0, 0, 32, 239, 1, 2, 3, 0, 1, 0, 1, 1, 0, 4, 32, 192, 0, 2, 10, 1, 0, 7, 32, 192, 0, 2, 11}
	m, err := stream_parser.DecodePIMControl(w, stream_parser.PIMWireContext{}, 32)
	require.NoError(t, err)
	g := m["Groups"].([]map[string]any)[0]
	require.Equal(t, "192.0.2.10", g["Joined Sources"].([]map[string]any)[0]["Address"])
	require.Equal(t, "192.0.2.11", g["Pruned Sources"].([]map[string]any)[0]["Address"])
	for cut := 4; cut < len(w); cut++ {
		_, err = stream_parser.DecodePIMControl(w[:cut], stream_parser.PIMWireContext{}, 32)
		require.Error(t, err, "cut %d", cut)
	}
	_, err = stream_parser.DecodePIMControl(w, stream_parser.PIMWireContext{}, 2)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	es := pimReplay(t, pimControlFrame(t, w, false))
	require.Equal(t, "decoded", es[0].Status)
	fields := es[0].DisplayFields()
	require.Equal(t, []any{"192.0.2.10"}, fields["pim.join_ip"])
	require.Equal(t, []any{"192.0.2.11"}, fields["pim.prune_ip"])
	require.Equal(t, []any{"239.1.2.3"}, fields["pim.group"])
	require.Equal(t, []any{true, true}, fields["pim.source_addr.flags.s"])
	// A source mask or unknown encoding cannot cause a scan for a plausible
	// later address. Unknown option values remain owned bytes, not options.
	bad := bytes.Clone(w)
	bad[5] = 9
	_, err = stream_parser.DecodePIMControl(bad, stream_parser.PIMWireContext{}, 32)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireContext)
}

func TestPIMNullRegisterShowsAdvertisedAddressesWithoutTransport(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		es := pimReplay(t, pimTestFrame(t, v6, true, false))
		require.Len(t, es, 1)
		e := es[0]
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Zero(t, e.FlowID)
		m := e.Session["Register IP Header"].(map[string]any)
		require.Equal(t, true, m["Dummy Header"])
		require.NotEqual(t, m["Source"], e.Source)
		require.Contains(t, m["Observation"], "not an inner transport packet")
		before, err := e.GetFields()
		require.NoError(t, err)
		e.Session["Register IP Header"].(map[string]any)["Source"] = "mutated"
		after, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func FuzzPIMControlEnvelope(f *testing.F) {
	for _, w := range [][]byte{{0x20, 0, 0, 0}, {0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}, {0x22, 0, 0, 0, 1, 0, 0, 32, 239, 1, 2, 3, 1, 0, 192, 0, 2, 1}} {
		f.Add(w, uint8(0))
	}
	f.Fuzz(func(t *testing.T, w []byte, mode uint8) {
		if len(w) > 65540 {
			return
		}
		context := stream_parser.PIMWireContext{}
		if mode&1 != 0 {
			context = stream_parser.PIMWireContext{Source: netip.MustParseAddr("192.0.2.1"), Destination: netip.MustParseAddr("224.0.0.13")}
		}
		if mode&2 != 0 {
			context = stream_parser.PIMWireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::d")}
		}
		original := bytes.Clone(w)
		m, err := stream_parser.DecodePIMControl(w, context, 32)
		require.Equal(t, original, w)
		if err == nil {
			require.Equal(t, uint64(2), m["Version"])
			require.NotEqual(t, "Unknown", m["Packet Name"])
			require.Equal(t, context.Source.IsValid(), m["Checksum Verified"])
		}
	})
}

func TestPIMSelectedControlMessageBoundariesAndAdvertisements(t *testing.T) {
	group := []byte{1, 0, 0, 32, 239, 1, 2, 3}
	source := []byte{1, 0, 192, 0, 2, 1}
	join := []byte{1, 0, 192, 0, 2, 2, 0, 1, 0, 210}
	join = append(join, group...)
	join = append(join, 0, 1, 0, 0, 1, 0, 4, 32, 192, 0, 2, 10)
	bootstrap := append([]byte{0, 1, 30, 64}, source...)
	bootstrap = append(bootstrap, group...)
	bootstrap = append(bootstrap, 2, 1, 0, 0)
	bootstrap = append(bootstrap, source...)
	bootstrap = append(bootstrap, 0, 150, 7, 0)
	assert := append(bytes.Clone(group), source...)
	assert = append(assert, 0x80, 0, 0, 10, 0, 0, 0, 123)
	candidate := append([]byte{1, 7, 0, 150}, source...)
	candidate = append(candidate, group...)
	refresh := append(bytes.Clone(group), source...)
	refresh = append(refresh, source...)
	refresh = append(refresh, 0, 0, 0, 10, 0, 0, 0, 123, 32, 1, 0xe0, 60)
	for _, tc := range []struct {
		typ  byte
		body []byte
	}{
		{2, append(bytes.Clone(group), source...)}, {3, join}, {4, bootstrap}, {5, assert}, {6, join}, {7, join}, {8, candidate}, {9, refresh},
	} {
		w := append([]byte{0x20 | tc.typ, 0, 0, 0}, tc.body...)
		m, err := stream_parser.DecodePIMControl(w, stream_parser.PIMWireContext{}, 64)
		require.NoError(t, err, "type %d", tc.typ)
		require.Equal(t, uint64(tc.typ), m["Type"])
		for cut := 0; cut < len(w); cut++ {
			if tc.typ == 4 && cut == 14 {
				continue
			} // complete BSR header with no group records
			_, err = stream_parser.DecodePIMControl(w[:cut], stream_parser.PIMWireContext{}, 64)
			require.Error(t, err, "type %d cut %d", tc.typ, cut)
		}
		for _, v6 := range []bool{false, true} {
			e := pimReplay(t, pimControlFrame(t, w, v6))[0]
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Zero(t, e.FlowID)
		}
		if tc.typ == 4 {
			require.Equal(t, false, m["Groups"].([]map[string]any)[0]["RP Set Complete In Message"])
			require.Contains(t, m["Bootstrap Reassembly"], "not attempted")
		}
		if tc.typ == 5 {
			require.Equal(t, uint64(10), m["Metric Preference"])
			require.Equal(t, true, m["RPT"])
		}
		if tc.typ == 9 {
			require.Equal(t, true, m["Prune Indicator"])
			require.Equal(t, uint64(60), m["Interval"])
		}
	}
}
