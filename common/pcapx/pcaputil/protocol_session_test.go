package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func pgSessionMsg(typ byte, body []byte) []byte {
	w := make([]byte, 5+len(body))
	w[0] = typ
	binary.BigEndian.PutUint32(w[1:], uint32(4+len(body)))
	copy(w[5:], body)
	return w
}

func pgSessionUntyped(code uint32, body []byte) []byte {
	w := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(w, uint32(8+len(body)))
	binary.BigEndian.PutUint32(w[4:], code)
	copy(w[8:], body)
	return w
}

func ldapSessionTLV(tag byte, body []byte) []byte {
	return append([]byte{tag, byte(len(body))}, body...)
}

func ldapSessionMsg(tag byte, body []byte) []byte {
	inner := append([]byte{2, 1, 1}, ldapSessionTLV(tag, body)...)
	return ldapSessionTLV(0x30, inner)
}

func TestProtocolSessionHTTP2AndMySQLProbeFeed(t *testing.T) {
	ts := time.Unix(1, 0)
	t.Run("http2", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		preface := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
		p := s.Probe(preface)
		require.Equal(t, ProbeAccept, p.Verdict)
		require.Equal(t, "http2", p.Protocol)
		r := s.Feed(0, ts, preface)
		require.Nil(t, r.Err)
		require.GreaterOrEqual(t, len(r.Events), 1)
		require.Equal(t, "http2", r.Events[0].Protocol)
		require.Equal(t, "HTTP2InitialClientStream", r.Events[0].Entry)
		s.Feed(1, ts, h2TestFrame(4, 0, 0, nil))
		decoded := 0
		for _, e := range append(r.Events, s.Feed(0, ts, h2TestFrame(4, 1, 0, nil)).Events...) {
			if e.Status == "decoded" || e.Status == "deferred" {
				decoded++
				require.NotEmpty(t, e.Session)
			}
		}
		require.Greater(t, decoded, 0)
	})
	t.Run("mysql", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		g, resp := mysqlTestHandshake(mysqlTestCaps)
		p := s.Probe(g)
		require.Equal(t, ProbeAccept, p.Verdict)
		require.Equal(t, "mysql", p.Protocol)
		r := s.Feed(1, ts, g)
		require.Nil(t, r.Err)
		require.Equal(t, "MySQLGreetingFields", r.Events[0].Entry)
		r = s.Feed(0, ts, resp)
		require.Nil(t, r.Err)
		require.Contains(t, r.Events[0].Entry, "HandshakeResponse")
	})
}

