package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func nfsRM(last bool, payload []byte) []byte {
	n := uint32(len(payload))
	if last {
		n |= nfsRMLast
	}
	h := make([]byte, 4)
	binary.BigEndian.PutUint32(h, n)
	return append(h, payload...)
}

func nfsU32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func nfsU64(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

func nfsOpaqueBytes(data []byte) []byte {
	pad := (4 - len(data)%4) % 4
	return append(append(nfsU32(uint32(len(data))), data...), make([]byte, pad)...)
}

func nfsFH() []byte {
	return nfsOpaqueBytes([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	})
}

func nfsAuthNull() []byte { return make([]byte, 16) }

func nfsCall(xid, proc uint32, stub []byte) []byte {
	b := nfsU32(xid)
	b = append(b, nfsU32(0)...)
	b = append(b, nfsU32(2)...)
	b = append(b, nfsU32(nfsProgram)...)
	b = append(b, nfsU32(nfsVersion)...)
	b = append(b, nfsU32(proc)...)
	b = append(b, nfsAuthNull()...)
	return nfsRM(true, append(b, stub...))
}

func nfsReply(xid, status uint32, rest []byte) []byte {
	b := nfsU32(xid)
	b = append(b, nfsU32(1)...)
	b = append(b, nfsU32(0)...) // MSG_ACCEPTED
	b = append(b, make([]byte, 8)...) // verifier AUTH_NULL
	b = append(b, nfsU32(0)...) // SUCCESS
	b = append(b, nfsU32(status)...)
	return nfsRM(true, append(b, rest...))
}

func nfsLookupCall(xid uint32, name string) []byte {
	return nfsCall(xid, 3, append(nfsFH(), nfsOpaqueBytes([]byte(name))...))
}

func nfsLookupOK(xid uint32) []byte {
	rest := nfsFH()
	rest = append(rest, nfsU32(0)...) // post_op_attr false
	rest = append(rest, nfsU32(0)...)
	return nfsReply(xid, 0, rest)
}

func nfsGetattrCall(xid uint32) []byte {
	return nfsCall(xid, 1, nfsFH())
}

func nfsGetattrOK(xid uint32, size uint64) []byte {
	attr := make([]byte, 84)
	binary.BigEndian.PutUint32(attr[0:], 1) // NF3REG
	binary.BigEndian.PutUint64(attr[20:], size)
	return nfsReply(xid, 0, attr)
}

func nfsReadCall(xid uint32, off uint64, count uint32) []byte {
	stub := nfsFH()
	stub = append(stub, nfsU64(off)...)
	stub = append(stub, nfsU32(count)...)
	return nfsCall(xid, 6, stub)
}

func nfsReadOK(xid uint32, data []byte, eof bool) []byte {
	rest := nfsU32(0) // post_op_attr false
	rest = append(rest, nfsU32(uint32(len(data)))...)
	if eof {
		rest = append(rest, nfsU32(1)...)
	} else {
		rest = append(rest, nfsU32(0)...)
	}
	rest = append(rest, nfsOpaqueBytes(data)...)
	return nfsReply(xid, 0, rest)
}

func nfsWriteCall(xid uint32, off uint64, data []byte) []byte {
	stub := nfsFH()
	stub = append(stub, nfsU64(off)...)
	stub = append(stub, nfsU32(uint32(len(data)))...)
	stub = append(stub, nfsU32(0)...) // UNSTABLE
	stub = append(stub, nfsOpaqueBytes(data)...)
	return nfsCall(xid, 7, stub)
}

func nfsWriteOK(xid, count uint32) []byte {
	rest := nfsU32(0)                 // pre_op false
	rest = append(rest, nfsU32(0)...) // post_op false
	rest = append(rest, nfsU32(count)...)
	rest = append(rest, nfsU32(0)...) // UNSTABLE
	rest = append(rest, make([]byte, 8)...)
	return nfsReply(xid, 0, rest)
}

func TestProtocolSessionNFSLookupGetattrReadWrite(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	look := nfsLookupCall(1, "foo")
	p := s.Probe(look)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "nfs", p.Protocol)
	require.Equal(t, "v3", p.Version)

	r := s.Feed(0, ts, look)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LOOKUP", r.Events[0].Session["Packet Name"])
	require.Equal(t, "foo", r.Events[0].Session["Name"])
	require.Equal(t, uint32(1), r.Events[0].Session["XID"])

	r = s.Feed(1, ts, nfsLookupOK(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LOOKUP", r.Events[0].Session["Matched Request"])
	require.Equal(t, uint32(0), r.Events[0].Session["NFS Status"])
	require.Equal(t, 32, r.Events[0].Session["Object Handle Bytes"])

	r = s.Feed(0, ts, nfsGetattrCall(2))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GETATTR", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, nfsGetattrOK(2, 42))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GETATTR", r.Events[0].Session["Matched Request"])
	require.Equal(t, uint32(1), r.Events[0].Session["File Type"])
	require.Equal(t, uint64(42), r.Events[0].Session["Size"])

	r = s.Feed(0, ts, nfsReadCall(3, 0, 4))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint64(0), r.Events[0].Session["Offset"])
	require.Equal(t, uint32(4), r.Events[0].Session["Count"])
	r = s.Feed(1, ts, nfsReadOK(3, []byte("abcd"), true))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(4), r.Events[0].Session["Count"])
	require.Equal(t, true, r.Events[0].Session["EOF"])
	require.Equal(t, 4, r.Events[0].Session["Data Bytes"])

	r = s.Feed(0, ts, nfsWriteCall(4, 8, []byte("efgh")))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "WRITE", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(8), r.Events[0].Session["Offset"])
	r = s.Feed(1, ts, nfsWriteOK(4, 4))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(4), r.Events[0].Session["Count"])

	// Interleaved XIDs: replies need not arrive in call order.
	require.Nil(t, s.Feed(0, ts, nfsLookupCall(10, "a")).Err)
	require.Nil(t, s.Feed(0, ts, nfsLookupCall(11, "b")).Err)
	r = s.Feed(1, ts, nfsLookupOK(11))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(11), r.Events[0].Session["XID"])
	require.Equal(t, "LOOKUP", r.Events[0].Session["Matched Request"])
	r = s.Feed(1, ts, nfsLookupOK(10))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(10), r.Events[0].Session["XID"])

	cut := s.Feed(0, ts, look[:6])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionNFSRecordMarkAndFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	call := nfsLookupCall(1, "z")
	body := call[4:]
	first := nfsRM(false, body[:20])
	last := nfsRM(true, body[20:])
	r := s.Feed(0, ts, append(first, last...))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "LOOKUP", r.Events[0].Session["Packet Name"])
	require.Equal(t, "z", r.Events[0].Session["Name"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	portmap := nfsCall(1, 3, nil)
	binary.BigEndian.PutUint32(portmap[4+12:4+16], 100000)
	require.Equal(t, ProbeReject, s2.Probe(portmap).Verdict)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, nfsLookupCall(1, "x")).Err)
	r = s3.Feed(1, ts, nfsLookupOK(99))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Unmatched"])

	s4, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	short := nfsRM(true, make([]byte, 8))
	r = s4.Feed(0, ts, short)
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s5, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s5.Feed(0, ts, nfsLookupCall(1, "x")).Err)
	r = s5.Feed(1, ts, nfsReply(1, 2, nil)) // NOENT
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, uint32(2), r.Events[0].Session["NFS Status"])
	require.Equal(t, "NFS3ERR_NOENT", r.Events[0].Session["NFS Error"])
}

func TestProtocolSessionNFSFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, nfsLookupCall(1, "foo")},
		{1, nfsLookupOK(1)},
		{0, nfsGetattrCall(2)},
		{1, nfsGetattrOK(2, 42)},
		{0, nfsReadCall(3, 0, 4)},
		{1, nfsReadOK(3, []byte("abcd"), true)},
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
