package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
)

func TestP0RoadmapCovered(t *testing.T) {
	var leftover []string
	for _, item := range ProtocolRoadmap {
		if item.Priority == priP0 && (item.Status == stTodo || item.Status == stPartial) {
			leftover = append(leftover, item.Name+"="+item.Status)
		}
	}
	require.Empty(t, leftover, "P0 protocols still todo/partial: %v", leftover)

	for _, item := range ProtocolCatalog {
		if item.Status != statusPartial {
			continue
		}
		for _, r := range ProtocolRoadmap {
			if r.Name == item.Name && r.Priority == priP0 {
				t.Errorf("P0 catalog protocol %q is still partial", item.Name)
			}
		}
	}
}

func TestSPNEGOAndEdges(t *testing.T) {
	raw := []byte{0x60, 0x0c, 0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x02, 0xa0, 0x02, 0x04, 0x00}
	s := parseRule(t, raw, "application-layer.spnego", "SPNEGO")
	require.Equal(t, uint64(0x60), uintVal(t, s.Child("Tag")))
	require.Equal(t, uint64(0x0c), uintVal(t, s.Child("Length")))
	require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}, bytesVal(t, s.Child("OID")))
	tok := mustChild(t, s, "Token")
	require.Equal(t, uint64(0xa0), uintVal(t, tok.Child("NegTag")))
	require.Equal(t, uint64(2), uintVal(t, tok.Child("NegLength")))
	require.Equal(t, []byte{0x04, 0x00}, bytesVal(t, tok.Child("Octets")))

	ntlmOID := []byte{0x60, 0x0c, 0x06, 0x0a, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}
	n := parseRule(t, ntlmOID, "application-layer.spnego", "SPNEGO")
	require.Equal(t, uint64(10), uintVal(t, n.Child("OID Length")))

	parseMustFail(t, nil, "application-layer.spnego", "SPNEGO")
	parseMustFail(t, []byte{0x30, 0x03, 0x02, 0x01, 0x01}, "application-layer.spnego", "SPNEGO")
	parseMustFail(t, []byte{0x60, 0x81, 0x01}, "application-layer.spnego", "SPNEGO")
	parseMustFail(t, []byte{0x60, 0x03, 0x04, 0x01, 0x00}, "application-layer.spnego", "SPNEGO")
	parseMustFail(t, []byte{0x60, 0x02, 0x06, 0x00}, "application-layer.spnego", "SPNEGO")
}

func TestNetNTLMv2AndPAC(t *testing.T) {
	blob := make([]byte, 44)
	blob[16] = 1
	blob[17] = 1
	copy(blob[32:40], []byte{9, 8, 7, 6, 5, 4, 3, 2})
	n := parseRule(t, blob, "application-layer.ntlm", "NetNTLMv2")
	require.Equal(t, uint64(1), uintVal(t, n.Child("RespType")))
	require.Equal(t, []byte{9, 8, 7, 6, 5, 4, 3, 2}, bytesVal(t, n.Child("ClientChallenge")))

	parseMustFail(t, blob[:16], "application-layer.ntlm", "NetNTLMv2")
	bad := append([]byte{}, blob...)
	bad[16] = 2
	parseMustFail(t, bad, "application-layer.ntlm", "NetNTLMv2")

	pac := make([]byte, 8+12)
	binary.LittleEndian.PutUint32(pac[0:], 1)
	binary.LittleEndian.PutUint32(pac[8:], 1)  // PAC_LOGON_INFO
	binary.LittleEndian.PutUint32(pac[12:], 8)
	binary.LittleEndian.PutUint32(pac[16:], 24)
	p := parseRule(t, pac, "application-layer.kerberos", "PAC")
	require.Equal(t, uint64(1), uintVal(t, p.Child("CBuffers")))
	buf := p.Child("Buffers")
	require.NotNil(t, buf)
	require.True(t, buf.IsList())
	require.Equal(t, uint64(1), uintVal(t, buf.Children()[0].Child("UlType")))

	empty := make([]byte, 8)
	e := parseRule(t, empty, "application-layer.kerberos", "PAC")
	require.Equal(t, uint64(0), uintVal(t, e.Child("CBuffers")))

	tooMany := make([]byte, 8)
	binary.LittleEndian.PutUint32(tooMany[0:], 65)
	parseMustFail(t, tooMany, "application-layer.kerberos", "PAC")
	parseMustFail(t, pac[:10], "application-layer.kerberos", "PAC")
}

