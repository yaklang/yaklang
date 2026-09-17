package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb2TCP(payload []byte) []byte {
	h := []byte{0, byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
	return append(h, payload...)
}

func smb2Hdr(cmd uint16, flags uint32, mid, sid uint64, tid uint32) []byte {
	h := make([]byte, 64)
	binary.LittleEndian.PutUint32(h[0:], 0x424d53fe)
	binary.LittleEndian.PutUint16(h[4:], 64)
	binary.LittleEndian.PutUint16(h[12:], cmd)
	binary.LittleEndian.PutUint16(h[14:], 1)
	binary.LittleEndian.PutUint32(h[16:], flags)
	binary.LittleEndian.PutUint64(h[24:], mid)
	binary.LittleEndian.PutUint32(h[36:], tid)
	binary.LittleEndian.PutUint64(h[40:], sid)
	return h
}

func smb2UTF16(s string) []byte {
	out := make([]byte, len(s)*2)
	for i := 0; i < len(s); i++ {
		out[i*2] = s[i]
	}
	return out
}

func smb2NegotiateReq(dialects ...uint16) []byte {
	body := make([]byte, 36+2*len(dialects))
	binary.LittleEndian.PutUint16(body[0:], 36)
	binary.LittleEndian.PutUint16(body[2:], uint16(len(dialects)))
	binary.LittleEndian.PutUint16(body[4:], 1)
	copy(body[12:28], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00})
	for i, d := range dialects {
		binary.LittleEndian.PutUint16(body[36+2*i:], d)
	}
	return smb2TCP(append(smb2Hdr(0, 0, 1, 0, 0), body...))
}

func smb2NegotiateResp(dialect uint16) []byte {
	body := make([]byte, 64)
	binary.LittleEndian.PutUint16(body[0:], 65)
	binary.LittleEndian.PutUint16(body[2:], 1)
	binary.LittleEndian.PutUint16(body[4:], dialect)
	copy(body[8:24], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	binary.LittleEndian.PutUint32(body[28:], 0x00800000)
	binary.LittleEndian.PutUint32(body[32:], 0x00800000)
	binary.LittleEndian.PutUint32(body[36:], 0x00800000)
	binary.LittleEndian.PutUint16(body[56:], 128)
	return smb2TCP(append(smb2Hdr(0, 1, 1, 0, 0), body...))
}

func smb2SessionSetup(mid uint64, resp bool) []byte {
	flags := uint32(0)
	var body []byte
	if resp {
		flags = 1
		body = make([]byte, 8)
		binary.LittleEndian.PutUint16(body[0:], 9)
	} else {
		body = make([]byte, 24)
		binary.LittleEndian.PutUint16(body[0:], 25)
	}
	sid := uint64(0)
	if resp {
		sid = 0x11
	}
	return smb2TCP(append(smb2Hdr(1, flags, mid, sid, 0), body...))
}

func smb2TreeConnect(mid, sid uint64, path string) []byte {
	p := smb2UTF16(path)
	body := make([]byte, 8+len(p))
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[4:], 72)
	binary.LittleEndian.PutUint16(body[6:], uint16(len(p)))
	copy(body[8:], p)
	return smb2TCP(append(smb2Hdr(3, 0, mid, sid, 0), body...))
}

func smb2TreeConnectResp(mid, sid uint64, tid uint32) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint16(body[0:], 16)
	body[2] = 1
	return smb2TCP(append(smb2Hdr(3, 1, mid, sid, tid), body...))
}

func smb2Create(mid, sid uint64, tid uint32, name string) []byte {
	n := smb2UTF16(name)
	body := make([]byte, 56+len(n))
	binary.LittleEndian.PutUint16(body[0:], 57)
	binary.LittleEndian.PutUint32(body[4:], 2)
	binary.LittleEndian.PutUint32(body[24:], 0x00120189)
	binary.LittleEndian.PutUint32(body[36:], 1)
	binary.LittleEndian.PutUint16(body[44:], 120)
	binary.LittleEndian.PutUint16(body[46:], uint16(len(n)))
	copy(body[56:], n)
	return smb2TCP(append(smb2Hdr(5, 0, mid, sid, tid), body...))
}

