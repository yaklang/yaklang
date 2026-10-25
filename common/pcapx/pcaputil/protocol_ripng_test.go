package pcaputil

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func ripngNativeEntry(address string, prefix, metric byte) []byte {
	return append(append([]byte(nil), net.ParseIP(address).To16()...), 0, 0, prefix, metric)
}

func ripngNativePacket(t *testing.T, w []byte, ipv6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{0x33, 0x33, 0, 0, 0, 9}, EthernetType: layers.EthernetTypeIPv6}
	u := &layers.UDP{SrcPort: 521, DstPort: 521}
	var network gopacket.NetworkLayer
	if ipv6 {
		network = &layers.IPv6{Version: 6, HopLimit: 255, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("fe80::1"), DstIP: net.ParseIP("ff02::9")}
	} else {
		eth.EthernetType = layers.EthernetTypeIPv4
		network = &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	}
	require.NoError(t, u.SetNetworkLayerForChecksum(network))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, network.(gopacket.SerializableLayer), u, gopacket.Payload(w)))
	return bytes.Clone(b.Bytes())
}

func TestRIPngNativeCompleteIPv6AndNoNextHopInheritance(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		var es []*ProtocolEvent
		a, err := NewPacketAnalyzer(WithCaptureReplayClock(), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) {
			if e.Protocol == "ripng" {
				es = append(es, e)
			}
		}))
		require.NoError(t, err)
		feed := func(w []byte, v6 bool, iface int) {
			raw := ripngNativePacket(t, w, v6)
			require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(7, 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, layers.LinkTypeEthernet))
			clear(raw)
		}
		route := ripngNativeEntry("2001:db8:1::", 64, 2)
		first := append([]byte{2, 1, 0, 0}, ripngNativeEntry("fe80::9", 0, 255)...)
		first = append(first, route...)
		feed(first, true, 0)
		feed([]byte{2, 1, 0, 0}, true, 0) // insufficient identity evidence
		feed(append([]byte{2, 1, 0, 0}, route...), false, 0)
		bad := append([]byte{2, 1, 0, 0}, route...)
		bad[len(bad)-1] = 17
		feed(bad, true, 0)
		// Missing, duplicate and reordered advertisements have no cross-message
		// route/next-hop state. An equal tuple in another domain is independent.
		for _, iface := range []int{0, 1, 0} {
			feed(append([]byte{2, 1, 0, 0}, route...), true, iface)
		}
		require.NoError(t, a.Close())
		require.Len(t, es, 4)
		for _, e := range es {
			require.Contains(t, []string{"decoded", "deferred"}, e.Status)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, "RIPngFields", e.Entry)
			m, err := e.GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 2, m["Command"])
		}
		rows := es[0].Session["Entries"].([]map[string]any)
		require.Equal(t, "fe80::9", rows[1]["Effective Next Hop Advisory"])
		for _, e := range es[1:] {
			require.Equal(t, "originator", e.Session["Entries"].([]map[string]any)[0]["Effective Next Hop Advisory"])
		}
		filter, err := CompileDisplayFilter("ripng.cmd == 2 and ripng.rte.metric == 255 and ripng.rte.ipv6_prefix == fe80::9")
		require.NoError(t, err)
		require.True(t, filter.EvaluateEvents([]*ProtocolEvent{es[0]}).Matched)
		rows[0]["IPv6 Prefix"] = "changed"
		require.True(t, filter.EvaluateEvents([]*ProtocolEvent{es[0]}).Matched, "public edits cannot replace owned semantic fields")
	}
}

func TestRIPngExplicitSelectionDoesNotCreateIPv6OrAcceptBrokenEntries(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		var es []*ProtocolEvent
		a, err := NewPacketAnalyzer(WithProtocolDecodeAs("udp", 521, "ripng"), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }))
		require.NoError(t, err)
		w := append([]byte{2, 1, 0, 0}, ripngNativeEntry("2001:db8::", 64, 0)...)
		raw := ripngNativePacket(t, w, v6)
		require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
		require.NoError(t, a.Close())
		require.Len(t, es, 1)
		require.Equal(t, "ripng", es[0].Protocol)
		require.NotEqual(t, "decoded", es[0].Status)
		require.Empty(t, es[0].Session)
		negative, err := CompileDisplayFilter("not ripng.rte.metric")
		require.NoError(t, err)
		require.False(t, negative.EvaluateEvents(es).Complete)
	}
}

func TestRIPngLiteralMappedIPv6FieldKeepsItsWireFamily(t *testing.T) {
	e := &ProtocolEvent{Protocol: "ripng", Status: "decoded", Session: map[string]any{"Entries": []map[string]any{{"IPv6 Prefix": "::ffff:192.0.2.1"}}}}
	values := e.DisplayFieldValues().FieldMap()["ripng.rte.ipv6_prefix"]
	require.Equal(t, []any{"::ffff:192.0.2.1"}, values)
	for _, query := range []string{"ripng.rte.ipv6_prefix == ::ffff:192.0.2.1", "ripng.rte.ipv6_prefix in ::/0", "ripng.rte.ipv6_prefix in ::ffff:192.0.2.0/120"} {
		f, err := CompileDisplayFilter(query)
		require.NoError(t, err)
		require.True(t, f.EvaluateEvents([]*ProtocolEvent{e}).Matched, query)
	}
	for _, query := range []string{"ripng.rte.ipv6_prefix == 192.0.2.1", "ripng.rte.ipv6_prefix in 192.0.2.0/24"} {
		_, err := CompileDisplayFilter(query)
		require.Error(t, err)
	}
}
