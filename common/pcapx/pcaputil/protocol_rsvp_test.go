package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func rsvpNativeWire(value byte) []byte {
	w := []byte{0x10, 1, 0, 0, 64, 0, 0, 16, 0, 8, 5, 1, 0, 0, 0, value}
	var sum uint32
	for at := 0; at < len(w); at += 2 {
		sum += uint32(binary.BigEndian.Uint16(w[at:]))
	}
	for sum>>16 != 0 {
		sum = sum&65535 + sum>>16
	}
	binary.BigEndian.PutUint16(w[2:], ^uint16(sum))
	return w
}
func rsvpNativeFrame(t *testing.T, wire []byte, v6 bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 64, Protocol: 46, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	if v6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: 46, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(wire)))
	return b.Bytes()
}

func rsvpIntservNativeWire(rate uint32) []byte {
	w := make([]byte, 44)
	copy(w, []byte{0x10, 1, 0, 0, 64, 0, 0, 44, 0, 36, 12, 2, 0, 0, 0, 7, 1, 0, 0, 6, 127, 0, 0, 5})
	binary.BigEndian.PutUint32(w[24:], rate)
	binary.BigEndian.PutUint32(w[28:], 0x45bb8000)
	binary.BigEndian.PutUint32(w[32:], 0x7f800000)
	binary.BigEndian.PutUint32(w[36:], 1)
	binary.BigEndian.PutUint32(w[40:], 1500)
	return w
}

func TestRSVPIntservNativePrivateTypedRangesWithoutConnectionStickiness(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		var frames [][]byte
		for _, rate := range []uint32{0x45bb8000, 0x3f800000, 0x45bb8000} {
			frames = append(frames, rsvpNativeFrame(t, rsvpIntservNativeWire(rate), v6))
		}
		events, stats := espNATTReplay(t, frames)
		require.Len(t, events, 3)
		require.Zero(t, stats.Flows)
		for i, e := range events {
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TCPFlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, []any{[]string{"6000", "1", "6000"}[i]}, e.DisplayFields()["rsvp.intserv.token_bucket_rate"])
			require.Equal(t, []any{"+Infinity"}, e.DisplayFields()["rsvp.intserv.peak_data_rate"])
			snapshot := e.DisplayFieldValues()
			require.False(t, snapshot.Incomplete)
			var seen bool
			for _, f := range snapshot.Values {
				if f.Name == "rsvp.intserv.token_bucket_rate" {
					seen = true
					require.Equal(t, &DisplayFieldRange{Offset: 24, Length: 4}, f.Range)
				}
			}
			require.True(t, seen)
			filter, err := CompileDisplayFilter(`rsvp.intserv.token_bucket_rate == "6000"`)
			require.NoError(t, err)
			require.Equal(t, i != 1, filter.Match(e))
			before, err := e.GetFields()
			require.NoError(t, err)
			e.Raw[24] ^= 255
			e.Session = map[string]any{}
			after, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, snapshot, e.DisplayFieldValues())
		}
	}
}