func TestProtocolSessionPostgreSQLExtendedQuery(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	startup := pgSessionUntyped(196608, []byte("user\x00test\x00database\x00demo\x00\x00"))
	p := s.Probe(startup)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "postgresql", p.Protocol)
	r := s.Feed(0, ts, startup)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PostgreSQLStartupFields", r.Events[0].Entry)
	require.Equal(t, true, r.Events[0].Session["Frontend"])
	require.Equal(t, []byte("user"), findSessionField(r.Events[0].Fields, "Parameter Name"))
	require.Equal(t, []byte("test"), findSessionField(r.Events[0].Fields, "Parameter Value"))

	auth := pgSessionMsg('R', []byte{0, 0, 0, 0})
	r = s.Feed(1, ts, auth)
	require.Nil(t, r.Err)
	require.Equal(t, "Authentication", r.Events[0].Session["Message Name"])
	require.Equal(t, false, r.Events[0].Session["Frontend"])

	s.Feed(1, ts, pgSessionMsg('Z', []byte{'I'}))
	parse := pgSessionMsg('P', []byte("s1\x00SELECT $1\x00\x00\x01\x00\x00\x00\x17"))
	r = s.Feed(0, ts, parse)
	require.Nil(t, r.Err)
	require.Equal(t, "Parse", r.Events[0].Session["Message Name"])
	require.Equal(t, []byte("s1"), findSessionField(r.Events[0].Fields, "Statement Name"))
	require.Equal(t, []byte("SELECT $1"), findSessionField(r.Events[0].Fields, "Query Bytes"))
	bind := pgSessionMsg('B', []byte{0, 's', '1', 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 'x', 0, 0})
	require.Nil(t, s.Feed(0, ts, bind).Err)
	exec := pgSessionMsg('E', []byte{0, 0, 0, 0, 0})
	r = s.Feed(0, ts, exec)
	require.Nil(t, r.Err)
	require.Equal(t, "Execute", r.Events[0].Session["Message Name"])
	require.Nil(t, s.Feed(0, ts, pgSessionMsg('S', nil)).Err)

	require.Nil(t, s.Feed(1, ts, pgSessionMsg('1', nil)).Err)
	require.Nil(t, s.Feed(1, ts, pgSessionMsg('2', nil)).Err)
	rowDesc := pgSessionMsg('T', []byte{0, 1, 'i', 'd', 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 23, 0, 4, 0xff, 0xff, 0xff, 0xff, 0, 1})
	r = s.Feed(1, ts, rowDesc)
	require.Nil(t, r.Err)
	require.Equal(t, "RowDescription", r.Events[0].Session["Message Name"])
	require.Equal(t, []byte("id"), findSessionField(r.Events[0].Fields, "Column Name"))
	data := pgSessionMsg('D', []byte{0, 1, 0, 0, 0, 4, 0, 0, 0, 3})
	r = s.Feed(1, ts, data)
	require.Equal(t, "DataRow", r.Events[0].Session["Message Name"])
	require.Equal(t, []byte{0, 0, 0, 3}, findSessionField(r.Events[0].Fields, "Value"))
	r = s.Feed(1, ts, pgSessionMsg('C', append([]byte("SELECT 1"), 0)))
	require.Equal(t, "CommandComplete", r.Events[0].Session["Message Name"])
	require.Equal(t, []byte("SELECT 1"), findSessionField(r.Events[0].Fields, "Command Tag"))
	r = s.Feed(1, ts, pgSessionMsg('Z', []byte{'I'}))
	require.Equal(t, "ReadyForQuery", r.Events[0].Session["Message Name"])

	// Ambiguous 'E' is Execute on the frontend role, never by port 5432.
	r = s.Feed(0, ts, pgSessionMsg('E', []byte{0, 0, 0, 0, 1}))
	require.Nil(t, r.Err)
	require.Equal(t, "Execute", r.Events[0].Session["Message Name"])
	r = s.Feed(1, ts, pgSessionMsg('E', []byte("SERROR\x00C42601\x00Msyntax\x00\x00")))
	require.Nil(t, r.Err)
	require.Equal(t, "ErrorResponse", r.Events[0].Session["Message Name"])

	cut := s.Feed(0, ts, parse[:3])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	ssl, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	req := pgSessionUntyped(80877103, nil)
	require.Equal(t, ProbeAccept, ssl.Probe(req).Verdict)
	r = ssl.Feed(0, ts, req)
	require.Nil(t, r.Err)
	require.Equal(t, "PostgreSQLSSLRequestFields", r.Events[0].Entry)
	r = ssl.Feed(1, ts, []byte{'S'})
	require.Nil(t, r.Err)
	require.Equal(t, "SSLResponse", r.Events[0].Session["Message Name"])
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "tls", r.State)
}