func smb2CreateResp(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 88)
	binary.LittleEndian.PutUint16(body[0:], 89)
	binary.LittleEndian.PutUint32(body[4:], 1)
	copy(body[64:80], fid)
	return smb2TCP(append(smb2Hdr(5, 1, mid, sid, tid), body...))
}

func smb2Read(mid, sid uint64, tid uint32, fid []byte, n uint32) []byte {
	body := make([]byte, 48)
	binary.LittleEndian.PutUint16(body[0:], 49)
	binary.LittleEndian.PutUint32(body[4:], n)
	copy(body[16:32], fid)
	return smb2TCP(append(smb2Hdr(8, 0, mid, sid, tid), body...))
}

func smb2ReadResp(mid, sid uint64, tid uint32, data []byte) []byte {
	body := make([]byte, 16+len(data))
	binary.LittleEndian.PutUint16(body[0:], 17)
	body[2] = 80
	binary.LittleEndian.PutUint32(body[4:], uint32(len(data)))
	copy(body[16:], data)
	return smb2TCP(append(smb2Hdr(8, 1, mid, sid, tid), body...))
}

func smb2Write(mid, sid uint64, tid uint32, fid, data []byte) []byte {
	body := make([]byte, 48+len(data))
	binary.LittleEndian.PutUint16(body[0:], 49)
	binary.LittleEndian.PutUint16(body[2:], 112)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(data)))
	copy(body[16:32], fid)
	copy(body[48:], data)
	return smb2TCP(append(smb2Hdr(9, 0, mid, sid, tid), body...))
}

func smb2WriteResp(mid, sid uint64, tid uint32, count uint32) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint16(body[0:], 17)
	binary.LittleEndian.PutUint32(body[4:], count)
	return smb2TCP(append(smb2Hdr(9, 1, mid, sid, tid), body...))
}

func smb2Close(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 24)
	copy(body[8:24], fid)
	return smb2TCP(append(smb2Hdr(6, 0, mid, sid, tid), body...))
}

func smb2CloseResp(mid, sid uint64, tid uint32, fid []byte) []byte {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 24)
	copy(body[8:24], fid)
	return smb2TCP(append(smb2Hdr(6, 1, mid, sid, tid), body...))
}

func smb2Transform() []byte {
	xf := make([]byte, 52)
	binary.LittleEndian.PutUint32(xf[0:], 0x424d53fd)
	binary.LittleEndian.PutUint32(xf[36:], 64)
	binary.LittleEndian.PutUint16(xf[42:], 1)
	binary.LittleEndian.PutUint64(xf[44:], 0x11)
	return smb2TCP(xf)
}