func TestRSVPIntservContextAndIndependentWireFailuresStayDistinct(t *testing.T) {
	future := rsvpIntservNativeWire(0x45bb8000)
	future[12] = 0x10
	for _, badChecksum := range []bool{false, true} {
		w := bytes.Clone(future)
		if badChecksum {
			w[3] = 1
		}
		frame := rsvpNativeFrame(t, w, false)
		packet := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.DecodeOptions{NoCopy: true})
		events, _ := espNATTReplay(t, [][]byte{frame})
		require.Len(t, events, 1)
		if badChecksum {
			require.NotNil(t, packet.ErrorLayer())
			require.ErrorIs(t, packet.ErrorLayer().Error(), stream_parser.ErrRSVPWireIntegrity)
			require.Equal(t, "malformed", events[0].Status)
		} else {
			require.Nil(t, packet.ErrorLayer())
			require.Equal(t, "context-required", events[0].Status)
		}
	}
	// Exhaust a nested collection with independent checksum evidence. A soft
	// budget reason must never hide the bad wire in capture or native ingress.
	w := make([]byte, 24+4095*4)
	copy(w, []byte{0x10, 1, 0, 1, 64, 0, 0, 0, 0, 0, 13, 2, 0, 0, 0, 0, 5, 0, 0, 0})
	// Intserv body starts12; parameters start20. The extra last word is a
	// second zero-length service, also bounded by this same collection budget.
	for at := 20; at < len(w)-4; at += 4 {
		w[at] = 250
	}
	binary.BigEndian.PutUint16(w[6:], uint16(len(w)))
	binary.BigEndian.PutUint16(w[8:], uint16(len(w)-8))
	binary.BigEndian.PutUint16(w[14:], uint16((len(w)-16)/4))
	binary.BigEndian.PutUint16(w[18:], uint16((len(w)-24)/4))
	_, err := stream_parser.DecodeRSVPWire(w, 4096)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	require.ErrorIs(t, err, stream_parser.ErrRSVPWireIntegrity)
	frame := rsvpNativeFrame(t, w, false)
	packet := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.DecodeOptions{NoCopy: true})
	require.NotNil(t, packet.ErrorLayer())
	require.True(t, errors.Is(packet.ErrorLayer().Error(), stream_parser.ErrRSVPWireIntegrity))
	events, _ := espNATTReplay(t, [][]byte{frame})
	require.Len(t, events, 1)
	require.Equal(t, "malformed", events[0].Status)
}

func TestRSVPNativeIndependentAdvertisementsAndPrivateFields(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		var frames [][]byte
		for _, v := range []byte{2, 1, 2, 3} {
			frames = append(frames, rsvpNativeFrame(t, rsvpNativeWire(v), v6))
		}
		es, stats := espNATTReplay(t, frames)
		require.Len(t, es, 4)
		require.Zero(t, stats.Flows)
		for i, e := range es {
			require.Equal(t, "rsvp", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Equal(t, "network", e.Transport)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TCPFlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, []any{uint64([]byte{2, 1, 2, 3}[i])}, e.DisplayFields()["rsvp.refresh_interval"])
			require.Equal(t, []any{true}, e.DisplayFields()["rsvp.checksum_verified"])
			before, err := e.GetFields()
			require.NoError(t, err)
			snapshot := e.DisplayFieldValues()
			e.Raw[0] = 255
			e.Session = map[string]any{}
			e.Protocol, e.Status, e.Source = "udp", "unrecognized", "forged"
			after, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, snapshot, e.DisplayFieldValues())
		}
	}
}

func TestRSVPParentBoundsUnknownLayoutAndChecksumEvidence(t *testing.T) {
	w := rsvpNativeWire(7)
	w[10], w[11] = 9, 2 // Existing scalar bytes contradict the newly selected Intserv header.
	w[2], w[3] = 0, 0
	es, _ := espNATTReplay(t, [][]byte{rsvpNativeFrame(t, w, false)})
	require.Len(t, es, 1)
	require.Equal(t, "malformed", es[0].Status)
	require.Contains(t, es[0].Error, "Intserv message word length")
	w[11] = 99 // An unselected C-Type remains opaque, not a confirmed flow.
	es, _ = espNATTReplay(t, [][]byte{rsvpNativeFrame(t, w, false)})
	require.Len(t, es, 1)
	e := es[0]
	require.Equal(t, "rsvp", e.Protocol)
	require.Equal(t, "decoded", e.Status, e.Error)
	require.Equal(t, "limited", e.DisplayFieldValues().Completeness)
	f, err := CompileDisplayFilter("not rsvp.session.ip")
	require.NoError(t, err)
	require.False(t, f.EvaluateEvents(es).Complete)
	require.Empty(t, e.DisplayFields()["rsvp.refresh_interval"])
	bad := rsvpNativeFrame(t, rsvpNativeWire(1), false)
	binary.BigEndian.PutUint16(bad[16:18], 0)
	es, _ = espNATTReplay(t, [][]byte{bad})
	for _, e := range es {
		require.NotEqual(t, "rsvp", e.Protocol)
	}
	corrupt := bytes.Clone(rsvpNativeWire(1))
	corrupt[len(corrupt)-1] ^= 1
	es, _ = espNATTReplay(t, [][]byte{rsvpNativeFrame(t, corrupt, true)})
	require.Len(t, es, 1)
	require.Equal(t, "rsvp", es[0].Protocol)
	require.Equal(t, "malformed", es[0].Status)
	require.Equal(t, []any{false}, es[0].DisplayFields()["rsvp.checksum_verified"])
}

