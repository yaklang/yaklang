package pcaputil

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const httpWaitSeedRequest = "GET /seed HTTP/1.1\r\nHost: example.test\r\n\r\n"
const httpWaitSeedResponse = "HTTP/1.1 204 No Content\r\n\r\n"

func httpWaitSession(t *testing.T, budget ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSession(budget)
	require.NoError(t, err)
	for dir, wire := range []string{httpWaitSeedRequest, httpWaitSeedResponse} {
		r := s.Feed(dir, time.Unix(100+int64(dir), 0), []byte(wire))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		_, err = r.Events[0].GetFields()
		require.NoError(t, err)
	}
	return s
}

func TestHTTPLateObservedMethodReplaysExactResponses(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			s := httpWaitSession(t, DefaultParserBudget())
			first := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
			if method == "HEAD" {
				// Representation length must not eat the following response.
				first = "HTTP/1.1 200 OK\r\nContent-Length: 9223372036854775807\r\n\r\n"
			}
			second := "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nEND"
			wait := s.Feed(1, time.Unix(102, 0), []byte(first+second))
			require.Empty(t, wait.Events)
			require.True(t, wait.NeedMore)
			require.Error(t, wait.Err)
			require.Equal(t, ErrNeedMore, wait.Err.Kind)
			req1 := fmt.Sprintf("%s /one HTTP/1.1\r\nHost: example.test\r\n\r\n", method)
			req2 := "GET /two HTTP/1.1\r\nHost: example.test\r\n\r\n"
			got := s.Feed(0, time.Unix(103, 0), []byte(req1+req2))
			require.Nil(t, got.Err)
			require.Len(t, got.Events, 4)
			for i, raw := range []string{req1, first, req2, second} {
				require.Equal(t, raw, string(got.Events[i].Raw))
				_, err := got.Events[i].GetFields()
				require.NoError(t, err)
			}
			require.Equal(t, got.Events[0].ID, got.Events[1].ResponseTo)
			require.Equal(t, got.Events[2].ID, got.Events[3].ResponseTo)
			// Wire completion/timestamp stays at the response's original input;
			// its callback is synchronously returned only by the later request Feed.
			require.Equal(t, time.Unix(102, 0), got.Events[1].Timestamp)
			s.Close("eof")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Zero(t, s.Stats().ContextRequired)
		})
	}
}

