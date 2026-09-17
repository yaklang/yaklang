package pcaputil

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dnsTCP(msg []byte) []byte {
	h := make([]byte, 2)
	binary.BigEndian.PutUint16(h, uint16(len(msg)))
	return append(h, msg...)
}

func dnsQName(name string) []byte {
	if name == "" || name == "." {
		return []byte{0}
	}
	var b []byte
	for _, lab := range strings.Split(name, ".") {
		b = append(b, byte(len(lab)))
		b = append(b, lab...)
	}
	return append(b, 0)
}

func dnsQuery(id uint16, name string, qtype uint16) []byte {
	q := dnsQName(name)
	var t [4]byte
	binary.BigEndian.PutUint16(t[0:2], qtype)
	binary.BigEndian.PutUint16(t[2:4], 1)
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], id)
	binary.BigEndian.PutUint16(msg[2:4], 0x0100) // RD
	binary.BigEndian.PutUint16(msg[4:6], 1)
	return dnsTCP(append(append(msg, q...), t[:]...))
}

func dnsAResponse(id uint16, name string, ip [4]byte) []byte {
	q := dnsQuery(id, name, 1)
	msg := append([]byte{}, q[2:]...)
	binary.BigEndian.PutUint16(msg[2:4], 0x8180) // QR RD RA
	binary.BigEndian.PutUint16(msg[6:8], 1)
	ans := []byte{0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, ip[0], ip[1], ip[2], ip[3]}
	return dnsTCP(append(msg, ans...))
}

func TestProtocolSessionDoTQueryResponse(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	q := dnsQuery(0x1234, "example.com", 1)
	p := s.Probe(q)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "dot", p.Protocol)
	require.Equal(t, "rfc7858", p.Version)

	r := s.Feed(0, ts, q)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Query", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(0x1234), r.Events[0].Session["Transaction ID"])
	require.Equal(t, "example.com", r.Events[0].Session["QNAME"])
	require.Equal(t, "A", r.Events[0].Session["QTYPE Name"])
	require.Equal(t, false, r.Events[0].Session["QR"])

	r = s.Feed(1, ts, dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "example.com", r.Events[0].Session["Matched Request"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	require.Equal(t, []string{"93.184.216.34"}, r.Events[0].Session["A Records"])

	// Interleaved transaction IDs.
	require.Nil(t, s.Feed(0, ts, dnsQuery(1, "a.example", 1)).Err)
	require.Nil(t, s.Feed(0, ts, dnsQuery(2, "b.example", 1)).Err)
	r = s.Feed(1, ts, dnsAResponse(2, "b.example", [4]byte{1, 2, 3, 4}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint16(2), r.Events[0].Session["Transaction ID"])
	require.Equal(t, "b.example", r.Events[0].Session["Matched Request"])
	r = s.Feed(1, ts, dnsAResponse(1, "a.example", [4]byte{5, 6, 7, 8}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint16(1), r.Events[0].Session["Transaction ID"])
	require.Equal(t, []string{"5.6.7.8"}, r.Events[0].Session["A Records"])
}

func TestProtocolSessionDoTFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "dot", s.Probe([]byte{0, 5, 1, 2, 3, 4, 5}).Protocol)
	require.NotEqual(t, ProbeAccept, s.Probe([]byte{0, 5, 1, 2, 3, 4, 5}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0, 20}).Verdict)
	// TLS record header is the outer transport, not DoT (RFC 7858 plaintext DNS).
	tlsHello := []byte{0x16, 0x03, 0x01, 0x00, 0x20, 0x01, 0x00, 0x00, 0x1c, 0, 0, 0, 0, 0, 0, 0}
	require.NotEqual(t, "dot", s.Probe(tlsHello).Protocol)

	cut := s.Feed(0, ts, dnsQuery(1, "example.com", 1)[:4])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, dnsQuery(1, "example.com", 1)).Err)
	r := s2.Feed(0, ts, append([]byte{0, 5}, 1, 2, 3, 4, 5))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(1, ts, dnsAResponse(99, "orphan.example", [4]byte{9, 9, 9, 9}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", r.Events[0].Session["Association Status"])
}

func TestProtocolSessionDoTFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, dnsQuery(0x1234, "example.com", 1)},
		{1, dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34})},
		{0, dnsQuery(0x22, "ietf.org", 1)},
		{1, dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44})},
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
						names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["QNAME"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}

func TestProtocolSessionDoTCoalescedMessages(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	both := append(dnsQuery(1, "one.example", 1), dnsQuery(2, "two.example", 1)...)
	r := s.Feed(0, ts, both)
	require.Nil(t, r.Err, "%v", r.Err)
	require.GreaterOrEqual(t, len(r.Events), 2)
	require.Equal(t, "one.example", r.Events[0].Session["QNAME"])
	require.Equal(t, "two.example", r.Events[1].Session["QNAME"])
}
