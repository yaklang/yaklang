package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func nativeIGMPFrame(t *testing.T, message []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{1, 0, 0x5e, 0, 0, 0x16}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 1, Protocol: layers.IPProtocolIGMP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("224.0.0.22")}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(message)))
	return bytes.Clone(b.Bytes())
}

func TestIGMPNativeRepeatedAliasesOwnDistinctSourceRanges(t *testing.T) {
	message, err := hex.DecodeString("220012340000000201010002ef010203c0000201c0000202deadbeef06000000ef040506")
	require.NoError(t, err)
	w := nativeIGMPFrame(t, message)
	var events []*ProtocolEvent
	var flows int
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{w})), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnTrafficFlowCreated(func(*TrafficFlow) { flows++ })))
	require.Zero(t, flows)
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "igmp", e.Protocol)
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, w, e.Raw)
	require.Zero(t, e.FlowID)
	values := func(name string) []DisplayFieldValue {
		var result []DisplayFieldValue
		for _, v := range e.DisplayFieldValues().Values {
			if v.Name == name {
				result = append(result, v)
			}
		}
		return result
	}
	groups := values("igmp.maddr")
	require.Len(t, groups, 2)
	require.Equal(t, &DisplayFieldRange{Offset: 46, Length: 4}, groups[0].Range)
	require.Equal(t, &DisplayFieldRange{Offset: 66, Length: 4}, groups[1].Range)
	addresses := values("igmp.saddr")
	require.Len(t, addresses, 2)
	require.Equal(t, &DisplayFieldRange{Offset: 50, Length: 4}, addresses[0].Range)
	require.Equal(t, &DisplayFieldRange{Offset: 54, Length: 4}, addresses[1].Range)
	before := e.DisplayFieldValues()
	fieldsBefore, err := e.GetFields()
	require.NoError(t, err)
	e.Raw = []byte{1}
	e.Protocol = "tcp"
	e.Session = map[string]any{"IGMP": map[string]any{"Version": 7}}
	e.ID = 999
	e.Source = "203.0.113.1"
	require.Equal(t, before, e.DisplayFieldValues(), "public fields cannot rewrite ingress-owned values or ordinals")
	fieldsAfter, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, fieldsBefore, fieldsAfter)
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var forged ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &forged))
	for _, v := range forged.DisplayFieldValues().Values {
		require.NotContains(t, v.Name, "igmp.")
	}
}

func TestIGMPNativeRetainsPrefixWhenGopacketDoesNotPublishValidBody(t *testing.T) {
	for _, message := range [][]byte{{0x22, 0, 0x12, 0x34, 0, 0, 0, 2, 1, 0, 0, 3, 239, 1, 2, 3, 192, 0, 2, 1}, {0x12, 0, 0x12, 0x34}, {}} {
		w := nativeIGMPFrame(t, message)
		var events []*ProtocolEvent
		var stats ProtocolStats
		require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{w})), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
		require.Len(t, events, 1)
		e := events[0]
		require.Equal(t, "igmp", e.Protocol)
		require.Equal(t, "incomplete", e.Status)
		require.Zero(t, stats.Decoded)
		require.Zero(t, stats.Messages)
		require.Equal(t, w, e.Raw)
		for _, v := range e.DisplayFieldValues().Values {
			if v.Range != nil {
				require.LessOrEqual(t, v.Range.Offset+v.Range.Length, len(w))
			}
		}
		if len(message) == 0 {
			require.NotContains(t, e.Session, "Type")
		}
	}
}

func TestIGMPCapturedOwnDecodeErrorRetainsStrictAccountingWithAndWithoutProtocolHook(t *testing.T) {
	for _, allow := range []bool{false, true} {
		for _, callback := range []bool{false, true} {
			t.Run(fmt.Sprint(allow, "/", callback), func(t *testing.T) {
				w := nativeIGMPFrame(t, []byte{0x12, 0, 0, 0})
				var capture TCPReassemblyStats
				var events []*ProtocolEvent
				var observed int
				opts := []CaptureOption{WithTCPReassemblyWorkers(1), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allow, Stream: true}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { capture = s }), WithEveryPacket(func(gopacket.Packet) { observed++ })}
				if callback {
					opts = append(opts, WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
				}
				err := ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{w})), opts...)
				if allow {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.EqualValues(t, 1, capture.DecodeErrors)
				require.Equal(t, 1, observed)
				if callback {
					require.Len(t, events, 1)
					require.Equal(t, "igmp", events[0].Protocol)
				} else {
					require.Empty(t, events)
				}
			})
		}
	}
}

func TestIGMPParentAndOtherProtocolOwnershipCannotBeBypassed(t *testing.T) {
	// These are source-selection tests, not error-string or label matches.
	w := nativeIGMPFrame(t, []byte{0x12, 0, 0, 0, 239, 1, 2, 3})
	for _, which := range []string{"version", "fragment", "bad-options", "quoted", "unknown-type"} {
		t.Run(which, func(t *testing.T) {
			mutant := bytes.Clone(w)
			switch which {
			case "version":
				mutant[14] = 0x65
			case "fragment":
				mutant[21] = 1
			case "bad-options":
				mutant[14] = 0x46
				mutant = append(mutant[:34], append([]byte{0x82, 255, 0, 0}, mutant[34:]...)...)
			case "quoted":
				mutant[23] = 1 // outer ICMP owns its bytes, never inspect a quote as protocol2
			case "unknown-type":
				mutant[34] = 0x13 // DVMRP has its own profile
			}
			p := gopacket.NewPacket(mutant, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
			wire, _, _, ok := igmpIPv4Payload(p)
			if which == "unknown-type" {
				require.True(t, ok)
				require.Equal(t, byte(0x13), wire[0])
			} else {
				require.False(t, ok)
			}
		})
	}
}
