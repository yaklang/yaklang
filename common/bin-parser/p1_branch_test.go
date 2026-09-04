package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1BranchRows(t *testing.T) {
	t.Run("ieee_802_1ad/arp", func(t *testing.T) {
		q := parseRule(t, append([]byte{0x00, 0x64, 0x08, 0x06}, []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}...), "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(0x0806), uintVal(t, q.Child("Type")))
	})
	t.Run("ieee_802_1ad/vlan", func(t *testing.T) {
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		q := parseRule(t, append([]byte{0x00, 0x64, 0x81, 0x00, 0x00, 0x01, 0x08, 0x06}, arp...), "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(0x8100), uintVal(t, q.Child("Type")))
	})

	t.Run("ppp/lcp", func(t *testing.T) {
		p := parseRule(t, []byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x01, 0x00, 0x0e, 0x03, 0x04, 0xc0, 0x23, 0x05, 0x06, 0x0f, 0x3f, 0x11, 0x7c}, "ppp", "PPP")
		require.Equal(t, uint64(0xc021), uintVal(t, p.Child("Protocol")))
	})
	t.Run("ppp/pap", func(t *testing.T) {
		pap := []byte{0xff, 0x03, 0xc0, 0x23, 0x01, 0x00, 0x00, 0x0e, 0x04, 0x69, 0x78, 0x69, 0x61, 0x04, 0x69, 0x78, 0x69, 0x61}
		p := parseRule(t, pap, "ppp", "PPP")
		require.Equal(t, uint64(0xc023), uintVal(t, p.Child("Protocol")))
	})

	t.Run("challenge_handshake_authentication_protocol/challenge", func(t *testing.T) {
		ch := parseRule(t, mustHex(t, "01030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574"), "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(1), uintVal(t, ch.Child("Code")))
	})
	t.Run("challenge_handshake_authentication_protocol/response", func(t *testing.T) {
		ch := parseRule(t, mustHex(t, "02030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574"), "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(2), uintVal(t, ch.Child("Code")))
	})

	t.Run("eapol/eap", func(t *testing.T) {
		ep := parseRule(t, []byte{0x01, 0x00, 0x00, 0x05, 0x01, 0x01, 0x00, 0x05, 0x01}, "eapol", "EAPOL")
		require.Equal(t, uint64(0), uintVal(t, ep.Child("Packet Type")))
	})
	t.Run("eapol/start", func(t *testing.T) {
		ep := parseRule(t, []byte{0x01, 0x01, 0x00, 0x00}, "eapol", "EAPOL")
		require.Equal(t, uint64(1), uintVal(t, ep.Child("Packet Type")))
	})

	t.Run("loopback/other", func(t *testing.T) {
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, []byte{0x00, 0x01}, "loopback", "Loopback").Child("Function")))
	})
	t.Run("loopback/custom", func(t *testing.T) {
		require.Equal(t, uint64(0x0002), uintVal(t, parseRule(t, []byte{0x00, 0x02}, "loopback", "Loopback").Child("Function")))
	})

	t.Run("l2tp/flags", func(t *testing.T) {
		l := parseRule(t, mustHex(t, "000200140001ff03002d"), "l2tp", "L2TP")
		require.Equal(t, uint64(0x0014), uintVal(t, l.Child("Tunnel ID")))
	})
	t.Run("l2tp/session", func(t *testing.T) {
		l := parseRule(t, mustHex(t, "000200140004ff03002d"), "l2tp", "L2TP")
		require.Equal(t, uint64(4), uintVal(t, l.Child("Session ID")))
	})

	t.Run("pptp/sccrq", func(t *testing.T) {
		pptp := make([]byte, 156)
		pptp[1] = 156
		pptp[3] = 1
		pptp[4], pptp[5], pptp[6], pptp[7] = 0x1a, 0x2b, 0x3c, 0x4d
		pptp[9] = 1
		require.Equal(t, uint64(0x1a2b3c4d), uintVal(t, parseRule(t, pptp, "application-layer.pptp", "PPTP").Child("MagicCookie")))
	})
	t.Run("pptp/sccrp", func(t *testing.T) {
		pptp := make([]byte, 156)
		pptp[1] = 156
		pptp[3] = 1
		pptp[4], pptp[5], pptp[6], pptp[7] = 0x1a, 0x2b, 0x3c, 0x4d
		pptp[9] = 2
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, pptp, "application-layer.pptp", "PPTP").Child("ControlMessageType")))
	})

	t.Run("ike/sa-init", func(t *testing.T) {
		ike := mustHex(t, "88694881497528ad0000000000000000212022080000000000000048"+
			"2200001400000010010100010000000802000002"+
			"28000010000200000102030405060708"+
			"00000008aabbccdd")
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, ike, "ike", "IKE"), "Payloads").Children()[0].Child("Proposals").Children()[0].Child("Protocol ID")))
	})
	t.Run("ike/ke-nonce", func(t *testing.T) {
		ike := mustHex(t, "88694881497528ad0000000000000000212022080000000000000048"+
			"2200001400000010010100010000000802000002"+
			"28000010000200000102030405060708"+
			"00000008aabbccdd")
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, ike, "ike", "IKE").Child("Payloads").Children()[1].Child("DH Group")))
	})

	t.Run("wireguard/init", func(t *testing.T) {
		wg := make([]byte, 148)
		wg[0] = 1
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, wg, "wireguard", "WireGuard").Child("Type")))
	})
	t.Run("wireguard/response", func(t *testing.T) {
		wg := make([]byte, 92)
		wg[0] = 2
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, wg, "wireguard", "WireGuard").Child("Type")))
	})

	t.Run("bgp/keepalive", func(t *testing.T) {
		bgp := mustHex(t, "ffffffffffffffffffffffffffffffff001304")
		require.Equal(t, uint64(4), uintVal(t, parseRule(t, bgp, "bgp", "BGP").Child("Type")))
	})
	t.Run("bgp/notification", func(t *testing.T) {
		bgp := append(bytesRepeat(0xff, 16), 0x00, 0x15, 0x03, 0x06, 0x00)
		require.Equal(t, uint64(3), uintVal(t, parseRule(t, bgp, "bgp", "BGP").Child("Type")))
	})

	t.Run("nbt_dg/direct", func(t *testing.T) {
		nb := make([]byte, 14)
		nb[0] = 0x10
		require.Equal(t, uint64(0x10), uintVal(t, parseRule(t, nb, "nbt_dg", "NBTDG").Child("Message Type")))
	})
	t.Run("nbt_dg/broadcast", func(t *testing.T) {
		nb := make([]byte, 14)
		nb[0] = 0x11
		require.Equal(t, uint64(0x11), uintVal(t, parseRule(t, nb, "nbt_dg", "NBTDG").Child("Message Type")))
	})

	t.Run("pop3/ok", func(t *testing.T) {
		require.Equal(t, "+OK", strVal(t, parseRule(t, []byte("+OK ready\r\n"), "pop3", "POP3").Child("Status")))
	})
	t.Run("pop3/err", func(t *testing.T) {
		require.Equal(t, "-ERR", strVal(t, parseRule(t, []byte("-ERR no\r\n"), "pop3", "POP3").Child("Status")))
	})

	t.Run("imap/untagged", func(t *testing.T) {
		require.Equal(t, "OK", strVal(t, parseRule(t, []byte("* OK IMAP4rev1 ready\r\n"), "imap", "IMAP").Child("Command")))
	})
	t.Run("imap/tagged", func(t *testing.T) {
		require.Equal(t, "OK", strVal(t, parseRule(t, []byte("a1 OK done\r\n"), "imap", "IMAP").Child("Command")))
	})

	t.Run("tftp/rrq", func(t *testing.T) {
		rrq := append([]byte{0x00, 0x01}, []byte("file\x00octet\x00")...)
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, rrq, "tftp", "TFTP").Child("Opcode")))
	})
	t.Run("tftp/error", func(t *testing.T) {
		errp := append([]byte{0x00, 0x05, 0x00, 0x01}, []byte("file not found\x00")...)
		require.Equal(t, uint64(5), uintVal(t, parseRule(t, errp, "tftp", "TFTP").Child("Opcode")))
	})

	t.Run("onc_rpc/call", func(t *testing.T) {
		rpc := make([]byte, 40)
		rpc[7] = 0
		rpc[11] = 2
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, rpc, "onc_rpc", "ONCRPC").Child("RPC Version")))
	})
	t.Run("onc_rpc/reply", func(t *testing.T) {
		rpc := make([]byte, 40)
		rpc[7] = 1
		rpc[11] = 2
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, rpc, "onc_rpc", "ONCRPC").Child("Message Type")))
	})

	t.Run("tacacs/authen", func(t *testing.T) {
		tac := make([]byte, 12)
		tac[0], tac[1] = 0xc0, 1
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, tac, "tacacs", "TACACS").Child("Type")))
	})
	t.Run("tacacs/author", func(t *testing.T) {
		tac := make([]byte, 12)
		tac[0], tac[1] = 0xc0, 2
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, tac, "tacacs", "TACACS").Child("Type")))
	})

	t.Run("mongodb/query", func(t *testing.T) {
		mongo := mustHex(t, "360000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"0f0000001070696e67000100000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		require.NotNil(t, mustChild(t, eth, "IP", "TCP", "MongoDB").Child("Op Code"))
	})
	t.Run("mongodb/reply-id", func(t *testing.T) {
		mongo := mustHex(t, "360000000200000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"0f0000001070696e67000100000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "MongoDB").Child("Request ID")))
	})

	t.Run("memcached/get", func(t *testing.T) {
		mc := []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}
		require.Equal(t, uint64(0x80), uintVal(t, parseRule(t, mc, "memcached", "Memcached").Child("Magic")))
	})
	t.Run("memcached/set", func(t *testing.T) {
		mc := []byte{0x80, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, mc, "memcached", "Memcached").Child("Opcode")))
	})

	t.Run("amqp/header", func(t *testing.T) {
		require.Equal(t, uint64('A'), uintVal(t, parseRule(t, []byte{'A', 'M', 'Q', 'P', 0x00, 0x00, 0x09, 0x01}, "amqp", "AMQP").Child("Type")))
	})
	t.Run("amqp/frame", func(t *testing.T) {
		fr := []byte{1, 0, 0, 0, 0, 0, 0, 0xce}
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, fr, "amqp", "AMQP").Child("Type")))
	})

	t.Run("thrift/binary", func(t *testing.T) {
		th := []byte{0x80, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0, 0}
		require.Equal(t, uint64(0x80010001), uintVal(t, parseRule(t, th, "thrift", "Thrift").Child("Version")))
	})
	t.Run("thrift/call", func(t *testing.T) {
		th := []byte{0x80, 0x01, 0x00, 01, 0, 0, 0, 0, 0, 0, 0, 1}
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, th, "thrift", "Thrift").Child("Seq ID")))
	})

	t.Run("rtmp/c0c1", func(t *testing.T) {
		rtmp := mustHex(t, "03000000010000000072616e64")
		require.Equal(t, uint64(3), uintVal(t, parseRule(t, rtmp, "rtmp", "RTMP").Child("Version")))
	})
	t.Run("rtmp/time", func(t *testing.T) {
		rtmp := mustHex(t, "03000000020000000072616e64")
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, parseRule(t, rtmp, "rtmp", "RTMP"), "C1").Child("Time")))
	})

	t.Run("jdwp/handshake", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001000101")...)
		require.Equal(t, "JDWP-Handshake", string(bytesVal(t, parseRule(t, jd, "jdwp", "JDWP").Child("Handshake"))))
	})
	t.Run("jdwp/command", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000002000107")...)
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, parseRule(t, jd, "jdwp", "JDWP"), "Command").Child("Command")))
	})

	t.Run("fastcgi/begin", func(t *testing.T) {
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, []byte{1, 1, 0, 1, 0, 0, 0, 0}, "fastcgi", "FastCGI").Child("Type")))
	})
	t.Run("fastcgi/params", func(t *testing.T) {
		require.Equal(t, uint64(4), uintVal(t, parseRule(t, []byte{1, 4, 0, 1, 0, 0, 0, 0}, "fastcgi", "FastCGI").Child("Type")))
	})

	t.Run("zabbix/data", func(t *testing.T) {
		zb := append([]byte("ZBXD"), 0x01, 0x02, 0x00, 0x00, 0x00, '{', '}')
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, zb, "zabbix", "Zabbix").Child("Length")))
	})
	t.Run("zabbix/flags", func(t *testing.T) {
		zb := append([]byte("ZBXD"), 0x03, 0x00, 0x00, 0x00, 0x00)
		require.Equal(t, uint64(3), uintVal(t, parseRule(t, zb, "zabbix", "Zabbix").Child("Flags")))
	})

	t.Run("net_remoting/preamble", func(t *testing.T) {
		nr := make([]byte, 14)
		copy(nr, []byte(".NET"))
		nr[5] = 1
		n := parseRule(t, nr, "net_remoting", "NetRemoting")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Major")))
	})
	t.Run("net_remoting/minor", func(t *testing.T) {
		nr := make([]byte, 14)
		copy(nr, []byte(".NET"))
		nr[5] = 1
		nr[7] = 1
		n := parseRule(t, nr, "net_remoting", "NetRemoting")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Minor")))
	})

	t.Run("rsync/v31", func(t *testing.T) {
		require.Equal(t, "31", strVal(t, parseRule(t, []byte("@RSYNCD: 31.0\n"), "rsync", "Rsync").Child("Major")))
	})
	t.Run("rsync/v30", func(t *testing.T) {
		require.Equal(t, "30", strVal(t, parseRule(t, []byte("@RSYNCD: 30.0\n"), "rsync", "Rsync").Child("Major")))
	})
}
