package pcaputil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func clickHouseTestVarUInt(value uint64) []byte {
	var wire []byte
	for value >= 0x80 {
		wire = append(wire, byte(value)|0x80)
		value >>= 7
	}
	return append(wire, byte(value))
}

func clickHouseTestString(value string) []byte {
	wire := clickHouseTestVarUInt(uint64(len(value)))
	return append(wire, value...)
}

func clickHouseTestClientHello(major, minor, revision uint64) []byte {
	wire := clickHouseTestVarUInt(0) // Client Hello packet type.
	wire = append(wire, clickHouseTestString("winlab-ch")...)
	wire = append(wire, clickHouseTestVarUInt(major)...)
	wire = append(wire, clickHouseTestVarUInt(minor)...)
	wire = append(wire, clickHouseTestVarUInt(revision)...)
	wire = append(wire, clickHouseTestString("lab")...)
	wire = append(wire, clickHouseTestString("winlab")...)
	wire = append(wire, clickHouseTestString("")...)
	return wire
}

func clickHouseTestServerHello(major, minor, revision uint64) []byte {
	wire := clickHouseTestVarUInt(0) // Server Hello packet type required by native protocol.
	wire = append(wire, clickHouseTestString("lab-clickhouse")...)
	wire = append(wire, clickHouseTestVarUInt(major)...)
	wire = append(wire, clickHouseTestVarUInt(minor)...)
	wire = append(wire, clickHouseTestVarUInt(revision)...)
	wire = append(wire, clickHouseTestString("UTC")...)
	wire = append(wire, clickHouseTestString("lab")...)
	wire = append(wire, clickHouseTestVarUInt(1)...)
	return wire
}

func TestClickHouseProbeRequiresACompleteSupportedClientHello(t *testing.T) {
	valid := clickHouseTestClientHello(23, 8, 54401)
	p := probeClickHouse(valid, DefaultParserBudget().ProbeBytes)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "clickhouse", p.Protocol)
	require.Equal(t, "native-23.8-r54401/client-hello", p.Version)

	for n := 0; n < len(valid); n++ {
		p := probeClickHouse(valid[:n], DefaultParserBudget().ProbeBytes)
		require.NotEqual(t, ProbeAccept, p.Verdict, "accepted incomplete Hello prefix length=%d", n)
		require.NotEqual(t, "clickhouse", p.Protocol, "claimed incomplete Hello prefix length=%d", n)
	}

	nearMatches := map[string][]byte{
		"wrong major":    clickHouseTestClientHello(24, 8, 54401),
		"wrong minor":    clickHouseTestClientHello(23, 9, 54401),
		"wrong revision": clickHouseTestClientHello(23, 8, 54402),
		"wrong packet type": func() []byte {
			wire := append([]byte(nil), valid...)
			wire[0] = 1
			return wire
		}(),
		"empty client name": func() []byte {
			wire := append([]byte(nil), valid...)
			wire[1] = 0
			return wire
		}(),
		"overlong string length": func() []byte {
			wire := append([]byte(nil), valid[:1]...)
			wire = append(wire, 0x89, 0x00)
			wire = append(wire, valid[2:]...)
			return wire
		}(),
	}
	for name, wire := range nearMatches {
		require.NotEqual(t, ProbeAccept, probeClickHouse(wire, DefaultParserBudget().ProbeBytes).Verdict, name)
		require.NotEqual(t, "clickhouse", probeWire(wire, DefaultParserBudget().ProbeBytes).Protocol, name)
	}
	for name, wire := range map[string][]byte{
		"Kafka Produce":      kafkaProduceV0SimilarPrefixFixture(),
		"PostgreSQL startup": pgSessionUntyped(196608, []byte("user\x00test\x00\x00")),
	} {
		require.NotEqual(t, "clickhouse", probeWire(wire, DefaultParserBudget().ProbeBytes).Protocol, name)
	}
}

