package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tdsPkt(typ, status byte, payload []byte) []byte {
	w := make([]byte, 8+len(payload))
	w[0], w[1] = typ, status
	binary.BigEndian.PutUint16(w[2:4], uint16(8+len(payload)))
	w[6] = 1
	copy(w[8:], payload)
	return w
}

func tdsPkts(typ byte, body []byte, chunk int) []byte {
	if chunk <= 0 {
		chunk = 65520
	}
	var wire []byte
	id := byte(1)
	for at := 0; at < len(body) || at == 0; {
		n := min(chunk, len(body)-at)
		h := []byte{typ, 0, 0, 0, 0, 0, id, 0}
		binary.BigEndian.PutUint16(h[2:4], uint16(n+8))
		if at+n == len(body) {
			h[1] = 1
		}
		wire = append(wire, h...)
		wire = append(wire, body[at:at+n]...)
		at += n
		id++
		if n == 0 {
			break
		}
	}
	return wire
}

func tdsUCS2(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		out = append(out, s[i], 0)
	}
	return out
}

func tdsPrelogin(encrypt byte) []byte {
	version := []byte{12, 0, 0, 0, 0, 0}
	enc := []byte{encrypt}
	header := 11
	var p []byte
	p = append(p, 0)
	p = binary.BigEndian.AppendUint16(p, uint16(header))
	p = binary.BigEndian.AppendUint16(p, 6)
	p = append(p, 1)
	p = binary.BigEndian.AppendUint16(p, uint16(header+len(version)))
	p = binary.BigEndian.AppendUint16(p, 1)
	p = append(p, 0xff)
	p = append(p, version...)
	p = append(p, enc...)
	return tdsPkt(18, 1, p)
}

func tdsPreloginReply(encrypt byte) []byte {
	w := tdsPrelogin(encrypt)
	w[0] = 4
	return w
}

func tdsLogin7() []byte {
	payload, err := hex.DecodeString("6600000004000074001000000000000000000000000000000000000000000000000000005e00040000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000068006f0073007400")
	if err != nil {
		panic(err)
	}
	return tdsPkt(16, 1, payload)
}

func tdsAllHeaders() []byte {
	b, err := hex.DecodeString("16000000120000000200080706050403020101000000")
	if err != nil {
		panic(err)
	}
	return b
}

func tdsSQLBatch72(query string) []byte {
	return tdsPkt(1, 1, append(tdsAllHeaders(), tdsUCS2(query)...))
}

func tdsRPC72() []byte {
	rpc, err := hex.DecodeString("ffff0a0000000000260404feffffff")
	if err != nil {
		panic(err)
	}
	return tdsPkt(3, 1, append(tdsAllHeaders(), rpc...))
}

func tdsDone72() []byte {
	body, err := hex.DecodeString("fd1000c1000100000000000000")
	if err != nil {
		panic(err)
	}
	return tdsPkt(4, 1, body)
}

func tdsQueryResult72() []byte {
	// COLMETADATA int4 column x, ROW -2, DONE, RETURNSTATUS -3.
	body := []byte{0x81, 1, 0, 0, 0}
	rest, err := hex.DecodeString("0000010038017800d1fefffffffd1000c100010000000000000079fdffffff")
	if err != nil {
		panic(err)
	}
	return tdsPkt(4, 1, append(body, rest...))
}

