package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func utf16LE(s string) []byte {
	out := make([]byte, len(s)*2)
	for i, c := range s {
		out[i*2] = byte(c)
	}
	return out
}

func TestSMB2TreeConnectCreateReadWriteClose(t *testing.T) {
	path := utf16LE(`\\srv\share`)
	tcBody := make([]byte, 8+len(path))
	binary.LittleEndian.PutUint16(tcBody[0:], 9)
	binary.LittleEndian.PutUint16(tcBody[4:], 72)
	binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
	copy(tcBody[8:], path)
	raw := append(smb2SyncHeader(3, 0, 4), tcBody...)
	smb := parseRule(t, raw, "application-layer.smb2", "SMB2")
	req := mustChild(t, smb, "Tree Connect Request")
	require.Equal(t, uint64(9), uintVal(t, req.Child("StructureSize")))
	require.Equal(t, path, bytesVal(t, req.Child("Path")))

	tcr := make([]byte, 16)
	binary.LittleEndian.PutUint16(tcr[0:], 16)
	tcr[2] = 1 // disk
	raw = append(smb2SyncHeader(3, 1, 4), tcr...)
	resp := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Tree Connect Response")
	require.Equal(t, uint64(1), uintVal(t, resp.Child("ShareType")))

	name := utf16LE("file.txt")
	cr := make([]byte, 56+len(name))
	binary.LittleEndian.PutUint16(cr[0:], 57)
	binary.LittleEndian.PutUint32(cr[4:], 2) // impersonation
	binary.LittleEndian.PutUint32(cr[24:], 0x00120189)
	binary.LittleEndian.PutUint32(cr[36:], 1) // FILE_OPEN
	binary.LittleEndian.PutUint16(cr[44:], 120)
	binary.LittleEndian.PutUint16(cr[46:], uint16(len(name)))
	copy(cr[56:], name)
	raw = append(smb2SyncHeader(5, 0, 5), cr...)
	creq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Create Request")
	require.Equal(t, uint64(57), uintVal(t, creq.Child("StructureSize")))
	require.Equal(t, name, bytesVal(t, creq.Child("Name")))
	require.Equal(t, uint64(1), uintVal(t, creq.Child("CreateDisposition")))

	fid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	cresp := make([]byte, 88)
	binary.LittleEndian.PutUint16(cresp[0:], 89)
	binary.LittleEndian.PutUint32(cresp[4:], 1)
	copy(cresp[64:80], fid)
	raw = append(smb2SyncHeader(5, 1, 5), cresp...)
	crr := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Create Response")
	require.Equal(t, uint64(89), uintVal(t, crr.Child("StructureSize")))
	require.Equal(t, fid, bytesVal(t, crr.Child("FileId")))

	rd := make([]byte, 48)
	binary.LittleEndian.PutUint16(rd[0:], 49)
	binary.LittleEndian.PutUint32(rd[4:], 8)
	copy(rd[16:32], fid)
	raw = append(smb2SyncHeader(8, 0, 6), rd...)
	rreq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Read Request")
	require.Equal(t, uint64(8), uintVal(t, rreq.Child("Length")))

	rdata := []byte("abcdefgh")
	rr := make([]byte, 16+len(rdata))
	binary.LittleEndian.PutUint16(rr[0:], 17)
	rr[2] = 80
	binary.LittleEndian.PutUint32(rr[4:], uint32(len(rdata)))
	copy(rr[16:], rdata)
	raw = append(smb2SyncHeader(8, 1, 6), rr...)
	rresp := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Read Response")
	require.Equal(t, rdata, bytesVal(t, rresp.Child("Data")))

	wr := make([]byte, 48+len(rdata))
	binary.LittleEndian.PutUint16(wr[0:], 49)
	binary.LittleEndian.PutUint16(wr[2:], 112)
	binary.LittleEndian.PutUint32(wr[4:], uint32(len(rdata)))
	copy(wr[16:32], fid)
	copy(wr[48:], rdata)
	raw = append(smb2SyncHeader(9, 0, 7), wr...)
	wreq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Write Request")
	require.Equal(t, rdata, bytesVal(t, wreq.Child("Data")))

	cl := make([]byte, 24)
	binary.LittleEndian.PutUint16(cl[0:], 24)
	copy(cl[8:24], fid)
	raw = append(smb2SyncHeader(6, 0, 8), cl...)
	closeReq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Close")
	require.Equal(t, uint64(24), uintVal(t, closeReq.Child("StructureSize")))

	loff := make([]byte, 4)
	binary.LittleEndian.PutUint16(loff[0:], 4)
	raw = append(smb2SyncHeader(2, 0, 9), loff...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Logoff", "StructureSize")))

	ioctl := make([]byte, 56)
	binary.LittleEndian.PutUint16(ioctl[0:], 57)
	binary.LittleEndian.PutUint32(ioctl[4:], 0x0011c017) // FSCTL_PIPE_TRANSCEIVE
	raw = append(smb2SyncHeader(11, 0, 10), ioctl...)
	io := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "IOCTL")
	require.Equal(t, uint64(0x0011c017), uintVal(t, io.Child("CtlCode")))

	echo := make([]byte, 4)
	binary.LittleEndian.PutUint16(echo[0:], 4)
	raw = append(smb2SyncHeader(13, 0, 11), echo...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Echo", "StructureSize")))

	td := make([]byte, 4)
	binary.LittleEndian.PutUint16(td[0:], 4)
	raw = append(smb2SyncHeader(4, 0, 12), td...)
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Tree Disconnect", "StructureSize")))

	parseMustFail(t, append(smb2SyncHeader(3, 0, 4), tcBody[:4]...), "application-layer.smb2", "SMB2")
}

func TestHTTP2SettingsPingGoaway(t *testing.T) {
	// SETTINGS with HEADER_TABLE_SIZE=4096 and ENABLE_PUSH=0
	st := make([]byte, 9+12)
	st[2] = 12
	st[3] = 4
	binary.BigEndian.PutUint16(st[9:], 1)
	binary.BigEndian.PutUint32(st[11:], 4096)
	binary.BigEndian.PutUint16(st[15:], 2)
	binary.BigEndian.PutUint32(st[17:], 0)
	n := parseRule(t, st, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(4), uintVal(t, n.Child("Type")))
	require.Equal(t, st[9:], bytesVal(t, n.Child("Settings")))

	ping := make([]byte, 9+8)
	ping[2] = 8
	ping[3] = 6
	copy(ping[9:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	p := parseRule(t, ping, "application-layer.http2", "HTTP2")
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, p.Child("Ping")))

	ga := make([]byte, 9+8)
	ga[2] = 8
	ga[3] = 7
	binary.BigEndian.PutUint32(ga[13:], 1) // NO_ERROR is 0; PROTOCOL_ERROR=1 at offset 9+4
	g := parseRule(t, ga, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(1), uintVal(t, g.Child("Error Code")))

	wu := make([]byte, 9+4)
	wu[2] = 4
	wu[3] = 8
	binary.BigEndian.PutUint32(wu[9:], 65535)
	w := parseRule(t, wu, "application-layer.http2", "HTTP2")
	require.Equal(t, uint64(65535), uintVal(t, w.Child("Window Size Increment")))

	parseMustFail(t, ping[:10], "application-layer.http2", "HTTP2")
	badSet := make([]byte, 9+5)
	badSet[2] = 5
	badSet[3] = 4
	parseMustFail(t, badSet, "application-layer.http2", "HTTP2")
}

func TestMQTTPublishSubscribePing(t *testing.T) {
	// PUBLISH qos0 topic "a" payload skipped
	pub := []byte{0x30, 0x03, 0x00, 0x01, 'a'}
	p := parseRule(t, pub, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(3), uintVal(t, p.Child("Packet Type")))
	require.Equal(t, "a", strVal(t, mustChild(t, p, "Payload", "Publish", "Topic")))

	sub := []byte{0x82, 0x02, 0x00, 0x07}
	s := parseRule(t, sub, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(8), uintVal(t, s.Child("Packet Type")))
	require.Equal(t, uint64(7), uintVal(t, mustChild(t, s, "Payload", "Subscribe", "Packet ID")))

	ping := []byte{0xc0, 0x00}
	pg := parseRule(t, ping, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(12), uintVal(t, pg.Child("Packet Type")))

	pong := []byte{0xd0, 0x00}
	require.Equal(t, uint64(13), uintVal(t, parseRule(t, pong, "application-layer.mqtt", "MQTT").Child("Packet Type")))

	puback := []byte{0x40, 0x02, 0x00, 0x01}
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, puback, "application-layer.mqtt", "MQTT"), "Payload", "Packet ID")))

	parseMustFail(t, []byte{0x30, 0x05, 0x00, 0x10}, "application-layer.mqtt", "MQTT")
}

