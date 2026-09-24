package pcaputil

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dnsWire(prefixed []byte) []byte {
	return prefixed[2:]
}

func dohGET(host, name string, id uint16) []byte {
	q := dnsWire(dnsQuery(id, name, 1))
	enc := base64.RawURLEncoding.EncodeToString(q)
	return []byte("GET /dns-query?dns=" + enc + " HTTP/1.1\r\nHost: " + host + "\r\nAccept: application/dns-message\r\n\r\n")
}

func dohPOST(host string, msg []byte) []byte {
	return []byte("POST /dns-query HTTP/1.1\r\nHost: " + host + "\r\nContent-Type: application/dns-message\r\nContent-Length: " +
		strconv.Itoa(len(msg)) + "\r\n\r\n" + string(msg))
}

func dohHTTPResp(status int, msg []byte) []byte {
	reason := "OK"
	if status == 415 {
		reason = "Unsupported Media Type"
	}
	if msg == nil {
		body := []byte("error")
		return []byte("HTTP/1.1 " + strconv.Itoa(status) + " " + reason + "\r\nContent-Type: text/plain\r\nContent-Length: " +
			strconv.Itoa(len(body)) + "\r\n\r\n" + string(body))
	}
	return []byte("HTTP/1.1 " + strconv.Itoa(status) + " " + reason + "\r\nContent-Type: application/dns-message\r\nContent-Length: " +
		strconv.Itoa(len(msg)) + "\r\n\r\n" + string(msg))
}

func TestProtocolSessionDoHHTTP1GETPOST(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	post := dohPOST("dns.example.test", dnsWire(dnsQuery(0x1234, "example.com", 1)))
	p := s.Probe(post)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "http", p.Protocol)

	r := s.Feed(0, ts, post)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doh", r.Events[0].Protocol)
	require.Equal(t, "http->doh", r.Events[0].Session["Protocol Transition"])
	require.Equal(t, "POST", r.Events[0].Session["Method"])
	require.Equal(t, "Query", r.Events[0].Session["Packet Name"])
	require.Equal(t, "example.com", r.Events[0].Session["QNAME"])
	require.Equal(t, uint16(0x1234), r.Events[0].Session["Transaction ID"])

	resp := dohHTTPResp(200, dnsWire(dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34})))
	r = s.Feed(1, ts, resp)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doh", r.Events[0].Protocol)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "example.com", r.Events[0].Session["Matched Request"])
	require.Equal(t, []string{"93.184.216.34"}, r.Events[0].Session["A Records"])

	r = s.Feed(0, ts, dohGET("dns.example.test", "ietf.org", 0x22))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GET", r.Events[0].Session["Method"])
	require.Equal(t, "ietf.org", r.Events[0].Session["QNAME"])
	r = s.Feed(1, ts, dohHTTPResp(200, dnsWire(dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44}))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	require.Equal(t, []string{"4.31.198.44"}, r.Events[0].Session["A Records"])
}

func TestProtocolSessionDoHHTTP2GET(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	preface := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
	require.Nil(t, s.Feed(0, ts, preface).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 0, 0, nil)).Err)
	require.Nil(t, s.Feed(0, ts, h2TestFrame(4, 1, 0, nil)).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 1, 0, nil)).Err)

	q := dnsWire(dnsQuery(7, "example.com", 1))
	path := "/dns-query?dns=" + base64.RawURLEncoding.EncodeToString(q)
	headers := h2TestHeaders(t, ":method", "GET", ":scheme", "http", ":path", path, ":authority", "dns.example.test", "accept", "application/dns-message")
	r := s.Feed(0, ts, h2TestFrame(1, 5, 1, headers))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doh", r.Events[0].Protocol)
	require.Equal(t, "http2->doh", r.Events[0].Session["Protocol Transition"])
	require.Equal(t, "example.com", r.Events[0].Session["QNAME"])
	require.Equal(t, uint32(1), r.Events[0].Session["Stream ID"])

	body := dnsWire(dnsAResponse(7, "example.com", [4]byte{1, 2, 3, 4}))
	respH := h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message")
	require.Nil(t, s.Feed(1, ts, h2TestFrame(1, 4, 1, respH)).Err)
	r = s.Feed(1, ts, h2TestFrame(0, 1, 1, body))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doh", r.Events[0].Protocol)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "example.com", r.Events[0].Session["Matched Request"])
	require.Equal(t, []string{"1.2.3.4"}, r.Events[0].Session["A Records"])
}

func TestProtocolSessionDoHHTTP2DoesNotUseAcceptAlone(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	preface := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
	require.Nil(t, s.Feed(0, ts, preface).Err)
	require.Nil(t, s.Feed(1, ts, h2TestFrame(4, 0, 0, nil)).Err)
	headers := h2TestHeaders(t, ":method", "GET", ":scheme", "https", ":path", "/search?filter=dns=example.com", ":authority", "www.example.test", "accept", "application/dns-message")
	r := s.Feed(0, ts, h2TestFrame(1, 5, 1, headers))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "http2", r.Events[0].Protocol)
	require.Equal(t, "decoded", r.Events[0].Status)
	require.NotEqual(t, true, r.Events[0].Session["DoH"])
}

func TestProtocolSessionDoHFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	plain := []byte("GET /index.html HTTP/1.1\r\nHost: example.test\r\n\r\n")
	r := s.Feed(0, ts, plain)
	require.Nil(t, r.Err, "%v", r.Err)
	require.NotEqual(t, "doh", r.Events[0].Protocol)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	badCT := []byte("POST /dns-query HTTP/1.1\r\nHost: dns.example.test\r\nContent-Type: text/plain\r\nContent-Length: 12\r\n\r\n0123456789ab")
	r = s2.Feed(0, ts, badCT)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "http", r.Events[0].Protocol)
	require.Equal(t, "decoded", r.Events[0].Status)
	require.NotEqual(t, true, r.Events[0].Session["DoH"])

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	short := dohPOST("dns.example.test", make([]byte, 11))
	r = s3.Feed(0, ts, short)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	badB64 := []byte("GET /dns-query?dns=!!! HTTP/1.1\r\nHost: dns.example.test\r\nAccept: application/dns-message\r\n\r\n")
	r = s4.Feed(0, ts, badB64)
	require.NotNil(t, r.Err)

	s5, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s5.Feed(0, ts, dohGET("dns.example.test", "x.test", 1)).Err)
	r = s5.Feed(1, ts, dohHTTPResp(415, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Error", r.Events[0].Session["Packet Name"])
	require.Equal(t, []byte("error"), r.Events[0].Session["Error Body"])
	require.Equal(t, 415, r.Events[0].Session["HTTP Status"])
}

func TestProtocolSessionDoHFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, dohPOST("dns.example.test", dnsWire(dnsQuery(0x1234, "example.com", 1)))},
		{1, dohHTTPResp(200, dnsWire(dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34})))},
		{0, dohGET("dns.example.test", "ietf.org", 0x22)},
		{1, dohHTTPResp(200, dnsWire(dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44})))},
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
						if e.Session["DoH"] == true {
							names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["QNAME"]))
						}
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
