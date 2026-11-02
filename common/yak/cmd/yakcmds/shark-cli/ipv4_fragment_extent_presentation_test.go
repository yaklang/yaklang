package sharkcli

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func fragmentExtentPresentationPacket(t *testing.T) *capturedPacket {
	t.Helper()
	wire := udpPacket(t, 53, bytes.Repeat([]byte{0xab}, 32))
	binary.BigEndian.PutUint16(wire[16:18], 0)
	binary.BigEndian.PutUint16(wire[20:22], 0x2000)
	wire[24], wire[25] = 0, 0
	var sum uint32
	for at := 14; at < 34; at += 2 {
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
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.Equal(t, "IPv4FragmentExtentUnavailable", events[0].ExpertCode)
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, reference: events[0].SourceBytes.PacketRefs[0], events: events, presentation: presentEvents(events)}
}

func TestIPv4FragmentExtentRefusalAgreesAcrossViews(t *testing.T) {
	raw := fragmentExtentPresentationPacket(t)
	row := summarize(raw)
	require.Equal(t, "IPv4", row.Protocol)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Contains(t, row.Info, "Total Length is zero")
	require.Equal(t, []string{"Ethernet", "IPv4"}, row.ProtocolStack)
	require.Empty(t, row.Source, "invalid extent does not qualify address-shaped bytes as a transport peer")
	require.Empty(t, row.ConnectionProtocol)
	require.Zero(t, row.StreamID)
	views := []struct {
		name string
		view packetDetail
	}{{"detail", detail(raw)}, {"quick", quickDetail(raw)}, {"packet-worker", workerMetadataDetailForPacket(t, raw)}}
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			require.Equal(t, "IPv4", v.view.protocol)
			text := detailLines(v.view.fields)
			require.Contains(t, text, "Total Length is zero")
			require.NotContains(t, text, "YAML dissector")
			require.NotContains(t, text, "DNS")
			require.Empty(t, v.view.sources)
			literal := 0
			for _, f := range v.view.fields {
				if f.text == "Observed Total Length: 0 · invalid fragment extent" {
					literal++
					require.True(t, f.hasRange)
					require.Equal(t, 0, f.source)
					require.Equal(t, 16, f.start)
					require.Equal(t, 18, f.end)
					require.Equal(t, []byte{0, 0}, raw.data[f.start:f.end])
				}
			}
			require.Equal(t, 1, literal)
		})
	}
	stream := workerMetadataDetail(t, raw.events, true)
	require.Equal(t, "IPv4", stream.protocol)
	require.Contains(t, detailLines(stream.fields), "Total Length is zero")
	for _, f := range stream.fields {
		require.False(t, f.hasRange, "a stream-only diagnostic cannot borrow a frame")
	}
}

func TestIPv4FragmentExtentRangesRequireTheCurrentPhysicalPacket(t *testing.T) {
	for _, change := range []string{"length", "flags", "frame", "domain", "source", "identifier", "offset", "MF"} {
		t.Run(change, func(t *testing.T) {
			raw := fragmentExtentPresentationPacket(t)
			switch change {
			case "length":
				binary.BigEndian.PutUint16(raw.data[16:18], uint16(len(raw.data)-14))
			case "flags":
				binary.BigEndian.PutUint16(raw.data[20:22], 0)
			case "frame":
				raw.number++
			case "domain":
				raw.reference.Domain.Interface++
			case "source":
				raw.events[0].SourceBytes.Kind = "reassembled"
			case "identifier":
				raw.events[0].Session["Observed Fragment ID"] = 999
			case "offset":
				raw.events[0].Session["Observed Fragment Offset"] = 8
			case "MF":
				raw.events[0].Session["Observed More Fragments"] = false
			}
			for _, view := range []packetDetail{detail(raw), quickDetail(raw)} {
				require.NotContains(t, detailLines(view.fields), "Observed Total Length: 0 · invalid fragment extent")
			}
		})
	}
}

func TestIPv4FragmentExtentLabelRequiresCompleteRefusalContract(t *testing.T) {
	for _, change := range []string{"profile", "length", "header", "fragment", "reassembly"} {
		t.Run(change, func(t *testing.T) {
			raw := fragmentExtentPresentationPacket(t)
			e := raw.events[0]
			switch change {
			case "profile":
				e.Profile = "unselected"
			case "length":
				delete(e.Session, "Observed Total Length")
			case "header":
				e.Session["Captured Header Bytes"] = 4
			case "fragment":
				e.Session["Observed More Fragments"] = false
			case "reassembly":
				e.Session["Fragment Reassembly Admitted"] = true
			}
			_, ok := ipv4FragmentExtentRefusal(e)
			require.False(t, ok)
			require.False(t, presentEvents(raw.events).networkRefused, "a label or partial hint cannot grant the native refusal contract")
		})
	}
}