func TestHTTP2PrefaceFrameAndEdges(t *testing.T) {
	preface := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	p := parseRule(t, preface, "application-layer.http2", "HTTP2Preface")
	require.Equal(t, preface, bytesVal(t, p.Child("Magic")))

	settings := make([]byte, 9)
	settings[3] = 4 // SETTINGS
	s := parseRule(t, settings, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(0), uintVal(t, s.Child("Length")))
	require.Equal(t, uint64(4), uintVal(t, s.Child("Type")))

	data := make([]byte, 9+5)
	data[2] = 5
	data[3] = 0
	data[4] = 0x01 // END_STREAM
	copy(data[9:], []byte("hello"))
	d := parseRule(t, data, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(5), uintVal(t, d.Child("Length")))
	require.Equal(t, []byte("hello"), bytesVal(t, d.Child("Octets")))

	headers := make([]byte, 9)
	headers[3] = 1
	binary.BigEndian.PutUint32(headers[5:], 1) // stream 1, R=0
	h := parseRule(t, headers, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(1), uintVal(t, h.Child("Type")))
	require.Equal(t, uint64(1), uintVal(t, h.Child("Stream Identifier")))

	parseMustFail(t, nil, "application-layer.http2", "HTTP2Preface")
	parseMustFail(t, []byte("GET / HTTP/1.1\r\n\r\nXXXXXXX"), "application-layer.http2", "HTTP2Preface")
	parseMustFail(t, []byte{0, 0, 1, 99, 0, 0, 0, 0, 0}, "application-layer.http2", "HTTP2")
	parseMustFail(t, data[:8], "application-layer.http2", "HTTP2")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, preface))
	require.Equal(t, preface, bytesVal(t, mustChild(t, eth, "IP", "TCP", "HTTP2Preface", "Magic")))

	eth2 := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, settings))
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, eth2, "IP", "TCP", "HTTP2", "Type")))
}

func TestWebSocketFramesAndEdges(t *testing.T) {
	text := append([]byte{0x81, 0x05}, []byte("hello")...)
	w := parseRule(t, text, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(1), uintVal(t, w.Child("FIN")))
	require.Equal(t, uint64(1), uintVal(t, w.Child("Opcode")))
	require.Equal(t, uint64(0), uintVal(t, w.Child("Mask")))
	require.Equal(t, "hello", strVal(t, w.Child("Text")))

	closeF := []byte{0x88, 0x00}
	c := parseRule(t, closeF, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(8), uintVal(t, c.Child("Opcode")))

	ping := []byte{0x89, 0x00}
	p := parseRule(t, ping, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(9), uintVal(t, p.Child("Opcode")))

	ext := make([]byte, 2+2+5)
	ext[0] = 0x82
	ext[1] = 126
	binary.BigEndian.PutUint16(ext[2:], 5)
	copy(ext[4:], []byte("world"))
	e := parseRule(t, ext, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(2), uintVal(t, e.Child("Opcode")))
	require.Equal(t, uint64(126), uintVal(t, e.Child("Payload Len")))
	require.Equal(t, "world", strVal(t, e.Child("Octets")))

	masked := []byte{0x81, 0x85, 1, 2, 3, 4, 'h' ^ 1, 'e' ^ 2, 'l' ^ 3, 'l' ^ 4, 'o' ^ 1}
	m := parseRule(t, masked, "application-layer.websocket", "WebSocket")
	require.Equal(t, uint64(1), uintVal(t, m.Child("Mask")))
	require.Equal(t, []byte{1, 2, 3, 4}, bytesVal(t, m.Child("Masking Key")))

	parseMustFail(t, nil, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0x83, 0x00}, "application-layer.websocket", "WebSocket")
	parseMustFail(t, []byte{0x81, 0x05, 'h'}, "application-layer.websocket", "WebSocket")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8080, text))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "WebSocket", "Opcode")))
}

