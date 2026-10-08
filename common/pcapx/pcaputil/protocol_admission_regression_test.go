package pcaputil

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestAdmissionRejectsBareWebSocket(t *testing.T) {
	// Valid frame bytes are insufficient to establish an HTTP-upgraded session.
	for _, raw := range [][]byte{{0x81, 0}, {0x82, 3, 1, 2, 3}, {0x81, 0x82, 1, 2, 3, 4, 0x69, 0x6b}} {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		require.NotEqual(t, "websocket", s.Probe(raw).Protocol)
		r := s.Feed(0, time.Unix(1, 0), raw)
		for _, e := range append(r.Events, s.Close("test")...) {
			require.NotEqual(t, "websocket", e.Protocol)
		}
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestAdmissionMySQLRequiresGreetingStructure(t *testing.T) {
	raw := make([]byte, 40)
	raw[0] = 36
	raw[4] = 10
	copy(raw[5:], []byte("reply\x00"))
	require.NotEqual(t, ProbeAccept, probeMySQL(raw, 64).Verdict)
	raw[5] = '8'
	raw[10] = 0
	require.NotEqual(t, ProbeAccept, probeMySQL(raw, 64).Verdict, "numeric text without protocol-41 capabilities")
}

func TestAdmissionLongFTPBanner(t *testing.T) {
	raw := []byte("220 FTP service " + strings.Repeat("x", 120) + "\r\n")
	for _, chunk := range []int{1, 7, len(raw)} {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		require.Equal(t, "ftp", s.Probe(raw).Protocol)
		var events []*ProtocolEvent
		for at := 0; at < len(raw); at += chunk {
			events = append(events, s.Feed(1, time.Unix(1, 0), raw[at:min(at+chunk, len(raw))]).Events...)
		}
		events = append(events, s.Close("test")...)
		require.Len(t, events, 1)
		require.Equal(t, "ftp", events[0].Protocol)
		require.Empty(t, events[0].Error)
		require.Equal(t, raw, events[0].Raw)
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
