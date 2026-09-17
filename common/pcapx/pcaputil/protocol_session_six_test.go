package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tnsConnect(data string) []byte {
	pkt := make([]byte, 8+26+len(data))
	binary.BigEndian.PutUint16(pkt[0:], uint16(len(pkt)))
	pkt[4] = 1
	binary.BigEndian.PutUint16(pkt[8:], 0x0134)
	binary.BigEndian.PutUint16(pkt[10:], 0x0134)
	binary.BigEndian.PutUint16(pkt[24:], uint16(len(data)))
	binary.BigEndian.PutUint16(pkt[26:], 34)
	copy(pkt[34:], data)
	return pkt
}

func tnsType(typ byte, extra []byte) []byte {
	pkt := make([]byte, 8+len(extra))
	binary.BigEndian.PutUint16(pkt[0:], uint16(len(pkt)))
	pkt[4] = typ
	copy(pkt[8:], extra)
	return pkt
}

func TestProtocolSessionTNSConnectAcceptData(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	conn := tnsConnect("(DESCRIPTION=)")
	p := s.Probe(conn)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "tns", p.Protocol)
	r := s.Feed(0, ts, conn)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Connect", r.Events[0].Session["Packet Name"])
	require.Equal(t, 0x0134, r.Events[0].Session["Version"])
	require.Equal(t, "(DESCRIPTION=)", r.Events[0].Session["Connect Data"])
	r = s.Feed(1, ts, tnsType(2, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Accept", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Connect", r.Events[0].Session["In Reply To"])
	r = s.Feed(0, ts, tnsType(6, []byte{0, 0, 'A', 'B'}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Data", r.Events[0].Session["Packet Name"])
}

func TestProtocolSessionTNSRefuseAndFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, tnsConnect("(DESCRIPTION=)")).Err)
	r := s.Feed(1, ts, tnsType(4, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Refuse", r.Events[0].Session["Packet Name"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.NotEqual(t, "tns", s2.Probe([]byte{0xff, 0x00, 0x01}).Protocol)
	require.Nil(t, s2.Feed(0, ts, tnsConnect("(DESCRIPTION=)")).Err)
	bad := tnsType(99, nil)
	r = s2.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionTNSFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, tnsConnect("(DESCRIPTION=)")},
		{1, tnsType(2, nil)},
		{0, tnsType(6, []byte{0, 0, 'A', 'B'})},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func radiusPkt(code, id byte, attrs []byte) []byte {
	pkt := make([]byte, 20+len(attrs))
	pkt[0], pkt[1] = code, id
	binary.BigEndian.PutUint16(pkt[2:], uint16(len(pkt)))
	copy(pkt[4:], []byte("0123456789abcdef"))
	copy(pkt[20:], attrs)
	return pkt
}

func radiusAttr(typ byte, val []byte) []byte {
	return append([]byte{typ, byte(2 + len(val))}, val...)
}

func TestProtocolSessionRADIUSAccessPairing(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	req := radiusPkt(1, 7, append(radiusAttr(1, []byte("alice")), radiusAttr(2, make([]byte, 16))...))
	p := s.Probe(req)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "radius", p.Protocol)
	r := s.Feed(0, ts, req)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Access-Request", r.Events[0].Session["Packet Name"])
	require.Equal(t, "alice", r.Events[0].Session["User-Name"])
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "opaque", r.Events[0].Session["User-Password"])
	r = s.Feed(1, ts, radiusPkt(2, 7, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Access-Accept", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Access-Request", r.Events[0].Session["In Reply To"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, radiusPkt(1, 9, radiusAttr(1, []byte("bob")))).Err)
	r = s2.Feed(1, ts, radiusPkt(3, 9, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Access-Reject", r.Events[0].Session["Packet Name"])
}

func TestProtocolSessionRADIUSFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "radius", s.Probe([]byte{0xff, 0x00, 0x01}).Protocol)
	r := s.Feed(0, ts, radiusPkt(1, 1, []byte{1, 8, 'a'}))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func dhcpMsg(op, msgType byte, xid uint32, chaddr []byte) []byte {
	w := make([]byte, 240+5)
	w[0], w[1], w[2] = op, 1, 6
	binary.BigEndian.PutUint32(w[4:], xid)
	copy(w[28:], chaddr)
	binary.BigEndian.PutUint32(w[236:], dhcpCookie)
	w[240], w[241], w[242] = 53, 1, msgType
	w[243], w[244] = 255, 0
	return w[:244]
}

func TestProtocolSessionDHCPDiscoverOfferAck(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	ch := []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}
	disc := dhcpMsg(1, 1, 0x12345678, ch)
	p := s.Probe(disc)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "dhcp", p.Protocol)
	r := s.Feed(0, ts, disc)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Discover", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint32(0x12345678), r.Events[0].Session["Xid"])
	require.Equal(t, ch, r.Events[0].Session["CHADDR"])
	r = s.Feed(1, ts, dhcpMsg(2, 2, 0x12345678, ch))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Offer", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Discover", r.Events[0].Session["In Reply To"])
	r = s.Feed(0, ts, dhcpMsg(1, 3, 0x12345678, ch))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Request", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, dhcpMsg(2, 5, 0x12345678, ch))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Ack", r.Events[0].Session["Packet Name"])
	require.Equal(t, "Request", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionDHCPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "dhcp", s.Probe([]byte{0xff, 0x00, 0x01}).Protocol)
	bad := dhcpMsg(1, 1, 1, []byte{1, 2, 3, 4, 5, 6})
	binary.BigEndian.PutUint32(bad[236:], 0xdeadbeef)
	r := s.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func ntpPkt(mode, stratum byte, origin, recv, xmit []byte) []byte {
	w := make([]byte, 48)
	w[0] = 4<<3 | mode
	w[1] = stratum
	copy(w[24:32], origin)
	copy(w[32:40], recv)
	copy(w[40:48], xmit)
	return w
}

