package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
)

func espNATTFrame(t *testing.T, wire []byte, v6 bool, src, dst uint16) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	udp := &layers.UDP{SrcPort: layers.UDPPort(src), DstPort: layers.UDPPort(dst)}
	v4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	var ip gopacket.SerializableLayer = v4
	require.NoError(t, udp.SetNetworkLayerForChecksum(v4))
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		v6ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
		ip = v6ip
		require.NoError(t, udp.SetNetworkLayerForChecksum(v6ip))
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(wire)))
	return bytes.Clone(b.Bytes())
}

func espNATTReplay(t *testing.T, frames [][]byte, opts ...CaptureOption) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, frames)), opts...))
	return events, stats
}

func TestESPNATTIndependentDatagramsAcrossIKEKeepaliveReorderAndReuse(t *testing.T) {
	w := []byte{0xf4, 0x87, 0xbf, 0xcf, 0, 0, 0, 8, 1, 2, 3, 4}
	ike := make([]byte, 32)
	ike[11], ike[21], ike[22], ike[23] = 1, 32, 34, 8
	binary.BigEndian.PutUint32(ike[28:], 28)
	for _, v6 := range []bool{false, true} {
		t.Run(fmt.Sprint(v6), func(t *testing.T) {
			var frames [][]byte
			for _, p := range [][]byte{w, ike, {0xff}, w[:7], bytes.Repeat([]byte{0}, 12), w} {
				frames = append(frames, espNATTFrame(t, p, v6, 4500, 4500))
			}
			// A lower sequence, changed SPI and duplicate are independent wire
			// observations, not replay/SA verdicts or a new captured connection.
			changed := bytes.Clone(w)
			changed[0], changed[7] = 0x87, 2
			frames = append(frames, espNATTFrame(t, changed, v6, 4500, 4500))
			events, stats := espNATTReplay(t, frames)
			require.Len(t, events, len(frames))
			for i, e := range events {
				if i != 0 && i != 5 && i != 6 {
					require.NotEqual(t, "esp", e.Protocol, "datagram %d must supply its own evidence", i)
					require.NotContains(t, e.DisplayFields(), "esp.spi")
					continue
				}
				require.Equal(t, "esp", e.Protocol)
				require.Equal(t, "udp", e.Transport)
				require.Equal(t, "esp-natt-observed-header", e.Profile)
				require.Equal(t, "observed-UDP4500-nonzero-SPI", e.Admission)
				require.Equal(t, "limited", e.Status)
				require.Equal(t, "headers", e.Completeness)
				require.Empty(t, e.Error)
				require.Empty(t, e.ExpertCode)
				require.Zero(t, e.FlowID)
				require.Zero(t, e.TCPFlowID)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				expected := w
				if i == 6 {
					expected = changed
				}
				require.Equal(t, expected, e.Raw)
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, binary.BigEndian.Uint32(expected[:4]), fields["SPI"])
				require.Equal(t, binary.BigEndian.Uint32(expected[4:8]), fields["Sequence"])
				require.Equal(t, expected[8:], fields["Opaque Body"])
				require.Equal(t, false, fields["Authentication Verified"])
				snapshot := e.DisplayFieldValues()
				require.False(t, snapshot.Incomplete)
				require.Nil(t, snapshot.Source.CompletedIn)
				require.Len(t, snapshot.Source.PacketRefs, 1)
				require.Equal(t, uint64(i+1), snapshot.Source.PacketRefs[0].Number)
				require.Equal(t, []any{uint64(binary.BigEndian.Uint32(expected[:4]))}, snapshot.FieldMap()["esp.spi"])
				for _, f := range snapshot.Values {
					if f.Name == "esp.sequence" {
						require.Equal(t, &DisplayFieldRange{Offset: 4, Length: 4}, f.Range)
					}
				}
				e.Session["SPI"] = uint32(7)
				e.Session["Wire Field Ranges"] = nil
				e.Fields = e.Session
				again, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, fields, again, "public mutations cannot change source-bound ESP facts")
				require.Equal(t, snapshot.Values, e.DisplayFieldValues().Values)
			}
			require.Equal(t, "ike", events[1].Protocol)
			require.Equal(t, "decoded", events[1].Status)
			require.Zero(t, stats.Flows)
			require.Zero(t, stats.CallbackPanics)
		})
	}
}

