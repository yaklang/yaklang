package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func pimPartialPresentationPacket(t *testing.T) *capturedPacket {
	t.Helper()
	body := []byte{0x24, 0, 0, 0, 0, 7, 30, 1, 1, 1, 0, 0}
	context := stream_parser.PIMWireContext{Source: netip.MustParseAddr("192.0.2.1"), Destination: netip.MustParseAddr("224.0.0.13")}
	binary.BigEndian.PutUint16(body[2:4], stream_parser.PIMChecksum(body, context, len(body)))
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{1, 0, 0x5e, 0, 0, 13}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 1, Protocol: 103, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("224.0.0.13")}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(body)))
	wire := bytes.Clone(buffer.Bytes())
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), CaptureLength: len(wire), Length: len(wire)}
	var capture bytes.Buffer
	writer := pcapgo.NewWriter(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(capture.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
}

func TestPIMPartialNativeQuickAndWorkerKeepTheirExactSourceTree(t *testing.T) {
	raw := pimPartialPresentationPacket(t)
	require.Equal(t, "context-required", raw.events[0].Status)
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(got.fields)
		require.Contains(t, text, "BSR=unavailable")
		require.Contains(t, text, "Address Family: 1")
		require.Contains(t, text, "Encoding: 1")
		require.NotContains(t, text, "<nil>")
		require.NotContains(t, text, "YAML dissector incomplete")
		require.NotContains(t, text, "DecodeFailure")
		require.NotEmpty(t, got.sources)
		require.Equal(t, raw.events[0].Raw, got.sources[0].Data)
		for _, opaque := range got.sources[1:] {
			require.Contains(t, opaque.Name, "frame offsets unavailable")
			require.Equal(t, raw.events[0].Raw[10:], opaque.Data, "an extra opaque source remains exactly the captured unparsed suffix")
		}
		found := false
		for _, field := range got.fields {
			if field.text == "Encoding: 1" {
				require.True(t, field.hasRange)
				require.Equal(t, 1, field.source)
				require.Equal(t, 9, field.start)
				require.Equal(t, 10, field.end)
				require.Equal(t, []byte{1}, got.sources[0].Data[field.start:field.end])
				found = true
			}
		}
		require.True(t, found)
	}
	for _, change := range []struct {
		name string
		fn   func(*pcaputil.ProtocolEvent)
	}{
		{"status", func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" }},
		{"source", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = "forged" }},
		{"frame", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number++ }},
		{"field", func(e *pcaputil.ProtocolEvent) { e.Session["BSR"].(map[string]any)["Encoding"] = 0 }},
		{"range", func(e *pcaputil.ProtocolEvent) {
			e.Session["BSR"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Encoding"] = []any{0, 1}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			encoded, err := json.Marshal(raw.events[0])
			require.NoError(t, err)
			var event pcaputil.ProtocolEvent
			require.NoError(t, json.Unmarshal(encoded, &event))
			change.fn(&event)
			var view packetDetail
			require.False(t, appendPIMCapturedPrefix(&view, raw, []*pcaputil.ProtocolEvent{&event}), "a partial label match cannot suppress another decoder's failure")
		})
	}
}

func TestPIMMissingSummaryWordsDoNotBecomeAddressesOrZeroCounts(t *testing.T) {
	for _, typ := range []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9} {
		event := &pcaputil.ProtocolEvent{Protocol: "pim", Session: map[string]any{"Type": typ, "Packet Name": stream_parser.PIMMessageName(typ)}}
		text := applicationEventSummary(event)
		require.Contains(t, text, "unavailable")
		require.NotContains(t, text, "= ·")
		require.NotContains(t, text, "=(,)")
		if typ == 3 || typ == 6 || typ == 7 || typ == 8 {
			require.NotContains(t, text, "groups=0")
		}
		if strings.Contains(text, "joins=") {
			require.NotContains(t, text, "joins=0")
			require.NotContains(t, text, "prunes=0")
		}
	}
}
