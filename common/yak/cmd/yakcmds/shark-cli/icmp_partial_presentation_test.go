package sharkcli

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func icmpv6PartialTestPacket(t *testing.T, payload []byte) *capturedPacket {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 255, NextHeader: layers.IPProtocolICMPv6, SrcIP: net.ParseIP("2001:db8::10"), DstIP: net.ParseIP("2001:db8::20")}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload(payload)))
	wire := bytes.Clone(buf.Bytes())
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), Length: len(wire), CaptureLength: len(wire)}
	var input bytes.Buffer
	w := pcapgo.NewWriter(&input)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
}

func TestICMPv6IncompleteBodyKeepsProgressiveHeaderAndSource(t *testing.T) {
	raw := icmpv6PartialTestPacket(t, []byte{128, 0, 0, 0})
	wire := raw.data
	require.Equal(t, "incomplete", raw.events[0].Status)
	row := summarize(raw)
	require.Equal(t, "ICMPv6", row.Protocol)
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Equal(t, "partial-message", row.Visibility)
	require.Equal(t, []string{"Ethernet", "IPv6", "ICMPv6"}, row.ProtocolStack)
	require.Contains(t, row.Info, "EchoRequest")
	require.Contains(t, row.Info, "4/8 bytes")
	require.Zero(t, row.StreamID)
	for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
		require.NotEmpty(t, got.sources)
		require.Equal(t, wire, got.sources[0].Data)
		var text strings.Builder
		for _, f := range got.fields {
			text.WriteString(f.text)
			text.WriteByte('\n')
		}
		require.Contains(t, text.String(), "Available ICMPv6 Bytes")
		require.Contains(t, text.String(), "Required ICMPv6 Header Bytes")
		require.NotContains(t, text.String(), "Identifier:")
		require.NotContains(t, text.String(), "Sequence:")
		require.NotContains(t, text.String(), "YAML dissector incomplete")
		require.NotContains(t, text.String(), "DecodeFailure")
	}
}

func TestICMPv6PartialDiagnosticCoversOnlyItsOwnBytes(t *testing.T) {
	redirect := make([]byte, 40)
	redirect[0] = 137
	copy(redirect[8:24], net.ParseIP("fe80::9").To16())
	copy(redirect[24:40], net.ParseIP("2001:db8::5").To16())
	redirect = append(redirect, 2, 1, 1, 2, 3, 4, 5, 6, 2, 0)
	raw := icmpv6PartialTestPacket(t, redirect)
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.Default)
	require.NotNil(t, p.ErrorLayer())
	require.True(t, matchedICMPv6PartialObservation(p, raw.events, 1, raw.reference.Domain))
	require.True(t, icmpv6PartialDecoderCovered(p, raw.events, 1, raw.reference.Domain))
	row := summarize(raw)
	require.Contains(t, row.Info, "invalid ND option length")
	require.NotContains(t, row.Info, "incomplete/undecoded")
	for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
		var text strings.Builder
		for _, f := range got.fields {
			text.WriteString(f.text)
			text.WriteByte('\n')
		}
		require.Contains(t, text.String(), "Target: fe80::9")
		require.Contains(t, text.String(), "Destination: 2001:db8::5")
		require.NotContains(t, text.String(), "YAML dissector incomplete")
		require.NotContains(t, text.String(), "DecodeFailure")
	}
	for name, mutate := range map[string]func(*pcaputil.ProtocolEvent){
		"wrong-type":        func(e *pcaputil.ProtocolEvent) { e.Session["Type"] = uint8(136) },
		"wrong-code":        func(e *pcaputil.ProtocolEvent) { e.Session["Code"] = uint8(1) },
		"wrong-target":      func(e *pcaputil.ProtocolEvent) { e.Session["Target"] = "fe80::99" },
		"wrong-destination": func(e *pcaputil.ProtocolEvent) { e.Session["Destination"] = "2001:db8::99" },
		"options-prefix":    func(e *pcaputil.ProtocolEvent) { e.Session["Options"] = nil },
		"hash":              func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) },
		"source-number":     func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number++ },
		"source-domain":     func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Domain.Interface++ },
		"missing-raw":       func(e *pcaputil.ProtocolEvent) { e.Raw = nil },
		"unrelated-profile": func(e *pcaputil.ProtocolEvent) { e.Profile = "some-other-diagnostic" },
		"unrelated-error":   func(e *pcaputil.ProtocolEvent) { e.Error = "unrelated decoder failure" },
		"complete-status":   func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" },
	} {
		t.Run(name, func(t *testing.T) {
			// JSON round-trip exercises the isolated worker's public metadata
			// representation as well as preventing aliasing the real event.
			wire, err := json.Marshal(raw.events[0])
			require.NoError(t, err)
			var forged pcaputil.ProtocolEvent
			require.NoError(t, json.Unmarshal(wire, &forged))
			require.True(t, matchedICMPv6PartialObservation(p, []*pcaputil.ProtocolEvent{&forged}, 1, raw.reference.Domain))
			mutate(&forged)
			require.False(t, matchedICMPv6PartialObservation(p, []*pcaputil.ProtocolEvent{&forged}, 1, raw.reference.Domain))
			require.False(t, icmpv6PartialDecoderCovered(p, []*pcaputil.ProtocolEvent{&forged}, 1, raw.reference.Domain))
		})
	}
	raw.ci.Length += 8
	require.Contains(t, summarize(raw).Info, "truncated capture:", "a precise native reason cannot erase physical capture evidence")
}
