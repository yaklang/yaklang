package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLSRecordFailurePreservesAuthenticatedHTTPPrefix(t *testing.T) {
	for _, suite := range []uint16{0x009c, 0x1301, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				for _, framing := range []string{"Connection: close\r\n", "Content-Length: 8\r\n", "Transfer-Encoding: chunked\r\n"} {
					t.Run(fmt.Sprintf("suite%04x/server%d/deferred%t/%s", suite, server, deferred, framing[:len(framing)-2]), func(t *testing.T) {
						f, s, events := closeNotifyTestFlow(t, suite, server, deferred)
						request := []byte("GET /prefix HTTP/1.1\r\nHost: observed.invalid\r\n\r\n")
						body := "body"
						if framing == "Transfer-Encoding: chunked\r\n" {
							body = "4\r\nab"
						}
						header := "HTTP/1.1 200 OK\r\n" + framing + "X-Repeat: one\r\nX-Repeat: two\r\n\r\n"
						prefix := []byte(header + body)
						closeNotifyTestSend(t, f, s, 1-server, 23, request, false)
						closeNotifyTestSend(t, f, s, server, 23, prefix, false)
						var contributor uint64
						for _, e := range *events {
							if e.Protocol == "tls" && e.Direction == server {
								contributor = e.ID
							}
						}
						require.NotZero(t, contributor)
						closeNotifyTestSend(t, f, s, server, 21, []byte{1, 0}, true)
						complete, incomplete := closeNotifyTestHTTP(*events)
						require.Len(t, complete, 1)
						require.Len(t, incomplete, 1, "a rejected later record must not erase previously authenticated HTTP bytes")
						e := incomplete[0]
						require.Equal(t, prefix, e.Raw)
						require.Equal(t, "headers", e.Completeness)
						require.Equal(t, "http-incomplete-message-header-observation", e.Profile)
						require.Equal(t, true, e.Session["TLS Plaintext Gap"])
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						require.Nil(t, e.SourceBytes.CompletedIn)
						require.Equal(t, []uint64{contributor}, e.SourceBytes.ParentPDUs)
						require.Equal(t, contributor, e.SourceBytes.ParentPDU)
						fields, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, 200, fields["Status Code"])
						require.Equal(t, []string{"one", "two"}, fields["Headers"].(map[string]any)["X-Repeat"])
						require.NotContains(t, fields, "Body")
						failure := (*events)[len(*events)-2]
						require.Equal(t, "tls", failure.Protocol, "the cause must be published before the prefix observation")
						require.NotEmpty(t, failure.Error)
						require.Equal(t, failure.ID, e.Session["TLS Gap Cause PDU"])
						require.Equal(t, server, e.Session["TLS Gap Direction"])
						require.Equal(t, s.version, e.Session["TLS Gap Version"])
						require.NotContains(t, e.SourceBytes.ParentPDUs, failure.ID)
						// A recovered authenticated record is useful as record evidence,
						// but cannot join the old HTTP prefix across the known hole.
						closeNotifyTestSend(t, f, s, server, 23, []byte("suffix"), false)
						closeNotifyTestSend(t, f, s, server, 21, []byte{1, 0}, false)
						f.close(TrafficFlowCloseReason_FIN)
						after, partial := closeNotifyTestHTTP(*events)
						require.Len(t, after, 1)
						require.Len(t, partial, 1, "later recovery and Close must not republish or complete the prefix")
						require.Equal(t, prefix, partial[0].Raw)
					})
				}
			}
		}
	}
}

func TestTLSHTTPPrefixGapIsDirectional(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
	response := []byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\nbody")
	closeNotifyTestSend(t, f, s, 1, 23, response, false)
	closeNotifyTestSend(t, f, s, 0, 21, []byte{1, 0}, true)
	_, partial := closeNotifyTestHTTP(*events)
	require.Empty(t, partial, "a client-side failure is not a response-side plaintext gap")
	closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, false)
	complete, partial := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 2)
	require.Empty(t, partial)
	require.Equal(t, response, complete[1].Raw)
}

