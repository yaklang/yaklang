package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1UDPApplications(t *testing.T) {
	// RFC 8415 DHCPv6 Solicit type 1, xid, empty options (header only).
	dhcpv6 := []byte{0x01, 0x00, 0x00, 0x01}
	d := parseRule(t, dhcpv6, "dhcpv6", "DHCPv6")
	require.Equal(t, uint64(1), uintVal(t, d.Child("Message Type")))
	eth := parseEthernet(t, ipv6UDPBytes(t, 546, 547, dhcpv6))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IPv6", "UDP", "DHCPv6").Child("Message Type")))

	// RFC 1350 TFTP RRQ
	rrq := append([]byte{0x00, 0x01}, []byte("file\x00octet\x00")...)
	tf := parseRule(t, rrq, "tftp", "TFTP")
	require.Equal(t, uint64(1), uintVal(t, tf.Child("Opcode")))
	require.Equal(t, "file", strVal(t, tf.Child("Filename")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 12345, 69, rrq))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "TFTP").Child("Opcode")))

	// RFC 3164 syslog: <PRI> + 15-char timestamp + hostname + tag/msg
	sys := []byte("<13>Sep  4 12:00:00 host sshd: ok\n")
	s := parseRule(t, sys, "syslog", "Syslog")
	require.Equal(t, "13", strVal(t, s.Child("PRI")))
	require.Equal(t, "Sep  4 12:00:00", strVal(t, s.Child("Timestamp")))
	require.Equal(t, "host", strVal(t, s.Child("Hostname")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 12345, 514, sys))
	require.Equal(t, "13", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("PRI")))
	require.Equal(t, "Sep  4 12:00:00", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("Timestamp")))

	// RFC 2453 §3.9.1 RIPv2 request-for-full-table: AFI 0, metric 16
	rip := mustHex(t, "010200000000000000000000000000000000000000000010")
	eth = parseEthernet(t, ipv4UDPBytes(t, 520, 520, rip))
	ripN := mustChild(t, eth, "IP", "UDP", "RIP")
	require.Equal(t, uint64(1), uintVal(t, ripN.Child("Command")))
	require.Equal(t, uint64(2), uintVal(t, ripN.Child("Version")))
	require.Equal(t, uint64(0), uintVal(t, ripN.Child("Entries").Children()[0].Child("Address Family")))
	require.Equal(t, uint64(16), uintVal(t, ripN.Child("Entries").Children()[0].Child("Metric")))

	// RFC 5769 §2.1 STUN Binding Request test vector
	stun := mustHex(t, ""+
		"00010058 2112a442 b7e7a701 bc34d686 fa87dfae "+
		"80220010 5354554e 20746573 7420636c 69656e74 "+
		"00240004 6e0001ff 80290008 932ff9b1 51263b36 "+
		"00060009 6576746a 3a683676 59202020 "+
		"00080014 9aeaa70c bfd8cb56 781ef2b5 b2d3f249 c1b571a2 "+
		"80280004 e57a3bcf")
	st := parseRule(t, stun, "stun", "STUN")
	require.Equal(t, uint64(0x0001), uintVal(t, st.Child("Message Type")))
	require.Equal(t, uint64(0x58), uintVal(t, st.Child("Length")))
	require.Equal(t, uint64(0x2112a442), uintVal(t, st.Child("Magic Cookie")))
	require.Equal(t, uint64(0x8022), uintVal(t, st.Child("Attributes").Children()[0].Child("Type")))
	require.Equal(t, "STUN test client", strVal(t, st.Child("Attributes").Children()[0].Child("Software")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 3478, 3478, stun))
	require.Equal(t, uint64(0x2112a442), uintVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Magic Cookie")))
	require.Equal(t, "STUN test client", strVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Attributes").Children()[0].Child("Software")))

	// RFC 3261 SIP REGISTER first line (gopacket sip_test.go style)
	sip := []byte("REGISTER sip:sip.provider.com SIP/2.0\r\nVia: SIP/2.0/UDP 10.0.0.1\r\nContent-Length: 0\r\n\r\n")
	si := parseRule(t, sip, "sip", "SIP")
	require.Equal(t, "REGISTER", strVal(t, mustChild(t, si, "SIP Request").Child("Method")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 5060, 5060, sip))
	require.Equal(t, "REGISTER", strVal(t, mustChild(t, eth, "IP", "UDP", "SIP", "SIP Request").Child("Method")))

	sdp := []byte("v=0\r\no=- 0 0 IN IP4 10.0.0.1\r\n")
	sd := parseRule(t, sdp, "sdp", "SDP")
	require.Equal(t, uint64('v'), uintVal(t, sd.Child("Type")))
	require.Equal(t, "0", strVal(t, sd.Child("Value")))
	require.Equal(t, uint64('o'), uintVal(t, sd.Child("Origin Type")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 5006, 5006, sdp))
	require.Equal(t, uint64('v'), uintVal(t, mustChild(t, eth, "IP", "UDP", "SDP").Child("Type")))
	require.Equal(t, uint64('o'), uintVal(t, mustChild(t, eth, "IP", "UDP", "SDP").Child("Origin Type")))


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

	// RFC 2661 data 0202 0014 0001 0002 … : T=0 L=0 S=0 O=1 Ver=2.
	// Length-present layout would steal 0x0014 as Length and report Tunnel=1.
	l2tp := mustHex(t, "02020014000100020000ff03002d")
	l := parseRule(t, l2tp, "l2tp", "L2TP")
	require.Equal(t, uint64(0x0014), uintVal(t, l.Child("Tunnel ID")))
	require.NotEqual(t, uint64(1), uintVal(t, l.Child("Tunnel ID")))
	require.Equal(t, uint64(2), uintVal(t, l.Child("Offset Size")))
	require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, l, "PPP").Child("Protocol")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1701, 1701, l2tp))
	require.Equal(t, uint64(0x0014), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP").Child("Tunnel ID")))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP").Child("Offset Size")))
	require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP", "PPP").Child("Protocol")))

	// RFC 7296 empty IKEv2 IKE_SA_INIT: version 0x20, exchange 0x22, length 0x1c
	ike := mustHex(t, "0000000000000001000000000000000000202208000000000000001c")
	ik := parseRule(t, ike, "ike", "IKE")
	require.Equal(t, uint64(0x20), uintVal(t, ik.Child("Version")))
	require.Equal(t, uint64(0x22), uintVal(t, ik.Child("Exchange Type")))
	require.Equal(t, uint64(28), uintVal(t, ik.Child("Length")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 4000, 500, ike))
	require.Equal(t, uint64(0x22), uintVal(t, mustChild(t, eth, "IP", "UDP", "IKE").Child("Exchange Type")))

	natt := parseEthernet(t, ipv4UDPBytes(t, 4500, 4500, ike))
	require.Equal(t, uint64(0x22), uintVal(t, mustChild(t, natt, "IP", "UDP", "NATT").Child("Exchange Type")))

	// OpenVPN P_CONTROL_HARD_RESET_CLIENT_V2 (opcode 7 << 3)
	openvpn := mustHex(t, "3801020304050607080000000001")
	ov := parseRule(t, openvpn, "openvpn", "OpenVPN")
	require.Equal(t, uint64(0x38), uintVal(t, ov.Child("OpcodeKey")))
	require.Equal(t, uint64(0), uintVal(t, ov.Child("Ack Count")))
	require.Equal(t, uint64(1), uintVal(t, ov.Child("Packet ID")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 1194, 1194, openvpn))
	require.Equal(t, uint64(0x38), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("OpcodeKey")))

	// WireGuard handshake initiation: type 1, 148 bytes (whitepaper §5.4.2)
	wg := mustHex(t, "0100000001000000"+
		"0000000000000000000000000000000000000000000000000000000000000000"+
		"000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
		"00000000000000000000000000000000000000000000000000000000"+
		"00000000000000000000000000000000"+
		"00000000000000000000000000000000")
	w := parseRule(t, wg, "wireguard", "WireGuard")
	require.Equal(t, uint64(1), uintVal(t, w.Child("Type")))
	require.Equal(t, uint64(1), uintVal(t, w.Child("Sender")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "WireGuard").Child("Type")))

	// RFC 2281 default Hello: Active, hellotime 3, holdtime 10, priority 100, auth cisco
	hsrp := mustHex(t, "000010030a640000636973636f000000c0a80101")
	hs := parseRule(t, hsrp, "hsrp", "HSRP")
	require.Equal(t, uint64(0), uintVal(t, hs.Child("Op Code")))
	require.Equal(t, uint64(16), uintVal(t, hs.Child("State")))
	require.Equal(t, uint64(3), uintVal(t, hs.Child("Hellotime")))
	require.Equal(t, uint64(10), uintVal(t, hs.Child("Holdtime")))
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

	// RFC 951 BOOTREQUEST + RFC 1497 magic cookie + END (DHCP Discover layout)
	bootp := mustHex(t, "010106001234567800000000"+
		"00000000000000000000000000000000"+
		"123456789abc00000000000000000000"+
		"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
		"0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
		"63825363ff")
	bp := parseRule(t, bootp, "application-layer.dhcp", "DHCP")
	require.Equal(t, uint64(1), uintVal(t, bp.Child("Operation")))
	require.Equal(t, uint64(0x63825363), uintVal(t, bp.Child("Magic Cookie")))
	eth = parseEthernet(t, ipv4UDPBytes(t, 68, 67, bootp))
	require.Equal(t, uint64(0x12345678), uintVal(t, mustChild(t, eth, "IP", "UDP", "DHCP").Child("Xid")))
}

