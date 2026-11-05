package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ipipOptionsPresentationPacket(t *testing.T, typ, length byte) *capturedPacket {
	t.Helper()
	base := ipipExtentPresentationPacket(t).data
	wire := append(append(bytes.Clone(base[:34]), typ, length, 0, 0), base[34:]...)
	wire[14], wire[24], wire[25] = 0x46, 0, 0
	binary.BigEndian.PutUint16(wire[18:20], 0x1234)
	var sum uint32
	for at := 14; at < 38; at += 2 {
		sum += uint32(binary.BigEndian.Uint16(wire[at : at+2]))
	}
	for sum>>16 != 0 {
		sum = sum&65535 + sum>>16
	}
	binary.BigEndian.PutUint16(wire[24:26], ^uint16(sum))
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), Length: len(wire), CaptureLength: len(wire)}
	var input bytes.Buffer
	w := pcapgo.NewWriter(&input)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, w.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	var stats pcaputil.TCPReassemblyStats
	err := pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnTCPReassemblyStats(func(s pcaputil.TCPReassemblyStats) { stats = s }), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) }))
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.DecodeErrors)
	require.Len(t, events, 1)
	require.Equal(t, "IPv4CarrierExtentUnavailable", events[0].ExpertCode)
	require.Equal(t, false, events[0].Session["Parent Header Decode Complete"])
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, reference: events[0].SourceBytes.PacketRefs[0], events: events, presentation: presentEvents(events)}
}

func TestIPv4IPIPOptionErrorLiteralRangesAndObservations(t *testing.T) {
	for _, option := range [][2]byte{{7, 0}, {7, 20}, {148, 1}} {
		t.Run(fmt.Sprintf("%d/%d", option[0], option[1]), func(t *testing.T) {
			raw := ipipOptionsPresentationPacket(t, option[0], option[1])
			row := summarize(raw)
			require.Equal(t, "IPv4", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, "option")
			require.Equal(t, []string{"Ethernet", "IPv4"}, row.ProtocolStack)
			require.Empty(t, row.Source)
			require.Empty(t, row.Destination)
			require.Empty(t, row.ConnectionProtocol)
			require.Zero(t, row.StreamID)
			for _, v := range []struct {
				name string
				view packetDetail
			}{{"detail", detail(raw)}, {"quick", quickDetail(raw)}, {"packet-worker", workerMetadataDetailForPacket(t, raw)}} {
				t.Run(v.name, func(t *testing.T) {
					require.Equal(t, "IPv4", v.view.protocol)
					text := detailLines(v.view.fields)
					require.Contains(t, text, "Parent Header Decode Complete: false")
					require.Contains(t, text, "Parent Header Decode Error:")
					require.Contains(t, text, "Header observation context")
					require.NotContains(t, text, "Other decoded fields")
					require.Contains(t, text, "Declared Next Protocol: IPv4 (4) · child not admitted")
					require.NotContains(t, text, "DNS")
					require.Empty(t, v.view.sources)
					length, identifier, contextIdentifier := 0, 0, 0
					for _, f := range v.view.fields {
						switch f.text {
						case "Observed Total Length: 0 · invalid carrier extent":
							length++
							require.True(t, f.hasRange)
							require.Equal(t, 0, f.source)
							require.Equal(t, 16, f.start)
							require.Equal(t, 18, f.end)
							require.Equal(t, []byte{0, 0}, raw.data[f.start:f.end])
						case "Observed Fragment ID: 4660":
							if !f.hasRange {
								// The generated context repeats the observation without
								// borrowing the physical field's exact wire projection.
								contextIdentifier++
								continue
							}
							identifier++
							require.True(t, f.hasRange)
							require.Equal(t, 18, f.start)
							require.Equal(t, 20, f.end)
							require.Equal(t, []byte{0x12, 0x34}, raw.data[f.start:f.end])
						}
					}
					require.Equal(t, 1, length)
					require.Equal(t, 1, identifier)
					require.Equal(t, 1, contextIdentifier)
				})
			}
			stream := workerMetadataDetail(t, raw.events, true)
			require.Equal(t, "IPv4", stream.protocol)
			require.Contains(t, detailLines(stream.fields), "Header observation context")
			require.Contains(t, detailLines(stream.fields), "Parent Header Decode Complete: false")
			for _, f := range stream.fields {
				require.False(t, f.hasRange)
			}
		})
	}
}
