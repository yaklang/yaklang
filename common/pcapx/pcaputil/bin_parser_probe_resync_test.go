package pcaputil

import (
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTCPProbeResyncEnabledByDefault(t *testing.T) {
	request := "GET /download.html HTTP/1.1\r\nHost: example.test\r\nUser-Agent: probe-resync-test\r\n\r\n"
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: request},
		{seq: 2 + uint32(len(request)), fin: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "http", event.Protocol)
	require.Equal(t, "decoded", event.Status, event.Error)
	require.Equal(t, request, string(event.Raw))
	require.EqualValues(t, 1, event.Offset)
	require.Equal(t, []PacketReference{{Number: 3, Domain: event.Domain}}, event.SourceBytes.PacketRefs)
	require.NotNil(t, event.SourceBytes.CompletedIn)
	require.EqualValues(t, 3, event.SourceBytes.CompletedIn.Number)
	require.EqualValues(t, 1, stats.UnclassifiedBytes)
	require.Zero(t, stats.Unknown)
	require.GreaterOrEqual(t, stats.ProbeCalls, uint64(2))
}

func TestTCPProbeResyncCanBeDisabled(t *testing.T) {
	request := "GET /download.html HTTP/1.1\r\nHost: example.test\r\nUser-Agent: probe-resync-test\r\n\r\n"
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: request},
		{seq: 2 + uint32(len(request)), fin: true},
	}

	events, _, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1, WithTCPProbeResync(false))
	require.NoError(t, err)
	for _, event := range events {
		require.NotEqual(t, "http", event.Protocol)
	}
}

func TestTCPProbeResyncKeepsMessageAcrossLaterBoundaries(t *testing.T) {
	request := "GET /split.html HTTP/1.1\r\nHost: example.test\r\nUser-Agent: probe-resync-test\r\n\r\n"
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: request[:2]},
		{seq: 4, data: request[2:]},
		{seq: 2 + uint32(len(request)), fin: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "http", event.Protocol)
	require.Equal(t, "decoded", event.Status, event.Error)
	require.Equal(t, request, string(event.Raw))
	require.Equal(t, []PacketReference{
		{Number: 3, Domain: event.Domain},
		{Number: 4, Domain: event.Domain},
	}, event.SourceBytes.PacketRefs)
	require.EqualValues(t, 1, stats.UnclassifiedBytes)
	require.Zero(t, stats.Unknown)
}

func TestTCPProbeResyncKeepsPartialCandidateAcrossLaterFrames(t *testing.T) {
	request := "GET /" + strings.Repeat("a", 256) + " HTTP/1.1\r\nHost: example.test\r\n\r\n"
	split := 128
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: request[:split]},
		{seq: 2 + uint32(split), data: request[split:]},
		{seq: 2 + uint32(len(request)), fin: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "http", event.Protocol)
	require.Equal(t, "decoded", event.Status, event.Error)
	require.Equal(t, request, string(event.Raw))
	require.Equal(t, []PacketReference{
		{Number: 3, Domain: event.Domain},
		{Number: 4, Domain: event.Domain},
	}, event.SourceBytes.PacketRefs)
	require.EqualValues(t, 1, stats.UnclassifiedBytes)
	require.Zero(t, stats.Unknown)
}

func TestTCPProbeResyncDoesNotScanInsideDelivery(t *testing.T) {
	request := "GET /hidden-after-junk.html HTTP/1.1\r\nHost: example.test\r\nX-Padding: " + strings.Repeat("a", 128) + "\r\n\r\n"
	prefix := "\x00" + request
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: prefix},
		{seq: 1 + uint32(len(prefix)), fin: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "unrecognized", events[0].Status)
	require.Equal(t, true, events[0].Session["Detection Stopped"])
	require.EqualValues(t, 1, stats.Unknown)
	require.Positive(t, stats.UnclassifiedBytes)
}

func TestTCPProbeResyncUsesProtocolAdmissionNotHTTPDetection(t *testing.T) {
	wire := bgpTestWire(4, nil)
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: string(wire)},
		{seq: 2 + uint32(len(wire)), fin: true},
	}

	events, stats, err := binReplay(t, binTestPcap(t, steps, layers.TCPPort(179), false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "bgp", event.Protocol, "%+v", events)
	require.Equal(t, "decoded", event.Status, event.Error)
	require.Equal(t, wire, event.Raw)
	require.EqualValues(t, 1, event.Offset)
	require.Equal(t, []PacketReference{{Number: 3, Domain: event.Domain}}, event.SourceBytes.PacketRefs)
	require.EqualValues(t, 1, stats.UnclassifiedBytes)
	require.Zero(t, stats.Unknown)
}

func TestTCPProbeResyncRejectsWeakLengthProtocolAtLaterBoundary(t *testing.T) {
	wire := kafkaRequest(18, 0, 1, "", nil)
	steps := []tcpStep{
		{seq: 0, syn: true},
		{seq: 1, data: "\x00"},
		{seq: 2, data: string(wire)},
		{seq: 2 + uint32(len(wire)), fin: true},
	}

	events, _, err := binReplay(t, binTestPcap(t, steps, layers.TCPPort(9092), false, false), 1)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	for _, event := range events {
		require.NotEqual(t, "kafka", event.Protocol)
	}
}