func TestP1TCPApplications(t *testing.T) {
	// RFC 4271 §4.4 KEEPALIVE: 16×0xff marker, Length 19, Type 4
	bgp := mustHex(t, "ffffffffffffffffffffffffffffffff001304")
	b := parseRule(t, bgp, "bgp", "BGP")
	require.Equal(t, uint64(19), uintVal(t, b.Child("Length")))
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
	require.Equal(t, "ready", strVal(t, po.Child("Arg")))
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

	// MongoDB OP_QUERY admin.$cmd {ping:1} — unique header + collection + BSON
	mongo := mustHex(t, "360000000100000000000000d407000000000000"+
		"61646d696e2e24636d64000000000001000000"+
		"0f0000001070696e67000100000000")
	eth = parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
	mg := mustChild(t, eth, "IP", "TCP", "MongoDB")
	require.Equal(t, uint64(2004), uintVal(t, mg.Child("Op Code")))
	require.Equal(t, uint64(54), uintVal(t, mg.Child("Message Length")))
	require.Equal(t, "admin.$cmd", strVal(t, mustChild(t, mg, "OP_QUERY").Child("Collection")))
	qel := mustChild(t, mg, "OP_QUERY", "BSONDoc", "Elements").Children()
	require.GreaterOrEqual(t, len(qel), 1)
	require.Equal(t, "ping", strVal(t, qel[0].Child("Name")))
	require.Equal(t, uint64(1), uintVal(t, qel[0].Child("Int32")))

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

	// ApiVersions v0: length 10, api 18, ver 0, corr 0, null client id
	kf := mustHex(t, "0000000a0012000000000000ffff")
	k := parseRule(t, kf, "kafka", "Kafka")
	require.Equal(t, int64(10), intVal(t, k.Child("Length")))
	require.Equal(t, int64(18), intVal(t, k.Child("API Key")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 9092, 9092, kf))
	require.Equal(t, int64(18), intVal(t, mustChild(t, eth, "IP", "TCP", "Kafka").Child("API Key")))

	t.Run("jsonrpc/request", func(t *testing.T) {
		jr := []byte("{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":1}")
		j := parseRule(t, jr, "jsonrpc", "JSONRPC")
		pairs := j.Child("Pairs").Children()
		require.GreaterOrEqual(t, len(pairs), 2)
		require.Equal(t, "jsonrpc", strVal(t, pairs[0].Child("Key")))
		require.Equal(t, "2.0", strVal(t, pairs[0].Child("StrVal")))
		require.Equal(t, "method", strVal(t, pairs[1].Child("Key")))
		require.Equal(t, "ping", strVal(t, pairs[1].Child("StrVal")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 40002, 8545, jr))
		ethPairs := mustChild(t, eth, "IP", "TCP", "JSONRPC").Child("Pairs").Children()
		require.Equal(t, "ping", strVal(t, ethPairs[1].Child("StrVal")))
	})
	t.Run("jsonrpc/result", func(t *testing.T) {
		jr := []byte("{\"jsonrpc\":\"2.0\",\"result\":\"pong\",\"id\":1}")
		j := parseRule(t, jr, "jsonrpc", "JSONRPC")
		pairs := j.Child("Pairs").Children()
		require.Equal(t, "result", strVal(t, pairs[1].Child("Key")))
		require.Equal(t, "pong", strVal(t, pairs[1].Child("StrVal")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 40002, 8545, jr))
		require.Equal(t, "result", strVal(t, mustChild(t, eth, "IP", "TCP", "JSONRPC").Child("Pairs").Children()[1].Child("Key")))
	})

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

	jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001000101")...)
	jw := parseRule(t, jd, "jdwp", "JDWP")
	require.Equal(t, "JDWP-Handshake", string(bytesVal(t, jw.Child("Handshake"))))
	require.Equal(t, uint64(11), uintVal(t, mustChild(t, jw, "Command").Child("Length")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, jw, "Command").Child("Command Set")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, jw, "Command").Child("Command")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("Id")))

	php := []byte("i:1;")
	ph := parseRule(t, php, "php_ser", "PHPSer")
	require.Equal(t, uint64('i'), uintVal(t, ph.Child("Kind")))
	require.Equal(t, "1", strVal(t, ph.Child("Int")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 40001, 7777, php))
	require.Equal(t, uint64('i'), uintVal(t, mustChild(t, eth, "IP", "TCP", "PHPSer").Child("Kind")))

	pk := []byte{0x80, 0x02, 0x4b, 0x01, 0x2e}
	pi := parseRule(t, pk, "pickle", "Pickle")
	ops := pi.Child("Ops").Children()
	require.GreaterOrEqual(t, len(ops), 3)
	require.Equal(t, uint64(0x80), uintVal(t, ops[0].Child("Opcode")))
	require.Equal(t, uint64(2), uintVal(t, ops[0].Child("Version")))
	require.Equal(t, uint64(0x4b), uintVal(t, ops[1].Child("Opcode")))
	require.Equal(t, uint64(1), uintVal(t, ops[1].Child("BININT1")))
	require.Equal(t, uint64(0x2e), uintVal(t, ops[2].Child("Opcode")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 11312, 11312, pk))
	require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, eth, "IP", "TCP", "Pickle", "Ops").Children()[0].Child("Opcode")))

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

	hs := []byte{'H', 0x02, 0x00, 'N'}
	he := parseRule(t, hs, "hessian", "Hessian")
	require.Equal(t, uint64('H'), uintVal(t, he.Child("Magic")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 8089, 8089, hs))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "Hessian").Child("Major")))
	require.Equal(t, uint64('N'), uintVal(t, mustChild(t, eth, "IP", "TCP", "Hessian", "Values").Children()[0].Child("Tag")))

	rs := []byte("@RSYNCD: 31.0\n")
	ry := parseRule(t, rs, "rsync", "Rsync")
	require.Equal(t, "@RSYNCD:", strVal(t, ry.Child("Magic")))
	require.Equal(t, "31", strVal(t, ry.Child("Major")))
	require.Equal(t, "0", strVal(t, ry.Child("Minor")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 873, 873, rs))
	require.Equal(t, "31", strVal(t, mustChild(t, eth, "IP", "TCP", "Rsync").Child("Major")))

	rtsp := []byte("OPTIONS rtsp://cam/stream RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	rt := parseRule(t, rtsp, "rtsp", "RTSP")
	require.Equal(t, "OPTIONS", strVal(t, mustChild(t, rt, "RTSP Request").Child("Method")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 554, 554, rtsp))
	require.Equal(t, "OPTIONS", strVal(t, mustChild(t, eth, "IP", "TCP", "RTSP", "RTSP Request").Child("Method")))

	rtmp := mustHex(t, "03000000010000000072616e64")
	rmp := parseRule(t, rtmp, "rtmp", "RTMP")
	require.Equal(t, uint64(3), uintVal(t, rmp.Child("Version")))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, rmp, "C1").Child("Time")))
	require.Equal(t, uint64(0), uintVal(t, mustChild(t, rmp, "C1").Child("Zero")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, rtmp))
	require.Equal(t, uint64(3), uintVal(t, mustChild(t, eth, "IP", "TCP", "RTMP").Child("Version")))

	// RFC 959 §3.4.2 block mode: Descriptor EOR 0x80 (not EOF 0x40), Byte Count 10, Data
	fd := []byte{0x80, 0x00, 0x0a, 'f', 'i', 'l', 'e', '-', 'b', 'y', 't', 'e', 's'}
	eth = parseEthernet(t, ipv4TCPFrame(t, 20, 20, fd))
	blk := mustChild(t, eth, "IP", "TCP", "FTPData", "Blocks").Children()[0]
	require.Equal(t, uint64(0x80), uintVal(t, blk.Child("Descriptor")))
	require.Equal(t, uint64(10), uintVal(t, blk.Child("Byte Count")))
	require.Equal(t, []byte("file-bytes"), bytesVal(t, blk.Child("File Data")))

	jk := []byte("Protocol:HTTP11\n")
	jn := parseRule(t, jk, "jenkins", "Jenkins")
	require.Equal(t, "Protocol:", strVal(t, jn.Child("Prefix")))
	require.Equal(t, "HTTP11", strVal(t, jn.Child("Value")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 50000, 50000, jk))
	require.Equal(t, "HTTP11", strVal(t, mustChild(t, eth, "IP", "TCP", "Jenkins").Child("Value")))

	salt := []byte{0x00, 0x00, 0x00, 0x04, 'p', 'i', 'n', 'g'}
	sa := parseRule(t, salt, "salt", "Salt")
	require.Equal(t, uint64(4), uintVal(t, sa.Child("Frames").Children()[0].Child("Length")))
	eth = parseEthernet(t, ipv4TCPFrame(t, 4505, 4505, salt))
	require.Equal(t, []byte("ping"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "Salt", "Frames").Children()[0].Child("Payload")))

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
	t.Run("linux_sll/host", func(t *testing.T) {
		sll := make([]byte, 16)
		binary.BigEndian.PutUint16(sll[0:], 0)
		binary.BigEndian.PutUint16(sll[2:], 1)
		binary.BigEndian.PutUint16(sll[4:], 6)
		binary.BigEndian.PutUint16(sll[14:], 0x0001)
		sl := parseRule(t, sll, "linux_sll", "LinuxSLL")
		require.Equal(t, uint64(0), uintVal(t, sl.Child("Packet Type")))
		require.Equal(t, uint64(1), uintVal(t, sl.Child("Protocol")))
	})
	t.Run("linux_sll/outgoing", func(t *testing.T) {
		sll := make([]byte, 16)
		binary.BigEndian.PutUint16(sll[0:], 4)
		binary.BigEndian.PutUint16(sll[2:], 1)
		binary.BigEndian.PutUint16(sll[4:], 6)
		binary.BigEndian.PutUint16(sll[14:], 0x0001)
		sl := parseRule(t, sll, "linux_sll", "LinuxSLL")
		require.Equal(t, uint64(4), uintVal(t, sl.Child("Packet Type")))
	})

	t.Run("ieee_802_11/dot11", func(t *testing.T) {
		dot11 := make([]byte, 24)
		binary.LittleEndian.PutUint16(dot11[0:], 0x0008) // data subtype
		d11 := parseRule(t, dot11, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0008), uintVal(t, d11.Child("Frame Control")))
	})
	t.Run("ieee_802_11/rsn", func(t *testing.T) {
		rsn := make([]byte, 2+2+4+2+4+2+4)
		rsn[0] = 48
		rsn[1] = 18
		binary.LittleEndian.PutUint16(rsn[2:], 1)
		binary.LittleEndian.PutUint16(rsn[8:], 1)
		binary.LittleEndian.PutUint16(rsn[14:], 1)
		rs := parseRule(t, rsn, "ieee_802_11", "RSN")
		require.Equal(t, uint64(48), uintVal(t, rs.Child("Element ID")))
		require.Equal(t, uint64(1), uintVal(t, rs.Child("Version")))
	})

	sctp := make([]byte, 16)
	binary.BigEndian.PutUint16(sctp[0:], 1234)
	binary.BigEndian.PutUint16(sctp[2:], 80)
	binary.BigEndian.PutUint16(sctp[14:], 4)
	eth := parseEthernet(t, ipv4ProtoFrame(t, 132, sctp))
	require.Equal(t, uint64(1234), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Source Port")))
	require.Equal(t, uint64(80), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Destination Port")))

	// gopacket layers/ipsec_test.go testPacketIPSecAHTransport Ethernet+IPv4+AH+ICMP
	ahFrame := mustHex(t, ""+
		"7ec0ffc648f11a0e3c4e3b3a08004500"+
		"006c650a400040335201c0a80101c0a8"+
		"01020104000000000101000000012533"+
		"01b1a20bb6f1bdbf9d9e0800fbe50618"+
		"0001c6e1a35400000000c8f704000000"+
		"0000101112131415161718191a1b1c1d"+
		"1e1f202122232425262728292a2b2c2d"+
		"2e2f3031323334353637")
	eth = parseEthernet(t, ahFrame)
	ah := mustChild(t, eth, "IP", "AH")
	require.Equal(t, uint64(4), uintVal(t, ah.Child("Payload Len")))
	require.Equal(t, uint64(0x101), uintVal(t, ah.Child("SPI")))
	require.Equal(t, uint64(1), uintVal(t, ah.Child("Sequence")))

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

	t.Run("ber/integer", func(t *testing.T) {
		berInt := parseRule(t, []byte{0x02, 0x01, 0x05}, "application-layer.ber", "BER Element")
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, berInt, "Type").Child("Tag")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, berInt, "Type").Child("Class")))
		require.Equal(t, uint64(5), uintVal(t, berInt.Child("Integer")))
	})
	t.Run("ber/null", func(t *testing.T) {
		berNull := parseRule(t, []byte{0x05, 0x00}, "application-layer.ber", "BER Element")
		require.Equal(t, uint64(5), uintVal(t, mustChild(t, berNull, "Type").Child("Tag")))
	})


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
