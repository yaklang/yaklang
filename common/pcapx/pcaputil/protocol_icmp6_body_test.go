package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

func icmp6BodyFrame(typ byte, body []byte) []byte {
	f := make([]byte, 14+40+4+len(body))
	copy(f, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0x86, 0xdd})
	ip := f[14:54]
	ip[0], ip[6], ip[7] = 0x60, 58, 255
	binary.BigEndian.PutUint16(ip[4:6], uint16(4+len(body)))
	copy(ip[8:24], net.ParseIP("2001:db8::10").To16())
	copy(ip[24:40], net.ParseIP("2001:db8::20").To16())
	f[54] = typ
	copy(f[58:], body)
	return f
}

// These prefixes contain the whole generic ICMPv6 header but omit required
// type-specific bytes. CaptureLength == Length does not prove capture loss.
func TestICMPv6OwnBodyBounds(t *testing.T) {
	for _, tc := range []struct {
		typ      byte
		required int
	}{
		{1, 4}, {2, 4}, {3, 4}, {4, 4}, {128, 4}, {129, 4},
		{133, 4}, {134, 12}, {135, 20}, {136, 20}, {137, 36},
	} {
		for n := 0; n < tc.required; n++ {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("type%d/body%d/deferred%t", tc.typ, n, deferred), func(t *testing.T) {
					frame := icmp6BodyFrame(tc.typ, bytes.Repeat([]byte{0x71}, n))
					var events []*ProtocolEvent
					var stats ProtocolStats
					seen := 0
					require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})),
						WithTCPReassemblyWorkers(2), WithProtocolDeferred(deferred),
						WithEveryPacket(func(p gopacket.Packet) { seen++; require.Equal(t, frame, p.Data()) }),
						WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
						WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
					require.Equal(t, 1, seen)
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, "icmpv6", e.Protocol)
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, "incomplete", e.Completeness)
					require.Equal(t, "ICMPv6HeaderTruncated", e.ExpertCode)
					require.Equal(t, "2001:db8::10", e.Source)
					require.Equal(t, "2001:db8::20", e.Destination)
					require.Equal(t, frame, e.Raw)
					require.Zero(t, e.FlowID)
					require.Zero(t, e.ResponseTo)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
					f, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, tc.typ, f["Type"])
					require.EqualValues(t, n+4, f["Available ICMPv6 Bytes"])
					require.EqualValues(t, tc.required+4, f["Required ICMPv6 Header Bytes"])
					for _, absent := range []string{"Identifier", "Sequence", "MTU", "Quoted Packet", "Destination", "Options"} {
						require.NotContains(t, f, absent)
					}
					if tc.typ != 137 || n < 20 {
						require.NotContains(t, f, "Target")
					} else {
						require.Equal(t, "7171:7171:7171:7171:7171:7171:7171:7171", f["Target"])
					}
					corruptNetworkObservation(f)
					corruptNetworkObservation(e.Session)
					f, err = e.GetFields()
					require.NoError(t, err)
					require.Equal(t, tc.typ, f["Type"])
					require.Zero(t, stats.Decoded)
					require.Zero(t, stats.Messages)
					require.EqualValues(t, 1, stats.Incomplete)
				})
			}
		}
	}
}

func TestICMPv6RedirectOptionsAndQuotedError(t *testing.T) {
	redirect := make([]byte, 36)
	copy(redirect[4:20], net.ParseIP("fe80::9").To16())
	copy(redirect[20:36], net.ParseIP("2001:db8::5").To16())
	goodOption := []byte{2, 1, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60}
	for _, tc := range []struct {
		name         string
		typ          byte
		body         []byte
		status, code string
	}{
		{"redirect-empty-options", 137, redirect, "decoded", ""},
		{"redirect-valid-option", 137, append(bytes.Clone(redirect), goodOption...), "decoded", ""},
		{"redirect-zero-option", 137, append(bytes.Clone(redirect), 2, 0), "malformed", "NDOptionLength"},
		{"redirect-short-option", 137, append(bytes.Clone(redirect), 2), "malformed", "NDOptionLength"},
		{"redirect-overrun-option", 137, append(bytes.Clone(redirect), 2, 1, 1), "malformed", "NDOptionLength"},
		{"redirect-valid-prefix-bad-tail", 137, append(append(bytes.Clone(redirect), goodOption...), 2, 0), "malformed", "NDOptionLength"},
		{"echo-empty-data", 128, []byte{0x12, 0x34, 0x56, 0x78}, "decoded", ""},
		{"too-big-empty-quote", 2, []byte{0, 0, 5, 0}, "decoded", ""},
		{"error-bad-quoted-ip", 1, append([]byte{0, 0, 0, 0}, bytes.Repeat([]byte{0x40}, 20)...), "decoded", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []*ProtocolEvent
			frame := icmp6BodyFrame(tc.typ, tc.body)
			require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{frame})), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, "icmpv6", e.Protocol)
			require.Equal(t, tc.status, e.Status)
			require.Equal(t, tc.code, e.ExpertCode)
			require.Equal(t, frame, e.Raw)
			f, err := e.GetFields()
			require.NoError(t, err)
			if tc.typ == 137 {
				require.Equal(t, "fe80::9", f["Target"])
				require.Equal(t, "2001:db8::5", f["Destination"])
				opts := f["Options"].([]map[string]any)
				if tc.name == "redirect-valid-option" || tc.name == "redirect-valid-prefix-bad-tail" {
					require.Equal(t, []map[string]any{{"Type": byte(2), "Value": goodOption[2:]}}, opts)
				} else {
					require.Empty(t, opts)
				}
			}
			if tc.typ == 128 {
				require.Equal(t, uint16(0x1234), f["Identifier"])
				require.Equal(t, uint16(0x5678), f["Sequence"])
			}
			if tc.typ == 1 {
				require.Equal(t, tc.body[4:], f["Quoted Packet"])
			}
			if tc.typ == 2 {
				require.Equal(t, uint32(1280), f["MTU"])
			}
		})
	}
}