func TestDCERPCBindAckFaultAndSMB1AndX(t *testing.T) {
	ackBody := make([]byte, 10)
	binary.LittleEndian.PutUint16(ackBody[0:], 5840)
	binary.LittleEndian.PutUint16(ackBody[2:], 5840)
	binary.LittleEndian.PutUint16(ackBody[8:], 0)
	raw := append(dcerpcHeader(12, uint16(16+len(ackBody)), 1), ackBody...)
	n := parseRule(t, raw, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(12), uintVal(t, n.Child("PType")))
	require.Equal(t, uint64(5840), uintVal(t, mustChild(t, n, "PDU", "BindAck", "Max Xmit Frag")))

	nak := append(dcerpcHeader(13, 18, 1), 0, 0)
	require.Equal(t, uint64(13), uintVal(t, parseRule(t, nak, "application-layer.dcerpc", "DCERPC").Child("PType")))

	faultBody := make([]byte, 12)
	binary.LittleEndian.PutUint32(faultBody[8:], 0x00000005)
	fr := append(dcerpcHeader(3, 28, 2), faultBody...)
	f := parseRule(t, fr, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(5), uintVal(t, mustChild(t, f, "PDU", "Fault", "Status")))

	respBody := make([]byte, 8)
	rr := append(dcerpcHeader(2, 24, 2), respBody...)
	require.Equal(t, uint64(2), uintVal(t, parseRule(t, rr, "application-layer.dcerpc", "DCERPC").Child("PType")))

	// SMB1 Tree Connect AndX (0x75)
	body := []byte{4, 0xff, 0, 0, 0, 0, 0}
	raw = append(smb1Header(0x75, 0x18, 0xc807, 2), body...)
	smb := parseRule(t, raw, "application-layer.smb", "SMB")
	require.Equal(t, uint64(0x75), uintVal(t, smb.Child("Command")))
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, smb, "AndX", "WordCount")))
}

