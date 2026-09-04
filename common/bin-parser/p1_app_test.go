package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1UDPApplications(t *testing.T) {
	// RFC 8415 DHCPv6 Solicit type 1, xid, empty options
	dhcpv6 := []byte{0x01, 0x00, 0x00, 0x01}
	d := parseRule(t, dhcpv6, "dhcpv6", "DHCPv6")
	require.Equal(t, uint64(1), uintVal(t, d.Child("Message Type")))
	eth := parseEthernet(t, ipv4UDPBytes(t, 546, 547, dhcpv6))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "DHCPv6").Child("Message Type")))

	// RFC 1350 TFTP RRQ
	rrq := append([]byte{0x00, 0x01}, []byte("file\x00octet\x00")...)
	tf := parseRule(t, rrq, "tftp", "TFTP")
	require.Equal(t, uint64(1), uintVal(t, tf.Child("Opcode")))
	require.Equal(t, "file", strVal(t, tf.Child("Filename")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 12345, 69, rrq))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "TFTP").Child("Opcode")))

	// RFC 5424 syslog
	sys := []byte("<13>Sep  4 12:00:00 host sshd: ok\n")
	s := parseRule(t, sys, "syslog", "Syslog")
	require.Equal(t, "13", strVal(t, s.Child("PRI")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 12345, 514, sys))
	require.Equal(t, "13", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("PRI")))

	// RFC 2453 RIP v2 request
	rip := make([]byte, 24)
	rip[0] = 1
	rip[1] = 2
	binary.BigEndian.PutUint32(rip[20:], 16)
	eth = parseEthernet(t, ipv4UDPBytes(t, 520, 520, rip))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "RIP").Child("Command")))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "RIP").Child("Version")))

	// RFC 5389 STUN Binding Request
	stun := make([]byte, 20)
	binary.BigEndian.PutUint16(stun[0:], 0x0001)
	binary.BigEndian.PutUint32(stun[4:], 0x2112a442)
	st := parseRule(t, stun, "stun", "STUN")
	require.Equal(t, uint64(0x0001), uintVal(t, st.Child("Message Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 3478, 3478, stun))
	require.Equal(t, uint64(0x2112a442), uintVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Magic Cookie")))

	// RFC 3261 SIP REGISTER first line (gopacket sip_test.go style)
	sip := []byte("REGISTER sip:sip.provider.com SIP/2.0\r\nVia: SIP/2.0/UDP 10.0.0.1\r\nContent-Length: 0\r\n\r\n")
	si := parseRule(t, sip, "sip", "SIP")
	require.Equal(t, "REGISTER", strVal(t, mustChild(t, si, "SIP Request").Child("Method")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 5060, 5060, sip))
	require.Equal(t, "REGISTER", strVal(t, mustChild(t, eth, "IP", "UDP", "SIP", "SIP Request").Child("Method")))

	sdp := []byte("v=0\r\no=- 0 0 IN IP4 10.0.0.1\r\n")
	sd := parseRule(t, sdp, "sip", "SDP")
	require.True(t, sd.Child("Lines").IsList())
	require.Equal(t, uint64('v'), uintVal(t, sd.Child("Lines").Children()[0].Child("Type")))

	// RFC 3550 RTP v2
	rtp := []byte{0x80, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0xaa}
	rt := parseRule(t, rtp, "rtp", "RTP")
	require.Equal(t, uint64(0x80), uintVal(t, rt.Child("VersionPXPCC")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 5004, 5004, rtp))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTP").Child("Sequence")))

	rtcp := []byte{0x80, 200, 0x00, 0x01, 0x00, 0x00, 0x00, 0x03}
	rc := parseRule(t, rtcp, "rtp", "RTCP")
	require.Equal(t, uint64(200), uintVal(t, rc.Child("Packet Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 5005, 5005, rtcp))
	require.Equal(t, uint64(200), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTCP").Child("Packet Type")))

	// RFC 2661 L2TP data
	l2tp := []byte{0x02, 0x02, 0x00, 0x14, 0x00, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	l := parseRule(t, l2tp, "l2tp", "L2TP")
	require.Equal(t, uint64(1), uintVal(t, l.Child("Tunnel ID")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1701, 1701, l2tp))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP").Child("Session ID")))

	// RFC 7296 IKEv2 header version 0x20 exchange SA_INIT 34
	ike := make([]byte, 28)
	ike[16] = 0
	ike[17] = 0x20
	ike[18] = 34
	binary.BigEndian.PutUint32(ike[24:], 28)
	ik := parseRule(t, ike, "ike", "IKE")
	require.Equal(t, uint64(0x20), uintVal(t, ik.Child("Version")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 4000, 500, ike))
	require.Equal(t, uint64(34), uintVal(t, mustChild(t, eth, "IP", "UDP", "IKE").Child("Exchange Type")))

	natt := parseEthernet(t, ipv4UDPBytes(t, 4500, 4500, ike))
	require.Equal(t, uint64(34), uintVal(t, mustChild(t, natt, "IP", "UDP", "NATT").Child("Exchange Type")))

	openvpn := make([]byte, 13)
	openvpn[0] = 0x20 // opcode 4 (P_CONTROL_V1) << 3
	ov := parseRule(t, openvpn, "openvpn", "OpenVPN")
	require.Equal(t, uint64(0x20), uintVal(t, ov.Child("OpcodeKey")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1194, 1194, openvpn))
	require.Equal(t, uint64(0x20), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("OpcodeKey")))

	wg := make([]byte, 148)
	wg[0] = 1
	w := parseRule(t, wg, "wireguard", "WireGuard")
	require.Equal(t, uint64(1), uintVal(t, w.Child("Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "WireGuard").Child("Type")))

	hsrp := make([]byte, 20)
	hsrp[1] = 0 // hello
	hsrp[5] = 100
	copy(hsrp[8:16], []byte("cisco"))
	hs := parseRule(t, hsrp, "hsrp", "HSRP")
	require.Equal(t, uint64(100), uintVal(t, hs.Child("Priority")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1985, 1985, hsrp))
	require.Equal(t, uint64(100), uintVal(t, mustChild(t, eth, "IP", "UDP", "HSRP").Child("Priority")))

	nbtdg := make([]byte, 14)
	nbtdg[0] = 0x10
	nb := parseRule(t, nbtdg, "nbt_dg", "NBTDG")
	require.Equal(t, uint64(0x10), uintVal(t, nb.Child("Message Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 138, 138, nbtdg))
	require.Equal(t, uint64(0x10), uintVal(t, mustChild(t, eth, "IP", "UDP", "NBTDG").Child("Message Type")))

	ipmi := []byte{0x06, 0x00, 0xff, 0x06, 0x00}
	im := parseRule(t, ipmi, "ipmi", "IPMI")
	require.Equal(t, uint64(6), uintVal(t, im.Child("Version")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 623, 623, ipmi))
	require.Equal(t, uint64(6), uintVal(t, mustChild(t, eth, "IP", "UDP", "IPMI").Child("Class")))

	dtls := []byte{0x16, 0xfe, 0xfd, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00}
	dt := parseRule(t, dtls, "dtls", "DTLS")
	require.Equal(t, uint64(0x16), uintVal(t, dt.Child("Content Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 443, 443, dtls))
	require.Equal(t, uint64(0xfefd), uintVal(t, mustChild(t, eth, "IP", "UDP", "DTLS").Child("Version")))

	ssdp := []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n")
	require.Equal(t, "M-SEARCH", strVal(t, mustChild(t, parseRule(t, ssdp, "application-layer.http", "HTTP"), "HTTP Request").Child("Method")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1900, 1900, ssdp))
	require.Equal(t, "M-SEARCH", strVal(t, mustChild(t, eth, "IP", "UDP", "SSDP", "HTTP Request").Child("Method")))
}

func TestP1TCPApplications(t *testing.T) {
	bgp := make([]byte, 19)
	for i := 0; i < 16; i++ {
		bgp[i] = 0xff
	}
	binary.BigEndian.PutUint16(bgp[16:], 19)
	bgp[18] = 4 // keepalive
	b := parseRule(t, bgp, "bgp", "BGP")
	require.Equal(t, uint64(4), uintVal(t, b.Child("Type")))
	eth := parseEthernet(t, ipv4TCPFrame(t, 179, 179, bgp))
	require.Equal(t, uint64(4), uintVal(t, mustChild(t, eth, "IP", "TCP", "BGP").Child("Type")))

	socks4 := []byte{0x04, 0x01, 0x00, 0x50, 10, 0, 0, 1, 'u', 0}
	s4 := parseRule(t, socks4, "socks4", "SOCKS4")
	require.Equal(t, uint64(4), uintVal(t, s4.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 1080, 1080, socks4))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("Command")))

	tel := []byte{0xff, 0xfd, 0x01}
	te := parseRule(t, tel, "telnet", "Telnet")
	require.Equal(t, uint64(0xfd), uintVal(t, mustChild(t, te, "IAC").Child("Command")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 23, 23, tel))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "Telnet", "IAC").Child("Option")))

	pop := []byte("+OK ready\r\n")
	po := parseRule(t, pop, "pop3", "POP3")
	require.Equal(t, "+OK", strVal(t, po.Child("Status")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 110, 110, pop))
	require.Equal(t, "+OK", strVal(t, mustChild(t, eth, "IP", "TCP", "POP3").Child("Status")))

	imap := []byte("* OK IMAP4rev1 ready\r\n")
	im := parseRule(t, imap, "imap", "IMAP")
	require.Equal(t, "*", strVal(t, im.Child("Tag")))
	require.Equal(t, "OK", strVal(t, im.Child("Command")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 143, 143, imap))
	require.Equal(t, "OK", strVal(t, mustChild(t, eth, "IP", "TCP", "IMAP").Child("Command")))

	vnc := []byte("RFB 003.008\n")
	vn := parseRule(t, vnc, "vnc", "VNC")
	require.Equal(t, "RFB ", strVal(t, vn.Child("Magic")))
	require.Equal(t, "003", strVal(t, vn.Child("Major")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 5900, 5900, vnc))
	require.Equal(t, "008", strVal(t, mustChild(t, eth, "IP", "TCP", "VNC").Child("Minor")))

	mongo := make([]byte, 16)
	binary.LittleEndian.PutUint32(mongo[0:], 16)
	binary.LittleEndian.PutUint32(mongo[12:], 2013) // OP_MSG
	mg := parseRule(t, mongo, "mongodb", "MongoDB")
	require.Equal(t, uint64(2013), uintVal(t, mg.Child("Op Code")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
	require.Equal(t, uint64(16), uintVal(t, mustChild(t, eth, "IP", "TCP", "MongoDB").Child("Message Length")))

	mc := make([]byte, 24)
	mc[0] = 0x80
	mc[1] = 0x00 // GET
	mb := parseRule(t, mc, "memcached", "Memcached")
	require.Equal(t, uint64(0x80), uintVal(t, mb.Child("Magic")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 11211, 11211, mc))
	require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, eth, "IP", "TCP", "Memcached").Child("Magic")))

	amqp := []byte{'A', 'M', 'Q', 'P', 0x00, 0x00, 0x09, 0x01}
	am := parseRule(t, amqp, "amqp", "AMQP")
	require.Equal(t, uint64('A'), uintVal(t, am.Child("Type")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 5672, 5672, amqp))
	require.Equal(t, uint64(9), uintVal(t, mustChild(t, eth, "IP", "TCP", "AMQP").Child("Minor")))

	kf := make([]byte, 12)
	binary.BigEndian.PutUint32(kf[0:], 8)
	k := parseRule(t, kf, "kafka", "Kafka")
	require.Equal(t, int64(8), intVal(t, k.Child("Length")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 9092, 9092, kf))
	require.Equal(t, int64(0), intVal(t, mustChild(t, eth, "IP", "TCP", "Kafka").Child("API Key")))

	jr := []byte("{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":1}\n")
	j := parseRule(t, jr, "jsonrpc", "JSONRPC")
	require.Contains(t, strVal(t, j.Child("Body")), "jsonrpc")
	require.Contains(t, strVal(t, j.Child("Body")), "ping")
	eth = parseEthernet(t, ipv4TCPFrame(t, 40002, 8545, jr))
	require.Contains(t, strVal(t, mustChild(t, eth, "IP", "TCP", "JSONRPC").Child("Body")), "ping")

	tac := make([]byte, 12)
	tac[0] = 0xc0
	tac[1] = 1
	ta := parseRule(t, tac, "tacacs", "TACACS")
	require.Equal(t, uint64(0xc0), uintVal(t, ta.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 49, 49, tac))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "TACACS").Child("Type")))

	rpc := make([]byte, 40)
	binary.BigEndian.PutUint32(rpc[4:], 0)
	binary.BigEndian.PutUint32(rpc[8:], 2)
	binary.BigEndian.PutUint32(rpc[12:], 100000) // portmap
	on := parseRule(t, rpc, "onc_rpc", "ONCRPC")
	require.Equal(t, uint64(2), uintVal(t, on.Child("RPC Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 111, 111, rpc))
	require.Equal(t, uint64(100000), uintVal(t, mustChild(t, eth, "IP", "TCP", "ONCRPC").Child("Program")))

	bt := append([]byte{19}, []byte("BitTorrent protocol")...)
	bt = append(bt, make([]byte, 48)...)
	btt := parseRule(t, bt, "bittorrent", "BitTorrent")
	require.Equal(t, uint64(19), uintVal(t, btt.Child("Pstrlen")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 6881, 6881, bt))
	require.Equal(t, "BitTorrent protocol", string(bytesVal(t, mustChild(t, eth, "IP", "TCP", "BitTorrent").Child("Pstr"))))

	fcgi := []byte{1, 1, 0, 1, 0, 0, 0, 0}
	fc := parseRule(t, fcgi, "fastcgi", "FastCGI")
	require.Equal(t, uint64(1), uintVal(t, fc.Child("Type")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 9000, 9000, fcgi))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "FastCGI").Child("Version")))

	jd := []byte("JDWP-Handshake")
	jw := parseRule(t, jd, "jdwp", "JDWP")
	require.Equal(t, "JDWP-Handshake", string(bytesVal(t, jw.Child("Handshake"))))
	eth = parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
	require.Equal(t, "JDWP-Handshake", string(bytesVal(t, mustChild(t, eth, "IP", "TCP", "JDWP").Child("Handshake"))))

	php := []byte("i:1;")
	ph := parseRule(t, php, "php_ser", "PHPSer")
	require.Equal(t, uint64('i'), uintVal(t, ph.Child("Kind")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 40001, 7777, php))
	require.Equal(t, uint64('i'), uintVal(t, mustChild(t, eth, "IP", "TCP", "PHPSer").Child("Kind")))

	pk := []byte{0x80, 0x02, 0x4b, 0x01, 0x2e}
	pi := parseRule(t, pk, "pickle", "Pickle")
	require.Equal(t, uint64(2), uintVal(t, pi.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 11312, 11312, pk))
	require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, eth, "IP", "TCP", "Pickle").Child("Opcode")))

	rmi := []byte{'J', 'R', 'M', 'I', 0x00, 0x02, 0x4b}
	rm := parseRule(t, rmi, "rmi", "RMI")
	require.Equal(t, "JRMI", string(bytesVal(t, rm.Child("Magic"))))
	eth = parseEthernet(t, ipv4TCPFrame(t, 1099, 1099, rmi))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "RMI").Child("Version")))

	th := make([]byte, 12)
	binary.BigEndian.PutUint32(th[0:], 0x80010001)
	thr := parseRule(t, th, "thrift", "Thrift")
	require.Equal(t, uint64(0x80010001), uintVal(t, thr.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 9090, 9090, th))
	require.Equal(t, uint64(0x80010001), uintVal(t, mustChild(t, eth, "IP", "TCP", "Thrift").Child("Version")))

	pb := []byte{0x08, 0x01}
	p := parseRule(t, pb, "protobuf", "Protobuf")
	require.True(t, p.Child("Fields").IsList())
	require.Equal(t, uint64(0x08), uintVal(t, p.Child("Fields").Children()[0].Child("Tag")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 4011, 4011, pb))
	require.Equal(t, uint64(0x08), uintVal(t, mustChild(t, eth, "IP", "TCP", "Protobuf", "Fields").Children()[0].Child("Tag")))

	zb := append([]byte("ZBXD"), 0x01, 0x02, 0x00, 0x00, 0x00, '{', '}')
	z := parseRule(t, zb, "zabbix", "Zabbix")
	require.Equal(t, "ZBXD", string(bytesVal(t, z.Child("Magic"))))
	eth = parseEthernet(t, ipv4TCPFrame(t, 10050, 10050, zb))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "Zabbix").Child("Length")))

	hs := []byte{'H', 0x02, 0x00, 0x00}
	he := parseRule(t, hs, "hessian", "Hessian")
	require.Equal(t, uint64('H'), uintVal(t, he.Child("Magic")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 8089, 8089, hs))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "Hessian").Child("Major")))

	rs := []byte("@RSYNCD: 31.0\n")
	ry := parseRule(t, rs, "rsync", "Rsync")
	require.Equal(t, "@RSYNCD:", strVal(t, ry.Child("Magic")))
	require.Equal(t, "31.0", strVal(t, ry.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 873, 873, rs))
	require.Equal(t, "31.0", strVal(t, mustChild(t, eth, "IP", "TCP", "Rsync").Child("Version")))

	rtsp := []byte("OPTIONS rtsp://cam/stream RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	rt := parseRule(t, rtsp, "rtsp", "RTSP")
	require.Equal(t, "OPTIONS", strVal(t, mustChild(t, rt, "RTSP Request").Child("Method")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 554, 554, rtsp))
	require.Equal(t, "OPTIONS", strVal(t, mustChild(t, eth, "IP", "TCP", "RTSP", "RTSP Request").Child("Method")))

	rtmp := []byte{0x03}
	rmp := parseRule(t, rtmp, "rtmp", "RTMP")
	require.Equal(t, uint64(3), uintVal(t, rmp.Child("Version")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, rtmp))
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, eth, "IP", "TCP", "RTMP").Child("Version")))

	fd := []byte("file-bytes")
	eth = parseEthernet(t, ipv4TCPFrame(t, 20, 20, fd))
	require.Equal(t, []byte("file-bytes"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "FTPData", "Records").Children()[0].Child("Data")))

	jk := []byte("Protocol:HTTP11\n")
	jn := parseRule(t, jk, "jenkins", "Jenkins")
	require.Equal(t, "Protocol:", strVal(t, jn.Child("Prefix")))
	require.Equal(t, "HTTP11", strVal(t, jn.Child("Value")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 50000, 50000, jk))
	require.Equal(t, "HTTP11", strVal(t, mustChild(t, eth, "IP", "TCP", "Jenkins").Child("Value")))

	salt := []byte{0x00, 0x00, 0x00, 0x04, 'p', 'i', 'n', 'g'}
	sa := parseRule(t, salt, "salt", "Salt")
	require.Equal(t, uint64(4), uintVal(t, sa.Child("Length")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 4505, 4505, salt))
	require.Equal(t, []byte("ping"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "Salt").Child("Payload")))

	nr := make([]byte, 14)
	copy(nr[0:4], []byte(".NET"))
	binary.BigEndian.PutUint16(nr[4:], 1)
	nrem := parseRule(t, nr, "net_remoting", "NetRemoting")
	require.Equal(t, ".NET", string(bytesVal(t, nrem.Child("Preamble"))))
	eth = parseEthernet(t, ipv4TCPFrame(t, 8088, 8088, nr))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "NetRemoting").Child("Major")))

	giop := make([]byte, 12)
	copy(giop[0:4], []byte("GIOP"))
	giop[4] = 1
	giop[5] = 2
	giop[7] = 3
	parseMustFail(t, []byte("XXXX"), "application-layer.iiop", "GIOP")
	eth = parseEthernet(t, ipv4TCPFrame(t, 2809, 2809, giop))
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, eth, "IP", "TCP", "GIOP").Child("Message Type")))

	t3 := make([]byte, 19)
	binary.BigEndian.PutUint32(t3[0:], 19)
	t3[4] = 1
	eth = parseEthernet(t, ipv4TCPFrame(t, 7001, 7001, t3))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "T3").Child("Cmd")))

	pptp := make([]byte, 156)
	binary.BigEndian.PutUint16(pptp[0:], 156)
	binary.BigEndian.PutUint16(pptp[2:], 1)
	binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
	binary.BigEndian.PutUint16(pptp[8:], 1)
	pp := parseRule(t, pptp, "application-layer.pptp", "PPTP")
	require.Equal(t, uint64(0x1a2b3c4d), uintVal(t, pp.Child("MagicCookie")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP").Child("ControlMessageType")))

	rtspResp := []byte("RTSP/1.0 200 OK\r\nCSeq: 1\r\n\r\n")
	rr := parseRule(t, rtspResp, "rtsp", "RTSP")
	require.Equal(t, "200", strVal(t, mustChild(t, rr, "RTSP Response").Child("Status")))
}

func TestP1MiscAndAliases(t *testing.T) {
	sll := make([]byte, 16)
	binary.BigEndian.PutUint16(sll[0:], 0)
	binary.BigEndian.PutUint16(sll[2:], 1)
	binary.BigEndian.PutUint16(sll[4:], 6)
	binary.BigEndian.PutUint16(sll[14:], 0x0001)
	sl := parseRule(t, sll, "linux_sll", "LinuxSLL")
	require.Equal(t, uint64(1), uintVal(t, sl.Child("Protocol")))

	dot11 := make([]byte, 24)
	binary.LittleEndian.PutUint16(dot11[0:], 0x0008) // data subtype
	d11 := parseRule(t, dot11, "ieee_802_11", "Dot11")
	require.Equal(t, uint64(0x0008), uintVal(t, d11.Child("Frame Control")))

	rsn := make([]byte, 2+2+4+2+4+2+4)
	rsn[0] = 48
	rsn[1] = 18
	binary.LittleEndian.PutUint16(rsn[2:], 1)
	binary.LittleEndian.PutUint16(rsn[8:], 1)
	binary.LittleEndian.PutUint16(rsn[14:], 1)
	rs := parseRule(t, rsn, "ieee_802_11", "RSN")
	require.Equal(t, uint64(48), uintVal(t, rs.Child("Element ID")))
	require.Equal(t, uint64(1), uintVal(t, rs.Child("Version")))

	sctp := make([]byte, 16)
	binary.BigEndian.PutUint16(sctp[0:], 1234)
	binary.BigEndian.PutUint16(sctp[2:], 80)
	binary.BigEndian.PutUint16(sctp[14:], 4)
	eth := parseEthernet(t, ipv4ProtoFrame(t, 132, sctp))
	require.Equal(t, uint64(1234), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Source Port")))
	require.Equal(t, uint64(80), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Destination Port")))

	ah := make([]byte, 16)
	ah[1] = 1 // payload len 1 -> icv = (1+2)*4-12 = 0
	a := parseRule(t, ah, "ipsec", "AH")
	require.Equal(t, uint64(1), uintVal(t, a.Child("Payload Len")))
	eth = parseEthernet(t, ipv4ProtoFrame(t, 51, ah))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "AH").Child("Payload Len")))

	esp := make([]byte, 8)
	binary.BigEndian.PutUint32(esp[0:], 1)
	es := parseRule(t, esp, "ipsec", "ESP")
	require.Equal(t, uint64(1), uintVal(t, es.Child("SPI")))
	eth = parseEthernet(t, ipv4ProtoFrame(t, 50, esp))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "ESP").Child("SPI")))

	eigrp := make([]byte, 20)
	eigrp[0] = 2
	ei := parseRule(t, eigrp, "eigrp", "EIGRP")
	require.Equal(t, uint64(2), uintVal(t, ei.Child("Version")))
	eth = parseEthernet(t, ipv4ProtoFrame(t, 88, eigrp))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "EIGRP").Child("Version")))

	// mDNS is DNS on UDP 5353
	dns := []byte{0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
	eth = parseEthernet(t, ipv4UDPBytes(t, 5353, 5353, dns))
	md := mustChild(t, eth, "IP", "UDP", "MDNS")
	require.Equal(t, uint64(0x0100), uintVal(t, md.Child("Header").Child("Flags")))
	llmnrQ := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	llmnr := parseEthernet(t, ipv4UDPBytes(t, 5355, 5355, llmnrQ))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, llmnr, "IP", "UDP", "LLMNR", "Header").Child("ID")))

	snmpv3 := []byte{0x30, 0x05, 0x02, 0x01, 0x03}
	sv := parseRule(t, snmpv3, "application-layer.snmp", "SNMPv3")
	require.Equal(t, []byte{0x03}, bytesVal(t, sv.Child("Version")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 161, 161, snmpv3))
	require.Equal(t, []byte{0x03}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "SNMPv3").Child("Version")))

	parseMustFail(t, nil, "application-layer.ber", "BER Element")

	eap := []byte{0x01, 0x00, 0x00, 0x05, 0x01, 0x01, 0x00, 0x05, 0x01}
	// EAPOL version 1, type EAP-Packet, body length 5, EAP request identity
	ep := parseRule(t, eap, "eapol", "EAPOL")
	require.Equal(t, uint64(0), uintVal(t, ep.Child("Packet Type")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, ep, "EAPPacket").Child("Code")))

	// [MS-WKST] / [MS-RPRN] / [MS-TSCH] / [MS-DCOM] bind UUID + opnum
	wkssvc := []byte{0x98, 0xd0, 0xff, 0x6b, 0x12, 0xa1, 0x10, 0x36, 0x98, 0x33, 0x46, 0xc3, 0xf8, 0x7e, 0x34, 0x5a}
	spoolss := []byte{0x78, 0x56, 0x34, 0x12, 0x34, 0x12, 0xcd, 0xab, 0xef, 0x00, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab}
	atsvc := []byte{0x82, 0x06, 0xf7, 0x1f, 0x51, 0x0a, 0xe8, 0x30, 0x07, 0x6d, 0x74, 0x0b, 0xe8, 0xce, 0xe9, 0x8b}
	iox := []byte{0xc4, 0xfe, 0xfc, 0x99, 0x60, 0x52, 0x1b, 0x10, 0xbb, 0xcb, 0x00, 0xaa, 0x00, 0x21, 0x34, 0x7a}
	for _, tc := range []struct {
		name  string
		uuid  []byte
		opnum uint16
	}{
		{"WKSSVC", wkssvc, 0},
		{"SPOOLSS", spoolss, 0},
		{"ATSVC", atsvc, 0},
		{"IObjectExporter", iox, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bind := dcerpcBindUUID(tc.uuid)
			n := parseRule(t, bind, "application-layer.dcerpc", "DCERPC")
			require.Equal(t, tc.uuid, bytesVal(t, mustChild(t, n, "PDU", "Bind", "Contexts").Children()[0].Child("Abstract Syntax")))
			req := dcerpcRequestOp(tc.opnum, []byte{1, 2, 3, 4})
			r := parseRule(t, req, "application-layer.dcerpc", "DCERPC")
			require.Equal(t, uint64(tc.opnum), uintVal(t, mustChild(t, r, "PDU", "Request", "OpNum")))
			eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 135, bind))
			require.Equal(t, tc.uuid, bytesVal(t, mustChild(t, eth, "IP", "TCP", "DCERPC", "PDU", "Bind", "Contexts").Children()[0].Child("Abstract Syntax")))
		})
	}
}
