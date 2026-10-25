package sharkcli

import (
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestL2TPv3DetailedFieldsUseOwnAVPSpans(t *testing.T) {
	w := []byte{0, 0, 0, 0, 0xc8, 3, 0, 30, 0x12, 0x34, 0x56, 0x78, 0, 7, 0, 5, 0x80, 8, 0, 0, 0, 0, 0, 6, 0, 10, 0, 0, 0, 25, 0x12, 0x34, 0x56, 0x78}
	m, err := stream_parser.DecodeL2TPv3Wire(w, false, 32)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "l2tp", Status: "decoded", Raw: w, Session: m}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "IPv4", Layers: []string{"Ethernet", "IPv4"}})
	require.Equal(t, "L2TPv3", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "L2TPv3"}, r.ProtocolStack)
	require.Contains(t, r.Info, "HELLO")
	require.Contains(t, r.Info, "Ns=7 Nr=5")
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	matched := false
	for _, f := range fs {
		require.NotContains(t, f.text, "Wire Field Ranges")
		if strings.HasPrefix(f.text, "Physical Channel ID:") {
			require.True(t, f.hasRange)
			require.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, sources[f.source-1].Data[f.start:f.end])
			matched = true
		}
	}
	require.True(t, matched)
}

func TestL2TPv3NativeBytesCoverOnlyTheirUnsupportedDecoderError(t *testing.T) {
	w := []byte{0, 0, 0, 7, 1, 2, 3, 4}
	m, err := stream_parser.DecodeL2TPv3Wire(w, false, 32)
	require.NoError(t, err)
	m["Opaque Payload Bytes"] = uint64(4)
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		&layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4},
		&layers.IPv4{Version: 4, TTL: 64, Protocol: 115, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}, gopacket.Payload(w)))
	frame := b.Bytes()
	p := gopacket.NewPacket(frame, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.NotNil(t, p.ErrorLayer(), "the generic decoder still lacks IP115")
	t.Logf("generic IP115 refusal: %v; captured IP payload bytes=%d", p.ErrorLayer().Error(), len(p.NetworkLayer().LayerPayload()))
	e := &pcaputil.ProtocolEvent{Protocol: "l2tp", Transport: "network", Profile: "l2tp3-bounded-wire-envelope", Status: "decoded", Raw: w, Session: m}
	raw := &capturedPacket{number: 1, data: frame, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(frame), CaptureLength: len(frame)}, events: []*pcaputil.ProtocolEvent{e}}
	r := summarize(raw)
	require.Equal(t, "L2TPv3", r.Protocol)
	require.NotContains(t, r.Info, "incomplete/undecoded")
	require.Contains(t, r.Info, "receiver context unavailable")
	e.Raw = []byte{0, 0, 0, 8, 1, 2, 3, 4}
	require.Contains(t, summarize(raw).Info, "incomplete/undecoded", "a label/foreign PDU cannot cover different wire bytes")
	e.Raw = w
	raw.ci.Length += 8
	require.Contains(t, summarize(raw).Info, "truncated capture", "native coverage must not hide an actual snaplen cut")
}

func TestL2TPv3MidstreamPageExplainsOpaqueReceiverContext(t *testing.T) {
	m, err := stream_parser.DecodeL2TPv3Wire([]byte{0, 0, 0, 7, 1, 2, 3, 4}, false, 32)
	require.NoError(t, err)
	m["Opaque Payload Bytes"], m["Connection Context"] = uint64(4), "not observed"
	e := &pcaputil.ProtocolEvent{Protocol: "l2tp", Status: "decoded", Completeness: "limited", Raw: []byte{0, 0, 0, 7, 1, 2, 3, 4}, Session: m}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "IPv4", Layers: []string{"Ethernet", "IPv4"}})
	require.Equal(t, "L2TPv3", r.Protocol)
	require.Contains(t, r.Info, "session=0x00000007")
	require.Contains(t, r.Info, "receiver context unavailable")
	fs, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, f := range fs {
		require.NotContains(t, f.text, "Opaque Session Payload:")
		require.NotContains(t, f.text, "TCP")
	}
}