func TestProtocolSessionTDSPreloginLoginBatchRPC(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	pre := tdsPrelogin(2)
	p := s.Probe(pre)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "tds", p.Protocol)
	r := s.Feed(0, ts, pre)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PRELOGIN", r.Events[0].Session["Packet Name"])
	require.Equal(t, byte(2), r.Events[0].Session["Encryption"])
	require.Equal(t, "ENCRYPT_NOT_SUP", r.Events[0].Session["Encryption Name"])
	require.Equal(t, true, r.Events[0].Session["Outstanding"])

	r = s.Feed(1, ts, tdsPreloginReply(2))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "PRELOGIN Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "PRELOGIN", r.Events[0].Session["Matched Request"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])

	login := tdsLogin7()
	r = s.Feed(0, ts, login)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LOGIN7", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint32(0x74000004), r.Events[0].Session["TDS Version"])
	require.Equal(t, "7.4", r.Events[0].Session["Version Name"])
	require.Equal(t, "host", r.Events[0].Session["HostName"])

	r = s.Feed(1, ts, tdsDone72())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LOGIN7", r.Events[0].Session["Matched Request"])
	require.Equal(t, true, r.Events[0].Session["Logged In"])
	require.Equal(t, []string{"DONE"}, r.Events[0].Session["Token Names"])

	r = s.Feed(0, ts, tdsSQLBatch72("SELECT 1"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SQLBatch", r.Events[0].Session["Packet Name"])
	require.Equal(t, "SELECT 1", r.Events[0].Session["SQL Text"])

	r = s.Feed(1, ts, tdsQueryResult72())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SQLBatch", r.Events[0].Session["Matched Request"])
	require.Contains(t, r.Events[0].Session["Token Names"], "COLMETADATA")
	require.Contains(t, r.Events[0].Session["Token Names"], "ROW")
	require.Contains(t, r.Events[0].Session["Token Names"], "DONE")

	r = s.Feed(0, ts, tdsRPC72())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RPC", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(10), r.Events[0].Session["Procedure ID"])

	r = s.Feed(1, ts, tdsDone72())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RPC", r.Events[0].Session["Matched Request"])

	cut := s.Feed(0, ts, tdsSQLBatch72("SELECT 1")[:5])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionTDSFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "tds", s.Probe(tdsSQLBatch72("SELECT 1")).Protocol)
	require.Equal(t, ProbeReject, s.Probe([]byte{99, 1, 0, 8, 0, 0, 1, 0}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0x12, 1}).Verdict)

	r := s.Feed(0, ts, []byte{99, 1, 0, 8, 0, 0, 1, 0})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, tdsPrelogin(2)).Err)
	require.Nil(t, s2.Feed(1, ts, tdsPreloginReply(2)).Err)
	r = s2.Feed(0, ts, tdsSQLBatch72("SELECT 1"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, tdsPrelogin(2)).Err)
	require.Nil(t, s3.Feed(1, ts, tdsPreloginReply(2)).Err)
	badLogin := tdsLogin7()
	binary.LittleEndian.PutUint32(badLogin[12:16], 0x7a000000)
	r = s3.Feed(0, ts, badLogin)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s4.Feed(0, ts, tdsPrelogin(2)[:6])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)

	s5, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s5.Feed(0, ts, tdsLogin7()).Err)
	r = s5.Feed(1, ts, tdsPkt(4, 1, []byte{0xaa}))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionTDSEncryptTransition(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, tdsPrelogin(1)).Err)
	r := s.Feed(1, ts, tdsPreloginReply(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "tds->tls", r.Events[0].Session["Protocol Transition"])
	require.Equal(t, "tls", r.State)
}

func TestProtocolSessionTDSFragmentation(t *testing.T) {
	pre := tdsPrelogin(2)
	reply := tdsPreloginReply(2)
	login := tdsLogin7()
	done := tdsDone72()
	batch := tdsSQLBatch72("SELECT 1")
	result := tdsQueryResult72()
	steps := []sessionStep{
		{0, pre}, {1, reply}, {0, login}, {1, done}, {0, batch}, {1, result},
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
						names = append(names, fmt.Sprint(e.Session["Packet Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}

func TestProtocolSessionTDSMultiPacketBatch(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, tdsPrelogin(2)).Err)
	require.Nil(t, s.Feed(1, ts, tdsPreloginReply(2)).Err)
	require.Nil(t, s.Feed(0, ts, tdsLogin7()).Err)
	require.Nil(t, s.Feed(1, ts, tdsDone72()).Err)
	body := append(tdsAllHeaders(), tdsUCS2("SELECT 1")...)
	wire := tdsPkts(1, body, 7)
	require.Greater(t, bytesCountHeaders(wire), 1)
	r := s.Feed(0, ts, wire)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SQLBatch", r.Events[0].Session["Packet Name"])
	require.Equal(t, "SELECT 1", r.Events[0].Session["SQL Text"])
}

func bytesCountHeaders(wire []byte) int {
	n := 0
	for at := 0; at+8 <= len(wire); {
		size := int(binary.BigEndian.Uint16(wire[at+2 : at+4]))
		if size < 8 {
			return n
		}
		n++
		at += size
	}
	return n
}
