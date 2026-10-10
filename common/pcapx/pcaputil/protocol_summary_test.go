package pcaputil

import (
	"encoding/binary"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestProtocolSummaryUnknownMidstream(t *testing.T) {
	wire := strings.Repeat("?", 100)
	events, stats, err := binReplay(t, binTestPcap(t, []tcpStep{{seq: 100, data: wire}, {seq: 200, data: wire}}, 443, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1, "later segments must not repeat detection diagnostics")
	e := events[0]
	require.Equal(t, "unrecognized", e.Status)
	require.Empty(t, e.Protocol, "443 alone is not evidence of TLS")
	require.Empty(t, e.Error)
	require.Contains(t, e.Summary, "TCP initiator not observed")
	require.Equal(t, false, e.Session["TCP Initiator Observed"])
	require.Equal(t, uint64(1), stats.ProbeCalls)
	require.Equal(t, uint64(200), stats.UnclassifiedBytes)
}

func TestProtocolSummaryUDP(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, protocol := range []string{"dns", "mdns", "llmnr"} {
			t.Run(protocol+"/deferred="+strconv.FormatBool(deferred), func(t *testing.T) {
				port := map[string]uint16{"dns": 53, "mdns": 5353, "llmnr": 5355}[protocol]
				s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(12345, port))
				require.NoError(t, err)
				defer s.Close("FIN")
				s.(*captureSession).f.a.config.Deferred = deferred
				wire := dnsQuery(1, "example.local", 1)[2:] // strip the helper's TCP length prefix
				result := s.Feed(0, time.Unix(1, 0), wire)
				require.Nil(t, result.Err)
				require.Len(t, result.Events, 1)
				e := result.Events[0]
				require.Equal(t, protocol, e.Protocol)
				require.Contains(t, e.Summary, "query")
				require.Contains(t, e.Summary, "example.local")
				require.Contains(t, e.Summary, " A")
				require.NotContains(t, e.Summary, "<nil>")
				// mDNS announcements need no echoed question. Use an answer-only
				// address record to exercise the observed answer-name fallback.
				if protocol == "mdns" {
					wire[2] = 0x84
					binary.BigEndian.PutUint16(wire[4:6], 0)
					binary.BigEndian.PutUint16(wire[6:8], 1)
					wire = append(wire, 0, 0, 0, 120, 0, 4, 192, 0, 2, 10)
					result = s.Feed(1, time.Unix(2, 0), wire)
					require.Nil(t, result.Err)
					require.Len(t, result.Events, 1)
					require.Contains(t, result.Events[0].Summary, "response (questions=0 answers=1) example.local A")
				}
			})
		}
	}
	unknown, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("udp"), WithSessionPorts(12345, 45678))
	require.NoError(t, err)
	defer unknown.Close("FIN")
	result := unknown.Feed(0, time.Unix(1, 0), []byte(strings.Repeat("?", 100)))
	require.Len(t, result.Events, 1)
	e := result.Events[0]
	require.Equal(t, "unrecognized", e.Status)
	require.Empty(t, e.Protocol)
	require.Equal(t, "ProtocolNotRecognized", e.ExpertCode)
	require.Contains(t, e.Summary, e.Source)
	require.Contains(t, e.Summary, e.Destination)
	require.Contains(t, e.Summary, "100 bytes")
}

func TestProtocolSummaryTLSCapture(t *testing.T) {
	wire, err := trafficfixture.ReadFile("testdata/protocol-sessions/first-batch-m1/tls-h2-bidi.pcap")
	require.NoError(t, err)
	for _, deferred := range []bool{false, true} {
		events, _, err := binReplay(t, wire, 1, WithProtocolDeferred(deferred))
		require.NoError(t, err)
		seenHello, seenEncrypted := false, false
		for _, e := range events {
			if e.Protocol != "tls" || e.Error != "" || e.Status != "decoded" && e.Status != "deferred" {
				continue
			}
			require.NotEqual(t, "tls", e.Summary)
			require.NotContains(t, e.Summary, "<nil>")
			seenHello = seenHello || strings.Contains(e.Summary, "ClientHello")
			seenEncrypted = seenEncrypted || strings.Contains(e.Summary, "encrypted")
			if e.Session["Content Visibility"] == "encrypted" {
				require.Contains(t, e.Summary, "encrypted")
				require.NotContains(t, e.Summary, "HTTP", "ciphertext must not claim an application message")
			}
		}
		require.True(t, seenHello)
		require.True(t, seenEncrypted)
	}
}