func TestProtocolSessionLDAPBindSearchAndWrite(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bindBody := append([]byte{2, 1, 3}, ldapSessionTLV(4, nil)...)
	bindBody = append(bindBody, 0x80, 0)
	bind := ldapSessionMsg(0x60, bindBody)
	p := s.Probe(bind)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "ldap", p.Protocol)
	r := s.Feed(0, ts, bind)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LDAPBindRequestFields", r.Events[0].Entry)
	require.Equal(t, "BindRequest", r.Events[0].Session["Message Name"])
	resp := ldapSessionMsg(0x61, []byte{10, 1, 0, 4, 0, 4, 0})
	r = s.Feed(1, ts, resp)
	require.Nil(t, r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])
	search := mustHexSession(t, "301c020101631704000a01020a01000201000201000101008702636e3000")
	r = s.Feed(0, ts, search)
	require.Nil(t, r.Err)
	require.Equal(t, "SearchRequest", r.Events[0].Session["Message Name"])
	mod := mustHexSession(t, "301d02010166180404636e3d783010300e0a010030090402636e3103040179")
	mod[4] = 2 // Distinct MessageID while earlier requests remain outstanding.
	r = s.Feed(0, ts, mod)
	require.Nil(t, r.Err)
	require.Equal(t, "ModifyRequest", r.Events[0].Session["Message Name"])
	require.Equal(t, "cn=x", findSessionField(r.Events[0].Fields, "Object Name"))
	entry := ldapSessionMsg(0x64, append(ldapSessionTLV(4, []byte("cn=x")), 0x30, 0))
	r = s.Feed(1, ts, entry)
	require.Nil(t, r.Err)
	require.Equal(t, "SearchResultEntry", r.Events[0].Session["Message Name"])
	done := ldapSessionMsg(0x65, []byte{10, 1, 0, 4, 0, 4, 0})
	r = s.Feed(1, ts, done)
	require.Nil(t, r.Err)
	require.Equal(t, "SearchResultDone", r.Events[0].Session["Message Name"])
	add := mustHexSession(t, "301802010168130404636e3d78300b30090402636e3103040178")
	add[4] = 3 // Distinct MessageID while earlier requests remain outstanding.
	r = s.Feed(0, ts, add)
	require.Nil(t, r.Err)
	require.Equal(t, "AddRequest", r.Events[0].Session["Message Name"])
	del := mustHexSession(t, "30090201014a04636e3d78")
	del[4] = 4 // Distinct MessageID while earlier requests remain outstanding.
	r = s.Feed(0, ts, del)
	require.Nil(t, r.Err)
	require.Equal(t, "DelRequest", r.Events[0].Session["Message Name"])
	moddn := mustHexSession(t, "30180201016c130406636e3d6f6c640406636e3d6e65770101ff")
	moddn[4] = 5 // Distinct MessageID while earlier requests remain outstanding.
	r = s.Feed(0, ts, moddn)
	require.Nil(t, r.Err)
	require.Equal(t, "ModifyDNRequest", r.Events[0].Session["Message Name"])
	ext := mustHexSession(t, "3020020101771b8016312e332e362e312e342e312e313436362e32303033378101ff")
	ext[4] = 6 // Distinct MessageID while earlier requests remain outstanding.
	r = s.Feed(0, ts, ext)
	require.Nil(t, r.Err)
	require.Equal(t, "ExtendedRequest", r.Events[0].Session["Message Name"])
	require.Equal(t, true, r.Events[0].Session["StartTLS"])
	bad := s.Feed(0, ts, []byte{0x30, 0x05, 0x02, 0x01, 0x01, 0x01, 0x00})
	require.NotNil(t, bad.Err)
	require.NotEqual(t, ErrNeedMore, bad.Err.Kind)
}

func TestProtocolSessionWebSocketUpgradeAndFrames(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	req := []byte("GET /chat HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	resp := []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	r := s.Feed(0, ts, req)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "http", r.State)
	r = s.Feed(1, ts, resp)
	require.Nil(t, r.Err)
	require.Equal(t, "http->websocket", r.Events[0].Session["Protocol Transition"])
	hello := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	r = s.Feed(1, ts, hello)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "websocket", r.State)
	require.Equal(t, "text", r.Events[0].Session["Opcode Name"])
	masked := []byte{0x81, 0x85, 1, 2, 3, 4, 'h' ^ 1, 'e' ^ 2, 'l' ^ 3, 'l' ^ 4, 'o' ^ 1}
	r = s.Feed(0, ts, masked)
	require.Nil(t, r.Err)
	require.Equal(t, []byte("hello"), r.Events[0].Session["Application"])
	cont := []byte{0x01, 0x01, 'A', 0x80, 0x01, 'B'}
	r = s.Feed(1, ts, cont[:2])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
	r = s.Feed(1, ts, cont[2:])
	require.Nil(t, r.Err)
	ping := []byte{0x89, 0x00}
	require.Equal(t, "ping", s.Feed(1, ts, ping).Events[0].Session["Opcode Name"])
	closeFrame := []byte{0x88, 0x05, 0x03, 0xe8, 'b', 'y', 'e'}
	r = s.Feed(1, ts, closeFrame)
	require.Nil(t, r.Err)
	require.Equal(t, "close", r.Events[0].Session["Opcode Name"])
	require.Equal(t, uint64(1000), r.Events[0].Session["Close Code"])
	require.NotNil(t, s.Feed(1, ts, []byte{0x83, 0x00}).Err)
}

