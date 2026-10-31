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

func TestIPv4EOLOptionKeepsPacketRowAndInspectorEndpointsConsistent(t *testing.T) {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 37, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2"), Options: []layers.IPv4Option{{OptionType: 0, OptionLength: 1}}}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(8, 0), Id: 0x1234, Seq: 7}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, icmp, gopacket.Payload([]byte("eol"))))
	wire := bytes.Clone(buf.Bytes())
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(42, 0), Length: len(wire), CaptureLength: len(wire)}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.Equal(t, "icmp", events[0].Protocol)
	require.Equal(t, "198.51.100.1", events[0].Source)
	raw := &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
	row := summarize(raw)
	require.Equal(t, "198.51.100.1", row.Source)
	require.Equal(t, "198.51.100.2", row.Destination)
	require.Equal(t, []string{"Ethernet", "IPv4", "ICMPv4"}, row.Layers)
	require.Equal(t, []string{"Ethernet", "IPv4", "ICMP"}, row.ProtocolStack)
	require.NotContains(t, row.Info, "incomplete/undecoded")
	require.NotContains(t, row.ProtocolStack, "IPv6HopByHop")
	for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
		require.NotEmpty(t, got.sources)
		require.Equal(t, wire, got.sources[0].Data)
		for _, field := range got.fields {
			require.False(t, strings.Contains(field.text, "<nil>"), field.text)
			require.NotContains(t, field.text, "IPv6HopByHop")
		}
	}
}
