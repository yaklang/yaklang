package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func gtpv2NativeWire(seq uint32, value byte) []byte {
	w := []byte{0x48, 32, 0, 13, 0, 0, 0, 0, byte(seq >> 16), byte(seq >> 8), byte(seq), 0, 3, 0, 1, 0, value}
	return w
}

func TestGTPv2NativeIndependentDatagramsAcrossReuseReorderAndDuplicate(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		var frames [][]byte
		for i, seq := range []uint32{10, 9, 10, 11} {
			frames = append(frames, espNATTFrame(t, gtpv2NativeWire(seq, byte(i+1)), v6, 40000, 2123))
		}
		es, stats := espNATTReplay(t, frames)
		require.Len(t, es, 4)
		require.Zero(t, stats.Flows)
		for i, e := range es {
			require.Equal(t, "gtpv2", e.Protocol)
			require.Equal(t, "decoded", e.Status, e.Error)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TCPFlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, []any{uint64(0)}, e.DisplayFields()["gtpv2.teid"])
			require.Equal(t, []any{uint64(i + 1)}, e.DisplayFields()["gtpv2.recovery"])
			before, err := e.GetFields()
			require.NoError(t, err)
			snapshot := e.DisplayFieldValues()
			e.Raw[0] = 0xff
			e.Session = map[string]any{"Messages": []any{}}
			e.Protocol, e.Status, e.Source = "tcp", "unrecognized", "forged"
			after, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, snapshot, e.DisplayFieldValues())
		}
	}
}

func TestGTPv2NativePortHintAndExplicitSelectionNeverBorrowIdentity(t *testing.T) {
	a := nf9NativeParser(t)
	w := gtpv2NativeWire(1, 7)
	for _, wire := range [][]byte{w[:3], w[:10], append(bytes.Clone(w), 0), {0x30, 1, 0, 4, 1, 2, 3, 4}} {
		e := &ProtocolEvent{Transport: "udp"}
		require.False(t, a.decodeGTPv2Datagram(e, wire, 40000, 2123, ""))
		e = &ProtocolEvent{Transport: "udp"}
		require.True(t, a.decodeGTPv2Datagram(e, wire, 40000, 2123, "gtpv2"))
		require.NotEqual(t, "decoded", e.Status)
	}
	require.False(t, a.decodeGTPv2Datagram(&ProtocolEvent{}, w, 40000, 9990, ""))
	e := &ProtocolEvent{Transport: "udp"}
	require.True(t, a.decodeGTPv2Datagram(e, w, 40000, 9990, "gtpv2"))
	require.Equal(t, "decoded", e.Status)
	c := NewDefaultConfig()
	require.NoError(t, WithProtocolDecodeAs("udp", 9990, "gtpv2")(c))
	// A complete physical record with a UDP extent beyond its IP parent is
	// refused even when its payload bytes resemble a complete GTP message.
	bad := espNATTFrame(t, w, false, 40000, 2123)
	binary.BigEndian.PutUint16(bad[38:40], 65535)
	es, _ := espNATTReplay(t, [][]byte{bad}, WithProtocolDecodeAs("udp", 2123, "gtpv2"))
	for _, e := range es {
		require.NotEqual(t, "gtpv2", e.Protocol)
	}
}

