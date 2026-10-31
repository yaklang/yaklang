package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestMembershipNativeUsesLiteralIPLengthInsteadOfDecoderCompatibilityLength(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, container string
		message                   []byte
		query                     bool
	}{
		{"IGMP-query", "igmp", "IGMP", []byte{0x11, 0x82, 0, 0, 0, 0, 0, 0, 0xff, 0x95, 0, 0}, true},
		{"IGMP-report", "igmp", "IGMP", []byte{0x22, 0, 0, 0, 0, 0, 0, 0}, false},
		{"MLD-query", "icmpv6", "MLD", append([]byte{130, 0, 0, 0, 0xff, 0xff, 0, 0}, make([]byte, 20)...), true},
		{"MLD-report", "icmpv6", "MLD", []byte{143, 0, 0, 0, 0, 0, 0, 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := nativeIGMPFrame(t, tc.message)
			if tc.protocol == "icmpv6" {
				wire[23] = byte(layers.IPProtocolICMPv6)
			}
			binary.BigEndian.PutUint16(wire[16:18], 0)
			p := gopacket.NewPacket(wire, protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
			ip, ok := p.NetworkLayer().(*layers.IPv4)
			require.True(t, ok)
			require.NotZero(t, ip.Length, "this dependency synthesizes Length from captured input")
			require.Zero(t, binary.BigEndian.Uint16(ip.LayerContents()[2:4]), "the actual word is still zero")
			var events []*ProtocolEvent
			var stats ProtocolStats
			require.NoError(t, ReplayPcap(bytes.NewReader(icmpFragmentCapture(t, [][]byte{wire})),
				WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true}),
				WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }),
				WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, tc.protocol, e.Protocol)
			require.Equal(t, "limited", e.Status)
			require.Nil(t, e.SourceBytes.CompletedIn)
			require.Zero(t, stats.Decoded)
			require.Zero(t, stats.Messages)
			if tc.query {
				fields := e.Session[tc.container].(map[string]any)
				require.NotContains(t, fields, "Version")
				require.NotContains(t, fields, "Maximum Response Time (ms)")
				require.NotContains(t, fields, "Maximum Response Delay (ms)")
				require.NotContains(t, fields, "Source Count")
			}
		})
	}
}
