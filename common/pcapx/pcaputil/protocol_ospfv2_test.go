package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func ospfv2NativeHello(instance byte, neighbors ...byte) []byte {
	w := []byte{2, 1, 0, 0, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, instance, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		255, 255, 255, 0, 0, 10, 2, 1, 0, 0, 0, 40, 192, 0, 2, 1, 0, 0, 0, 0}
	for _, n := range neighbors {
		w = append(w, 192, 0, 2, n)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	var sum uint32
	for i := 0; i < len(w); i += 2 {
		if i < 16 || i >= 24 {
			sum += uint32(binary.BigEndian.Uint16(w[i:]))
		}
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	binary.BigEndian.PutUint16(w[12:], ^uint16(sum))
	return w
}

func ospfv2NativeFrame(t *testing.T, wire []byte, offset uint16, more bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{1, 0, 0x5e, 0, 0, 5}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, Id: 19, TTL: 1, Protocol: layers.IPProtocolOSPF, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("224.0.0.5"), FragOffset: offset}
	if more {
		ip.Flags = layers.IPv4MoreFragments
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(wire)))
	return bytes.Clone(b.Bytes())
}

func TestOSPFv2NativeIndependentReusePrivateFieldsAndVersionScope(t *testing.T) {
	one, two := ospfv2NativeHello(17, 2, 3), ospfv2NativeHello(18)
	frames := [][]byte{ospfv2NativeFrame(t, one, 0, false), ospfv2NativeFrame(t, two, 0, false), ospfv2NativeFrame(t, one, 0, false)}
	es, stats := espNATTReplay(t, frames)
	require.Len(t, es, 3)
	require.Zero(t, stats.Flows)
	for i, e := range es {
		require.Equal(t, "ospfv2-bounded-control", e.Profile)
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Zero(t, e.FlowID)
		require.Zero(t, e.TCPFlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Len(t, e.SourceBytes.PacketRefs, 1)
		require.Equal(t, uint64(i+1), e.SourceBytes.PacketRefs[0].Number)
		require.Equal(t, []any{uint64(2)}, e.DisplayFields()["ospf.version"])
		if i == 1 {
			require.Empty(t, e.DisplayFields()["ospfv2.neighbor"])
		} else {
			require.Equal(t, []any{"192.0.2.2", "192.0.2.3"}, e.DisplayFields()["ospfv2.neighbor"])
		}
		for key := range e.DisplayFields() {
			require.False(t, strings.HasPrefix(key, "ospfv3."), key)
		}
		fields, err := e.GetFields()
		require.NoError(t, err)
		before := e.DisplayFieldValues()
		e.Raw[0] = 99
		e.Session["Hello"].(map[string]any)["Options"] = uint64(255)
		e.Session["Header"].(map[string]any)["Version"] = uint64(3)
		e.Protocol, e.Status, e.Profile = "tcp", "unrecognized", "forged"
		after, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, fields, after)
		require.Equal(t, before, e.DisplayFieldValues())
	}
	v3, _ := espNATTReplay(t, [][]byte{ospfv3NativeFrame(ospfv3NativeHello(7, 2), 89, nil)})
	for key := range v3[0].DisplayFields() {
		require.False(t, strings.HasPrefix(key, "ospfv2."), key)
	}
}

func TestOSPFv2NativeFragmentReorderLossAndDuplicate(t *testing.T) {
	w := ospfv2NativeHello(17, 2, 3)
	first, last := ospfv2NativeFrame(t, w[:32], 0, true), ospfv2NativeFrame(t, w[32:], 4, false)
	for _, frames := range [][][]byte{{last, first}, {first, first, last}} {
		es, _ := espNATTReplay(t, frames)
		var found []*ProtocolEvent
		for _, e := range es {
			if e.Profile == "ospfv2-bounded-control" {
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
		require.NotEqual(t, "ospfv2-bounded-control", e.Profile)
	}
}

func TestOSPFv2NativeBadChecksumPartialBudgetAndCarrierGuard(t *testing.T) {
	w := ospfv2NativeHello(17, 2, 3)
	bad := bytes.Clone(w)
	bad[len(bad)-1] ^= 1
	es, _ := espNATTReplay(t, [][]byte{ospfv2NativeFrame(t, bad, 0, false), ospfv2NativeFrame(t, w[:12], 0, false), ospfv2NativeFrame(t, w, 0, false)})
	require.Len(t, es, 3)
	require.Equal(t, "malformed", es[0].Status)
	require.Equal(t, false, es[0].Session["Checksum Verified"])
	require.Equal(t, "malformed", es[1].Status)
	require.NotContains(t, es[1].DisplayFields(), "ospfv2.neighbor")
	require.Equal(t, "decoded", es[2].Status)
	var limited []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(e *ProtocolEvent) { limited = append(limited, e) })(c))
	require.NoError(t, c.prepareBinParser())
	c.binParser.budget.MaxCollectionElements = 1
	for _, wire := range [][]byte{w, bad} {
		p := protocolAnalysisPacket(gopacket.NewPacket(ospfv2NativeFrame(t, wire, 0, false), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default), nil)
		_, handled := c.binParser.networkPacket(p)
		require.True(t, handled)
	}
	require.Len(t, limited, 2)
	require.Equal(t, "limited", limited[0].Status)
	require.Equal(t, "malformed", limited[1].Status)
	require.Contains(t, limited[1].Error, "collection budget")
	require.Contains(t, limited[1].Error, "checksum failed")
	c.binParser.closeUDPSessions()
	p := protocolAnalysisPacket(gopacket.NewPacket(espNATTFrame(t, w, false, 89, 89), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default), nil)
	_, _, selected := declaredOSPFv2Wire(p)
	require.False(t, selected)
}