func TestICMPTimestampRedirectAndNBNSAnswer(t *testing.T) {
	ts := make([]byte, 20)
	ts[0] = 13
	binary.BigEndian.PutUint16(ts[4:], 7)
	binary.BigEndian.PutUint16(ts[6:], 1)
	binary.BigEndian.PutUint32(ts[8:], 100)
	n := parseRule(t, ts, "internet_control_message_protocol", "ICMP")
	require.Equal(t, uint64(13), uintVal(t, n.Child("Type")))
	require.Equal(t, uint64(7), uintVal(t, mustChild(t, n, "ICMP Timestamp", "Identifier")))
	require.Equal(t, uint64(100), uintVal(t, mustChild(t, n, "ICMP Timestamp", "Originate")))

	redir := make([]byte, 8)
	redir[0] = 5
	copy(redir[4:], []byte{10, 0, 0, 1})
	r := parseRule(t, redir, "internet_control_message_protocol", "ICMP")
	require.Equal(t, []byte{10, 0, 0, 1}, bytesVal(t, mustChild(t, r, "ICMP Redirect", "Gateway")))

	parseMustFail(t, ts[:6], "internet_control_message_protocol", "ICMP")

	q := dnsLikeQuery("TEST")
	// turn into a response with one A answer
	ans := make([]byte, len(q)+20)
	copy(ans, q)
	binary.BigEndian.PutUint16(ans[2:], 0x8400)
	binary.BigEndian.PutUint16(ans[6:], 1)
	off := len(q)
	ans[off] = 4
	copy(ans[off+1:], []byte("TEST"))
	ans[off+5] = 0
	binary.BigEndian.PutUint16(ans[off+6:], 1)
	binary.BigEndian.PutUint16(ans[off+8:], 1)
	binary.BigEndian.PutUint32(ans[off+10:], 60)
	binary.BigEndian.PutUint16(ans[off+14:], 4)
	copy(ans[off+16:], []byte{10, 0, 0, 9})
	nb := parseRule(t, ans, "application-layer.nbns", "NBNS")
	answers := nb.Child("Answers")
	require.True(t, answers.IsList())
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, nb, "Header", "Answer RRs")))
	require.Equal(t, []byte{10, 0, 0, 9}, bytesVal(t, answers.Children()[0].Child("RData")))

	ll := parseRule(t, ans, "application-layer.nbns", "LLMNR")
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, ll, "Header", "Answer RRs")))
}