func TestProtocolSessionRedisRESP2RESP3AndPipeline(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	ping := []byte("*1\r\n$4\r\nPING\r\n")
	p := s.Probe(ping)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "redis", p.Protocol)
	r := s.Feed(0, ts, ping)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Array", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte("+PONG\r\n"))
	require.Equal(t, "Simple", r.Events[0].Session["RESP Type"])
	pipe := []byte("*2\r\n$3\r\nGET\r\n$1\r\nk\r\n*2\r\n$3\r\nGET\r\n$1\r\nv\r\n")
	r = s.Feed(0, ts, pipe)
	require.Nil(t, r.Err)
	require.GreaterOrEqual(t, len(r.Events), 2)
	r = s.Feed(1, ts, []byte("%1\r\n+key\r\n+val\r\n"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Map", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte(">2\r\n+message\r\n+chan\r\n"))
	require.Equal(t, "Push", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte("#t\r\n"))
	require.Equal(t, "Boolean", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte("_\r\n"))
	require.Equal(t, "Null", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte("!5\r\nERR x\r\n"))
	require.Equal(t, "BlobError", r.Events[0].Session["RESP Type"])
	r = s.Feed(1, ts, []byte("=9\r\ntxt:hello\r\n"))
	require.Equal(t, "Verbatim", r.Events[0].Session["RESP Type"])
	cut := s.Feed(0, ts, []byte("$6\r\nHEL"))
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionGRPCOverHTTP2(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	preface := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
	require.Nil(t, s.Feed(0, ts, preface).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 0, 0, nil)).Err)
	require.Nil(t, s.Feed(0, ts, h2TestFrame(4, 1, 0, nil)).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 1, 0, nil)).Err)
	headers := h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/svc.P/M", ":authority", "example.test", "content-type", "application/grpc")
	r := s.Feed(0, ts, h2TestFrame(1, 4, 1, headers))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["GRPC"])
	msg := make([]byte, 5+4)
	binary.BigEndian.PutUint32(msg[1:5], 4)
	copy(msg[5:], []byte{1, 2, 3, 4})
	r = s.Feed(0, ts, h2TestFrame(0, 1, 1, msg))
	require.Nil(t, r.Err, "%v", r.Err)
	require.NotEmpty(t, r.Events)
	msgs, ok := r.Events[0].Session["GRPC Messages"].([]map[string]any)
	require.True(t, ok, "DATA must expose gRPC message prefix fields")
	require.Equal(t, uint64(4), msgs[0]["Length"])
	require.Equal(t, []byte{1, 2, 3, 4}, msgs[0]["Payload"])
	resp := h2TestHeaders(t, ":status", "200", "content-type", "application/grpc")
	r = s.Feed(1, ts, h2TestFrame(1, 4, 1, resp))
	require.Nil(t, r.Err, "%v", r.Err)
	trailers := h2TestHeaders(t, "grpc-status", "0", "grpc-message", "ok")
	r = s.Feed(1, ts, h2TestFrame(1, 5, 1, trailers))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "0", r.Events[0].Session["GRPC Status"])
}

func TestProtocolSessionGRPCSpanAndPackDATA(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	preface := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
	require.Nil(t, s.Feed(0, ts, preface).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 0, 0, nil)).Err)
	require.Nil(t, s.Feed(0, ts, h2TestFrame(4, 1, 0, nil)).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 1, 0, nil)).Err)
	headers := h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/svc.P/M", ":authority", "example.test", "content-type", "application/grpc")
	require.Nil(t, s.Feed(0, ts, h2TestFrame(1, 4, 1, headers)).Err)
	first := []byte{0, 0, 0, 0, 3, 'a'}
	r := s.Feed(0, ts, h2TestFrame(0, 0, 1, first))
	require.Nil(t, r.Err)
	require.Nil(t, r.Events[0].Session["GRPC Messages"])
	r = s.Feed(0, ts, h2TestFrame(0, 0, 1, []byte{'b', 'c'}))
	require.Nil(t, r.Err)
	msgs := r.Events[0].Session["GRPC Messages"].([]map[string]any)
	require.Equal(t, uint64(3), msgs[0]["Length"])
	require.Equal(t, []byte("abc"), msgs[0]["Payload"])
	packed := make([]byte, 0, 20)
	for _, payload := range [][]byte{{1}, {2, 3}} {
		msg := make([]byte, 5+len(payload))
		binary.BigEndian.PutUint32(msg[1:5], uint32(len(payload)))
		copy(msg[5:], payload)
		packed = append(packed, msg...)
	}
	r = s.Feed(0, ts, h2TestFrame(0, 1, 1, packed))
	require.Nil(t, r.Err)
	msgs = r.Events[0].Session["GRPC Messages"].([]map[string]any)
	require.Len(t, msgs, 2)
	require.Equal(t, []byte{1}, msgs[0]["Payload"])
	require.Equal(t, []byte{2, 3}, msgs[1]["Payload"])
}

