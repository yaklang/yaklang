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

func mldPresentationPacket(t *testing.T, cut int) *capturedPacket {
	t.Helper()
	message, err := hex.DecodeString("8f00123400000002" +
		"01010002ff3e0000000000000000000000001234" +
		"20010db8000000000000000000000001" +
		"00000000000000000000ffffc0000201deadbeef" +
		"06000000ff0200000000000000000000000000fb")
	require.NoError(t, err)
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{0x33, 0x33, 0, 0, 0, 0x16}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 1, NextHeader: layers.IPProtocolICMPv6, SrcIP: net.ParseIP("fe80::1"), DstIP: net.ParseIP("ff02::16")}
	w := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(w, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload(message)))
	wire := bytes.Clone(w.Bytes())
	length := len(wire)
	if cut >= 0 {
		wire = wire[:54+cut]
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

func TestMLDInspectorNativeAndWorkerHaveProgressiveExactRecordRanges(t *testing.T) {
	raw := mldPresentationPacket(t, -1)
	require.Equal(t, "MLDv2", summarize(raw).Protocol)
	require.Contains(t, summarize(raw).Info, "records=2")
	require.Contains(t, summarize(raw).Info, "first group=ff3e::1234")
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "MLDv2 Multicast Listener Report")
		require.Contains(t, text, "Address Records[0]")
		require.Contains(t, text, "Address Records[1]")
		require.Contains(t, text, "Sources[1]: ::ffff:192.0.2.1")
		require.Contains(t, text, "Auxiliary Data (uninterpreted)")
		require.NotContains(t, text, "YAML dissector incomplete")
		require.NotContains(t, text, "Wire Field Ranges")
		require.NotContains(t, text, "Dissector failed")
		require.Len(t, got.sources, 1)
		require.Equal(t, raw.data, got.sources[0].Data)
		var ranges [][2]int
		for _, field := range got.fields {
			if field.text == "Multicast Address: ff3e::1234" || field.text == "Multicast Address: ff02::fb" {
				require.True(t, field.hasRange)
				require.Equal(t, 1, field.source)
				ranges = append(ranges, [2]int{field.start, field.end})
			}
		}
		require.Equal(t, [][2]int{{66, 82}, {122, 138}}, ranges, "the second record must not reuse the first record's bytes")
		assertMembershipFieldDepths(t, got.fields, "MLD", "Address Records[0]", "Sources", "Sources[1]: ::ffff:192.0.2.1")
	}
}

func TestMLDInspectorIncompleteAndStrictCapturedContract(t *testing.T) {
	raw := mldPresentationPacket(t, 64)
	require.Equal(t, "incomplete", raw.events[0].Status)
	require.Equal(t, "MLDv2", summarize(raw).Protocol)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "address record fixed header unavailable")
		require.Contains(t, text, "Address Record Count: 2")
		require.NotContains(t, text, "Address Records[1]", "the absent record has no invented identity")
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
		{"wrong-status", func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" }},
		{"wrong-source", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number++ }},
		{"wrong-byte-range", func(e *pcaputil.ProtocolEvent) {
			e.Session["MLD"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Address Record Count"] = []any{0, 2}
		}},
		{"invented-record", func(e *pcaputil.ProtocolEvent) { e.Session["MLD"].(map[string]any)["Address Records"] = []any{} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			encoded, err := json.Marshal(raw.events[0])
			require.NoError(t, err)
			var event pcaputil.ProtocolEvent
			require.NoError(t, json.Unmarshal(encoded, &event))
			change.fn(&event)
			var view packetDetail
			require.False(t, appendMLDCapturedPrefix(&view, raw, []*pcaputil.ProtocolEvent{&event}))
			require.Empty(t, view.fields)
		})
	}
}
