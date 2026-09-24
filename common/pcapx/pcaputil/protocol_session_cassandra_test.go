package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func cassandraReviewSession(t *testing.T, ports [2]uint16) *captureSession {
	t.Helper()
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	capture := session.(*captureSession)
	capture.f.ports = ports
	return capture
}

func cassandraReviewEnvelope(version, flags, opcode byte, body []byte) []byte {
	wire := make([]byte, 9, 9+len(body))
	wire[0], wire[1], wire[4] = version, flags, opcode
	binary.BigEndian.PutUint32(wire[5:], uint32(len(body)))
	return append(wire, body...)
}

func cassandraReviewStartupBody() []byte {
	body := []byte{0, 1, 0, 11}
	body = append(body, "CQL_VERSION"...)
	body = append(body, 0, 5)
	return append(body, "3.0.0"...)
}

func TestCassandraInitialExchangeAdmission(t *testing.T) {
	valid := []struct {
		name string
		wire []byte
	}{
		{"v4-options-request", cassandraReviewEnvelope(4, 0, 5, nil)},
		{"v4-startup-request", cassandraReviewEnvelope(4, 0, 1, cassandraReviewStartupBody())},
		{"v4-supported-response", cassandraReviewEnvelope(0x84, 0, 6, []byte{0, 0})},
		{"v4-ready-response", cassandraReviewEnvelope(0x84, 0, 2, nil)},
		{"v5-options-request", cassandraReviewEnvelope(5, 0, 5, nil)},
		{"v5-startup-request", cassandraReviewEnvelope(5, 0, 1, cassandraReviewStartupBody())},
		{"v5-ignored-compression-flag", cassandraReviewEnvelope(5, 1, 5, nil)},
		{"v5-supported-response", cassandraReviewEnvelope(0x85, 0, 6, []byte{0, 0})},
		{"v5-ready-response", cassandraReviewEnvelope(0x85, 0, 2, nil)},
		{"v5-authenticate-response", cassandraReviewEnvelope(0x85, 0, 3, []byte{0, 0})},
		{"v5-error-response", cassandraReviewEnvelope(0x85, 0, 0, []byte{0, 0, 0, 0, 0, 0})},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			probe := cassandraReviewSession(t, [2]uint16{49152, 9042}).Probe(tc.wire)
			require.Equal(t, ProbeAccept, probe.Verdict)
			require.Equal(t, "cassandra", probe.Protocol)
		})
	}

	invalid := []struct {
		name string
		wire []byte
	}{
		{"request-version-with-response-opcode", cassandraReviewEnvelope(4, 0, 6, []byte{0, 0})},
		{"response-version-with-startup", cassandraReviewEnvelope(0x84, 0, 1, []byte{0, 0})},
		{"response-version-with-options", cassandraReviewEnvelope(0x85, 0, 5, nil)},
		{"non-initial-query", cassandraReviewEnvelope(5, 0, 7, nil)},
		{"compressed-v4-startup", cassandraReviewEnvelope(4, 1, 1, cassandraReviewStartupBody())},
		{"negative-stream", func() []byte {
			wire := cassandraReviewEnvelope(5, 0, 5, nil)
			wire[2] = 0x80
			return wire
		}()},
		{"negative-body-length", func() []byte {
			wire := cassandraReviewEnvelope(5, 0, 5, nil)
			binary.BigEndian.PutUint32(wire[5:9], 0xffffffff)
			return wire
		}()},
		{"impossible-options-body", cassandraReviewEnvelope(5, 0, 5, []byte{0})},
		{"truncated-header", cassandraReviewEnvelope(5, 0, 5, nil)[:8]},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			probe := cassandraReviewSession(t, [2]uint16{49152, 9042}).Probe(tc.wire)
			require.NotEqual(t, "cassandra", probe.Protocol)
		})
	}

	// A Cassandra-shaped header alone is not enough to claim Cassandra on a
	// non-native port; the candidate remains gated to port 9042.
	offPort := cassandraReviewSession(t, [2]uint16{49152, 9142}).Probe(valid[6].wire)
	require.NotEqual(t, "cassandra", offPort.Protocol)
}

func TestCassandraInitialExchangeFragmentAndUnsupportedResponse(t *testing.T) {
	startup := cassandraReviewEnvelope(5, 0, 1, cassandraReviewStartupBody())
	probe := cassandraReviewSession(t, [2]uint16{49152, 9042})
	incomplete := probe.Probe(startup[:9])
	require.Equal(t, ProbeNeedMore, incomplete.Verdict)
	require.Equal(t, "cassandra", incomplete.Protocol)
	require.Equal(t, len(startup)-9, incomplete.NeedBytes)
	require.Equal(t, ProbeAccept, probe.Probe(startup).Verdict)

	session := cassandraReviewSession(t, [2]uint16{49152, 9042})
	first := session.Feed(0, time.Now(), startup[:4])
	require.True(t, first.NeedMore)
	require.Equal(t, "undetected", first.State)
	second := session.Feed(0, time.Now(), startup[4:])
	require.Equal(t, "cassandra", second.State)
	require.Len(t, second.Events, 1)
	require.Equal(t, "context-required", second.Events[0].Status)

	// READY is a valid v5 initial response. It must be classified as
	// Cassandra, but this profile intentionally does not decode v5 sessions.
	ready := cassandraReviewSession(t, [2]uint16{49152, 9042})
	response := cassandraReviewEnvelope(0x85, 0, 2, nil)
	result := ready.Feed(1, time.Now(), response)
	require.Equal(t, "cassandra", result.State)
	require.Len(t, result.Events, 1)
	require.Equal(t, "context-required", result.Events[0].Status)

	// The exact same v4 OPTIONS envelope is a supported, complete positive.
	v4 := cassandraReviewSession(t, [2]uint16{49152, 9042})
	result = v4.Feed(0, time.Now(), cassandraReviewEnvelope(4, 0, 5, nil))
	require.Equal(t, "cassandra", result.State)
	require.Len(t, result.Events, 1)
	require.Equal(t, "decoded", result.Events[0].Status)
}
