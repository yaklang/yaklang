package bin_parser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP1WiresharkAndRFCSamples(t *testing.T) {
	t.Run("ssh/kexinit", func(t *testing.T) {
		// RFC 4253 §7.1 SSH_MSG_KEXINIT; Wireshark ssh.kex.algorithms.
		raw := mustHex(t, "000000950414000102030405060708090a0b0c0d0e0f00000011637572766532353531392d7368613235360000000b7373682d656432353531390000000a6165733132382d6374720000000a6165733132382d6374720000000d686d61632d736861322d3235360000000d686d61632d736861322d323536000000046e6f6e65000000046e6f6e650000000000000000000000000000000000")
		n := parseRule(t, raw, "application-layer.ssh", "SSHPacket")
		require.Equal(t, uint64(20), uintVal(t, mustChild(t, n, "Payload").Child("Message Number")))
		kex := mustChild(t, n, "Payload", "SSHKexInit")
		require.Equal(t, "curve25519-sha256", strVal(t, kex.Child("Kex Algos")))
		require.Equal(t, "ssh-ed25519", strVal(t, kex.Child("Host Key Algos")))
		require.Equal(t, "aes128-ctr", strVal(t, kex.Child("Enc C2S")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, raw))
		wired := mustChild(t, eth, "IP", "TCP", "SSHPacket", "Payload", "SSHKexInit")
		require.Equal(t, "curve25519-sha256", strVal(t, wired.Child("Kex Algos")))
	})

	t.Run("ssh/kexdh", func(t *testing.T) {
		// RFC 4253 §8 SSH_MSG_KEXDH_INIT (30) mpint e.
		raw := mustHex(t, "0000000b041e000000010200000000")
		n := parseRule(t, raw, "application-layer.ssh", "SSHPacket")
		require.Equal(t, uint64(30), uintVal(t, mustChild(t, n, "Payload").Child("Message Number")))
		dh := mustChild(t, n, "Payload", "SSHKexDHInit")
		require.Equal(t, uint64(1), uintVal(t, dh.Child("E Length")))
		require.Equal(t, []byte{2}, bytesVal(t, dh.Child("E")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 22, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "SSHPacket", "Payload", "SSHKexDHInit").Child("E Length")))
	})

	t.Run("dhcp/discover", func(t *testing.T) {
		// RFC 2131 DHCPDISCOVER + RFC 2132 §9.6 option 53 type 1, §3.14 option 12 Host Name.
		// Wireshark dhcp.option.dhcp / dhcp.option.hostname.
		raw := mustHex(t, "010106001234567800000000"+
			"00000000000000000000000000000000"+
			"123456789abc00000000000000000000"+
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"638253633501010c0b6578616d706c652e636f6dff")
		n := parseRule(t, raw, "application-layer.dhcp", "DHCP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Operation")))
		opts := n.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 2)
		require.Equal(t, uint64(1), uintVal(t, opts[0].Child("Message Type")))
		require.Equal(t, "example.com", strVal(t, opts[1].Child("Host Name")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 68, 67, raw))
		wired := mustChild(t, eth, "IP", "UDP", "DHCP")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("Options").Children()[0].Child("Message Type")))
		require.Equal(t, "example.com", strVal(t, wired.Child("Options").Children()[1].Child("Host Name")))
	})

	t.Run("dhcp/offer", func(t *testing.T) {
		// RFC 2131 DHCPOFFER + RFC 2132 §9.6 type 2, §9.7 Server Identifier 192.168.0.1.
		raw := mustHex(t, "02010600aabbccdd00000000"+
			"00000000c0a8007bc0a8000100000000"+
			"123456789abc00000000000000000000"+
			"00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"+
			"638253633501023604c0a80001ff")
		n := parseRule(t, raw, "application-layer.dhcp", "DHCP")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Operation")))
		require.Equal(t, []byte{192, 168, 0, 123}, bytesVal(t, n.Child("Your IP")))
		opts := n.Child("Options").Children()
		require.Equal(t, uint64(2), uintVal(t, opts[0].Child("Message Type")))
		require.Equal(t, []byte{192, 168, 0, 1}, bytesVal(t, opts[1].Child("Server ID")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 67, 68, raw))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "UDP", "DHCP").Child("Options").Children()[0].Child("Message Type")))
	})

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

	t.Run("stun/binding-req", func(t *testing.T) {
		// RFC 5769 §2.1 Binding Request: SOFTWARE, PRIORITY, USERNAME "evtj:h6vY".
		stun := mustHex(t, ""+
			"00010058 2112a442 b7e7a701 bc34d686 fa87dfae "+
			"80220010 5354554e 20746573 7420636c 69656e74 "+
			"00240004 6e0001ff 80290008 932ff9b1 51263b36 "+
			"00060009 6576746a 3a683676 59202020 "+
			"00080014 9aeaa70c bfd8cb56 781ef2b5 b2d3f249 c1b571a2 "+
			"80280004 e57a3bcf")
		n := parseRule(t, stun, "stun", "STUN")
		attrs := n.Child("Attributes").Children()
		require.Equal(t, "STUN test client", strVal(t, attrs[0].Child("Software")))
		require.Equal(t, uint64(0x6e0001ff), uintVal(t, attrs[1].Child("Priority")))
		require.Equal(t, "evtj:h6vY", strVal(t, attrs[3].Child("Username")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 3478, 3478, stun))
		require.Equal(t, "STUN test client", strVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Attributes").Children()[0].Child("Software")))
		require.Equal(t, "evtj:h6vY", strVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Attributes").Children()[3].Child("Username")))
	})

	t.Run("stun/binding-success", func(t *testing.T) {
		// RFC 5769 §2.2 XOR-MAPPED-ADDRESS: 192.0.2.1:32853 (X-Port 0xa147, X-Address e112a643).
		succ := mustHex(t, "0101000c2112a442b7e7a701bc34d686fa87dfae002000080001a147e112a643")
		n := parseRule(t, succ, "stun", "STUN")
		require.Equal(t, uint64(0x0101), uintVal(t, n.Child("Message Type")))
		xa := n.Child("Attributes").Children()[0]
		require.Equal(t, uint64(0x0020), uintVal(t, xa.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, xa.Child("Family")))
		require.Equal(t, uint64(0xa147), uintVal(t, xa.Child("X-Port")))
		require.Equal(t, []byte{0xe1, 0x12, 0xa6, 0x43}, bytesVal(t, xa.Child("X-Address")))
		port := uint16(0xa147) ^ 0x2112
		require.Equal(t, uint16(32853), port)
		eth := parseEthernet(t, ipv4UDPBytes(t, 3478, 3478, succ))
		require.Equal(t, uint64(0xa147), uintVal(t, mustChild(t, eth, "IP", "UDP", "STUN").Child("Attributes").Children()[0].Child("X-Port")))
	})

	t.Run("kafka/metadata", func(t *testing.T) {
		// Apache Kafka protocol.html Request Header v1 client_id NULLABLE_STRING;
		// Metadata Request v0 API key 3 [topics] STRING (Wireshark packet-kafka.c kafka_get_string).
		// Size 23, Client ID "test", one topic "foo" (same layout as the protocol-guide
		// 00000012…74657374ffffffff all-topics request plus one STRING topic).
		kf := mustHex(t, "00000017 0003 0000 00000001 0004 74657374 00000001 0003 666f6f")
		n := parseRule(t, kf, "kafka", "Kafka")
		require.Equal(t, int64(23), intVal(t, n.Child("Length")))
		require.Equal(t, int64(3), intVal(t, n.Child("API Key")))
		require.Equal(t, int64(1), intVal(t, n.Child("Correlation ID")))
		require.Equal(t, int64(4), intVal(t, n.Child("Client ID Len")))
		require.Equal(t, "test", strVal(t, n.Child("Client ID")))
		require.Equal(t, int64(1), intVal(t, n.Child("Topics Count")))
		topics := n.Child("Topics").Children()
		require.Equal(t, 1, len(topics))
		require.Equal(t, int64(3), intVal(t, topics[0].Child("Name Len")))
		require.Equal(t, "foo", strVal(t, topics[0].Child("Name")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 9092, 9092, kf))
		k := mustChild(t, eth, "IP", "TCP", "Kafka")
		require.Equal(t, "test", strVal(t, k.Child("Client ID")))
		require.Equal(t, "foo", strVal(t, k.Child("Topics").Children()[0].Child("Name")))
	})

	t.Run("kafka/apiversions", func(t *testing.T) {
		// Apache Kafka protocol.html ApiVersions API key 18 v0: empty body, NULLABLE_STRING client_id -1.
		kf := mustHex(t, "0000000a0012000000000000ffff")
		n := parseRule(t, kf, "kafka", "Kafka")
		require.Equal(t, int64(10), intVal(t, n.Child("Length")))
		require.Equal(t, int64(18), intVal(t, n.Child("API Key")))
		require.Equal(t, int64(-1), intVal(t, n.Child("Client ID Len")))
		require.Nil(t, n.Child("Client ID"))
		require.Nil(t, n.Child("Topics Count"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 9092, 9092, kf))
		require.Equal(t, int64(18), intVal(t, mustChild(t, eth, "IP", "TCP", "Kafka").Child("API Key")))
		require.Nil(t, mustChild(t, eth, "IP", "TCP", "Kafka").Child("Client ID"))
	})

	t.Run("protobuf/varint", func(t *testing.T) {
		// protobuf.dev encoding: int32 field 1 = 150 is 08 96 01 (Wireshark packet-protobuf.c tag/varint).
		pb := mustHex(t, "08 96 01")
		n := parseRule(t, pb, "protobuf", "Protobuf")
		f := n.Child("Fields").Children()[0]
		require.Equal(t, uint64(0x08), uintVal(t, f.Child("Tag")))
		require.Equal(t, uint64(0x96), uintVal(t, f.Child("Varint")))
		require.Equal(t, uint64(0x01), uintVal(t, f.Child("Varint2")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 4011, 4011, pb))
		ef := mustChild(t, eth, "IP", "TCP", "Protobuf", "Fields").Children()[0]
		require.Equal(t, uint64(0x96), uintVal(t, ef.Child("Varint")))
		require.Equal(t, uint64(0x01), uintVal(t, ef.Child("Varint2")))
	})

	t.Run("protobuf/string", func(t *testing.T) {
		// protobuf.dev encoding: string field 2 = "testing" is 12 07 74 65 73 74 69 6e 67.
		pb := append([]byte{0x12, 0x07}, []byte("testing")...)
		n := parseRule(t, pb, "protobuf", "Protobuf")
		f := n.Child("Fields").Children()[0]
		require.Equal(t, uint64(0x12), uintVal(t, f.Child("Tag")))
		require.Equal(t, uint64(7), uintVal(t, f.Child("Len")))
		require.Equal(t, "testing", strVal(t, f.Child("Str")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 4011, 4011, pb))
		require.Equal(t, "testing", strVal(t, mustChild(t, eth, "IP", "TCP", "Protobuf", "Fields").Children()[0].Child("Str")))
	})

	t.Run("iiop/locate", func(t *testing.T) {
		// CORBA 3.1 Part 2 §9.4.5 LocateRequestHeader_1_2 KeyAddr; Wireshark packet-giop.c giop.objektkey.
		// In-tree WebLogic CosNaming LocateRequest, object_key "NameService".
		giop := mustHex(t, "47494f50010200030000001700000002000000000000000b4e616d6553657276696365")
		n := parseRule(t, giop, "application-layer.iiop", "GIOP")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Message Type")))
		require.Equal(t, uint64(23), uintVal(t, n.Child("Message Size")))
		lr := mustChild(t, n, "LocateRequest")
		require.Equal(t, uint64(2), uintVal(t, lr.Child("Request ID")))
		require.Equal(t, uint64(0), uintVal(t, lr.Child("Addr Disc")))
		require.Equal(t, uint64(11), uintVal(t, lr.Child("Key Len")))
		require.Equal(t, "NameService", strVal(t, lr.Child("Object Key")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 2809, 2809, giop))
		require.Equal(t, "NameService", strVal(t, mustChild(t, eth, "IP", "TCP", "GIOP", "LocateRequest").Child("Object Key")))
	})

	t.Run("iiop/request", func(t *testing.T) {
		// CORBA 3.1 RequestHeader_1_2: KeyAddr, operation "_is_a" (NUL-terminated CDR string).
		giop := mustHex(t, "47494f50010200000000001a00000001030000000000000000000000000000065f69735f6100")
		n := parseRule(t, giop, "application-layer.iiop", "GIOP")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Message Type")))
		req := mustChild(t, n, "GIOPRequest")
		require.Equal(t, uint64(1), uintVal(t, req.Child("Request ID")))
		require.Equal(t, uint64(3), uintVal(t, req.Child("Response Flags")))
		require.Equal(t, uint64(6), uintVal(t, req.Child("Op Len")))
		require.Equal(t, "_is_a", strings.TrimRight(strVal(t, req.Child("Operation")), "\x00"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 2809, 2809, giop))
		require.Equal(t, "_is_a", strings.TrimRight(strVal(t, mustChild(t, eth, "IP", "TCP", "GIOP", "GIOPRequest").Child("Operation")), "\x00"))
	})

	t.Run("t3/hello", func(t *testing.T) {
		// WebLogic T3 handshake (Wireshark tcp.port==7001; SANS ISC t3 12.2.1): AS:255 HL:19.
		hello := []byte("t3 12.2.1\nAS:255\nHL:19\nMS:10000000\n\n")
		n := parseRule(t, hello, "application-layer.t3", "T3")
		h := mustChild(t, n, "T3Hello")
		require.Equal(t, "t3", strVal(t, h.Child("Proto")))
		require.Equal(t, "12.2.1", strVal(t, h.Child("Version")))
		require.Equal(t, "255", strVal(t, h.Child("AS")))
		require.Equal(t, "19", strVal(t, h.Child("HL")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 7001, 7001, hello))
		require.Equal(t, "12.2.1", strVal(t, mustChild(t, eth, "IP", "TCP", "T3", "T3Hello").Child("Version")))
		require.Equal(t, "19", strVal(t, mustChild(t, eth, "IP", "TCP", "T3", "T3Hello").Child("HL")))
	})

	t.Run("t3/identify", func(t *testing.T) {
		// WebLogic T3 binary header HL:19: Cmd/Qos/Flags + ResponseId/InvokableId/AbbrevOffset
		// (parse_test.go _TestT3 000005be016501ffffffff… layout, length 19).
		pkt := make([]byte, 19)
		pkt[3] = 19
		pkt[4] = 1    // Cmd
		pkt[5] = 0x65 // Qos (in-tree IDENTIFY-style)
		pkt[6] = 1    // Flags
		n := parseRule(t, pkt, "application-layer.t3", "T3")
		id := mustChild(t, n, "T3Identify")
		require.Equal(t, uint64(19), uintVal(t, id.Child("Total Length")))
		require.Equal(t, uint64(1), uintVal(t, id.Child("Cmd")))
		require.Equal(t, uint64(0x65), uintVal(t, id.Child("Qos")))
		require.Equal(t, int64(0), intVal(t, id.Child("ResponseId")))
		require.Equal(t, int64(0), intVal(t, id.Child("AbbrevOffset")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 7001, 7001, pkt))
		eid := mustChild(t, eth, "IP", "TCP", "T3", "T3Identify")
		require.Equal(t, uint64(0x65), uintVal(t, eid.Child("Qos")))
		require.Equal(t, int64(0), intVal(t, eid.Child("InvokableId")))
	})

	t.Run("quic/initial", func(t *testing.T) {
		// RFC 9001 Appendix A.2 Client Initial (truncated): DCID 8394c8f03e515708.
		// Wireshark packet-quic.c quic.dcid; golang.org/x/net internal/quic packet_test rfc9001_a1.
		initp := mustHex(t, "c000000001088394c8f03e51570800")
		n := parseRule(t, initp, "application-layer.quic", "QUIC")
		require.Equal(t, uint64(0xc0), uintVal(t, n.Child("First Byte")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(8), uintVal(t, n.Child("DCID Length")))
		require.Equal(t, []byte{0x83, 0x94, 0xc8, 0xf0, 0x3e, 0x51, 0x57, 0x08}, bytesVal(t, n.Child("DCID")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("SCID Length")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 50000, 443, initp))
		require.Equal(t, []byte{0x83, 0x94, 0xc8, 0xf0, 0x3e, 0x51, 0x57, 0x08}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "QUIC").Child("DCID")))
	})

	t.Run("quic/server-initial", func(t *testing.T) {
		// RFC 9001 Appendix A.3 Server Initial (truncated): SCID f067a5502a4262b5, DCID empty.
		srv := mustHex(t, "cf000000010008f067a5502a4262b500")
		n := parseRule(t, srv, "application-layer.quic", "QUIC")
		require.Equal(t, uint64(0), uintVal(t, n.Child("DCID Length")))
		require.Equal(t, uint64(8), uintVal(t, n.Child("SCID Length")))
		require.Equal(t, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, bytesVal(t, n.Child("SCID")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 443, 50000, srv))
		require.Equal(t, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "QUIC").Child("SCID")))
	})

	t.Run("ospf/hello", func(t *testing.T) {
		// gopacket layers/ospf_test.go OSPFv2 Hello: mask 255.255.255.0, interval 10, dead 40, DR 192.168.170.8.
		hello := mustHex(t, "0201002cc0a8aa0800000001273b00000000000000000000ffffff00000a020100000028c0a8aa0800000000")
		n := parseRule(t, hello, "ospf", "OSPF")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Type")))
		h := mustChild(t, n, "OSPFHello")
		require.Equal(t, []byte{0xff, 0xff, 0xff, 0x00}, bytesVal(t, h.Child("Network Mask")))
		require.Equal(t, uint64(10), uintVal(t, h.Child("Hello Interval")))
		require.Equal(t, uint64(1), uintVal(t, h.Child("Priority")))
		require.Equal(t, uint64(40), uintVal(t, h.Child("Dead Interval")))
		require.Equal(t, []byte{0xc0, 0xa8, 0xaa, 0x08}, bytesVal(t, h.Child("Designated Router")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 89, hello))
		require.Equal(t, uint64(10), uintVal(t, mustChild(t, eth, "IP", "OSPF", "OSPFHello").Child("Hello Interval")))
	})

	t.Run("ospf/dbd", func(t *testing.T) {
		// RFC 2328 §A.3.3 Database Description: MTU 1500, Options E, Flags I|M|MS (ExStart), DD sequence 1.
		dbd := mustHex(t, "02020020c0a800010000000000000000000000000000000005dc020700000001")
		n := parseRule(t, dbd, "ospf", "OSPF")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Type")))
		d := mustChild(t, n, "OSPFDBDesc")
		require.Equal(t, uint64(1500), uintVal(t, d.Child("Interface MTU")))
		require.Equal(t, uint64(0x02), uintVal(t, d.Child("Options")))
		require.Equal(t, uint64(0x07), uintVal(t, d.Child("Flags")))
		require.Equal(t, uint64(1), uintVal(t, d.Child("DD Sequence")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 89, dbd))
		require.Equal(t, uint64(1500), uintVal(t, mustChild(t, eth, "IP", "OSPF", "OSPFDBDesc").Child("Interface MTU")))
		require.Equal(t, uint64(0x07), uintVal(t, mustChild(t, eth, "IP", "OSPF", "OSPFDBDesc").Child("Flags")))
	})

	t.Run("wireguard/init", func(t *testing.T) {
		// wireguard.com/protocol handshake_initiation; Wireshark packet-wireguard.c wg.sender.
		// Type 1, 148 bytes, little-endian Sender=1 (whitepaper §5.4.2).
		wg := make([]byte, 148)
		wg[0] = 1
		wg[4] = 1
		n := parseRule(t, wg, "wireguard", "WireGuard")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "WGInit").Child("Sender")))
		require.Equal(t, 32, len(bytesVal(t, mustChild(t, n, "WGInit").Child("Ephemeral"))))
		eth := parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "WireGuard", "WGInit").Child("Sender")))
	})

	t.Run("wireguard/response", func(t *testing.T) {
		// wireguard.com/protocol handshake_response 92 bytes; Wireshark wg.receiver / encrypted_empty.
		// Type 2, Sender=2, Receiver=1 (echo of initiator sender).
		wg := make([]byte, 92)
		wg[0] = 2
		wg[4] = 2
		wg[8] = 1
		n := parseRule(t, wg, "wireguard", "WireGuard")
		resp := mustChild(t, n, "WGResponse")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(2), uintVal(t, resp.Child("Sender")))
		require.Equal(t, uint64(1), uintVal(t, resp.Child("Receiver")))
		require.Equal(t, 16, len(bytesVal(t, resp.Child("Encrypted Empty"))))
		eth := parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "WireGuard", "WGResponse").Child("Receiver")))
	})

	t.Run("wireguard/cookie", func(t *testing.T) {
		// wireguard.com/protocol packet_cookie_reply 64 bytes; Wireshark wg.cookie.nonce.
		// Type 3, Receiver=1, Nonce 24, Encrypted Cookie AEAD_LEN(16)=32.
		wg := make([]byte, 64)
		wg[0] = 3
		wg[4] = 1
		copy(wg[8:12], []byte("nonc"))
		n := parseRule(t, wg, "wireguard", "WireGuard")
		ck := mustChild(t, n, "WGCookie")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, ck.Child("Receiver")))
		require.Equal(t, 24, len(bytesVal(t, ck.Child("Nonce"))))
		require.Equal(t, 32, len(bytesVal(t, ck.Child("Encrypted Cookie"))))
		eth := parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "WireGuard", "WGCookie").Child("Receiver")))
	})

	t.Run("wireguard/transport", func(t *testing.T) {
		// wireguard.com/protocol transport; Wireshark wg.counter / wg.encrypted_packet.
		// Type 4, Receiver=1, Counter=7, 16-byte AEAD ciphertext. UDP/51820.
		wg := make([]byte, 4+4+8+16)
		wg[0] = 4
		wg[4] = 1
		wg[8] = 7
		copy(wg[16:], []byte("ciphertext123456"))
		n := parseRule(t, wg, "wireguard", "WireGuard")
		tr := mustChild(t, n, "WGTransport")
		require.Equal(t, uint64(4), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, tr.Child("Receiver")))
		require.Equal(t, uint64(7), uintVal(t, tr.Child("Counter")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 51820, 51820, wg))
		wired := mustChild(t, eth, "IP", "UDP", "WireGuard", "WGTransport")
		require.Equal(t, uint64(7), uintVal(t, wired.Child("Counter")))
		require.Equal(t, []byte("ciphertext123456"), bytesVal(t, wired.Child("Ciphertext")))
	})

	t.Run("salt/zmtp", func(t *testing.T) {
		// ZeroMQ RFC 23 / Wireshark packet-zmtp.c greeting: 0xff…0x7f, version 3.0, mechanism NULL.
		g := make([]byte, 64)
		g[0] = 0xff
		g[9] = 0x7f
		g[10] = 3
		copy(g[12:16], []byte("NULL"))
		n := parseRule(t, g, "salt", "Salt")
		gr := mustChild(t, n, "SaltGreeting")
		require.Equal(t, uint64(0xff), uintVal(t, gr.Child("Signature")))
		require.Equal(t, uint64(3), uintVal(t, gr.Child("Major")))
		require.Equal(t, uint64(0), uintVal(t, gr.Child("Minor")))
		require.True(t, strings.HasPrefix(strVal(t, gr.Child("Mechanism")), "NULL"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 4505, 4505, g))
		require.Equal(t, uint64(3), uintVal(t, mustChild(t, eth, "IP", "TCP", "Salt", "SaltGreeting").Child("Major")))
	})

	t.Run("salt/ping", func(t *testing.T) {
		// Salt ZeroMQ transport TCP/4505: uint32 length + command "ping" (salt.transport.zeromq).
		ping := []byte{0x00, 0x00, 0x00, 0x04, 'p', 'i', 'n', 'g'}
		n := parseRule(t, ping, "salt", "Salt")
		fr := n.Child("Frames").Children()[0]
		require.Equal(t, uint64(4), uintVal(t, fr.Child("Length")))
		require.Equal(t, "ping", strVal(t, fr.Child("Command")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 4505, 4505, ping))
		require.Equal(t, "ping", strVal(t, mustChild(t, eth, "IP", "TCP", "Salt", "Frames").Children()[0].Child("Command")))
	})

	t.Run("sip/invite", func(t *testing.T) {
		// RFC 3261 Figure 1 INVITE + SDP (Content-Type application/sdp).
		sdp := "v=0\r\no=alice 2890844526 2890844526 IN IP4 pc33.atlanta.example.com\r\n"
		inv := "INVITE sip:bob@biloxi.example.com SIP/2.0\r\n" +
			"Via: SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n" +
			"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n" +
			"To: Bob <sip:bob@biloxi.example.com>\r\n" +
			"Call-ID: a84b4c76e66710@pc33.atlanta.example.com\r\n" +
			"CSeq: 314159 INVITE\r\n" +
			"Content-Type: application/sdp\r\n" +
			"Content-Length: 68\r\n" +
			"\r\n" + sdp
		n := parseRule(t, []byte(inv), "sip", "SIP")
		req := mustChild(t, n, "SIP Request")
		require.Equal(t, "INVITE", strVal(t, req.Child("Method")))
		require.Equal(t, "sip:bob@biloxi.example.com", strVal(t, req.Child("URI")))
		sdpn := mustChild(t, req, "Body", "SDPSession")
		require.Equal(t, uint64('v'), uintVal(t, sdpn.Child("Type")))
		require.Equal(t, "0", strVal(t, sdpn.Child("Value")))
		require.Equal(t, uint64('o'), uintVal(t, sdpn.Child("Origin Type")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5060, 5060, []byte(inv)))
		require.Equal(t, "INVITE", strVal(t, mustChild(t, eth, "IP", "UDP", "SIP", "SIP Request").Child("Method")))
		require.Equal(t, uint64('o'), uintVal(t, mustChild(t, eth, "IP", "UDP", "SIP", "SIP Request", "Body", "SDPSession").Child("Origin Type")))
	})

	t.Run("sip/200", func(t *testing.T) {
		ok := "SIP/2.0 200 OK\r\nVia: SIP/2.0/UDP 10.0.0.1\r\nCall-ID: a84b4c76e66710@pc33.atlanta.example.com\r\nCSeq: 314159 INVITE\r\nContent-Length: 0\r\n\r\n"
		n := parseRule(t, []byte(ok), "sip", "SIP")
		resp := mustChild(t, n, "SIP Response")
		require.Equal(t, "SIP/2.0", strVal(t, resp.Child("Version")))
		require.Equal(t, "200", strVal(t, resp.Child("Status")))
		require.Equal(t, "OK", strVal(t, resp.Child("Reason")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5060, 5060, []byte(ok)))
		require.Equal(t, "200", strVal(t, mustChild(t, eth, "IP", "UDP", "SIP", "SIP Response").Child("Status")))
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

	t.Run("icmpv6/dest-unreach", func(t *testing.T) {
		// RFC 4443 §3.1 Destination Unreachable Type 1 Code 4 Port Unreachable.
		// Unused 32 bits then as much of the invoking packet as possible.
		invoking := []byte{0x60, 0x00, 0x00, 0x00}
		du := append([]byte{0x01, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, invoking...)
		n := parseRule(t, du, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(4), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, n, "Destination Unreachable").Child("Unused")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, du))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Destination Unreachable")
		require.Equal(t, uint64(0), uintVal(t, wired.Child("Unused")))
		require.Equal(t, invoking, bytesVal(t, wired.Child("Original Datagram")))
	})

	t.Run("icmpv6/packet-too-big", func(t *testing.T) {
		// RFC 4443 §3.2 Packet Too Big Type 2 Code 0, MTU 1280 (IPv6 minimum).
		invoking := []byte{0x60, 0x00, 0x00, 0x00}
		ptb := append([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05, 0x00}, invoking...)
		n := parseRule(t, ptb, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1280), uintVal(t, mustChild(t, n, "Packet Too Big").Child("MTU")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, ptb))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Packet Too Big")
		require.Equal(t, uint64(1280), uintVal(t, wired.Child("MTU")))
		require.Equal(t, invoking, bytesVal(t, wired.Child("Original Datagram")))
	})

	t.Run("icmpv6/time-exceeded", func(t *testing.T) {
		// RFC 4443 §3.3 Time Exceeded Type 3 Code 0 hop limit exceeded in transit.
		invoking := []byte{0x60, 0x00, 0x00, 0x00}
		te := append([]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, invoking...)
		n := parseRule(t, te, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, n, "Time Exceeded").Child("Unused")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, te))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Time Exceeded")
		require.Equal(t, uint64(0), uintVal(t, wired.Child("Unused")))
		require.Equal(t, invoking, bytesVal(t, wired.Child("Original Datagram")))
	})

	t.Run("icmpv6/param-problem", func(t *testing.T) {
		// RFC 4443 §3.4 Parameter Problem Type 4 Code 1 unrecognized Next Header.
		// Pointer 6 is the IPv6 Next Header octet (RFC 8200 §3).
		invoking := []byte{0x60, 0x00, 0x00, 0x00}
		pp := append([]byte{0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06}, invoking...)
		n := parseRule(t, pp, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(4), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, n, "Parameter Problem").Child("Pointer")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, pp))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Parameter Problem")
		require.Equal(t, uint64(6), uintVal(t, wired.Child("Pointer")))
		require.Equal(t, invoking, bytesVal(t, wired.Child("Original Datagram")))
	})

	t.Run("icmpv6/mld-query", func(t *testing.T) {
		// RFC 2710 §3 Multicast Listener Query Type 130 General Query.
		// Maximum Response Delay 10000 ms (RFC 2710 §7.2 default); Multicast Address :: .
		// Wireshark icmpv6.mld.mrc / icmpv6.mld.multicast_address.
		unspec := make([]byte, 16)
		q := append([]byte{0x82, 0x00, 0x00, 0x00, 0x27, 0x10, 0x00, 0x00}, unspec...)
		n := parseRule(t, q, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(130), uintVal(t, n.Child("Type")))
		query := mustChild(t, n, "Multicast Listener Query")
		require.Equal(t, uint64(10000), uintVal(t, query.Child("Maximum Response Delay")))
		require.Equal(t, unspec, bytesVal(t, query.Child("Multicast Address")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, q))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Multicast Listener Query")
		require.Equal(t, uint64(10000), uintVal(t, wired.Child("Maximum Response Delay")))
		require.Equal(t, unspec, bytesVal(t, wired.Child("Multicast Address")))
	})

	t.Run("icmpv6/mld-report", func(t *testing.T) {
		// RFC 2710 §3 Multicast Listener Report Type 131; Delay 0; group ff02::1.
		ff02ones := []byte{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		r := append([]byte{0x83, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, ff02ones...)
		n := parseRule(t, r, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(131), uintVal(t, n.Child("Type")))
		require.Equal(t, ff02ones, bytesVal(t, mustChild(t, n, "Multicast Listener Report").Child("Multicast Address")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, r))
		require.Equal(t, ff02ones, bytesVal(t, mustChild(t, eth, "IPv6", "ICMPv6", "Multicast Listener Report").Child("Multicast Address")))
	})

	t.Run("icmpv6/mld-done", func(t *testing.T) {
		// RFC 2710 §3 Multicast Listener Done Type 132; Delay 0; group ff02::1.
		ff02ones := []byte{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		d := append([]byte{0x84, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, ff02ones...)
		n := parseRule(t, d, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(132), uintVal(t, n.Child("Type")))
		require.Equal(t, ff02ones, bytesVal(t, mustChild(t, n, "Multicast Listener Done").Child("Multicast Address")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, d))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, eth, "IPv6", "ICMPv6", "Multicast Listener Done").Child("Maximum Response Delay")))
	})

	t.Run("icmpv6/redirect", func(t *testing.T) {
		// RFC 4861 §4.5 Redirect Type 137: Target fe80::1, Destination 2001:db8::1.
		// Wireshark icmpv6.nd.target_address / icmpv6.nd.redirect.dest. Ethernet+IPv6.
		target := []byte{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		dest := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
		rd := append(append([]byte{0x89, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, target...), dest...)
		n := parseRule(t, rd, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(137), uintVal(t, n.Child("Type")))
		redir := mustChild(t, n, "Redirect")
		require.Equal(t, target, bytesVal(t, redir.Child("Target Address")))
		require.Equal(t, dest, bytesVal(t, redir.Child("Destination Address")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, rd))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Redirect")
		require.Equal(t, target, bytesVal(t, wired.Child("Target Address")))
		require.Equal(t, dest, bytesVal(t, wired.Child("Destination Address")))
	})

	t.Run("icmpv6/mldv2-report", func(t *testing.T) {
		// RFC 3810 §5.2 Version 2 Multicast Listener Report Type 143.
		// One MODE_IS_EXCLUDE record (type 2), 0 sources, group ff02::1:ff00:1.
		// Wireshark icmpv6.mldr.mar.record_type / icmpv6.mldr.mar.multicast_address.
		group := []byte{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0xff, 0, 0, 0x01}
		r := append([]byte{0x8f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00}, group...)
		n := parseRule(t, r, "internet_control_message_protocol_v6", "ICMPV6")
		require.Equal(t, uint64(143), uintVal(t, n.Child("Type")))
		rep := mustChild(t, n, "Multicast Listener Report v2")
		require.Equal(t, uint64(1), uintVal(t, rep.Child("Number of Mcast Address Records")))
		rec := rep.Child("Records").Children()[0]
		require.Equal(t, uint64(2), uintVal(t, rec.Child("Record Type")))
		require.Equal(t, group, bytesVal(t, rec.Child("Multicast Address")))
		eth := parseEthernet(t, ipv6ICMPBytes(t, r))
		wired := mustChild(t, eth, "IPv6", "ICMPv6", "Multicast Listener Report v2")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("Number of Mcast Address Records")))
		require.Equal(t, uint64(2), uintVal(t, wired.Child("Records").Children()[0].Child("Record Type")))
		require.Equal(t, group, bytesVal(t, wired.Child("Records").Children()[0].Child("Multicast Address")))
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

	t.Run("tftp/blksize", func(t *testing.T) {
		// RFC 2347 / RFC 2348 RRQ with option blksize 512. Wireshark tftp.option.name / tftp.option.value.
		raw := append([]byte{0x00, 0x01}, []byte("foo\x00octet\x00blksize\x00512\x00")...)
		tf := parseRule(t, raw, "tftp", "TFTP")
		require.Equal(t, "foo", strVal(t, tf.Child("Filename")))
		require.Equal(t, "octet", strVal(t, tf.Child("Mode")))
		opts := tf.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 1)
		require.Equal(t, "blksize", strVal(t, opts[0].Child("Name")))
		require.Equal(t, "512", strVal(t, opts[0].Child("Value")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 12345, 69, raw))
		wired := mustChild(t, eth, "IP", "UDP", "TFTP").Child("Options").Children()
		require.Equal(t, "blksize", strVal(t, wired[0].Child("Name")))
		require.Equal(t, "512", strVal(t, wired[0].Child("Value")))
	})

	t.Run("tftp/oack", func(t *testing.T) {
		// RFC 2347 OACK (opcode 6) tsize 1234. Wireshark tftp.option.name=tsize.
		raw := append([]byte{0x00, 0x06}, []byte("tsize\x001234\x00")...)
		tf := parseRule(t, raw, "tftp", "TFTP")
		require.Equal(t, uint64(6), uintVal(t, tf.Child("Opcode")))
		opt := mustChild(t, tf, "Options").Children()[0]
		require.Equal(t, "tsize", strVal(t, opt.Child("Name")))
		require.Equal(t, "1234", strVal(t, opt.Child("Value")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 12345, 69, raw))
		require.Equal(t, "tsize", strVal(t, mustChild(t, eth, "IP", "UDP", "TFTP").Child("Options").Children()[0].Child("Name")))
	})

	t.Run("tftp/timeout", func(t *testing.T) {
		// RFC 2347 two options: RFC 2348 blksize 1432 + RFC 2349 timeout 5. UDP/69.
		raw := append([]byte{0x00, 0x01}, []byte("boot\x00octet\x00blksize\x001432\x00timeout\x005\x00")...)
		tf := parseRule(t, raw, "tftp", "TFTP")
		opts := tf.Child("Options").Children()
		require.Equal(t, 2, len(opts))
		require.Equal(t, "blksize", strVal(t, opts[0].Child("Name")))
		require.Equal(t, "1432", strVal(t, opts[0].Child("Value")))
		require.Equal(t, "timeout", strVal(t, opts[1].Child("Name")))
		require.Equal(t, "5", strVal(t, opts[1].Child("Value")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 12345, 69, raw))
		wired := mustChild(t, eth, "IP", "UDP", "TFTP").Child("Options").Children()
		require.Equal(t, "timeout", strVal(t, wired[1].Child("Name")))
		require.Equal(t, "5", strVal(t, wired[1].Child("Value")))
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
		require.Equal(t, "Capability list follows", strVal(t, po.Child("Arg")))
		lines := mustChild(t, po, "POP3Extra").Children()
		require.GreaterOrEqual(t, len(lines), 3)
		require.Equal(t, "USER", strVal(t, lines[0].Child("Text")))
		require.Equal(t, "UIDL", strVal(t, lines[1].Child("Text")))
		require.Equal(t, ".", strVal(t, lines[2].Child("Text")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, capa))
		el := mustChild(t, eth, "IP", "TCP", "POP3", "POP3Extra").Children()
		require.Equal(t, "USER", strVal(t, el[0].Child("Text")))
	})

	t.Run("pop3/stat", func(t *testing.T) {
		// RFC 1939 STAT: +OK nn mm (messages, octets). Wireshark pop.response.
		raw := []byte("+OK 2 320\r\n")
		po := parseRule(t, raw, "pop3", "POP3")
		require.Equal(t, "+OK", strVal(t, po.Child("Status")))
		st := mustChild(t, po, "POP3Stat")
		require.Equal(t, "2", strVal(t, st.Child("Messages")))
		require.Equal(t, "320", strVal(t, st.Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, raw))
		wired := mustChild(t, eth, "IP", "TCP", "POP3", "POP3Stat")
		require.Equal(t, "2", strVal(t, wired.Child("Messages")))
		require.Equal(t, "320", strVal(t, wired.Child("Octets")))
	})

	t.Run("pop3/list", func(t *testing.T) {
		// RFC 1939 LIST scan listing: msg-number octets, terminated by ".".
		raw := []byte("+OK 2 messages (320 octets)\r\n1 120\r\n2 200\r\n.\r\n")
		po := parseRule(t, raw, "pop3", "POP3")
		require.Equal(t, "+OK", strVal(t, po.Child("Status")))
		require.Equal(t, "2 messages (320 octets)", strVal(t, po.Child("Arg")))
		lines := mustChild(t, po, "POP3Extra").Children()
		require.GreaterOrEqual(t, len(lines), 3)
		require.Equal(t, "1", strVal(t, lines[0].Child("Number")))
		require.Equal(t, "120", strVal(t, lines[0].Child("Size")))
		require.Equal(t, "2", strVal(t, lines[1].Child("Number")))
		require.Equal(t, "200", strVal(t, lines[1].Child("Size")))
		require.Equal(t, ".", strVal(t, lines[2].Child("Text")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 110, 110, raw))
		wired := mustChild(t, eth, "IP", "TCP", "POP3", "POP3Extra").Children()
		require.Equal(t, "1", strVal(t, wired[0].Child("Number")))
		require.Equal(t, "120", strVal(t, wired[0].Child("Size")))
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
		// RFC 1994 §4.1 Challenge: Value-Size 16, Name HiPer.att.net.
		// Wireshark chap.code / chap.name. PPP protocol 0xc223.
		chap := mustHex(t, "01030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574")
		ch := parseRule(t, chap, "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(1), uintVal(t, ch.Child("Code")))
		require.Equal(t, uint64(16), uintVal(t, mustChild(t, ch, "Data").Child("Value Size")))
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, ch, "Data").Child("Name")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, append([]byte{0x30, 0x81, 0x88, 0x0b, 0x00, 0x26, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0x03, 0xc2, 0x23}, chap...)))
		wired := mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "CHAP")
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, wired, "Data").Child("Name")))
		require.Equal(t, uint64(16), uintVal(t, mustChild(t, wired, "Data").Child("Value Size")))
	})

	t.Run("chap/response", func(t *testing.T) {
		// RFC 1994 §4.2 Response: same Value-Size/Name layout, Code=2.
		chap := mustHex(t, "02030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574")
		ch := parseRule(t, chap, "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(2), uintVal(t, ch.Child("Code")))
		require.Equal(t, uint64(16), uintVal(t, mustChild(t, ch, "CHAPResponse").Child("Value Size")))
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, ch, "CHAPResponse").Child("Name")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, append([]byte{0x30, 0x81, 0x88, 0x0b, 0x00, 0x26, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0x03, 0xc2, 0x23}, chap...)))
		require.Equal(t, "HiPer.att.net", strVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "CHAP", "CHAPResponse").Child("Name")))
	})

	t.Run("chap/success", func(t *testing.T) {
		// RFC 1994 §4.3 Success: Code 3, Message "Welcome". Wireshark chap.message. PPP 0xc223.
		chap := append([]byte{0x03, 0x03, 0x00, 0x0b}, []byte("Welcome")...)
		ch := parseRule(t, chap, "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(3), uintVal(t, ch.Child("Code")))
		require.Equal(t, "Welcome", strVal(t, mustChild(t, ch, "CHAPSuccess").Child("Message")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, append([]byte{0x30, 0x81, 0x88, 0x0b, 0x00, 0x0f, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0x03, 0xc2, 0x23}, chap...)))
		require.Equal(t, "Welcome", strVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "CHAP", "CHAPSuccess").Child("Message")))
	})

	t.Run("chap/failure", func(t *testing.T) {
		// RFC 1994 §4.4 Failure: Code 4, Message "Login incorrect". Wireshark chap.message.
		msg := []byte("Login incorrect")
		chap := append([]byte{0x04, 0x03, 0x00, byte(4 + len(msg))}, msg...)
		ch := parseRule(t, chap, "challenge_handshake_authentication_protocol", "CHAP")
		require.Equal(t, uint64(4), uintVal(t, ch.Child("Code")))
		require.Equal(t, "Login incorrect", strVal(t, mustChild(t, ch, "CHAPFailure").Child("Message")))
		plen := uint16(4 + len(chap))
		gre := []byte{0x30, 0x81, 0x88, 0x0b, byte(plen >> 8), byte(plen), 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0x03, 0xc2, 0x23}
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, append(gre, chap...)))
		require.Equal(t, "Login incorrect", strVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "CHAP", "CHAPFailure").Child("Message")))
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

	t.Run("rtcp/sr", func(t *testing.T) {
		// RFC 3550 §6.4.1 Sender Report: 20-octet sender info after SSRC.
		// WebRTC sender_report_unittest.cc kPacket; Wireshark rtcp.sender.packetcount.
		sr := mustHex(t, "80c80006123456781112141822242628333435364445464755565758")
		n := parseRule(t, sr, "rtp", "RTCP")
		require.Equal(t, uint64(200), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(0x12345678), uintVal(t, n.Child("SSRC")))
		info := mustChild(t, n, "RTCPSR")
		require.Equal(t, uint64(0x11121418), uintVal(t, info.Child("NTP MSW")))
		require.Equal(t, uint64(0x22242628), uintVal(t, info.Child("NTP LSW")))
		require.Equal(t, uint64(0x33343536), uintVal(t, info.Child("RTP Timestamp")))
		require.Equal(t, uint64(0x44454647), uintVal(t, info.Child("Packet Count")))
		require.Equal(t, uint64(0x55565758), uintVal(t, info.Child("Octet Count")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5005, 5005, sr))
		require.Equal(t, uint64(0x44454647), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTCP", "RTCPSR").Child("Packet Count")))
		require.Equal(t, uint64(0x55565758), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTCP", "RTCPSR").Child("Octet Count")))
	})

	t.Run("rtcp/rr", func(t *testing.T) {
		// RFC 3550 §6.4.2 Receiver Report: PT=201, RC=0, no sender info.
		rr := mustHex(t, "80c9000112345678")
		n := parseRule(t, rr, "rtp", "RTCP")
		require.Equal(t, uint64(201), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(0x12345678), uintVal(t, n.Child("SSRC")))
		require.Nil(t, n.Child("RTCPSR"))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5005, 5005, rr))
		require.Equal(t, uint64(201), uintVal(t, mustChild(t, eth, "IP", "UDP", "RTCP").Child("Packet Type")))
	})

	t.Run("websocket/text", func(t *testing.T) {
		// RFC 6455 §5.7 unmasked text: 0x81 0x05 "Hello" (PROTOCOL_DELIVERY L2 sample).
		ws := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
		n := parseRule(t, ws, "application-layer.websocket", "WebSocket")
		require.Equal(t, uint64(1), uintVal(t, n.Child("FIN")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Opcode")))
		require.Equal(t, "Hello", strVal(t, n.Child("Text")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8080, ws))
		require.Equal(t, "Hello", strVal(t, mustChild(t, eth, "IP", "TCP", "WebSocket").Child("Text")))
	})

	t.Run("websocket/close", func(t *testing.T) {
		// RFC 6455 §5.5.1 / §7.4.1 unmasked Close status 1000 (normal closure).
		ws := []byte{0x88, 0x02, 0x03, 0xe8}
		n := parseRule(t, ws, "application-layer.websocket", "WebSocket")
		require.Equal(t, uint64(8), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1000), uintVal(t, n.Child("Close Code")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8080, ws))
		require.Equal(t, uint64(1000), uintVal(t, mustChild(t, eth, "IP", "TCP", "WebSocket").Child("Close Code")))
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
		zb := zabbixPacket(0x01, []byte("{}"), true)
		z := parseRule(t, zb, "zabbix", "Zabbix")
		require.Equal(t, "{}", strVal(t, z.Child("JSON")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, z, "Flags").Child("Protocol")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 10050, 10050, zb))
		require.Equal(t, "{}", strVal(t, mustChild(t, eth, "IP", "TCP", "Zabbix").Child("JSON")))
	})

	t.Run("zabbix/active-checks", func(t *testing.T) {
		// Zabbix 4.0 13-byte header (reserved uint32) + agent "active checks" JSON.
		// Wireshark zabbix.flags / zabbix.reserved / zabbix.json. TCP/10050.
		js := []byte(`{"request":"active checks","host":"testhost"}`)
		zb := zabbixPacket(0x01, js, true)
		z := parseRule(t, zb, "zabbix", "Zabbix")
		require.Equal(t, uint64(len(js)), uintVal(t, z.Child("Length")))
		require.Equal(t, uint64(0), uintVal(t, z.Child("Reserved")))
		require.Equal(t, string(js), strVal(t, z.Child("JSON")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, z, "Flags").Child("Compression")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 10050, 10050, zb))
		require.Equal(t, string(js), strVal(t, mustChild(t, eth, "IP", "TCP", "Zabbix").Child("JSON")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, eth, "IP", "TCP", "Zabbix").Child("Reserved")))
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

	t.Run("pptp/ocrq", func(t *testing.T) {
		// RFC 2637 §2.7 Outgoing-Call-Request (control type 7), 168-byte message.
		// Wireshark pptp.control_message_type / pptp.call_id / pptp.phone_number. TCP/1723.
		pptp := make([]byte, 168)
		binary.BigEndian.PutUint16(pptp[0:], 168)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 7)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint16(pptp[14:], 2)
		binary.BigEndian.PutUint32(pptp[16:], 300)
		binary.BigEndian.PutUint32(pptp[20:], 64000)
		binary.BigEndian.PutUint32(pptp[24:], 1)
		binary.BigEndian.PutUint32(pptp[28:], 1)
		binary.BigEndian.PutUint16(pptp[32:], 64)
		binary.BigEndian.PutUint16(pptp[36:], 7)
		copy(pptp[40:], []byte("5551212"))
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(7), uintVal(t, n.Child("ControlMessageType")))
		ocrq := mustChild(t, n, "Outgoing Call Req")
		require.Equal(t, uint64(1), uintVal(t, ocrq.Child("CallId")))
		require.Equal(t, uint64(7), uintVal(t, ocrq.Child("PhoneNumberLength")))
		require.True(t, strings.HasPrefix(strVal(t, ocrq.Child("PhoneNumber")), "5551212"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Outgoing Call Req")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("CallId")))
		require.True(t, strings.HasPrefix(strVal(t, wired.Child("PhoneNumber")), "5551212"))
	})

	t.Run("pptp/echo", func(t *testing.T) {
		// RFC 2637 §2.5 Echo-Request (control type 5), 16-byte message, Identifier 1.
		// Wireshark pptp.control_message_type / pptp.identifier. TCP/1723.
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 5)
		binary.BigEndian.PutUint32(pptp[12:], 1)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(5), uintVal(t, n.Child("ControlMessageType")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Echo Request").Child("Identifier")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP", "Echo Request").Child("Identifier")))
	})

	t.Run("pptp/set-link-info", func(t *testing.T) {
		// RFC 2637 §2.15 Set-Link-Info (control type 15), 24-byte message.
		// Peer's Call ID 1; Send/Recv ACCM 0xFFFFFFFF (RFC default until this message).
		// Wireshark pptp.peer_call_id / pptp.send_accm / pptp.recv_accm. TCP/1723.
		pptp := make([]byte, 24)
		binary.BigEndian.PutUint16(pptp[0:], 24)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 15)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint32(pptp[16:], 0xffffffff)
		binary.BigEndian.PutUint32(pptp[20:], 0xffffffff)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(15), uintVal(t, n.Child("ControlMessageType")))
		sli := mustChild(t, n, "Set Link Info")
		require.Equal(t, uint64(1), uintVal(t, sli.Child("PeerCallId")))
		require.Equal(t, uint64(0xffffffff), uintVal(t, sli.Child("Send Accm")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Set Link Info")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("PeerCallId")))
		require.Equal(t, uint64(0xffffffff), uintVal(t, wired.Child("Recv Accm")))
	})

	t.Run("pptp/echo-reply", func(t *testing.T) {
		// RFC 2637 §2.6 Echo-Reply (control type 6), 20-byte message.
		// Identifier 1 matches Echo-Request; Result Code 1 = OK. Wireshark pptp.result. TCP/1723.
		pptp := make([]byte, 20)
		binary.BigEndian.PutUint16(pptp[0:], 20)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 6)
		binary.BigEndian.PutUint32(pptp[12:], 1)
		pptp[16] = 1
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(6), uintVal(t, n.Child("ControlMessageType")))
		rep := mustChild(t, n, "Echo Reply")
		require.Equal(t, uint64(1), uintVal(t, rep.Child("Identifier")))
		require.Equal(t, uint64(1), uintVal(t, rep.Child("ResultCode")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Echo Reply")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("Identifier")))
		require.Equal(t, uint64(1), uintVal(t, wired.Child("ResultCode")))
	})

	t.Run("pptp/stop-req", func(t *testing.T) {
		// RFC 2637 §2.3 Stop-Control-Connection-Request (control type 3), 16-byte message.
		// Reason 1 = None (normal stop). Wireshark pptp.reason. TCP/1723.
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 3)
		pptp[12] = 1
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(3), uintVal(t, n.Child("ControlMessageType")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Stop Control Conn Req").Child("Reason")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP", "Stop Control Conn Req").Child("Reason")))
	})

	t.Run("pptp/stop-reply", func(t *testing.T) {
		// RFC 2637 §2.4 Stop-Control-Connection-Reply (control type 4), 16-byte message.
		// Result Code 1 = OK. Wireshark pptp.result. TCP/1723.
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 4)
		pptp[12] = 1
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(4), uintVal(t, n.Child("ControlMessageType")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Stop Control Conn Reply").Child("ResultCode")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP", "Stop Control Conn Reply").Child("ResultCode")))
	})

	t.Run("pptp/icrq", func(t *testing.T) {
		// RFC 2637 §2.9 Incoming-Call-Request (control type 9), 220-byte message.
		// CallId 1, Dialed Number "5551212". Wireshark pptp.call_id / pptp.dialed_number. TCP/1723.
		pptp := make([]byte, 220)
		binary.BigEndian.PutUint16(pptp[0:], 220)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 9)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint16(pptp[14:], 2)
		binary.BigEndian.PutUint32(pptp[16:], 1)
		binary.BigEndian.PutUint16(pptp[24:], 7)
		copy(pptp[28:], []byte("5551212"))
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(9), uintVal(t, n.Child("ControlMessageType")))
		icrq := mustChild(t, n, "Incoming Call Req")
		require.Equal(t, uint64(1), uintVal(t, icrq.Child("CallId")))
		require.Equal(t, uint64(7), uintVal(t, icrq.Child("DialedNumberLength")))
		require.True(t, strings.HasPrefix(strVal(t, icrq.Child("DialedNumber")), "5551212"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Incoming Call Req")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("CallId")))
		require.True(t, strings.HasPrefix(strVal(t, wired.Child("DialedNumber")), "5551212"))
	})

	t.Run("pptp/icrp", func(t *testing.T) {
		// RFC 2637 §2.10 Incoming-Call-Reply (control type 10), 28-byte message.
		// CallId 1, PeerCallId 1, Result Code 1 = Connect, Recv Window 64.
		// Wireshark pptp.call_id / pptp.peer_call_id / pptp.result. TCP/1723.
		pptp := make([]byte, 28)
		binary.BigEndian.PutUint16(pptp[0:], 28)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 10)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint16(pptp[14:], 1)
		pptp[16] = 1
		binary.BigEndian.PutUint16(pptp[20:], 64)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(10), uintVal(t, n.Child("ControlMessageType")))
		icrp := mustChild(t, n, "Incoming Call Reply")
		require.Equal(t, uint64(1), uintVal(t, icrp.Child("CallId")))
		require.Equal(t, uint64(1), uintVal(t, icrp.Child("PeerCallId")))
		require.Equal(t, uint64(1), uintVal(t, icrp.Child("ResultCode")))
		require.Equal(t, uint64(64), uintVal(t, icrp.Child("RecvWindowSize")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Incoming Call Reply")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("CallId")))
		require.Equal(t, uint64(64), uintVal(t, wired.Child("RecvWindowSize")))
	})

	t.Run("pptp/icc", func(t *testing.T) {
		// RFC 2637 §2.11 Incoming-Call-Connected (control type 11), 28-byte message.
		// PeerCallId 1, Connect Speed 64000, Recv Window 64, Framing Type 1 (Async).
		// Wireshark pptp.peer_call_id / pptp.connect_speed / pptp.framing_type. TCP/1723.
		pptp := make([]byte, 28)
		binary.BigEndian.PutUint16(pptp[0:], 28)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 11)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint32(pptp[16:], 64000)
		binary.BigEndian.PutUint16(pptp[20:], 64)
		binary.BigEndian.PutUint32(pptp[24:], 1)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(11), uintVal(t, n.Child("ControlMessageType")))
		icc := mustChild(t, n, "Incoming Call Connected")
		require.Equal(t, uint64(1), uintVal(t, icc.Child("PeerCallId")))
		require.Equal(t, uint64(64000), uintVal(t, icc.Child("ConnectSpeed")))
		require.Equal(t, uint64(64), uintVal(t, icc.Child("RecvWindowSize")))
		require.Equal(t, uint64(1), uintVal(t, icc.Child("FramingType")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Incoming Call Connected")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("PeerCallId")))
		require.Equal(t, uint64(64000), uintVal(t, wired.Child("ConnectSpeed")))
	})

	t.Run("pptp/ccrq", func(t *testing.T) {
		// RFC 2637 §2.12 Call-Clear-Request (control type 12), 16-byte message.
		// Call ID assigned by the PNS. Wireshark pptp.call_id / pptp.control_message_type. TCP/1723.
		pptp := make([]byte, 16)
		binary.BigEndian.PutUint16(pptp[0:], 16)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 12)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(12), uintVal(t, n.Child("ControlMessageType")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Call Clear Req").Child("CallId")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "PPTP", "Call Clear Req").Child("CallId")))
	})

	t.Run("pptp/cdn", func(t *testing.T) {
		// RFC 2637 §2.13 Call-Disconnect-Notify (control type 13), 148-byte message.
		// CallId 1, Result Code 1 = Lost Carrier. Wireshark pptp.call_id / pptp.result. TCP/1723.
		pptp := make([]byte, 148)
		binary.BigEndian.PutUint16(pptp[0:], 148)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 13)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		pptp[16] = 1
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(13), uintVal(t, n.Child("ControlMessageType")))
		cdn := mustChild(t, n, "Call Disconnect Notify")
		require.Equal(t, uint64(1), uintVal(t, cdn.Child("CallId")))
		require.Equal(t, uint64(1), uintVal(t, cdn.Child("ResultCode")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "Call Disconnect Notify")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("CallId")))
		require.Equal(t, uint64(1), uintVal(t, wired.Child("ResultCode")))
	})

	t.Run("pptp/wan-error", func(t *testing.T) {
		// RFC 2637 §2.14 WAN-Error-Notify (control type 14), 40-byte message.
		// Peer Call ID 1, CRC Errors 1. Wireshark pptp.peer_call_id / pptp.crc_errors. TCP/1723.
		pptp := make([]byte, 40)
		binary.BigEndian.PutUint16(pptp[0:], 40)
		binary.BigEndian.PutUint16(pptp[2:], 1)
		binary.BigEndian.PutUint32(pptp[4:], 0x1a2b3c4d)
		binary.BigEndian.PutUint16(pptp[8:], 14)
		binary.BigEndian.PutUint16(pptp[12:], 1)
		binary.BigEndian.PutUint32(pptp[16:], 1)
		n := parseRule(t, pptp, "application-layer.pptp", "PPTP")
		require.Equal(t, uint64(14), uintVal(t, n.Child("ControlMessageType")))
		wan := mustChild(t, n, "WAN Error Notify")
		require.Equal(t, uint64(1), uintVal(t, wan.Child("PeerCallId")))
		require.Equal(t, uint64(1), uintVal(t, wan.Child("CRC Errors")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1723, 1723, pptp))
		wired := mustChild(t, eth, "IP", "TCP", "PPTP", "WAN Error Notify")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("PeerCallId")))
		require.Equal(t, uint64(1), uintVal(t, wired.Child("CRC Errors")))
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

	t.Run("eap/md5", func(t *testing.T) {
		// RFC 3748 §5.4 EAP-Request/MD5-Challenge: Value-Size 16, Name "host".
		// Wireshark eap.type=4 / eap.md5.value_size. EtherType 0x888e.
		val := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
		eap := append(append([]byte{0x01, 0x00, 0x00, 0x1a, 0x01, 0x01, 0x00, 0x1a, 0x04, 0x10}, val...), []byte("host")...)
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(4), uintVal(t, pkt.Child("Type")))
		md5 := mustChild(t, pkt, "MD5-Challenge")
		require.Equal(t, uint64(16), uintVal(t, md5.Child("Value Size")))
		require.Equal(t, val, bytesVal(t, md5.Child("Value")))
		require.Equal(t, "host", strVal(t, md5.Child("Name")))
		require.Nil(t, pkt.Child("Identity"))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		wired := mustChild(t, eth, "EAPOL", "EAPPacket", "MD5-Challenge")
		require.Equal(t, uint64(16), uintVal(t, wired.Child("Value Size")))
		require.Equal(t, "host", strVal(t, wired.Child("Name")))
	})

	t.Run("eap/notification", func(t *testing.T) {
		// RFC 3748 §5.2 EAP-Request/Notification Type-Data displayable "hello".
		// Wireshark eap.type=2 / eap.notification. EtherType 0x888e.
		eap := append([]byte{0x01, 0x00, 0x00, 0x0a, 0x01, 0x01, 0x00, 0x0a, 0x02}, []byte("hello")...)
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(2), uintVal(t, pkt.Child("Type")))
		require.Equal(t, "hello", strVal(t, pkt.Child("Notification")))
		require.Nil(t, pkt.Child("Identity"))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		require.Equal(t, "hello", strVal(t, mustChild(t, eth, "EAPOL", "EAPPacket").Child("Notification")))
	})

	t.Run("eap/nak", func(t *testing.T) {
		// RFC 3748 §5.3 EAP-Response/Nak: Type-Data desired Types 4 (MD5) then 13 (TLS).
		// Wireshark eap.type=3 / eap.desired_type. EtherType 0x888e.
		eap := []byte{0x01, 0x00, 0x00, 0x07, 0x02, 0x01, 0x00, 0x07, 0x03, 0x04, 0x0d}
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(3), uintVal(t, pkt.Child("Type")))
		nak := pkt.Child("Nak").Children()
		require.Equal(t, 2, len(nak))
		require.Equal(t, uint64(4), uintVal(t, nak[0]))
		require.Equal(t, uint64(13), uintVal(t, nak[1]))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		wired := mustChild(t, eth, "EAPOL", "EAPPacket").Child("Nak").Children()
		require.Equal(t, uint64(4), uintVal(t, wired[0]))
		require.Equal(t, uint64(13), uintVal(t, wired[1]))
	})

	t.Run("eap/otp", func(t *testing.T) {
		// RFC 3748 §5.5 OTP Type-Data is the RFC 2289 challenge "otp-md5 499 ke1234".
		// Wireshark eap.type=5 / eap.otp. EtherType 0x888e.
		ch := []byte("otp-md5 499 ke1234")
		eapLen := byte(5 + len(ch))
		eap := append([]byte{0x01, 0x00, 0x00, eapLen, 0x01, 0x01, 0x00, eapLen, 0x05}, ch...)
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(5), uintVal(t, pkt.Child("Type")))
		require.Equal(t, "otp-md5 499 ke1234", strVal(t, pkt.Child("OTP")))
		require.Nil(t, pkt.Child("Identity"))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		require.Equal(t, "otp-md5 499 ke1234", strVal(t, mustChild(t, eth, "EAPOL", "EAPPacket").Child("OTP")))
	})

	t.Run("eap/gtc", func(t *testing.T) {
		// RFC 3748 §5.6 GTC Request Type-Data is a displayable token-card challenge.
		// Wireshark eap.type=6 / eap.gtc. EtherType 0x888e.
		msg := []byte("Enter PIN")
		eapLen := byte(5 + len(msg))
		eap := append([]byte{0x01, 0x00, 0x00, eapLen, 0x01, 0x01, 0x00, eapLen, 0x06}, msg...)
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(6), uintVal(t, pkt.Child("Type")))
		require.Equal(t, "Enter PIN", strVal(t, pkt.Child("GTC")))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		require.Equal(t, "Enter PIN", strVal(t, mustChild(t, eth, "EAPOL", "EAPPacket").Child("GTC")))
	})

	t.Run("eap/expanded", func(t *testing.T) {
		// RFC 3748 §5.7 Expanded Type 254: Vendor-Id 311 (Microsoft), Vendor-Type 25 PEAP.
		// Wireshark eap.ext.vendor_id / eap.ext.vendor_type. EtherType 0x888e.
		eap := []byte{0x01, 0x00, 0x00, 0x0c, 0x01, 0x01, 0x00, 0x0c, 0xfe, 0x00, 0x00, 0x37, 0x00, 0x00, 0x00, 0x19}
		pkt := mustChild(t, parseRule(t, eap, "eapol", "EAPOL"), "EAPPacket")
		require.Equal(t, uint64(254), uintVal(t, pkt.Child("Type")))
		ex := mustChild(t, pkt, "Expanded")
		require.Equal(t, []byte{0x00, 0x00, 0x37}, bytesVal(t, ex.Child("Vendor-Id")))
		require.Equal(t, uint64(25), uintVal(t, ex.Child("Vendor Type")))
		require.Nil(t, pkt.Child("Identity"))
		eth := parseEthernet(t, eapolEthernetFrame(t, eap))
		wired := mustChild(t, eth, "EAPOL", "EAPPacket", "Expanded")
		require.Equal(t, []byte{0x00, 0x00, 0x37}, bytesVal(t, wired.Child("Vendor-Id")))
		require.Equal(t, uint64(25), uintVal(t, wired.Child("Vendor Type")))
	})

	t.Run("eapol/mka", func(t *testing.T) {
		// IEEE 802.1X-2010 Table 11-3 Packet Type 5 EAPOL-MKA. Wireshark eapol.type.
		// No MKA dissector: Body Length-bounded leftover is Next Protocol Data, not Unknown raw.
		raw := []byte{0x03, 0x05, 0x00, 0x04, 'm', 'k', 'a', '1'}
		n := parseRule(t, raw, "eapol", "EAPOL")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Protocol Version")))
		require.Equal(t, uint64(5), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(4), uintVal(t, n.Child("Body Length")))
		require.Equal(t, "mka1", joinUint8(t, n.Child("Next Protocol Data")))
		eth := parseEthernet(t, eapolEthernetFrame(t, raw))
		require.Equal(t, uint64(5), uintVal(t, mustChild(t, eth, "EAPOL").Child("Packet Type")))
		require.Equal(t, "mka1", joinUint8(t, mustChild(t, eth, "EAPOL").Child("Next Protocol Data")))
	})

	t.Run("eapol/announcement", func(t *testing.T) {
		// IEEE 802.1X-2010 Table 11-3 Packet Type 6 EAPOL-Announcement. EtherType 0x888e.
		raw := []byte{0x03, 0x06, 0x00, 0x03, 'a', 'n', 'n'}
		n := parseRule(t, raw, "eapol", "EAPOL")
		require.Equal(t, uint64(6), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, "ann", joinUint8(t, n.Child("Next Protocol Data")))
		eth := parseEthernet(t, eapolEthernetFrame(t, raw))
		require.Equal(t, "ann", joinUint8(t, mustChild(t, eth, "EAPOL").Child("Next Protocol Data")))
	})

	t.Run("jdwp/command-set", func(t *testing.T) {
		// JPDA VirtualMachine/Version (set 1, cmd 1), 11-byte header. Wireshark jdwp.commandset. TCP/5005.
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001000101")...)
		n := parseRule(t, jd, "jdwp", "JDWP")
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Command").Child("Command Set")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Command").Child("Command")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("Command Set")))
	})

	t.Run("jdwp/createstring", func(t *testing.T) {
		// JPDA VirtualMachine/CreateString (set 1, cmd 11): UTF-8 length-prefixed "hello".
		// Wireshark jdwp.command / jdwp.length. TCP/5005.
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "000000140000000200010b0000000568656c6c6f")...)
		n := parseRule(t, jd, "jdwp", "JDWP")
		cmd := mustChild(t, n, "Command")
		require.Equal(t, uint64(20), uintVal(t, cmd.Child("Length")))
		require.Equal(t, uint64(1), uintVal(t, cmd.Child("Command Set")))
		require.Equal(t, uint64(11), uintVal(t, cmd.Child("Command")))
		require.Equal(t, uint64(5), uintVal(t, cmd.Child("String Length")))
		require.Equal(t, "hello", strVal(t, cmd.Child("String")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
		require.Equal(t, "hello", strVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("String")))
	})

	t.Run("jdwp/event-set", func(t *testing.T) {
		// JPDA EventRequest/Set (set 15, cmd 1): CLASS_PREPARE, suspend all, 0 modifiers.
		// Wireshark jdwp.commandset. TCP/5005.
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000001100000003000f01080200000000")...)
		n := parseRule(t, jd, "jdwp", "JDWP")
		cmd := mustChild(t, n, "Command")
		require.Equal(t, uint64(15), uintVal(t, cmd.Child("Command Set")))
		require.Equal(t, uint64(1), uintVal(t, cmd.Child("Command")))
		require.Equal(t, uint64(8), uintVal(t, cmd.Child("Event Kind")))
		require.Equal(t, uint64(2), uintVal(t, cmd.Child("Suspend Policy")))
		require.Equal(t, uint64(0), uintVal(t, cmd.Child("Modifiers")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
		require.Equal(t, uint64(8), uintVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("Event Kind")))
	})

	t.Run("jdwp/reply", func(t *testing.T) {
		// JPDA reply header: flags 0x80, error code 0. Wireshark jdwp.errorcode.
		jd := append([]byte("JDWP-Handshake"), mustHex(t, "0000000b00000001800000")...)
		n := parseRule(t, jd, "jdwp", "JDWP")
		cmd := mustChild(t, n, "Command")
		require.Equal(t, uint64(0x80), uintVal(t, cmd.Child("Flags")))
		require.Equal(t, uint64(0), uintVal(t, cmd.Child("Error Code")))
		require.Nil(t, cmd.Child("Command Set"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5005, 5005, jd))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, eth, "IP", "TCP", "JDWP", "Command").Child("Error Code")))
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

	t.Run("snmp/get", func(t *testing.T) {
		// RFC 1157 GetRequest, community "public", sysDescr.0 (1.3.6.1.2.1.1.1.0).
		raw := mustHex(t, "302602010004067075626c6963a019020101020100020100300e300c06082b060102010101000500")
		n := parseRule(t, raw, "application-layer.snmp", "SNMP")
		require.Equal(t, []byte("public"), bytesVal(t, n.Child("Community")))
		require.Equal(t, uint64(0xa0), uintVal(t, n.Child("PDU Tag")))
		require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, n, "PDU Body", "Request ID")))
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, mustChild(t, n, "PDU Body", "Variable Bindings", "Bindings", "OID")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 161, 161, raw))
		require.Equal(t, []byte("public"), bytesVal(t, mustChild(t, eth, "IP", "UDP", "SNMP").Child("Community")))
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "SNMP", "PDU Body", "Variable Bindings", "Bindings", "OID")))
	})

	t.Run("snmpv3/header", func(t *testing.T) {
		// RFC 3412 §6 HeaderData: msgID=1, msgMaxSize=65507, msgFlags=0x04, securityModel=3 (USM).
		raw := mustHex(t, "3013020103300e020101020300ffe3040104020103")
		n := parseRule(t, raw, "application-layer.snmp", "SNMPv3")
		require.Equal(t, []byte{0x03}, bytesVal(t, n.Child("Version")))
		hdr := mustChild(t, n, "SNMPHeaderData")
		require.Equal(t, uint64(1), uintVal(t, hdr.Child("MsgID")))
		require.Equal(t, uint64(65507), uintVal(t, hdr.Child("MsgMaxSize")))
		require.Equal(t, uint64(4), uintVal(t, hdr.Child("MsgFlags")))
		require.Equal(t, uint64(3), uintVal(t, hdr.Child("Security Model")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 161, 161, raw))
		wired := mustChild(t, eth, "IP", "UDP", "SNMPv3", "SNMPHeaderData")
		require.Equal(t, uint64(1), uintVal(t, wired.Child("MsgID")))
		require.Equal(t, uint64(65507), uintVal(t, wired.Child("MsgMaxSize")))
		require.Equal(t, uint64(4), uintVal(t, wired.Child("MsgFlags")))
		require.Equal(t, uint64(3), uintVal(t, wired.Child("Security Model")))
	})

	t.Run("dcerpc/bind", func(t *testing.T) {
		// [MS-RPCE] 2.2.2.6 bind: EPM UUID 8a885d04-... is NDR transfer; abstract e1af8308-...
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
		require.Equal(t, uint64(11), uintVal(t, n.Child("PType")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "PDU", "Bind").Child("Num Ctx Items")))
		require.Equal(t, epm, bytesVal(t, mustChild(t, n, "PDU", "Bind", "Contexts").Children()[0].Child("Abstract Syntax")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 135, raw))
		require.Equal(t, uint64(11), uintVal(t, mustChild(t, eth, "IP", "TCP", "DCERPC").Child("PType")))
	})

	t.Run("ldap/bind", func(t *testing.T) {
		// Apache Directory BindRequestTest / RFC 4511 §4.2 simple bind:
		// version 3, name uid=akarasulu,dc=example,dc=com, simple "password".
		raw := mustHex(t, "3033020101602e020103041f"+
			"7569643d616b61726173756c752c64633d6578616d706c652c64633d636f6d"+
			"800870617373776f7264")
		n := parseRule(t, raw, "application-layer.ldap", "LDAPMessage")
		br := mustChild(t, n, "Body", "ProtocolOp", "BindRequest")
		require.Equal(t, []byte{3}, bytesVal(t, br.Child("Version")))
		require.Equal(t, "uid=akarasulu,dc=example,dc=com", strVal(t, br.Child("Name")))
		require.Equal(t, uint64(0x80), uintVal(t, br.Child("Auth Tag")))
		require.Equal(t, "password", strVal(t, br.Child("Auth")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 389, raw))
		wired := mustChild(t, eth, "IP", "TCP", "LDAPMessage", "Body", "ProtocolOp", "BindRequest")
		require.Equal(t, "uid=akarasulu,dc=example,dc=com", strVal(t, wired.Child("Name")))
		require.Equal(t, "password", strVal(t, wired.Child("Auth")))
		cldap := parseEthernet(t, ipv4UDPBytes(t, 389, 389, raw))
		require.Equal(t, "uid=akarasulu,dc=example,dc=com", strVal(t, mustChild(t, cldap, "IP", "UDP", "LDAPMessage", "Body", "ProtocolOp", "BindRequest").Child("Name")))
	})

	t.Run("ldap/unbind", func(t *testing.T) {
		// RFC 4511 §4.3 UnbindRequest APPLICATION 2 NULL.
		raw := mustHex(t, "30050201014200")
		n := parseRule(t, raw, "application-layer.ldap", "LDAPMessage")
		require.Equal(t, uint64(0x42), uintVal(t, mustChild(t, n, "Body").Child("ProtocolOp Tag")))
		require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, n, "Body").Child("MessageID")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 389, raw))
		require.Equal(t, uint64(0x42), uintVal(t, mustChild(t, eth, "IP", "TCP", "LDAPMessage", "Body").Child("ProtocolOp Tag")))
	})

	t.Run("dcerpc/bind-ack", func(t *testing.T) {
		// [MS-RPCE] 2.2.2.3 / DCE 1.1 bind_ack port_any_t; Wireshark packet-dcerpc.c
		// dissect_dcerpc_cn_bind_ack dcerpc.cn_sec_addr FT_STRINGZ "135".
		raw := mustHex(t, "05000c03100000003c00000001000000d016d0160100000004003133350000000100000000000000045d888aeb1cc9119fe808002b10486002000000")
		n := parseRule(t, raw, "application-layer.dcerpc", "DCERPC")
		require.Equal(t, uint64(12), uintVal(t, n.Child("PType")))
		ack := mustChild(t, n, "PDU", "BindAck")
		require.Equal(t, uint64(5840), uintVal(t, ack.Child("Max Xmit Frag")))
		require.Equal(t, uint64(4), uintVal(t, ack.Child("Sec Addr Len")))
		require.Equal(t, "135", strings.TrimRight(strVal(t, ack.Child("Sec Addr")), "\x00"))
		res := mustChild(t, ack, "DCERPCResults")
		require.Equal(t, uint64(1), uintVal(t, res.Child("Num Results")))
		require.Equal(t, uint64(0), uintVal(t, res.Child("Ack Result")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 135, 50000, raw))
		wired := mustChild(t, eth, "IP", "TCP", "DCERPC", "PDU", "BindAck")
		require.Equal(t, "135", strings.TrimRight(strVal(t, wired.Child("Sec Addr")), "\x00"))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, wired, "DCERPCResults").Child("Ack Result")))
	})

	t.Run("spnego/ntlm", func(t *testing.T) {
		// RFC 4178 §4.2 NegTokenInit thisMech 1.3.6.1.5.5.2; Wireshark spnego.mech.oid.
		// First mechTypes: NLMP 1.3.6.1.4.1.311.2.2.10 ([MS-SPNG] 4).
		raw := mustHex(t, "601c06062b0601050502a0123010a00e300c060a2b06010401823702020a")
		n := parseRule(t, raw, "application-layer.spnego", "SPNEGO")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}, bytesVal(t, n.Child("OID")))
		init := mustChild(t, n, "Token", "SPNEGOInit")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}, bytesVal(t, init.Child("MechOID")))
		body := make([]byte, 24)
		binary.LittleEndian.PutUint16(body[0:], 25)
		body[3] = 1
		binary.LittleEndian.PutUint16(body[12:], 88)
		binary.LittleEndian.PutUint16(body[14:], uint16(len(raw)))
		smb := append(smb2SyncHeader(1, 0, 3), body...)
		smb = append(smb, raw...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, smb))
		wired := mustChild(t, eth, "IP", "TCP", "SMB2", "Session Setup Request", "SPNEGO", "Token", "SPNEGOInit")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}, bytesVal(t, wired.Child("MechOID")))
	})

	t.Run("spnego/krb5", func(t *testing.T) {
		// RFC 4178 mechTypes Kerberos V5 1.2.840.113554.1.2.2 (Wireshark spnego.negTokenInit).
		raw := mustHex(t, "601b06062b0601050502a011300fa00d300b06092a864886f712010202")
		n := parseRule(t, raw, "application-layer.spnego", "SPNEGO")
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}, bytesVal(t, n.Child("OID")))
		init := mustChild(t, n, "Token", "SPNEGOInit")
		require.Equal(t, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02}, bytesVal(t, init.Child("MechOID")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, append(append(smb2SyncHeader(1, 0, 4), func() []byte {
			body := make([]byte, 24)
			binary.LittleEndian.PutUint16(body[0:], 25)
			body[3] = 1
			binary.LittleEndian.PutUint16(body[12:], 88)
			binary.LittleEndian.PutUint16(body[14:], uint16(len(raw)))
			return body
		}()...), raw...)))
		require.Equal(t, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02}, bytesVal(t, mustChild(t, eth, "IP", "TCP", "SMB2", "Session Setup Request", "SPNEGO", "Token", "SPNEGOInit").Child("MechOID")))
	})

	t.Run("kerberos/as-req", func(t *testing.T) {
		// RFC 4120 §5.4.1 KDC-REQ: pvno [1] INTEGER 5, msg-type [2] INTEGER 10 (AS-REQ).
		// Wireshark kerberos.pvno / kerberos.msg_type; APPLICATION 10 (0x6a).
		raw := mustHex(t, "6a0c300aa103020105a20302010a")
		n := parseRule(t, raw, "application-layer.kerberos", "Kerberos")
		require.Equal(t, uint64(0x6a), uintVal(t, n.Child("Application Tag")))
		req := mustChild(t, n, "Body", "KerberosASReq")
		require.Equal(t, uint64(5), uintVal(t, req.Child("Pvno")))
		require.Equal(t, uint64(10), uintVal(t, req.Child("MsgType")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 88, 88, raw))
		wired := mustChild(t, eth, "IP", "UDP", "Kerberos", "Body", "KerberosASReq")
		require.Equal(t, uint64(5), uintVal(t, wired.Child("Pvno")))
		require.Equal(t, uint64(10), uintVal(t, wired.Child("MsgType")))
	})

	t.Run("kerberos/as-rep", func(t *testing.T) {
		// RFC 4120 §5.4.2 KDC-REP: pvno [0] INTEGER 5, msg-type [1] INTEGER 11 (AS-REP).
		raw := mustHex(t, "6b0c300aa003020105a10302010b")
		n := parseRule(t, raw, "application-layer.kerberos", "Kerberos")
		rep := mustChild(t, n, "Body", "KerberosASRep")
		require.Equal(t, uint64(5), uintVal(t, rep.Child("Pvno")))
		require.Equal(t, uint64(11), uintVal(t, rep.Child("MsgType")))
		pdu := make([]byte, 4+len(raw))
		binary.BigEndian.PutUint32(pdu, uint32(len(raw)))
		copy(pdu[4:], raw)
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 88, pdu))
		wired := mustChild(t, eth, "IP", "TCP", "KerberosTCP", "Record", "Body", "KerberosASRep")
		require.Equal(t, uint64(5), uintVal(t, wired.Child("Pvno")))
		require.Equal(t, uint64(11), uintVal(t, wired.Child("MsgType")))
	})

	t.Run("ajp/cping", func(t *testing.T) {
		// Apache AJP13 / Wireshark packet-ajp13.c MTYPE_CPING: magic 0x1234, length 1, code 10.
		raw := mustHex(t, "123400010a")
		n := parseRule(t, raw, "application-layer.ajp", "AJP")
		require.Equal(t, uint64(0x1234), uintVal(t, n.Child("Magic")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Length")))
		require.Equal(t, uint64(0x0a), uintVal(t, n.Child("Code")))
		require.Nil(t, n.Child("AJPForward"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8009, raw))
		require.Equal(t, uint64(0x0a), uintVal(t, mustChild(t, eth, "IP", "TCP", "AJP").Child("Code")))
		pong := mustHex(t, "4142000109")
		g := parseRule(t, pong, "application-layer.ajp", "AJP")
		require.Equal(t, uint64(0x4142), uintVal(t, g.Child("Magic")))
		require.Equal(t, uint64(0x09), uintVal(t, g.Child("Code")))
	})

	t.Run("ajp/forward", func(t *testing.T) {
		// Apache AJP13 FORWARD_REQUEST: method GET(2), protocol HTTP/1.1, uri /,
		// remote 127.0.0.1, server localhost:80, 0 headers, terminator 0xff.
		// Wireshark ajp13.method / ajp13.uri.
		raw := mustHex(t, "1234003202020008485454502f312e310000012f0000093132372e302e302e310000000000096c6f63616c686f7374000050000000ff")
		n := parseRule(t, raw, "application-layer.ajp", "AJP")
		require.Equal(t, uint64(0x02), uintVal(t, n.Child("Code")))
		fwd := mustChild(t, n, "AJPForward")
		require.Equal(t, uint64(2), uintVal(t, fwd.Child("Method")))
		require.Equal(t, "HTTP/1.1", strVal(t, fwd.Child("Protocol")))
		require.Equal(t, "/", strVal(t, fwd.Child("URI")))
		require.Equal(t, "127.0.0.1", strVal(t, fwd.Child("RemoteAddr")))
		require.Equal(t, "localhost", strVal(t, fwd.Child("ServerName")))
		require.Equal(t, uint64(80), uintVal(t, fwd.Child("ServerPort")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 8009, raw))
		wired := mustChild(t, eth, "IP", "TCP", "AJP", "AJPForward")
		require.Equal(t, uint64(2), uintVal(t, wired.Child("Method")))
		require.Equal(t, "/", strVal(t, wired.Child("URI")))
	})

	t.Run("tds/prelogin", func(t *testing.T) {
		// [MS-TDS] 2.2.6.5 PRELOGIN VERSION token 0 + terminator 0xff, then UL_VERSION 12.0.
		// Wireshark packet-tds.c TDS7_PRELOGIN_OPTION_VERSION / tds.prelogin.
		raw := tdsPacket(18, 1, mustHex(t, "0000060006ff0c0000000000"))
		n := parseRule(t, raw, "application-layer.tds", "TDS")
		require.Equal(t, uint64(18), uintVal(t, n.Child("Type")))
		toks := n.Child("Prelogin")
		require.True(t, toks.IsList())
		require.Equal(t, uint64(0), uintVal(t, toks.Children()[0].Child("Type")))
		require.Equal(t, uint64(0xff), uintVal(t, toks.Children()[len(toks.Children())-1].Child("Type")))
		ver := mustChild(t, n, "TDSVersionData")
		require.Equal(t, uint64(12), uintVal(t, ver.Child("Version Major")))
		require.Equal(t, uint64(0), uintVal(t, ver.Child("Version Minor")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1433, raw))
		require.Equal(t, uint64(12), uintVal(t, mustChild(t, eth, "IP", "TCP", "TDS", "TDSVersionData").Child("Version Major")))
	})

	t.Run("tds/login7", func(t *testing.T) {
		// [MS-TDS] 2.2.6.4 LOGIN7: Length, TDSVersion 7.4 (0x74000004), PacketSize 4096,
		// ibHostName + UTF-16LE "host". Wireshark tds.version / hostname.
		payload := mustHex(t, "6600000004000074001000000000000000000000000000000000000000000000000000005e00040000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000068006f0073007400")
		raw := tdsPacket(16, 1, payload)
		n := parseRule(t, raw, "application-layer.tds", "TDS")
		require.Equal(t, uint64(16), uintVal(t, n.Child("Type")))
		lg := mustChild(t, n, "TDSLogin7")
		require.Equal(t, uint64(102), uintVal(t, lg.Child("Login Length")))
		require.Equal(t, uint64(0x74000004), uintVal(t, lg.Child("TDS Version")))
		require.Equal(t, uint64(4096), uintVal(t, lg.Child("Packet Size")))
		require.Equal(t, "host", strings.ReplaceAll(strVal(t, lg.Child("HostName")), "\x00", ""))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1433, raw))
		wired := mustChild(t, eth, "IP", "TCP", "TDS", "TDSLogin7")
		require.Equal(t, uint64(0x74000004), uintVal(t, wired.Child("TDS Version")))
		require.Equal(t, "host", strings.ReplaceAll(strVal(t, wired.Child("HostName")), "\x00", ""))
	})

	t.Run("mysql/query", func(t *testing.T) {
		// MySQL COM_QUERY (0x03) "SELECT 1".
		// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_com_query.html
		// Wireshark mysql.query.
		raw := mysqlPacket(0, append([]byte{0x03}, []byte("SELECT 1")...))
		n := parseRule(t, raw, "application-layer.mysql", "MySQLPacket")
		require.Equal(t, uint64(0x03), uintVal(t, mustChild(t, n, "Payload").Child("First")))
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, n, "Payload").Child("Query")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 3306, raw))
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, eth, "IP", "TCP", "MySQL", "Payload").Child("Query")))
	})

	t.Run("mysql/err", func(t *testing.T) {
		// MySQL ERR packet: Error Code 1045, SQLSTATE 28000, "Access denied".
		// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_err_packet.html
		// Wireshark mysql.error_code / mysql.sqlstate.
		raw := mysqlPacket(1, []byte{0xff, 0x15, 0x04, '#', '2', '8', '0', '0', '0', 'A', 'c', 'c', 'e', 's', 's', ' ', 'd', 'e', 'n', 'i', 'e', 'd'})
		errp := mustChild(t, parseRule(t, raw, "application-layer.mysql", "MySQLPacket"), "Payload", "MySQLERR")
		require.Equal(t, uint64(1045), uintVal(t, errp.Child("Error Code")))
		require.Equal(t, "28000", strVal(t, errp.Child("SQL State")))
		require.Equal(t, "Access denied", strVal(t, errp.Child("Error Message")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 3306, 50000, raw))
		wired := mustChild(t, eth, "IP", "TCP", "MySQL", "Payload", "MySQLERR")
		require.Equal(t, uint64(1045), uintVal(t, wired.Child("Error Code")))
		require.Equal(t, "28000", strVal(t, wired.Child("SQL State")))
	})

	t.Run("mysql/ok", func(t *testing.T) {
		// MySQL OK packet after COM_QUERY: affected_rows=0, last_insert_id=0,
		// SERVER_STATUS_AUTOCOMMIT=0x0002, warnings=0.
		// https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_ok_packet.html
		raw := mysqlPacket(1, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00})
		okp := mustChild(t, parseRule(t, raw, "application-layer.mysql", "MySQLPacket"), "Payload", "MySQLOK")
		require.Equal(t, uint64(0), uintVal(t, okp.Child("Affected Rows")))
		require.Equal(t, uint64(0), uintVal(t, okp.Child("Last Insert ID")))
		require.Equal(t, uint64(2), uintVal(t, okp.Child("Status Flags")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 3306, 50000, raw))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "MySQL", "Payload", "MySQLOK").Child("Status Flags")))
	})

	t.Run("redis/ping", func(t *testing.T) {
		// RESP2: clients send commands as an Array of Bulk Strings.
		// https://github.com/redis/redis-specifications/blob/master/protocol/RESP2.md
		// Wireshark redis.command PING.
		raw := []byte("*1\r\n$4\r\nPING\r\n")
		n := redisRoot(t, parseRule(t, raw, "application-layer.redis", "Redis"))
		require.Equal(t, uint64('*'), uintVal(t, n.Child("Prefix")))
		require.Equal(t, "PING", strVal(t, mustChild(t, n, "Array", "RedisCommand").Child("Command")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 6379, raw))
		wired := redisRoot(t, mustChild(t, eth, "IP", "TCP", "Redis"))
		require.Equal(t, "PING", strVal(t, mustChild(t, wired, "Array", "RedisCommand").Child("Command")))
	})

	t.Run("redis/get", func(t *testing.T) {
		// RESP2 GET mykey: *2 $3 GET $5 mykey. Wireshark redis.command / redis.bulk.
		raw := []byte("*2\r\n$3\r\nGET\r\n$5\r\nmykey\r\n")
		n := redisRoot(t, parseRule(t, raw, "application-layer.redis", "Redis"))
		require.Equal(t, "GET", strVal(t, mustChild(t, n, "Array", "RedisCommand").Child("Command")))
		args := mustChild(t, n, "Array", "Arguments")
		require.True(t, args.IsList())
		require.Len(t, args.Children(), 1)
		require.Equal(t, "mykey", strVal(t, mustChild(t, args.Children()[0], "Bulk").Child("Bulk")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 6379, raw))
		wired := redisRoot(t, mustChild(t, eth, "IP", "TCP", "Redis"))
		require.Equal(t, "GET", strVal(t, mustChild(t, wired, "Array", "RedisCommand").Child("Command")))
		require.Equal(t, "mykey", strVal(t, mustChild(t, mustChild(t, wired, "Array", "Arguments").Children()[0], "Bulk").Child("Bulk")))
	})

	t.Run("postgres/query", func(t *testing.T) {
		// PostgreSQL protocol 3.0 Query (F) 'Q' + SQL NUL. Wireshark pgsql.query.
		// https://www.postgresql.org/docs/current/protocol-message-formats.html
		raw := pgTyped('Q', append([]byte("SELECT 1"), 0))
		n := parseRule(t, raw, "application-layer.postgresql", "PostgreSQL")
		require.Equal(t, uint64('Q'), uintVal(t, n.Child("First")))
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, n, "Payload", "PostgreSQLQuery").Child("SQL")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 5432, raw))
		require.Equal(t, "SELECT 1", strVal(t, mustChild(t, eth, "IP", "TCP", "PostgreSQL", "Payload", "PostgreSQLQuery").Child("SQL")))
	})

	t.Run("postgres/error", func(t *testing.T) {
		// ErrorResponse fields S/C/M: Severity ERROR, SQLSTATE 42601 syntax_error.
		// https://www.postgresql.org/docs/current/protocol-error-fields.html
		// Wireshark pgsql.error.code / pgsql.error.message.
		raw := pgTyped('E', []byte("SERROR\x00C42601\x00Msyntax\x00\x00"))
		fields := mustChild(t, parseRule(t, raw, "application-layer.postgresql", "PostgreSQL"), "Payload", "PostgreSQLError").Children()
		require.GreaterOrEqual(t, len(fields), 3)
		require.Equal(t, "ERROR", strVal(t, fields[0].Child("Severity")))
		require.Equal(t, "42601", strVal(t, fields[1].Child("SQLState")))
		require.Equal(t, "syntax", strVal(t, fields[2].Child("Message")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5432, 50000, raw))
		wired := mustChild(t, eth, "IP", "TCP", "PostgreSQL", "Payload", "PostgreSQLError").Children()
		require.Equal(t, "42601", strVal(t, wired[1].Child("SQLState")))
	})

	t.Run("tpkt/cookie", func(t *testing.T) {
		// [MS-RDPBCGR] 4.1.1 Client X.224 Connection Request PDU.
		// Cookie: mstshash=eltons + RDP_NEG_REQ type 1 length 8 PROTOCOL_RDP.
		// TPKT TPDU stays raw (G4 ABI). Wireshark tpkt / cotp / rdp.neg.request.
		raw := mustHex(t, "0300002c27e00000000000436f6f6b69653a206d737473686173683d656c746f6e730d0a0100080000000000")
		tpkt := parseRule(t, raw, "application-layer.msrdp", "TPKT")
		require.Equal(t, uint64(3), uintVal(t, tpkt.Child("Version")))
		require.Equal(t, uint64(0x2c), uintVal(t, tpkt.Child("PacketLength")))
		require.Equal(t, raw[4:], bytesVal(t, tpkt.Child("TPDU")))
		rdp := parseRule(t, raw, "application-layer.msrdp", "RDP")
		require.Equal(t, uint64(0xe0), uintVal(t, mustChild(t, rdp, "X224").Child("Flag")))
		require.Equal(t, "Cookie: mstshash=eltons", strVal(t, mustChild(t, rdp, "X224", "VariableData", "RDPCookie").Child("Line")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, rdp, "X224", "VariableData", "RDPNegotiation").Child("Type")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, rdp, "X224", "VariableData", "RDPNegotiation").Child("Protocol")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 3389, raw))
		wired := mustChild(t, eth, "IP", "TCP", "RDP", "X224", "VariableData")
		require.Equal(t, "Cookie: mstshash=eltons", strVal(t, mustChild(t, wired, "RDPCookie").Child("Line")))
		require.Equal(t, uint64(0), uintVal(t, mustChild(t, wired, "RDPNegotiation").Child("Protocol")))
	})

	t.Run("tpkt/cc", func(t *testing.T) {
		// [MS-RDPBCGR] X.224 Connection Confirm: TPDU code 0xd0, no cookie.
		raw := []byte{0x03, 0x00, 0x00, 0x0b, 0x06, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00}
		require.Equal(t, uint64(0xd0), uintVal(t, mustChild(t, parseRule(t, raw, "application-layer.msrdp", "RDP"), "X224").Child("Flag")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 3389, 50000, raw))
		require.Equal(t, uint64(0xd0), uintVal(t, mustChild(t, eth, "IP", "TCP", "RDP", "X224").Child("Flag")))
	})

	t.Run("mdns/ptr", func(t *testing.T) {
		// RFC 6763 DNS-SD PTR query _http._tcp.local. Wireshark dns.qry.name.
		raw := []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x05, '_', 'h', 't', 't', 'p', 0x04, '_', 't', 'c', 'p', 0x05, 'l', 'o', 'c', 'a', 'l', 0x00,
			0x00, 0x0c, 0x00, 0x01,
		}
		n := parseRule(t, raw, "application-layer.dns", "DNS")
		q := n.Child("Questions").Children()[0]
		require.Equal(t, uint64(12), uintVal(t, q.Child("Type")))
		labels := q.Child("Name").Children()
		require.GreaterOrEqual(t, len(labels), 3)
		require.Equal(t, "_http", strVal(t, labels[0].Child("Text")))
		require.Equal(t, "_tcp", strVal(t, labels[1].Child("Text")))
		require.Equal(t, "local", strVal(t, labels[2].Child("Text")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5353, 5353, raw))
		wired := mustChild(t, eth, "IP", "UDP", "MDNS")
		require.Equal(t, "_http", strVal(t, wired.Child("Questions").Children()[0].Child("Name").Children()[0].Child("Text")))
		require.Equal(t, uint64(12), uintVal(t, wired.Child("Questions").Children()[0].Child("Type")))
	})

	t.Run("mdns/a", func(t *testing.T) {
		// Existing ethernet DNS A response for cloudconfig.jetbrains.com (parse_test.go fixture).
		// Wireshark dns.a 52.18.236.21. Replay on UDP/5353 as mDNS.
		raw := mustHex(t, "bc35818000010002000000000b636c6f7564636f6e666967096a6574627261696e7303636f6d0000010001c00c000100010000001300043412ec15c00c00010001000000130004364dbb13")
		n := parseRule(t, raw, "application-layer.dns", "DNS")
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, n, "Header").Child("Answer RRs")))
		ans := n.Child("Answers").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, ans.Child("Type")))
		require.Equal(t, []byte{0x34, 0x12, 0xec, 0x15}, bytesVal(t, mustChild(t, ans, "DNSA").Child("Address")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5353, 5353, raw))
		wired := mustChild(t, eth, "IP", "UDP", "MDNS", "Answers").Children()[0]
		require.Equal(t, []byte{0x34, 0x12, 0xec, 0x15}, bytesVal(t, mustChild(t, wired, "DNSA").Child("Address")))
	})

	t.Run("nbns/stat", func(t *testing.T) {
		// RFC 1002 NBSTAT query for "*" (first-level encoded CK + 30 A's).
		// bettercap packets/nbns.go NBNSRequest; Wireshark nbns.name / nbns.type=NBSTAT.
		raw := nbnsStarStatQuery()
		n := parseRule(t, raw, "application-layer.nbns", "NBNS")
		q := n.Child("Questions").Children()[0]
		require.Equal(t, uint64(0x21), uintVal(t, q.Child("Type")))
		require.Equal(t, "CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", strVal(t, q.Child("Name").Children()[0].Child("Text")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 137, 137, raw))
		wired := mustChild(t, eth, "IP", "UDP", "NBNS", "Questions").Children()[0]
		require.Equal(t, uint64(0x21), uintVal(t, wired.Child("Type")))
		require.Equal(t, "CKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", strVal(t, wired.Child("Name").Children()[0].Child("Text")))
	})

	t.Run("llmnr/a", func(t *testing.T) {
		// RFC 4795 LLMNR is DNS-shaped; A RDATA 10.0.0.9. Wireshark llmnr / dns.a.
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
		n := parseRule(t, ans, "application-layer.nbns", "LLMNR")
		require.Equal(t, "TEST", strVal(t, n.Child("Questions").Children()[0].Child("Name").Children()[0].Child("Text")))
		require.Equal(t, []byte{10, 0, 0, 9}, bytesVal(t, mustChild(t, n.Child("Answers").Children()[0], "NBNSA").Child("Address")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5355, 5355, ans))
		require.Equal(t, []byte{10, 0, 0, 9}, bytesVal(t, mustChild(t, mustChild(t, eth, "IP", "UDP", "LLMNR", "Answers").Children()[0], "NBNSA").Child("Address")))
	})

	t.Run("mqtt/publish", func(t *testing.T) {
		// MQTT 3.1.1 §3.3 PUBLISH QoS0: Topic Name then application Message.
		// Wireshark mqtt.topic / mqtt.msg (sensor/temp, 23.5).
		raw := append([]byte{0x30, 0x11, 0x00, 0x0b}, []byte("sensor/temp23.5")...)
		n := parseRule(t, raw, "application-layer.mqtt", "MQTT")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Packet Type")))
		pub := mustChild(t, n, "Payload", "Publish")
		require.Equal(t, "sensor/temp", strVal(t, pub.Child("Topic")))
		require.Equal(t, "23.5", strVal(t, pub.Child("Message")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1883, raw))
		wired := mustChild(t, eth, "IP", "TCP", "MQTT", "Payload", "Publish")
		require.Equal(t, "sensor/temp", strVal(t, wired.Child("Topic")))
		require.Equal(t, "23.5", strVal(t, wired.Child("Message")))
	})

	t.Run("mqtt/qos1", func(t *testing.T) {
		// MQTT 3.1.1 §3.3.2-2: Packet Identifier present only when QoS > 0.
		raw := append([]byte{0x32, 0x0a, 0x00, 0x01, 'a', 0x00, 0x07}, []byte("hello")...)
		pub := mustChild(t, parseRule(t, raw, "application-layer.mqtt", "MQTT"), "Payload", "Publish")
		require.Equal(t, "a", strVal(t, pub.Child("Topic")))
		require.Equal(t, uint64(7), uintVal(t, pub.Child("Packet ID")))
		require.Equal(t, "hello", strVal(t, pub.Child("Message")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1883, raw))
		wired := mustChild(t, eth, "IP", "TCP", "MQTT", "Payload", "Publish")
		require.Equal(t, uint64(7), uintVal(t, wired.Child("Packet ID")))
		require.Equal(t, "hello", strVal(t, wired.Child("Message")))
	})

	t.Run("http/post", func(t *testing.T) {
		// RFC 9112 §6.2 Content-Length; Wireshark http.request.method / http.file_data.
		raw := []byte("POST /submit HTTP/1.1\r\nHost: origin.example\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 7\r\n\r\nfoo=bar")
		req := mustChild(t, parseRule(t, raw, "application-layer.http", "HTTP"), "HTTP Request")
		require.Equal(t, "POST", strVal(t, req.Child("Method")))
		require.Equal(t, "/submit", strVal(t, req.Child("Path")))
		require.Equal(t, "foo=bar", strVal(t, mustChild(t, req, "Body").Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, raw))
		wired := mustChild(t, eth, "IP", "TCP", "HTTP", "HTTP Request")
		require.Equal(t, "POST", strVal(t, wired.Child("Method")))
		require.Equal(t, "foo=bar", strVal(t, mustChild(t, wired, "Body").Child("Octets")))
	})

	t.Run("http/chunked", func(t *testing.T) {
		// RFC 9112 §6.3.4 / §7.1 chunked: size 5 "hello" then last-chunk 0.
		// Wireshark http.transfer_encoding / http.chunk_size / http.file_data.
		raw := []byte("POST / HTTP/1.1\r\nHost: example.tld\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n")
		chunks := mustChild(t, parseRule(t, raw, "application-layer.http", "HTTP"), "HTTP Request", "Body", "DataChunks").Children()
		require.GreaterOrEqual(t, len(chunks), 1)
		require.Equal(t, "5", strVal(t, chunks[0].Child("Size")))
		require.Equal(t, "hello", strVal(t, chunks[0].Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, raw))
		wired := mustChild(t, eth, "IP", "TCP", "HTTP", "HTTP Request", "Body", "DataChunks").Children()
		require.Equal(t, "hello", strVal(t, wired[0].Child("Octets")))
	})

	t.Run("http/ok", func(t *testing.T) {
		// RFC 9112 §4 status line + §6.2 Content-Length body.
		raw := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello")
		resp := mustChild(t, parseRule(t, raw, "application-layer.http", "HTTP"), "HTTP Response")
		require.Equal(t, "HTTP/1.1", strVal(t, resp.Child("Version")))
		require.Equal(t, "200", strVal(t, resp.Child("Status")))
		require.Equal(t, "hello", strVal(t, mustChild(t, resp, "Body").Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 80, 50000, raw))
		require.Equal(t, "hello", strVal(t, mustChild(t, eth, "IP", "TCP", "HTTP", "HTTP Response", "Body").Child("Octets")))
	})

	t.Run("http2/data", func(t *testing.T) {
		// RFC 9113 §6.1 DATA END_STREAM; Wireshark http2.data.data.
		raw := make([]byte, 9+5)
		raw[2] = 5
		raw[4] = 0x01
		copy(raw[9:], []byte("hello"))
		n := parseRule(t, raw, "application-layer.http2", "HTTP2")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Type")))
		require.Equal(t, "hello", strVal(t, n.Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, raw))
		require.Equal(t, "hello", strVal(t, mustChild(t, eth, "IP", "TCP", "HTTP2").Child("Octets")))
	})

	t.Run("http2/rst", func(t *testing.T) {
		// RFC 9113 §6.4 RST_STREAM PROTOCOL_ERROR=1 on stream 1.
		// Wireshark http2.rst_stream.error.
		raw := []byte{0x00, 0x00, 0x04, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01}
		n := parseRule(t, raw, "application-layer.http2", "HTTP2")
		require.Equal(t, uint64(3), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Stream Identifier")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Error Code")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 443, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "HTTP2").Child("Error Code")))
	})

	t.Run("http2/goaway", func(t *testing.T) {
		// RFC 9113 §6.8 GOAWAY Additional Debug Data; Wireshark http2.goaway.debug_data.
		raw := append([]byte{0x00, 0x00, 0x0b, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, []byte("bye")...)
		n := parseRule(t, raw, "application-layer.http2", "HTTP2")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Error Code")))
		require.Equal(t, "bye", strVal(t, n.Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 443, raw))
		require.Equal(t, "bye", strVal(t, mustChild(t, eth, "IP", "TCP", "HTTP2").Child("Octets")))
	})

	t.Run("tls/suites", func(t *testing.T) {
		// RFC 5246 / RFC 8446 ClientHello cipher_suites: TLS_RSA_WITH_AES_128_CBC_SHA
		// (0x002f) and TLS_RSA_WITH_AES_256_CBC_SHA (0x0035). Wireshark tls.handshake.ciphersuite.
		hello := append([]byte{0x03, 0x03}, make([]byte, 32)...)
		hello = append(hello, 0, 0x00, 0x04, 0x00, 0x2f, 0x00, 0x35, 0x01, 0x00)
		hs := append([]byte{0x01, 0x00, 0x00, byte(len(hello))}, hello...)
		suites := mustChild(t, parseRule(t, hs, "application-layer.tls_hello", "TLSClientHello"), "ClientHello", "Cipher Suites").Children()
		require.Equal(t, uint64(0x002f), uintVal(t, suites[0].Child("Suite")))
		require.Equal(t, uint64(0x0035), uintVal(t, suites[1].Child("Suite")))
		rec := append([]byte{0x16, 0x03, 0x03, 0x00, byte(len(hs))}, hs...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 443, rec))
		wired := mustChild(t, eth, "IP", "TCP", "TLS", "Record Layer", "TLSClientHello", "ClientHello", "Cipher Suites").Children()
		require.Equal(t, uint64(0x002f), uintVal(t, wired[0].Child("Suite")))
	})

	t.Run("tls/sni", func(t *testing.T) {
		// RFC 6066 §3 server_name HostName example.com. Wireshark tls.handshake.extensions_server_name.
		body := append([]byte{0x03, 0x03}, make([]byte, 32)...)
		body = append(body, 0x00, 0x00, 0x04, 0x00, 0x2f, 0x00, 0x35, 0x01, 0x00)
		sni := append([]byte{0x00, 0x14, 0x00, 0x00, 0x00, 0x10, 0x00, 0x0e, 0x00, 0x00, 0x0b}, []byte("example.com")...)
		body = append(body, sni...)
		hs := append([]byte{0x01, 0x00, 0x00, byte(len(body))}, body...)
		exts := mustChild(t, parseRule(t, hs, "application-layer.tls_hello", "TLSClientHello"), "ClientHello", "Extensions").Children()
		require.Equal(t, uint64(0), uintVal(t, exts[0].Child("Type")))
		require.Equal(t, "example.com", strVal(t, mustChild(t, exts[0], "SNI").Child("Host Name")))
		rec := append([]byte{0x16, 0x03, 0x03, 0x00, byte(len(hs))}, hs...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 443, rec))
		wired := mustChild(t, eth, "IP", "TCP", "TLS", "Record Layer", "TLSClientHello", "ClientHello", "Extensions").Children()
		require.Equal(t, "example.com", strVal(t, mustChild(t, wired[0], "SNI").Child("Host Name")))
	})

	t.Run("tns/connect", func(t *testing.T) {
		// Oracle TNS CONNECT; Wireshark tns.connect_data / tns.connect_data_length.
		// Connect descriptor SERVICE_NAME=ORCL (Oracle Net / tnsnames.ora).
		cdata := []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=ORCL)))")
		pkt := tnsConnectPacket(cdata)
		n := parseRule(t, pkt, "application-layer.tns", "TNS")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(len(cdata)), uintVal(t, mustChild(t, n, "Connect").Child("Connect Data Length")))
		require.Equal(t, string(cdata), strVal(t, mustChild(t, n, "Connect").Child("Connect Data")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1521, pkt))
		require.Equal(t, string(cdata), strVal(t, mustChild(t, eth, "IP", "TCP", "TNS", "Connect").Child("Connect Data")))
	})

	t.Run("tns/data", func(t *testing.T) {
		// TNS DATA (type 6): 2-byte Data Flag then payload. Wireshark tns.data_flag.
		raw := make([]byte, 12)
		binary.BigEndian.PutUint16(raw[0:], 12)
		raw[4] = 6
		copy(raw[10:], []byte("AB"))
		n := parseRule(t, raw, "application-layer.tns", "TNS")
		require.Equal(t, uint64(6), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Data Flag")))
		require.Equal(t, "AB", strVal(t, n.Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1521, raw))
		require.Equal(t, "AB", strVal(t, mustChild(t, eth, "IP", "TCP", "TNS").Child("Octets")))
	})

	t.Run("smb2/tree-connect", func(t *testing.T) {
		// [MS-SMB2] 2.2.9 TREE_CONNECT Request; Wireshark smb2.tree_connect.path.
		path := utf16LE(`\\srv\share`)
		tcBody := make([]byte, 8+len(path))
		binary.LittleEndian.PutUint16(tcBody[0:], 9)
		binary.LittleEndian.PutUint16(tcBody[4:], 72)
		binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
		copy(tcBody[8:], path)
		raw := append(smb2SyncHeader(3, 0, 4), tcBody...)
		req := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Tree Connect Request")
		require.Equal(t, path, bytesVal(t, req.Child("Path")))
		require.Equal(t, `\\srv\share`, strings.ReplaceAll(strVal(t, req.Child("Path")), "\x00", ""))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
		require.Equal(t, `\\srv\share`, strings.ReplaceAll(strVal(t, mustChild(t, eth, "IP", "TCP", "SMB2", "Tree Connect Request").Child("Path")), "\x00", ""))
	})

	t.Run("smb2/create", func(t *testing.T) {
		// [MS-SMB2] 2.2.13 CREATE Request; Wireshark smb2.filename.
		name := utf16LE("file.txt")
		cr := make([]byte, 56+len(name))
		binary.LittleEndian.PutUint16(cr[0:], 57)
		binary.LittleEndian.PutUint32(cr[4:], 2)
		binary.LittleEndian.PutUint32(cr[36:], 1)
		binary.LittleEndian.PutUint16(cr[44:], 120)
		binary.LittleEndian.PutUint16(cr[46:], uint16(len(name)))
		copy(cr[56:], name)
		raw := append(smb2SyncHeader(5, 0, 5), cr...)
		creq := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Create Request")
		require.Equal(t, "file.txt", strings.ReplaceAll(strVal(t, creq.Child("Name")), "\x00", ""))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
		require.Equal(t, "file.txt", strings.ReplaceAll(strVal(t, mustChild(t, eth, "IP", "TCP", "SMB2", "Create Request").Child("Name")), "\x00", ""))
	})

	t.Run("smb2/read", func(t *testing.T) {
		// [MS-SMB2] 2.2.20 READ Response; Wireshark smb2.read_length / smb2.file_data.
		rdata := []byte("abcdefgh")
		rr := make([]byte, 16+len(rdata))
		binary.LittleEndian.PutUint16(rr[0:], 17)
		rr[2] = 80
		binary.LittleEndian.PutUint32(rr[4:], uint32(len(rdata)))
		copy(rr[16:], rdata)
		raw := append(smb2SyncHeader(8, 1, 6), rr...)
		require.Equal(t, "abcdefgh", strVal(t, mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Read Response").Child("Octets")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 445, 50000, raw))
		require.Equal(t, "abcdefgh", strVal(t, mustChild(t, eth, "IP", "TCP", "SMB2", "Read Response").Child("Octets")))
	})

	t.Run("smb/tree-connect", func(t *testing.T) {
		// [MS-CIFS] 2.2.4.55 TREE_CONNECT_ANDX; Wireshark smb.path / smb.service.
		raw := smb1TreeConnectAndX(`\\srv\share`, "A:")
		n := parseRule(t, raw, "application-layer.smb", "SMB")
		require.Equal(t, uint64(0x75), uintVal(t, n.Child("Command")))
		tc := mustChild(t, n, "TreeConnectAndX")
		require.Equal(t, `\\srv\share`, strVal(t, tc.Child("Path")))
		require.Equal(t, "A:", strVal(t, tc.Child("Service")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, raw))
		wired := mustChild(t, eth, "IP", "TCP", "SMB", "TreeConnectAndX")
		require.Equal(t, `\\srv\share`, strVal(t, wired.Child("Path")))
		require.Equal(t, "A:", strVal(t, wired.Child("Service")))
	})

	t.Run("gre/rfc2784", func(t *testing.T) {
		// RFC 2784 §2.1: C=K=S=0 Ver=0, Protocol Type 0x0806 ARP.
		// Wireshark gre.proto. A PPTP layout would steal 0x0001 as Payload Length.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append([]byte{0x00, 0x00, 0x08, 0x06}, arp...)
		n := parseRule(t, raw, "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Flags And Version")))
		require.Equal(t, uint64(0x0806), uintVal(t, n.Child("Protocol Type")))
		require.Nil(t, n.Child("Call ID"))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Payload", "ARP").Child("Opcode")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, raw))
		require.Equal(t, uint64(0x0806), uintVal(t, mustChild(t, eth, "IP", "GRE").Child("Protocol Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "GRE", "Payload", "ARP").Child("Opcode")))
	})

	t.Run("gre/key", func(t *testing.T) {
		// RFC 2890 Key Present (K=1 Ver=0); Wireshark gre.key.
		// PPTP would parse 0x12345678 as Payload Length=0x1234 Call ID=0x5678.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append([]byte{0x20, 0x00, 0x08, 0x06, 0x12, 0x34, 0x56, 0x78}, arp...)
		n := parseRule(t, raw, "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0x2000), uintVal(t, n.Child("Flags And Version")))
		require.Equal(t, uint64(0x12345678), uintVal(t, n.Child("Key")))
		require.NotEqual(t, uint64(0x5678), uintVal(t, n.Child("Key")))
		require.Nil(t, n.Child("Call ID"))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, raw))
		require.Equal(t, uint64(0x12345678), uintVal(t, mustChild(t, eth, "IP", "GRE").Child("Key")))
	})

	t.Run("gre/next-type", func(t *testing.T) {
		// RFC 2784 Protocol Type is an EtherType. IEEE Std 802-2014 Table C-1
		// Local Experimental EtherType 1 (0x88B5) has no GRE dissector arm:
		// leftover is Next Protocol Data, not an unnamed tail. IP proto 47.
		raw := append([]byte{0x00, 0x00, 0x88, 0xb5}, []byte("exp1")...)
		n := parseRule(t, raw, "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0x88b5), uintVal(t, n.Child("Protocol Type")))
		require.Equal(t, "exp1", joinUint8(t, mustChild(t, n, "Payload").Child("Next Protocol Data")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, raw))
		require.Equal(t, uint64(0x88b5), uintVal(t, mustChild(t, eth, "IP", "GRE").Child("Protocol Type")))
		require.Equal(t, "exp1", joinUint8(t, mustChild(t, eth, "IP", "GRE", "Payload").Child("Next Protocol Data")))
	})

	t.Run("gre/eth-bridging", func(t *testing.T) {
		// RFC 1701 / IANA EtherType 0x6558 Transparent Ethernet Bridging.
		// gopacket EthernetTypeTransparentEthernetBridging. Inner Ethernet II + ARP.
		// Wireshark gre.proto / eth.type / arp.opcode. IP proto 47. Do not import ethernet.yaml.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		dst := []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
		src := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		inner := append(append(append(dst, src...), 0x08, 0x06), arp...)
		raw := append([]byte{0x00, 0x00, 0x65, 0x58}, inner...)
		n := parseRule(t, raw, "generic_routing_encapsulation", "GRE")
		require.Equal(t, uint64(0x6558), uintVal(t, n.Child("Protocol Type")))
		ethin := mustChild(t, n, "Payload", "Ethernet")
		require.Equal(t, dst, bytesVal(t, ethin.Child("Destination")))
		require.Equal(t, src, bytesVal(t, ethin.Child("Source")))
		require.Equal(t, uint64(0x0806), uintVal(t, ethin.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, ethin, "ARP").Child("Opcode")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, raw))
		wired := mustChild(t, eth, "IP", "GRE", "Payload", "Ethernet")
		require.Equal(t, dst, bytesVal(t, wired.Child("Destination")))
		require.Equal(t, uint64(0x0806), uintVal(t, wired.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, wired, "ARP").Child("Opcode")))
	})

	t.Run("qinq/s-tag", func(t *testing.T) {
		// IEEE 802.1ad S-TAG: PCP=5 DEI=1 VID=100, EtherType ARP.
		// Wireshark ieee8021ad.priority / ieee8021ad.dei / ieee8021ad.id.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		qinq := parseRule(t, append([]byte{0xb0, 0x64, 0x08, 0x06}, arp...), "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(5), uintVal(t, qinq.Child("PCP")))
		require.Equal(t, uint64(1), uintVal(t, qinq.Child("DEI")))
		require.Equal(t, uint64(100), vlanVID(t, qinq))
		require.Equal(t, uint64(0x0806), uintVal(t, qinq.Child("Type")))
		frame := make([]byte, 14+4+len(arp))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x88a8)
		copy(frame[14:], append([]byte{0xb0, 0x64, 0x08, 0x06}, arp...))
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(5), uintVal(t, mustChild(t, eth, "QinQ").Child("PCP")))
		require.Equal(t, uint64(100), vlanVID(t, mustChild(t, eth, "QinQ")))
	})

	t.Run("qinq/c-tag", func(t *testing.T) {
		// IEEE 802.1ad: outer S-VID 100, inner C-TAG PCP=3 VID=200 + ARP.
		// Wireshark ieee8021ad.id / ieee8021ad.cvid / vlan.id.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append([]byte{0x00, 0x64, 0x81, 0x00, 0x60, 0xc8, 0x08, 0x06}, arp...)
		q := parseRule(t, raw, "ieee_802_1ad", "QinQ")
		require.Equal(t, uint64(100), vlanVID(t, q))
		ctag := mustChild(t, q, "CTag")
		require.Equal(t, uint64(3), uintVal(t, ctag.Child("PCP")))
		require.Equal(t, uint64(200), vlanVID(t, ctag))
		require.Equal(t, uint64(0x0806), uintVal(t, ctag.Child("Type")))
		frame := make([]byte, 14+len(raw))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x88a8)
		copy(frame[14:], raw)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(200), vlanVID(t, mustChild(t, eth, "QinQ", "CTag")))
	})

	t.Run("loopback/reply", func(t *testing.T) {
		// Wireshark LOOP (packet-loop.c): little-endian skipCount then functions from offset 2.
		// Function 1 Reply + Receipt Number. wiki.wireshark.org/Configuration_Test_Protocol
		raw := mustHex(t, "000001000100")
		n := parseRule(t, raw, "loopback", "Loopback")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Skip Count")))
		fn := n.Child("Functions").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, fn.Child("Function")))
		require.Equal(t, uint64(1), uintVal(t, fn.Child("Reply").Child("Receipt Number")))
		frame := make([]byte, 14+len(raw))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x9000)
		copy(frame[14:], raw)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "Loopback").Child("Functions").Children()[0].Child("Reply").Child("Receipt Number")))
	})

	t.Run("loopback/forward", func(t *testing.T) {
		// Wireshark LOOP parses ALL functions from offset 2; skipCount=8 is recorded, not a tcpdump skip.
		// Function 2 Forward Data MAC aa:00:04:00:1d:04 then Function 1 Reply receipt=1.
		raw := mustHex(t, "08000200aa0004001d0401000100")
		n := parseRule(t, raw, "loopback", "Loopback")
		require.Equal(t, uint64(8), uintVal(t, n.Child("Skip Count")))
		fns := n.Child("Functions").Children()
		require.GreaterOrEqual(t, len(fns), 2)
		require.Equal(t, uint64(2), uintVal(t, fns[0].Child("Function")))
		require.Equal(t, []byte{0xaa, 0x00, 0x04, 0x00, 0x1d, 0x04}, bytesVal(t, fns[0].Child("Forward").Child("Forwarding Address")))
		require.Equal(t, uint64(1), uintVal(t, fns[1].Child("Function")))
		require.Equal(t, uint64(1), uintVal(t, fns[1].Child("Reply").Child("Receipt Number")))
		frame := make([]byte, 14+len(raw))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x9000)
		copy(frame[14:], raw)
		eth := parseEthernet(t, frame)
		wired := mustChild(t, eth, "Loopback")
		require.Equal(t, uint64(8), uintVal(t, wired.Child("Skip Count")))
		require.Equal(t, []byte{0xaa, 0x00, 0x04, 0x00, 0x1d, 0x04}, bytesVal(t, wired.Child("Functions").Children()[0].Child("Forward").Child("Forwarding Address")))
	})

	t.Run("stp/config", func(t *testing.T) {
		// IEEE 802.1D Config BPDU; Wireshark stp.root.prio / stp.root.ext / stp.root.hw.
		// Root Priority 32768 (nibble 8), system ID ext 1, MAC aa:bb:cc:00:01:00.
		raw := stpConfigBPDU()
		n := parseRule(t, raw, "stp", "STP")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Protocol ID")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("BPDU Type")))
		cfg := mustChild(t, n, "Config")
		require.Equal(t, uint64(8), uintVal(t, cfg.Child("Root Priority")))
		require.Equal(t, uint64(32768), uintVal(t, cfg.Child("Root Priority"))<<12)
		require.Equal(t, uint64(1), uintVal(t, cfg.Child("Root Ext High"))<<8|uintVal(t, cfg.Child("Root Ext Low")))
		require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0x00, 0x01, 0x00}, bytesVal(t, cfg.Child("Root MAC")))
		require.Equal(t, uint64(0), uintVal(t, cfg.Child("Root Path Cost")))
		llc := append([]byte{0x42, 0x42, 0x03}, raw...)
		eth := parseEthernet(t, ethernet8023([]byte{0x01, 0x80, 0xc2, 0x00, 0x00, 0x00}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, llc))
		require.Equal(t, uint64(32768), uintVal(t, mustChild(t, eth, "LLC", "STP", "Config").Child("Root Priority"))<<12)
		require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0x00, 0x01, 0x00}, bytesVal(t, mustChild(t, eth, "LLC", "STP", "Config").Child("Root MAC")))
	})

	t.Run("stp/tcn", func(t *testing.T) {
		// IEEE 802.1D Topology Change Notification: Protocol ID, Version, Type=0x80. No Config body.
		raw := []byte{0x00, 0x00, 0x00, 0x80}
		n := parseRule(t, raw, "stp", "STP")
		require.Equal(t, uint64(0x80), uintVal(t, n.Child("BPDU Type")))
		require.Nil(t, n.Child("Config"))
		llc := append([]byte{0x42, 0x42, 0x03}, raw...)
		eth := parseEthernet(t, ethernet8023([]byte{0x01, 0x80, 0xc2, 0x00, 0x00, 0x00}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, llc))
		require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, eth, "LLC", "STP").Child("BPDU Type")))
		require.Nil(t, mustChild(t, eth, "LLC", "STP").Child("Config"))
	})

	t.Run("socks4/connect", func(t *testing.T) {
		// SOCKS4 CONNECT (Ying-Da Lee): VN=4 CD=1 DSTPORT=80 DSTIP=10.0.0.1 USERID "u".
		// Wireshark socks.command / socks.dst / socks.dstport / socks.userid. TCP/1080.
		raw := []byte{0x04, 0x01, 0x00, 0x50, 10, 0, 0, 1, 'u', 0}
		n := parseRule(t, raw, "socks4", "SOCKS4")
		require.Equal(t, uint64(4), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Command")))
		require.Equal(t, uint64(80), uintVal(t, n.Child("Port")))
		require.Equal(t, []byte{10, 0, 0, 1}, bytesVal(t, n.Child("IP")))
		require.Equal(t, "u", strVal(t, n.Child("UserID")))
		require.Nil(t, n.Child("Domain"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1080, 1080, raw))
		require.Equal(t, "u", strVal(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("UserID")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("Command")))
	})

	t.Run("socks4/4a", func(t *testing.T) {
		// SOCKS4a: DSTIP 0.0.0.1 then USERID and DOMAIN. Wireshark socks.v4a_dns_name.
		raw := append([]byte{0x04, 0x01, 0x00, 0x50, 0, 0, 0, 1, 'u', 0}, append([]byte("example.com"), 0)...)
		n := parseRule(t, raw, "socks4", "SOCKS4")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Command")))
		require.Equal(t, []byte{0, 0, 0, 1}, bytesVal(t, n.Child("IP")))
		require.Equal(t, "u", strVal(t, n.Child("UserID")))
		require.Equal(t, "example.com", strVal(t, n.Child("Domain")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1080, 1080, raw))
		require.Equal(t, "example.com", strVal(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("Domain")))
	})

	t.Run("socks4/reply", func(t *testing.T) {
		// SOCKS4 reply VN=0 CD=90 granted (Wireshark socks.results). No USERID.
		raw := []byte{0x00, 0x5a, 0x00, 0x50, 10, 0, 0, 1}
		n := parseRule(t, raw, "socks4", "SOCKS4")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(90), uintVal(t, n.Child("Command")))
		require.Equal(t, uint64(80), uintVal(t, n.Child("Port")))
		require.Equal(t, []byte{10, 0, 0, 1}, bytesVal(t, n.Child("IP")))
		require.Nil(t, n.Child("UserID"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1080, 1080, raw))
		require.Equal(t, uint64(90), uintVal(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("Command")))
		require.Nil(t, mustChild(t, eth, "IP", "TCP", "SOCKS4").Child("UserID"))
	})

	t.Run("telnet/do-echo", func(t *testing.T) {
		// RFC 854 IAC DO ECHO (option 1). Wireshark telnet.cmd / telnet.option. TCP/23.
		raw := []byte{0xff, 0xfd, 0x01}
		n := parseRule(t, raw, "telnet", "Telnet")
		require.Equal(t, uint64(0xff), uintVal(t, mustChild(t, n, "IAC").Child("IACByte")))
		require.Equal(t, uint64(0xfd), uintVal(t, mustChild(t, n, "IAC").Child("Command")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "IAC").Child("Option")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 23, 23, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "Telnet", "IAC").Child("Option")))
	})

	t.Run("telnet/ttype", func(t *testing.T) {
		// RFC 1091 TERMINAL-TYPE IS "xterm": IAC SB 24 IS xterm IAC SE.
		// Wireshark telnet.ttype. TCP/23.
		raw := append([]byte{0xff, 0xfa, 0x18, 0x00}, append([]byte("xterm"), 0xff, 0xf0)...)
		n := parseRule(t, raw, "telnet", "Telnet")
		iac := mustChild(t, n, "IAC")
		require.Equal(t, uint64(250), uintVal(t, iac.Child("Command")))
		require.Equal(t, uint64(24), uintVal(t, iac.Child("SB Option")))
		require.Equal(t, uint64(0), uintVal(t, iac.Child("SB Command")))
		require.Equal(t, "xterm", joinUint8(t, iac.Child("Terminal Type")))
		require.Equal(t, uint64(240), uintVal(t, iac.Child("SE")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 23, 23, raw))
		require.Equal(t, "xterm", joinUint8(t, mustChild(t, eth, "IP", "TCP", "Telnet", "IAC").Child("Terminal Type")))
	})

	t.Run("telnet/naws", func(t *testing.T) {
		// RFC 1073 NAWS 80x24: IAC SB 31 00 50 00 18 IAC SE. Wireshark telnet.naws.
		raw := []byte{0xff, 0xfa, 0x1f, 0x00, 0x50, 0x00, 0x18, 0xff, 0xf0}
		n := parseRule(t, raw, "telnet", "Telnet")
		iac := mustChild(t, n, "IAC")
		require.Equal(t, uint64(31), uintVal(t, iac.Child("SB Option")))
		require.Equal(t, uint64(80), uintVal(t, iac.Child("Width")))
		require.Equal(t, uint64(24), uintVal(t, iac.Child("Height")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 23, 23, raw))
		require.Equal(t, uint64(80), uintVal(t, mustChild(t, eth, "IP", "TCP", "Telnet", "IAC").Child("Width")))
	})

	t.Run("vnc/version", func(t *testing.T) {
		// RFC 6143 §7.1.1 ProtocolVersion "RFB 003.008\n". Wireshark vnc.version. TCP/5900.
		raw := []byte("RFB 003.008\n")
		n := parseRule(t, raw, "vnc", "VNC")
		require.Equal(t, "RFB ", strVal(t, n.Child("Magic")))
		require.Equal(t, "003", strVal(t, n.Child("Major")))
		require.Equal(t, "008", strVal(t, n.Child("Minor")))
		require.Nil(t, n.Child("Number of Security Types"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5900, 5900, raw))
		require.Equal(t, "008", strVal(t, mustChild(t, eth, "IP", "TCP", "VNC").Child("Minor")))
	})

	t.Run("vnc/security", func(t *testing.T) {
		// RFC 6143 §7.1.2: number-of-security-types=2, None (1) and VNC Authentication (2).
		// Wireshark vnc.num_security_types / vnc.security_type. TCP/5900.
		raw := append([]byte("RFB 003.008\n"), 0x02, 0x01, 0x02)
		n := parseRule(t, raw, "vnc", "VNC")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Number of Security Types")))
		types := n.Child("Security Types").Children()
		require.Equal(t, 2, len(types))
		require.Equal(t, uint64(1), uintVal(t, types[0]))
		require.Equal(t, uint64(2), uintVal(t, types[1]))
		eth := parseEthernet(t, ipv4TCPFrame(t, 5900, 5900, raw))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "TCP", "VNC").Child("Number of Security Types")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "VNC").Child("Security Types").Children()[0]))
	})

	t.Run("vnc/server-init", func(t *testing.T) {
		// RFC 6143 §7.3.2 ServerInit: 800x600, 32bpp, true-colour, name "x11".
		// Wireshark vnc.width / vnc.height / vnc.desktop_name.
		raw := vncServerInit()
		n := parseRule(t, raw, "vnc", "ServerInit")
		require.Equal(t, uint64(800), uintVal(t, n.Child("Width")))
		require.Equal(t, uint64(600), uintVal(t, n.Child("Height")))
		require.Equal(t, uint64(32), uintVal(t, n.Child("Bits Per Pixel")))
		require.Equal(t, uint64(24), uintVal(t, n.Child("Depth")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("True Colour")))
		require.Equal(t, uint64(16), uintVal(t, n.Child("Red Shift")))
		require.Equal(t, "x11", strVal(t, n.Child("Name")))
	})

	t.Run("syslog/bsd", func(t *testing.T) {
		// RFC 3164 §4.1: <PRI>TIMESTAMP HOSTNAME MSG. Wireshark syslog.facility / syslog.hostname. UDP/514.
		raw := []byte("<13>Sep  4 12:00:00 host sshd: ok\n")
		n := parseRule(t, raw, "syslog", "Syslog")
		require.Equal(t, "13", strVal(t, n.Child("PRI")))
		require.Equal(t, "Sep  4 12:00:00", strVal(t, n.Child("Timestamp")))
		require.Equal(t, "host", strVal(t, n.Child("Hostname")))
		require.Equal(t, "sshd: ok", strVal(t, n.Child("Message")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 12345, 514, raw))
		require.Equal(t, "13", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("PRI")))
		require.Equal(t, "host", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("Hostname")))
	})

	t.Run("syslog/rfc5424", func(t *testing.T) {
		// RFC 5424 §6.5 example 1. Wireshark syslog.version / syslog.msgid / syslog.procid. UDP/514.
		raw := []byte("<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - 'su root' failed for lonvick on /dev/pts/8\n")
		n := parseRule(t, raw, "syslog", "Syslog")
		require.Equal(t, "34", strVal(t, n.Child("PRI")))
		require.Equal(t, "1", strVal(t, n.Child("Version")))
		require.Equal(t, "2003-10-11T22:14:15.003Z", strVal(t, n.Child("Timestamp")))
		require.Equal(t, "mymachine.example.com", strVal(t, n.Child("Hostname")))
		require.Equal(t, "su", strVal(t, n.Child("App Name")))
		require.Equal(t, "-", strVal(t, n.Child("ProcID")))
		require.Equal(t, "ID47", strVal(t, n.Child("MsgID")))
		require.Equal(t, "-", strVal(t, n.Child("Structured Data")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 12345, 514, raw))
		require.Equal(t, "ID47", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("MsgID")))
		require.Equal(t, "1", strVal(t, mustChild(t, eth, "IP", "UDP", "Syslog").Child("Version")))
	})

	t.Run("sdp/origin", func(t *testing.T) {
		// RFC 4566 §5 o=<username> <sess-id> <sess-version> <nettype> <addrtype> <unicast-address>.
		// Wireshark sdp.owner.username / sdp.owner.address. UDP/5006.
		raw := []byte("v=0\r\no=alice 2890844526 2890844526 IN IP4 pc33.atlanta.example.com\r\n")
		n := parseRule(t, raw, "sdp", "SDP")
		require.Equal(t, uint64('v'), uintVal(t, n.Child("Type")))
		require.Equal(t, "0", strVal(t, n.Child("Value")))
		require.Equal(t, "alice", strVal(t, n.Child("Username")))
		require.Equal(t, "2890844526", strVal(t, n.Child("Sess ID")))
		require.Equal(t, "IN", strVal(t, n.Child("Net Type")))
		require.Equal(t, "IP4", strVal(t, n.Child("Addr Type")))
		require.Equal(t, "pc33.atlanta.example.com", strVal(t, n.Child("Address")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5006, 5006, raw))
		require.Equal(t, "alice", strVal(t, mustChild(t, eth, "IP", "UDP", "SDP").Child("Username")))
		require.Equal(t, "pc33.atlanta.example.com", strVal(t, mustChild(t, eth, "IP", "UDP", "SDP").Child("Address")))
	})

	t.Run("sdp/media", func(t *testing.T) {
		// RFC 4566 §5 session + m=audio 49170 RTP/AVP 0. Wireshark sdp.session_name / sdp.media.port.
		raw := []byte("v=0\r\no=jdoe 2890844526 2890842807 IN IP4 10.47.16.5\r\n" +
			"s=SDP Seminar\r\nc=IN IP4 224.2.17.12/127\r\nt=2873397496 2873404696\r\n" +
			"m=audio 49170 RTP/AVP 0\r\n")
		n := parseRule(t, raw, "sdp", "SDP")
		require.Equal(t, "jdoe", strVal(t, n.Child("Username")))
		require.Equal(t, "10.47.16.5", strVal(t, n.Child("Address")))
		lines := n.Child("Lines").Children()
		require.GreaterOrEqual(t, len(lines), 4)
		require.Equal(t, "SDP Seminar", strVal(t, lines[0].Child("Session Name")))
		require.Equal(t, "IN", strVal(t, lines[1].Child("Net Type")))
		require.Equal(t, "224.2.17.12/127", strVal(t, lines[1].Child("Connection Address")))
		require.Equal(t, "2873397496", strVal(t, lines[2].Child("Start")))
		require.Equal(t, "audio", strVal(t, lines[3].Child("Media")))
		require.Equal(t, "49170", strVal(t, lines[3].Child("Port")))
		require.Equal(t, "RTP/AVP", strVal(t, lines[3].Child("Proto")))
		require.Equal(t, "0", strVal(t, lines[3].Child("Format")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 5006, 5006, raw))
		wired := mustChild(t, eth, "IP", "UDP", "SDP").Child("Lines").Children()
		require.Equal(t, "49170", strVal(t, wired[3].Child("Port")))
		require.Equal(t, "SDP Seminar", strVal(t, wired[0].Child("Session Name")))
	})

	t.Run("bittorrent/handshake", func(t *testing.T) {
		// BEP 3 handshake: pstrlen 19, "BitTorrent protocol", info_hash, peer_id.
		// Wireshark bittorrent.protocol / bittorrent.info_hash / bittorrent.peer_id. TCP/6881.
		raw := btHandshake()
		n := parseRule(t, raw, "bittorrent", "BitTorrent")
		require.Equal(t, uint64(19), uintVal(t, n.Child("Pstrlen")))
		require.Equal(t, "BitTorrent protocol", string(bytesVal(t, n.Child("Pstr"))))
		require.Equal(t, []byte("0123456789abcdefghij"), bytesVal(t, n.Child("Info Hash")))
		require.Equal(t, []byte("-UT2210-abcdefghijkl"), bytesVal(t, n.Child("Peer ID")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 6881, 6881, raw))
		require.Equal(t, []byte("-UT2210-abcdefghijkl"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "BitTorrent").Child("Peer ID")))
		require.Equal(t, []byte("0123456789abcdefghij"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "BitTorrent").Child("Info Hash")))
	})

	t.Run("bittorrent/have", func(t *testing.T) {
		// BEP 3 have: length 5, id 4, piece index 7. Wireshark bittorrent.msg.id / bittorrent.piece.index.
		raw := append(btHandshake(), 0, 0, 0, 5, 4, 0, 0, 0, 7)
		n := parseRule(t, raw, "bittorrent", "BitTorrent")
		msg := n.Child("Messages").Children()[0]
		require.Equal(t, uint64(5), uintVal(t, msg.Child("Length")))
		require.Equal(t, uint64(4), uintVal(t, msg.Child("Message ID")))
		require.Equal(t, uint64(7), uintVal(t, msg.Child("Piece Index")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 6881, 6881, raw))
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, eth, "IP", "TCP", "BitTorrent").Child("Messages").Children()[0].Child("Piece Index")))
	})

	t.Run("bittorrent/request", func(t *testing.T) {
		// BEP 3 request: length 13, id 6, index 1, begin 0, length 16384.
		req := make([]byte, 17)
		binary.BigEndian.PutUint32(req[0:], 13)
		req[4] = 6
		binary.BigEndian.PutUint32(req[5:], 1)
		binary.BigEndian.PutUint32(req[9:], 0)
		binary.BigEndian.PutUint32(req[13:], 16384)
		raw := append(btHandshake(), req...)
		n := parseRule(t, raw, "bittorrent", "BitTorrent")
		msg := n.Child("Messages").Children()[0]
		require.Equal(t, uint64(6), uintVal(t, msg.Child("Message ID")))
		require.Equal(t, uint64(1), uintVal(t, msg.Child("Index")))
		require.Equal(t, uint64(0), uintVal(t, msg.Child("Begin")))
		require.Equal(t, uint64(16384), uintVal(t, msg.Child("Block Length")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 6881, 6881, raw))
		require.Equal(t, uint64(16384), uintVal(t, mustChild(t, eth, "IP", "TCP", "BitTorrent").Child("Messages").Children()[0].Child("Block Length")))
	})

	t.Run("php_ser/int", func(t *testing.T) {
		// PHP unserialize integer (php.net/unserialize). Multi-digit i:12; not string,1.
		raw := []byte("i:12;")
		n := parseRule(t, raw, "php_ser", "PHPSer")
		require.Equal(t, uint64('i'), uintVal(t, n.Child("Kind")))
		require.Equal(t, "12", strVal(t, n.Child("Int")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 40001, 7777, raw))
		require.Equal(t, "12", strVal(t, mustChild(t, eth, "IP", "TCP", "PHPSer").Child("Int")))
	})

	t.Run("php_ser/string", func(t *testing.T) {
		// PHP serialize s:5:"hello"; length-prefixed quoted string.
		raw := []byte("s:5:\"hello\";")
		n := parseRule(t, raw, "php_ser", "PHPSer")
		require.Equal(t, uint64('s'), uintVal(t, n.Child("Kind")))
		require.Equal(t, "5", strVal(t, n.Child("Strlen")))
		require.Equal(t, "hello", strVal(t, n.Child("String")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 40001, 7777, raw))
		require.Equal(t, "hello", strVal(t, mustChild(t, eth, "IP", "TCP", "PHPSer").Child("String")))
	})

	t.Run("php_ser/array", func(t *testing.T) {
		// PHP serialize a:1:{s:3:"foo";i:42;} one key/value pair.
		raw := []byte("a:1:{s:3:\"foo\";i:42;}")
		n := parseRule(t, raw, "php_ser", "PHPSer")
		require.Equal(t, uint64('a'), uintVal(t, n.Child("Kind")))
		require.Equal(t, "1", strVal(t, n.Child("Count")))
		mem := n.Child("Members").Children()
		require.GreaterOrEqual(t, len(mem), 2)
		require.Equal(t, "foo", strVal(t, mem[0].Child("String")))
		require.Equal(t, "42", strVal(t, mem[1].Child("Int")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 40001, 7777, raw))
		require.Equal(t, "foo", strVal(t, mustChild(t, eth, "IP", "TCP", "PHPSer").Child("Members").Children()[0].Child("String")))
	})

	t.Run("rsync/greeting", func(t *testing.T) {
		// rsync daemon greeting @RSYNCD: 31.0. Wireshark rsync.version. TCP/873.
		raw := []byte("@RSYNCD: 31.0\n")
		n := parseRule(t, raw, "rsync", "Rsync")
		require.Equal(t, "@RSYNCD:", strVal(t, n.Child("Magic")))
		require.Equal(t, "31", strVal(t, n.Child("Major")))
		require.Equal(t, "0", strVal(t, n.Child("Minor")))
		require.Nil(t, n.Child("Module"))
		eth := parseEthernet(t, ipv4TCPFrame(t, 873, 873, raw))
		require.Equal(t, "31", strVal(t, mustChild(t, eth, "IP", "TCP", "Rsync").Child("Major")))
	})

	t.Run("rsync/module", func(t *testing.T) {
		// Client selects module after greeting. Wireshark rsync.module. TCP/873.
		raw := []byte("@RSYNCD: 31.0\npublic\n")
		n := parseRule(t, raw, "rsync", "Rsync")
		require.Equal(t, "public", strVal(t, n.Child("Module")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 873, 873, raw))
		require.Equal(t, "public", strVal(t, mustChild(t, eth, "IP", "TCP", "Rsync").Child("Module")))
	})

	t.Run("rsync/ok", func(t *testing.T) {
		// Server @RSYNCD: OK after module. Wireshark rsync.status. TCP/873.
		raw := []byte("@RSYNCD: 31.0\n@RSYNCD: OK\n")
		n := parseRule(t, raw, "rsync", "Rsync")
		require.Equal(t, "RSYNCD", strVal(t, n.Child("Status Kind")))
		require.Equal(t, "OK", strVal(t, n.Child("Status")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 873, 873, raw))
		require.Equal(t, "OK", strVal(t, mustChild(t, eth, "IP", "TCP", "Rsync").Child("Status")))
	})

	t.Run("linux_sll/ipv4", func(t *testing.T) {
		// LINKTYPE_LINUX_SLL (tcpdump): HOST, ARPHRD_ETHER, MAC, Ethertype IPv4.
		// Wireshark sll.pkttype / sll.src.eth / sll.etype. Payload IPv4 Version 4.
		mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ip := []byte{
			0x45, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00,
			0x40, 0x01, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2,
			0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01,
		}
		raw := append(linuxSLL(0, 1, 6, mac, 0x0800), ip...)
		n := parseRule(t, raw, "linux_sll", "LinuxSLL")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("ARPHRD")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("Address Length")))
		require.Equal(t, mac, bytesVal(t, n.Child("Source MAC")))
		require.Equal(t, uint64(0x0800), uintVal(t, n.Child("Protocol")))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, n, "IP").Child("Version")))
		require.Equal(t, []byte{10, 0, 0, 1}, bytesVal(t, mustChild(t, n, "IP").Child("Source")))
	})

	t.Run("linux_sll/arp", func(t *testing.T) {
		// SLL outgoing + ARP request. Wireshark sll.pkttype=4 / arp.opcode.
		mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append(linuxSLL(4, 1, 6, mac, 0x0806), arp...)
		n := parseRule(t, raw, "linux_sll", "LinuxSLL")
		require.Equal(t, uint64(4), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(0x0806), uintVal(t, n.Child("Protocol")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "ARP").Child("Opcode")))
		require.Equal(t, mac, bytesVal(t, mustChild(t, n, "ARP").Child("Sender MAC address")))
	})

	t.Run("linux_sll/next-type", func(t *testing.T) {
		// LINKTYPE_LINUX_SLL (tcpdump cooked) + Linux if_ether.h ETH_P_802_EX1 0x88B5.
		// Wireshark sll.etype: unrecognized protocol leftover is Next Protocol Data, not raw.
		mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		raw := append(linuxSLL(0, 1, 6, mac, 0x88b5), []byte("exp1")...)
		n := parseRule(t, raw, "linux_sll", "LinuxSLL")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Packet Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("ARPHRD")))
		require.Equal(t, uint64(0x88b5), uintVal(t, n.Child("Protocol")))
		require.Equal(t, "exp1", joinUint8(t, n.Child("Next Protocol Data")))
	})

	t.Run("ieee_802_11/qos", func(t *testing.T) {
		// IEEE 802.11-2016 §9.2.4.5.4 QoS Data (type 2 subtype 8), TID 6 (AC_VO).
		// Wireshark wlan.fc.type_subtype / wlan.qos.tid. 3-address + MSDU leftover.
		addr1 := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		addr2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		addr3 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
		raw := make([]byte, 0, 30)
		raw = append(raw, 0x88, 0x00, 0x00, 0x00)
		raw = append(raw, addr1...)
		raw = append(raw, addr2...)
		raw = append(raw, addr3...)
		raw = append(raw, 0x00, 0x00, 0x06, 0x00)
		raw = append(raw, []byte("qos1")...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0088), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, addr1, bytesVal(t, n.Child("Addr1")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("QoS Control"))&0xf)
		require.Nil(t, n.Child("Addr4"))
		require.Equal(t, "qos1", joinUint8(t, n.Child("Next Protocol Data")))
	})

	t.Run("ieee_802_11/addr4", func(t *testing.T) {
		// IEEE 802.11-2016 §9.2.4.3.4: ToDS=1 FromDS=1 four-address (WDS) Data.
		// Wireshark wlan.fc.tods / wlan.fc.fromds / wlan.addr. Addr4 after Sequence Control.
		addr1 := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		addr2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		addr3 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
		addr4 := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60}
		raw := make([]byte, 0, 34)
		raw = append(raw, 0x08, 0x03, 0x00, 0x00)
		raw = append(raw, addr1...)
		raw = append(raw, addr2...)
		raw = append(raw, addr3...)
		raw = append(raw, 0x00, 0x00)
		raw = append(raw, addr4...)
		raw = append(raw, []byte("wds1")...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0308), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, addr4, bytesVal(t, n.Child("Addr4")))
		require.Nil(t, n.Child("QoS Control"))
		require.Equal(t, "wds1", joinUint8(t, n.Child("Next Protocol Data")))
	})

	t.Run("ieee_802_11/ack", func(t *testing.T) {
		// IEEE 802.11-2016 Table 9-1 Control ACK (type 1 subtype 13): RA only, no Addr2/Seq.
		// Wireshark wlan.fc.type_subtype / wlan.ra. 10-byte MAC header.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		raw := append([]byte{0xd4, 0x00, 0x00, 0x00}, ra...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x00d4), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, ra, bytesVal(t, n.Child("Addr1")))
		require.Nil(t, n.Child("Addr2"))
		require.Nil(t, n.Child("Seq"))
		require.Nil(t, n.Child("Next Protocol Data"))
	})

	t.Run("ieee_802_11/htc", func(t *testing.T) {
		// IEEE 802.11-2016 §9.2.4.1.10: QoS Data Order=1 includes HT Control after QoS Control.
		// Wireshark wlan.fc.order / wlan.htc.
		addr1 := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		addr2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		addr3 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
		raw := make([]byte, 0, 34)
		raw = append(raw, 0x88, 0x80, 0x00, 0x00)
		raw = append(raw, addr1...)
		raw = append(raw, addr2...)
		raw = append(raw, addr3...)
		raw = append(raw, 0x00, 0x00, 0x06, 0x00, 0x78, 0x56, 0x34, 0x12)
		raw = append(raw, []byte("htc1")...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x8088), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("QoS Control"))&0xf)
		require.Equal(t, uint64(0x12345678), uintVal(t, n.Child("HT Control")))
		require.Equal(t, "htc1", joinUint8(t, n.Child("Next Protocol Data")))
	})

	t.Run("ieee_802_11/block-ack", func(t *testing.T) {
		// IEEE 802.11-2016 §9.3.1.8 Compressed BlockAck (type 1 subtype 9).
		// BA Control TID 6 + Compressed Bitmap, Starting Seq 16, 8-octet bitmap.
		// Wireshark wlan.ba.control / wlan.ba.ssc / wlan.ba.bm.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ta := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		bm := []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		raw := make([]byte, 0, 28)
		raw = append(raw, 0x94, 0x00, 0x00, 0x00)
		raw = append(raw, ra...)
		raw = append(raw, ta...)
		raw = append(raw, 0x04, 0x60, 0x00, 0x01)
		raw = append(raw, bm...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0094), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, ra, bytesVal(t, n.Child("Addr1")))
		require.Equal(t, ta, bytesVal(t, n.Child("Addr2")))
		require.Equal(t, uint64(0x6004), uintVal(t, n.Child("BA Control")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("BA Control"))>>12)
		require.Equal(t, uint64(0x0100), uintVal(t, n.Child("Starting Sequence Control")))
		require.Equal(t, bm, bytesVal(t, n.Child("BA Bitmap")))
		require.Nil(t, n.Child("Seq"))
		require.Nil(t, n.Child("Next Protocol Data"))
	})

	t.Run("ieee_802_11/bar", func(t *testing.T) {
		// IEEE 802.11-2016 §9.3.1.7 Compressed BlockAckReq (type 1 subtype 8).
		// BAR Control TID 6 + Compressed Bitmap, Starting Seq 16; no BA Bitmap.
		// Wireshark wlan.bar.control / wlan.bar.ssc.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ta := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		raw := make([]byte, 0, 20)
		raw = append(raw, 0x84, 0x00, 0x00, 0x00)
		raw = append(raw, ra...)
		raw = append(raw, ta...)
		raw = append(raw, 0x04, 0x60, 0x00, 0x01)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0084), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, ra, bytesVal(t, n.Child("Addr1")))
		require.Equal(t, ta, bytesVal(t, n.Child("Addr2")))
		require.Equal(t, uint64(0x6004), uintVal(t, n.Child("BAR Control")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("BAR Control"))>>12)
		require.Equal(t, uint64(0x0100), uintVal(t, n.Child("Starting Sequence Control")))
		require.Nil(t, n.Child("BA Bitmap"))
		require.Nil(t, n.Child("Seq"))
		require.Nil(t, n.Child("Next Protocol Data"))
	})

	t.Run("ieee_802_11/mtid-ba", func(t *testing.T) {
		// IEEE 802.11-2016 §9.3.1.8 Figure 9-28/9-31 Multi-TID Compressed BlockAck.
		// BA Control Multi-TID+Compressed, TID_INFO=1 (two TIDs). Wireshark wlan.ba.control.multitid.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ta := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		bm0 := []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		bm6 := []byte{0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		raw := make([]byte, 0, 42)
		raw = append(raw, 0x94, 0x00, 0x00, 0x00)
		raw = append(raw, ra...)
		raw = append(raw, ta...)
		raw = append(raw, 0x06, 0x10)
		raw = append(raw, 0x00, 0x00, 0x00, 0x01)
		raw = append(raw, bm0...)
		raw = append(raw, 0x00, 0x60, 0x00, 0x02)
		raw = append(raw, bm6...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0094), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, uint64(0x1006), uintVal(t, n.Child("BA Control")))
		require.Equal(t, uint64(1), (uintVal(t, n.Child("BA Control"))>>1)&1)
		tids := n.Child("BA TID Info").Children()
		require.Equal(t, 2, len(tids))
		require.Equal(t, uint64(0), uintVal(t, tids[0].Child("Per TID Info"))>>12)
		require.Equal(t, uint64(0x0100), uintVal(t, tids[0].Child("Starting Sequence Control")))
		require.Equal(t, bm0, bytesVal(t, tids[0].Child("BA Bitmap")))
		require.Equal(t, uint64(6), uintVal(t, tids[1].Child("Per TID Info"))>>12)
		require.Equal(t, uint64(0x0200), uintVal(t, tids[1].Child("Starting Sequence Control")))
		require.Equal(t, bm6, bytesVal(t, tids[1].Child("BA Bitmap")))
		require.Nil(t, n.Child("Starting Sequence Control"))
		require.Nil(t, n.Child("BA Bitmap"))
	})

	t.Run("ieee_802_11/mtid-bar", func(t *testing.T) {
		// IEEE 802.11-2016 §9.3.1.7 Multi-TID Compressed BlockAckReq: Per TID Info + SSC, no bitmap.
		// Wireshark wlan.bar.control.multitid.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ta := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		raw := make([]byte, 0, 26)
		raw = append(raw, 0x84, 0x00, 0x00, 0x00)
		raw = append(raw, ra...)
		raw = append(raw, ta...)
		raw = append(raw, 0x06, 0x10)
		raw = append(raw, 0x00, 0x00, 0x00, 0x01, 0x00, 0x60, 0x00, 0x02)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0084), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, uint64(0x1006), uintVal(t, n.Child("BAR Control")))
		tids := n.Child("BAR TID Info").Children()
		require.Equal(t, 2, len(tids))
		require.Equal(t, uint64(0), uintVal(t, tids[0].Child("Per TID Info"))>>12)
		require.Equal(t, uint64(0x0100), uintVal(t, tids[0].Child("Starting Sequence Control")))
		require.Equal(t, uint64(6), uintVal(t, tids[1].Child("Per TID Info"))>>12)
		require.Equal(t, uint64(0x0200), uintVal(t, tids[1].Child("Starting Sequence Control")))
		require.Nil(t, n.Child("Starting Sequence Control"))
		require.Nil(t, n.Child("BA Bitmap"))
	})

	t.Run("ieee_802_11/basic-ba", func(t *testing.T) {
		// IEEE 802.11-2016 §9.3.1.8 / Table 9-20 Basic BlockAck (Multi-TID=0, Compressed=0).
		// BA Information: Starting Sequence Control + 128-octet Block Ack Bitmap.
		// Wireshark wlan.ba.control / wlan.ba.ssc / wlan.ba.bm. TID 6, SSN 16.
		ra := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		ta := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
		bm := make([]byte, 128)
		bm[0] = 0xff
		raw := make([]byte, 0, 148)
		raw = append(raw, 0x94, 0x00, 0x00, 0x00)
		raw = append(raw, ra...)
		raw = append(raw, ta...)
		raw = append(raw, 0x00, 0x60, 0x00, 0x01)
		raw = append(raw, bm...)
		n := parseRule(t, raw, "ieee_802_11", "Dot11")
		require.Equal(t, uint64(0x0094), uintVal(t, n.Child("Frame Control")))
		require.Equal(t, ra, bytesVal(t, n.Child("Addr1")))
		require.Equal(t, ta, bytesVal(t, n.Child("Addr2")))
		require.Equal(t, uint64(0x6000), uintVal(t, n.Child("BA Control")))
		require.Equal(t, uint64(6), uintVal(t, n.Child("BA Control"))>>12)
		require.Equal(t, uint64(0), (uintVal(t, n.Child("BA Control"))>>2)&1)
		require.Equal(t, uint64(0x0100), uintVal(t, n.Child("Starting Sequence Control")))
		require.Equal(t, bm, bytesVal(t, n.Child("Block Ack Bitmap")))
		require.Equal(t, 128, len(bytesVal(t, n.Child("Block Ack Bitmap"))))
		require.Nil(t, n.Child("BA Bitmap"))
		require.Nil(t, n.Child("Seq"))
		require.Nil(t, n.Child("Next Protocol Data"))
	})

	t.Run("igmp/v1-report", func(t *testing.T) {
		// RFC 1112 / gopacket igmp_test.go: Type 0x12 membership report, group 224.0.1.60.
		raw := []byte{0x12, 0x00, 0x0c, 0xc3, 224, 0, 1, 60}
		n := parseRule(t, raw, "igmp", "IGMP")
		require.Equal(t, uint64(0x12), uintVal(t, n.Child("Type")))
		require.Equal(t, []byte{224, 0, 1, 60}, bytesVal(t, n.Child("Group Address")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 2, raw))
		require.Equal(t, uint64(0x12), uintVal(t, mustChild(t, eth, "IP", "IGMP").Child("Type")))
		require.Equal(t, []byte{224, 0, 1, 60}, bytesVal(t, mustChild(t, eth, "IP", "IGMP").Child("Group Address")))
	})

	t.Run("igmp/v3-query", func(t *testing.T) {
		// RFC 3376 §4.1 Membership Query with one source. Wireshark igmp.type / igmp.maddr.
		raw := []byte{0x11, 0x64, 0x00, 0x00, 239, 1, 1, 1, 0x02, 0x7d, 0x00, 0x01, 192, 0, 2, 1}
		n := parseRule(t, raw, "igmp", "IGMP")
		require.Equal(t, uint64(0x11), uintVal(t, n.Child("Type")))
		require.Equal(t, []byte{239, 1, 1, 1}, bytesVal(t, n.Child("Group Address")))
		require.Equal(t, uint64(2), uintVal(t, n.Child("SQRV")))
		require.Equal(t, uint64(125), uintVal(t, n.Child("QQIC")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Number of Sources")))
		require.Equal(t, []byte{192, 0, 2, 1}, bytesVal(t, n.Child("Sources").Children()[0]))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 2, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "IGMP").Child("Number of Sources")))
		require.Equal(t, []byte{192, 0, 2, 1}, bytesVal(t, mustChild(t, eth, "IP", "IGMP").Child("Sources").Children()[0]))
	})

	t.Run("igmp/v3-report", func(t *testing.T) {
		// RFC 3376 §4.2 Membership Report: one MODE_IS_EXCLUDE record for 239.1.1.1.
		raw := []byte{0x22, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00, 239, 1, 1, 1}
		n := parseRule(t, raw, "igmp", "IGMP")
		require.Equal(t, uint64(0x22), uintVal(t, n.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Number of Group Records")))
		rec := n.Child("Records").Children()[0]
		require.Equal(t, uint64(2), uintVal(t, rec.Child("Record Type")))
		require.Equal(t, []byte{239, 1, 1, 1}, bytesVal(t, rec.Child("Multicast Address")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 2, raw))
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, eth, "IP", "IGMP").Child("Records").Children()[0].Child("Record Type")))
	})

	t.Run("mpls/bottom", func(t *testing.T) {
		// RFC 3032 one-label stack, Bottom=1, then IPv4. Wireshark mpls.label / mpls.bottom.
		raw := []byte{0x00, 0x01, 0x31, 0xfe, 0x45, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2, 0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
		n := parseRule(t, raw, "mpls", "MPLS")
		lab := n.Child("Labels").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, lab.Child("Bottom")))
		require.Equal(t, uint64(0xfe), uintVal(t, lab.Child("TTL")))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, n, "IP").Child("Version")))
		frame := make([]byte, 14+len(raw))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x8847)
		copy(frame[14:], raw)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "MPLS").Child("Labels").Children()[0].Child("Bottom")))
	})

	t.Run("mpls/stack", func(t *testing.T) {
		// RFC 3032 two-label stack: first Bottom=0, second Bottom=1. Wireshark mpls.label.
		raw := []byte{0x00, 0x01, 0x10, 0xfe, 0x00, 0x01, 0x31, 0xfe, 0x45, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2, 0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
		n := parseRule(t, raw, "mpls", "MPLS")
		labs := n.Child("Labels").Children()
		require.GreaterOrEqual(t, len(labs), 2)
		require.Equal(t, uint64(0), uintVal(t, labs[0].Child("Bottom")))
		require.Equal(t, uint64(1), uintVal(t, labs[1].Child("Bottom")))
		frame := make([]byte, 14+len(raw))
		copy(frame[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
		copy(frame[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		binary.BigEndian.PutUint16(frame[12:14], 0x8847)
		copy(frame[14:], raw)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "MPLS").Child("Labels").Children()[1].Child("Bottom")))
	})

	t.Run("vxlan/vni", func(t *testing.T) {
		// RFC 7348 §5: I-flag set, VNI 255, inner Ethernet IPv4. Wireshark vxlan.vni / vxlan.flags.i.
		innerMAC := []byte{0x00, 0x30, 0x88, 0x01, 0x00, 0x02, 0x00, 0x16, 0x3e, 0x37, 0xf6, 0x04}
		ip := []byte{0x45, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2, 0x08, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
		raw := append(append([]byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x00}, innerMAC...), append([]byte{0x08, 0x00}, ip...)...)
		n := parseRule(t, raw, "vxlan", "VXLAN")
		require.Equal(t, uint64(1), uintVal(t, n.Child("I")))
		require.Equal(t, uint64(255), uintVal(t, n.Child("VNI")))
		inner := mustChild(t, n, "Inner")
		require.Equal(t, uint64(0x0800), uintVal(t, inner.Child("Type")))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, inner, "IP").Child("Version")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 4789, 4789, raw))
		require.Equal(t, uint64(255), uintVal(t, mustChild(t, eth, "IP", "UDP", "VXLAN").Child("VNI")))
		require.Equal(t, uint64(4), uintVal(t, mustChild(t, eth, "IP", "UDP", "VXLAN", "Inner", "IP").Child("Version")))
	})

	t.Run("vxlan/arp", func(t *testing.T) {
		// RFC 7348 inner Ethernet ARP request Opcode 1. Wireshark vxlan / arp.opcode. UDP/4789.
		innerMAC := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append(append([]byte{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00}, innerMAC...), append([]byte{0x08, 0x06}, arp...)...)
		n := parseRule(t, raw, "vxlan", "VXLAN")
		require.Equal(t, uint64(1), uintVal(t, n.Child("I")))
		require.Equal(t, uint64(10), uintVal(t, n.Child("VNI")))
		require.Equal(t, uint64(0x0806), uintVal(t, mustChild(t, n, "Inner").Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, n, "Inner", "ARP").Child("Opcode")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 4789, 4789, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "VXLAN", "Inner", "ARP").Child("Opcode")))
	})

	t.Run("rmi/ping", func(t *testing.T) {
		// Java RMI spec §10.2 Ping = 0x52 after StreamProtocol 0x4b. Wireshark rmi.outputstream.message. TCP/1099.
		raw := []byte{'J', 'R', 'M', 'I', 0x00, 0x02, 0x4b, 0x52}
		n := parseRule(t, raw, "rmi", "RMI")
		require.Equal(t, "JRMI", string(bytesVal(t, n.Child("Magic"))))
		require.Equal(t, uint64(0x4b), uintVal(t, n.Child("Protocol")))
		require.Equal(t, uint64(0x52), uintVal(t, mustChild(t, n, "Message").Child("Type")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1099, 1099, raw))
		require.Equal(t, uint64(0x52), uintVal(t, mustChild(t, eth, "IP", "TCP", "RMI", "Message").Child("Type")))
	})

	t.Run("rmi/call", func(t *testing.T) {
		// Java RMI spec §10.2 Call = 0x50 then Java serialization STREAM_MAGIC 0xaced. Wireshark rmi.ser.magic. TCP/1099.
		raw := []byte{'J', 'R', 'M', 'I', 0x00, 0x02, 0x4b, 0x50, 0xac, 0xed, 0x00, 0x05}
		n := parseRule(t, raw, "rmi", "RMI")
		msg := mustChild(t, n, "Message")
		require.Equal(t, uint64(0x50), uintVal(t, msg.Child("Type")))
		require.Equal(t, uint64(0xaced), uintVal(t, msg.Child("Ser Magic")))
		require.Equal(t, uint64(5), uintVal(t, msg.Child("Ser Version")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 1099, 1099, raw))
		require.Equal(t, uint64(0xaced), uintVal(t, mustChild(t, eth, "IP", "TCP", "RMI", "Message").Child("Ser Magic")))
	})

	t.Run("llc/stp", func(t *testing.T) {
		// IEEE 802.2 UI (Control 0x03) DSAP/SSAP 0x42 + 802.1D TCN. Wireshark llc.dsap / stp.type.
		raw := []byte{0x42, 0x42, 0x03, 0x00, 0x00, 0x00, 0x80}
		n := parseRule(t, raw, "llc", "LLC")
		require.Equal(t, uint64(0x42), uintVal(t, n.Child("DSAP")))
		require.Equal(t, uint64(0x03), uintVal(t, n.Child("Control")))
		require.Nil(t, n.Child("Control Extended"))
		require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, n, "STP").Child("BPDU Type")))
		eth := parseEthernet(t, ethernet8023([]byte{0x01, 0x80, 0xc2, 0x00, 0x00, 0x00}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, raw))
		require.Equal(t, uint64(0x80), uintVal(t, mustChild(t, eth, "LLC", "STP").Child("BPDU Type")))
	})

	t.Run("llc/snap", func(t *testing.T) {
		// RFC 1042 SNAP UI OUI 00:00:00 PID 0x0806 ARP request. Wireshark snap.oui / arp.opcode.
		arp := []byte{0x00, 0x01, 0x08, 0x00, 0x06, 0x04, 0x00, 0x01, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 10, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 0, 0, 2}
		raw := append([]byte{0xaa, 0xaa, 0x03, 0x00, 0x00, 0x00, 0x08, 0x06}, arp...)
		n := parseRule(t, raw, "llc", "LLC")
		require.Equal(t, uint64(0xaa), uintVal(t, n.Child("DSAP")))
		snap := mustChild(t, n, "SNAP")
		require.Equal(t, []byte{0x00, 0x00, 0x00}, bytesVal(t, snap.Child("OUI")))
		require.Equal(t, uint64(0x0806), uintVal(t, snap.Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, snap, "ARP").Child("Opcode")))
		eth := parseEthernet(t, ethernet8023([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, raw))
		require.Equal(t, uint64(0x0806), uintVal(t, mustChild(t, eth, "LLC", "SNAP").Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "LLC", "SNAP", "ARP").Child("Opcode")))
	})

	t.Run("llc/xid", func(t *testing.T) {
		// IEEE 802.2 §5.4.1.1.1 XID command Control 0xAF, Format 0x81, Type-1 LLC, window 127.
		// Wireshark llc.control / llc.xid.
		raw := []byte{0x00, 0x00, 0xaf, 0x81, 0x01, 0x7f}
		n := parseRule(t, raw, "llc", "LLC")
		require.Equal(t, uint64(0xaf), uintVal(t, n.Child("Control")))
		require.Equal(t, uint64(0x81), uintVal(t, n.Child("XID Format")))
		require.Equal(t, uint64(0x01), uintVal(t, n.Child("XID Types")))
		require.Equal(t, uint64(0x7f), uintVal(t, n.Child("XID Window")))
		eth := parseEthernet(t, ethernet8023([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, raw))
		require.Equal(t, uint64(0x81), uintVal(t, mustChild(t, eth, "LLC").Child("XID Format")))
	})

	t.Run("openvpn/hard-reset", func(t *testing.T) {
		// OpenVPN P_CONTROL_HARD_RESET_CLIENT_V2: opcode 7, key 0, session, ack 0, packet-id 1.
		// Wireshark openvpn.opcode / openvpn.sessionid. UDP/1194.
		raw := mustHex(t, "3801020304050607080000000001")
		n := parseRule(t, raw, "openvpn", "OpenVPN")
		require.Equal(t, uint64(7), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Key ID")))
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, n.Child("Session ID")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Ack Count")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Packet ID")))
		require.Nil(t, n.Child("Peer ID"))
		eth := parseEthernet(t, ipv4UDPBytes(t, 1194, 1194, raw))
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Packet ID")))
	})

	t.Run("openvpn/data-v2", func(t *testing.T) {
		// OpenVPN P_DATA_V2 (opcode 9): peer-id then encrypted payload. Wireshark openvpn.peerid. UDP/1194.
		raw := []byte{0x48, 0x00, 0x00, 0x01, 0xde, 0xad, 0xbe, 0xef}
		n := parseRule(t, raw, "openvpn", "OpenVPN")
		require.Equal(t, uint64(9), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(0), uintVal(t, n.Child("Key ID")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Peer ID")))
		require.Nil(t, n.Child("Session ID"))
		eth := parseEthernet(t, ipv4UDPBytes(t, 1194, 1194, raw))
		require.Equal(t, uint64(9), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Peer ID")))
		require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Payload")))
	})

	t.Run("openvpn/ack", func(t *testing.T) {
		// OpenVPN P_ACK_V1 (opcode 5): one ACK packet-id + remote session. Wireshark openvpn.mpid_arraylength.
		raw := mustHex(t, "28010203040506070801000000010807060504030201")
		n := parseRule(t, raw, "openvpn", "OpenVPN")
		require.Equal(t, uint64(5), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Ack Count")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Acks").Children()[0]))
		require.Equal(t, []byte{8, 7, 6, 5, 4, 3, 2, 1}, bytesVal(t, n.Child("Remote Session ID")))
		require.Nil(t, n.Child("Packet ID"))
		eth := parseEthernet(t, ipv4UDPBytes(t, 1194, 1194, raw))
		require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "UDP", "OpenVPN").Child("Ack Count")))
	})

	t.Run("sctp/data", func(t *testing.T) {
		// Wireshark SampleCaptures/sctp.cap frame 1: DATA B+E, PPI 7 MEGACO/2. RFC 4960 §3.3.1.
		raw := mustHex(t, "40000b8000016f0a6db018820003005b280243450000a0bd000000074d454741434f2f32203c6d672d74723e3a31363338340a5265706c79203d203137343039317b0a436f6e74657874203d203235357b0a4d6f64696679203d204d55582f3235350a7d0a7d0a67")
		n := parseRule(t, raw, "sctp", "SCTP")
		require.Equal(t, uint64(16384), uintVal(t, n.Child("Source Port")))
		ch := n.Child("Chunks").Children()[0]
		require.Equal(t, uint64(0), uintVal(t, ch.Child("Type")))
		require.Equal(t, uint64(0x03), uintVal(t, ch.Child("Flags")))
		require.Equal(t, uint64(0x28024345), uintVal(t, ch.Child("TSN")))
		require.Equal(t, uint64(7), uintVal(t, ch.Child("PPI")))
		require.True(t, strings.HasPrefix(strVal(t, ch.Child("User Data")), "MEGACO/2"))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 132, raw))
		require.Equal(t, uint64(7), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Chunks").Children()[0].Child("PPI")))
		require.True(t, strings.HasPrefix(strVal(t, mustChild(t, eth, "IP", "SCTP").Child("Chunks").Children()[0].Child("User Data")), "MEGACO/2"))
	})

	t.Run("sctp/sack", func(t *testing.T) {
		// Wireshark SampleCaptures/sctp.cap frame 2: SACK Cumulative TSN 0x28024345 a_rwnd 0x2000. RFC 4960 §3.3.4.
		raw := mustHex(t, "0b804000214415232bf2024e03000010280243450000200000000000")
		n := parseRule(t, raw, "sctp", "SCTP")
		ch := n.Child("Chunks").Children()[0]
		require.Equal(t, uint64(3), uintVal(t, ch.Child("Type")))
		require.Equal(t, uint64(0x28024345), uintVal(t, ch.Child("Cumulative TSN")))
		require.Equal(t, uint64(0x2000), uintVal(t, ch.Child("Rwnd")))
		require.Equal(t, uint64(0), uintVal(t, ch.Child("Gap Ack Blocks")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 132, raw))
		require.Equal(t, uint64(0x28024345), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Chunks").Children()[0].Child("Cumulative TSN")))
	})

	t.Run("sctp/init", func(t *testing.T) {
		// Wireshark SampleCaptures/sctp-test.cap frame 1: INIT Initiate Tag 0x43232544 OS/IS 17. RFC 4960 §3.3.2.
		raw := mustHex(t, "00070007000000003761a74601000020432325440000ffff001100115cfe379fc0000004000c000600050000")
		n := parseRule(t, raw, "sctp", "SCTP")
		ch := n.Child("Chunks").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, ch.Child("Type")))
		require.Equal(t, uint64(0x43232544), uintVal(t, ch.Child("Initiate Tag")))
		require.Equal(t, uint64(17), uintVal(t, ch.Child("Outbound Streams")))
		require.Equal(t, uint64(17), uintVal(t, ch.Child("Inbound Streams")))
		require.Equal(t, uint64(0x5cfe379f), uintVal(t, ch.Child("Initial TSN")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 132, raw))
		require.Equal(t, uint64(0x43232544), uintVal(t, mustChild(t, eth, "IP", "SCTP").Child("Chunks").Children()[0].Child("Initiate Tag")))
	})

	t.Run("eigrp/hello", func(t *testing.T) {
		// Wireshark SampleCaptures/EIGRP_Neighbors.cap Hello: AS 100, Parameters K1=1 K3=1 Hold 15s. RFC 7868 §6.2 / eigrp.par.k1.
		raw := mustHex(t, "0205ee68000000000000000000000000000000640001000c010001000000000f000400080c040102")
		n := parseRule(t, raw, "eigrp", "EIGRP")
		require.Equal(t, uint64(2), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(5), uintVal(t, n.Child("Opcode")))
		require.Equal(t, uint64(100), uintVal(t, n.Child("AS Number")))
		tlvs := n.Child("TLVs").Children()
		require.GreaterOrEqual(t, len(tlvs), 2)
		require.Equal(t, uint64(1), uintVal(t, tlvs[0].Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, tlvs[0].Child("K1")))
		require.Equal(t, uint64(0), uintVal(t, tlvs[0].Child("K2")))
		require.Equal(t, uint64(1), uintVal(t, tlvs[0].Child("K3")))
		require.Equal(t, uint64(15), uintVal(t, tlvs[0].Child("Hold Time")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 88, raw))
		require.Equal(t, uint64(100), uintVal(t, mustChild(t, eth, "IP", "EIGRP").Child("AS Number")))
		require.Equal(t, uint64(15), uintVal(t, mustChild(t, eth, "IP", "EIGRP").Child("TLVs").Children()[0].Child("Hold Time")))
	})

	t.Run("eigrp/swver", func(t *testing.T) {
		// Same EIGRP_Neighbors.cap Hello Software Version TLV: IOS 12.4, EIGRP 1.2. Wireshark eigrp.release_version.
		raw := mustHex(t, "0205ee68000000000000000000000000000000640001000c010001000000000f000400080c040102")
		n := parseRule(t, raw, "eigrp", "EIGRP")
		sw := n.Child("TLVs").Children()[1]
		require.Equal(t, uint64(4), uintVal(t, sw.Child("Type")))
		require.Equal(t, uint64(12), uintVal(t, sw.Child("IOS Major")))
		require.Equal(t, uint64(4), uintVal(t, sw.Child("IOS Minor")))
		require.Equal(t, uint64(1), uintVal(t, sw.Child("EIGRP Major")))
		require.Equal(t, uint64(2), uintVal(t, sw.Child("EIGRP Minor")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 88, raw))
		require.Equal(t, uint64(12), uintVal(t, mustChild(t, eth, "IP", "EIGRP").Child("TLVs").Children()[1].Child("IOS Major")))
	})

	t.Run("cdp/device-id", func(t *testing.T) {
		// Wireshark SampleCaptures/cdp.pcap frame 1: CDP v1 Device ID "R1" over SNAP OUI 00:00:0c PID 0x2000.
		n := parseRule(t, wiresharkCDP(t), "cdp", "CDP")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Version")))
		require.Equal(t, uint64(180), uintVal(t, n.Child("TTL")))
		tlvs := n.Child("TLVs").Children()
		require.GreaterOrEqual(t, len(tlvs), 6)
		require.Equal(t, uint64(1), uintVal(t, tlvs[0].Child("Type")))
		require.Equal(t, "R1", strVal(t, tlvs[0].Child("Device ID")))
		snap := append([]byte{0xaa, 0xaa, 0x03, 0x00, 0x00, 0x0c, 0x20, 0x00}, wiresharkCDP(t)...)
		eth := parseEthernet(t, ethernet8023([]byte{0x01, 0x00, 0x0c, 0xcc, 0xcc, 0xcc}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, snap))
		require.Equal(t, "R1", strVal(t, mustChild(t, eth, "LLC", "SNAP", "CDP").Child("TLVs").Children()[0].Child("Device ID")))
	})

	t.Run("cdp/port", func(t *testing.T) {
		// Same cdp.pcap: Port ID "Ethernet0", Capabilities Router, Address 192.168.10.1, Platform "cisco 1601".
		n := parseRule(t, wiresharkCDP(t), "cdp", "CDP")
		tlvs := n.Child("TLVs").Children()
		require.Equal(t, uint64(2), uintVal(t, tlvs[1].Child("Type")))
		require.Equal(t, uint64(1), uintVal(t, tlvs[1].Child("Number of Addresses")))
		require.Equal(t, uint64(0xcc), uintVal(t, tlvs[1].Child("Protocol")))
		require.Equal(t, []byte{192, 168, 10, 1}, bytesVal(t, tlvs[1].Child("Address")))
		require.Equal(t, "Ethernet0", strVal(t, tlvs[2].Child("Port ID")))
		require.Equal(t, uint64(1), uintVal(t, tlvs[3].Child("Capabilities")))
		require.Equal(t, "cisco 1601", strVal(t, tlvs[5].Child("Platform")))
		snap := append([]byte{0xaa, 0xaa, 0x03, 0x00, 0x00, 0x0c, 0x20, 0x00}, wiresharkCDP(t)...)
		eth := parseEthernet(t, ethernet8023([]byte{0x01, 0x00, 0x0c, 0xcc, 0xcc, 0xcc}, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, snap))
		cdp := mustChild(t, eth, "LLC", "SNAP", "CDP")
		require.Equal(t, "Ethernet0", strVal(t, cdp.Child("TLVs").Children()[2].Child("Port ID")))
		require.Equal(t, "cisco 1601", strVal(t, cdp.Child("TLVs").Children()[5].Child("Platform")))
	})

	t.Run("lldp/chassis", func(t *testing.T) {
		// gopacket layers/lldp_test.go Siemens SCALANCE: Chassis ID subtype 7 local "switch1". IEEE 802.1AB.
		n := parseRule(t, gopacketSiemensLLDP(t), "lldp", "LLDP")
		ch := n.Child("TLVs").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, ch.Child("TypeLen"))>>9)
		require.Equal(t, uint64(7), uintVal(t, ch.Child("Chassis ID Subtype")))
		require.Equal(t, "switch1", strVal(t, ch.Child("Chassis ID")))
		eth := parseEthernet(t, gopacketSiemensLLDPFrame(t))
		require.Equal(t, "switch1", strVal(t, mustChild(t, eth, "LLDP").Child("TLVs").Children()[0].Child("Chassis ID")))
	})

	t.Run("lldp/port", func(t *testing.T) {
		// Same Siemens frame: Port ID "port-001", TTL 20s, System Name "Switch1". EtherType 0x88cc.
		n := parseRule(t, gopacketSiemensLLDP(t), "lldp", "LLDP")
		tlvs := n.Child("TLVs").Children()
		require.Equal(t, uint64(2), uintVal(t, tlvs[1].Child("TypeLen"))>>9)
		require.Equal(t, uint64(7), uintVal(t, tlvs[1].Child("Port ID Subtype")))
		require.Equal(t, "port-001", strVal(t, tlvs[1].Child("Port ID")))
		require.Equal(t, uint64(3), uintVal(t, tlvs[2].Child("TypeLen"))>>9)
		require.Equal(t, uint64(20), uintVal(t, tlvs[2].Child("TTL")))
		require.Equal(t, "Switch1", strVal(t, tlvs[4].Child("System Name")))
		eth := parseEthernet(t, gopacketSiemensLLDPFrame(t))
		lldp := mustChild(t, eth, "LLDP")
		require.Equal(t, "port-001", strVal(t, lldp.Child("TLVs").Children()[1].Child("Port ID")))
		require.Equal(t, uint64(20), uintVal(t, lldp.Child("TLVs").Children()[2].Child("TTL")))
	})

	t.Run("pppoe/padi", func(t *testing.T) {
		// RFC 2516 §5.1 PADI: exactly one Service-Name tag. Empty TAG_LENGTH is "any service" (Appendix B);
		// named payload uses Service-Name "isp" so the string is observable. EtherType 0x8863.
		anySvc := mustHex(t, "11090000000401010000")
		n := parseRule(t, anySvc, "pppoe", "PPPoE")
		require.Equal(t, uint64(0x09), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(0x0101), uintVal(t, n.Child("Payload").Child("Tags").Children()[0].Child("Type")))
		require.Nil(t, n.Child("Payload").Child("Tags").Children()[0].Child("Service-Name"))
		padi := rfcPPPoEPADI(t)
		n = parseRule(t, padi, "pppoe", "PPPoE")
		require.Equal(t, "isp", strVal(t, n.Child("Payload").Child("Tags").Children()[0].Child("Service-Name")))
		eth := parseEthernet(t, pppoeDiscoveryFrame(t, padi))
		require.Equal(t, "isp", strVal(t, mustChild(t, eth, "PPPoEDiscovery").Child("Payload").Child("Tags").Children()[0].Child("Service-Name")))
	})

	t.Run("pppoe/pado", func(t *testing.T) {
		// RFC 2516 §5.2 PADO: AC-Name + Service-Name identical to PADI, plus Host-Uniq echo.
		pado := rfcPPPoEPADO(t)
		n := parseRule(t, pado, "pppoe", "PPPoE")
		require.Equal(t, uint64(0x07), uintVal(t, n.Child("Code")))
		tags := n.Child("Payload").Child("Tags").Children()
		require.Equal(t, "isp", strVal(t, tags[0].Child("Service-Name")))
		require.Equal(t, "BRAS1", strVal(t, tags[1].Child("AC-Name")))
		require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, bytesVal(t, tags[2].Child("Host-Uniq")))
		eth := parseEthernet(t, pppoeDiscoveryFrame(t, pado))
		p := mustChild(t, eth, "PPPoEDiscovery")
		require.Equal(t, "BRAS1", strVal(t, p.Child("Payload").Child("Tags").Children()[1].Child("AC-Name")))
		require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, bytesVal(t, p.Child("Payload").Child("Tags").Children()[2].Child("Host-Uniq")))
	})

	t.Run("pppoe/session", func(t *testing.T) {
		// RFC 2516 §4 session (code 0) carries PPP. Protocol 0x002d as in RFC 2661 data example. EtherType 0x8864.
		sess := mustHex(t, "110000010004ff03002d")
		n := parseRule(t, sess, "pppoe", "PPPoE")
		require.Equal(t, uint64(0), uintVal(t, n.Child("Code")))
		require.Equal(t, uint64(1), uintVal(t, n.Child("Session ID")))
		require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, n, "Payload", "PPP").Child("Protocol")))
		eth := parseEthernet(t, pppoeSessionFrame(t, sess))
		require.Equal(t, uint64(0x002d), uintVal(t, mustChild(t, eth, "PPPoESession", "Payload", "PPP").Child("Protocol")))
	})

	t.Run("radius/user-name", func(t *testing.T) {
		// Wireshark radtest.pcap / gopacket layers/radius_test.go Access-Request User-Name "Admin". RFC 2865 §5.1.
		raw := radiusAccessRequestFrame[42:]
		n := parseRule(t, raw, "application-layer.radius", "RADIUS")
		require.Equal(t, uint64(1), uintVal(t, n.Child("Code")))
		require.Equal(t, "Admin", strVal(t, n.Child("Attributes").Children()[0].Child("User-Name")))
		eth := parseEthernet(t, radiusAccessRequestFrame)
		require.Equal(t, "Admin", strVal(t, mustChild(t, eth, "IP", "UDP", "RADIUS").Child("Attributes").Children()[0].Child("User-Name")))
	})

	t.Run("radius/nas-ip", func(t *testing.T) {
		// Same radtest.pcap: NAS-IP-Address 127.0.0.1, NAS-Port 0. RFC 2865 §5.4 / §5.5. UDP/1812.
		raw := radiusAccessRequestFrame[42:]
		n := parseRule(t, raw, "application-layer.radius", "RADIUS")
		attrs := n.Child("Attributes").Children()
		require.Equal(t, []byte{0x7f, 0x00, 0x01, 0x01}, bytesVal(t, attrs[2].Child("NAS-IP-Address")))
		require.Equal(t, uint64(0), uintVal(t, attrs[3].Child("NAS-Port")))
		eth := parseEthernet(t, radiusAccessRequestFrame)
		r := mustChild(t, eth, "IP", "UDP", "RADIUS")
		require.Equal(t, []byte{0x7f, 0x00, 0x01, 0x01}, bytesVal(t, r.Child("Attributes").Children()[2].Child("NAS-IP-Address")))
	})

	t.Run("ntlm/challenge", func(t *testing.T) {
		// [MS-NLMP] 2.2.1.2 CHALLENGE_MESSAGE TargetName UTF-16LE "DOMAIN" at BufferOffset 48.
		raw := ntlmsspChallengeTarget("DOMAIN")
		n := parseRule(t, raw, "application-layer.ntlm", "NTLMSSP")
		require.Equal(t, uint64(2), uintVal(t, n.Child("MessageType")))
		require.Equal(t, utf16LE("DOMAIN"), bytesVal(t, n.Child("Target Name")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, smb2SessionNTLM(raw)))
		require.Equal(t, utf16LE("DOMAIN"), bytesVal(t, mustChild(t, eth, "IP", "TCP", "SMB2", "Session Setup Request", "NTLMSSP").Child("Target Name")))
	})

	t.Run("ntlm/authenticate", func(t *testing.T) {
		// [MS-NLMP] 2.2.1.3 AUTHENTICATE_MESSAGE DomainName "CORP" + UserName "Admin" UTF-16LE. TCP/445.
		raw := ntlmsspAuthUser("CORP", "Admin")
		n := parseRule(t, raw, "application-layer.ntlm", "NTLMSSP")
		require.Equal(t, uint64(3), uintVal(t, n.Child("MessageType")))
		require.Equal(t, utf16LE("CORP"), bytesVal(t, n.Child("Domain Name")))
		require.Equal(t, utf16LE("Admin"), bytesVal(t, n.Child("User Name")))
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 445, smb2SessionNTLM(raw)))
		wired := mustChild(t, eth, "IP", "TCP", "SMB2", "Session Setup Request", "NTLMSSP")
		require.Equal(t, utf16LE("Admin"), bytesVal(t, wired.Child("User Name")))
		require.Equal(t, utf16LE("CORP"), bytesVal(t, wired.Child("Domain Name")))
	})

	t.Run("ber/integer", func(t *testing.T) {
		// X.690 §8.3 INTEGER 5. Same encoding as RFC 1157 version INTEGER.
		n := parseRule(t, []byte{0x02, 0x01, 0x05}, "application-layer.ber", "BER Element")
		require.Equal(t, uint64(2), uintVal(t, mustChild(t, n, "Type").Child("Tag")))
		require.Equal(t, uint64(5), uintVal(t, n.Child("Integer")))
		eth := parseEthernet(t, ipv4UDPBytes(t, 161, 161, mustHex(t, "302602010004067075626c6963a019020101020100020100300e300c06082b060102010101000500")))
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "SNMP", "PDU Body", "Variable Bindings", "Bindings", "OID")))
	})

	t.Run("ber/oid", func(t *testing.T) {
		// X.690 §8.19 / RFC 1213 sysDescr.0 (1.3.6.1.2.1.1.1.0) as OBJECT IDENTIFIER.
		oid := []byte{0x06, 0x08, 0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}
		n := parseRule(t, oid, "application-layer.ber", "BER Element")
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, n, "Type").Child("Tag")))
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, n.Child("OBJECT IDENTIFIER")))
		require.Nil(t, n.Child("Value"))
		eth := parseEthernet(t, ipv4UDPBytes(t, 161, 161, mustHex(t, "302602010004067075626c6963a019020101020100020100300e300c06082b060102010101000500")))
		require.Equal(t, []byte{0x2b, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00}, bytesVal(t, mustChild(t, eth, "IP", "UDP", "SNMP", "PDU Body", "Variable Bindings", "Bindings", "OID")))
	})

	t.Run("tcp/mss", func(t *testing.T) {
		// gopacket layers/tcp_test.go testPacketTCPOptionDecode: SYN options [mss 8192,eol]. RFC 793 §3.1.
		n := parseRule(t, gopacketTCPSYMSS()[34:62], "transmission_control_protocol", "TCP")
		require.Equal(t, uint64(12345), uintVal(t, n.Child("Source Port")))
		opts := n.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 1)
		require.Equal(t, uint64(2), uintVal(t, opts[0].Child("Kind")))
		require.Equal(t, uint64(8192), uintVal(t, opts[0].Child("MSS")))
		eth := parseEthernet(t, gopacketTCPSYMSS())
		require.Equal(t, uint64(8192), uintVal(t, mustChild(t, eth, "IP", "TCP").Child("Options").Children()[0].Child("MSS")))
	})

	t.Run("tcp/timestamp", func(t *testing.T) {
		// RFC 7323 §3 TCP Timestamps option kind 8: TSval 2, TSecr 1 (gopacket TCPOptionKindTimestamps fixture).
		raw := rfcTCPTimestamp()
		n := parseRule(t, raw, "transmission_control_protocol", "TCP")
		opts := n.Child("Options").Children()
		require.GreaterOrEqual(t, len(opts), 1)
		ts := opts[len(opts)-1]
		require.Equal(t, uint64(8), uintVal(t, ts.Child("Kind")))
		require.Equal(t, uint64(2), uintVal(t, ts.Child("TS Val")))
		require.Equal(t, uint64(1), uintVal(t, ts.Child("TS Echo Reply")))
		eth := parseEthernet(t, ipv4ProtoFrame(t, 6, raw))
		wired := mustChild(t, eth, "IP", "TCP").Child("Options").Children()
		require.Equal(t, uint64(2), uintVal(t, wired[len(wired)-1].Child("TS Val")))
	})

	t.Run("ip/next-proto", func(t *testing.T) {
		// RFC 791 Protocol 99 (unassigned): leftover is named Next Protocol Data, not Unknown raw.
		eth := parseEthernet(t, ipv4ProtoFrame(t, 99, []byte("abcd")))
		require.Equal(t, uint64(99), uintVal(t, mustChild(t, eth, "IP").Child("Protocol")))
		require.Equal(t, "abcd", strVal(t, mustChild(t, eth, "IP").Child("Next Protocol Data")))
	})

	t.Run("ipv6/next-proto", func(t *testing.T) {
		// RFC 8200 Next Header 99: named Next Protocol Data instead of Unknown raw.
		eth := parseEthernet(t, ipv6ProtoFrame(t, 99, []byte("efgh")))
		require.Equal(t, uint64(99), uintVal(t, mustChild(t, eth, "IPv6").Child("Next Header")))
		require.Equal(t, "efgh", strVal(t, mustChild(t, eth, "IPv6").Child("Next Protocol Data")))
	})

	t.Run("rtmp/chunk-size", func(t *testing.T) {
		// Adobe RTMP spec: Type 0 chunk, message type 1 Set Chunk Size 128. After C0+C1 (1536).
		raw := append(rtmpC0C1(), mustHex(t, "02000000000004010000000000000080")...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, raw))
		rt := mustChild(t, eth, "IP", "TCP", "RTMP")
		require.Equal(t, uint64(3), uintVal(t, rt.Child("Version")))
		ch := rt.Child("Chunks").Children()[0]
		require.Equal(t, uint64(1), uintVal(t, ch.Child("Message Type")))
		require.Equal(t, uint64(128), uintVal(t, ch.Child("Chunk Size")))
	})

	t.Run("rtmp/connect", func(t *testing.T) {
		// Adobe RTMP AMF0 command: string "connect" (message type 20). TCP/1935.
		raw := append(rtmpC0C1(), mustHex(t, "0300000000000a1400000000020007636f6e6e656374")...)
		eth := parseEthernet(t, ipv4TCPFrame(t, 1935, 1935, raw))
		ch := mustChild(t, eth, "IP", "TCP", "RTMP").Child("Chunks").Children()[0]
		require.Equal(t, uint64(20), uintVal(t, ch.Child("Message Type")))
		require.Equal(t, "connect", strVal(t, mustChild(t, ch, "AMF0").Child("Command Name")))
	})

	t.Run("ethernet/next-type", func(t *testing.T) {
		// IEEE Std 802-2014 Table C-1 Local Experimental EtherType 1 (0x88B5).
		// Unrecognized EtherType leftover is named Next Protocol Data, not Unknown raw.
		frame := append([]byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
			0x88, 0xb5,
		}, []byte("exp1")...)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(0x88b5), uintVal(t, eth.Child("Type")))
		require.Equal(t, "exp1", joinUint8(t, eth.Child("Next Protocol Data")))
	})

	t.Run("ethernet/ptp", func(t *testing.T) {
		// IEEE 1588 PTP EtherType 0x88F7 (Wireshark SampleCaptures/ptp.pcap).
		// No PTP dissector: payload is Next Protocol Data, not Unknown raw.
		frame := append([]byte{
			0x01, 0x1b, 0x19, 0x00, 0x00, 0x00,
			0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
			0x88, 0xf7,
		}, []byte("sync")...)
		eth := parseEthernet(t, frame)
		require.Equal(t, uint64(0x88f7), uintVal(t, eth.Child("Type")))
		require.Equal(t, "sync", joinUint8(t, eth.Child("Next Protocol Data")))
	})

	t.Run("ppp/next-type", func(t *testing.T) {
		// RFC 1661 Protocol field + IANA 0x002d Van Jacobson Compressed TCP/IP (RFC 1144).
		// No VJ dissector: leftover is Next Protocol Data, not Unknown raw. Ethernet+GRE 0x880B.
		ppp := append([]byte{0xff, 0x03, 0x00, 0x2d}, []byte("vj01")...)
		n := parseRule(t, ppp, "ppp", "PPP")
		require.Equal(t, uint64(0x002d), uintVal(t, n.Child("Protocol")))
		require.Equal(t, "vj01", joinUint8(t, n.Child("Next Protocol Data")))
		gre := append([]byte{0x00, 0x00, 0x88, 0x0b}, ppp...)
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		wired := mustChild(t, eth, "IP", "GRE", "Payload", "PPP")
		require.Equal(t, uint64(0x002d), uintVal(t, wired.Child("Protocol")))
		require.Equal(t, "vj01", joinUint8(t, wired.Child("Next Protocol Data")))
	})

	t.Run("ppp/ipv6", func(t *testing.T) {
		// RFC 5072 IPv6 over PPP protocol 0x0057. Wireshark ppp.protocol. Ethernet+GRE 0x880B.
		ip6 := []byte{
			0x60, 0x00, 0x00, 0x00, 0x00, 0x04, 0x63, 0x40,
			0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
			0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
			'v', '6', 'o', 'k',
		}
		ppp := append([]byte{0xff, 0x03, 0x00, 0x57}, ip6...)
		n := parseRule(t, ppp, "ppp", "PPP")
		require.Equal(t, uint64(0x0057), uintVal(t, n.Child("Protocol")))
		require.Equal(t, uint64(6), uintVal(t, mustChild(t, n, "IPv6").Child("Version")))
		require.Equal(t, "v6ok", strVal(t, mustChild(t, n, "IPv6").Child("Next Protocol Data")))
		gre := append([]byte{0x00, 0x00, 0x88, 0x0b}, ppp...)
		eth := parseEthernet(t, ipv4ProtoFrame(t, 0x2f, gre))
		v6 := mustChild(t, eth, "IP", "GRE", "Payload", "PPP", "IPv6")
		require.Equal(t, uint64(6), uintVal(t, v6.Child("Version")))
		require.Equal(t, "v6ok", strVal(t, v6.Child("Next Protocol Data")))
	})
}