func TestProtocolSessionSMB2NegotiateCreateIO(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	neg := smb2NegotiateReq(0x0202, 0x0210, 0x0300, 0x0311)
	p := s.Probe(neg)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "smb2", p.Protocol)

	r := s.Feed(0, ts, neg)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "NEGOTIATE", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(1), r.Events[0].Session["Message ID"])
	require.Equal(t, true, r.Events[0].Session["Outstanding"])

	r = s.Feed(1, ts, smb2NegotiateResp(0x0311))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "NEGOTIATE", r.Events[0].Session["Matched Request"])
	require.Equal(t, uint16(0x0311), r.Events[0].Session["Dialect"])
	require.Equal(t, "3.1.1", r.Events[0].Session["Version Name"])

	r = s.Feed(0, ts, smb2SessionSetup(2, false))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SESSION_SETUP", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, smb2SessionSetup(2, true))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SESSION_SETUP", r.Events[0].Session["Matched Request"])
	require.Equal(t, uint64(0x11), r.Events[0].Session["Session ID"])

	r = s.Feed(0, ts, smb2TreeConnect(3, 0x11, `\\srv\share`))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, `\\srv\share`, r.Events[0].Session["Path"])
	r = s.Feed(1, ts, smb2TreeConnectResp(3, 0x11, 1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "TREE_CONNECT", r.Events[0].Session["Matched Request"])
	require.Equal(t, uint32(1), r.Events[0].Session["Tree ID"])

	fid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	r = s.Feed(0, ts, smb2Create(4, 0x11, 1, "file.txt"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CREATE", r.Events[0].Session["Packet Name"])
	require.Equal(t, "file.txt", r.Events[0].Session["File Name"])
	r = s.Feed(1, ts, smb2CreateResp(4, 0x11, 1, fid))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, hex.EncodeToString(fid), r.Events[0].Session["FileId"])

	r = s.Feed(0, ts, smb2Read(5, 0x11, 1, fid, 8))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "READ", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, smb2ReadResp(5, 0x11, 1, []byte("abcdefgh")))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "READ", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, smb2Write(6, 0x11, 1, fid, []byte("abcdefgh")))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "WRITE", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, smb2WriteResp(6, 0x11, 1, 8))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(8), r.Events[0].Session["Count"])

	r = s.Feed(0, ts, smb2Close(7, 0x11, 1, fid))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CLOSE", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, smb2CloseResp(7, 0x11, 1, fid))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CLOSE", r.Events[0].Session["Matched Request"])

	cut := s.Feed(0, ts, smb2NegotiateReq(0x0311)[:5])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionSMB2CompoundAndTransform(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, smb2NegotiateReq(0x0311)).Err)
	require.Nil(t, s.Feed(1, ts, smb2NegotiateResp(0x0311)).Err)

	fid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	create := smb2Create(10, 0x11, 1, "a.txt")
	closeP := smb2Close(11, 0x11, 1, fid)
	cbody := create[4:]
	for len(cbody)%8 != 0 {
		cbody = append(cbody, 0)
	}
	kbody := closeP[4:]
	binary.LittleEndian.PutUint32(cbody[20:24], uint32(len(cbody)))
	binary.LittleEndian.PutUint32(kbody[16:20], 4) // RELATED
	compound := smb2TCP(append(cbody, kbody...))
	r := s.Feed(0, ts, compound)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Compound"])
	require.Equal(t, 2, r.Events[0].Session["Compound Count"])
	require.Equal(t, []string{"CREATE", "CLOSE"}, r.Events[0].Session["Command Names"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s2.Feed(0, ts, smb2Transform())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "transform", r.Events[0].Session["Packet Name"])
}

func TestProtocolSessionSMB2FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	smb1 := smb2TCP(append([]byte{0xff, 'S', 'M', 'B'}, make([]byte, 28)...))
	require.Equal(t, ProbeReject, s.Probe(smb1).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{0, 0, 0}).Verdict)

	r := s.Feed(0, ts, smb1)
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s2.Feed(1, ts, smb2NegotiateResp(0x0311))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", r.Events[0].Session["Association Status"])

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, smb2NegotiateReq(0x0311)).Err)
	payload := make([]byte, 64)
	for i := range payload {
		payload[i] = 0x11
	}
	r = s3.Feed(0, ts, smb2TCP(payload))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s4.Feed(0, ts, smb2NegotiateReq(0x0311)[:6])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionSMB2Fragmentation(t *testing.T) {
	fid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	steps := []sessionStep{
		{0, smb2NegotiateReq(0x0311)},
		{1, smb2NegotiateResp(0x0311)},
		{0, smb2SessionSetup(2, false)},
		{1, smb2SessionSetup(2, true)},
		{0, smb2TreeConnect(3, 0x11, `\\srv\share`)},
		{1, smb2TreeConnectResp(3, 0x11, 1)},
		{0, smb2Create(4, 0x11, 1, "file.txt")},
		{1, smb2CreateResp(4, 0x11, 1, fid)},
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
