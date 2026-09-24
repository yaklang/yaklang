package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolSessionHTTPAdmissionValidatesStartLine(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	valid := []byte("PROPFIND /card/alice/ HTTP/1.1\r\n")
	p := s.Probe(valid)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "http", p.Protocol)

	for i := 0; i < len(valid); i++ {
		p = s.Probe(valid[:i])
		require.NotEqual(t, "http", p.Protocol, "accepted incomplete request line at byte %d", i)
	}

	for _, malformed := range [][]byte{
		[]byte("PROPFIND /card/alice/ HTTP/9.9\r\n"), // same-length version near miss
		[]byte("PROPFIND /card/alice/\t HTTP/1.1\r\n"),
		[]byte("PROPFIND /card/alice/ HTTP/1.1 extra\r\n"),
		[]byte("HTTP/1.1 20x Multi-Status\r\n"),
	} {
		p = s.Probe(malformed)
		require.NotEqual(t, ProbeAccept, p.Verdict, "%q", malformed)
		require.NotEqual(t, "http", p.Protocol, "%q", malformed)
	}

	for _, responseLine := range []string{
		"HTTP/1.1 207 Multi-Status\r\n",
		"HTTP/1.1 204 \r\n", // the reason phrase may be empty
	} {
		p = s.Probe([]byte(responseLine))
		require.Equal(t, ProbeAccept, p.Verdict)
		require.Equal(t, "http", p.Protocol)
	}
}

func TestProtocolSessionHTTPAdmissionSurvivesFragmentationAndRejectsBadVersion(t *testing.T) {
	ts := time.Unix(1, 0)
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	fragments := [][]byte{
		[]byte("PROPFIND /card/alice/ HTTP/1.1"),
		[]byte("\r"),
		[]byte("\nHost: lab.invalid\r\nContent-Length: 0\r\n\r\n"),
	}
	for i, fragment := range fragments {
		result := s.Feed(0, ts, fragment)
		if i < len(fragments)-1 {
			require.NotEqual(t, "http", result.State)
			require.Empty(t, result.Events)
			continue
		}
		require.Equal(t, "http", result.State)
		require.Nil(t, result.Err, "%v", result.Err)
		require.Len(t, result.Events, 1)
		require.Equal(t, "http", result.Events[0].Protocol)
		require.Equal(t, "decoded", result.Events[0].Status)
		require.Equal(t, "PROPFIND /card/alice/ HTTP/1.1", result.Events[0].Summary)
	}

	bad, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	invalid := []byte("PROPFIND /card/alice/ HTTP/9.9\r\nHost: lab.invalid\r\nContent-Length: 0\r\n\r\n")
	var allEvents []*ProtocolEvent
	for _, fragment := range [][]byte{invalid[:25], invalid[25:34], invalid[34:]} {
		result := bad.Feed(0, ts, fragment)
		allEvents = append(allEvents, result.Events...)
		require.NotEqual(t, "http", result.State)
	}
	for _, event := range allEvents {
		require.NotEqual(t, "http", event.Protocol, "%s: %s", event.Status, event.Summary)
	}
}
