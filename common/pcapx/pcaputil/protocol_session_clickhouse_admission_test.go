package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Synthetic native-protocol Hello packets use the documented field layouts,
// with a conventional client name and default database/user. No real server
// capture or credential is claimed by these fixtures.
func clickHouseAdmissionClientHello(password string) []byte {
	wire := []byte{0}
	wire = append(wire, clickHouseTestString("ClickHouse client")...)
	wire = append(wire, clickHouseTestVarUInt(23)...)
	wire = append(wire, clickHouseTestVarUInt(8)...)
	wire = append(wire, clickHouseTestVarUInt(54401)...)
	wire = append(wire, clickHouseTestString("default")...)
	wire = append(wire, clickHouseTestString("default")...)
	wire = append(wire, clickHouseTestString(password)...)
	return wire
}

func TestClickHouseHelloAmbiguityDoesNotDependOnSegmentation(t *testing.T) {
	server := clickHouseTestServerHello(23, 8, 54401)
	// A one-byte client password and the server's patch=1 have identical
	// prefixes. A following Pong can otherwise become the client's password.
	ambiguousClient := append(bytes.Clone(server), 'x')
	_, _, ok := parseClickHouseClientHello(ambiguousClient)
	require.True(t, ok)
	for name, wire := range map[string][]byte{
		"server-only":                         server,
		"server-and-pong":                     append(bytes.Clone(server), 4),
		"one-byte-password-client":            ambiguousClient,
		"conventional-name-one-byte-password": clickHouseAdmissionClientHello("x"),
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, "native-23.8-r54401/ambiguous-hello", probeClickHouse(wire, 64).Version)
			for split := 0; split <= len(wire); split++ {
				s, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				var events []*ProtocolEvent
				for _, part := range [][]byte{wire[:split], wire[split:]} {
					if len(part) == 0 {
						continue
					}
					events = append(events, s.Feed(1, time.Unix(1, 0), part).Events...)
				}
				require.Len(t, events, 1, "split=%d", split)
				require.Equal(t, "clickhouse", events[0].Protocol, "split=%d", split)
				require.Equal(t, "context-required", events[0].Status, "split=%d: %s", split, events[0].Summary)
				require.Contains(t, events[0].Summary, "Hello role is ambiguous")
				require.Empty(t, events[0].Fields["Role"])
			}
		})
	}
}

func TestClickHouseConventionalClientFirstCoalescedCapture(t *testing.T) {
	for _, password := range []string{"", "synthetic-password"} {
		hello := clickHouseAdmissionClientHello(password)
		require.Equal(t, "native-23.8-r54401/client-hello", probeClickHouse(hello, 64).Version)
		steps := []sessionStep{
			{0, append(bytes.Clone(hello), 4)},
			{1, append(clickHouseTestServerHello(23, 8, 54401), 4)},
		}
		for _, chunk := range []int{0, 1, 7, 64} {
			t.Run(fmt.Sprintf("password-length=%d/chunk=%d", len(password), chunk), func(t *testing.T) {
				// Explicitly synthetic PCAP, including Hello+Ping and Hello+Pong in
				// one TCP segment when chunk=0 and arbitrary reassembly splits otherwise.
				raw := sessionTestPCAP(t, steps, layers.TCPPort(9000), chunk, false, true)
				var events []*ProtocolEvent
				require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
				require.Len(t, events, 4)
				var names []string
				for _, e := range events {
					require.Equal(t, "clickhouse", e.Protocol)
					require.Equal(t, "decoded", e.Status, "%s", e.Summary)
					names = append(names, e.Fields["Packet Name"].(string))
				}
				require.Equal(t, []string{"Hello", "Ping", "Hello", "Pong"}, names)
				require.Equal(t, "client", events[0].Fields["Role"])
				require.Equal(t, "ClickHouse client", events[0].Fields["Client Name"])
				require.Equal(t, "server", events[2].Fields["Role"])
				require.Equal(t, "matched", events[3].Fields["Request Association"])
			})
		}
	}
}