func TestRSVPNativeIPFragmentsRequireCompleteReassembly(t *testing.T) {
	w := append(rsvpNativeWire(7), 0, 8, 5, 1, 0, 0, 0, 9)
	binary.BigEndian.PutUint16(w[6:], uint16(len(w)))
	w[2], w[3] = 0, 0
	whole := rsvpNativeFrame(t, w, false)
	fragment := func(offset int, more bool, data []byte) []byte {
		frame := append(bytes.Clone(whole[:34]), data...)
		binary.BigEndian.PutUint16(frame[16:18], uint16(20+len(data)))
		binary.BigEndian.PutUint16(frame[18:20], 0x3142)
		flags := uint16(offset / 8)
		if more {
			flags |= 0x2000
		}
		binary.BigEndian.PutUint16(frame[20:22], flags)
		frame[24], frame[25] = 0, 0
		var sum uint32
		for at := 14; at < 34; at += 2 {
			sum += uint32(binary.BigEndian.Uint16(frame[at:]))
		}
		for sum>>16 != 0 {
			sum = sum&65535 + sum>>16
		}
		binary.BigEndian.PutUint16(frame[24:26], ^uint16(sum))
		return frame
	}
	first, last := fragment(0, true, w[:16]), fragment(16, false, w[16:])
	for _, frames := range [][][]byte{{last, first}, {first, first, last}} {
		es, _ := espNATTReplay(t, frames)
		var found []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "rsvp" {
				found = append(found, e)
			}
		}
		require.Len(t, found, 1)
		require.Equal(t, w, found[0].Raw)
		require.Equal(t, []any{uint64(7), uint64(9)}, found[0].DisplayFields()["rsvp.refresh_interval"])
		require.GreaterOrEqual(t, len(found[0].SourceBytes.PacketRefs), 2)
	}
	es, _ := espNATTReplay(t, [][]byte{last})
	for _, e := range es {
		require.NotEqual(t, "rsvp", e.Protocol, "orphan suffix is not a complete control message")
	}
}

func TestRSVPNativeIPv6FragmentsPreserveOnlyCompleteMessage(t *testing.T) {
	w := append(rsvpNativeWire(7), 0, 8, 5, 1, 0, 0, 0, 9)
	binary.BigEndian.PutUint16(w[6:], uint16(len(w)))
	w[2], w[3] = 0, 0
	whole := rsvpNativeFrame(t, w, true)
	fragment := func(offset int, more bool, data []byte) []byte {
		frame := bytes.Clone(whole[:54])
		frame[20] = byte(layers.IPProtocolIPv6Fragment)
		binary.BigEndian.PutUint16(frame[18:20], uint16(8+len(data)))
		header := []byte{46, 0, 0, 0, 0, 0, 0x31, 0x42}
		flags := uint16(offset)
		if more {
			flags |= 1
		}
		binary.BigEndian.PutUint16(header[2:4], flags)
		frame = append(frame, header...)
		return append(frame, data...)
	}
	first, last := fragment(0, true, w[:16]), fragment(16, false, w[16:])
	for _, frames := range [][][]byte{{last, first}, {first, first, last}} {
		es, stats := espNATTReplay(t, frames)
		var found []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "rsvp" {
				found = append(found, e)
			}
		}
		require.Len(t, found, 1)
		require.Equal(t, w, found[0].Raw)
		require.Equal(t, []any{uint64(7), uint64(9)}, found[0].DisplayFields()["rsvp.refresh_interval"])
		require.Equal(t, "2001:db8::1", found[0].Source)
		require.Equal(t, "2001:db8::2", found[0].Destination)
		require.GreaterOrEqual(t, len(found[0].SourceBytes.PacketRefs), 2)
		require.Zero(t, stats.Flows)
	}
	for _, frames := range [][][]byte{{first}, {last}} {
		es, _ := espNATTReplay(t, frames)
		for _, e := range es {
			require.NotEqual(t, "rsvp", e.Protocol, "a missing fragment cannot supply advertised selectors")
		}
	}
}

