package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolSessionDoQQueryResponse(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)

	q := dnsQuery(0x1234, "example.com", 1)
	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, q)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doq", r.Events[0].Protocol)
	require.Equal(t, true, r.Events[0].Session["DoQ"])
	require.Equal(t, "quic->doq", r.Events[0].Session["Protocol Transition"])
	require.Equal(t, "Query", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(0x1234), r.Events[0].Session["Transaction ID"])
	require.Equal(t, "example.com", r.Events[0].Session["QNAME"])
	require.Equal(t, "A", r.Events[0].Session["QTYPE Name"])
	require.Equal(t, uint64(0), r.Events[0].Session["DoQ Stream ID"])

	r = s.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34}))))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doq", r.Events[0].Protocol)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "example.com", r.Events[0].Session["Matched Request"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	require.Equal(t, []string{"93.184.216.34"}, r.Events[0].Session["A Records"])
}

func TestProtocolSessionDoQFailClosedResetAndMissing(t *testing.T) {
	s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	dcid, scid := quicTestDCID(), quicTestSCID()
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, dnsQuery(1, "a.example", 1)))).Err)

	bad := make([]byte, 4)
	binary.BigEndian.PutUint16(bad, 3)
	r := s.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 1, quicStreamFrame(0, uint64(len(dnsQuery(1, "a.example", 1))), false, bad)))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)

	s2, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	r = s2.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, dnsAResponse(9, "orphan.example", [4]byte{1, 1, 1, 1}))))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	require.Equal(t, "missing-request", r.Events[0].Session["Association Status"])
	require.Equal(t, "doq", r.Events[0].Protocol)

	s3, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))).Err)
	require.Nil(t, s3.Feed(0, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, false, dnsQuery(2, "rst.example", 1)))).Err)
	r = s3.Feed(1, ts, quicLongPacket(1, 1, dcid, scid, nil, 0, quicResetStream(0, 1, 0)))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "doq", r.Events[0].Protocol)
	require.Equal(t, "reset", r.Events[0].Session["DoQ Stream State"])
}

func TestProtocolSessionDoQFragmentation(t *testing.T) {
	dcid, scid := quicTestDCID(), quicTestSCID()
	steps := []sessionStep{
		{0, quicLongPacket(0, 1, dcid, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))},
		{0, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, dnsQuery(0x22, "ietf.org", 1)))},
		{1, quicLongPacket(1, 1, dcid, scid, nil, 0, quicStreamFrame(0, 0, true, dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44})))},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewDecryptedQUICSession(DefaultParserBudget(), "synthetic algorithm fixture; not wire evidence")
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
					if e.Protocol == "doq" && e.Session["Packet Name"] != nil {
						names = append(names, fmt.Sprintf("%v:%v:%v", e.Session["DoQ Stream ID"], e.Session["Packet Name"], e.Session["QNAME"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
