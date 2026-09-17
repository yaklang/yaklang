package pcaputil

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dcerpcHdr(ptype, flags byte, frag uint16, call uint32) []byte {
	h := make([]byte, 16)
	h[0] = 5
	h[2] = ptype
	h[3] = flags
	h[4] = 0x10
	binary.LittleEndian.PutUint16(h[8:], frag)
	binary.LittleEndian.PutUint32(h[12:], call)
	return h
}

func dcerpcPDU(ptype, flags byte, call uint32, body []byte) []byte {
	return append(dcerpcHdr(ptype, flags, uint16(16+len(body)), call), body...)
}

func dcerpcBind(uuid []byte, ctx uint16, call uint32) []byte {
	body := make([]byte, 12+44)
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	body[8] = 1
	binary.LittleEndian.PutUint16(body[12:], ctx)
	body[14] = 1
	copy(body[16:32], uuid)
	binary.LittleEndian.PutUint32(body[32:], 3)
	copy(body[36:52], dcerpcNDR)
	binary.LittleEndian.PutUint32(body[52:], 2)
	return dcerpcPDU(11, 0x03, call, body)
}

func dcerpcBindAck(call uint32) []byte {
	body := make([]byte, 10)
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	return dcerpcPDU(12, 0x03, call, body)
}

func dcerpcRequest(call uint32, ctx, op uint16, stub []byte) []byte {
	body := make([]byte, 8+len(stub))
	binary.LittleEndian.PutUint16(body[4:], ctx)
	binary.LittleEndian.PutUint16(body[6:], op)
	copy(body[8:], stub)
	return dcerpcPDU(0, 0x03, call, body)
}

func dcerpcResponse(call uint32, ctx uint16, stub []byte) []byte {
	body := make([]byte, 8+len(stub))
	binary.LittleEndian.PutUint16(body[4:], ctx)
	copy(body[8:], stub)
	return dcerpcPDU(2, 0x03, call, body)
}

func dcerpcFault(call uint32, status uint32) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body[8:], status)
	return dcerpcPDU(3, 0x03, call, body)
}

func TestProtocolSessionDCERPCBindEPMAndSRVSVC(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bind := dcerpcBind(dcerpcEPM, 0, 1)
	p := s.Probe(bind)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "dcerpc", p.Protocol)
	require.Equal(t, "5.0", p.Version)

	r := s.Feed(0, ts, bind)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Bind", r.Events[0].Session["Packet Name"])
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])
	require.Equal(t, uint32(1), r.Events[0].Session["Call ID"])
	require.Equal(t, true, r.Events[0].Session["Outstanding"])

	r = s.Feed(1, ts, dcerpcBindAck(1))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "BindAck", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Bind", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(3), r.Events[0].Session["OpNum"])
	require.Equal(t, "EPM", r.Events[0].Session["Interface"])

	r = s.Feed(1, ts, dcerpcResponse(2, 0, []byte{9, 9}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Response", r.Events[0].Session["Packet Name"])
	require.Equal(t, "EPM.ept_map", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, dcerpcBind(dcerpcSRVSVC, 1, 3))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC", r.Events[0].Session["Interface"])
	require.Nil(t, s.Feed(1, ts, dcerpcBindAck(3)).Err)

	r = s.Feed(0, ts, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint16(15), r.Events[0].Session["OpNum"])
	r = s.Feed(1, ts, dcerpcResponse(4, 1, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, dcerpcFault(5, 0x00000005))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Fault", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint32(5), r.Events[0].Session["Status"])
	require.Equal(t, true, r.Events[0].Session["Unmatched"])

	first := dcerpcPDU(0, dcerpcFirst, 6, append([]byte{0, 0, 0, 0, 1, 0, 15, 0}, []byte{0xaa, 0xbb}...))
	last := dcerpcPDU(0, dcerpcLast, 6, []byte{0xcc, 0xdd})
	r = s.Feed(0, ts, first)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Fragment"])
	r = s.Feed(0, ts, last)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Reassembled"])
	require.Equal(t, "SRVSVC.NetrShareEnum", r.Events[0].Session["Packet Name"])
	require.Equal(t, 4, r.Events[0].Session["Stub Bytes"])

	cut := s.Feed(0, ts, bind[:8])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)
}

func TestProtocolSessionDCERPCFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeReject, s.Probe([]byte{4, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 1, 0, 0, 0}).Verdict)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{5, 0, 11}).Verdict)
	require.Equal(t, ProbeReject, s.Probe(dcerpcHdr(30, 3, 16, 1)).Verdict)

	r := s.Feed(0, ts, []byte{4, 0, 0, 0})
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Status != "decoded")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)).Err)
	short := dcerpcHdr(0, 3, 10, 2)
	r = s2.Feed(0, ts, short)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, dcerpcBind(dcerpcEPM, 0, 1)[:10])
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionDCERPCFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, dcerpcBind(dcerpcEPM, 0, 1)},
		{1, dcerpcBindAck(1)},
		{0, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4})},
		{1, dcerpcResponse(2, 0, []byte{9, 9})},
		{0, dcerpcBind(dcerpcSRVSVC, 1, 3)},
		{1, dcerpcBindAck(3)},
		{0, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0})},
		{1, dcerpcResponse(4, 1, nil)},
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
