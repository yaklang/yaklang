package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Keep callers that store the original constructor as a function value compiling.
var _ func(ParserBudget) (ProtocolSession, error) = NewProtocolSession

func TestProtocolSessionObservedTransport(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport(transport), WithSessionPorts(19555, 40000))
		require.NoError(t, err)
		if transport == "udp" {
			q := dnsQuery(123, "transport.example", 1)[2:]
			require.Equal(t, "dns", s.Probe(q).Protocol)
			r := s.Feed(0, time.Unix(1, 0), q)
			require.Nil(t, r.Err)
			require.False(t, r.NeedMore)
			require.Len(t, r.Events, 1)
			require.Equal(t, "udp", r.Events[0].Transport)
			require.Equal(t, "dns", r.Events[0].Protocol)
			response := dnsAResponse(123, "transport.example", [4]byte{192, 0, 2, 9})[2:]
			r = s.Feed(1, time.Unix(2, 0), response)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, "matched", r.Events[0].Session["Association Status"])
			require.NotEmpty(t, r.Events[0].Session["Matched Request"])
		} else {
			for _, raw := range [][]byte{{0x40, 1, 0, 9}, ntpFixtureRequest(), {0x81, 0}} {
				require.NotEqual(t, ProbeAccept, s.Probe(raw).Verdict)
				for _, e := range s.Feed(0, time.Unix(1, 0), raw).Events {
					require.NotContains(t, []string{"coap", "ntp", "websocket"}, e.Protocol)
				}
			}
		}
		s.Close("test")
		require.Zero(t, s.Stats().BufferedBytes)
		require.Zero(t, s.Stats().Flows)
	}
	_, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("sctp"))
	require.Error(t, err)
}

func ntpFixtureRequest() []byte { raw := make([]byte, 48); raw[0] = 0x23; raw[40] = 1; return raw }

func TestProtocolSessionUDPDoesNotJoinTruncatedDatagrams(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"))
	require.NoError(t, err)
	q := dnsQuery(12, "boundary.example", 1)[2:]
	for _, part := range [][]byte{q[:8], q[8:]} {
		r := s.Feed(0, time.Unix(1, 0), part)
		require.False(t, r.NeedMore)
		for _, e := range r.Events {
			require.NotEqual(t, "dns", e.Protocol)
		}
	}
	r := s.Feed(0, time.Unix(2, 0), q)
	require.Nil(t, r.Err)
	require.Equal(t, "dns", r.Events[0].Protocol)
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDNSContentEvidenceRejectsEmptyHeader(t *testing.T) {
	require.False(t, dnsDatagramEvidence(make([]byte, 12)))
	require.False(t, dnsHeader(make([]byte, 12)))
	q := dnsQuery(1, "", 1)[2:]
	require.True(t, dnsDatagramEvidence(q))
	require.False(t, dnsDatagramEvidence(append(q, 0)))
	t.Run("stream-prefix-is-not-message-evidence", func(t *testing.T) {
		// RFC 1035 sections 4.1 and 4.2.2: a length and plausible header do
		// not establish DNS without a complete valid message. This foreign
		// frame has one plausible question followed by undeclared trailing data.
		foreign := make([]byte, 258)
		copy(foreign, []byte{1, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 'w', 'w', 'w', 0, 0, 1, 0, 1})
		for _, chunk := range []int{1, 7, 14, len(foreign)} {
			s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("tcp"))
			require.NoError(t, err)
			for at := 0; at < len(foreign); at += chunk {
				r := s.Feed(0, time.Unix(1, 0), foreign[at:min(at+chunk, len(foreign))])
				require.NotContains(t, []string{"dns", "dot"}, r.State, "chunk=%d at=%d", chunk, at)
				for _, e := range r.Events {
					require.NotContains(t, []string{"dns", "dot"}, e.Protocol)
				}
			}
			for _, e := range s.Close("test") {
				require.NotContains(t, []string{"dns", "dot"}, e.Protocol)
			}
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
	t.Run("stream-query-over-signature-window", func(t *testing.T) {
		name := "abcdefghijklmnopqrstuvwxyz0123456789.example.test"
		query := dnsQuery(42, name, 1)
		response := dnsAResponse(42, name, [4]byte{192, 0, 2, 42})
		for _, chunk := range []int{1, 7, 14, len(response)} {
			s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("tcp"))
			require.NoError(t, err)
			require.Equal(t, ProbeAccept, s.Probe(query).Verdict)
			var events []*ProtocolEvent
			for dir, wire := range [][]byte{query, response} {
				for at := 0; at < len(wire); at += chunk {
					r := s.Feed(dir, time.Unix(1, 0), wire[at:min(at+chunk, len(wire))])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
					events = append(events, r.Events...)
				}
			}
			events = append(events, s.Close("test")...)
			require.Len(t, events, 2)
			for _, e := range events {
				require.Equal(t, "dns", e.Protocol)
				require.Equal(t, "decoded", e.Status)
				require.Empty(t, e.Error)
				require.Equal(t, name, e.Session["QNAME"])
			}
			require.Equal(t, "matched", events[1].Session["Association Status"])
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
}