func TestProtocolSessionNTPClientServer(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	xmit := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	cli := ntpPkt(3, 0, make([]byte, 8), make([]byte, 8), xmit)
	p := s.Probe(cli)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "ntp", p.Protocol)
	r := s.Feed(0, ts, cli)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Client", r.Events[0].Session["Packet Name"])
	require.Equal(t, 4, r.Events[0].Session["Version"])
	require.Equal(t, 3, r.Events[0].Session["Mode"])
	srv := ntpPkt(4, 2, xmit, []byte{9, 9, 9, 9, 9, 9, 9, 9}, []byte{8, 8, 8, 8, 8, 8, 8, 8})
	r = s.Feed(1, ts, srv)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Server", r.Events[0].Session["Packet Name"])
	require.Equal(t, 2, r.Events[0].Session["Stratum"])
	require.Equal(t, "Client", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionNTPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bad := make([]byte, 48)
	bad[0] = 3 << 3 // version 3
	require.NotEqual(t, "ntp", s.Probe(bad).Protocol)
	r := s.Feed(0, ts, bad[:10])
	require.True(t, r.Err != nil || r.State == "undetected" || r.NeedMore)
}

func coapMsg(typ, tkl, code byte, mid uint16, token []byte, opts, payload []byte) []byte {
	w := []byte{(1 << 6) | (typ << 4) | tkl, code, 0, 0}
	binary.BigEndian.PutUint16(w[2:], mid)
	w = append(w, token...)
	w = append(w, opts...)
	if payload != nil {
		w = append(w, 0xff)
		w = append(w, payload...)
	}
	return w
}