func TestQUICHeadersAndEdges(t *testing.T) {
	initial := []byte{0xc0, 0x00, 0x00, 0x00, 0x01, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0}
	q := parseRule(t, initial, "application-layer.quic", "QUIC")
	require.Equal(t, uint64(0xc0), uintVal(t, q.Child("First Byte")))
	require.Equal(t, uint64(1), uintVal(t, q.Child("Version")))
	require.Equal(t, uint64(8), uintVal(t, q.Child("DCID Length")))
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, q.Child("DCID")))
	require.Equal(t, uint64(0), uintVal(t, q.Child("SCID Length")))

	vn := []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0, 0}
	v := parseRule(t, vn, "application-layer.quic", "QUIC")
	require.Equal(t, uint64(0), uintVal(t, v.Child("Version")))

	short := []byte{0x40, 0x01, 0x02}
	sh := parseRule(t, short, "application-layer.quic", "QUIC")
	require.Equal(t, uint64(0x40), uintVal(t, sh.Child("First Byte")))

	parseMustFail(t, nil, "application-layer.quic", "QUIC")
	parseMustFail(t, []byte{0x00}, "application-layer.quic", "QUIC")
	parseMustFail(t, []byte{0xc0, 0x00, 0x00, 0x00, 0x01, 21}, "application-layer.quic", "QUIC")
	parseMustFail(t, []byte{0xc0, 0x00, 0x00, 0x00, 0x01, 2, 1}, "application-layer.quic", "QUIC")

	eth := parseEthernet(t, ipv4UDPFrame(t, 50000, 443, gopacket.Payload(initial)))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "QUIC", "Version")))
}

func TestRADIUSAndEdges(t *testing.T) {
	user := []byte("nmap")
	attr := append([]byte{1, byte(2 + len(user))}, user...)
	pkt := make([]byte, 20+len(attr))
	pkt[0] = 1
	pkt[1] = 7
	binary.BigEndian.PutUint16(pkt[2:], uint16(len(pkt)))
	copy(pkt[20:], attr)
	r := parseRule(t, pkt, "application-layer.radius", "RADIUS")
	require.Equal(t, uint64(1), uintVal(t, r.Child("Code")))
	require.Equal(t, uint64(7), uintVal(t, r.Child("Identifier")))
	attrs := r.Child("Attributes")
	require.True(t, attrs.IsList())
	require.GreaterOrEqual(t, len(attrs.Children()), 1)
	require.Equal(t, uint64(1), uintVal(t, attrs.Children()[0].Child("Type")))
	require.Equal(t, user, bytesVal(t, attrs.Children()[0].Child("Value")))

	accept := make([]byte, 20)
	accept[0] = 2
	binary.BigEndian.PutUint16(accept[2:], 20)
	a := parseRule(t, accept, "application-layer.radius", "RADIUS")
	require.Equal(t, uint64(2), uintVal(t, a.Child("Code")))

	reject := make([]byte, 20)
	reject[0] = 3
	binary.BigEndian.PutUint16(reject[2:], 20)
	parseRule(t, reject, "application-layer.radius", "RADIUS")

	parseMustFail(t, nil, "application-layer.radius", "RADIUS")
	parseMustFail(t, []byte{99, 1, 0, 20}, "application-layer.radius", "RADIUS")
	short := make([]byte, 20)
	short[0] = 1
	binary.BigEndian.PutUint16(short[2:], 19)
	parseMustFail(t, short, "application-layer.radius", "RADIUS")
	parseMustFail(t, pkt[:22], "application-layer.radius", "RADIUS")

	eth := parseEthernet(t, ipv4UDPFrame(t, 50000, 1812, gopacket.Payload(pkt)))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "RADIUS", "Code")))
}

