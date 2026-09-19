package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func iec104U(ctrl byte) []byte {
	return []byte{0x68, 4, ctrl, 0, 0, 0}
}

func iec104I(send, recv uint16, typeID byte, cot uint16, ioa uint32, info []byte) []byte {
	asdu := []byte{typeID, 1, byte(cot), byte(cot >> 8), 1, 0, byte(ioa), byte(ioa >> 8), byte(ioa >> 16)}
	asdu = append(asdu, info...)
	ctrl := make([]byte, 4)
	binary.LittleEndian.PutUint16(ctrl[0:], send<<1)
	binary.LittleEndian.PutUint16(ctrl[2:], recv<<1)
	payload := append(ctrl, asdu...)
	return append([]byte{0x68, byte(len(payload))}, payload...)
}

func TestProtocolSessionIEC104StartDTAndInterrogation(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	start := iec104U(0x07)
	p := s.Probe(start)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "iec104", p.Protocol)
	r := s.Feed(0, ts, start)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "STARTDT ACT", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, iec104U(0x0b))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "STARTDT CON", r.Events[0].Session["Packet Name"])
	require.Equal(t, "STARTDT ACT", r.Events[0].Session["In Reply To"])
	r = s.Feed(0, ts, iec104I(0, 0, 100, 6, 0, []byte{20}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "C_IC_NA_1", r.Events[0].Session["Packet Name"])
	require.Equal(t, 100, r.Events[0].Session["TypeID"])
	require.Equal(t, 6, r.Events[0].Session["COT"])
	require.Equal(t, 0, r.Events[0].Session["IOA"])
}

func TestProtocolSessionIEC104FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeReject, s.Probe([]byte{0x68}).Verdict)
	require.NotEqual(t, "iec104", s.Probe([]byte{0x67, 4, 7, 0, 0, 0}).Protocol)
	require.Nil(t, s.Feed(0, ts, iec104U(0x07)).Err)
	r := s.Feed(0, ts, []byte{0x68, 2, 0, 0})
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionIEC104Fragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, iec104U(0x07)},
		{1, iec104U(0x0b)},
		{0, iec104I(0, 0, 100, 6, 0, []byte{20})},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func TestDNP3CRC123456789(t *testing.T) {
	require.Equal(t, uint16(0xEA82), dnp3CRC([]byte("123456789")))
}

func dnp3Link(ctrl byte, dest, src uint16, user []byte) []byte {
	hdr := make([]byte, 8)
	hdr[0], hdr[1], hdr[2], hdr[3] = 0x05, 0x64, byte(5+len(user)), ctrl
	binary.LittleEndian.PutUint16(hdr[4:], dest)
	binary.LittleEndian.PutUint16(hdr[6:], src)
	crc := dnp3CRC(hdr)
	out := append(append([]byte{}, hdr...), byte(crc), byte(crc>>8))
	for len(user) > 0 {
		n := min(len(user), 16)
		block := user[:n]
		user = user[n:]
		c := dnp3CRC(block)
		out = append(out, block...)
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func TestProtocolSessionDNP3ReadResponse(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	req := dnp3Link(0xC4, 1, 2, []byte{0xC0, 0xC0, 0x01})
	p := s.Probe(req)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "dnp3", p.Protocol)
	r := s.Feed(0, ts, req)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "READ", r.Events[0].Session["Packet Name"])
	require.Equal(t, 0xC0, r.Events[0].Session["Transport Header"])
	require.Equal(t, 0xC0, r.Events[0].Session["Application Control"])
	require.Equal(t, 1, r.Events[0].Session["Destination"])
	require.Equal(t, 2, r.Events[0].Session["Source"])
	r = s.Feed(1, ts, dnp3Link(0x44, 2, 1, []byte{0xC0, 0xC0, 0x81, 0, 0}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RESPONSE", r.Events[0].Session["Packet Name"])
	require.Equal(t, 0, r.Events[0].Session["IIN1"])
	require.Equal(t, "READ", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionDNP3FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "dnp3", s.Probe([]byte{0x05}).Protocol)
	require.NotEqual(t, "dnp3", s.Probe([]byte{0x05, 0x63, 5, 0xC4, 1, 0, 2, 0, 0, 0}).Protocol)
	trunc := dnp3Link(0xC4, 1, 2, []byte{0xC0, 0x01})
	r := s.Feed(0, ts, trunc)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
	if len(r.Events) > 0 && r.Events[0].Session != nil {
		require.NotEqual(t, "READ", r.Events[0].Session["Packet Name"])
	}
	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, dnp3Link(0xC4, 1, 2, []byte{0xC0, 0xC0, 0x01})).Err)
	bad := dnp3Link(0xC4, 1, 2, []byte{0xC0, 0xC0, 0x01})
	bad[8] ^= 0xff
	r = s2.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionDNP3Fragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, dnp3Link(0xC4, 1, 2, []byte{0xC0, 0xC0, 0x01})},
		{1, dnp3Link(0x44, 2, 1, []byte{0xC0, 0xC0, 0x81, 0, 0})},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func c37frame(kind, ver byte, id uint16, body []byte) []byte {
	w := make([]byte, 14+len(body)+2)
	w[0] = 0xaa
	w[1] = kind<<4 | ver
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	binary.BigEndian.PutUint16(w[4:], id)
	copy(w[14:], body)
	crc := c37118CRC(w[:len(w)-2])
	binary.BigEndian.PutUint16(w[len(w)-2:], crc)
	return w
}