func TestESPNATTParentAndExplicitSelectionCannotInventClearHeaders(t *testing.T) {
	w := []byte{0xf4, 0x87, 0xbf, 0xcf, 0, 0, 0, 0, 1, 2, 3, 4}
	for _, v6 := range []bool{false, true} {
		frame := espNATTFrame(t, w, v6, 42000, 55001)
		events, _ := espNATTReplay(t, [][]byte{frame})
		require.NotEqual(t, "esp", events[0].Protocol)
		events, _ = espNATTReplay(t, [][]byte{frame}, WithProtocolDecodeAs("udp", 55001, "esp-natt"))
		require.Equal(t, "esp", events[0].Protocol)
		require.Equal(t, "explicit-decode-as", events[0].Admission)
		require.Equal(t, uint32(0), events[0].Session["Sequence"], "the observed low word alone cannot prove a replay violation")
		for cut := 0; cut < 12; cut++ {
			events, _ = espNATTReplay(t, [][]byte{espNATTFrame(t, w[:cut], v6, 42000, 55001)}, WithProtocolDecodeAs("udp", 55001, "esp-natt"))
			for _, e := range events {
				require.NotEqual(t, "esp", e.Protocol)
				require.NotContains(t, e.DisplayFields(), "esp.spi")
			}
		}
		frame = espNATTFrame(t, w, v6, 4500, 4500)
		events, _ = espNATTReplay(t, [][]byte{frame}, WithProtocolDecodeAs("udp", 4500, "ike-natt"))
		require.NotEqual(t, "esp", events[0].Protocol, "an independent explicit profile is authoritative")
		hlen := 20
		if v6 {
			hlen = 40
		}
		// Contradictory UDP/IP lengths and capture suffix loss are refusals,
		// even though the captured prefix contains both clear ESP words.
		bad := bytes.Clone(frame)
		binary.BigEndian.PutUint16(bad[14+hlen+4:], 2000)
		events, _ = espNATTReplay(t, [][]byte{bad})
		for _, e := range events {
			require.NotEqual(t, "esp", e.Protocol)
			require.NotContains(t, e.DisplayFields(), "esp.spi")
		}
		rawIP := frame[14 : 14+hlen+8+len(w)-1]
		events = carrierTestReplay(t, rawIP, len(rawIP)+1, true)
		for _, e := range events {
			require.NotEqual(t, "esp", e.Protocol)
			require.NotContains(t, e.DisplayFields(), "esp.sequence")
		}
	}
	long := append(bytes.Clone(w), bytes.Repeat([]byte{1}, 80)...)
	events, _ := espNATTReplay(t, [][]byte{espNATTFrame(t, long, false, 4500, 4500)}, WithProtocolBudget(64, 1024))
	require.Len(t, events, 1)
	require.NotEqual(t, "esp", events[0].Protocol, "oversized UDP is refused before native admission")
	require.NotContains(t, events[0].DisplayFields(), "esp.spi")
	found := false
	for _, p := range binparser.ProtocolCatalog {
		if p.Name == "UDP-Encapsulated ESP Header Fields" {
			require.Equal(t, "NAT-T", p.ProfileOf)
			require.Equal(t, "ipsec_esp.yaml", p.RuleFile)
			require.Equal(t, "ESPHeaderFields", p.EntryNode)
			require.Equal(t, "partial", p.Status)
			found = true
		}
	}
	require.True(t, found)
}

func TestESPNATTFragmentLossReorderAndDuplicateDoNotCreateSA(t *testing.T) {
	w := append([]byte{0xf4, 0x87, 0xbf, 0xcf, 0, 0, 0, 8}, bytes.Repeat([]byte{0x45}, 24)...)
	for _, v6 := range []bool{false, true} {
		frame := espNATTFrame(t, w, v6, 4500, 4500)
		parts := icmpFragmentFrames(t, frame, 16, false)
		if v6 {
			for _, part := range parts {
				part[54] = 17 // The shared fragment builder defaults to ICMPv6.
			}
		}
		for _, order := range [][][]byte{parts, {parts[1], parts[0]}, {parts[1], parts[1], parts[0]}} {
			events, stats := espNATTReplay(t, order)
			var observed []*ProtocolEvent
			for _, e := range events {
				if e.Protocol == "esp" {
					observed = append(observed, e)
				}
			}
			require.Len(t, observed, 1)
			require.Equal(t, w, observed[0].Raw)
			require.Equal(t, "reassembled", observed[0].SourceBytes.Kind)
			require.GreaterOrEqual(t, len(observed[0].SourceBytes.PacketRefs), 2)
			require.Zero(t, observed[0].FlowID)
			require.Zero(t, stats.Flows)
		}
		for _, lost := range [][][]byte{{parts[0]}, {parts[1]}} {
			events, _ := espNATTReplay(t, lost)
			for _, e := range events {
				require.NotEqual(t, "esp", e.Protocol)
				require.NotContains(t, e.DisplayFields(), "esp.spi")
			}
		}
	}
}
