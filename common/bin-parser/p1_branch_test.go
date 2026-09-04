package bin_parser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1BranchRows(t *testing.T) {
	t.Run("ieee_802_1ad/arp", func(t *testing.T) {
		q := parseRule(t, append([]byte{0xb0, 0x64, 0x08, 0x06}, []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}...), "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(5), uintVal(t, q.Child("PCP")))
		require.Equal(t, uint64(1), uintVal(t, q.Child("DEI")))
		require.Equal(t, uint64(100), vlanVID(t, q))
		require.Equal(t, uint64(0x0806), uintVal(t, q.Child("Type")))
	})
	t.Run("ieee_802_1ad/vlan", func(t *testing.T) {
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		q := parseRule(t, append([]byte{0x00, 0x64, 0x81, 0x00, 0x60, 0xc8, 0x08, 0x06}, arp...), "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(0x8100), uintVal(t, q.Child("Type")))
		require.Equal(t, uint64(3), uintVal(t, mustChild(t, q, "CTag").Child("PCP")))
		require.Equal(t, uint64(200), vlanVID(t, mustChild(t, q, "CTag")))
	})

	t.Run("stun/binding-req", func(t *testing.T) {
		stun := mustHex(t, "000100582112a442b7e7a701bc34d686fa87dfae802200105354554e207465737420636c69656e74002400046e0001ff80290008932ff9b151263b36000600096576746a3a68367659202020000800149aeaa70cbfd8cb56781ef2b5b2d3f249c1b571a280280004e57a3bcf")
		n := parseRule(t, stun, "stun", "STUN")
		require.Equal(t, "STUN test client", strVal(t, n.Child("Attributes").Children()[0].Child("Software")))
	})
	t.Run("stun/binding-success", func(t *testing.T) {
		succ := mustHex(t, "0101000c2112a442b7e7a701bc34d686fa87dfae002000080001a147e112a643")
		n := parseRule(t, succ, "stun", "STUN")
		require.Equal(t, uint64(0xa147), uintVal(t, n.Child("Attributes").Children()[0].Child("X-Port")))
	})
	t.Run("bittorrent/handshake", func(t *testing.T) {
		n := parseRule(t, btHandshake(), "bittorrent", "BitTorrent")
		require.Equal(t, uint64(19), uintVal(t, n.Child("Pstrlen")))
		require.Equal(t, []byte("-UT2210-abcdefghijkl"), bytesVal(t, n.Child("Peer ID")))
	})
	t.Run("bittorrent/have", func(t *testing.T) {
		raw := append(btHandshake(), 0, 0, 0, 5, 4, 0, 0, 0, 7)
		n := parseRule(t, raw, "bittorrent", "BitTorrent")
		msg := n.Child("Messages").Children()[0]
		require.Equal(t, uint64(4), uintVal(t, msg.Child("Message ID")))
		require.Equal(t, uint64(7), uintVal(t, msg.Child("Piece Index")))
	})
	t.Run("sdp/origin", func(t *testing.T) {
		n := parseRule(t, []byte("v=0\r\no=alice 2890844526 2890844526 IN IP4 pc33.atlanta.example.com\r\n"), "sdp", "SDP")
		require.Equal(t, "alice", strVal(t, n.Child("Username")))
		require.Equal(t, "pc33.atlanta.example.com", strVal(t, n.Child("Address")))
	})
	t.Run("sdp/media", func(t *testing.T) {
		raw := []byte("v=0\r\no=jdoe 2890844526 2890842807 IN IP4 10.47.16.5\r\ns=SDP Seminar\r\nm=audio 49170 RTP/AVP 0\r\n")
		n := parseRule(t, raw, "sdp", "SDP")
		lines := n.Child("Lines").Children()
		require.GreaterOrEqual(t, len(lines), 2)
		require.Equal(t, "SDP Seminar", strVal(t, lines[0].Child("Session Name")))
		require.Equal(t, "audio", strVal(t, lines[1].Child("Media")))
		require.Equal(t, "49170", strVal(t, lines[1].Child("Port")))
	})
	t.Run("sip/invite", func(t *testing.T) {
		sdp := "v=0\r\no=- 0 0 IN IP4 10.0.0.1\r\n"
		inv := "INVITE sip:bob@example.com SIP/2.0\r\nContent-Type: application/sdp\r\nContent-Length: 30\r\n\r\n" + sdp
		n := parseRule(t, []byte(inv), "sip", "SIP")
		require.Equal(t, "INVITE", strVal(t, mustChild(t, n, "SIP Request").Child("Method")))
		require.Equal(t, uint64('v'), uintVal(t, mustChild(t, n, "SIP Request", "Body", "SDPSession").Child("Type")))
	})
	t.Run("sip/200", func(t *testing.T) {
		ok := "SIP/2.0 200 OK\r\nContent-Length: 0\r\n\r\n"
		n := parseRule(t, []byte(ok), "sip", "SIP")
		require.Equal(t, "200", strVal(t, mustChild(t, n, "SIP Response").Child("Status")))
	})
	t.Run("internet_control_message_protocol/echo", func(t *testing.T) {
		echo := append([]byte{0x08, 0x00, 0x00, 0x00, 0x12, 0x34, 0x00, 0x01}, []byte("abcd")...)
		require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, parseRule(t, echo, "internet_control_message_protocol", "ICMP"), "ICMP Echo").Child("Identifier")))
		require.Equal(t, "abcd", strVal(t, mustChild(t, parseEthernet(t, ipv4ProtoFrame(t, 1, echo)), "IP", "ICMP", "ICMP Echo").Child("Echo Data")))
	})
	t.Run("internet_control_message_protocol/echo-reply", func(t *testing.T) {
		rep := append([]byte{0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x00, 0x01}, []byte("abcd")...)
		require.Equal(t, uint64(0), uintVal(t, parseRule(t, rep, "internet_control_message_protocol", "ICMP").Child("Type")))
		require.Equal(t, "abcd", strVal(t, mustChild(t, parseEthernet(t, ipv4ProtoFrame(t, 1, rep)), "IP", "ICMP", "ICMP Echo Reply").Child("Echo Data")))
	})
	t.Run("internet_control_message_protocol_v6/echo", func(t *testing.T) {
		echo := append([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02}, []byte("ping6")...)
		require.Equal(t, uint64(128), uintVal(t, parseRule(t, echo, "internet_control_message_protocol_v6", "ICMPV6").Child("Type")))
		require.Equal(t, "ping6", strVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, echo)), "IPv6", "ICMPv6", "Echo Request").Child("Echo Data")))
	})
	t.Run("internet_control_message_protocol_v6/dest-unreach", func(t *testing.T) {
		du := append([]byte{0x01, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, []byte{0x60, 0x00, 0x00, 0x00}...)
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, du, "internet_control_message_protocol_v6", "ICMPV6").Child("Type")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, du)), "IPv6", "ICMPv6", "Destination Unreachable").Child("Unused")))
	})
	t.Run("internet_control_message_protocol_v6/packet-too-big", func(t *testing.T) {
		ptb := append([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05, 0x00}, []byte{0x60, 0x00, 0x00, 0x00}...)
		require.Equal(t, uint64(1280), uintVal(t, mustChild(t, parseRule(t, ptb, "internet_control_message_protocol_v6", "ICMPV6"), "Packet Too Big").Child("MTU")))
		require.Equal(t, uint64(1280), uintVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, ptb)), "IPv6", "ICMPv6", "Packet Too Big").Child("MTU")))
	})
	t.Run("internet_control_message_protocol_v6/time-exceeded", func(t *testing.T) {
		te := append([]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, []byte{0x60, 0x00, 0x00, 0x00}...)
		require.Equal(t, uint64(3), uintVal(t, parseRule(t, te, "internet_control_message_protocol_v6", "ICMPV6").Child("Type")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, te)), "IPv6", "ICMPv6", "Time Exceeded").Child("Unused")))
	})
	t.Run("internet_control_message_protocol_v6/param-problem", func(t *testing.T) {
		pp := append([]byte{0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06}, []byte{0x60, 0x00, 0x00, 0x00}...)
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, parseRule(t, pp, "internet_control_message_protocol_v6", "ICMPV6"), "Parameter Problem").Child("Pointer")))
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, pp)), "IPv6", "ICMPv6", "Parameter Problem").Child("Pointer")))
	})
	t.Run("internet_control_message_protocol_v6/mld-query", func(t *testing.T) {
		unspec := make([]byte, 16)
		q := append([]byte{0x82, 0x00, 0x00, 0x00, 0x27, 0x10, 0x00, 0x00}, unspec...)
		require.Equal(t, uint64(10000), uintVal(t, mustChild(t, parseRule(t, q, "internet_control_message_protocol_v6", "ICMPV6"), "Multicast Listener Query").Child("Maximum Response Delay")))
		require.Equal(t, unspec, bytesVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, q)), "IPv6", "ICMPv6", "Multicast Listener Query").Child("Multicast Address")))
	})
	t.Run("internet_control_message_protocol_v6/redirect", func(t *testing.T) {
		target := []byte{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		dest := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		rd := append(append([]byte{0x89, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, target...), dest...)
		require.Equal(t, target, bytesVal(t, mustChild(t, parseRule(t, rd, "internet_control_message_protocol_v6", "ICMPV6"), "Redirect").Child("Target Address")))
		require.Equal(t, dest, bytesVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, rd)), "IPv6", "ICMPv6", "Redirect").Child("Destination Address")))
	})
	t.Run("internet_control_message_protocol_v6/mldv2-report", func(t *testing.T) {
		group := []byte{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0xff, 0, 0, 0x01}
		r := append([]byte{0x8f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00}, group...)
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, parseRule(t, r, "internet_control_message_protocol_v6", "ICMPV6"), "Multicast Listener Report v2").Child("Records").Children()[0].Child("Record Type")))
		require.Equal(t, group, bytesVal(t, mustChild(t, parseEthernet(t, ipv6ICMPBytes(t, r)), "IPv6", "ICMPv6", "Multicast Listener Report v2").Child("Records").Children()[0].Child("Multicast Address")))
	})
	t.Run("internet_control_message_protocol_v6/ra-opt", func(t *testing.T) {
		require.Equal(t, uint64(64), uintVal(t, mustChild(t, parseEthernet(t, []byte{
			0x33, 0x33, 0x00, 0x00, 0x00, 0x01, 0xc2, 0x00, 0x54, 0xf5, 0x00, 0x00, 0x86, 0xdd, 0x6e, 0x00,
			0x00, 0x00, 0x00, 0x40, 0x3a, 0xff, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x00,
			0x54, 0xff, 0xfe, 0xf5, 0x00, 0x00, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x86, 0x00, 0xc4, 0xfe, 0x40, 0x00, 0x07, 0x08, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0xc2, 0x00, 0x54, 0xf5, 0x00, 0x00, 0x05, 0x01,
			0x00, 0x00, 0x00, 0x00, 0x05, 0xdc, 0x03, 0x04, 0x40, 0xc0, 0x00, 0x27, 0x8d, 0x00, 0x00, 0x09,
			0x3a, 0x80, 0x00, 0x00, 0x00, 0x00, 0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}), "IPv6", "ICMPv6", "Router Advertisement").Child("Hop Limit")))
	})
	t.Run("link_control_protocol/conf-req", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "0101000e0304c02305060f3f117c"), "link_control_protocol", "LCP")
		require.Equal(t, uint64(0xc023), uintVal(t, n.Child("Options").Children()[0].Child("Auth Protocol")))
	})
	t.Run("link_control_protocol/echo", func(t *testing.T) {
		n := parseRule(t, []byte{0x09, 0x01, 0x00, 0x08, 0x12, 0x34, 0x56, 0x78}, "link_control_protocol", "LCP")
		require.Equal(t, uint64(0x12345678), uintVal(t, mustChild(t, n, "Echo").Child("Magic Number")))
	})
	t.Run("ppp/lcp", func(t *testing.T) {
		p := parseRule(t, []byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x01, 0x00, 0x0e, 0x03, 0x04, 0xc0, 0x23, 0x05, 0x06, 0x0f, 0x3f, 0x11, 0x7c}, "ppp", "PPP")
		require.Equal(t, uint64(0xc021), uintVal(t, p.Child("Protocol")))
	})
	t.Run("password_authentication_protocol/request", func(t *testing.T) {
		n := parseRule(t, []byte{0x01, 0x00, 0x00, 0x0e, 0x04, 'i', 'x', 'i', 'a', 0x04, 'i', 'x', 'i', 'a'}, "password_authentication_protocol", "PAP")
		require.Equal(t, "ixia", strVal(t, mustChild(t, n, "Request").Child("Peer ID")))
	})
	t.Run("password_authentication_protocol/ack", func(t *testing.T) {
		n := parseRule(t, []byte{0x02, 0x00, 0x00, 0x07, 0x02, 'O', 'K'}, "password_authentication_protocol", "PAP")
		require.Equal(t, "OK", strVal(t, mustChild(t, n, "Response").Child("Message")))
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
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, ch, "CHAPResponse").Child("Name")))
	})

	t.Run("eapol/eap", func(t *testing.T) {
		ep := parseRule(t, []byte{0x01, 0x00, 0x00, 0x05, 0x01, 0x01, 0x00, 0x05, 0x01}, "eapol", "EAPOL")
		require.Equal(t, uint64(0), uintVal(t, ep.Child("Packet Type")))
	})
	t.Run("eapol/start", func(t *testing.T) {
		ep := parseRule(t, []byte{0x01, 0x01, 0x00, 0x00}, "eapol", "EAPOL")
		require.Equal(t, uint64(1), uintVal(t, ep.Child("Packet Type")))
	})

	t.Run("syslog/bsd", func(t *testing.T) {
		n := parseRule(t, []byte("<13>Sep  4 12:00:00 host sshd: ok\n"), "syslog", "Syslog")
		require.Equal(t, "13", strVal(t, n.Child("PRI")))
		require.Equal(t, "host", strVal(t, n.Child("Hostname")))
	})
	t.Run("syslog/rfc5424", func(t *testing.T) {
		raw := []byte("<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - 'su root' failed for lonvick on /dev/pts/8\n")
		n := parseRule(t, raw, "syslog", "Syslog")
		require.Equal(t, "1", strVal(t, n.Child("Version")))
		require.Equal(t, "ID47", strVal(t, n.Child("MsgID")))
	})
	t.Run("vnc/version", func(t *testing.T) {
		n := parseRule(t, []byte("RFB 003.008\n"), "vnc", "VNC")
		require.Equal(t, "003", strVal(t, n.Child("Major")))
		require.Equal(t, "008", strVal(t, n.Child("Minor")))
	})
	t.Run("vnc/security", func(t *testing.T) {
		raw := append([]byte("RFB 003.008\n"), 0x02, 0x01, 0x02)
		n := parseRule(t, raw, "vnc", "VNC")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Number of Security Types")))
		types := n.Child("Security Types").Children()
		require.Equal(t, 2, len(types))
		require.Equal(t, uint64(1), uintVal(t, types[0]))
		require.Equal(t, uint64(2), uintVal(t, types[1]))
	})
	t.Run("telnet/do-echo", func(t *testing.T) {
		n := parseRule(t, []byte{0xff, 0xfd, 0x01}, "telnet", "Telnet")
		require.Equal(t, uint64(0xfd), uintVal(t, mustChild(t, n, "IAC").Child("Command")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "IAC").Child("Option")))
	})
	t.Run("telnet/ttype", func(t *testing.T) {
		raw := append([]byte{0xff, 0xfa, 0x18, 0x00}, append([]byte("xterm"), 0xff, 0xf0)...)
		n := parseRule(t, raw, "telnet", "Telnet")
		require.Equal(t, uint64(24), uintVal(t, mustChild(t, n, "IAC").Child("SB Option")))
		require.Equal(t, "xterm", joinUint8(t, mustChild(t, n, "IAC").Child("Terminal Type")))
	})
	t.Run("socks4/connect", func(t *testing.T) {
		n := parseRule(t, []byte{0x04, 0x01, 0x00, 0x50, 10, 0, 0, 1, 'u', 0}, "socks4", "SOCKS4")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Command")))
		require.Equal(t, "u", strVal(t, n.Child("UserID")))
	})
	t.Run("socks4/reply", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x5a, 0x00, 0x50, 10, 0, 0, 1}, "socks4", "SOCKS4")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(90), uintVal(t, n.Child("Command")))
		require.Nil(t, n.Child("UserID"))
	})
	t.Run("stp/config", func(t *testing.T) {
		n := parseRule(t, stpConfigBPDU(), "stp", "STP")
		require.Equal(t, uint64(0), uintVal(t, n.Child("BPDU Type")))
		require.Equal(t, uint64(8), uintVal(t, mustChild(t, n, "Config").Child("Root Priority")))
	})
	t.Run("stp/tcn", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x00, 0x00, 0x80}, "stp", "STP")
		require.Equal(t, uint64(0x80), uintVal(t, n.Child("BPDU Type")))
		require.Nil(t, n.Child("Config"))
	})
	t.Run("loopback/other", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, "loopback", "Loopback")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Functions").Children()[0].Child("Function")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Functions").Children()[0].Child("Reply").Child("Receipt Number")))
	})
	t.Run("loopback/custom", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x00, 0x02, 0x00, 0xaa, 0x00, 0x04, 0x00, 0x1d, 0x04}, "loopback", "Loopback")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Functions").Children()[0].Child("Function")))
		require.Equal(t, []byte{0xaa, 0x00, 0x04, 0x00, 0x1d, 0x04}, bytesVal(t, n.Child("Functions").Children()[0].Child("Forward").Child("Forwarding Address")))
	})

	t.Run("ssh/kexinit", func(t *testing.T) {
		kex := mustChild(t, parseRule(t, mustHex(t, "000000950414000102030405060708090a0b0c0d0e0f00000011637572766532353531392d7368613235360000000b7373682d656432353531390000000a6165733132382d6374720000000a6165733132382d6374720000000d686d61632d736861322d3235360000000d686d61632d736861322d323536000000046e6f6e65000000046e6f6e650000000000000000000000000000000000"), "application-layer.ssh", "SSHPacket"), "Payload", "SSHKexInit")
		require.Equal(t, "curve25519-sha256", strVal(t, kex.Child("Kex Algos")))
	})
	t.Run("ssh/kexdh", func(t *testing.T) {
		dh := mustChild(t, parseRule(t, mustHex(t, "0000000b041e000000010200000000"), "application-layer.ssh", "SSHPacket"), "Payload", "SSHKexDHInit")
		require.Equal(t, uint64(1), uintVal(t, dh.Child("E Length")))
	})
	t.Run("dhcp/discover", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "010106001234567800000000"+
			"00000000000000000000000000000000"+
			"123456789abc00000000000000000000"+
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"638253633501010c0b6578616d706c652e636f6dff"), "application-layer.dhcp", "DHCP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Options").Children()[0].Child("Message Type")))
		require.Equal(t, "example.com", strVal(t, n.Child("Options").Children()[1].Child("Host Name")))
	})
	t.Run("dhcp/offer", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "02010600aabbccdd00000000"+
			"00000000c0a8007bc0a8000100000000"+
			"123456789abc00000000000000000000"+
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"638253633501023604c0a80001ff"), "application-layer.dhcp", "DHCP")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Options").Children()[0].Child("Message Type")))
		require.Equal(t, []byte{192, 168, 0, 1}, bytesVal(t, n.Child("Options").Children()[1].Child("Server ID")))
	})
	t.Run("dhcpv6/solicit", func(t *testing.T) {
		sol := mustHex(t, "010000010001000a000300010000000000010008000200000003000c0000000100000000000000000006000400170018")
		n := parseRule(t, sol, "dhcpv6", "DHCPv6")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Message Type")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Options").Children()[1].Child("Elapsed Time")))
	})
	t.Run("dhcpv6/reply", func(t *testing.T) {
		rep := mustHex(t, "070000010001000a000300010000000000010002000a00030001000000000002000300280000000100000e10000015180005001820010db800000000000000000000000100000e1000001c200017001020010db8000000000000000000000053")
		n := parseRule(t, rep, "dhcpv6", "DHCPv6")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Message Type")))
		require.Equal(t, uint64(3600), uintVal(t, n.Child("Options").Children()[2].Child("T1")))
	})
	t.Run("ftp_data/stream", func(t *testing.T) {
		require.Equal(t, "abc", strVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 20, 20, []byte("abc"))), "IP", "TCP", "FTPData").Child("File Data")))
	})
	t.Run("ftp_data/eor", func(t *testing.T) {
		blk := mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 20, 20, []byte{0x80, 0x00, 0x03, 'a', 'b', 'c'})), "IP", "TCP", "FTPData", "Blocks").Children()[0]
		require.Equal(t, uint64(0x80), uintVal(t, blk.Child("Descriptor")))
	})
	t.Run("ftp_data/eof", func(t *testing.T) {
		blk := mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 20, 20, []byte{0x40, 0x00, 0x03, 'a', 'b', 'c'})), "IP", "TCP", "FTPData", "Blocks").Children()[0]
		require.Equal(t, uint64(0x40), uintVal(t, blk.Child("Descriptor")))
	})

	t.Run("l2tp/flags", func(t *testing.T) {
		l := parseRule(t, mustHex(t, "02020014000100020000ff03002d"), "l2tp", "L2TP")
		require.Equal(t, uint64(0x0014), uintVal(t, l.Child("Tunnel ID")))
		require.Equal(t, uint64(2), uintVal(t, l.Child("Offset Size")))
		require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, l, "PPP").Child("Protocol")))
	})
	t.Run("l2tp/session", func(t *testing.T) {
		l := parseRule(t, mustHex(t, "02020014000400020000ff03002d"), "l2tp", "L2TP")
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
	t.Run("pptp/echo", func(t *testing.T) {
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 5)
		binary.BigEndian.PutUint32(pptp[12:], 1)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Echo Request").Child("Identifier")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Echo Request").Child("Identifier")))
	})
	t.Run("pptp/echo-reply", func(t *testing.T) {
		pptp := make([]byte, 20)
		binary.BigEndian.PutUint16(pptp[0:], 20)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 6)
		binary.BigEndian.PutUint32(pptp[12:], 1)
		pptp[16] = 1
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Echo Reply").Child("ResultCode")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Echo Reply").Child("Identifier")))
	})
	t.Run("pptp/stop-req", func(t *testing.T) {
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 3)
		pptp[12] = 1
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Stop Control Conn Req").Child("Reason")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Stop Control Conn Req").Child("Reason")))
	})
	t.Run("pptp/stop-reply", func(t *testing.T) {
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 4)
		pptp[12] = 1
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Stop Control Conn Reply").Child("ResultCode")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Stop Control Conn Reply").Child("ResultCode")))
	})
	t.Run("pptp/icrq", func(t *testing.T) {
		pptp := make([]byte, 220)
		binary.BigEndian.PutUint16(pptp[0:], 220)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 9)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint16(pptp[24:], 7)
		copy(pptp[28:], []byte("5551212"))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Incoming Call Req").Child("CallId")))
		require.True(t, strings.HasPrefix(strVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Incoming Call Req").Child("DialedNumber")), "5551212"))
	})
	t.Run("pptp/icrp", func(t *testing.T) {
		pptp := make([]byte, 28)
		binary.BigEndian.PutUint16(pptp[0:], 28)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 10)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint16(pptp[14:], 1)
		pptp[16] = 1
		binary.BigEndian.PutUint16(pptp[20:], 64)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Incoming Call Reply").Child("ResultCode")))
		require.Equal(t, uint64(64), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Incoming Call Reply").Child("RecvWindowSize")))
	})
	t.Run("pptp/icc", func(t *testing.T) {
		pptp := make([]byte, 28)
		binary.BigEndian.PutUint16(pptp[0:], 28)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 11)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint32(pptp[16:], 64000)
		binary.BigEndian.PutUint16(pptp[20:], 64)
		binary.BigEndian.PutUint32(pptp[24:], 1)
		require.Equal(t, uint64(64000), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Incoming Call Connected").Child("ConnectSpeed")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Incoming Call Connected").Child("PeerCallId")))
	})
	t.Run("pptp/ccrq", func(t *testing.T) {
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 12)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Call Clear Req").Child("CallId")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Call Clear Req").Child("CallId")))
	})
	t.Run("pptp/set-link-info", func(t *testing.T) {
		pptp := make([]byte, 24)
		binary.BigEndian.PutUint16(pptp[0:], 24)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 15)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint32(pptp[16:], 0xffffffff)
		binary.BigEndian.PutUint32(pptp[20:], 0xffffffff)
		require.Equal(t, uint64(0xffffffff), uintVal(t, mustChild(t, parseRule(t, pptp, "application-layer.pptp", "PPTP"), "Set Link Info").Child("Send Accm")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp)), "IP", "TCP", "PPTP", "Set Link Info").Child("PeerCallId")))
	})

	t.Run("dtls/handshake", func(t *testing.T) {
		dt := mustHex(t, "16fefd000000000000000000360100002a000000000000002afefd"+
			"000000000000000000000000000000000000000000000000000000000000000000000002002f0100")
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, dt, "dtls", "DTLS").Child("Fragment").Child("Handshake Type")))
	})
	t.Run("dtls/client-hello", func(t *testing.T) {
		dt := mustHex(t, "16fefd000000000000000000360100002a000000000000002afefd"+
			"000000000000000000000000000000000000000000000000000000000000000000000002002f0100")
		require.Equal(t, uint64(0xfefd), uintVal(t, parseRule(t, dt, "dtls", "DTLS").Child("Fragment").Child("Client Version")))
	})
	t.Run("rtp/extension", func(t *testing.T) {
		require.Equal(t, uint64(0xbede), uintVal(t, parseRule(t, mustHex(t, "900000010000000200000003bede000110000000aa"), "rtp", "RTP").Child("Ext Profile")))
	})
	t.Run("rtp/seq", func(t *testing.T) {
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, []byte{0x80, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0xaa}, "rtp", "RTP").Child("Sequence")))
	})

	t.Run("rtcp/sr", func(t *testing.T) {
		sr := mustChild(t, parseRule(t, mustHex(t, "80c80006123456781112141822242628333435364445464755565758"), "rtp", "RTCP"), "RTCPSR")
		require.Equal(t, uint64(0x44454647), uintVal(t, sr.Child("Packet Count")))
		require.Equal(t, uint64(0x11121418), uintVal(t, sr.Child("NTP MSW")))
	})
	t.Run("rtcp/rr", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "80c9000112345678"), "rtp", "RTCP")
		require.Equal(t, uint64(201), uintVal(t, n.Child("Packet Type")))
		require.Nil(t, n.Child("RTCPSR"))
	})

	t.Run("websocket/text", func(t *testing.T) {
		n := parseRule(t, []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}, "application-layer.websocket", "WebSocket")
		require.Equal(t, "Hello", strVal(t, n.Child("Text")))
	})
	t.Run("websocket/close", func(t *testing.T) {
		n := parseRule(t, []byte{0x88, 0x02, 0x03, 0xe8}, "application-layer.websocket", "WebSocket")
		require.Equal(t, uint64(1000), uintVal(t, n.Child("Close Code")))
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
		wg[4] = 1
		n := parseRule(t, wg, "wireguard", "WireGuard")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "WGInit").Child("Sender")))
	})
	t.Run("wireguard/response", func(t *testing.T) {
		wg := make([]byte, 92)
		wg[0] = 2
		wg[4] = 2
		wg[8] = 1
		n := parseRule(t, wg, "wireguard", "WireGuard")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Type")))
		resp := mustChild(t, n, "WGResponse")
		require.Equal(t, uint64(2), uintVal(t, resp.Child("Sender")))
		require.Equal(t, uint64(1), uintVal(t, resp.Child("Receiver")))
	})
	t.Run("wireguard/cookie", func(t *testing.T) {
		wg := make([]byte, 64)
		wg[0] = 3
		wg[4] = 1
		n := parseRule(t, wg, "wireguard", "WireGuard")
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "WGCookie").Child("Receiver")))
	})
	t.Run("wireguard/transport", func(t *testing.T) {
		wg := make([]byte, 4+4+8+16)
		wg[0] = 4
		wg[4] = 1
		wg[8] = 7
		n := parseRule(t, wg, "wireguard", "WireGuard")
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, n, "WGTransport").Child("Counter")))
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
	t.Run("pop3/stat", func(t *testing.T) {
		st := mustChild(t, parseRule(t, []byte("+OK 2 320\r\n"), "pop3", "POP3"), "POP3Stat")
		require.Equal(t, "2", strVal(t, st.Child("Messages")))
		require.Equal(t, "320", strVal(t, st.Child("Octets")))
	})
	t.Run("pop3/list", func(t *testing.T) {
		scan := parseRule(t, []byte("+OK 2 messages (320 octets)\r\n1 120\r\n2 200\r\n.\r\n"), "pop3", "POP3").Child("POP3Extra").Children()
		require.Equal(t, "1", strVal(t, scan[0].Child("Number")))
		require.Equal(t, "120", strVal(t, scan[0].Child("Size")))
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
		n := parseRule(t, errp, "tftp", "TFTP")
		require.Equal(t, uint64(5), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Error Code")))
		require.Equal(t, "file not found", strVal(t, n.Child("Error Message")))
	})
	t.Run("tftp/blksize", func(t *testing.T) {
		raw := append([]byte{0x00, 0x01}, []byte("foo\x00octet\x00blksize\x00512\x00")...)
		opt := parseRule(t, raw, "tftp", "TFTP").Child("Options").Children()[0]
		require.Equal(t, "blksize", strVal(t, opt.Child("Name")))
		require.Equal(t, "512", strVal(t, opt.Child("Value")))
	})
	t.Run("tftp/oack", func(t *testing.T) {
		raw := append([]byte{0x00, 0x06}, []byte("tsize\x001234\x00")...)
		require.Equal(t, "tsize", strVal(t, parseRule(t, raw, "tftp", "TFTP").Child("Options").Children()[0].Child("Name")))
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

	t.Run("kafka/metadata", func(t *testing.T) {
		kf := mustHex(t, "000000170003000000000001000474657374000000010003666f6f")
		n := parseRule(t, kf, "kafka", "Kafka")
		require.Equal(t, "test", strVal(t, n.Child("Client ID")))
		require.Equal(t, "foo", strVal(t, n.Child("Topics").Children()[0].Child("Name")))
	})
	t.Run("kafka/apiversions", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "0000000a0012000000000000ffff"), "kafka", "Kafka")
		require.Equal(t, int64(18), intVal(t, n.Child("API Key")))
		require.Equal(t, int64(-1), intVal(t, n.Child("Client ID Len")))
		require.Nil(t, n.Child("Client ID"))
	})

	t.Run("protobuf/varint", func(t *testing.T) {
		f := parseRule(t, mustHex(t, "089601"), "protobuf", "Protobuf").Child("Fields").Children()[0]
		require.Equal(t, uint64(0x96), uintVal(t, f.Child("Varint")))
		require.Equal(t, uint64(0x01), uintVal(t, f.Child("Varint2")))
	})
	t.Run("protobuf/string", func(t *testing.T) {
		f := parseRule(t, append([]byte{0x12, 0x07}, []byte("testing")...), "protobuf", "Protobuf").Child("Fields").Children()[0]
		require.Equal(t, "testing", strVal(t, f.Child("Str")))
	})

	t.Run("iiop/locate", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "47494f50010200030000001700000002000000000000000b4e616d6553657276696365"), "application-layer.iiop", "GIOP")
		require.Equal(t, "NameService", strVal(t, mustChild(t, n, "LocateRequest").Child("Object Key")))
	})
	t.Run("iiop/request", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "47494f50010200000000001a00000001030000000000000000000000000000065f69735f6100"), "application-layer.iiop", "GIOP")
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, n, "GIOPRequest").Child("Op Len")))
	})

	t.Run("t3/hello", func(t *testing.T) {
		h := mustChild(t, parseRule(t, []byte("t3 12.2.1\nAS:255\nHL:19\nMS:10000000\n\n"), "application-layer.t3", "T3"), "T3Hello")
		require.Equal(t, "12.2.1", strVal(t, h.Child("Version")))
		require.Equal(t, "255", strVal(t, h.Child("AS")))
	})
	t.Run("t3/identify", func(t *testing.T) {
		pkt := make([]byte, 19)
		pkt[3], pkt[4], pkt[5] = 19, 1, 0x65
		id := mustChild(t, parseRule(t, pkt, "application-layer.t3", "T3"), "T3Identify")
		require.Equal(t, uint64(1), uintVal(t, id.Child("Cmd")))
		require.Equal(t, uint64(0x65), uintVal(t, id.Child("Qos")))
	})

	t.Run("quic/initial", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "c000000001088394c8f03e51570800"), "application-layer.quic", "QUIC")
		require.Equal(t, []byte{0x83, 0x94, 0xc8, 0xf0, 0x3e, 0x51, 0x57, 0x08}, bytesVal(t, n.Child("DCID")))
	})
	t.Run("quic/server-initial", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "cf000000010008f067a5502a4262b500"), "application-layer.quic", "QUIC")
		require.Equal(t, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, bytesVal(t, n.Child("SCID")))
	})

	t.Run("ospf/hello", func(t *testing.T) {
		h := mustChild(t, parseRule(t, mustHex(t, "0201002cc0a8aa0800000001273b00000000000000000000ffffff00000a020100000028c0a8aa0800000000"), "ospf", "OSPF"), "OSPFHello")
		require.Equal(t, uint64(10), uintVal(t, h.Child("Hello Interval")))
		require.Equal(t, uint64(40), uintVal(t, h.Child("Dead Interval")))
	})
	t.Run("ospf/dbd", func(t *testing.T) {
		d := mustChild(t, parseRule(t, mustHex(t, "02020020c0a800010000000000000000000000000000000005dc020700000001"), "ospf", "OSPF"), "OSPFDBDesc")
		require.Equal(t, uint64(1500), uintVal(t, d.Child("Interface MTU")))
		require.Equal(t, uint64(0x07), uintVal(t, d.Child("Flags")))
	})

	t.Run("salt/zmtp", func(t *testing.T) {
		g := make([]byte, 64)
		g[0], g[9], g[10] = 0xff, 0x7f, 3
		copy(g[12:16], []byte("NULL"))
		require.Equal(t, uint64(3), uintVal(t, mustChild(t, parseRule(t, g, "salt", "Salt"), "SaltGreeting").Child("Major")))
	})
	t.Run("salt/ping", func(t *testing.T) {
		fr := parseRule(t, []byte{0x00, 0x00, 0x00, 0x04, 'p', 'i', 'n', 'g'}, "salt", "Salt").Child("Frames").Children()[0]
		require.Equal(t, "ping", strVal(t, fr.Child("Command")))
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
	t.Run("rtmp/chunk-size", func(t *testing.T) {
		raw := append(rtmpC0C1(), mustHex(t, "02000000000004010000000000000080")...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, raw))
		require.Equal(t, uint64(128), uintVal(t, mustChild(t, eth, "IP", "TCP", "RTMP").Child("Chunks").Children()[0].Child("Chunk Size")))
	})
	t.Run("rtmp/connect", func(t *testing.T) {
		raw := append(rtmpC0C1(), mustHex(t, "0300000000000a1400000000020007636f6e6e656374")...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, raw))
		ch := mustChild(t, eth, "IP", "TCP", "RTMP").Child("Chunks").Children()[0]
		require.Equal(t, "connect", strVal(t, mustChild(t, ch, "AMF0").Child("Command Name")))
	})

	t.Run("jdwp/handshake", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001000101")...)
		require.Equal(t, "JDWP-Handshake", string(bytesVal(t, parseRule(t, jd, "jdwp", "JDWP").Child("Handshake"))))
	})
	t.Run("jdwp/command", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000002000107")...)
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, parseRule(t, jd, "jdwp", "JDWP"), "Command").Child("Command")))
	})
	t.Run("jdwp/createstring", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "000000140000000200010b0000000568656c6c6f")...)
		require.Equal(t, "hello", strVal(t, mustChild(t, parseRule(t, jd, "jdwp", "JDWP"), "Command").Child("String")))
	})
	t.Run("jdwp/reply", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001800000")...)
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, parseRule(t, jd, "jdwp", "JDWP"), "Command").Child("Error Code")))
	})

	t.Run("fastcgi/begin", func(t *testing.T) {
		require.Equal(t, uint64(1), uintVal(t, parseRule(t, []byte{1, 1, 0, 1, 0, 0, 0, 0}, "fastcgi", "FastCGI").Child("Type")))
	})
	t.Run("fastcgi/params", func(t *testing.T) {
		require.Equal(t, uint64(4), uintVal(t, parseRule(t, []byte{1, 4, 0, 1, 0, 0, 0, 0}, "fastcgi", "FastCGI").Child("Type")))
	})

	t.Run("php_ser/int", func(t *testing.T) {
		n := parseRule(t, []byte("i:12;"), "php_ser", "PHPSer")
		require.Equal(t, uint64('i'), uintVal(t, n.Child("Kind")))
		require.Equal(t, "12", strVal(t, n.Child("Int")))
	})
	t.Run("php_ser/string", func(t *testing.T) {
		n := parseRule(t, []byte("s:5:\"hello\";"), "php_ser", "PHPSer")
		require.Equal(t, "hello", strVal(t, n.Child("String")))
	})
	t.Run("zabbix/data", func(t *testing.T) {
		zb := zabbixPacket(0x01, []byte("{}"), true)
		require.Equal(t, uint64(2), uintVal(t, parseRule(t, zb, "zabbix", "Zabbix").Child("Length")))
	})
	t.Run("zabbix/flags", func(t *testing.T) {
		zb := zabbixPacket(0x03, nil, true)
		n := parseRule(t, zb, "zabbix", "Zabbix")
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Flags").Child("Protocol")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Flags").Child("Compression")))
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

	t.Run("mpls/bottom", func(t *testing.T) {
		raw := []byte{0x00, 0x01, 0x31, 0xfe}
		n := parseRule(t, raw, "mpls", "MPLS")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Labels").Children()[0].Child("Bottom")))
	})
	t.Run("mpls/stack", func(t *testing.T) {
		raw := []byte{0x00, 0x01, 0x10, 0xfe, 0x00, 0x01, 0x31, 0xfe}
		labs := parseRule(t, raw, "mpls", "MPLS").Child("Labels").Children()
		require.Equal(t, uint64(0), uintVal(t, labs[0].Child("Bottom")))
		require.Equal(t, uint64(1), uintVal(t, labs[1].Child("Bottom")))
	})
	t.Run("vxlan/vni", func(t *testing.T) {
		innerMAC := []byte{0x00, 0x30, 0x88, 0x01, 0x00, 0x02, 0x00, 0x16, 0x3e, 0x37, 0xf6, 0x04}
		ip := []byte{0x45, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2, 0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
		raw := append(append([]byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x00}, innerMAC...), append([]byte{0x08, 0x00}, ip...)...)
		n := parseRule(t, raw, "vxlan", "VXLAN")
		require.Equal(t, uint64(255), uintVal(t, n.Child("VNI")))
	})
	t.Run("vxlan/arp", func(t *testing.T) {
		innerMAC := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append(append([]byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00}, innerMAC...), append([]byte{0x08, 0x06}, arp...)...)
		n := parseRule(t, raw, "vxlan", "VXLAN")
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Inner", "ARP").Child("Opcode")))
	})
	t.Run("rmi/ping", func(t *testing.T) {
		n := parseRule(t, []byte{'J', 'R', 'M', 'I', 0x00, 0x02, 0x4b, 0x52}, "rmi", "RMI")
		require.Equal(t, uint64(0x52), uintVal(t, mustChild(t, n, "Message").Child("Type")))
	})
	t.Run("rmi/call", func(t *testing.T) {
		n := parseRule(t, []byte{'J', 'R', 'M', 'I', 0x00, 0x02, 0x4c, 0x50, 0xac, 0xed, 0x00, 0x05}, "rmi", "RMI")
		require.Equal(t, uint64(0x4c), uintVal(t, n.Child("Protocol")))
		require.Equal(t, uint64(0xaced), uintVal(t, mustChild(t, n, "Message").Child("Ser Magic")))
	})
	t.Run("llc/stp", func(t *testing.T) {
		n := parseRule(t, []byte{0x42, 0x42, 0x03, 0x00, 0x00, 0x00, 0x80}, "llc", "LLC")
		require.Equal(t, uint64(0x42), uintVal(t, n.Child("DSAP")))
		require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, n, "STP").Child("BPDU Type")))
	})
	t.Run("llc/snap", func(t *testing.T) {
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append([]byte{0xaa, 0xaa, 0x03, 0x00, 0x00, 0x00, 0x08, 0x06}, arp...)
		n := parseRule(t, raw, "llc", "LLC")
		require.Equal(t, uint64(0x0806), uintVal(t, mustChild(t, n, "SNAP").Child("Type")))
	})
	t.Run("llc/xid", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x00, 0xaf, 0x81, 0x01, 0x7f}, "llc", "LLC")
		require.Equal(t, uint64(0x81), uintVal(t, n.Child("XID Format")))
	})
	t.Run("llc/iframe", func(t *testing.T) {
		n := parseRule(t, []byte{0x7e, 0x7e, 0x00, 0x01}, "llc", "LLC")
		require.Equal(t, uint64(0x00), uintVal(t, n.Child("Control")))
		require.Equal(t, uint64(0x01), uintVal(t, n.Child("Control Extended")))
	})
	t.Run("openvpn/hard-reset", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "3801020304050607080000000001"), "openvpn", "OpenVPN")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Opcode")))
	})
	t.Run("openvpn/data-v2", func(t *testing.T) {
		n := parseRule(t, []byte{0x48, 0x00, 0x00, 0x01, 0xde, 0xad, 0xbe, 0xef}, "openvpn", "OpenVPN")
		require.Equal(t, uint64(9), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Peer ID")))
	})
	t.Run("openvpn/ack", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "28010203040506070801000000010807060504030201"), "openvpn", "OpenVPN")
		require.Equal(t, uint64(5), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Acks").Children()[0]))
	})
	t.Run("sctp/data", func(t *testing.T) {
		raw := mustHex(t, "40000b8000016f0a6db018820003005b280243450000a0bd000000074d454741434f2f32203c6d672d74723e3a31363338340a5265706c79203d203137343039317b0a436f6e74657874203d203235357b0a4d6f64696679203d204d55582f3235350a7d0a7d0a67")
		n := parseRule(t, raw, "sctp", "SCTP")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Chunks").Children()[0].Child("PPI")))
	})
	t.Run("sctp/init", func(t *testing.T) {
		raw := mustHex(t, "00070007000000003761a74601000020432325440000ffff001100115cfe379fc0000004000c000600050000")
		n := parseRule(t, raw, "sctp", "SCTP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Chunks").Children()[0].Child("Type")))
		require.Equal(t, uint64(17), uintVal(t, n.Child("Chunks").Children()[0].Child("Outbound Streams")))
	})
	t.Run("sctp/sack", func(t *testing.T) {
		raw := mustHex(t, "0b804000214415232bf2024e03000010280243450000200000000000")
		require.Equal(t, uint64(3), uintVal(t, parseRule(t, raw, "sctp", "SCTP").Child("Chunks").Children()[0].Child("Type")))
	})
	t.Run("sctp/cookie-ack", func(t *testing.T) {
		raw := mustHex(t, "0007000743232544ceec2d790b000004")
		require.Equal(t, uint64(11), uintVal(t, parseRule(t, raw, "sctp", "SCTP").Child("Chunks").Children()[0].Child("Type")))
	})
	t.Run("eigrp/hello", func(t *testing.T) {
		raw := mustHex(t, "0205ee68000000000000000000000000000000640001000c010001000000000f000400080c040102")
		n := parseRule(t, raw, "eigrp", "EIGRP")
		require.Equal(t, uint64(5), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("TLVs").Children()[0].Child("K1")))
	})
	t.Run("eigrp/swver", func(t *testing.T) {
		raw := mustHex(t, "0205ee68000000000000000000000000000000640001000c010001000000000f000400080c040102")
		n := parseRule(t, raw, "eigrp", "EIGRP")
		require.Equal(t, uint64(12), uintVal(t, n.Child("TLVs").Children()[1].Child("IOS Major")))
	})
	t.Run("cdp/device-id", func(t *testing.T) {
		n := parseRule(t, wiresharkCDP(t), "cdp", "CDP")
		require.Equal(t, "R1", strVal(t, n.Child("TLVs").Children()[0].Child("Device ID")))
	})
	t.Run("cdp/port", func(t *testing.T) {
		n := parseRule(t, wiresharkCDP(t), "cdp", "CDP")
		require.Equal(t, "Ethernet0", strVal(t, n.Child("TLVs").Children()[2].Child("Port ID")))
	})
	t.Run("lldp/chassis", func(t *testing.T) {
		n := parseRule(t, gopacketSiemensLLDP(t), "lldp", "LLDP")
		require.Equal(t, "switch1", strVal(t, n.Child("TLVs").Children()[0].Child("Chassis ID")))
	})
	t.Run("lldp/port", func(t *testing.T) {
		n := parseRule(t, gopacketSiemensLLDP(t), "lldp", "LLDP")
		require.Equal(t, "port-001", strVal(t, n.Child("TLVs").Children()[1].Child("Port ID")))
		require.Equal(t, uint64(20), uintVal(t, n.Child("TLVs").Children()[2].Child("TTL")))
	})
	t.Run("pppoe/padi", func(t *testing.T) {
		n := parseRule(t, rfcPPPoEPADI(t), "pppoe", "PPPoE")
		require.Equal(t, "isp", strVal(t, n.Child("Payload").Child("Tags").Children()[0].Child("Service-Name")))
	})
	t.Run("pppoe/pado", func(t *testing.T) {
		n := parseRule(t, rfcPPPoEPADO(t), "pppoe", "PPPoE")
		require.Equal(t, "BRAS1", strVal(t, n.Child("Payload").Child("Tags").Children()[1].Child("AC-Name")))
	})
	t.Run("radius/user-name", func(t *testing.T) {
		n := parseRule(t, radiusAccessRequestFrame[42:], "application-layer.radius", "RADIUS")
		require.Equal(t, "Admin", strVal(t, n.Child("Attributes").Children()[0].Child("User-Name")))
	})
	t.Run("radius/nas-ip", func(t *testing.T) {
		n := parseRule(t, radiusAccessRequestFrame[42:], "application-layer.radius", "RADIUS")
		require.Equal(t, []byte{0x7f, 0x00, 0x01, 0x01}, bytesVal(t, n.Child("Attributes").Children()[2].Child("NAS-IP-Address")))
	})
	t.Run("ntlm/challenge", func(t *testing.T) {
		n := parseRule(t, ntlmsspChallengeTarget("DOMAIN"), "application-layer.ntlm", "NTLMSSP")
		require.Equal(t, utf16LE("DOMAIN"), bytesVal(t, n.Child("Target Name")))
	})
	t.Run("ntlm/authenticate", func(t *testing.T) {
		n := parseRule(t, ntlmsspAuthUser("CORP", "Admin"), "application-layer.ntlm", "NTLMSSP")
		require.Equal(t, utf16LE("Admin"), bytesVal(t, n.Child("User Name")))
	})
	t.Run("ber/integer", func(t *testing.T) {
		n := parseRule(t, []byte{0x02, 0x01, 0x05}, "application-layer.ber", "BER Element")
		require.Equal(t, uint64(5), uintVal(t, n.Child("Integer")))
	})
	t.Run("ber/oid", func(t *testing.T) {
		n := parseRule(t, []byte{0x06, 0x08, 0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, "application-layer.ber", "BER Element")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, n.Child("OBJECT IDENTIFIER")))
	})
	t.Run("tcp/mss", func(t *testing.T) {
		n := parseRule(t, gopacketTCPSYMSS()[34:62], "transmission_control_protocol", "TCP")
		require.Equal(t, uint64(8192), uintVal(t, n.Child("Options").Children()[0].Child("MSS")))
	})
	t.Run("tcp/timestamp", func(t *testing.T) {
		n := parseRule(t, rfcTCPTimestamp(), "transmission_control_protocol", "TCP")
		opts := n.Child("Options").Children()
		require.Equal(t, uint64(2), uintVal(t, opts[len(opts)-1].Child("TS Val")))
	})
	t.Run("igmp/v1-report", func(t *testing.T) {
		n := parseRule(t, []byte{0x12, 0x00, 0x0c, 0xc3, 224, 0, 1, 60}, "igmp", "IGMP")
		require.Equal(t, uint64(0x12), uintVal(t, n.Child("Type")))
	})
	t.Run("igmp/v3-query", func(t *testing.T) {
		raw := []byte{0x11, 0x64, 0x00, 0x00, 239, 1, 1, 1, 0x02, 0x7d, 0x00, 0x01, 192, 0, 2, 1}
		n := parseRule(t, raw, "igmp", "IGMP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Number of Sources")))
		require.Equal(t, []byte{192, 0, 2, 1}, bytesVal(t, n.Child("Sources").Children()[0]))
	})
	t.Run("rsync/v31", func(t *testing.T) {
		require.Equal(t, "31", strVal(t, parseRule(t, []byte("@RSYNCD: 31.0\n"), "rsync", "Rsync").Child("Major")))
	})
	t.Run("rsync/v30", func(t *testing.T) {
		require.Equal(t, "30", strVal(t, parseRule(t, []byte("@RSYNCD: 30.0\n"), "rsync", "Rsync").Child("Major")))
	})
	t.Run("rsync/module", func(t *testing.T) {
		require.Equal(t, "public", strVal(t, parseRule(t, []byte("@RSYNCD: 31.0\npublic\n"), "rsync", "Rsync").Child("Module")))
	})
	t.Run("rsync/ok", func(t *testing.T) {
		require.Equal(t, "OK", strVal(t, parseRule(t, []byte("@RSYNCD: 31.0\n@RSYNCD: OK\n"), "rsync", "Rsync").Child("Status")))
	})

	t.Run("snmp/get", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "302602010004067075626c6963a019020101020100020100300e300c06082b060102010101000500"), "application-layer.snmp", "SNMP")
		require.Equal(t, []byte("public"), bytesVal(t, n.Child("Community")))
		require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, n, "PDU Body", "Request ID")))
	})
	t.Run("snmpv3/header", func(t *testing.T) {
		hdr := mustChild(t, parseRule(t, mustHex(t, "3013020103300e020101020300ffe3040104020103"), "application-layer.snmp", "SNMPv3"), "SNMPHeaderData")
		require.Equal(t, uint64(1), uintVal(t, hdr.Child("MsgID")))
		require.Equal(t, uint64(65507), uintVal(t, hdr.Child("MsgMaxSize")))
		require.Equal(t, uint64(4), uintVal(t, hdr.Child("MsgFlags")))
	})

	t.Run("ldap/bind", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "3033020101602e020103041f"+
			"7569643d616b61726173756c752c64633d6578616d706c652c64633d636f6d"+
			"800870617373776f7264"), "application-layer.ldap", "LDAPMessage")
		br := mustChild(t, n, "Body", "ProtocolOp", "BindRequest")
		require.Equal(t, "uid=akarasulu,dc=example,dc=com", strVal(t, br.Child("Name")))
		require.Equal(t, "password", strVal(t, br.Child("Auth")))
	})
	t.Run("ldap/unbind", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "30050201014200"), "application-layer.ldap", "LDAPMessage")
		require.Equal(t, uint64(0x42), uintVal(t, mustChild(t, n, "Body").Child("ProtocolOp Tag")))
	})

	t.Run("dcerpc/bind", func(t *testing.T) {
		n := parseRule(t, append(dcerpcHeader(11, 28, 1), make([]byte, 12)...), "application-layer.dcerpc", "DCERPC")
		require.Equal(t, uint64(11), uintVal(t, n.Child("PType")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, n, "PDU", "Bind").Child("Num Ctx Items")))
	})
	t.Run("dcerpc/bind-ack", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "05000c03100000003c00000001000000d016d0160100000004003133350000000100000000000000045d888aeb1cc9119fe808002b10486002000000"), "application-layer.dcerpc", "DCERPC")
		ack := mustChild(t, n, "PDU", "BindAck")
		require.Equal(t, uint64(4), uintVal(t, ack.Child("Sec Addr Len")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, ack, "DCERPCResults").Child("Ack Result")))
	})

	t.Run("spnego/ntlm", func(t *testing.T) {
		init := mustChild(t, parseRule(t, mustHex(t, "601c06062b0601050502a0123010a00e300c060a2b06010401823702020a"), "application-layer.spnego", "SPNEGO"), "Token", "SPNEGOInit")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}, bytesVal(t, init.Child("MechOID")))
	})
	t.Run("spnego/krb5", func(t *testing.T) {
		init := mustChild(t, parseRule(t, mustHex(t, "601b06062b0601050502a011300fa00d300b06092a864886f712010202"), "application-layer.spnego", "SPNEGO"), "Token", "SPNEGOInit")
		require.Equal(t, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02}, bytesVal(t, init.Child("MechOID")))
	})

	t.Run("kerberos/as-req", func(t *testing.T) {
		req := mustChild(t, parseRule(t, mustHex(t, "6a0c300aa103020105a20302010a"), "application-layer.kerberos", "Kerberos"), "Body", "KerberosASReq")
		require.Equal(t, uint64(5), uintVal(t, req.Child("Pvno")))
		require.Equal(t, uint64(10), uintVal(t, req.Child("MsgType")))
	})
	t.Run("kerberos/as-rep", func(t *testing.T) {
		rep := mustChild(t, parseRule(t, mustHex(t, "6b0c300aa003020105a10302010b"), "application-layer.kerberos", "Kerberos"), "Body", "KerberosASRep")
		require.Equal(t, uint64(5), uintVal(t, rep.Child("Pvno")))
		require.Equal(t, uint64(11), uintVal(t, rep.Child("MsgType")))
	})

	t.Run("ajp/cping", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "123400010a"), "application-layer.ajp", "AJP")
		require.Equal(t, uint64(0x0a), uintVal(t, n.Child("Code")))
		require.Nil(t, n.Child("AJPForward"))
	})
	t.Run("ajp/forward", func(t *testing.T) {
		fwd := mustChild(t, parseRule(t, mustHex(t, "1234003202020008485454502f312e310000012f0000093132372e302e302e310000000000096c6f63616c686f7374000050000000ff"), "application-layer.ajp", "AJP"), "AJPForward")
		require.Equal(t, uint64(2), uintVal(t, fwd.Child("Method")))
		require.Equal(t, "/", strVal(t, fwd.Child("URI")))
	})

	t.Run("tds/prelogin", func(t *testing.T) {
		n := parseRule(t, tdsPacket(18, 1, mustHex(t, "0000060006ff0c0000000000")), "application-layer.tds", "TDS")
		require.Equal(t, uint64(12), uintVal(t, mustChild(t, n, "TDSVersionData").Child("Version Major")))
	})
	t.Run("tds/login7", func(t *testing.T) {
		lg := mustChild(t, parseRule(t, tdsPacket(16, 1, mustHex(t, "6600000004000074001000000000000000000000000000000000000000000000000000005e00040000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000068006f0073007400")), "application-layer.tds", "TDS"), "TDSLogin7")
		require.Equal(t, uint64(0x74000004), uintVal(t, lg.Child("TDS Version")))
		require.Equal(t, uint64(4), uintVal(t, lg.Child("HostName Chars")))
	})

	t.Run("mysql/query", func(t *testing.T) {
		n := parseRule(t, mysqlPacket(0, append([]byte{0x03}, []byte("SELECT 1")...)), "application-layer.mysql", "MySQLPacket")
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, n, "Payload").Child("Query")))
	})
	t.Run("mysql/err", func(t *testing.T) {
		errp := mustChild(t, parseRule(t, mysqlPacket(1, []byte{0xff, 0x15, 0x04, '#', '2', '8', '0', '0', '0', 'A', 'c', 'c', 'e', 's', 's', ' ', 'd', 'e', 'n', 'i', 'e', 'd'}), "application-layer.mysql", "MySQLPacket"), "Payload", "MySQLERR")
		require.Equal(t, uint64(1045), uintVal(t, errp.Child("Error Code")))
		require.Equal(t, "28000", strVal(t, errp.Child("SQL State")))
	})
	t.Run("mysql/ok", func(t *testing.T) {
		okp := mustChild(t, parseRule(t, mysqlPacket(1, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}), "application-layer.mysql", "MySQLPacket"), "Payload", "MySQLOK")
		require.Equal(t, uint64(2), uintVal(t, okp.Child("Status Flags")))
	})

	t.Run("redis/ping", func(t *testing.T) {
		n := redisRoot(t, parseRule(t, []byte("*1\r\n$4\r\nPING\r\n"), "application-layer.redis", "Redis"))
		require.Equal(t, "PING", strVal(t, mustChild(t, n, "Array", "RedisCommand").Child("Command")))
	})
	t.Run("redis/get", func(t *testing.T) {
		n := redisRoot(t, parseRule(t, []byte("*2\r\n$3\r\nGET\r\n$5\r\nmykey\r\n"), "application-layer.redis", "Redis"))
		require.Equal(t, "GET", strVal(t, mustChild(t, n, "Array", "RedisCommand").Child("Command")))
		require.Equal(t, "mykey", strVal(t, mustChild(t, mustChild(t, n, "Array", "Arguments").Children()[0], "Bulk").Child("Bulk")))
	})

	t.Run("postgres/query", func(t *testing.T) {
		n := parseRule(t, pgTyped('Q', append([]byte("SELECT 1"), 0)), "application-layer.postgresql", "PostgreSQL")
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, n, "Payload", "PostgreSQLQuery").Child("SQL")))
	})
	t.Run("postgres/error", func(t *testing.T) {
		fields := mustChild(t, parseRule(t, pgTyped('E', []byte("SERROR\x00C42601\x00Msyntax\x00\x00")), "application-layer.postgresql", "PostgreSQL"), "Payload", "PostgreSQLError").Children()
		require.Equal(t, "42601", strVal(t, fields[1].Child("SQLState")))
		require.Equal(t, "syntax", strVal(t, fields[2].Child("Message")))
	})

	t.Run("tpkt/cookie", func(t *testing.T) {
		rdp := parseRule(t, mustHex(t, "0300002c27e00000000000436f6f6b69653a206d737473686173683d656c746f6e730d0a0100080000000000"), "application-layer.msrdp", "RDP")
		require.Equal(t, "Cookie: mstshash=eltons", strVal(t, mustChild(t, rdp, "X224", "VariableData", "RDPCookie").Child("Line")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, rdp, "X224", "VariableData", "RDPNegotiation").Child("Protocol")))
	})
	t.Run("tpkt/cc", func(t *testing.T) {
		require.Equal(t, uint64(0xd0), uintVal(t, mustChild(t, parseRule(t, []byte{0x03, 0x00, 0x00, 0x0b, 0x06, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00}, "application-layer.msrdp", "RDP"), "X224").Child("Flag")))
	})

	t.Run("mdns/ptr", func(t *testing.T) {
		raw := []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x05, '_', 'h', 't', 't', 'p', 0x04, '_', 't', 'c', 'p', 0x05, 'l', 'o', 'c', 'a', 'l', 0x00,
			0x00, 0x0c, 0x00, 0x01,
		}
		q := parseRule(t, raw, "application-layer.dns", "DNS").Child("Questions").Children()[0]
		require.Equal(t, "_http", strVal(t, q.Child("Name").Children()[0].Child("Text")))
		require.Equal(t, uint64(12), uintVal(t, q.Child("Type")))
	})
	t.Run("mdns/a", func(t *testing.T) {
		n := parseRule(t, mustHex(t, "bc35818000010002000000000b636c6f7564636f6e666967096a6574627261696e7303636f6d0000010001c00c000100010000001300043412ec15c00c00010001000000130004364dbb13"), "application-layer.dns", "DNS")
		require.Equal(t, []byte{0x34, 0x12, 0xec, 0x15}, bytesVal(t, mustChild(t, n.Child("Answers").Children()[0], "DNSA").Child("Address")))
	})

	t.Run("nbns/stat", func(t *testing.T) {
		q := parseRule(t, nbnsStarStatQuery(), "application-layer.nbns", "NBNS").Child("Questions").Children()[0]
		require.Equal(t, uint64(0x21), uintVal(t, q.Child("Type")))
		require.Equal(t, "CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", strVal(t, q.Child("Name").Children()[0].Child("Text")))
	})
	t.Run("llmnr/a", func(t *testing.T) {
		q := dnsLikeQuery("TEST")
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
		require.Equal(t, []byte{10, 0, 0, 9}, bytesVal(t, mustChild(t, parseRule(t, ans, "application-layer.nbns", "LLMNR").Child("Answers").Children()[0], "NBNSA").Child("Address")))
	})

	t.Run("mqtt/publish", func(t *testing.T) {
		pub := mustChild(t, parseRule(t, append([]byte{0x30, 0x11, 0x00, 0x0b}, []byte("sensor/temp23.5")...), "application-layer.mqtt", "MQTT"), "Payload", "Publish")
		require.Equal(t, "sensor/temp", strVal(t, pub.Child("Topic")))
		require.Equal(t, "23.5", strVal(t, pub.Child("Message")))
	})
	t.Run("mqtt/qos1", func(t *testing.T) {
		pub := mustChild(t, parseRule(t, append([]byte{0x32, 0x0a, 0x00, 0x01, 'a', 0x00, 0x07}, []byte("hello")...), "application-layer.mqtt", "MQTT"), "Payload", "Publish")
		require.Equal(t, uint64(7), uintVal(t, pub.Child("Packet ID")))
		require.Equal(t, "hello", strVal(t, pub.Child("Message")))
	})

	t.Run("http/post", func(t *testing.T) {
		req := mustChild(t, parseRule(t, []byte("POST /submit HTTP/1.1\r\nHost: origin.example\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 7\r\n\r\nfoo=bar"), "application-layer.http", "HTTP"), "HTTP Request")
		require.Equal(t, "POST", strVal(t, req.Child("Method")))
		require.Equal(t, "foo=bar", strVal(t, mustChild(t, req, "Body").Child("Octets")))
	})
	t.Run("http/chunked", func(t *testing.T) {
		chunks := mustChild(t, parseRule(t, []byte("POST / HTTP/1.1\r\nHost: example.tld\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"), "application-layer.http", "HTTP"), "HTTP Request", "Body", "DataChunks").Children()
		require.Equal(t, "hello", strVal(t, chunks[0].Child("Octets")))
	})
	t.Run("http/ok", func(t *testing.T) {
		resp := mustChild(t, parseRule(t, []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello"), "application-layer.http", "HTTP"), "HTTP Response")
		require.Equal(t, "200", strVal(t, resp.Child("Status")))
		require.Equal(t, "hello", strVal(t, mustChild(t, resp, "Body").Child("Octets")))
	})

	t.Run("http2/data", func(t *testing.T) {
		raw := make([]byte, 9+5)
		raw[2] = 5
		raw[4] = 0x01
		copy(raw[9:], []byte("hello"))
		require.Equal(t, "hello", strVal(t, parseRule(t, raw, "application-layer.http2", "HTTP2").Child("Octets")))
	})
	t.Run("http2/rst", func(t *testing.T) {
		n := parseRule(t, []byte{0x00, 0x00, 0x04, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01}, "application-layer.http2", "HTTP2")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Error Code")))
	})
	t.Run("http2/goaway", func(t *testing.T) {
		raw := append([]byte{0x00, 0x00, 0x0b, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, []byte("bye")...)
		require.Equal(t, "bye", strVal(t, parseRule(t, raw, "application-layer.http2", "HTTP2").Child("Octets")))
	})

	t.Run("tls/suites", func(t *testing.T) {
		hello := append([]byte{0x03, 0x03}, make([]byte, 32)...)
		hello = append(hello, 0, 0x00, 0x04, 0x00, 0x2f, 0x00, 0x35, 0x01, 0x00)
		hs := append([]byte{0x01, 0x00, 0x00, byte(len(hello))}, hello...)
		suites := mustChild(t, parseRule(t, hs, "application-layer.tls_hello", "TLSClientHello"), "ClientHello", "Cipher Suites").Children()
		require.Equal(t, uint64(0x002f), uintVal(t, suites[0].Child("Suite")))
		require.Equal(t, uint64(0x0035), uintVal(t, suites[1].Child("Suite")))
	})
	t.Run("tls/sni", func(t *testing.T) {
		body := append([]byte{0x03, 0x03}, make([]byte, 32)...)
		body = append(body, 0x00, 0x00, 0x04, 0x00, 0x2f, 0x00, 0x35, 0x01, 0x00)
		body = append(body, append([]byte{0x00, 0x14, 0x00, 0x00, 0x00, 0x10, 0x00, 0x0e, 0x00, 0x00, 0x0b}, []byte("example.com")...)...)
		hs := append([]byte{0x01, 0x00, 0x00, byte(len(body))}, body...)
		exts := mustChild(t, parseRule(t, hs, "application-layer.tls_hello", "TLSClientHello"), "ClientHello", "Extensions").Children()
		require.Equal(t, "example.com", strVal(t, mustChild(t, exts[0], "SNI").Child("Host Name")))
	})

	t.Run("tns/connect", func(t *testing.T) {
		cdata := []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=ORCL)))")
		require.Equal(t, string(cdata), strVal(t, mustChild(t, parseRule(t, tnsConnectPacket(cdata), "application-layer.tns", "TNS"), "Connect").Child("Connect Data")))
	})
	t.Run("tns/data", func(t *testing.T) {
		raw := make([]byte, 12)
		binary.BigEndian.PutUint16(raw[0:], 12)
		raw[4] = 6
		copy(raw[10:], []byte("AB"))
		n := parseRule(t, raw, "application-layer.tns", "TNS")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Data Flag")))
		require.Equal(t, "AB", strVal(t, n.Child("Octets")))
	})

	t.Run("smb2/tree-connect", func(t *testing.T) {
		path := utf16LE(`\\srv\share`)
		tcBody := make([]byte, 8+len(path))
		binary.LittleEndian.PutUint16(tcBody[0:], 9)
		binary.LittleEndian.PutUint16(tcBody[4:], 72)
		binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
		copy(tcBody[8:], path)
		require.Equal(t, `\\srv\share`, strings.ReplaceAll(strVal(t, mustChild(t, parseRule(t, append(smb2SyncHeader(3, 0, 4), tcBody...), "application-layer.smb2", "SMB2"), "Tree Connect Request").Child("Path")), "\x00", ""))
	})
	t.Run("smb2/read", func(t *testing.T) {
		rr := make([]byte, 16+8)
		binary.LittleEndian.PutUint16(rr[0:], 17)
		rr[2] = 80
		binary.LittleEndian.PutUint32(rr[4:], 8)
		copy(rr[16:], []byte("abcdefgh"))
		require.Equal(t, "abcdefgh", strVal(t, mustChild(t, parseRule(t, append(smb2SyncHeader(8, 1, 6), rr...), "application-layer.smb2", "SMB2"), "Read Response").Child("Octets")))
	})
	t.Run("smb/tree-connect", func(t *testing.T) {
		tc := mustChild(t, parseRule(t, smb1TreeConnectAndX(`\\srv\share`, "A:"), "application-layer.smb", "SMB"), "TreeConnectAndX")
		require.Equal(t, `\\srv\share`, strVal(t, tc.Child("Path")))
		require.Equal(t, "A:", strVal(t, tc.Child("Service")))
	})

	t.Run("gre/rfc2784", func(t *testing.T) {
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		n := parseRule(t, append([]byte{0x00, 0x00, 0x08, 0x06}, arp...), "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0x0806), uintVal(t, n.Child("Protocol Type")))
		require.Nil(t, n.Child("Call ID"))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Payload", "ARP").Child("Opcode")))
	})
	t.Run("gre/key", func(t *testing.T) {
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		n := parseRule(t, append([]byte{0x20, 0x00, 0x08, 0x06, 0x12, 0x34, 0x56, 0x78}, arp...), "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0x12345678), uintVal(t, n.Child("Key")))
		require.Nil(t, n.Child("Call ID"))
	})
}