func wiresharkCDP(t *testing.T) []byte {
	return mustHex(t, "01b4dff000010006523100020011000000010101cc0004c0a80a010003000d45746865726e6574300004000800000001000500d8436973636f20496e7465726e6574776f726b204f7065726174696e672053797374656d20536f667477617265200a494f532028746d29203136303020536f667477617265202843313630302d4e592d4c292c2056657273696f6e2031312e3228313229502c2052454c4541534520534f4654574152452028666331290a436f707972696768742028632920313938362d3139393820627920636973636f2053797374656d732c20496e632e0a436f6d70696c6564205475652030332d4d61722d39382030363a33332062792064736368776172740006000e636973636f2031363031")
}

func gopacketSiemensLLDPFrame(t *testing.T) []byte {
	t.Helper()
	return []byte{
		0x01, 0x80, 0xc2, 0x00, 0x00, 0x0e, 0x00, 0x1b, 0x1b, 0x02, 0xe6, 0x1f, 0x88, 0xcc, 0x02, 0x08,
		0x07, 0x73, 0x77, 0x69, 0x74, 0x63, 0x68, 0x31, 0x04, 0x09, 0x07, 0x70, 0x6f, 0x72, 0x74, 0x2d,
		0x30, 0x30, 0x31, 0x06, 0x02, 0x00, 0x14, 0x08, 0x2d, 0x53, 0x69, 0x65, 0x6d, 0x65, 0x6e, 0x73,
		0x2c, 0x20, 0x53, 0x49, 0x4d, 0x41, 0x54, 0x49, 0x43, 0x20, 0x4e, 0x45, 0x54, 0x2c, 0x20, 0x45,
		0x74, 0x68, 0x65, 0x72, 0x6e, 0x65, 0x74, 0x20, 0x53, 0x77, 0x69, 0x74, 0x63, 0x68, 0x20, 0x50,
		0x6f, 0x72, 0x74, 0x20, 0x30, 0x31, 0x0a, 0x07, 0x53, 0x77, 0x69, 0x74, 0x63, 0x68, 0x31, 0x0c,
		0x4c, 0x53, 0x69, 0x65, 0x6d, 0x65, 0x6e, 0x73, 0x2c, 0x20, 0x53, 0x49, 0x4d, 0x41, 0x54, 0x49,
		0x43, 0x20, 0x4e, 0x45, 0x54, 0x2c, 0x20, 0x53, 0x43, 0x41, 0x4c, 0x41, 0x4e, 0x43, 0x45, 0x20,
		0x58, 0x32, 0x31, 0x32, 0x2d, 0x32, 0x2c, 0x20, 0x36, 0x47, 0x4b, 0x35, 0x20, 0x32, 0x31, 0x32,
		0x2d, 0x32, 0x42, 0x42, 0x30, 0x30, 0x2d, 0x32, 0x41, 0x41, 0x33, 0x2c, 0x20, 0x48, 0x57, 0x3a,
		0x20, 0x37, 0x2c, 0x20, 0x46, 0x57, 0x3a, 0x20, 0x56, 0x34, 0x2e, 0x30, 0x32, 0x0e, 0x04, 0x00,
		0x80, 0x00, 0x80, 0x10, 0x14, 0x05, 0x01, 0x8d, 0x51, 0x00, 0xbe, 0x02, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x81, 0xc0, 0x6e, 0xfe, 0x08, 0x00, 0x0e, 0xcf, 0x02, 0x00,
		0x00, 0x00, 0x00, 0xfe, 0x0a, 0x00, 0x0e, 0xcf, 0x05, 0x00, 0x1b, 0x1b, 0x02, 0xe6, 0x1e, 0xfe,
		0x09, 0x00, 0x12, 0x0f, 0x01, 0x03, 0x6c, 0x00, 0x00, 0x10, 0x00, 0x00,
	}
}

