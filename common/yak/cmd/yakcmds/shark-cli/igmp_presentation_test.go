package sharkcli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func igmpPresentationPacket(t *testing.T, cut int) *capturedPacket {
	t.Helper()
	message, err := hex.DecodeString("2200123400000002" + "01010002ef010203c0000201c0000202deadbeef" + "06000000ef040506")
	require.NoError(t, err)
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{1, 0, 0x5e, 0, 0, 0x16}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 1, Protocol: layers.IPProtocolIGMP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("224.0.0.22")}
	w := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(w, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload(message)))
	wire := bytes.Clone(w.Bytes())
	length := len(wire)
	if cut >= 0 {
		wire = wire[:34+cut]
	}
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), CaptureLength: len(wire), Length: length}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
}

func TestIGMPInspectorNativeAndWorkerPreserveRecordHierarchyAndByteSources(t *testing.T) {
	raw := igmpPresentationPacket(t, -1)
	require.Equal(t, "IGMPv3", summarize(raw).Protocol)
	require.Contains(t, summarize(raw).Info, "records=2")
	require.Contains(t, summarize(raw).Info, "first group=239.1.2.3")
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "IGMPv3 Membership Report")
		require.Contains(t, text, "Group Records[0]")
		require.Contains(t, text, "Group Records[1]")
		require.Contains(t, text, "Sources[1]: 192.0.2.2")
		require.Contains(t, text, "Auxiliary Data (uninterpreted)")
		require.NotContains(t, text, "Wire Field Ranges")
		require.NotContains(t, text, "YAML dissector incomplete")
		require.NotContains(t, text, "Dissector failed")
		require.Len(t, got.sources, 1)
		require.Equal(t, raw.data, got.sources[0].Data)
		var ranges [][2]int
		for _, field := range got.fields {
			if field.text == "Multicast Address: 239.1.2.3" || field.text == "Multicast Address: 239.4.5.6" {
				require.True(t, field.hasRange)
				require.Equal(t, 1, field.source)
				ranges = append(ranges, [2]int{field.start, field.end})
			}
		}
		require.Equal(t, [][2]int{{46, 50}, {66, 70}}, ranges)
		assertMembershipFieldDepths(t, got.fields, "IGMP", "Group Records[0]", "Sources", "Sources[1]: 192.0.2.2")
	}
}

func TestIGMPInspectorTruncatedReportKeepsPrefixAndRejectsInventedProjection(t *testing.T) {
	raw := igmpPresentationPacket(t, 28)
	require.Equal(t, "incomplete", raw.events[0].Status)
	require.Equal(t, "IGMPv3", summarize(raw).Protocol)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "Group Record Count: 2")
		require.Contains(t, text, "group record fixed header unavailable")
		require.NotContains(t, text, "Group Records[1]")
		require.NotContains(t, text, "YAML dissector incomplete")
		for _, field := range got.fields {
			if field.hasRange {
				require.LessOrEqual(t, field.end, len(raw.data))
			}
		}
	}
	for _, change := range []struct {
		name string
		fn   func(*pcaputil.ProtocolEvent)
	}{
		{"status", func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" }},
		{"source", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number++ }},
		{"range", func(e *pcaputil.ProtocolEvent) {
			e.Session["IGMP"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Group Record Count"] = []any{0, 2}
		}},
		{"records", func(e *pcaputil.ProtocolEvent) { e.Session["IGMP"].(map[string]any)["Group Records"] = []any{} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			encoded, err := json.Marshal(raw.events[0])
			require.NoError(t, err)
			var event pcaputil.ProtocolEvent
			require.NoError(t, json.Unmarshal(encoded, &event))
			change.fn(&event)
			var view packetDetail
			require.False(t, appendIGMPCapturedPrefix(&view, raw, []*pcaputil.ProtocolEvent{&event}))
			require.Empty(t, view.fields)
		})
	}
}
