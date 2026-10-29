package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func ospfv3NativeHello(instance byte, neighbors ...byte) []byte {
	w := []byte{3, 1, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, instance, 0,
		0, 0, 0, 7, 1, 0, 0, 0x13, 0, 10, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0}
	for _, n := range neighbors {
		w = append(w, 192, 0, 2, n)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	context := stream_parser.OSPFv3WireContext{Source: netip.MustParseAddr("fe80::1"), Destination: netip.MustParseAddr("ff02::5")}
	binary.BigEndian.PutUint16(w[12:], stream_parser.OSPFv3Checksum(w, context))
	return w
}

func ospfv3NativeFrame(w []byte, next byte, extensions []byte) []byte {
	frame := make([]byte, 14+40)
	copy(frame, []byte{0x33, 0x33, 0, 0, 0, 5, 2, 0, 0, 0, 0, 1, 0x86, 0xdd})
	frame[14], frame[20], frame[21] = 0x60, next, 1
	copy(frame[22:38], netip.MustParseAddr("fe80::1").AsSlice())
	copy(frame[38:54], netip.MustParseAddr("ff02::5").AsSlice())
	frame = append(frame, extensions...)
	frame = append(frame, w...)
	binary.BigEndian.PutUint16(frame[18:], uint16(len(frame)-54))
	return frame
}

func TestOSPFv3NativeIndependentReuseReorderAndPrivateFields(t *testing.T) {
	one, two := ospfv3NativeHello(0, 2, 3), ospfv3NativeHello(1)
	ah := make([]byte, 24)
	ah[0], ah[1], ah[7], ah[11] = 89, 4, 7, 9
	frames := [][]byte{ospfv3NativeFrame(one, 89, nil), ospfv3NativeFrame(two, 51, ah), ospfv3NativeFrame(one, 89, nil)}
	events, stats := espNATTReplay(t, frames)
	require.Len(t, events, 3)
	require.Zero(t, stats.Flows)
	for i, e := range events {
		require.Equal(t, "ospf", e.Protocol)
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Equal(t, true, e.Session["Checksum Verified"])
		require.Equal(t, false, e.Session["Authentication Verified"])
		require.Zero(t, e.FlowID)
		require.Zero(t, e.TCPFlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Len(t, e.SourceBytes.PacketRefs, 1)
		require.Equal(t, uint64(i+1), e.SourceBytes.PacketRefs[0].Number)
		require.NotNil(t, e.SourceBytes.CompletedIn)
		fields, err := e.GetFields()
		require.NoError(t, err)
		before := e.DisplayFieldValues()
		if i == 1 {
			require.Empty(t, e.DisplayFields()["ospfv3.neighbor"])
		} else {
			require.Equal(t, []any{"192.0.2.2", "192.0.2.3"}, e.DisplayFields()["ospfv3.neighbor"])
		}
		e.Raw[0] = 99
		e.Session["Header"].(map[string]any)["Router ID"] = "forged"
		e.Protocol, e.Status, e.Source = "tcp", "unrecognized", "forged"
		after, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, fields, after)
		require.Equal(t, before, e.DisplayFieldValues())
	}
}

func TestOSPFv3NativeMalformedAndParentBoundsNeverBorrowFields(t *testing.T) {
	good := ospfv3NativeHello(0, 2)
	bad := bytes.Clone(good)
	bad[len(bad)-1] ^= 1
	wrong := bytes.Clone(good)
	wrong[0] = 2
	frames := [][]byte{ospfv3NativeFrame(good, 89, nil), ospfv3NativeFrame(bad, 89, nil), ospfv3NativeFrame(wrong, 89, nil), ospfv3NativeFrame(good[:18], 89, nil), ospfv3NativeFrame(good, 89, nil)}
	es, _ := espNATTReplay(t, frames)
	require.Len(t, es, 5)
	require.Equal(t, []string{"decoded", "malformed", "malformed", "malformed", "decoded"}, []string{es[0].Status, es[1].Status, es[2].Status, es[3].Status, es[4].Status})
	for _, i := range []int{1, 2, 3} {
		require.Nil(t, es[i].SourceBytes.CompletedIn)
		require.Zero(t, es[i].FlowID)
	}
	require.Empty(t, es[3].DisplayFields()["ospfv3.neighbor"])
	require.Equal(t, []any{"192.0.2.2"}, es[4].DisplayFields()["ospfv3.neighbor"])
	// A UDP payload/quoted packet cannot select the OSPF control observer.
	udp := espNATTFrame(t, good, true, 520, 521)
	p := protocolAnalysisPacket(gopacket.NewPacket(udp, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default), nil)
	_, _, selected := declaredOSPFv3Wire(p)
	require.False(t, selected)
	parent := ospfv3NativeFrame(good, 51, []byte{89, 0, 0, 0, 0, 0, 0, 0})
	p = protocolAnalysisPacket(gopacket.NewPacket(parent, CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default), nil)
	_, _, selected = declaredOSPFv3Wire(p)
	require.False(t, selected, "invalid AH length cannot admit a child")
	short, _ := espNATTReplay(t, [][]byte{ospfv3NativeFrame([]byte{3}, 89, nil)})
	require.Len(t, short, 1)
	require.Equal(t, []any{uint64(3)}, short[0].DisplayFields()["ospf.version"], "a later absent Instance ID must not erase the captured Version")
	require.NotContains(t, short[0].DisplayFields(), "ospfv3.instance_id")
}

func TestOSPFv3NativeFragmentsWaitForCompleteBoundedReassembly(t *testing.T) {
	w := ospfv3NativeHello(0, 2)
	fragment := func(off int, more bool, data []byte) []byte {
		h := []byte{89, 0, 0, 0, 0, 0, 0, 19}
		v := uint16(off)
		if more {
			v |= 1
		}
		binary.BigEndian.PutUint16(h[2:], v)
		return ospfv3NativeFrame(data, 44, h)
	}
	first, last := fragment(0, true, w[:24]), fragment(24, false, w[24:])
	for _, frames := range [][][]byte{{last, first}, {first, first, last}} {
		es, _ := espNATTReplay(t, frames)
		var found []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "ospf" {
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
		require.NotEqual(t, "ospf", e.Protocol, "orphan suffix is not a complete control message")
	}
}

func TestOSPFv3NativeBudgetAndRoutingChecksumContext(t *testing.T) {
	w := ospfv3NativeHello(0, 2, 3)
	var es []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })(c))
	require.NoError(t, c.prepareBinParser())
	c.binParser.budget.MaxCollectionElements = 1
	p := protocolAnalysisPacket(gopacket.NewPacket(ospfv3NativeFrame(w, 89, nil), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default), nil)
	_, handled := c.binParser.networkPacket(p)
	require.True(t, handled)
	require.Len(t, es, 1)
	require.Equal(t, "limited", es[0].Status)
	require.Equal(t, []any{"192.0.2.2"}, es[0].DisplayFields()["ospfv3.neighbor"])
	f, err := CompileDisplayFilter("not ospfv3.prefix")
	require.NoError(t, err)
	require.False(t, f.EvaluateEvents(es).Complete)
	c.binParser.closeUDPSessions()
	// Unknown active Routing Type cannot be called a failed checksum.
	rh := make([]byte, 8)
	rh[0], rh[2], rh[3] = 89, 99, 1
	ip := &layers.IPv6{BaseLayer: layers.BaseLayer{Payload: append(rh, w...)}, NextHeader: 43, SrcIP: netip.MustParseAddr("fe80::1").AsSlice(), DstIP: netip.MustParseAddr("ff02::5").AsSlice()}
	context := ospfv3Context(ip)
	require.True(t, context.PseudoHeaderUnavailable)
	m, decodeErr := stream_parser.DecodeOSPFv3Wire(w, context, 32)
	require.ErrorIs(t, decodeErr, stream_parser.ErrObservedWireContext)
	require.Equal(t, false, m["Checksum Verified"])
}

func TestOSPFv3ObserverContextAndLocalBudgetAreNotCaptureErrors(t *testing.T) {
	unknown := []byte{3, 6, 0, 16, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	for _, w := range [][]byte{unknown, ospfv3NativeHello(0, bytes.Repeat([]byte{2}, 4097)...)} {
		p := gopacket.NewPacket(ospfv3NativeFrame(w, 89, nil), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
		require.Nil(t, p.ErrorLayer(), "unknown semantics/resource policy must stay separate from malformed wire")
		require.NotNil(t, p.Layer(layers.LayerTypeOSPF))
	}
	es, _ := espNATTReplay(t, [][]byte{ospfv3NativeFrame(unknown, 89, nil)})
	require.Len(t, es, 1)
	require.Equal(t, "context-required", es[0].Status)
	require.Equal(t, false, es[0].Session["Authentication Verified"])
}