func TestProtocolSessionC37118CommandAndDataWithoutCFG(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	cmd := c37frame(4, 1, 7, []byte{0, 2})
	p := s.Probe(cmd)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "c37118", p.Protocol)
	r := s.Feed(0, ts, cmd)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CMD", r.Events[0].Session["Packet Name"])
	require.Equal(t, 2, r.Events[0].Session["Command"])
	data := c37frame(0, 1, 7, []byte{0, 0, 0, 0, 0, 0})
	r = s.Feed(1, ts, data)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	require.Equal(t, "DATA", r.Events[0].Session["Packet Name"])
	require.Equal(t, true, r.Events[0].Session["Configuration Required"])
}

func TestProtocolSessionC37118CFGThenData(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	cfg := c37frame(3, 1, 7, []byte{0, 0, 0, 1, 0, 0})
	r := s.Feed(1, ts, cfg)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CFG-2", r.Events[0].Session["Packet Name"])
	r = s.Feed(0, ts, c37frame(0, 1, 7, []byte{0, 0, 0, 0, 0, 0}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "DATA", r.Events[0].Session["Packet Name"])
	_, required := r.Events[0].Session["Configuration Required"]
	require.False(t, required)
}

func TestProtocolSessionC37118FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "c37118", s.Probe([]byte{0xaa}).Protocol)
	require.NotEqual(t, "c37118", s.Probe([]byte{0xab, 0x10, 0, 16}).Protocol)
	bad := c37frame(4, 1, 7, []byte{0, 2})
	bad[len(bad)-1] ^= 0xff
	r := s.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionC37118Fragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, c37frame(4, 1, 7, []byte{0, 2})},
		{1, c37frame(3, 1, 7, []byte{0, 0, 0, 1, 0, 0})},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func berPut(tag byte, val []byte) []byte {
	return append([]byte{tag, byte(len(val))}, val...)
}

func goosePDU() []byte {
	all := berPut(0x83, []byte{1})
	inner := append([]byte{}, berPut(0x80, []byte("cbRef"))...)
	inner = append(inner, berPut(0x81, []byte{0x27, 0x10})...)
	inner = append(inner, berPut(0x82, []byte("dataset1"))...)
	inner = append(inner, berPut(0x83, []byte("goid1"))...)
	inner = append(inner, berPut(0x84, make([]byte, 8))...)
	inner = append(inner, berPut(0x85, []byte{2})...)
	inner = append(inner, berPut(0x86, []byte{3})...)
	inner = append(inner, berPut(0x87, []byte{0})...)
	inner = append(inner, berPut(0x88, []byte{1})...)
	inner = append(inner, berPut(0x89, []byte{0})...)
	inner = append(inner, berPut(0x8a, []byte{1})...)
	inner = append(inner, berPut(0xab, all)...)
	pdu := berPut(0x61, inner)
	n := 8 + len(pdu)
	w := make([]byte, n)
	binary.BigEndian.PutUint16(w[0:], 0x1234)
	binary.BigEndian.PutUint16(w[2:], uint16(n))
	copy(w[8:], pdu)
	return w
}

func TestProtocolSessionGOOSEDatasetAndState(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	g := goosePDU()
	p := s.Probe(g)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "goose", p.Protocol)
	r := s.Feed(0, ts, g)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "GOOSE", r.Events[0].Session["Packet Name"])
	require.Equal(t, 0x1234, r.Events[0].Session["APPID"])
	require.Equal(t, "dataset1", r.Events[0].Session["Dataset"])
	require.Equal(t, 2, r.Events[0].Session["State Number"])
	require.Equal(t, 3, r.Events[0].Session["Sequence Number"])
	require.Equal(t, []bool{true}, r.Events[0].Session["Boolean"])
}

func TestProtocolSessionGOOSEFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "goose", s.Probe([]byte{0xff, 0x00, 0x01}).Protocol)
	require.Nil(t, s.Feed(0, ts, goosePDU()).Err)
	bad := goosePDU()
	bad[8] = 0x60
	r := s.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}