func TestProtocolSessionFragmentationInvariant(t *testing.T) {
	startup := pgSessionUntyped(196608, []byte("user\x00test\x00database\x00demo\x00\x00"))
	auth := pgSessionMsg('R', []byte{0, 0, 0, 0})
	ready := pgSessionMsg('Z', []byte{'I'})
	query := pgSessionMsg('Q', append([]byte("SELECT 1"), 0))
	complete := pgSessionMsg('C', append([]byte("SELECT 1"), 0))
	steps := []sessionStep{
		{0, startup}, {1, auth}, {1, ready}, {0, query}, {1, complete}, {1, ready},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						if name, ok := e.Session["Message Name"].(string); ok {
							names = append(names, name)
						} else {
							names = append(names, e.Entry)
						}
					}
				}
				w = w[n:]
			}
		}
		s.Close("FIN")
		return names
	})
}

func TestProtocolSessionFragmentationOtherProtocols(t *testing.T) {
	t.Run("redis", func(t *testing.T) {
		steps := []sessionStep{{0, []byte("*1\r\n$4\r\nPING\r\n")}, {1, []byte("+PONG\r\n")}}
		assertFragmentation(t, steps, func(chunk int) []string {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			ts := time.Unix(1, 0)
			var names []string
			for _, st := range steps {
				for w := st.wire; len(w) > 0; {
					n := len(w)
					if chunk > 0 {
						n = min(n, chunk)
					}
					r := s.Feed(st.dir, ts, w[:n])
					for _, e := range r.Events {
						if e.Status == "decoded" || e.Status == "deferred" {
							names = append(names, fmt.Sprint(e.Session["RESP Type"]))
						}
					}
					w = w[n:]
				}
			}
			return names
		})
	})
	t.Run("websocket", func(t *testing.T) {
		steps := []sessionStep{
			{0, []byte("GET /chat HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")},
			{1, []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")},
			{1, []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}},
		}
		assertFragmentation(t, steps, func(chunk int) []string {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			ts := time.Unix(1, 0)
			var names []string
			for _, st := range steps {
				for w := st.wire; len(w) > 0; {
					n := len(w)
					if chunk > 0 {
						n = min(n, chunk)
					}
					r := s.Feed(st.dir, ts, w[:n])
					for _, e := range r.Events {
						if e.Status == "decoded" || e.Status == "deferred" {
							if name, ok := e.Session["Opcode Name"].(string); ok {
								names = append(names, name)
							} else if tr, ok := e.Session["Protocol Transition"].(string); ok {
								names = append(names, tr)
							} else {
								names = append(names, e.Entry)
							}
						}
					}
					w = w[n:]
				}
			}
			return names
		})
	})
	t.Run("ldap", func(t *testing.T) {
		bindBody := append([]byte{2, 1, 3}, ldapSessionTLV(4, nil)...)
		bindBody = append(bindBody, 0x80, 0)
		steps := []sessionStep{
			{0, ldapSessionMsg(0x60, bindBody)},
			{1, ldapSessionMsg(0x61, []byte{10, 1, 0, 4, 0, 4, 0})},
		}
		assertFragmentation(t, steps, func(chunk int) []string {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			ts := time.Unix(1, 0)
			var names []string
			for _, st := range steps {
				for w := st.wire; len(w) > 0; {
					n := len(w)
					if chunk > 0 {
						n = min(n, chunk)
					}
					r := s.Feed(st.dir, ts, w[:n])
					for _, e := range r.Events {
						if e.Status == "decoded" || e.Status == "deferred" {
							names = append(names, fmt.Sprint(e.Session["Message Name"]))
						}
					}
					w = w[n:]
				}
			}
			return names
		})
	})
}

func TestProtocolSessionProbeAndBudget(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte("PRI * HTTP")).Verdict)
	require.Equal(t, ProbeReject, s.Probe([]byte{0xff, 0x00, 0x01}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0, 0, 0, 8}).Verdict)
	limited, err := NewProtocolSession(ParserBudget{MaxMessageBytes: 64, MaxBufferedBytes: 1 << 20, ProbeBytes: 16})
	require.NoError(t, err)
	body := make([]byte, 80)
	r := limited.Feed(0, time.Unix(1, 0), pgSessionMsg('Q', append(body, 0)))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
}

func assertFragmentation(t *testing.T, _ []sessionStep, run func(chunk int) []string) {
	t.Helper()
	whole := run(0)
	require.NotEmpty(t, whole)
	for _, chunk := range []int{1, 2, 5, 9} {
		got := run(chunk)
		require.Equal(t, whole, got, "chunk=%d", chunk)
	}
}

func mustHexSession(t testing.TB, s string) []byte {
	t.Helper()
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		var v byte
		for _, c := range []byte{s[2*i], s[2*i+1]} {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= c - '0'
			case c >= 'a' && c <= 'f':
				v |= c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v |= c - 'A' + 10
			}
		}
		b[i] = v
	}
	return b
}