func TestHTTPLateConnectDoesNotConsumeTunnelAsBody(t *testing.T) {
	s := httpWaitSession(t, DefaultParserBudget())
	header := "HTTP/1.1 200 Connection Established\r\nContent-Length: 99999\r\n\r\n"
	opaque := []byte{0x8f, 0xac, 0x01}
	wait := s.Feed(1, time.Unix(102, 0), append([]byte(header), opaque...))
	require.Empty(t, wait.Events)
	require.Equal(t, ErrNeedMore, wait.Err.Kind)
	request := "CONNECT example.test:443 HTTP/1.1\r\nHost: example.test:443\r\n\r\n"
	got := s.Feed(0, time.Unix(103, 0), []byte(request))
	require.Nil(t, got.Err)
	require.GreaterOrEqual(t, len(got.Events), 2)
	require.Equal(t, request, string(got.Events[0].Raw))
	require.Equal(t, header, string(got.Events[1].Raw))
	require.Equal(t, got.Events[0].ID, got.Events[1].ResponseTo)
	for _, e := range got.Events {
		if e.Protocol == "http" {
			require.False(t, bytes.Contains(e.Raw, opaque))
		}
	}
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPLateExpectContinueUsesObservedHeaderBeforeBody(t *testing.T) {
	s := httpWaitSession(t, DefaultParserBudget())
	interim := "HTTP/1.1 100 Continue\r\n\r\n"
	wait := s.Feed(1, time.Unix(102, 0), []byte(interim))
	require.Empty(t, wait.Events)
	require.Equal(t, ErrNeedMore, wait.Err.Kind)
	header := "POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 3\r\nExpect: 100-continue\r\n\r\n"
	known := s.Feed(0, time.Unix(103, 0), []byte(header))
	require.Len(t, known.Events, 1)
	require.Equal(t, interim, string(known.Events[0].Raw))
	require.NotZero(t, known.Events[0].ResponseTo)
	require.True(t, known.NeedMore)
	body := s.Feed(0, time.Unix(104, 0), []byte("abc"))
	require.Nil(t, body.Err)
	require.Len(t, body.Events, 1)
	require.Equal(t, header+"abc", string(body.Events[0].Raw))
	require.Equal(t, body.Events[0].ID, known.Events[0].ResponseTo)
	final := s.Feed(1, time.Unix(105, 0), []byte(httpWaitSeedResponse))
	require.Nil(t, final.Err)
	require.Len(t, final.Events, 1)
	require.Equal(t, body.Events[0].ID, final.Events[0].ResponseTo)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPContextPermanentLossDoesNotGuessMethodOrBoundary(t *testing.T) {
	s := httpWaitSession(t, DefaultParserBudget())
	header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\n"
	all := header + "OKHTTP/1.1 204 No Content\r\n\r\n"
	wait := s.Feed(1, time.Unix(102, 0), []byte(all))
	require.Empty(t, wait.Events)
	require.Equal(t, ErrNeedMore, wait.Err.Kind)
	closed := s.Close("eof")
	require.Len(t, closed, 1)
	e := closed[0]
	require.Equal(t, "context-required", e.Status)
	require.Equal(t, "headers", e.Completeness)
	require.Equal(t, header, string(e.Raw))
	require.Zero(t, e.ResponseTo)
	require.NotContains(t, e.Session, "Method")
	require.Equal(t, false, e.Session["Body Boundary Known"])
	require.Equal(t, len(all), e.Session["Retained Stream Bytes"])
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, fields["Headers"].(map[string]any)["X-Repeat"])
	require.NotContains(t, fields, "Body")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPInitialOrphanRemainsImmediateContextRequired(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	header := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n"
	r := s.Feed(1, time.Unix(100, 0), []byte(header+"OK"))
	require.Len(t, r.Events, 1)
	require.Equal(t, "context-required", r.Events[0].Status)
	require.Equal(t, header, string(r.Events[0].Raw))
	require.Zero(t, r.Events[0].ResponseTo)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPContextWaitIsBoundedWithoutPayloadOmission(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes, b.MaxFrameBytes, b.MaxBufferedBytes = 256, 256, 16384
	s := httpWaitSession(t, b)
	header := "HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n"
	r := s.Feed(1, time.Unix(102, 0), []byte(header+strings.Repeat("x", 100)))
	require.Empty(t, r.Events)
	require.Equal(t, ErrNeedMore, r.Err.Kind)
	r = s.Feed(1, time.Unix(103, 0), []byte(strings.Repeat("x", 256)))
	require.NotEmpty(t, r.Events)
	for _, e := range r.Events {
		require.Equal(t, "limited", e.Status)
		require.NotEqual(t, "HTTPPayloadOmitted", e.ExpertCode)
		require.Zero(t, e.ResponseTo)
		require.Equal(t, header, string(e.Raw))
	}
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestHTTPContextReplayPreservesResponseByteProvenance(t *testing.T) {
	s := httpWaitSession(t, DefaultParserBudget())
	cs := s.(*captureSession)
	f := cs.f
	f.byteSource = "decrypted"
	f.domain = CaptureDomain{Section: 1, Interface: 2, Encapsulation: "vlan:80"}
	response := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
	d := &f.directions[1]
	ref := PacketReference{Number: 17, Domain: f.domain}
	require.True(t, f.a.reserveEvidence(80))
	d.contributions = append(d.contributions, contribution{start: d.offset, end: d.offset + uint64(len(response)), parent: 117, ref: ref})
	wait := s.Feed(1, time.Unix(102, 0), []byte(response))
	require.Empty(t, wait.Events)
	// A different current parent at request arrival must not replace the old
	// server byte contributors. This is a private attribution unit, not AEAD proof.
	f.parentID = 999
	request := "GET /late HTTP/1.1\r\nHost: example.test\r\n\r\n"
	r := s.Feed(0, time.Unix(103, 0), []byte(request))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 2)
	e := r.Events[1]
	require.Equal(t, []PacketReference{ref}, e.SourceBytes.PacketRefs)
	require.Equal(t, &ref, e.SourceBytes.CompletedIn)
	require.Contains(t, e.SourceBytes.ParentPDUs, uint64(117))
	require.NotContains(t, e.SourceBytes.ParentPDUs, uint64(999))
	require.Equal(t, f.domain, e.Domain)
	require.Equal(t, r.Events[0].ID, e.ResponseTo)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}
