package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Public, deterministic test secrets. The live bilateral shutdown captures and
// independent AEAD oracle are kept outside Git with the execution evidence.
func closeNotifyTestFlow(t *testing.T, suite uint16, server int, deferred bool) (*binFlow, *binTLS, *[]*ProtocolEvent) {
	t.Helper()
	a, events := remainingParser(t)
	a.config.Deferred = deferred
	s := newBinTLS()
	s.version, s.suite, s.client = 0x303, suite, 1-server
	s.random, s.serverRandom = bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)
	if suite >= 0x1301 && suite <= 0x1303 {
		s.version = 0x304
	}
	for dir := 0; dir < 2; dir++ {
		if s.version == 0x304 {
			n := 32
			if suite == 0x1302 {
				n = 48
			}
			require.NoError(t, s.install13(dir, bytes.Repeat([]byte{byte(0x41 + dir)}, n)))
			s.app[dir] = true
		} else {
			ok, err := s.loadKey(dir, precontextRecoveryTestKeys{random: s.random, master: bytes.Repeat([]byte{0x33}, 48)})
			require.NoError(t, err)
			require.True(t, ok)
			s.ccs[dir] = true
		}
	}
	f := &binFlow{a: a, id: 7, protocol: "tls", tls: s, transportFlow: &TrafficFlow{}, endpoints: [2]string{"192.0.2.1:40000", "192.0.2.2:443"}}
	t.Cleanup(func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) })
	return f, s, events
}

func closeNotifyTestSend(t *testing.T, f *binFlow, s *binTLS, dir int, typ byte, plain []byte, badTag bool) {
	t.Helper()
	var wire []byte
	if s.version == 0x304 {
		wire = boundaryTLSRecord(t, s, dir, s.seq[dir], plain, typ, 0)
	} else {
		wire = precontextRecoveryTestRecord(t, s, dir, s.seq[dir], typ, plain)
	}
	if badTag {
		wire[len(wire)-1] ^= 1
	}
	f.feed(dir, wire, time.Unix(1000+int64(s.seq[dir]), 0))
}

func closeNotifyTestHTTP(events []*ProtocolEvent) (complete, incomplete []*ProtocolEvent) {
	for _, e := range events {
		if e.Protocol == "http" {
			switch e.Status {
			case "decoded", "deferred":
				complete = append(complete, e)
			case "incomplete":
				incomplete = append(incomplete, e)
			}
		}
	}
	return
}

func TestTLSAuthenticatedCloseNotifyCompletesOnlyItsHTTPDirection(t *testing.T) {
	for _, suite := range []uint16{0x009c, 0xc02f, 0xc030, 0xcca8, 0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("suite%04x/server%d/deferred%t", suite, server, deferred), func(t *testing.T) {
					f, s, events := closeNotifyTestFlow(t, suite, server, deferred)
					request := []byte("GET /shutdown HTTP/1.1\r\nHost: owned.invalid\r\n\r\n")
					response := []byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\nexact response")
					closeNotifyTestSend(t, f, s, 1-server, 23, request, false)
					closeNotifyTestSend(t, f, s, server, 23, response, false)
					complete, _ := closeNotifyTestHTTP(*events)
					require.Len(t, complete, 1, "close-delimited response must await its own authenticated EOF")
					closeNotifyTestSend(t, f, s, 1-server, 21, []byte{1, 0}, false)
					complete, _ = closeNotifyTestHTTP(*events)
					require.Len(t, complete, 1, "the request sender's close cannot finish the response")
					closeNotifyTestSend(t, f, s, server, 21, []byte{1, 0}, false)
					complete, incomplete := closeNotifyTestHTTP(*events)
					require.Len(t, complete, 2)
					require.Empty(t, incomplete)
					require.Equal(t, request, complete[0].Raw)
					require.Equal(t, response, complete[1].Raw)
					require.Equal(t, complete[0].ID, complete[1].ResponseTo)
					require.Equal(t, "transaction", complete[1].Completeness)
					fields, err := complete[1].GetFields()
					require.NoError(t, err)
					parsed := fields["Message"].(map[string]any)["HTTP Response"].(map[string]any)
					require.Equal(t, "200", parsed["FirstLine"].(map[string]any)["Status"])
					require.Equal(t, "exact response", parsed["Body"].(map[string]any)["Octets"])
					require.Equal(t, "authenticated TLS close_notify", complete[1].Session["Message Boundary"])
					require.NotZero(t, complete[1].Session["TLS Boundary PDU"])
					require.Equal(t, server, complete[1].Session["TLS Boundary Direction"])
					f.close(TrafficFlowCloseReason_FIN)
					after, incomplete := closeNotifyTestHTTP(*events)
					require.Len(t, after, 2, "transport close must not publish the completed message again")
					require.Empty(t, incomplete)
				})
			}
		}
	}
}

func TestTLS13CloseNotifyIgnoresLegacyLevelAndKeepsPeerOpen(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
	closeNotifyTestSend(t, f, s, 0, 21, []byte{2, 0}, false)
	// TLS 1.3 half-close has no effect on the peer's write side.
	closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\n\r\nbody"), false)
	closeNotifyTestSend(t, f, s, 1, 21, []byte{2, 0}, false)
	complete, incomplete := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 2)
	require.Empty(t, incomplete)
	closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"), false)
	after, _ := closeNotifyTestHTTP(*events)
	require.Len(t, after, 2)
}

