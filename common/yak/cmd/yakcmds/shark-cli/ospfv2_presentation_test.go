package sharkcli

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ospfv2PresentationPacket(t *testing.T) *capturedPacket {
	w := []byte{2, 1, 0, 52, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		255, 255, 255, 0, 0, 10, 0x12, 1, 0, 0, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0, 192, 0, 2, 2, 192, 0, 2, 2}
	binary.BigEndian.PutUint16(w[12:], stream_parser.OSPFv2Checksum(w))
	w = append(w, 0xff, 0xf6, 0, 3, 0, 1, 0, 4, 0, 0, 0, 1)
	raw := replayPIMPresentationPacket(t, refusalPresentationWire(t, 4, layers.IPProtocolOSPF, w))
	require.Len(t, raw.events, 1)
	raw.reference = raw.events[0].SourceBytes.PacketRefs[0]
	return raw
}

func TestOSPFv2ProgressiveNativeQuickAndWorkerSources(t *testing.T) {
	raw := ospfv2PresentationPacket(t)
	e := raw.events[0]
	require.Equal(t, "decoded", e.Status, e.Error)
	require.Equal(t, "ospfv2_observed", e.Rule)
	require.Equal(t, "OSPFv2ControlFields", e.Entry)
	row := summarize(raw)
	require.Equal(t, "OSPFv2", row.Protocol)
	require.Contains(t, row.Info, "Hello")
	require.Equal(t, 1, strings.Count(strings.Join(row.ProtocolStack, "/"), "OSPFv2"))
	var prefix packetDetail
	require.True(t, appendOSPFv2CapturedPrefix(&prefix, raw, raw.events))
	for _, d := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
		text := detailLines(d.fields)
		require.Contains(t, text, "IPv4 header")
		require.Contains(t, text, "Type: 1 (Hello)")
		require.Contains(t, text, "TLV Type: 1 (LLS Extended Options)")
		require.Contains(t, text, "Instance ID: 17")
		require.NotContains(t, text, "Wire Field Ranges")
		require.NotContains(t, text, "YAML dissector incomplete")
		require.NotContains(t, text, "<nil>")
		var offsets []int
		positions := map[string]int{}
		for i, f := range d.fields {
			if f.branch && (f.text == "Header" || f.text == "Authentication" || f.text == "Hello" || f.text == "LLS") {
				positions[f.text] = i
			}
			if f.text == "Neighbor Router ID: 192.0.2.2" {
				require.True(t, f.hasRange)
				require.Equal(t, 1, f.source)
				require.Equal(t, []byte{192, 0, 2, 2}, d.sources[0].Data[f.start:f.end])
				offsets = append(offsets, f.start)
			}
			if strings.HasPrefix(f.text, "Checksum Verified:") || strings.HasPrefix(f.text, "Authentication Verified:") {
				require.False(t, f.hasRange)
			}
		}
		require.Equal(t, []int{44, 48}, offsets)
		require.Len(t, positions, 4)
		require.Less(t, positions["Header"], positions["Authentication"])
		require.Less(t, positions["Authentication"], positions["Hello"])
		require.Less(t, positions["Hello"], positions["LLS"])
	}
	bad := *e
	bad.SourceBytes.SHA256 = "forged"
	var d packetDetail
	require.False(t, appendOSPFv2CapturedPrefix(&d, raw, []*pcaputil.ProtocolEvent{&bad}))
}

func TestOSPFControlCannotOverrideAnAlreadyRefusedNetworkParent(t *testing.T) {
	refused := refusalPresentationEvent("OSPFv2ParentLengthInvalid", []string{"Ethernet", "IPv4"})
	for _, v := range []int{2, 3} {
		child := &pcaputil.ProtocolEvent{Protocol: "ospf", Transport: "network", Profile: "ospfv2-bounded-control", Rule: "ospfv2_observed", Entry: "OSPFv2ControlFields", Raw: []byte{2}, Status: "decoded"}
		if v == 3 {
			child.Profile, child.Rule, child.Entry, child.Raw = "ospfv3-bounded-control", "ospfv3_observed", "OSPFv3ControlFields", []byte{3}
		}
		for _, events := range [][]*pcaputil.ProtocolEvent{{refused, child}, {child, refused}} {
			p := presentEvents(events)
			require.True(t, p.networkRefused)
			require.NotContains(t, p.stack, "OSPFv2")
			require.NotContains(t, p.stack, "OSPFv3")
		}
	}
}
