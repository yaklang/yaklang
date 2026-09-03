package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func parseMustFail(t *testing.T, data []byte, rule string, keys ...string) {
	t.Helper()
	_, err := parser.ParseBinary(bytes.NewReader(data), rule, keys...)
	require.Error(t, err, "expected parse failure for %s", rule)
}

func ntlmsspMessage(msgType uint32, body []byte) []byte {
	buf := make([]byte, 12+len(body))
	copy(buf[:8], []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(buf[8:], msgType)
	copy(buf[12:], body)
	return buf
}

func ntlmsspChallenge(challenge []byte) []byte {
	buf := make([]byte, 48)
	copy(buf[:8], []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(buf[8:], 2)
	binary.LittleEndian.PutUint32(buf[20:], 0xe2088205)
	copy(buf[24:32], challenge)
	return buf
}

func ntlmsspAuth() []byte {
	buf := make([]byte, 64)
	copy(buf[:8], []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(buf[8:], 3)
	binary.LittleEndian.PutUint32(buf[60:], 0x00008201)
	return buf
}

func TestNTLMSSPNegotiateAndEdges(t *testing.T) {
	neg := ntlmsspMessage(1, []byte{0x07, 0x82, 0x08, 0xe2})
	n := parseRule(t, neg, "application-layer.ntlm", "NTLMSSP")
	require.Equal(t, []byte("NTLMSSP\x00"), bytesVal(t, n.Child("Signature")))
	require.Equal(t, uint64(1), uintVal(t, n.Child("MessageType")))
	require.Equal(t, uint64(0xe2088207), uintVal(t, n.Child("NegotiateFlags")))

	ch := ntlmsspChallenge([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	c := parseRule(t, ch, "application-layer.ntlm", "NTLMSSP")
	require.Equal(t, uint64(2), uintVal(t, c.Child("MessageType")))
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, c.Child("ServerChallenge")))
	require.Equal(t, uint64(0xe2088205), uintVal(t, c.Child("NegotiateFlags")))

	a := parseRule(t, ntlmsspAuth(), "application-layer.ntlm", "NTLMSSP")
	require.Equal(t, uint64(3), uintVal(t, a.Child("MessageType")))
	require.Equal(t, uint64(0x00008201), uintVal(t, a.Child("NegotiateFlags")))

	parseMustFail(t, nil, "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, []byte("NTLMSS"), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, append([]byte("NOTLMSSP"), 1, 0, 0, 0), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, ntlmsspMessage(0, nil), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, ntlmsspMessage(4, nil), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, append([]byte("NTLMSSP\x00"), 1), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, ntlmsspMessage(1, nil), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, ntlmsspMessage(2, make([]byte, 8)), "application-layer.ntlm", "NTLMSSP")
	parseMustFail(t, ntlmsspMessage(3, make([]byte, 4)), "application-layer.ntlm", "NTLMSSP")
}

func TestNTLMSSPInsideSMB2SessionSetup(t *testing.T) {
	ntlm := ntlmsspMessage(1, []byte{0x07, 0x82, 0x08, 0xe2})
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[3] = 1
	binary.LittleEndian.PutUint16(body[12:], 88)
	binary.LittleEndian.PutUint16(body[14:], uint16(len(ntlm)))
	raw := append(smb2SyncHeader(1, 0, 2), body...)
	raw = append(raw, ntlm...)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	require.Equal(t, uint64(1), uintVal(t, smb.Child("Command")))
	ss := mustChild(t, smb, "Session Setup Request")
	nt := mustChild(t, ss, "NTLMSSP")
	require.Equal(t, uint64(1), uintVal(t, nt.Child("MessageType")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
	wired := mustChild(t, eth, "IP", "TCP", "SMB2", "Session Setup Request", "NTLMSSP")
	require.Equal(t, uint64(1), uintVal(t, wired.Child("MessageType")))

	spnego := []byte{0x60, 0x0c, 0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x02, 0xa0, 0x02, 0x04, 0x00}
	body = make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[3] = 1
	binary.LittleEndian.PutUint16(body[12:], 88)
	binary.LittleEndian.PutUint16(body[14:], uint16(len(spnego)))
	raw = append(smb2SyncHeader(1, 0, 3), body...)
	raw = append(raw, spnego...)
	smb = parseRule(t, raw, "application-layer.smb2", "SMB2")
	sp := mustChild(t, smb, "Session Setup Request", "SPNEGO")
	require.Equal(t, uint64(0x60), uintVal(t, sp.Child("Tag")))
	require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}, bytesVal(t, sp.Child("OID")))
}

func kerberosAP(tag byte, body []byte) []byte {
	if len(body) > 127 {
		panic("short-form only")
	}
	return append([]byte{tag, byte(len(body))}, body...)
}

func TestKerberosTagsAndEdges(t *testing.T) {
	body := []byte{0x30, 0x03, 0x02, 0x01, 0x05}
	cases := []struct {
		name string
		tag  byte
	}{
		{"AS-REQ", 0x6a},
		{"AS-REP", 0x6b},
		{"TGS-REQ", 0x6c},
		{"TGS-REP", 0x6d},
		{"AP-REQ", 0x6e},
		{"AP-REP", 0x6f},
		{"ERROR", 0x7e},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := kerberosAP(tc.tag, body)
			k := parseRule(t, raw, "application-layer.kerberos", "Kerberos")
			require.Equal(t, uint64(tc.tag), uintVal(t, k.Child("Application Tag")))
			require.Equal(t, uint64(len(body)), uintVal(t, k.Child("Length")))
			require.Equal(t, body, bytesVal(t, k.Child("Body")))
		})
	}

	parseMustFail(t, nil, "application-layer.kerberos", "Kerberos")
	parseMustFail(t, []byte{0x30, 0x03, 0x02, 0x01, 0x05}, "application-layer.kerberos", "Kerberos")
	parseMustFail(t, []byte{0x6a, 0x81, 0x01}, "application-layer.kerberos", "Kerberos")
	parseMustFail(t, []byte{0x6a, 0x05, 0x00}, "application-layer.kerberos", "Kerberos") // truncated body

	udp := kerberosAP(0x6a, body)
	eth := parseEthernet(t, ipv4UDPFrame(t, 50000, 88, gopacket.Payload(udp)))
	require.Equal(t, uint64(0x6a), uintVal(t, mustChild(t, eth, "IP", "UDP", "Kerberos", "Application Tag")))

	rec := make([]byte, 4+len(udp))
	binary.BigEndian.PutUint32(rec, uint32(len(udp)))
	copy(rec[4:], udp)
	tcp := parseRule(t, rec, "application-layer.kerberos", "KerberosTCP")
	require.Equal(t, uint64(len(udp)), uintVal(t, tcp.Child("Record Length")))
	require.Equal(t, uint64(0x6a), uintVal(t, mustChild(t, tcp, "Record", "Application Tag")))

	ethTCP := parseEthernet(t, ipv4TCPFrame(t, 50000, 88, rec))
	require.Equal(t, uint64(0x6a), uintVal(t, mustChild(t, ethTCP, "IP", "TCP", "KerberosTCP", "Record", "Application Tag")))
}

func pgTyped(typ byte, payload []byte) []byte {
	buf := make([]byte, 5+len(payload))
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:], uint32(4+len(payload)))
	copy(buf[5:], payload)
	return buf
}