func TestRDPTPKTOn3389(t *testing.T) {
	// TPKT version 3, length 8, dummy X224 4 bytes
	tpkt := []byte{0x03, 0x00, 0x00, 0x08, 0x02, 0xe0, 0x00, 0x00}
	n := parseRule(t, tpkt, "application-layer.msrdp", "TPKT")
	require.Equal(t, uint64(3), uintVal(t, n.Child("Version")))
	require.Equal(t, uint64(8), uintVal(t, n.Child("PacketLength")))
	require.Equal(t, []byte{0x02, 0xe0, 0x00, 0x00}, bytesVal(t, n.Child("TPDU")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 3389, tpkt))
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, eth, "IP", "TCP", "TPKT", "Version")))

	parseMustFail(t, []byte{0x03, 0x00}, "application-layer.msrdp", "TPKT")
}

func TestSSHPacketAndFTPSMTPCommands(t *testing.T) {
	ident := []byte("SSH-2.0-OpenSSH_8.9\r\n")
	require.Equal(t, "SSH-2.0-OpenSSH_8.9", strVal(t, parseRule(t, ident, "application-layer.ssh", "SSH").Child("Identification")))

	pkt := make([]byte, 28)
	binary.BigEndian.PutUint32(pkt[0:], 24)
	pkt[4] = 6
	pkt[5] = 20 // SSH_MSG_KEXINIT
	p := parseRule(t, pkt, "application-layer.ssh", "SSHPacket")
	require.Equal(t, uint64(24), uintVal(t, p.Child("Packet Length")))
	require.Equal(t, uint64(20), uintVal(t, mustChild(t, p, "Payload", "Message Number")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, pkt))
	require.Equal(t, uint64(20), uintVal(t, mustChild(t, eth, "IP", "TCP", "SSHPacket", "Payload", "Message Number")))

	parseMustFail(t, []byte{0x00, 0x00, 0x88, 0xb9, 0x04}, "application-layer.ssh", "SSHPacket")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x05, 0x06}, "application-layer.ssh", "SSHPacket")

	user := []byte("USER anonymous\r\n")
	f := parseRule(t, user, "application-layer.ftp", "FTPCommand")
	require.Equal(t, "USER anonymous", strVal(t, f.Child("Line")))
	ethF := parseEthernet(t, ipv4TCPFrame(t, 50000, 21, user))
	require.Equal(t, "USER anonymous", strVal(t, mustChild(t, ethF, "IP", "TCP", "FTPCommand", "Line")))
	parseMustFail(t, []byte("GET / HTTP/1.1\r\n"), "application-layer.ftp", "FTPCommand")
	parseMustFail(t, []byte("USER"), "application-layer.ftp", "FTPCommand")

	ehlo := []byte("EHLO mail.example.com\r\n")
	s := parseRule(t, ehlo, "application-layer.smtp", "SMTPCommand")
	require.Equal(t, "EHLO mail.example.com", strVal(t, s.Child("Line")))
	ethS := parseEthernet(t, ipv4TCPFrame(t, 50000, 25, ehlo))
	require.Equal(t, "EHLO mail.example.com", strVal(t, mustChild(t, ethS, "IP", "TCP", "SMTPCommand", "Line")))
	parseMustFail(t, []byte("FOO bar\r\n"), "application-layer.smtp", "SMTPCommand")
}