func gopacketSiemensLLDP(t *testing.T) []byte {
	t.Helper()
	return gopacketSiemensLLDPFrame(t)[14:]
}

func rfcPPPoEPADI(t *testing.T) []byte {
	t.Helper()
	// RFC 2516 §5.1 PADI with Service-Name "isp" (Appendix B uses empty name for "any").
	return mustHex(t, "11090000000701010003697370")
}

func rfcPPPoEPADO(t *testing.T) []byte {
	t.Helper()
	// RFC 2516 §5.2 PADO: Service-Name "isp", AC-Name "BRAS1", Host-Uniq 01020304.
	return mustHex(t, "110700000018010100036973700102000542524153310103000401020304")
}

func pppoeDiscoveryFrame(t *testing.T, pdu []byte) []byte {
	t.Helper()
	eth := make([]byte, 14+len(pdu))
	copy(eth[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(eth[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	binary.BigEndian.PutUint16(eth[12:14], 0x8863)
	copy(eth[14:], pdu)
	return eth
}

func pppoeSessionFrame(t *testing.T, pdu []byte) []byte {
	t.Helper()
	eth := make([]byte, 14+len(pdu))
	copy(eth[0:6], []byte{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee})
	copy(eth[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	binary.BigEndian.PutUint16(eth[12:14], 0x8864)
	copy(eth[14:], pdu)
	return eth
}

func gopacketTCPSYMSS() []byte {
	// gopacket layers/tcp_test.go testPacketTCPOptionDecode: SYN mss 8192,eol + "Test".
	return []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x34, 0x00, 0x00, 0x00, 0x00, 0x80, 0x06, 0xb9, 0x70, 0xc0, 0xa8, 0x00, 0x01, 0xc0, 0xa8,
		0x00, 0x02, 0x30, 0x39, 0xd4, 0x31, 0xde, 0xad, 0xbe, 0xef, 0x00, 0x00, 0x00, 0x00, 0x70, 0x02,
		0x00, 0x00, 0x82, 0x9c, 0x00, 0x00, 0x02, 0x04, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x54, 0x65,
		0x73, 0x74,
	}
}

func rtmpC0C1() []byte {
	b := make([]byte, 1+1536)
	b[0] = 3
	binary.BigEndian.PutUint32(b[1:], 1)
	copy(b[9:], []byte("rand"))
	return b
}

func rfcTCPTimestamp() []byte {
	// RFC 7323 §3 Timestamps: kind 8 length 10, TSval 2, TSecr 1. Header length 8 (NOP NOP + TS).
	b := make([]byte, 32)
	binary.BigEndian.PutUint16(b[0:], 12345)
	binary.BigEndian.PutUint16(b[2:], 80)
	binary.BigEndian.PutUint32(b[4:], 1)
	binary.BigEndian.PutUint32(b[8:], 1)
	b[12] = 0x80
	b[13] = 0x10
	binary.BigEndian.PutUint16(b[14:], 8192)
	copy(b[20:], []byte{0x01, 0x01, 0x08, 0x0a, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01})
	return b
}

func ntlmsspChallengeTarget(domain string) []byte {
	name := utf16LE(domain)
	buf := make([]byte, 48+len(name))
	copy(buf[:8], []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(buf[8:], 2)
	binary.LittleEndian.PutUint16(buf[12:], uint16(len(name)))
	binary.LittleEndian.PutUint16(buf[14:], uint16(len(name)))
	binary.LittleEndian.PutUint32(buf[16:], 48)
	binary.LittleEndian.PutUint32(buf[20:], 0xe2088205)
	copy(buf[24:32], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	binary.LittleEndian.PutUint32(buf[44:], uint32(48+len(name)))
	copy(buf[48:], name)
	return buf
}

func ntlmsspAuthUser(domain, user string) []byte {
	d := utf16LE(domain)
	u := utf16LE(user)
	off := uint32(64)
	buf := make([]byte, 64+len(d)+len(u))
	copy(buf[:8], []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(buf[8:], 3)
	binary.LittleEndian.PutUint32(buf[16:], off)
	binary.LittleEndian.PutUint32(buf[24:], off)
	binary.LittleEndian.PutUint16(buf[28:], uint16(len(d)))
	binary.LittleEndian.PutUint16(buf[30:], uint16(len(d)))
	binary.LittleEndian.PutUint32(buf[32:], off)
	binary.LittleEndian.PutUint16(buf[36:], uint16(len(u)))
	binary.LittleEndian.PutUint16(buf[38:], uint16(len(u)))
	binary.LittleEndian.PutUint32(buf[40:], off+uint32(len(d)))
	binary.LittleEndian.PutUint32(buf[48:], off+uint32(len(d)+len(u)))
	binary.LittleEndian.PutUint32(buf[56:], off+uint32(len(d)+len(u)))
	binary.LittleEndian.PutUint32(buf[60:], 0x00008201)
	copy(buf[64:], d)
	copy(buf[64+len(d):], u)
	return buf
}

func smb2SessionNTLM(ntlm []byte) []byte {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:], 25)
	body[3] = 1
	binary.LittleEndian.PutUint16(body[12:], 88)
	binary.LittleEndian.PutUint16(body[14:], uint16(len(ntlm)))
	raw := append(smb2SyncHeader(1, 0, 2), body...)
	return append(raw, ntlm...)
}

func linuxSLL(pktType, arphrd, halen uint16, mac []byte, proto uint16) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint16(b[0:], pktType)
	binary.BigEndian.PutUint16(b[2:], arphrd)
	binary.BigEndian.PutUint16(b[4:], halen)
	copy(b[6:], mac)
	binary.BigEndian.PutUint16(b[14:], proto)
	return b
}

func zabbixPacket(flags uint8, json []byte, reserved bool) []byte {
	b := append([]byte("ZBXD"), flags)
	lenb := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenb, uint32(len(json)))
	b = append(b, lenb...)
	if reserved {
		b = append(b, 0, 0, 0, 0)
	}
	return append(b, json...)
}

func btHandshake() []byte {
	b := make([]byte, 68)
	b[0] = 19
	copy(b[1:], []byte("BitTorrent protocol"))
	copy(b[28:], []byte("0123456789abcdefghij"))
	copy(b[48:], []byte("-UT2210-abcdefghijkl"))
	return b
}

func vncServerInit() []byte {
	// RFC 6143 §7.3.2 / §7.4 PIXEL_FORMAT (16 bytes) + name-length + name-string.
	b := make([]byte, 2+2+16+4+3)
	binary.BigEndian.PutUint16(b[0:], 800)
	binary.BigEndian.PutUint16(b[2:], 600)
	b[4] = 32
	b[5] = 24
	b[6] = 0
	b[7] = 1
	binary.BigEndian.PutUint16(b[8:], 0x00ff)
	binary.BigEndian.PutUint16(b[10:], 0x00ff)
	binary.BigEndian.PutUint16(b[12:], 0x00ff)
	b[14] = 16
	b[15] = 8
	b[16] = 0
	binary.BigEndian.PutUint32(b[20:], 3)
	copy(b[24:], []byte("x11"))
	return b
}

func stpConfigBPDU() []byte {
	// gopacket layers/stp_test.go 35-octet IEEE 802.1D Config BPDU.
	return []byte{
		0x00, 0x00, 0x00, 0x00, 0x01, 0x80, 0x01, 0xaa, 0xbb, 0xcc,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x01, 0xaa,
		0xbb, 0xcc, 0x00, 0x01, 0x00, 0x80, 0x01, 0x00, 0x00, 0x14,
		0x00, 0x02, 0x00, 0x0f, 0x00,
	}
}

func tnsConnectPacket(cdata []byte) []byte {
	pkt := make([]byte, 8+26+len(cdata))
	binary.BigEndian.PutUint16(pkt[0:], uint16(len(pkt)))
	pkt[4] = 1
	binary.BigEndian.PutUint16(pkt[8:], 0x0134)
	binary.BigEndian.PutUint16(pkt[10:], 0x0134)
	binary.BigEndian.PutUint16(pkt[24:], uint16(len(cdata)))
	binary.BigEndian.PutUint16(pkt[26:], 34)
	copy(pkt[34:], cdata)
	return pkt
}

func smb1TreeConnectAndX(path, service string) []byte {
	payload := append([]byte{0x04}, append([]byte(path), 0)...)
	payload = append(payload, append([]byte{0x04}, append([]byte(service), 0)...)...)
	body := []byte{4, 0xff, 0, 0, 0, 0, 0, 0, 0, byte(len(payload)), 0}
	body = append(body, payload...)
	return append(smb1Header(0x75, 0x18, 0xc807, 2), body...)
}