func TestPostgreSQLStartupSSLQueryAndEdges(t *testing.T) {
	ssl := make([]byte, 8)
	binary.BigEndian.PutUint32(ssl[0:], 8)
	binary.BigEndian.PutUint32(ssl[4:], 80877103)
	s := parseRule(t, ssl, "application-layer.postgresql", "SSLRequest")
	require.Equal(t, uint64(8), uintVal(t, s.Child("Length")))
	require.Equal(t, uint64(80877103), uintVal(t, s.Child("Code")))

	params := append([]byte("user\x00postgres\x00database\x00test\x00"), 0)
	st := make([]byte, 8+len(params))
	binary.BigEndian.PutUint32(st[0:], uint32(8+len(params)))
	binary.BigEndian.PutUint32(st[4:], 196608)
	copy(st[8:], params)
	startup := parseRule(t, st, "application-layer.postgresql", "Startup")
	require.Equal(t, uint64(196608), uintVal(t, startup.Child("Protocol")))
	require.Equal(t, params, bytesVal(t, startup.Child("Parameters")))

	q := pgTyped('Q', append([]byte("SELECT 1"), 0))
	msg := parseRule(t, q, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('Q'), uintVal(t, msg.Child("First")))
	require.Equal(t, append([]byte("SELECT 1"), 0), bytesVal(t, msg.Child("Payload")))

	authOK := pgTyped('R', []byte{0, 0, 0, 0})
	r := parseRule(t, authOK, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('R'), uintVal(t, r.Child("First")))

	errp := pgTyped('E', []byte("SERROR\x00C42601\x00Msyntax\x00\x00"))
	e := parseRule(t, errp, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('E'), uintVal(t, e.Child("First")))

	ready := pgTyped('Z', []byte{'I'})
	z := parseRule(t, ready, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('Z'), uintVal(t, z.Child("First")))

	cancel := make([]byte, 16)
	binary.BigEndian.PutUint32(cancel[0:], 16)
	binary.BigEndian.PutUint32(cancel[4:], 80877102)
	binary.BigEndian.PutUint32(cancel[8:], 99)
	binary.BigEndian.PutUint32(cancel[12:], 0x11223344)
	c := parseRule(t, cancel, "application-layer.postgresql", "CancelRequest")
	require.Equal(t, uint64(99), uintVal(t, c.Child("ProcessID")))

	parseMustFail(t, nil, "application-layer.postgresql", "PostgreSQL")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x04}, "application-layer.postgresql", "Startup")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01}, "application-layer.postgresql", "SSLRequest")
	parseMustFail(t, pgTyped('Q', nil)[:3], "application-layer.postgresql", "PostgreSQL")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, q))
	require.Equal(t, uint64('Q'), uintVal(t, mustChild(t, eth, "IP", "TCP", "PostgreSQL", "First")))

	ethSSL := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, ssl))
	require.Equal(t, uint64(80877103), uintVal(t, mustChild(t, ethSSL, "IP", "TCP", "PGSSLRequest", "Code")))

	ethCancel := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, cancel))
	require.Equal(t, uint64(99), uintVal(t, mustChild(t, ethCancel, "IP", "TCP", "PGCancel", "ProcessID")))

	ethStart := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, st))
	require.Equal(t, uint64(196608), uintVal(t, mustChild(t, ethStart, "IP", "TCP", "PGStartup", "Protocol")))

	term := pgTyped('X', nil)
	x := parseRule(t, term, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('X'), uintVal(t, x.Child("First")))

	pwd := pgTyped('p', append([]byte("secret"), 0))
	pp := parseRule(t, pwd, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('p'), uintVal(t, pp.Child("First")))
	require.Equal(t, append([]byte("secret"), 0), bytesVal(t, pp.Child("Payload")))

	copyData := pgTyped('d', []byte{1, 2, 3})
	d := parseRule(t, copyData, "application-layer.postgresql", "PostgreSQL")
	require.Equal(t, uint64('d'), uintVal(t, d.Child("First")))

	parseMustFail(t, pgTyped('Q', []byte("x"))[:5], "application-layer.postgresql", "PostgreSQL")
	parseMustFail(t, nil, "application-layer.postgresql", "SSLRequest")
	parseMustFail(t, nil, "application-layer.postgresql", "CancelRequest")
}