func TestSMB1CloseTransactionLDAPJavaQUIC(t *testing.T) {
	closeBody := []byte{3, 0x00, 0x40, 0xff, 0xff, 0xff, 0xff, 0, 0}
	raw := append(smb1Header(0x04, 0x18, 0xc807, 3), closeBody...)
	cl := parseRule(t, raw, "application-layer.smb", "SMB")
	require.Equal(t, uint64(0x04), uintVal(t, cl.Child("Command")))
	require.Equal(t, uint64(0x4000), uintVal(t, mustChild(t, cl, "Close", "FID")))

	td := append(smb1Header(0x71, 0x18, 0xc807, 4), 0, 0, 0)
	require.Equal(t, uint64(0x71), uintVal(t, parseRule(t, td, "application-layer.smb", "SMB").Child("Command")))

	tx := make([]byte, 1+28+2+1)
	tx[0] = 14
	binary.LittleEndian.PutUint16(tx[1+28:], 1)
	raw = append(smb1Header(0x25, 0x18, 0xc807, 5), tx...)
	tr := parseRule(t, raw, "application-layer.smb", "SMB")
	require.Equal(t, uint64(14), uintVal(t, mustChild(t, tr, "Transaction", "WordCount")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, tr, "Transaction", "ByteCount")))

	raw = append(smb1Header(0x32, 0x18, 0xc807, 6), tx...)
	require.Equal(t, uint64(0x32), uintVal(t, parseRule(t, raw, "application-layer.smb", "SMB").Child("Command")))

	badClose := append(smb1Header(0x04, 0x18, 0xc807, 7), 2, 0, 0, 0, 0, 0, 0, 0)
	parseMustFail(t, badClose, "application-layer.smb", "SMB")
	parseMustFail(t, append(smb1Header(0x25, 0x18, 0xc807, 8), 4), "application-layer.smb", "SMB")

	bind := []byte{0x30, 0x0c, 0x02, 0x01, 0x01, 0x60, 0x07, 0x02, 0x01, 0x03, 0x04, 0x00, 0x80, 0x00}
	ldap := parseRule(t, bind, "application-layer.ldap", "LDAPMessage")
	require.Equal(t, uint64(0x30), uintVal(t, ldap.Child("Identifier")))
	require.Equal(t, uint64(0x60), uintVal(t, mustChild(t, ldap, "Body", "ProtocolOp Tag")))
	require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, ldap, "Body", "MessageID")))

	unbind := []byte{0x30, 0x05, 0x02, 0x01, 0x01, 0x42, 0x00}
	require.Equal(t, uint64(0x42), uintVal(t, mustChild(t, parseRule(t, unbind, "application-layer.ldap", "LDAPMessage"), "Body", "ProtocolOp Tag")))

	ethL := parseEthernet(t, ipv4TCPFrame(t, 50000, 389, bind))
	require.Equal(t, uint64(0x60), uintVal(t, mustChild(t, ethL, "IP", "TCP", "LDAPMessage", "Body", "ProtocolOp Tag")))

	parseMustFail(t, []byte{0x31, 0x03, 0x02, 0x01, 0x01}, "application-layer.ldap", "LDAPMessage")
	parseMustFail(t, []byte{0x30, 0x05, 0x02, 0x01, 0x01, 0x01, 0x00}, "application-layer.ldap", "LDAPMessage")

	js := []byte{0xac, 0xed, 0x00, 0x05, 0x70}
	j := parseRule(t, js, "application-layer.java_ser", "JavaSer")
	require.Equal(t, uint64(0x70), uintVal(t, mustChild(t, j, "JavaContent", "Content Type")))
	parseMustFail(t, []byte{0x00}, "application-layer.java_ser", "JavaContent")

	tok := []byte{0xc0, 0x00, 0x00, 0x00, 0x01, 0, 0, 3, 1, 2, 3}
	q := parseRule(t, tok, "application-layer.quic", "QUIC")
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, q, "QUICToken", "Token Length")))
	require.Equal(t, []byte{1, 2, 3}, bytesVal(t, mustChild(t, q, "QUICToken", "Token")))
}

