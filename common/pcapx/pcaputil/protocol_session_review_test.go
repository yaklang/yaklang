package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newReviewSession(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSession(b)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	return s
}

func reviewWSFrame(op byte, fin, mask bool, p string) []byte {
	b := []byte{op, byte(len(p))}
	if fin {
		b[0] |= 128
	}
	if mask {
		b[1] |= 128
		b = append(b, 1, 2, 3, 4)
	}
	for i := range p {
		c := p[i]
		if mask {
			c ^= byte(i%4 + 1)
		}
		b = append(b, c)
	}
	return b
}

func reviewH2(t *testing.T, content string) ProtocolSession {
	t.Helper()
	s := newReviewSession(t, ParserBudget{})
	for dir, wire := range [][]byte{append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...), h2TestFrame(4, 0, 0, nil)} {
		require.Nil(t, s.Feed(dir, time.Time{}, wire).Err)
	}
	require.Nil(t, s.Feed(0, time.Time{}, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/svc/m", "content-type", content))).Err)
	require.Nil(t, s.Feed(1, time.Time{}, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", content))).Err)
	return s
}

func TestProtocolSessionReviewLifecycleAndBudget(t *testing.T) {
	s := newReviewSession(t, ParserBudget{MaxBufferedBytes: 2 << 20})
	require.Equal(t, 2<<20, s.(*captureSession).f.a.config.MaxBufferedBytes)
	for i := 0; i < 100; i++ {
		require.Len(t, s.Feed(0, time.Time{}, []byte("+OK\r\n")).Events, 1)
	}
	require.Empty(t, s.(*captureSession).events, "returned events must not remain retained by the session")
	s.Close("FIN")
	require.Empty(t, s.Close("FIN"))
	r := s.Feed(0, time.Time{}, []byte("+OK\r\n"))
	require.Zero(t, r.Consumed)
	require.NotNil(t, r.Err)
}

func TestProtocolSessionReviewWebSocketDirections(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	require.Nil(t, s.Feed(0, time.Time{}, reviewWSFrame(1, false, true, "client-")).Err)
	require.Nil(t, s.Feed(1, time.Time{}, reviewWSFrame(1, false, false, "server-")).Err)
	r := s.Feed(0, time.Time{}, reviewWSFrame(0, true, true, "end"))
	require.Nil(t, r.Err)
	require.Equal(t, []byte("client-end"), r.Events[0].Session["Application"])
	r = s.Feed(1, time.Time{}, reviewWSFrame(0, true, false, "end"))
	require.Nil(t, r.Err)
	require.Equal(t, []byte("server-end"), r.Events[0].Session["Application"])
}

func TestProtocolSessionReviewWebSocketInvalidSequence(t *testing.T) {
	for name, wire := range map[string][]byte{
		"orphan":     reviewWSFrame(0, true, true, "x"),
		"wrong-mask": reviewWSFrame(1, true, false, "x"),
	} {
		t.Run(name, func(t *testing.T) {
			s := newReviewSession(t, ParserBudget{})
			require.Nil(t, s.Feed(0, time.Time{}, reviewWSFrame(1, true, true, "start")).Err)
			require.NotNil(t, s.Feed(0, time.Time{}, wire).Err)
		})
	}
}

func TestProtocolSessionReviewGRPCAdmissionAndDirections(t *testing.T) {
	t.Run("ordinary-http2", func(t *testing.T) {
		s := reviewH2(t, "application/octet-stream")
		r := s.Feed(0, time.Time{}, h2TestFrame(0, 0, 1, []byte{0, 0, 0, 0, 1, 'x'}))
		require.Nil(t, r.Err)
		require.Equal(t, "http2", r.Events[0].Protocol)
		require.Nil(t, r.Events[0].Session["GRPC Messages"])
	})
	t.Run("duplex-and-final-data", func(t *testing.T) {
		s := reviewH2(t, "application/grpc")
		require.Nil(t, s.Feed(0, time.Time{}, h2TestFrame(0, 0, 1, []byte{0, 0, 0, 0, 2, 'a'})).Err)
		r := s.Feed(1, time.Time{}, h2TestFrame(0, 1, 1, []byte{0, 0, 0, 0, 1, 'z'}))
		require.Nil(t, r.Err)
		require.Equal(t, []byte("z"), r.Events[0].Session["GRPC Messages"].([]map[string]any)[0]["Payload"])
		r = s.Feed(0, time.Time{}, h2TestFrame(0, 1, 1, []byte{'b'}))
		require.Nil(t, r.Err)
		require.Equal(t, []byte("ab"), r.Events[0].Session["GRPC Messages"].([]map[string]any)[0]["Payload"])
	})
}

func TestProtocolSessionReviewRedisFraming(t *testing.T) {
	for _, wire := range []string{"$1\r\naXX", "$-2\r\n", "%9223372036854775807\r\n", "$9223372036854775807\r\n", "#x\r\n", "_junk\r\n"} {
		t.Run(wire, func(t *testing.T) { _, err := redisFrameLength([]byte(wire), 0); require.Error(t, err) })
	}
	s := newReviewSession(t, ParserBudget{})
	wire := append([]byte("$100\r\n"), bytes.Repeat([]byte{'a'}, 100)...)
	wire = append(wire, '\r', '\n')
	require.Equal(t, ProbeAccept, s.Probe(wire).Verdict, "probe must not require a bulk body beyond its prefix budget")
	r := s.Feed(0, time.Time{}, wire)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	r = s.Feed(1, time.Time{}, []byte("*2\r\n:1\r\n:2\r\n"))
	require.Nil(t, r.Err, "RESP arrays may contain non-bulk elements")
}

func TestProtocolSessionReviewLDAPStartTLS(t *testing.T) {
	for _, code := range []byte{0, 53} {
		t.Run(string(rune('0'+code)), func(t *testing.T) {
			s := newReviewSession(t, ParserBudget{})
			req := ldapSessionMsg(0x77, ldapSessionTLV(0x80, []byte("1.3.6.1.4.1.1466.20037")))
			require.Nil(t, s.Feed(0, time.Time{}, req).Err)
			r := s.Feed(1, time.Time{}, ldapSessionMsg(0x78, []byte{10, 1, code, 4, 0, 4, 0}))
			require.Nil(t, r.Err)
			if code == 0 {
				require.Equal(t, "tls", r.State)
			} else {
				require.Equal(t, "ldap", r.State)
			}
		})
	}
}

func TestProtocolSessionReviewPostgresAmbiguousExecute(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	body := append([]byte("Portal\x00"), 0, 0, 0, 0)
	r := s.Feed(0, time.Time{}, pgSessionMsg('E', body))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	short := pgSessionMsg('S', nil)
	require.Equal(t, uint32(4), binary.BigEndian.Uint32(short[1:]))
	require.NotEqual(t, ProbeReject, s.Probe(short).Verdict)
}

func TestProtocolSessionReviewStructuralBudgets(t *testing.T) {
	for name, budget := range map[string]ParserBudget{
		"depth":    {MaxRecursionDepth: 1},
		"elements": {MaxCollectionElements: 1},
	} {
		t.Run(name, func(t *testing.T) {
			s := newReviewSession(t, budget)
			r := s.Feed(0, time.Time{}, []byte("*2\r\n+one\r\n+two\r\n"))
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	s := newReviewSession(t, ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1024, ProbeBytes: 16})
	require.Nil(t, s.Feed(0, time.Time{}, reviewWSFrame(2, false, true, string(bytes.Repeat([]byte{'x'}, 40)))).Err)
	r := s.Feed(0, time.Time{}, reviewWSFrame(0, false, true, string(bytes.Repeat([]byte{'x'}, 40))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestProtocolSessionReviewGRPCErrors(t *testing.T) {
	for name, payload := range map[string][]byte{
		"compressed-flag":  {2, 0, 0, 0, 0},
		"declared-size":    {0, 0x7f, 0xff, 0xff, 0xff},
		"truncated-at-end": {0, 0, 0, 0, 2, 'x'},
	} {
		t.Run(name, func(t *testing.T) {
			s := reviewH2(t, "application/grpc")
			r := s.Feed(0, time.Time{}, h2TestFrame(0, 1, 1, payload))
			require.NotNil(t, r.Err)
			require.NotEqual(t, "decoded", r.Events[0].Status)
			require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
			// A message error releases its partial message, while the connection
			// retains HPACK state so an unrelated stream can still be decoded.
			r = s.Feed(0, time.Time{}, h2TestFrame(1, 5, 3, h2TestHeaders(t, ":method", "GET", ":scheme", "http", ":path", "/healthy")))
			require.Nil(t, r.Err)
			require.NotEmpty(t, r.Events)
			require.Empty(t, r.Events[0].Error)
			s.Close("FIN")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("accounting", func(t *testing.T) {
		s := reviewH2(t, "application/grpc")
		before := s.Stats().BufferedBytes
		payload := append([]byte{0, 0, 0, 0x30, 0}, bytes.Repeat([]byte{'x'}, 8192)...)
		require.Nil(t, s.Feed(0, time.Time{}, h2TestFrame(0, 0, 1, payload)).Err)
		require.GreaterOrEqual(t, s.Stats().BufferedBytes, before+int64(len(payload)))
		s.Close("FIN")
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestProtocolSessionReviewLDAPCorrelation(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	search := mustHexSession(t, "301c020101631704000a01020a01000201000201000101008702636e3000")
	require.Nil(t, s.Feed(0, time.Time{}, search).Err)
	abandon := ldapSessionMsg(0x50, []byte{1})
	abandon[4] = 2
	r := s.Feed(0, time.Time{}, abandon)
	require.Nil(t, r.Err)
	require.Equal(t, uint64(1), r.Events[0].Session["Abandoned Message ID"])
	require.Empty(t, s.Close("FIN"), "Abandon must remove the target, not its own MessageID")
}

func TestProtocolSessionReviewWebSocketRejectedUpgrade(t *testing.T) {
	s := newReviewSession(t, ParserBudget{})
	request := []byte("GET / HTTP/1.1\r\nHost: test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n\r\n")
	require.Nil(t, s.Feed(0, time.Time{}, request).Err)
	require.Nil(t, s.Feed(1, time.Time{}, []byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")).Err)
	require.Nil(t, s.Feed(0, time.Time{}, []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\n")).Err)
	r := s.Feed(1, time.Time{}, []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: other\r\nConnection: Upgrade\r\n\r\n"))
	require.NotEqual(t, "websocket", r.State, "a rejected prior request cannot authorize a later upgrade")
}

func FuzzProtocolSessionReview(f *testing.F) {
	for _, wire := range [][]byte{[]byte("*1\r\n$4\r\nPING\r\n"), []byte("$9223372036854775807\r\n"), {0x30, 0x84, 0xff, 0xff, 0xff, 0xff}, reviewWSFrame(1, false, true, "a"), pgSessionUntyped(196608, []byte("user\x00test\x00\x00"))} {
		f.Add(wire, uint8(3))
	}
	f.Fuzz(func(t *testing.T, wire []byte, chunk uint8) {
		if len(wire) > 8192 {
			t.Skip()
		}
		s, err := NewProtocolSession(ParserBudget{MaxMessageBytes: 4096, MaxBufferedBytes: 128 << 10})
		require.NoError(t, err)
		for at := 0; at < len(wire); {
			end := min(len(wire), at+int(chunk)+1)
			s.Feed(0, time.Time{}, wire[at:end])
			at = end
		}
		s.Close("FIN")
		require.Zero(t, s.Stats().BufferedBytes)
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(128<<10))
		require.Nil(t, s.(*captureSession).f.a.err.Load(), "malformed wire must fail without panic recovery")
	})
}
