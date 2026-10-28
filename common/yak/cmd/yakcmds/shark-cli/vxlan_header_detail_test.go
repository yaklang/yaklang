package sharkcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func vxlanHeaderDetailPacket(t *testing.T, link layers.LinkType, flags byte) *capturedPacket {
	t.Helper()
	dns := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1}
	inner := udpPacket(t, 53, dns)
	wire := udpPacket(t, 4789, append([]byte{flags, 0x48, 0x12, 0x34, 0, 0, 42, 0}, inner...))
	if link == layers.LinkTypeNull {
		wire = append([]byte{2, 0, 0, 0}, wire[14:]...)
	}
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(wire), Length: len(wire)}
	var capture bytes.Buffer
	w := pcapgo.NewWriter(&capture)
	require.NoError(t, w.WriteFileHeader(65535, link))
	require.NoError(t, w.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	err := pcaputil.ReplayPcap(bytes.NewReader(capture.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) }))
	if flags&8 == 0 {
		require.ErrorContains(t, err, "I bit is clear")
	} else {
		require.NoError(t, err)
	}
	require.Len(t, events, 1)
	return &capturedPacket{number: 1, data: wire, ci: ci, link: link, events: events, presentation: presentEvents(events)}
}

func TestVXLANCapturedHeaderPrecedesApplicationAndRetainsPhysicalRanges(t *testing.T) {
	for _, link := range []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeNull} {
		raw := vxlanHeaderDetailPacket(t, link, 0x88)
		require.Equal(t, "DNS", summarize(raw).Protocol)
		request, err := marshalDecodeRequest(packetDecodeRequest(raw))
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
		worker, err := decodeWorkerOutput(output.Bytes(), raw)
		require.NoError(t, err)
		for _, got := range []packetDetail{detail(raw), quickDetail(raw), worker} {
			require.Equal(t, "Captured VXLAN header 1", got.fields[1].text)
			start := 42
			if link == layers.LinkTypeNull {
				start = 32
			}
			for text, span := range map[string][2]int{"Flags: 0x88": {0, 1}, "I: true": {0, 1}, "VNI: 42": {4, 7}, "G: true": {0, 1}, "GBP Don't Learn: true": {1, 2}, "GBP Applied: true": {1, 2}, "GBP Policy ID: 4660": {2, 4}} {
				found := false
				for _, f := range got.fields {
					if f.text == text {
						found = true
						require.True(t, f.hasRange)
						require.Zero(t, f.source, "header bytes belong to the physical frame")
						require.Equal(t, start+span[0], f.start)
						require.Equal(t, start+span[1], f.end)
					}
				}
				require.True(t, found, text)
			}
		}
	}
}

func TestVXLANCapturedGBPFieldsRequireGAndParentAdmission(t *testing.T) {
	for _, flags := range []byte{0, 8} {
		raw := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, flags)
		for _, got := range []packetDetail{detail(raw), quickDetail(raw)} {
			for _, f := range got.fields {
				require.False(t, strings.HasPrefix(f.text, "GBP "), "G=0 is reserved data, not a group-policy observation")
				if flags == 0 {
					require.NotContains(t, f.text, "Captured VXLAN header")
					require.NotContains(t, f.text, "example.com")
				}
			}
		}
	}
}

func TestVXLANCapturedHeaderAnnotationCannotOverrideUDPRefusal(t *testing.T) {
	raw := vxlanHeaderDetailPacket(t, layers.LinkTypeEthernet, 0x88)
	for _, protocol := range []string{"", "udp"} {
		raw.events = []*pcaputil.ProtocolEvent{{Protocol: protocol, Transport: "udp", Status: "incomplete", Error: "NeedMore: UDP carrier incomplete"}}
		raw.presentation = presentEvents(raw.events)
		for _, got := range []packetDetail{detail(raw), quickDetail(raw)} {
			for _, f := range got.fields {
				require.NotContains(t, f.text, "Captured VXLAN header")
			}
		}
	}
}