func TestTNSConnectSNMPVarbindX224AndDCERPCStub(t *testing.T) {
	cdata := []byte("(DESCRIPTION=)")
	pkt := make([]byte, 8+26+len(cdata))
	binary.BigEndian.PutUint16(pkt[0:], uint16(len(pkt)))
	pkt[4] = 1
	binary.BigEndian.PutUint16(pkt[8:], 0x0134)
	binary.BigEndian.PutUint16(pkt[10:], 0x0134)
	binary.BigEndian.PutUint16(pkt[24:], uint16(len(cdata)))
	binary.BigEndian.PutUint16(pkt[26:], 34)
	copy(pkt[34:], cdata)
	n := parseRule(t, pkt, "application-layer.tns", "TNS")
	require.Equal(t, uint64(1), uintVal(t, n.Child("Packet Type")))
	require.Equal(t, uint64(len(cdata)), uintVal(t, mustChild(t, n, "Connect", "Connect Data Length")))
	require.Equal(t, cdata, bytesVal(t, mustChild(t, n, "Connect", "Connect Data")))
	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1521, pkt))
	require.Equal(t, cdata, bytesVal(t, mustChild(t, eth, "IP", "TCP", "TNS", "Connect", "Connect Data")))

	shortConnect := make([]byte, 34)
	binary.BigEndian.PutUint16(shortConnect[0:], 48)
	shortConnect[4] = 1
	binary.BigEndian.PutUint16(shortConnect[24:], 14)
	parseMustFail(t, shortConnect, "application-layer.tns", "TNS")

	snmpRaw := []byte{0x30, 0x26, 0x02, 0x01, 0x00, 0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c', 0xa0, 0x19, 0x02, 0x01, 0x01, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x30, 0x0e, 0x30, 0x0c, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00, 0x05, 0x00}
	s := parseRule(t, snmpRaw, "application-layer.snmp", "SNMP")
	require.Equal(t, uint64(0x30), uintVal(t, mustChild(t, s, "PDU Body", "Variable Bindings", "Sequence Tag")))
	require.Equal(t, uint64(0x0e), uintVal(t, mustChild(t, s, "PDU Body", "Variable Bindings", "Sequence Length")))

	x224 := []byte{0x06, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00}
	x := parseRule(t, x224, "application-layer.msrdp", "X224")
	require.Equal(t, uint64(6), uintVal(t, x.Child("Length")))
	require.Equal(t, uint64(0xe0), uintVal(t, x.Child("Flag")))

	neg := []byte{0x01, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}
	ng := parseRule(t, neg, "application-layer.msrdp", "Negotiation")
	require.Equal(t, uint64(1), uintVal(t, ng.Child("Type")))
	require.Equal(t, uint64(8), uintVal(t, ng.Child("Length")))

	stub := []byte{9, 8, 7, 6}
	reqBody := make([]byte, 8+len(stub))
	binary.LittleEndian.PutUint16(reqBody[6:], 3)
	copy(reqBody[8:], stub)
	req := append(dcerpcHeader(0, uint16(16+len(reqBody)), 2), reqBody...)
	r := parseRule(t, req, "application-layer.dcerpc", "DCERPC")
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, r, "PDU", "Request", "OpNum")))
	require.Equal(t, stub, bytesVal(t, mustChild(t, r, "PDU", "Request", "Stub")))
}

