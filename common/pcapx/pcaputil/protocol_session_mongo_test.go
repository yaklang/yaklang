package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
)

func mongoLE32(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}

func mongoBSONInt32(name string, v uint32) []byte {
	n := append([]byte{0x10}, append([]byte(name), 0)...)
	n = append(n, mongoLE32(v)...)
	n = append(n, 0)
	return append(mongoLE32(uint32(4+len(n))), n...)
}

func mongoOpMsg(req, respTo uint32, flags uint32, sections ...[]byte) []byte {
	body := mongoLE32(flags)
	for _, s := range sections {
		body = append(body, s...)
	}
	total := uint32(16 + len(body))
	w := make([]byte, 0, total)
	w = append(w, mongoLE32(total)...)
	w = append(w, mongoLE32(req)...)
	w = append(w, mongoLE32(respTo)...)
	w = append(w, mongoLE32(2013)...)
	return append(w, body...)
}

func mongoKind0(doc []byte) []byte {
	return append([]byte{0}, doc...)
}

func mongoKind1(id string, docs ...[]byte) []byte {
	inner := append([]byte(id), 0)
	for _, d := range docs {
		inner = append(inner, d...)
	}
	sec := append(mongoLE32(uint32(4+len(inner))), inner...)
	return append([]byte{1}, sec...)
}

func mongoCompressed(req, respTo uint32, orig uint32, compressor byte, uncompressed []byte) []byte {
	var payload []byte
	switch compressor {
	case 0:
		payload = uncompressed
	case 1:
		payload = snappy.Encode(nil, uncompressed)
	default:
		payload = uncompressed
	}
	body := append(mongoLE32(orig), mongoLE32(uint32(len(uncompressed)))...)
	body = append(body, compressor)
	body = append(body, payload...)
	total := uint32(16 + len(body))
	w := make([]byte, 0, total)
	w = append(w, mongoLE32(total)...)
	w = append(w, mongoLE32(req)...)
	w = append(w, mongoLE32(respTo)...)
	w = append(w, mongoLE32(2012)...)
	return append(w, body...)
}

func TestProtocolSessionMongoOPMsgAndCompressed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	ping := mongoOpMsg(1, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))
	p := s.Probe(ping)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "mongodb", p.Protocol)
	r := s.Feed(0, ts, ping)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "OP_MSG", r.Events[0].Session["Opcode Name"])
	require.Equal(t, uint64(1), r.Events[0].Session["Request ID"])
	require.Equal(t, "ping", findSessionField(r.Events[0].Fields, "Name"))
	require.Equal(t, uint32(1), findSessionField(r.Events[0].Fields, "Int32"))

	ok := mongoOpMsg(2, 1, 0, mongoKind0(mongoBSONInt32("ok", 1)))
	r = s.Feed(1, ts, ok)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Matched Request"])
	require.Equal(t, uint64(1), r.Events[0].Session["Response To"])

	seq := mongoOpMsg(3, 0, 0, mongoKind0(mongoBSONInt32("insert", 1)), mongoKind1("documents", mongoBSONInt32("n", 1)))
	r = s.Feed(0, ts, seq)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "documents", r.Events[0].Session["Sequence Identifier"])

	inner := mongoOpMsg(4, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))
	comp := mongoCompressed(4, 0, 2013, 1, inner[16:])
	r = s.Feed(0, ts, comp)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "OP_COMPRESSED", r.Events[0].Session["Opcode Name"])
	require.Equal(t, "snappy", r.Events[0].Session["Compressor"])
	require.Equal(t, "OP_MSG", r.Events[0].Session["Inner Opcode Name"])

	cut := s.Feed(0, ts, ping[:6])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionMongoFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, mongoOpMsg(1, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))).Err)
	badOp := append([]byte{}, mongoOpMsg(2, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))...)
	binary.LittleEndian.PutUint32(badOp[12:16], 1999)
	r := s.Feed(0, ts, badOp)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, mongoOpMsg(1, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))).Err)
	badComp := mongoCompressed(2, 0, 2013, 9, []byte{1, 2, 3, 4})
	r = s2.Feed(0, ts, badComp)
	require.NotNil(t, r.Err)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, []byte{8, 0, 0, 0, 1, 0, 0, 0})
	require.NotNil(t, r.Err)
}

func TestProtocolSessionMongoFragmentation(t *testing.T) {
	req := mongoOpMsg(1, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))
	resp := mongoOpMsg(2, 1, 0, mongoKind0(mongoBSONInt32("ok", 1)))
	steps := []sessionStep{{0, req}, {1, resp}}
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
						names = append(names, fmt.Sprint(e.Session["Opcode Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