func TestClickHouseNativeHelloPingAndPongWithCorrectServerType(t *testing.T) {
	steps := []sessionStep{
		{dir: 0, wire: clickHouseTestClientHello(23, 8, 54401)},
		{dir: 0, wire: clickHouseTestVarUInt(4)},
		{dir: 1, wire: clickHouseTestServerHello(23, 8, 54401)},
		{dir: 1, wire: clickHouseTestVarUInt(4)},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		events, _ := sessionTestFlow(t, "clickhouse", steps, chunk, false)
		names := make([]string, 0, len(events))
		for _, event := range events {
			require.Equal(t, "clickhouse", event.Protocol)
			require.Equal(t, "decoded", event.Status, "%s: %s", event.Summary, event.Error)
			names = append(names, event.Fields["Packet Name"].(string))
		}
		return names
	})

	events, _ := sessionTestFlow(t, "clickhouse", steps, 0, false)
	require.Len(t, events, 4)
	require.Equal(t, []string{"Hello", "Ping", "Hello", "Pong"}, []string{
		events[0].Fields["Packet Name"].(string),
		events[1].Fields["Packet Name"].(string),
		events[2].Fields["Packet Name"].(string),
		events[3].Fields["Packet Name"].(string),
	})
	require.Equal(t, "winlab-ch", events[0].Fields["Client Name"])
	require.Equal(t, "winlab", events[0].Fields["User"])
	require.Equal(t, "lab", events[0].Fields["Database"])
	require.Equal(t, "[redacted]", events[0].Fields["Password"])
	require.Equal(t, "lab-clickhouse", events[2].Fields["Server Name"])
	require.Equal(t, uint64(54401), events[2].Fields["Revision"])
	require.Equal(t, "UTC", events[2].Fields["Timezone"])
	require.Equal(t, "matched", events[3].Fields["Request Association"])
}

func TestWinlab5013ClickHouseCaptureFailsClosedOnMalformedServerHello(t *testing.T) {
	var clickhouse []*ProtocolEvent
	for _, event := range replayWinlab5013Protocols(t, "15-clickhouse.pcapng") {
		if event.Protocol == "clickhouse" {
			clickhouse = append(clickhouse, event)
		}
	}
	require.NotEmpty(t, clickhouse)
	require.Equal(t, "decoded", clickhouse[0].Status)
	require.Equal(t, "Hello", clickhouse[0].Fields["Packet Name"])
	require.Equal(t, "client", clickhouse[0].Fields["Role"])
	if len(clickhouse) > 1 {
		require.Equal(t, "decoded", clickhouse[1].Status)
		require.Equal(t, "Ping", clickhouse[1].Fields["Packet Name"])
	}
	for _, event := range clickhouse {
		require.NotEqual(t, "server", event.Fields["Role"], "the original fixture omits the server Hello packet type")
		require.NotEqual(t, "Pong", event.Fields["Packet Name"], "the malformed server Hello must not establish a valid response phase")
	}
	require.Len(t, clickhouse, 3)
	require.Equal(t, "malformed", clickhouse[2].Status)
	require.Contains(t, clickhouse[2].Summary, "not a valid native Hello")
}

func TestWinlab5013CorrectedClickHouseCaptureDecodesNativeHelloPingPong(t *testing.T) {
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "corrected", "15-clickhouse-valid.pcapng")
	var events []*ProtocolEvent
	err := ReplayPcapFile(path,
		WithTCPReassemblyWorkers(1),
		WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }),
	)
	require.NoError(t, err)
	require.Len(t, events, 4)
	for _, event := range events {
		require.Equal(t, "clickhouse", event.Protocol, "%s: %s", event.Status, event.Summary)
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Summary)
	}
	require.Equal(t, []string{"Hello", "Ping", "Hello", "Pong"}, []string{
		events[0].Fields["Packet Name"].(string),
		events[1].Fields["Packet Name"].(string),
		events[2].Fields["Packet Name"].(string),
		events[3].Fields["Packet Name"].(string),
	})
	require.Equal(t, "winlab-ch", events[0].Fields["Client Name"])
	require.Equal(t, "winlab", events[0].Fields["User"])
	require.Equal(t, "lab", events[0].Fields["Database"])
	require.Equal(t, "lab-clickhouse", events[2].Fields["Server Name"])
	require.Equal(t, "UTC", events[2].Fields["Timezone"])
	require.Equal(t, "matched", events[3].Fields["Request Association"])
}
