package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func icmp4UnionFrame(t *testing.T, typ, code byte, word [4]byte, body []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP("198.51.100.10"), DstIP: net.ParseIP("198.51.100.20")}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(typ, code), Id: binary.BigEndian.Uint16(word[:2]), Seq: binary.BigEndian.Uint16(word[2:])}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, icmp, gopacket.Payload(body)))
	return bytes.Clone(b.Bytes())
}

func icmp4UnionReplay(t *testing.T, frame []byte) (*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})), WithTCPReassemblyWorkers(2), WithProtocolDeferred(true), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "icmp", e.Protocol)
	require.Equal(t, frame, e.Raw)
	require.Equal(t, "198.51.100.10", e.Source)
	require.Equal(t, "198.51.100.20", e.Destination)
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Len(t, e.SourceBytes.PacketRefs, 1)
	require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
	return e, stats
}

func TestICMPv4RestHeaderUnionDoesNotInventEchoIdentity(t *testing.T) {
	quote := append([]byte{0x40}, make([]byte, 19)...)
	for _, tc := range []struct {
		name      string
		typ, code byte
		word      [4]byte
		want      map[string]any
	}{
		{"echo", 8, 0, [4]byte{0x12, 0x34, 0x56, 0x78}, map[string]any{"Identifier": uint16(0x1234), "Sequence": uint16(0x5678)}},
		{"MTU", 3, 4, [4]byte{0x12, 0x34, 0x05, 0xd4}, map[string]any{"MTU": uint16(1492)}},
		{"unreachable-not-MTU", 3, 3, [4]byte{0x12, 0x34, 0x56, 0x78}, nil},
		{"redirect", 5, 1, [4]byte{192, 0, 2, 254}, map[string]any{"Gateway": "192.0.2.254"}},
		{"time-exceeded", 11, 0, [4]byte{0x12, 0x34, 0x56, 0x78}, nil},
		{"parameter-problem", 12, 0, [4]byte{23, 1, 2, 3}, map[string]any{"Pointer": uint8(23)}},
		{"source-quench-observed", 4, 0, [4]byte{0x12, 0x34, 0x56, 0x78}, nil},
		{"unknown-type-opaque", 250, 17, [4]byte{0x12, 0x34, 0x56, 0x78}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, stats := icmp4UnionReplay(t, icmp4UnionFrame(t, tc.typ, tc.code, tc.word, quote))
			require.Equal(t, "decoded", e.Status)
			require.EqualValues(t, 1, stats.Decoded)
			want := map[string]any{"Type": tc.typ, "Code": tc.code}
			for k, v := range tc.want {
				want[k] = v
			}
			if tc.typ != 8 {
				want["Rest of Header"] = tc.word[:]
			}
			if tc.typ == 3 || tc.typ == 4 || tc.typ == 5 || tc.typ == 11 || tc.typ == 12 {
				want["Quoted Packet"] = quote
			}
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, f)
			corruptNetworkObservation(f)
			corruptNetworkObservation(e.Session)
			f, err = e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, f)
		})
	}
}

func TestICMPv4TimestampAndMaskNeedTheirOwnFixedBody(t *testing.T) {
	words := []byte{1, 2, 3, 4, 0x80, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	for _, typ := range []byte{13, 14, 17, 18} {
		size := 12
		if typ >= 17 {
			size = 4
		}
		for n := 0; n <= size; n++ {
			t.Run(fmt.Sprintf("type%d/bytes%d", typ, n), func(t *testing.T) {
				e, stats := icmp4UnionReplay(t, icmp4UnionFrame(t, typ, 0, [4]byte{0x12, 0x34, 0x56, 0x78}, words[:n]))
				want := map[string]any{"Type": typ, "Code": byte(0), "Identifier": uint16(0x1234), "Sequence": uint16(0x5678)}
				if typ < 17 {
					for i, key := range []string{"Originate Timestamp", "Receive Timestamp", "Transmit Timestamp"} {
						if n >= 4*(i+1) {
							want[key] = []uint32{0x01020304, 0x80000000, 0xffffffff}[i]
						}
					}
				} else if n == 4 {
					want["Address Mask"] = "1.2.3.4"
				}
				if n < size {
					want["Available ICMP Bytes"], want["Required ICMP Header Bytes"] = 8+n, 8+size
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, "incomplete", e.Completeness)
					require.Equal(t, "ICMPHeaderTruncated", e.ExpertCode)
					require.Equal(t, "icmp-header-observation", e.Profile)
					require.Zero(t, stats.Decoded)
					require.Zero(t, stats.Messages)
					require.EqualValues(t, 1, stats.Incomplete)
				} else {
					require.Equal(t, "decoded", e.Status)
					require.EqualValues(t, 1, stats.Decoded)
				}
				f, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
				corruptNetworkObservation(f)
				corruptNetworkObservation(e.Session)
				f, err = e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, f)
			})
		}
	}
}
