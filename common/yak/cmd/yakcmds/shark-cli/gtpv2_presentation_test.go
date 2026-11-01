package sharkcli

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func gtpv2PresentationFrame(t *testing.T, wire []byte, v6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 2123}
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
	return b.Bytes()
}

func TestGTPv2NativeQuickWorkerProgressiveFieldsAndExactSources(t *testing.T) {
	one := []byte{0x58, 33, 0, 14, 1, 2, 3, 4, 0, 0, 5, 0, 2, 0, 2, 0, 16, 0}
	two := []byte{0x48, 95, 0, 13, 1, 2, 3, 4, 0, 0, 6, 0, 3, 0, 1, 0, 9}
	w := append(one, two...)
	for _, v6 := range []bool{false, true} {
		frame := gtpv2PresentationFrame(t, w, v6)
		raw := replayPIMPresentationPacket(t, frame)
		require.Len(t, raw.events, 1)
		raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
		require.Equal(t, "decoded", raw.events[0].Status, raw.events[0].Error)
		row := summarize(raw)
		require.Equal(t, "GTPv2-C", row.Protocol)
		require.Contains(t, row.Info, "Create Session Response")
		require.Contains(t, row.Info, "Create Bearer Request")
		for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
			text := detailLines(d.fields)
			require.Contains(t, text, "UDP header")
			require.Contains(t, text, "Message Type: 33 (Create Session Response)")
			require.Contains(t, text, "Message Type: 95 (Create Bearer Request)")
			require.NotContains(t, text, "Wire Field Ranges")
			require.NotContains(t, text, "YAML dissector incomplete")
			require.NotContains(t, text, "<nil>")
			var spans []int
			for _, f := range d.fields {
				if f.text == "TEID: 16909060" {
					require.True(t, f.hasRange)
					require.Equal(t, 1, f.source)
					require.Equal(t, uint32(0x01020304), binary.BigEndian.Uint32(d.sources[0].Data[f.start:f.end]))
					spans = append(spans, f.start)
				}
				if strings.HasPrefix(f.text, "Piggyback:") || strings.HasPrefix(f.text, "TEID Present:") {
					require.False(t, f.hasRange)
				}
			}
			require.Equal(t, []int{4, len(one) + 4}, spans)
		}
	}
}

func TestGTPv2SourceBoundedLimitedMetadataKeepsProgressivePrefix(t *testing.T) {
	w := []byte{0x40, 1, 0, 14, 0, 0, 1, 0, 3, 0, 1, 0, 1, 3, 0, 1, 0, 2}
	raw := replayPIMPresentationPacket(t, gtpv2PresentationFrame(t, w, false))
	original := raw.events[0]
	partial, err := stream_parser.DecodeGTPv2Wire(w, 1)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	// This is a source-proved limited UI metadata record; no private native
	// field cache or actual low-budget ingress is claimed by this unit test.
	e := &pcaputil.ProtocolEvent{ID: original.ID, Protocol: "gtpv2", Transport: "udp", Profile: "gtpv2-bounded-UDP-observation", Rule: "gtpv2_observed", Entry: "GTPv2ControlFields", Status: "limited", Completeness: "limited", Error: err.Error(), Raw: w, Length: len(w), Session: partial, Fields: partial, SourceBytes: original.SourceBytes}
	raw.events = []*pcaputil.ProtocolEvent{e}
	raw.reference = e.SourceBytes.PacketRefs[0]
	raw.presentation = presentEvents(raw.events)
	var prefix packetDetail
	require.True(t, appendGTPv2CapturedPrefix(&prefix, raw, raw.events), "a matching smaller-budget observation remains source-reproducible")
	for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(d.fields)
		require.Contains(t, text, "Captured network prefix")
		require.Contains(t, text, "UDP header")
		require.Contains(t, text, "Recovery Counter: 1")
		require.NotContains(t, text, "Recovery Counter: 2")
		require.NotContains(t, text, "YAML dissector incomplete")
	}
}