func TestTNSConnectAndEdges(t *testing.T) {
	data := []byte("(DESCRIPTION=)")
	pkt := make([]byte, 8+26+len(data))
	binary.BigEndian.PutUint16(pkt[0:], uint16(len(pkt)))
	pkt[4] = 1
	binary.BigEndian.PutUint16(pkt[8:], 0x0134)
	binary.BigEndian.PutUint16(pkt[10:], 0x0134)
	binary.BigEndian.PutUint16(pkt[24:], uint16(len(data)))
	binary.BigEndian.PutUint16(pkt[26:], 34)
	copy(pkt[34:], data)
	n := parseRule(t, pkt, "application-layer.tns", "TNS")
	require.Equal(t, uint64(len(pkt)), uintVal(t, n.Child("Packet Length")))
	require.Equal(t, uint64(1), uintVal(t, n.Child("Packet Type")))
	require.Equal(t, "(DESCRIPTION=)", strVal(t, mustChild(t, n, "Connect").Child("Connect Data")))

	accept := make([]byte, 8)
	binary.BigEndian.PutUint16(accept[0:], 8)
	accept[4] = 2
	a := parseRule(t, accept, "application-layer.tns", "TNS")
	require.Equal(t, uint64(2), uintVal(t, a.Child("Packet Type")))

	refuse := make([]byte, 8)
	binary.BigEndian.PutUint16(refuse[0:], 8)
	refuse[4] = 4
	parseRule(t, refuse, "application-layer.tns", "TNS")

	dataPkt := make([]byte, 12)
	binary.BigEndian.PutUint16(dataPkt[0:], 12)
	dataPkt[4] = 6
	dataPkt[10] = 'A'
	dataPkt[11] = 'B'
	d := parseRule(t, dataPkt, "application-layer.tns", "TNS")
	require.Equal(t, uint64(6), uintVal(t, d.Child("Packet Type")))
	require.Equal(t, uint64(0), uintVal(t, d.Child("Data Flag")))
	require.Equal(t, "AB", strVal(t, d.Child("Octets")))

	parseMustFail(t, nil, "application-layer.tns", "TNS")
	parseMustFail(t, []byte{0, 7, 0, 0, 1, 0, 0}, "application-layer.tns", "TNS")
	badType := make([]byte, 8)
	binary.BigEndian.PutUint16(badType[0:], 8)
	badType[4] = 99
	parseMustFail(t, badType, "application-layer.tns", "TNS")
	shortConnect := make([]byte, 22)
	binary.BigEndian.PutUint16(shortConnect[0:], 22)
	shortConnect[4] = 1
	parseMustFail(t, shortConnect, "application-layer.tns", "TNS")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1521, pkt))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "TNS", "Packet Type")))
	require.Equal(t, "(DESCRIPTION=)", strVal(t, mustChild(t, eth, "IP", "TCP", "TNS", "Connect").Child("Connect Data")))
}

func dcerpcHeader(ptype byte, frag uint16, callID uint32) []byte {
	h := make([]byte, 16)
	h[0] = 5
	h[2] = ptype
	h[3] = 0x03
	h[4] = 0x10
	binary.LittleEndian.PutUint16(h[8:], frag)
	binary.LittleEndian.PutUint32(h[12:], callID)
	return h
}

func TestDCERPCBindRequestAndEdges(t *testing.T) {
	epm := []byte{0x08, 0x83, 0xaf, 0xe1, 0x1f, 0x5d, 0xc9, 0x11, 0x91, 0xa4, 0x08, 0x00, 0x2b, 0x14, 0xa0, 0xfa}
	ndr := []byte{0x04, 0x5d, 0x88, 0x8a, 0xeb, 0x1c, 0xc9, 0x11, 0x9f, 0xe8, 0x08, 0x00, 0x2b, 0x10, 0x48, 0x60}
	body := make([]byte, 12+44)
	binary.LittleEndian.PutUint16(body[0:], 5840)
	binary.LittleEndian.PutUint16(body[2:], 5840)
	body[8] = 1
	body[12] = 1
	copy(body[16:32], epm)
	binary.LittleEndian.PutUint32(body[32:], 3)
	copy(body[36:52], ndr)
	binary.LittleEndian.PutUint32(body[52:], 2)
	raw := append(dcerpcHeader(11, uint16(16+len(body)), 1), body...)
	n := parseRule(t, raw, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(5), uintVal(t, n.Child("RPC Vers")))
	require.Equal(t, uint64(11), uintVal(t, n.Child("PType")))
	bind := mustChild(t, n, "PDU", "Bind")
	require.Equal(t, uint64(1), uintVal(t, bind.Child("Num Ctx Items")))
	ctx := bind.Child("Contexts")
	require.True(t, ctx.IsList())
	require.Equal(t, epm, bytesVal(t, ctx.Children()[0].Child("Abstract Syntax")))

	reqBody := make([]byte, 8)
	binary.LittleEndian.PutUint16(reqBody[6:], 15)
	req := append(dcerpcHeader(0, 24, 2), reqBody...)
	r := parseRule(t, req, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(0), uintVal(t, r.Child("PType")))
	require.Equal(t, uint64(15), uintVal(t, mustChild(t, r, "PDU", "Request", "OpNum")))

	resp := append(dcerpcHeader(2, 24, 2), reqBody...)
	rp := parseRule(t, resp, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(2), uintVal(t, rp.Child("PType")))

	parseMustFail(t, nil, "application-layer.dcerpc", "DCERPC")
	parseMustFail(t, []byte{4, 0, 0, 0}, "application-layer.dcerpc", "DCERPC")
	badP := dcerpcHeader(30, 16, 1)
	parseMustFail(t, badP, "application-layer.dcerpc", "DCERPC")
	shortFrag := dcerpcHeader(0, 10, 1)
	parseMustFail(t, shortFrag, "application-layer.dcerpc", "DCERPC")

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 135, raw))
	require.Equal(t, uint64(11), uintVal(t, mustChild(t, eth, "IP", "TCP", "DCERPC", "PType")))
	ethReq := parseEthernet(t, ipv4TCPFrame(t, 50000, 135, req))
	require.Equal(t, uint64(15), uintVal(t, mustChild(t, ethReq, "IP", "TCP", "DCERPC", "PDU", "Request", "OpNum")))
}

