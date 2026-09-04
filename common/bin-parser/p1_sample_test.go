package bin_parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1WiresharkAndRFCSamples(t *testing.T) {
	t.Run("dhcpv6/solicit", func(t *testing.T) {
		// RFC 8415 §18.2.1 Solicit: CLIENTID DUID-LL, ELAPSED_TIME 0, IA_NA, ORO DNS/DOMAIN.
		sol := mustHex(t, ""+
			"01 000001 "+
			"0001 000a 0003 0001 000000000001 "+
			"0008 0002 0000 "+
			"0003 000c 00000001 00000000 00000000 "+
			"0006 0004 0017 0018")
		n := parseRule(t, sol, "dhcpv6", "DHCPv6")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Message Type")))
		opts := n.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 4)
		require.Equal(t, uint64(1), uintVal(t, opts[0].Child("Code")))
		require.Equal(t, uint64(3), uintVal(t, opts[0].Child("DUID Type")))
		require.Equal(t, uint64(1), uintVal(t, opts[0].Child("Hardware Type")))
		require.Equal(t, []byte{0, 0, 0, 0, 0, 1}, bytesVal(t, opts[0].Child("Link Layer")))
		require.Equal(t, uint64(8), uintVal(t, opts[1].Child("Code")))
		require.Equal(t, uint64(0), uintVal(t, opts[1].Child("Elapsed Time")))
		require.Equal(t, uint64(1), uintVal(t, opts[2].Child("IAID")))
		require.Equal(t, uint64(23), uintVal(t, opts[3].Child("ORO1")))
		require.Equal(t, uint64(24), uintVal(t, opts[3].Child("ORO2")))
		eth := parseEthernet(t, ipv6UDPBytes(t, 546, 547, sol))
		v6 := mustChild(t, eth, "IPv6", "UDP", "DHCPv6")
		require.Equal(t, uint64(0), uintVal(t, v6.Child("Options").Children()[1].Child("Elapsed Time")))
		require.Equal(t, uint64(1), uintVal(t, v6.Child("Options").Children()[2].Child("IAID")))
	})

	t.Run("dhcpv6/reply", func(t *testing.T) {
		// RFC 8415 §21.4/21.6/21.19 Reply: SERVERID, IA_NA+IAADDR 2001:db8::1, DNS 2001:db8::53.
		rep := mustHex(t, ""+
			"07 000001 "+
			"0001 000a 0003 0001 000000000001 "+
			"0002 000a 0003 0001 000000000002 "+
			"0003 0028 00000001 00000e10 00001518 "+
			"0005 0018 20010db8000000000000000000000001 00000e10 00001c20 "+
			"0017 0010 20010db8000000000000000000000053")
		n := parseRule(t, rep, "dhcpv6", "DHCPv6")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Message Type")))
		opts := n.Child("Options").Children()
		require.Equal(t, uint64(2), uintVal(t, opts[1].Child("Code")))
		require.Equal(t, []byte{0, 0, 0, 0, 0, 2}, bytesVal(t, opts[1].Child("Link Layer")))
		iana := opts[2]
		require.Equal(t, uint64(3600), uintVal(t, iana.Child("T1")))
		require.Equal(t, uint64(5400), uintVal(t, iana.Child("T2")))
		require.Equal(t, uint64(5), uintVal(t, iana.Child("IAAddr Code")))
		require.Equal(t, []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, bytesVal(t, iana.Child("Address")))
		require.Equal(t, uint64(3600), uintVal(t, iana.Child("Preferred")))
		require.Equal(t, uint64(7200), uintVal(t, iana.Child("Valid")))
		require.Equal(t, []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x53}, bytesVal(t, opts[3].Child("DNS Address")))
		eth := parseEthernet(t, ipv6UDPBytes(t, 547, 546, rep))
		v6 := mustChild(t, eth, "IPv6", "UDP", "DHCPv6")
		require.Equal(t, uint64(7), uintVal(t, v6.Child("Message Type")))
		require.Equal(t, uint64(3600), uintVal(t, v6.Child("Options").Children()[2].Child("T1")))
	})

	t.Run("icmp/echo", func(t *testing.T) {
		// RFC 792 Echo: Type 8 Identifier/Sequence + Data (gopacket layers.ICMPv4TypeEchoRequest).
		echo := append([]byte{0x08, 0x00, 0x00, 0x00, 0x12, 0x34, 0x00, 0x01}, []byte("abcdefghijklmnop")...)
		n := parseRule(t, echo, "internet_control_message_protocol", "ICMP")
		require.Equal(t, uint64(8), uintVal(t, n.Child("Type")))
		e := mustChild(t, n, "ICMP Echo")
		require.Equal(t, uint64(0x1234), uintVal(t, e.Child("Identifier")))
		require.Equal(t, uint64(1), uintVal(t, e.Child("Sequence Number")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 1, echo))
		require.Equal(t, "abcdefghijklmnop", strVal(t, mustChild(t, eth, "IP", "ICMP", "ICMP Echo").Child("Echo Data")))
	})

	t.Run("icmp/echo-reply", func(t *testing.T) {
		rep := append([]byte{0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x00, 0x01}, []byte("abcdefghijklmnop")...)
		n := parseRule(t, rep, "internet_control_message_protocol", "ICMP")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Type")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 1, rep))
		require.Equal(t, "abcdefghijklmnop", strVal(t, mustChild(t, eth, "IP", "ICMP", "ICMP Echo Reply").Child("Echo Data")))
		require.Equal(t, uint64(0x1234), uintVal(t, mustChild(t, eth, "IP", "ICMP", "ICMP Echo Reply").Child("Identifier")))
	})

	t.Run("icmpv6/echo", func(t *testing.T) {
		// RFC 4443 Echo Request Type 128, Identifier/Sequence + Data.
		echo := append([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02}, []byte("ping6")...)
		n := parseRule(t, echo, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(128), uintVal(t, n.Child("Type")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, echo))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IPv6", "ICMPv6", "Echo Request").Child("Identifier")))
		require.Equal(t, "ping6", strVal(t, mustChild(t, eth, "IPv6", "ICMPv6", "Echo Request").Child("Echo Data")))
	})

	t.Run("icmpv6/ra-opt", func(t *testing.T) {
		// gopacket layers/icmp6msg_test.go Router Advertisement: SLLA + MTU 1500 + Prefix 2001:db8:0:1::/64.
		ra := []byte{
			0x33, 0x33, 0x00, 0x00, 0x00, 0x01, 0xc2, 0x00, 0x54, 0xf5, 0x00, 0x00, 0x86, 0xdd, 0x6e, 0x00,
			0x00, 0x00, 0x00, 0x40, 0x3a, 0xff, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x00,
			0x54, 0xff, 0xfe, 0xf5, 0x00, 0x00, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x86, 0x00, 0xc4, 0xfe, 0x40, 0x00, 0x07, 0x08, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0xc2, 0x00, 0x54, 0xf5, 0x00, 0x00, 0x05, 0x01,
			0x00, 0x00, 0x00, 0x00, 0x05, 0xdc, 0x03, 0x04, 0x40, 0xc0, 0x00, 0x27, 0x8d, 0x00, 0x00, 0x09,
			0x3a, 0x80, 0x00, 0x00, 0x00, 0x00, 0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		adv := mustChild(t, parseEthernet(t, ra), "IPv6", "ICMPv6", "Router Advertisement")
		opts := adv.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 3)
		require.Equal(t, uint64(1), uintVal(t, opts[0].Child("Type")))
		require.Equal(t, []byte{0xc2, 0x00, 0x54, 0xf5, 0x00, 0x00}, bytesVal(t, opts[0].Child("Link Layer")))
		require.Equal(t, uint64(5), uintVal(t, opts[1].Child("Type")))
		require.Equal(t, uint64(1500), uintVal(t, opts[1].Child("MTU")))
		require.Equal(t, uint64(3), uintVal(t, opts[2].Child("Type")))
		require.Equal(t, uint64(64), uintVal(t, opts[2].Child("Prefix Length")))
	})

	t.Run("pap/request", func(t *testing.T) {
		// RFC 1334 §2.2.1 Authenticate-Request: Peer-ID "ixia", Password "ixia".
		pap := []byte{0x01, 0x00, 0x00, 0x0e, 0x04, 'i', 'x', 'i', 'a', 0x04, 'i', 'x', 'i', 'a'}
		n := parseRule(t, pap, "password_authentication_protocol", "PAP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Code")))
		req := mustChild(t, n, "Request")
		require.Equal(t, "ixia", strVal(t, req.Child("Peer ID")))
		require.Equal(t, "ixia", strVal(t, req.Child("Password")))
		gre := append(mustHex(t, "3081880b0012000100000001ffffffffff03c023"), pap...)
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		r := mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "PAP", "Request")
		require.Equal(t, "ixia", strVal(t, r.Child("Peer ID")))
		require.Equal(t, "ixia", strVal(t, r.Child("Password")))
	})

	t.Run("pap/ack", func(t *testing.T) {
		// RFC 1334 §2.2.2 Authenticate-Ack: Msg-Length 2, Message "OK".
		ack := []byte{0x02, 0x00, 0x00, 0x07, 0x02, 'O', 'K'}
		n := parseRule(t, ack, "password_authentication_protocol", "PAP")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Code")))
		require.Equal(t, "OK", strVal(t, mustChild(t, n, "Response").Child("Message")))
		gre := append(mustHex(t, "3081880b000b000100000001ffffffffff03c023"), ack...)
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		require.Equal(t, "OK", strVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "PAP", "Response").Child("Message")))
	})

	t.Run("lcp/rfc1661", func(t *testing.T) {
		// RFC 1661 §5.1/§6.2/§6.4 Configure-Request: PAP 0xc023 + Magic-Number 0x0f3f117c.
		lcp := mustHex(t, "0101000e0304c02305060f3f117c")
		n := parseRule(t, lcp, "link_control_protocol", "LCP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Code")))
		opts := n.Child("Options").Children()
		require.Equal(t, uint64(3), uintVal(t, opts[0].Child("Type")))
		require.Equal(t, uint64(0xc023), uintVal(t, opts[0].Child("Auth Protocol")))
		require.Equal(t, uint64(5), uintVal(t, opts[1].Child("Type")))
		require.Equal(t, uint64(0x0f3f117c), uintVal(t, opts[1].Child("Magic Number")))
		gre := mustHex(t, "3081880b0012000100000001ffffffffff03c021"+
			"0101000e0304c02305060f3f117c")
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		l := mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "LCP")
		require.Equal(t, uint64(0xc023), uintVal(t, l.Child("Options").Children()[0].Child("Auth Protocol")))
		require.Equal(t, uint64(0x0f3f117c), uintVal(t, l.Child("Options").Children()[1].Child("Magic Number")))
	})

	t.Run("lcp/echo", func(t *testing.T) {
		// RFC 1661 §5.8 Echo-Request: Magic-Number 0x12345678, no extra data.
		echo := []byte{0x09, 0x01, 0x00, 0x08, 0x12, 0x34, 0x56, 0x78}
		n := parseRule(t, echo, "link_control_protocol", "LCP")
		require.Equal(t, uint64(9), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(0x12345678), uintVal(t, mustChild(t, n, "Echo").Child("Magic Number")))
		gre := append(mustHex(t, "3081880b000c000100000001ffffffffff03c021"), echo...)
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		require.Equal(t, uint64(0x12345678), uintVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "LCP", "Echo").Child("Magic Number")))
	})

	t.Run("ftp_data/stream", func(t *testing.T) {
		// RFC 959 §3.4.1 STREAM mode (default): no block header, TCP payload is the file.
		fd := []byte("README.txt contents")
		eth := parseEthernet(t, ipv4TCPFrame(t, 20, 20, fd))
		require.Equal(t, "README.txt contents", strVal(t, mustChild(t, eth, "IP", "TCP", "FTPData").Child("File Data")))
	})

	t.Run("ftp_data/eor", func(t *testing.T) {
		// RFC 959 §3.4.2: bit 128 EOR, not EOF. TCP/20 bounds the block.
		fd := []byte{0x80, 0x00, 0x0a, 'f', 'i', 'l', 'e', '-', 'b', 'y', 't', 'e', 's'}
		eth := parseEthernet(t, ipv4TCPFrame(t, 20, 20, fd))
		blk := mustChild(t, eth, "IP", "TCP", "FTPData", "Blocks").Children()[0]
		require.Equal(t, uint64(0x80), uintVal(t, blk.Child("Descriptor")))
		require.Equal(t, uint64(10), uintVal(t, blk.Child("Byte Count")))
		require.Equal(t, "file-bytes", strVal(t, blk.Child("File Data")))
	})

	t.Run("ftp_data/eof", func(t *testing.T) {
		fd := []byte{0x40, 0x00, 0x03, 'e', 'n', 'd'}
		eth := parseEthernet(t, ipv4TCPFrame(t, 20, 20, fd))
		blk := mustChild(t, eth, "IP", "TCP", "FTPData", "Blocks").Children()[0]
		require.Equal(t, uint64(0x40), uintVal(t, blk.Child("Descriptor")))
		require.Equal(t, "end", strVal(t, blk.Child("File Data")))
	})

	t.Run("tftp/wireshark", func(t *testing.T) {
		// Wireshark test/captures/tftp.pcap packet 1: Token Ring SNAP IPv4 UDP/69
		// RRQ filename C:\IBMTCPIP\lccm.1 mode octet (RFC 1350).
		tftpRRQ := mustHex(t, "0001433a5c49424d54435049505c6c63636d2e31006f6374657400")
		tf := parseRule(t, tftpRRQ, "tftp", "TFTP")
		require.Equal(t, uint64(1), uintVal(t, tf.Child("Opcode")))
		require.Equal(t, `C:\IBMTCPIP\lccm.1`, strVal(t, tf.Child("Filename")))
		require.Equal(t, "octet", strVal(t, tf.Child("Mode")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 63801, 69, tftpRRQ))
		require.Equal(t, `C:\IBMTCPIP\lccm.1`, strVal(t, mustChild(t, eth, "IP", "UDP", "TFTP").Child("Filename")))

		tftpData := append([]byte{0x00, 0x03, 0x00, 0x01}, []byte("abc")...)
		td := mustChild(t, parseEthernet(t, ipv4UDPBytes(t, 12345, 69, tftpData)), "IP", "UDP", "TFTP")
		require.Equal(t, uint64(3), uintVal(t, td.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, td.Child("Block")))
		require.Equal(t, "abc", strVal(t, td.Child("File Data")))
	})

	t.Run("pop3/rfc1939", func(t *testing.T) {
		popGreet := []byte("+OK POP3 server ready\r\n")
		po := parseRule(t, popGreet, "pop3", "POP3")
		require.Equal(t, "+OK", strVal(t, po.Child("Status")))
		require.Equal(t, "POP3 server ready", strVal(t, po.Child("Arg")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, popGreet))
		require.Equal(t, "+OK", strVal(t, mustChild(t, eth, "IP", "TCP", "POP3").Child("Status")))
	})

	t.Run("pop3/capa", func(t *testing.T) {
		// RFC 2449 CAPA multiline: +OK / USER / UIDL / .
		capa := []byte("+OK Capability list follows\r\nUSER\r\nUIDL\r\n.\r\n")
		po := parseRule(t, capa, "pop3", "POP3")
		require.Equal(t, "+OK", strVal(t, po.Child("Status")))
		lines := mustChild(t, po, "POP3Extra").Children()
		require.GreaterOrEqual(t, len(lines), 3)
		require.Equal(t, "USER", strVal(t, lines[0].Child("Text")))
		require.Equal(t, "UIDL", strVal(t, lines[1].Child("Text")))
		require.Equal(t, ".", strVal(t, lines[2].Child("Text")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, capa))
		el := mustChild(t, eth, "IP", "TCP", "POP3", "POP3Extra").Children()
		require.Equal(t, "USER", strVal(t, el[0].Child("Text")))
	})

	t.Run("imap/rfc3501", func(t *testing.T) {
		imapGreet := []byte("* OK IMAP4rev1 server ready\r\n")
		im := parseRule(t, imapGreet, "imap", "IMAP")
		require.Equal(t, "*", strVal(t, im.Child("Tag")))
		require.Equal(t, "OK", strVal(t, im.Child("Command")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 143, 143, imapGreet))
		require.Equal(t, "IMAP4rev1 server ready", strVal(t, mustChild(t, eth, "IP", "TCP", "IMAP").Child("Arg")))
	})

	t.Run("imap/fetch", func(t *testing.T) {
		// RFC 3501 §7.4.2 untagged FETCH with FLAGS and RFC822.SIZE.
		fetch := []byte("* 12 FETCH (FLAGS (\\Seen) RFC822.SIZE 448)\r\n")
		im := parseRule(t, fetch, "imap", "IMAP")
		require.Equal(t, "*", strVal(t, im.Child("Tag")))
		require.Equal(t, "12", strVal(t, im.Child("Command")))
		require.Equal(t, "FETCH", strVal(t, im.Child("Item")))
		attrs := im.Child("Attrs").Children()
		require.GreaterOrEqual(t, len(attrs), 2)
		require.Equal(t, "FLAGS", strVal(t, attrs[0].Child("Name")))
		require.Equal(t, "(\\Seen)", strVal(t, attrs[0].Child("Flags")))
		require.Equal(t, "RFC822.SIZE", strVal(t, attrs[1].Child("Name")))
		require.Equal(t, "448", strVal(t, attrs[1].Child("Size")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 143, 143, fetch))
		ethAttrs := mustChild(t, eth, "IP", "TCP", "IMAP").Child("Attrs").Children()
		require.Equal(t, "FLAGS", strVal(t, ethAttrs[0].Child("Name")))
		require.Equal(t, "448", strVal(t, ethAttrs[1].Child("Size")))
	})

	t.Run("imap/flags-extra", func(t *testing.T) {
		// RFC 3501 untagged FLAGS then greeting (two lines).
		body := []byte("* FLAGS (\\Seen)\r\n* OK IMAP4rev1 server ready\r\n")
		im := parseRule(t, body, "imap", "IMAP")
		require.Equal(t, "FLAGS", strVal(t, im.Child("Command")))
		require.Equal(t, "(\\Seen)", strVal(t, im.Child("Arg")))
		extra := mustChild(t, im, "IMAPExtra").Children()
		require.GreaterOrEqual(t, len(extra), 1)
		require.Equal(t, "* OK IMAP4rev1 server ready", strVal(t, extra[0].Child("Text")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 143, 143, body))
		el := mustChild(t, eth, "IP", "TCP", "IMAP", "IMAPExtra").Children()
		require.Equal(t, "* OK IMAP4rev1 server ready", strVal(t, el[0].Child("Text")))
	})

	t.Run("bgp/open", func(t *testing.T) {
		bgpOpen := mustHex(t, "ffffffffffffffffffffffffffffffff001d01040001005a0a00000100")
		bo := parseRule(t, bgpOpen, "bgp", "BGP")
		require.Equal(t, uint64(1), uintVal(t, bo.Child("Type")))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, bo, "OPEN").Child("Version")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, bo, "OPEN").Child("My AS")))
		require.Equal(t, uint64(90), uintVal(t, mustChild(t, bo, "OPEN").Child("Hold Time")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 179, 179, bgpOpen))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, eth, "IP", "TCP", "BGP", "OPEN").Child("Version")))
	})

	t.Run("bgp/notification", func(t *testing.T) {
		bgpNote := append(bytesRepeat(0xff, 16), 0x00, 0x15, 0x03, 0x04, 0x00)
		bn := parseRule(t, bgpNote, "bgp", "BGP")
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, bn, "NOTIFICATION").Child("Error Code")))
	})

	t.Run("chap/rfc1994", func(t *testing.T) {
		chap := mustHex(t, "01030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574")
		ch := parseRule(t, chap, "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(1), uintVal(t, ch.Child("Code")))
		require.Equal(t, uint64(16), uintVal(t, mustChild(t, ch, "Data").Child("Value Size")))
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, ch, "Data").Child("Name")))
	})

	t.Run("tacacs/rfc8907", func(t *testing.T) {
		tac := []byte{
			0xc0, 0x01, 0x01, 0x01,
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x00, 0x00, 0x0d,
			0x01, 0x01, 0x01, 0x01,
			0x05, 0x00, 0x00, 0x00,
			'a', 'd', 'm', 'i', 'n',
		}
		ta := parseRule(t, tac, "tacacs", "TACACS")
		require.Equal(t, uint64(1), uintVal(t, ta.Child("Type")))
		require.Equal(t, uint64(5), uintVal(t, ta.Child("User Len")))
		require.Equal(t, "admin", strVal(t, ta.Child("User")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 49, 49, tac))
		require.Equal(t, "admin", strVal(t, mustChild(t, eth, "IP", "TCP", "TACACS").Child("User")))
	})

	t.Run("l2tp/rfc2661", func(t *testing.T) {
		l2 := mustHex(t, "c802001400010000000000000008000000000001")
		eth := parseEthernet(t, ipv4UDPBytes(t, 1701, 1701, l2))
		l := mustChild(t, eth, "IP", "UDP", "L2TP")
		require.Equal(t, uint64(1), uintVal(t, l.Child("Tunnel ID")))
		avps := l.Child("AVPs").Children()
		require.GreaterOrEqual(t, len(avps), 1)
		require.Equal(t, uint64(0), uintVal(t, avps[0].Child("Attribute Type")))
	})

	t.Run("l2tp/data-ppp", func(t *testing.T) {
		// RFC 2661 data 0202 0014 0001 0002 … (O-bit): Tunnel 0x0014, not Length-present Tunnel=1.
		l2 := mustHex(t, "02020014000100020000ff03002d")
		n := parseRule(t, l2, "l2tp", "L2TP")
		require.Equal(t, uint64(0x0014), uintVal(t, n.Child("Tunnel ID")))
		require.NotEqual(t, uint64(1), uintVal(t, n.Child("Tunnel ID")))
		require.Equal(t, uint64(2), uintVal(t, n.Child("Offset Size")))
		require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, n, "PPP").Child("Protocol")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 1701, 1701, l2))
		require.Equal(t, uint64(0x0014), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP").Child("Tunnel ID")))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP").Child("Offset Size")))
		require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, eth, "IP", "UDP", "L2TP", "PPP").Child("Protocol")))
	})

	t.Run("ike/sa-init", func(t *testing.T) {
		// RFC 7296 §3.3 SA + Wireshark test/captures ikev2 initiator SPI layout.
		// IKE_SA_INIT: SA proposal proto IKE, PRF HMAC_SHA1 transform.
		ike := mustHex(t, "88694881497528ad0000000000000000212022080000000000000048"+
			"2200001400000010010100010000000802000002"+
			"28000010000200000102030405060708"+
			"00000008aabbccdd")
		ik := parseRule(t, ike, "ike", "IKE")
		require.Equal(t, uint64(0x22), uintVal(t, ik.Child("Exchange Type")))
		sa := mustChild(t, ik, "Payloads").Children()[0]
		prop := mustChild(t, sa, "Proposals").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, prop.Child("Protocol ID")))
		xf := mustChild(t, prop, "Transforms").Children()[0]
		require.Equal(t, uint64(2), uintVal(t, xf.Child("Transform Type")))
		require.Equal(t, uint64(2), uintVal(t, xf.Child("Transform ID")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 500, 500, ike))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "IKE", "Payloads").Children()[0].Child("Proposals").Children()[0].Child("Transforms").Children()[0].Child("Transform ID")))
	})

	t.Run("dtls/handshake", func(t *testing.T) {
		// RFC 6347 §4.2.2 DTLS handshake header inside record fragment (type 22).
		dt := mustHex(t, "16fefd000000000000000000360100002a000000000000002afefd"+
			"000000000000000000000000000000000000000000000000000000000000000000000002002f0100")
		n := parseRule(t, dt, "dtls", "DTLS")
		require.Equal(t, uint64(0x16), uintVal(t, n.Child("Content Type")))
		fr := n.Child("Fragment")
		require.Equal(t, uint64(1), uintVal(t, fr.Child("Handshake Type")))
		require.Equal(t, uint64(0), uintVal(t, fr.Child("Message Seq")))
		require.Equal(t, uint64(42), uintVal(t, fr.Child("Fragment Length")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 443, 443, dt))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "DTLS", "Fragment").Child("Handshake Type")))
	})

	t.Run("dtls/client-hello", func(t *testing.T) {
		dt := mustHex(t, "16fefd000000000000000000360100002a000000000000002afefd"+
			"000000000000000000000000000000000000000000000000000000000000000000000002002f0100")
		eth := parseEthernet(t, ipv4UDPBytes(t, 443, 443, dt))
		require.Equal(t, uint64(0xfefd), uintVal(t, mustChild(t, eth, "IP", "UDP", "DTLS", "Fragment").Child("Client Version")))
	})

	t.Run("rtp/extension", func(t *testing.T) {
		// RFC 3550 §5.3.1 X bit + RFC 5285 one-byte header extension profile 0xBEDE.
		rtp := mustHex(t, "900000010000000200000003bede000110000000aa")
		n := parseRule(t, rtp, "rtp", "RTP")
		require.Equal(t, uint64(0xbede), uintVal(t, n.Child("Ext Profile")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Ext Length")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5004, 5004, rtp))
		require.Equal(t, uint64(0xbede), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTP").Child("Ext Profile")))
	})

	t.Run("rtp/seq", func(t *testing.T) {
		rtp := []byte{0x80, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0xaa}
		eth := parseEthernet(t, ipv4UDPBytes(t, 5004, 5004, rtp))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTP").Child("Sequence")))
	})

	t.Run("ike/ke-nonce", func(t *testing.T) {
		// RFC 7296 §3.4 KE DH group 2 + §3.9 Nonce.
		ike := mustHex(t, "88694881497528ad0000000000000000212022080000000000000048"+
			"2200001400000010010100010000000802000002"+
			"28000010000200000102030405060708"+
			"00000008aabbccdd")
		eth := parseEthernet(t, ipv4UDPBytes(t, 500, 500, ike))
		pl := mustChild(t, eth, "IP", "UDP", "IKE", "Payloads").Children()
		require.Equal(t, uint64(2), uintVal(t, pl[1].Child("DH Group")))
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, pl[1].Child("Key Exchange")))
		require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0xdd}, bytesVal(t, pl[2].Child("Nonce")))
	})

	t.Run("amqp/connection-start", func(t *testing.T) {
		// AMQP 0-9-1 §4.2.4 Connection.Start (class 10 method 10):
		// version 0.9, server-properties table {product: "RabbitMQ"},
		// mechanisms longstr PLAIN, locales longstr en_US, frame-end 0xce.
		frame := mustHex(t, "01000000000031000a000a0009000000150770726f6475637453000000085261626269744d5100000005504c41494e00000005656e5f5553ce")
		am := parseRule(t, frame, "amqp", "AMQP")
		require.Equal(t, uint64(1), uintVal(t, am.Child("Type")))
		require.Equal(t, uint64(10), uintVal(t, am.Child("Class ID")))
		require.Equal(t, uint64(10), uintVal(t, am.Child("Method ID")))
		require.Equal(t, uint64(0), uintVal(t, am.Child("Version Major")))
		require.Equal(t, uint64(9), uintVal(t, am.Child("Version Minor")))
		require.Equal(t, uint64(0xce), uintVal(t, am.Child("Frame End")))
		ent := mustChild(t, am, "Properties", "AMQPTable", "Entries").Children()
		require.Equal(t, "product", strVal(t, ent[0].Child("Name")))
		require.Equal(t, "RabbitMQ", strVal(t, ent[0].Child("Str")))
		require.Equal(t, "PLAIN", strVal(t, mustChild(t, am, "Mechanisms").Child("Value")))
		require.Equal(t, "en_US", strVal(t, mustChild(t, am, "Locales").Child("Value")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5672, 5672, frame))
		require.Equal(t, "PLAIN", strVal(t, mustChild(t, eth, "IP", "TCP", "AMQP", "Mechanisms").Child("Value")))
	})

	t.Run("amqp/nested-table", func(t *testing.T) {
		// Connection.Start with nested capabilities table (F) + tags array (A).
		frame := mustHex(t, "0100000000006a000a000a00090000004e0770726f6475637453000000085261626269744d510c6361706162696c69746965734600000015127075626c69736865725f636f6e6669726d73740104746167734100000008530000000367656e00000005504c41494e00000005656e5f5553ce")
		am := parseRule(t, frame, "amqp", "AMQP")
		ent := mustChild(t, am, "Properties", "AMQPTable", "Entries").Children()
		require.GreaterOrEqual(t, len(ent), 3)
		require.Equal(t, "capabilities", strVal(t, ent[1].Child("Name")))
		caps := mustChild(t, ent[1], "AMQPTable", "Entries").Children()
		require.Equal(t, "publisher_confirms", strVal(t, caps[0].Child("Name")))
		require.Equal(t, uint64(1), uintVal(t, caps[0].Child("Bool")))
		require.Equal(t, "tags", strVal(t, ent[2].Child("Name")))
		tags := mustChild(t, ent[2], "AMQPArray", "Values").Children()
		require.Equal(t, "gen", strVal(t, tags[0].Child("Str")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5672, 5672, frame))
		require.Equal(t, "publisher_confirms", strVal(t, mustChild(t, eth, "IP", "TCP", "AMQP", "Properties", "AMQPTable", "Entries").Children()[1].Child("AMQPTable").Child("Entries").Children()[0].Child("Name")))
	})

	t.Run("amqp/field-types", func(t *testing.T) {
		// AMQP 0-9-1 §4.2.4 field-value: S longstr, s shortstr, I long, T timestamp, V void.
		frame := mustHex(t, "01000000000065000a000a0009000000490770726f6475637453000000085261626269744d5108706c6174666f726d73054c696e75780b6368616e6e656c2d6d617849000007ff036e6f77540000000000000001046e6f6e655600000005504c41494e00000005656e5f5553ce")
		am := parseRule(t, frame, "amqp", "AMQP")
		ent := mustChild(t, am, "Properties", "AMQPTable", "Entries").Children()
		require.GreaterOrEqual(t, len(ent), 5)
		require.Equal(t, "platform", strVal(t, ent[1].Child("Name")))
		require.Equal(t, "Linux", strVal(t, ent[1].Child("Short")))
		require.Equal(t, "channel-max", strVal(t, ent[2].Child("Name")))
		require.Equal(t, int64(2047), intVal(t, ent[2].Child("Long")))
		require.Equal(t, "now", strVal(t, ent[3].Child("Name")))
		require.Equal(t, uint64(1), uintVal(t, ent[3].Child("Timestamp")))
		require.Equal(t, "none", strVal(t, ent[4].Child("Name")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5672, 5672, frame))
		require.Equal(t, "Linux", strVal(t, mustChild(t, eth, "IP", "TCP", "AMQP", "Properties", "AMQPTable", "Entries").Children()[1].Child("Short")))
	})

	t.Run("thrift/i32-field", func(t *testing.T) {
		// Binary protocol: version 0x80010001, empty name, seq 1, field type i32 id 1 value 7, STOP
		th := mustHex(t, "8001000100000000000000010800010000000700")
		n := parseEthernet(t, ipv4TCPFrame(t, 9090, 9090, th))
		tf := mustChild(t, n, "IP", "TCP", "Thrift")
		require.Equal(t, uint64(1), uintVal(t, tf.Child("Seq ID")))
		f := tf.Child("Fields").Children()[0]
		require.Equal(t, uint64(8), uintVal(t, f.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, f.Child("Field ID")))
		require.Equal(t, uint64(7), uintVal(t, f.Child("I32")))
	})

	t.Run("mongodb/bson-ping", func(t *testing.T) {
		mongo := mustHex(t, "360000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"0f0000001070696e67000100000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_QUERY", "BSONDoc", "Elements").Children()
		require.Equal(t, "ping", strVal(t, el[0].Child("Name")))
		require.Equal(t, uint64(1), uintVal(t, el[0].Child("Int32")))
	})

	t.Run("mongodb/op-msg", func(t *testing.T) {
		// OP_MSG (2013) section kind 0 + BSON {ping:1}
		msg := mustHex(t, "240000000100000000000000dd0700000000000000"+
			"0f0000001070696e67000100000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, msg))
		require.Equal(t, uint64(2013), uintVal(t, mustChild(t, eth, "IP", "TCP", "MongoDB").Child("Op Code")))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_MSG", "BSONDoc", "Elements").Children()
		require.Equal(t, "ping", strVal(t, el[0].Child("Name")))
	})

	t.Run("mongodb/bson-string", func(t *testing.T) {
		// OP_QUERY admin.$cmd {msg: "hi"} BSON type 0x02
		mongo := mustHex(t, "380000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"11000000026d7367000300000068690000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_QUERY", "BSONDoc", "Elements").Children()
		require.Equal(t, "msg", strVal(t, el[0].Child("Name")))
		require.Equal(t, "hi", strings.TrimRight(strVal(t, el[0].Child("Str")), "\x00"))
	})

	t.Run("mongodb/bson-int64", func(t *testing.T) {
		// OP_QUERY admin.$cmd {n: int64(2)} BSON type 0x12
		mongo := mustHex(t, "370000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"10000000126e00020000000000000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_QUERY", "BSONDoc", "Elements").Children()
		require.Equal(t, "n", strVal(t, el[0].Child("Name")))
		require.Equal(t, uint64(2), uintVal(t, el[0].Child("Int64")))
	})

	t.Run("mongodb/bson-nested", func(t *testing.T) {
		// OP_QUERY {filter: {n: 1}} BSON type 0x03 embedded document
		mongo := mustHex(t, "400000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"190000000366696c746572000c000000106e00010000000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_QUERY", "BSONDoc", "Elements").Children()
		require.Equal(t, "filter", strVal(t, el[0].Child("Name")))
		inner := mustChild(t, el[0], "BSONDoc", "Elements").Children()
		require.Equal(t, "n", strVal(t, inner[0].Child("Name")))
		require.Equal(t, uint64(1), uintVal(t, inner[0].Child("Int32")))
	})

	t.Run("mongodb/bson-array", func(t *testing.T) {
		// OP_QUERY {a: [2]} BSON type 0x04 array
		mongo := mustHex(t, "3b0000000100000000000000d407000000000000"+
			"61646d696e2e24636d64000000000001000000"+
			"140000000461000c000000103000020000000000")
		eth := parseEthernet(t, ipv4TCPFrame(t, 27017, 27017, mongo))
		el := mustChild(t, eth, "IP", "TCP", "MongoDB", "OP_QUERY", "BSONDoc", "Elements").Children()
		require.Equal(t, "a", strVal(t, el[0].Child("Name")))
		inner := mustChild(t, el[0], "BSONDoc", "Elements").Children()
		require.Equal(t, "0", strVal(t, inner[0].Child("Name")))
		require.Equal(t, uint64(2), uintVal(t, inner[0].Child("Int32")))
	})

	t.Run("memcached/get-key", func(t *testing.T) {
		// Binary GET key "a" (Couchbase binary protocol)
		mc := mustHex(t, "80000001000000000000000100000000000000000000000061")
		m := parseRule(t, mc, "memcached", "Memcached")
		require.Equal(t, uint64(1), uintVal(t, m.Child("Key Length")))
		require.Equal(t, "a", strVal(t, m.Child("Key")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 11211, 11211, mc))
		require.Equal(t, "a", strVal(t, mustChild(t, eth, "IP", "TCP", "Memcached").Child("Key")))
	})

	t.Run("fastcgi/begin-request", func(t *testing.T) {
		// FastCGI BEGIN_REQUEST Role=responder (1) (spec 3.3)
		fc := mustHex(t, "01010001000800000001000000000000")
		n := parseEthernet(t, ipv4TCPFrame(t, 9000, 9000, fc))
		br := mustChild(t, n, "IP", "TCP", "FastCGI", "BEGIN_REQUEST")
		require.Equal(t, uint64(1), uintVal(t, br.Child("Role")))
	})

	t.Run("fastcgi/params", func(t *testing.T) {
		// FastCGI PARAMS (type 4) name/value SCRIPT_NAME=/
		fc := mustHex(t, "01040001000e00000b015343524950545f4e414d452f")
		n := parseEthernet(t, ipv4TCPFrame(t, 9000, 9000, fc))
		pair := mustChild(t, n, "IP", "TCP", "FastCGI", "Params").Children()[0]
		require.Equal(t, "SCRIPT_NAME", strVal(t, pair.Child("Name")))
		require.Equal(t, "/", strVal(t, pair.Child("Value")))
	})

	t.Run("zabbix/json", func(t *testing.T) {
		zb := append([]byte("ZBXD"), 0x01, 0x02, 0x00, 0x00, 0x00, '{', '}')
		z := parseRule(t, zb, "zabbix", "Zabbix")
		require.Equal(t, "{}", strVal(t, z.Child("JSON")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 10050, 10050, zb))
		require.Equal(t, "{}", strVal(t, mustChild(t, eth, "IP", "TCP", "Zabbix").Child("JSON")))
	})

	t.Run("pptp/sccrq", func(t *testing.T) {
		pptp := make([]byte, 156)
		pptp[1] = 156
		pptp[3] = 1
		pptp[4], pptp[5], pptp[6], pptp[7] = 0x1a, 0x2b, 0x3c, 0x4d
		pptp[9] = 1
		pptp[13] = 1
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		require.Equal(t, uint64(0x1a2b3c4d), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP").Child("MagicCookie")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP").Child("ProtocolVersion")))
	})

	t.Run("eap/identity", func(t *testing.T) {
		// RFC 3748 §5.1: Length=5 Request/Identity has no Type-Data.
		req := []byte{0x01, 0x00, 0x00, 0x05, 0x01, 0x01, 0x00, 0x05, 0x01}
		require.Nil(t, mustChild(t, parseRule(t, req, "eapol", "EAPOL"), "EAPPacket").Child("Identity"))
		// RFC 3748 §5.1 EAP-Response/Identity Type-Data "anonymous" (Length>5).
		eap := append([]byte{0x01, 0x00, 0x00, 0x0e, 0x02, 0x01, 0x00, 0x0e, 0x01}, []byte("anonymous")...)
		ep := parseRule(t, eap, "eapol", "EAPOL")
		pkt := mustChild(t, ep, "EAPPacket")
		require.Equal(t, uint64(1), uintVal(t, pkt.Child("Type")))
		require.Equal(t, "anonymous", strVal(t, pkt.Child("Identity")))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		require.Equal(t, "anonymous", strVal(t, mustChild(t, eth, "EAPOL", "EAPPacket").Child("Identity")))
	})

	t.Run("jdwp/command-set", func(t *testing.T) {
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001000101")...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("Command Set")))
	})

	t.Run("net-remoting/preamble", func(t *testing.T) {
		nr := make([]byte, 14)
		copy(nr, []byte(".NET"))
		nr[5] = 1
		n := parseRule(t, nr, "net_remoting", "NetRemoting")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Major")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 8088, 8088, nr))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "NetRemoting").Child("Major")))
	})

	t.Run("net-remoting/nrbf", func(t *testing.T) {
		// MS-NRBF SerializedStreamHeader after .NET TCP preamble (ContentLength 17).
		nr := mustHex(t, "2e4e4554000100000000000000110001000000ffffffff0100000000000000")
		n := parseRule(t, nr, "net_remoting", "NetRemoting")
		require.Equal(t, uint64(17), uintVal(t, n.Child("Content Length")))
		ser := mustChild(t, n, "Serialized")
		require.Equal(t, uint64(0), uintVal(t, ser.Child("Record Type")))
		require.Equal(t, int64(1), intVal(t, ser.Child("Root Id")))
		require.Equal(t, int64(1), intVal(t, ser.Child("Format Major")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 8088, 8088, nr))
		require.Equal(t, int64(1), intVal(t, mustChild(t, eth, "IP", "TCP", "NetRemoting", "Serialized").Child("Root Id")))
	})

	t.Run("nbt-dg/names", func(t *testing.T) {
		// RFC 1002 Direct Unique Datagram: SOURCE / *SMBSERVER + user data "hi"
		nb := mustHex(t, "100000010a000001008a00220000534f55524345202020202020202020202a534d425345525645522020202020206869")
		n := parseEthernet(t, ipv4UDPBytes(t, 138, 138, nb))
		dg := mustChild(t, n, "IP", "UDP", "NBTDG", "Datagram")
		require.Equal(t, "SOURCE", strings.TrimSpace(strVal(t, dg.Child("Source Name"))))
		require.Equal(t, "*SMBSERVER", strings.TrimSpace(strVal(t, dg.Child("Dest Name"))))
		require.Equal(t, "hi", strVal(t, dg.Child("User Data")))
	})
}