func tdsPacket(typ, status byte, payload []byte) []byte {
	buf := make([]byte, 8+len(payload))
	buf[0] = typ
	buf[1] = status
	binary.BigEndian.PutUint16(buf[2:], uint16(8+len(payload)))
	buf[6] = 1
	copy(buf[8:], payload)
	return buf
}

func TestTDSPreloginLoginBatchAndEdges(t *testing.T) {
	pre := tdsPacket(18, 1, []byte{0x00, 0x00, 0x1a, 0x00, 0x06, 0xff})
	p := parseRule(t, pre, "application-layer.tds", "TDS")
	require.Equal(t, uint64(18), uintVal(t, p.Child("Type")))
	require.Equal(t, uint64(1), uintVal(t, p.Child("Status")))
	require.Equal(t, uint64(len(pre)), uintVal(t, p.Child("Length")))
	require.Equal(t, []byte{0x00, 0x00, 0x1a, 0x00, 0x06, 0xff}, bytesVal(t, p.Child("Payload")))

	login := tdsPacket(16, 1, make([]byte, 32))
	l := parseRule(t, login, "application-layer.tds", "TDS")
	require.Equal(t, uint64(16), uintVal(t, l.Child("Type")))

	batch := tdsPacket(1, 1, []byte{0x53, 0x00, 0x45, 0x00}) // UTF-16LE "SE"
	b := parseRule(t, batch, "application-layer.tds", "TDS")
	require.Equal(t, uint64(1), uintVal(t, b.Child("Type")))

	resp := tdsPacket(4, 1, []byte{0x04, 0x01, 0x00})
	r := parseRule(t, resp, "application-layer.tds", "TDS")
	require.Equal(t, uint64(4), uintVal(t, r.Child("Type")))

	att := tdsPacket(6, 1, nil)
	a := parseRule(t, att, "application-layer.tds", "TDS")
	require.Equal(t, uint64(6), uintVal(t, a.Child("Type")))
	require.Equal(t, uint64(8), uintVal(t, a.Child("Length")))

	parseMustFail(t, nil, "application-layer.tds", "TDS")
	parseMustFail(t, []byte{99, 1, 0, 8, 0, 0, 1, 0}, "application-layer.tds", "TDS")
	short := tdsPacket(18, 1, []byte{1, 2})
	short[2], short[3] = 0, 7
	parseMustFail(t, short, "application-layer.tds", "TDS")
	trunc := tdsPacket(18, 1, []byte{1, 2, 3, 4})
	parseMustFail(t, trunc[:6], "application-layer.tds", "TDS")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1433, pre))
	require.Equal(t, uint64(18), uintVal(t, mustChild(t, eth, "IP", "TCP", "TDS", "Type")))

	rpc := tdsPacket(3, 1, []byte{0x00, 0x01})
	rp := parseRule(t, rpc, "application-layer.tds", "TDS")
	require.Equal(t, uint64(3), uintVal(t, rp.Child("Type")))

	sspi := tdsPacket(17, 1, ntlmsspMessage(1, []byte{0x07, 0x82, 0x08, 0xe2}))
	s := parseRule(t, sspi, "application-layer.tds", "TDS")
	require.Equal(t, uint64(17), uintVal(t, s.Child("Type")))

	tm := tdsPacket(14, 0, []byte{0x02})
	tr := parseRule(t, tm, "application-layer.tds", "TDS")
	require.Equal(t, uint64(14), uintVal(t, tr.Child("Type")))
	require.Equal(t, uint64(0), uintVal(t, tr.Child("Status")))

	parseMustFail(t, tdsPacket(18, 1, []byte{1})[:8], "application-layer.tds", "TDS")
}