func TestGTPv2NativeFragmentsWaitForCompleteUDPReassembly(t *testing.T) {
	w := gtpv2NativeWire(0x010203, 7)
	whole := espNATTFrame(t, w, false, 40000, 2123)
	udpSize := int(binary.BigEndian.Uint16(whole[38:40]))
	udp := bytes.Clone(whole[34 : 34+udpSize])
	fragment := func(offset int, more bool, data []byte) []byte {
		frame := append(bytes.Clone(whole[:34]), data...)
		binary.BigEndian.PutUint16(frame[16:18], uint16(20+len(data)))
		binary.BigEndian.PutUint16(frame[18:20], 0x1234)
		flags := uint16(offset / 8)
		if more {
			flags |= 0x2000
		}
		binary.BigEndian.PutUint16(frame[20:22], flags)
		frame[24], frame[25] = 0, 0
		var sum uint32
		for at := 14; at < 34; at += 2 {
			sum += uint32(binary.BigEndian.Uint16(frame[at : at+2]))
		}
		for sum>>16 != 0 {
			sum = sum&0xffff + sum>>16
		}
		binary.BigEndian.PutUint16(frame[24:26], ^uint16(sum))
		if len(frame) < 60 {
			frame = append(frame, make([]byte, 60-len(frame))...)
		}
		return frame
	}
	first, last := fragment(0, true, udp[:24]), fragment(24, false, udp[24:])
	for _, frames := range [][][]byte{{last, first}, {first, first, last}} {
		es, _ := espNATTReplay(t, frames)
		var found []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "gtpv2" {
				found = append(found, e)
			}
		}
		require.Len(t, found, 1)
		require.Equal(t, "decoded", found[0].Status, found[0].Error)
		require.Equal(t, w, found[0].Raw)
		require.GreaterOrEqual(t, len(found[0].SourceBytes.PacketRefs), 2)
	}
	es, _ := espNATTReplay(t, [][]byte{last})
	for _, e := range es {
		require.NotEqual(t, "gtpv2", e.Protocol, "orphan suffix cannot identify a complete UDP control message")
	}
}

func TestGTPv2UnselectedInterfaceCannotCertifyEPCFieldsOrTheirAbsence(t *testing.T) {
	w := gtpv2NativeWire(1, 7)
	w[1] = 4 // S101 selects a different interface specification and IE namespace.
	es, _ := espNATTReplay(t, [][]byte{espNATTFrame(t, w, false, 40000, 2123)}, WithProtocolDecodeAs("udp", 2123, "gtpv2"))
	var selected *ProtocolEvent
	for _, e := range es {
		if e.Protocol == "gtpv2" {
			selected = e
		}
	}
	require.NotNil(t, selected)
	require.NotEqual(t, "decoded", selected.Status)
	require.Equal(t, []any{uint64(4)}, selected.DisplayFields()["gtpv2.message_type"])
	require.Empty(t, selected.DisplayFields()["gtpv2.ie_type"])
	require.Empty(t, selected.DisplayFields()["gtpv2.recovery"])
	for _, expr := range []string{"gtpv2.recovery", "not gtpv2.recovery", "gtpv2.recovery != 7"} {
		filter, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		result := filter.EvaluateEvents([]*ProtocolEvent{selected})
		require.True(t, result.Partial, expr)
		require.False(t, result.Complete, expr)
		require.False(t, result.Matched, expr)
	}
	filter, err := CompileDisplayFilter("gtpv2.message_type == 4")
	require.NoError(t, err)
	require.True(t, filter.Match(selected), "a retained literal header remains a usable witness")
}

func TestGTPv2AutomaticFallbackPreservesAnExistingIKEEnvelope(t *testing.T) {
	// Independently authored overlap: valid IKE_AUTH with a bounded encrypted
	// SK payload also resembles a GTP header plus one opaque unknown IE.
	wire, err := hex.DecodeString("482000200102030400000100fa0014002e202320000000010000002400000008aabbccdd")
	require.NoError(t, err)
	a := nf9NativeParser(t)
	e := &ProtocolEvent{Transport: "udp"}
	require.True(t, a.decodeNativeDatagram(e, wire, 2123, 500))
	require.Equal(t, "ike", e.Protocol, "a weak source-port GTP envelope cannot preempt a complete existing grammar")
	require.Equal(t, "decoded", e.Status, e.Error)
	selected := &ProtocolEvent{Transport: "udp"}
	require.True(t, a.decodeGTPv2Datagram(selected, wire, 2123, 500, "gtpv2"))
	require.Equal(t, "gtpv2", selected.Protocol, "explicit selection keeps its separate wire interpretation")
	reserved := gtpv2NativeWire(1, 0)[:12]
	reserved[1] = 178
	binary.BigEndian.PutUint16(reserved[2:], 8)
	require.False(t, a.decodeGTPv2Datagram(&ProtocolEvent{Transport: "udp"}, reserved, 40000, 2123, ""), "a named reserved code point is not a selected message type")
}
