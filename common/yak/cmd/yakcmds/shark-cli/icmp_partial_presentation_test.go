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

func TestICMPv6IncompleteBodyKeepsProgressiveHeaderAndSource(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	ip := &layers.IPv6{Version: 6, HopLimit: 255, NextHeader: layers.IPProtocolICMPv6, SrcIP: net.ParseIP("2001:db8::10"), DstIP: net.ParseIP("2001:db8::20")}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload([]byte{128, 0, 0, 0})))
	wire := bytes.Clone(buf.Bytes())
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), Length: len(wire), CaptureLength: len(wire)}
	var input bytes.Buffer
	w := pcapgo.NewWriter(&input)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.Equal(t, "incomplete", events[0].Status)
	raw := &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
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
	}
}