func TestProtocolSessionCoAPGetContentAndError(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	get := coapMsg(0, 1, 1, 7, []byte{0xab}, []byte{0xb1, 's'}, nil)
	p := s.Probe(get)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "coap", p.Protocol)
	r := s.Feed(0, ts, get)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CON", r.Events[0].Session["Packet Name"])
	require.Equal(t, "GET", r.Events[0].Session["Code"])
	require.Equal(t, "s", r.Events[0].Session["Uri-Path"])
	require.Equal(t, []byte{0xab}, r.Events[0].Session["Token"])
	ack := coapMsg(2, 1, 69, 7, []byte{0xab}, nil, []byte("ok"))
	r = s.Feed(1, ts, ack)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "ACK", r.Events[0].Session["Packet Name"])
	require.Equal(t, "2.05 Content", r.Events[0].Session["Code"])
	require.Equal(t, "GET", r.Events[0].Session["In Reply To"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, coapMsg(0, 1, 1, 8, []byte{1}, []byte{0xb1, 'x'}, nil)).Err)
	r = s2.Feed(1, ts, coapMsg(2, 1, 132, 8, []byte{1}, nil, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "4.04 Not Found", r.Events[0].Session["Code"])

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s3.Feed(0, ts, coapMsg(0, 0, 1, 9, nil, nil, nil)).Err)
	r = s3.Feed(1, ts, coapMsg(3, 0, 0, 9, nil, nil, nil))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "RST", r.Events[0].Session["Packet Name"])
}

func TestProtocolSessionCoAPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bad := []byte{0x80, 1, 0, 1} // version 2
	require.NotEqual(t, "coap", s.Probe(bad).Protocol)
	r := s.Feed(0, ts, []byte{0x40})
	require.True(t, r.NeedMore || r.Err != nil || r.State == "undetected")
}

func mbap(tid uint16, unit, fc byte, data []byte) []byte {
	pdu := append([]byte{unit, fc}, data...)
	w := make([]byte, 6+len(pdu))
	binary.BigEndian.PutUint16(w[0:], tid)
	binary.BigEndian.PutUint16(w[4:], uint16(len(pdu)))
	copy(w[6:], pdu)
	return w
}

func TestProtocolSessionModbusReadWriteException(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	req := mbap(1, 1, 3, []byte{0, 0, 0, 2})
	p := s.Probe(req)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "modbus", p.Protocol)
	r := s.Feed(0, ts, req)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Read Holding Registers", r.Events[0].Session["Packet Name"])
	require.Equal(t, 1, r.Events[0].Session["Transaction ID"])
	r = s.Feed(1, ts, mbap(1, 1, 3, []byte{4, 0, 1, 0, 2}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "response", r.Events[0].Session["Role"])
	require.Equal(t, "Read Holding Registers", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mbap(2, 1, 6, []byte{0, 1, 0, 5}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Write Single Register", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, mbap(2, 1, 6, []byte{0, 1, 0, 5}))
	require.Nil(t, r.Err, "%v", r.Err)

	r = s.Feed(0, ts, mbap(3, 1, 1, []byte{0, 0, 0, 8}))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mbap(3, 1, 0x81, []byte{2}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Exception"])
	require.Equal(t, 2, r.Events[0].Session["Exception Code"])
	require.Equal(t, "Read Coils", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionModbusHighTransactionID(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	req := mbap(0x8001, 1, 3, []byte{0, 0, 0, 2})
	p := s.Probe(req)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "modbus", p.Protocol)
	r := s.Feed(0, ts, req)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Read Holding Registers", r.Events[0].Session["Packet Name"])
	require.Equal(t, 0x8001, r.Events[0].Session["Transaction ID"])
	r = s.Feed(1, ts, mbap(0x8001, 1, 3, []byte{4, 0, 1, 0, 2}))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "response", r.Events[0].Session["Role"])
	require.Equal(t, "Read Holding Registers", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionModbusFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bad := mbap(1, 1, 3, []byte{0, 0, 0, 2})
	binary.BigEndian.PutUint16(bad[2:], 1)
	require.NotEqual(t, "modbus", s.Probe(bad).Protocol)
	r := s.Feed(0, ts, bad)
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Protocol != "modbus")
}

func TestProtocolSessionModbusFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, mbap(1, 1, 3, []byte{0, 0, 0, 2})},
		{1, mbap(1, 1, 3, []byte{4, 0, 1, 0, 2})},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}