func TestTLSCloseNotifyFragmentCannotBridgeMissingRecord(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0xc02f, 1, false)
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
	closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\n\r\nbody"), false)
	closeNotifyTestSend(t, f, s, 1, 21, []byte{1}, false)
	f.noteTLSRecordFailure(1, map[string]any{})
	require.False(t, s.alertPending[1])
	closeNotifyTestSend(t, f, s, 1, 21, []byte{0}, false)
	require.False(t, s.closeNotify[1])
	complete, _ := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 1)
}

func TestTLSCloseNotifyAuthoritySurvivesPublicCallbackEdits(t *testing.T) {
	for _, hide := range []bool{false, true} {
		t.Run(fmt.Sprintf("hide-authenticated-close-%t", hide), func(t *testing.T) {
			f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
			originalCallback := f.a.config.OnEvent
			closing := false
			var boundary uint64
			f.a.config.OnEvent = func(e *ProtocolEvent) {
				originalCallback(e)
				if e.Protocol == "tls" {
					if closing {
						boundary = e.ID
						e.ID = 999999
					}
					e.Session = map[string]any{"Authenticated Close Notify": !hide, "Authentication Verified": !hide}
					e.Raw = []byte("editable callback bytes")
				}
			}
			closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
			closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\n\r\nbody"), false)
			complete, _ := closeNotifyTestHTTP(*events)
			require.Len(t, complete, 1, "an editable DTO cannot synthesize sender EOF")
			if hide {
				closing = true
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, false)
				complete, _ = closeNotifyTestHTTP(*events)
				require.Len(t, complete, 2)
				require.Equal(t, boundary, complete[1].Session["TLS Boundary PDU"])
				require.NotEqual(t, uint64(999999), complete[1].Session["TLS Boundary PDU"])
			}
		})
	}
}

func TestTLSCloseNotifyDoesNotCompleteTruncatedLengthOrChunks(t *testing.T) {
	for _, response := range []string{"HTTP/1.1 200 OK\r\nContent-Length: 8\r\n\r\nbody", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nab"} {
		t.Run(response[17:31], func(t *testing.T) {
			f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
			closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
			closeNotifyTestSend(t, f, s, 1, 23, []byte(response), false)
			closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, false)
			f.close(TrafficFlowCloseReason_FIN)
			complete, incomplete := closeNotifyTestHTTP(*events)
			require.Len(t, complete, 1)
			require.Len(t, incomplete, 1)
			require.Equal(t, []byte(response), incomplete[0].Raw)
			require.Zero(t, incomplete[0].ResponseTo)
		})
	}
}

func TestTLSCloseNotifyDoesNotInventHTTPCompletion(t *testing.T) {
	for _, suite := range []uint16{0xc02f, 0x1301} {
		for _, mode := range []string{"no-alert", "opposite-alert", "bad-tag", "other-alert", "plaintext-gap", "fragment-without-suffix"} {
			t.Run(fmt.Sprintf("suite%04x/%s", suite, mode), func(t *testing.T) {
				f, s, events := closeNotifyTestFlow(t, suite, 1, false)
				closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
				closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\nbody"), false)
				switch mode {
				case "opposite-alert":
					closeNotifyTestSend(t, f, s, 0, 21, []byte{1, 0}, false)
				case "bad-tag":
					closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, true)
				case "other-alert":
					closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 90}, false)
				case "plaintext-gap":
					f.markTLSInnerGap(1)
					closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, false)
				case "fragment-without-suffix":
					closeNotifyTestSend(t, f, s, 1, 21, []byte{1}, false)
				}
				f.close(TrafficFlowCloseReason_FIN)
				complete, _ := closeNotifyTestHTTP(*events)
				require.Len(t, complete, 1, "FIN, other direction/alert, invalid tag or missing bytes cannot authenticate the response EOF")
			})
		}
	}
}

func TestTLS12CloseNotifyFragmentAuthenticationAndPostCloseData(t *testing.T) {
	for _, mode := range []string{"two-records", "coalesced", "close-before-warning", "close-before-fragment", "bad-final-tag", "fatal-level", "fatal-before-close"} {
		t.Run(mode, func(t *testing.T) {
			f, s, events := closeNotifyTestFlow(t, 0xc02f, 1, false)
			closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"), false)
			closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\n\r\nbody"), false)
			switch mode {
			case "two-records", "bad-final-tag":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1}, false)
				closeNotifyTestSend(t, f, s, 1, 21, []byte{0}, mode == "bad-final-tag")
			case "coalesced":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 90, 1, 0}, false)
			case "close-before-warning":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0, 1, 90}, false)
			case "close-before-fragment":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0, 1}, false)
			case "fatal-level":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{2, 0}, false)
			case "fatal-before-close":
				closeNotifyTestSend(t, f, s, 1, 21, []byte{2, 40}, false)
				closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, false)
			}
			complete, _ := closeNotifyTestHTTP(*events)
			if mode == "bad-final-tag" || mode == "fatal-level" || mode == "fatal-before-close" {
				require.Len(t, complete, 1)
				return
			}
			require.Len(t, complete, 2)
			closeNotifyTestSend(t, f, s, 1, 23, []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"), false)
			after, _ := closeNotifyTestHTTP(*events)
			require.Len(t, after, 2, "records following authenticated closure must not open another application message")
		})
	}
}