func TestTLSHTTPGapContextCannotBeReplacedByCallback(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
	callback := f.a.config.OnEvent
	var failure uint64
	f.a.config.OnEvent = func(e *ProtocolEvent) {
		callback(e)
		if e.Protocol == "tls" && e.Error != "" {
			failure = e.ID
			e.ID = 999999
			e.Session = map[string]any{"TLS Gap Cause PDU": uint64(888888)}
			e.SourceBytes.ParentPDUs = []uint64{777777}
		}
	}
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
	response := []byte("HTTP/1.1 200 OK\r\n\r\nbody")
	closeNotifyTestSend(t, f, s, 1, 23, response, false)
	closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, true)
	_, partial := closeNotifyTestHTTP(*events)
	require.Len(t, partial, 1)
	require.NotZero(t, failure)
	require.Equal(t, failure, partial[0].Session["TLS Gap Cause PDU"])
	require.Equal(t, response, partial[0].Raw)
	require.NotContains(t, partial[0].SourceBytes.ParentPDUs, uint64(777777))
	partial[0].Fields = map[string]any{"Status Code": 999, "Body": "forged"}
	fields, err := partial[0].GetFields()
	require.NoError(t, err)
	require.Equal(t, 200, fields["Status Code"])
	require.NotContains(t, fields, "Body")
}

func TestTLSHTTPGapWithoutOwningRecordKeepsUnknownCause(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
	response := []byte("HTTP/1.1 200 OK\r\n\r\nbody")
	closeNotifyTestSend(t, f, s, 1, 23, response, false)
	f.markTLSInnerGap(1)
	f.markTLSInnerGap(1)
	_, partial := closeNotifyTestHTTP(*events)
	require.Empty(t, partial, "marking a gap must not publish before its record callback")
	f.close(TrafficFlowCloseReason_FIN)
	complete, partial := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 1)
	require.Len(t, partial, 1)
	require.Equal(t, response, partial[0].Raw)
	require.Equal(t, true, partial[0].Session["TLS Plaintext Gap"])
	require.NotContains(t, partial[0].Session, "TLS Gap Cause PDU")
	require.NotContains(t, partial[0].Session, "TLS Gap Version")
	require.Zero(t, partial[0].ResponseTo)
	require.Zero(t, f.a.stats().BufferedBytes)
}

func TestTLSHTTPGapCannotInventValidatedHeader(t *testing.T) {
	for _, response := range []string{"HTTP/1.1 200 OK\r\nContent-Length: 8\r\n", "not an HTTP response"} {
		f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
		closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
		closeNotifyTestSend(t, f, s, 1, 23, []byte(response), false)
		closeNotifyTestSend(t, f, s, 1, 21, []byte{1, 0}, true)
		f.close(TrafficFlowCloseReason_FIN)
		for _, e := range *events {
			if e.Protocol == "http" && e.Direction == 1 {
				require.NotEqual(t, "http-incomplete-message-header-observation", e.Profile)
			}
		}
	}
}

func TestTLSAuthenticatedEpochRecoveryPreservesHTTPPrefix(t *testing.T) {
	f, s, events := closeNotifyTestFlow(t, 0x1301, 1, false)
	closeNotifyTestSend(t, f, s, 0, 23, []byte("GET / HTTP/1.1\r\nHost: observed.invalid\r\n\r\n"), false)
	response := []byte("HTTP/1.1 200 OK\r\n\r\nold authenticated prefix")
	closeNotifyTestSend(t, f, s, 1, 23, response, false)
	next, err := quicHKDFExpandLabel(s.secret[1], "traffic upd", len(s.secret[1]))
	require.NoError(t, err)
	trial := *s
	require.NoError(t, trial.install13(1, next))
	wire := boundaryTLSRecord(t, &trial, 1, 0, []byte("later epoch bytes"), 23, 0)
	f.feed(1, wire, time.Unix(2000, 0))
	complete, partial := closeNotifyTestHTTP(*events)
	require.Len(t, complete, 1)
	require.Len(t, partial, 1)
	require.Equal(t, response, partial[0].Raw)
	parent := (*events)[len(*events)-2]
	require.Equal(t, true, parent.Session["Authentication Verified"])
	require.Equal(t, true, parent.Session["Unobserved KeyUpdate Inferred From Authentication"])
	require.Empty(t, parent.Error)
	require.Equal(t, parent.ID, partial[0].Session["TLS Gap Cause PDU"])
	require.NotContains(t, partial[0].SourceBytes.ParentPDUs, parent.ID)
	require.Zero(t, partial[0].ResponseTo)
}