func TestJavaSerAndSMB3AndEdges(t *testing.T) {
	js := []byte{0xac, 0xed, 0x00, 0x05}
	j := parseRule(t, js, "application-layer.java_ser", "JavaSer")
	require.Equal(t, uint64(0xaced), uintVal(t, j.Child("Magic")))
	require.Equal(t, uint64(5), uintVal(t, j.Child("Version")))
	parseMustFail(t, []byte{0xac, 0xed, 0x00, 0x04}, "application-layer.java_ser", "JavaSer")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x05}, "application-layer.java_ser", "JavaSer")
	parseMustFail(t, js[:2], "application-layer.java_ser", "JavaSer")

	ethJ := parseEthernet(t, ipv4TCPFrame(t, 50000, 1099, js))
	require.Equal(t, uint64(0xaced), uintVal(t, mustChild(t, ethJ, "IP", "TCP", "JavaSer", "Magic")))

	xf := make([]byte, 52)
	binary.LittleEndian.PutUint32(xf[0:], 0x424d53fd)
	binary.LittleEndian.PutUint32(xf[36:], 100)
	binary.LittleEndian.PutUint16(xf[42:], 1)
	binary.LittleEndian.PutUint64(xf[44:], 0x1111)
	s := parseRule(t, xf, "application-layer.smb3", "SMB3Transform")
	require.Equal(t, uint64(0x424d53fd), uintVal(t, s.Child("ProtocolId")))
	require.Equal(t, uint64(100), uintVal(t, s.Child("OriginalMessageSize")))
	require.Equal(t, uint64(0x1111), uintVal(t, s.Child("SessionId")))
	parseMustFail(t, nil, "application-layer.smb3", "SMB3Transform")
	bad := append([]byte{}, xf...)
	binary.LittleEndian.PutUint32(bad[0:], 0x424d53fe)
	parseMustFail(t, bad, "application-layer.smb3", "SMB3Transform")
	parseMustFail(t, xf[:20], "application-layer.smb3", "SMB3Transform")

	ethS := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, xf))
	require.Equal(t, uint64(0x424d53fd), uintVal(t, mustChild(t, ethS, "IP", "TCP", "SMB3Transform", "ProtocolId")))
}

func dnsLikeQuery(name string) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:], 0x1234)
	binary.BigEndian.PutUint16(buf[2:], 0x0100)
	binary.BigEndian.PutUint16(buf[4:], 1)
	buf = append(buf, byte(len(name)))
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint16(tmp[0:], 1)
	binary.BigEndian.PutUint16(tmp[2:], 1)
	return append(buf, tmp...)
}

// RFC 1002 first-level encoded "*" NBSTAT query (bettercap packets/nbns.go NBNSRequest).
func nbnsStarStatQuery() []byte {
	buf := []byte{0x82, 0x28, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 32, 'C', 'K'}
	for i := 0; i < 30; i++ {
		buf = append(buf, 'A')
	}
	buf = append(buf, 0, 0x00, 0x21, 0x00, 0x01)
	return buf
}

