package bin_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1WiresharkAndRFCSamples(t *testing.T) {
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
		require.Equal(t, "POP3 server ready", strVal(t, po.Child("Rest")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, popGreet))
		require.Equal(t, "+OK", strVal(t, mustChild(t, eth, "IP", "TCP", "POP3").Child("Status")))
	})

	t.Run("imap/rfc3501", func(t *testing.T) {
		imapGreet := []byte("* OK IMAP4rev1 server ready\r\n")
		im := parseRule(t, imapGreet, "imap", "IMAP")
		require.Equal(t, "*", strVal(t, im.Child("Tag")))
		require.Equal(t, "OK", strVal(t, im.Child("Command")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 143, 143, imapGreet))
		require.Equal(t, "IMAP4rev1 server ready", strVal(t, mustChild(t, eth, "IP", "TCP", "IMAP").Child("Rest")))
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
}