func TestRSVPCompleteParentLengthIsNotCaptureLoss(t *testing.T) {
	for _, w := range [][]byte{
		{0x10},
		{0x10, 1, 0, 0, 64, 0, 0, 16},
		{0x10, 12, 0, 0, 64, 0, 0, 16},
	} {
		es, _ := espNATTReplay(t, [][]byte{rsvpNativeFrame(t, w, false)})
		require.Len(t, es, 1)
		require.Equal(t, "rsvp", es[0].Protocol)
		require.Equal(t, "malformed", es[0].Status, es[0].Error)
		require.Equal(t, "MalformedMessage", es[0].ExpertCode)
		if len(w) == 1 {
			require.Empty(t, es[0].DisplayFields()["rsvp.checksum_omitted"])
			require.Empty(t, es[0].DisplayFields()["rsvp.checksum_verified"])
		}
	}
	// The v1 fixed header cannot be selected from an unselected version.
	es, _ := espNATTReplay(t, [][]byte{rsvpNativeFrame(t, []byte{0x20}, false)})
	require.Len(t, es, 1)
	require.Equal(t, "context-required", es[0].Status)
}

func TestRSVPIPv6ExtensionsCannotBorrowIPv4Parent(t *testing.T) {
	for _, next := range []byte{0, 43, 60} {
		wire := append([]byte{46, 0, 0, 0, 0, 0, 0, 0}, rsvpNativeWire(7)...)
		for _, validParent := range []bool{true, false} {
			frame := rsvpNativeFrame(t, wire, false)
			frame[23] = next
			if !validParent {
				binary.BigEndian.PutUint16(frame[16:18], 0)
			}
			frame[24], frame[25] = 0, 0
			var sum uint32
			for at := 14; at < 34; at += 2 {
				sum += uint32(binary.BigEndian.Uint16(frame[at:]))
			}
			for sum>>16 != 0 {
				sum = sum&65535 + sum>>16
			}
			binary.BigEndian.PutUint16(frame[24:26], ^uint16(sum))
			captured := gopacket.NewPacket(frame, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			require.Nil(t, captured.Layer(captureRSVPType), "IPv4 protocol %d is not an IPv6 parent", next)
			private := protocolAnalysisPacket(captured, nil)
			require.Nil(t, private.Layer(captureRSVPType))
			if captured.ErrorLayer() != nil {
				require.NotNil(t, private.ErrorLayer(), "RSVP selection must not erase an unselected carrier's decoder error")
			}
			var es []*ProtocolEvent
			var captureStats TCPReassemblyStats
			err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { captureStats = s }))
			if captured.ErrorLayer() != nil && next != 0 {
				require.Error(t, err, "IPv4 protocol %d, valid parent %t: preserve the strict capture error", next, validParent)
				require.Positive(t, captureStats.DecodeErrors)
			} else if captured.ErrorLayer() == nil {
				require.NoError(t, err)
			}
			for _, e := range es {
				require.NotEqual(t, "rsvp", e.Protocol, "IPv4 protocol %d cannot select an IPv6 extension's child", next)
				require.NotEqual(t, "RSVPParentLengthInvalid", e.ExpertCode)
			}
		}
		frame := rsvpNativeFrame(t, wire, true)
		frame[20] = next
		es, _ := espNATTReplay(t, [][]byte{frame})
		require.Len(t, es, 1)
		require.Equal(t, "rsvp", es[0].Protocol)
		require.Equal(t, "decoded", es[0].Status, es[0].Error)
		require.Equal(t, []any{uint64(7)}, es[0].DisplayFields()["rsvp.refresh_interval"])
	}
}