func TestNBNSLLMNRAndEdges(t *testing.T) {
	q := dnsLikeQuery("TEST")
	n := parseRule(t, q, "application-layer.nbns", "NBNS")
	require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, n, "Header", "ID")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Header", "Questions")))
	qs := n.Child("Questions")
	require.True(t, qs.IsList())
	require.Equal(t, uint64(1), uintVal(t, qs.Children()[0].Child("Type")))
	require.Equal(t, "TEST", strVal(t, qs.Children()[0].Child("Name").Children()[0].Child("Text")))

	l := parseRule(t, q, "application-layer.nbns", "LLMNR")
	require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, l, "Header", "ID")))

	empty := make([]byte, 12)
	e := parseRule(t, empty, "application-layer.nbns", "NBNS")
	require.Equal(t, uint64(0), uintVal(t, mustChild(t, e, "Header", "Questions")))

	parseMustFail(t, nil, "application-layer.nbns", "NBNS")
	parseMustFail(t, q[:10], "application-layer.nbns", "NBNS")
	parseMustFail(t, q[:14], "application-layer.nbns", "NBNS")

	ethN := parseEthernet(t, ipv4UDPFrame(t, 50000, 137, gopacket.Payload(q)))
	require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, ethN, "IP", "UDP", "NBNS", "Header", "ID")))
	ethL := parseEthernet(t, ipv4UDPFrame(t, 50000, 5355, gopacket.Payload(q)))
	require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, ethL, "IP", "UDP", "LLMNR", "Header", "ID")))
}

func TestTLSClientHelloJA3AndHTTPWPAD(t *testing.T) {
	hello := make([]byte, 0, 64)
	hello = append(hello, 0x03, 0x03)
	hello = append(hello, make([]byte, 32)...)
	hello = append(hello, 0)
	hello = append(hello, 0x00, 0x04, 0x00, 0x2f, 0x00, 0x35)
	hello = append(hello, 0x01, 0x00)
	hs := []byte{0x01, 0x00, 0x00, byte(len(hello))}
	hs = append(hs, hello...)
	ch := parseRule(t, hs, "application-layer.tls_hello", "TLSClientHello")
	require.Equal(t, uint64(1), uintVal(t, ch.Child("Handshake Type")))
	inner := mustChild(t, ch, "ClientHello")
	require.Equal(t, uint64(0x0303), uintVal(t, inner.Child("Legacy Version")))
	suites := inner.Child("Cipher Suites").Children()
	require.Equal(t, uint64(0x002f), uintVal(t, suites[0].Child("Suite")))
	require.Equal(t, uint64(0x0035), uintVal(t, suites[1].Child("Suite")))

	parseMustFail(t, []byte{0x02, 0x00, 0x00, 0x00}, "application-layer.tls_hello", "TLSClientHello")
	parseMustFail(t, hs[:3], "application-layer.tls_hello", "TLSClientHello")

	rec := []byte{0x16, 0x03, 0x03, 0x00, byte(len(hs))}
	rec = append(rec, hs...)
	ethT := parseEthernet(t, ipv4TCPFrame(t, 50000, 443, rec))
	require.Equal(t, uint64(22), uintVal(t, mustChild(t, ethT, "IP", "TCP", "TLS", "Record Layer", "ContentType")))

	wpad := []byte("GET /wpad.dat HTTP/1.1\r\nHost: wpad\r\nContent-Length: 0\r\n\r\n")
	h := parseRule(t, wpad, "application-layer.http", "HTTP")
	require.Equal(t, "GET", strVal(t, mustChild(t, h, "HTTP Request", "Method")))
	require.Equal(t, "/wpad.dat", strVal(t, mustChild(t, h, "HTTP Request", "Path")))

	connect := []byte("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nContent-Length: 0\r\n\r\n")
	c := parseRule(t, connect, "application-layer.http", "HTTP")
	require.Equal(t, "CONNECT", strVal(t, mustChild(t, c, "HTTP Request", "Method")))
	require.Equal(t, "example.com:443", strVal(t, mustChild(t, c, "HTTP Request", "Path")))

	ethW := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, wpad))
	require.Equal(t, "/wpad.dat", strVal(t, mustChild(t, ethW, "IP", "TCP", "HTTP", "HTTP Request", "Path")))
	ethC := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, connect))
	require.Equal(t, "CONNECT", strVal(t, mustChild(t, ethC, "IP", "TCP", "HTTP", "HTTP Request", "Method")))
}

func TestKerberosEmptyBodyAndTCPCap(t *testing.T) {
	empty := []byte{0x6a, 0x00}
	k := parseRule(t, empty, "application-layer.kerberos", "Kerberos")
	require.Equal(t, uint64(0x6a), uintVal(t, k.Child("Application Tag")))
	require.Equal(t, uint64(0), uintVal(t, k.Child("Length")))

	huge := make([]byte, 4)
	binary.BigEndian.PutUint32(huge, 0x200000)
	parseMustFail(t, huge, "application-layer.kerberos", "KerberosTCP")
	parseMustFail(t, []byte{0, 0, 0, 1, 0x6a}, "application-layer.kerberos", "KerberosTCP")
}
