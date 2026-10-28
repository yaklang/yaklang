package sharkcli

import (
	"bytes"
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

func TestICMPv4TimestampPartialNativeAndWorkerAgree(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP("198.51.100.10"), DstIP: net.ParseIP("198.51.100.20")}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(14, 0), Id: 0x1234, Seq: 7}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, icmp, gopacket.Payload([]byte{1, 2, 3, 4, 0x80})))
	wire := bytes.Clone(b.Bytes())
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), CaptureLength: len(wire), Length: len(wire)}
	var input bytes.Buffer
	w := pcapgo.NewWriter(&input)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	raw := &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
	row := summarize(raw)
	require.Equal(t, "ICMP", row.Protocol)
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Equal(t, []string{"Ethernet", "IPv4", "ICMP"}, row.ProtocolStack)
	require.Contains(t, row.Info, "13/20 bytes")
	for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
		var text strings.Builder
		for _, f := range got.fields {
			text.WriteString(f.text)
			text.WriteByte('\n')
		}
		require.Contains(t, text.String(), "Originate Timestamp: 16909060")
		require.NotContains(t, text.String(), "Receive Timestamp:")
		require.NotContains(t, text.String(), "Transmit Timestamp:")
		require.NotContains(t, text.String(), "YAML dissector incomplete")
		require.NotContains(t, text.String(), "DecodeFailure")
		require.Equal(t, wire, got.sources[0].Data)
	}
	p := gopacket.NewPacket(wire, pcaputil.CaptureLinkDecoder(raw.link), gopacket.Default)
	require.True(t, matchedICMPv4PartialObservation(p, events, 1, raw.reference.Domain))
	e := *events[0]
	e.Session = map[string]any{}
	for k, v := range events[0].Session {
		e.Session[k] = v
	}
	e.Session["Receive Timestamp"] = uint32(0)
	require.False(t, matchedICMPv4PartialObservation(p, []*pcaputil.ProtocolEvent{&e}, 1, raw.reference.Domain), "a partial word cannot default to zero")
	raw.ci.Length += 17
	require.Contains(t, summarize(raw).Info, "truncated capture:")
}