func ajpPacket(magic uint16, code byte, body []byte) []byte {
	buf := make([]byte, 5+len(body))
	binary.BigEndian.PutUint16(buf[0:], magic)
	binary.BigEndian.PutUint16(buf[2:], uint16(1+len(body)))
	buf[4] = code
	copy(buf[5:], body)
	return buf
}

func TestAJPPingPongForwardAndEdges(t *testing.T) {
	cping := ajpPacket(0x1234, 0x0a, nil)
	p := parseRule(t, cping, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x1234), uintVal(t, p.Child("Magic")))
	require.Equal(t, uint64(1), uintVal(t, p.Child("Length")))
	require.Equal(t, uint64(0x0a), uintVal(t, p.Child("Code")))

	cpong := ajpPacket(0x4142, 0x09, nil)
	g := parseRule(t, cpong, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x4142), uintVal(t, g.Child("Magic")))
	require.Equal(t, uint64(0x09), uintVal(t, g.Child("Code")))

	fwd := ajpPacket(0x1234, 0x02, []byte{0x02, 0x00, 0x01, '/'})
	f := parseRule(t, fwd, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x02), uintVal(t, f.Child("Code")))
	require.Equal(t, []byte{0x02, 0x00, 0x01, '/'}, bytesVal(t, f.Child("Body")))

	shutdown := ajpPacket(0x1234, 0x07, nil)
	s := parseRule(t, shutdown, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x07), uintVal(t, s.Child("Code")))

	parseMustFail(t, nil, "application-layer.ajp", "AJP")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x01, 0x0a}, "application-layer.ajp", "AJP")
	parseMustFail(t, []byte{0x12, 0x34, 0x00, 0x00}, "application-layer.ajp", "AJP")
	parseMustFail(t, ajpPacket(0x1234, 0x0a, []byte{1, 2})[:4], "application-layer.ajp", "AJP")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8009, cping))
	require.Equal(t, uint64(0x0a), uintVal(t, mustChild(t, eth, "IP", "TCP", "AJP", "Code")))

	headers := ajpPacket(0x4142, 0x04, []byte{0x00, 0xc8, 0x00, 0x02, 'O', 'K'})
	h := parseRule(t, headers, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x04), uintVal(t, h.Child("Code")))

	end := ajpPacket(0x4142, 0x05, []byte{1})
	e := parseRule(t, end, "application-layer.ajp", "AJP")
	require.Equal(t, uint64(0x05), uintVal(t, e.Child("Code")))

	parseMustFail(t, ajpPacket(0x1234, 0x0a, []byte{1, 2, 3})[:5], "application-layer.ajp", "AJP")
	parseMustFail(t, []byte{0x12, 0x34, 0xff, 0xff, 0x0a}, "application-layer.ajp", "AJP")
}
