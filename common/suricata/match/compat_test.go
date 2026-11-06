package match

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestDotPrefixDNSTransform(t *testing.T) {
	dotPrefixed := compileTestRule(t, `alert dns any any -> any any (dns.query; dotprefix; content:".minergate.com"; endswith; sid:1;)`)
	plain := compileTestRule(t, `alert dns any any -> any any (dns.query; content:".minergate.com"; endswith; sid:2;)`)

	require.True(t, dotPrefixed.MatchPackage(buildDNSQueryPacket(t, "minergate.com")), "apex domain needs the synthetic leading dot")
	require.True(t, dotPrefixed.MatchPackage(buildDNSQueryPacket(t, "sub.minergate.com")))
	require.False(t, dotPrefixed.MatchPackage(buildDNSQueryPacket(t, "notminergate.com")), "dotprefix must preserve the label boundary")
	require.False(t, plain.MatchPackage(buildDNSQueryPacket(t, "minergate.com")), "without dotprefix the apex domain has no leading dot")
}

func TestNegativeContentSemantics(t *testing.T) {
	packet := buildTCPPacket(t, []byte("hello world"))
	allowed := compileTestRule(t, `alert tcp any any -> any any (content:"hello"; content:!"evil"; sid:1;)`)
	blocked := compileTestRule(t, `alert tcp any any -> any any (content:"hello"; content:!"world"; sid:2;)`)
	require.True(t, allowed.MatchPackage(packet), "negative content must match when the literal is absent")
	require.False(t, blocked.MatchPackage(packet), "negative content must reject when the literal is present")

	pcreAllowed := compileTestRule(t, `alert tcp any any -> any any (content:"hello"; pcre:!"/evil/"; sid:3;)`)
	pcreBlocked := compileTestRule(t, `alert tcp any any -> any any (content:"hello"; pcre:!"/world/"; sid:4;)`)
	require.True(t, pcreAllowed.MatchPackage(packet), "negative pcre must match when the regex is absent")
	require.False(t, pcreBlocked.MatchPackage(packet), "negative pcre must reject when the regex is present")

	lookahead := compileTestRule(t, `alert tcp any any -> any any (pcre:"/^(?!evil)/"; sid:5;)`)
	require.True(t, lookahead.MatchPackage(packet), "PCRE lookahead must be accepted by the PCRE2 backend")
	evil := compileTestRule(t, `alert tcp any any -> any any (pcre:"/^(?!evil)/"; sid:6;)`)
	require.False(t, evil.MatchPackage(buildTCPPacket(t, []byte("evil"))))
}

func TestByteTestSemantics(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x00, 0x02}
	absolute := compileTestRule(t, `alert tcp any any -> any any (content:"|00 01 00 02|"; byte_test:2,=,0x01,0; sid:1;)`)
	relative := compileTestRule(t, `alert tcp any any -> any any (content:"|00 01|"; byte_test:2,=,0x0002,0,relative; sid:2;)`)
	require.True(t, absolute.MatchPackage(buildTCPPacket(t, payload)))
	require.True(t, relative.MatchPackage(buildTCPPacket(t, payload)))

	bitmask := compileTestRule(t, `alert tcp any any -> any any (byte_test:1,&,0x80,0; sid:3;)`)
	notBitmask := compileTestRule(t, `alert tcp any any -> any any (byte_test:1,!&,0x80,0; sid:4;)`)
	require.True(t, bitmask.MatchPackage(buildTCPPacket(t, []byte{0x80})))
	require.False(t, notBitmask.MatchPackage(buildTCPPacket(t, []byte{0x80})))

	multiple := compileTestRule(t, `alert tcp any any -> any any (byte_test:1,=,1,0; byte_test:1,=,2,1; content:"|02|"; sid:5;)`)
	require.True(t, multiple.MatchPackage(buildTCPPacket(t, []byte{1, 2, 0})))

	contentLength := compileTestRule(t, `alert http any any -> any any (http.content_len; byte_test:0,=,0,0,string,dec; sid:6;)`)
	require.True(t, contentLength.MatchPackage(buildTCPPacket(t, []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\n\r\n"))))
	require.False(t, contentLength.MatchPackage(buildTCPPacket(t, []byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 10\r\n\r\n"))))
}

func TestByteJumpSemantics(t *testing.T) {
	relative := compileTestRule(t, `alert tcp any any -> any any (content:"|AA BB|"; byte_jump:1,0,relative; content:"X"; sid:10;)`)
	require.True(t, relative.MatchPackage(buildTCPPacket(t, []byte{0xAA, 0xBB, 0x00, 'X'})))

	fromBeginning := compileTestRule(t, `alert tcp any any -> any any (content:"|AA BB|"; byte_jump:1,0,relative,from_beginning; content:"X"; sid:11;)`)
	require.True(t, fromBeginning.MatchPackage(buildTCPPacket(t, []byte{0xAA, 0xBB, 0x03, 'X'})))

	fromBeginningBackward := compileTestRule(t, `alert tcp any any -> any any (content:"|AA 03|"; byte_jump:1,-1,relative,from_beginning; content:"X"; sid:15;)`)
	require.True(t, fromBeginningBackward.MatchPackage(buildTCPPacket(t, []byte{0xAA, 0x03, 0xBB, 'X'})))

	fromEnd := compileTestRule(t, `alert tcp any any -> any any (byte_jump:0,0,from_end,post_offset -1; content:"X"; sid:12;)`)
	require.True(t, fromEnd.MatchPackage(buildTCPPacket(t, []byte{0xAA, 0xBB, 'X'})))

	jumpBeforeContent := compileTestRule(t, `alert tcp any any -> any any (byte_jump:1,0; content:"X"; sid:13;)`)
	require.True(t, jumpBeforeContent.MatchPackage(buildTCPPacket(t, []byte{0x00, 'X'})))

	interleaved := compileTestRule(t, `alert tcp any any -> any any (content:"|AA BB|"; byte_jump:1,0,relative; byte_test:1,=,0x58,0,relative; content:"X"; sid:14;)`)
	require.True(t, interleaved.MatchPackage(buildTCPPacket(t, []byte{0xAA, 0xBB, 0x00, 'X'})))
}

func TestIPProtocolMatcher(t *testing.T) {
	matched := compileTestRule(t, `alert ip [192.0.2.1] any -> any any (content:"ping"; sid:1;)`)
	notMatched := compileTestRule(t, `alert ip [192.0.2.9] any -> any any (content:"ping"; sid:2;)`)
	packet := buildTCPPacket(t, []byte("ping"))
	require.True(t, matched.MatchPackage(packet))
	require.False(t, notMatched.MatchPackage(packet))
}

func TestByteExtractVariables(t *testing.T) {
	within := compileTestRule(t, `alert tcp any any -> any any (content:"|00|"; byte_extract:1,0,len,relative; content:"ABC"; within:len; sid:1;)`)
	require.True(t, within.MatchPackage(buildTCPPacket(t, []byte{0x00, 0x04, 'A', 'B', 'C'})))
	require.False(t, within.MatchPackage(buildTCPPacket(t, []byte{0x00, 0x03, 'A', 'B', 'C'})))

	value := compileTestRule(t, `alert tcp any any -> any any (content:"|00|"; byte_extract:1,0,want,relative; content:"ABC"; byte_test:1,=,want,0,relative; sid:2;)`)
	require.True(t, value.MatchPackage(buildTCPPacket(t, []byte{0x00, 0x04, 'A', 'B', 'C', 0x04})))

	offset := compileTestRule(t, `alert tcp any any -> any any (content:"|00|"; byte_extract:1,0,off,relative; content:"ABC"; offset:off; depth:3; sid:3;)`)
	require.True(t, offset.MatchPackage(buildTCPPacket(t, []byte{0x00, 0x02, 'A', 'B', 'C'})))
}

func TestRawBytesMatchesPacketHeaders(t *testing.T) {
	packet := buildTCPPacket(t, []byte("x"))
	raw := compileTestRule(t, `alert tcp any any -> any any (rawbytes; content:"|c0 00 02 01|"; sid:1;)`)
	normal := compileTestRule(t, `alert tcp any any -> any any (content:"|c0 00 02 01|"; sid:2;)`)
	require.True(t, raw.MatchPackage(packet))
	require.False(t, normal.MatchPackage(packet))
}

func TestBase64DecodeAndData(t *testing.T) {
	request := []byte("GET / HTTP/1.1\r\nHost: example.com\r\nAuthorization: Basic dXNlcjpwYXNz\r\n\r\n")
	decoded := compileTestRule(t, `alert http any any -> any any (http.header; content:"Authorization: Basic "; base64_decode:offset 0,relative; base64_data; content:"user:pass"; fast_pattern; sid:1;)`)
	wrong := compileTestRule(t, `alert http any any -> any any (http.header; content:"Authorization: Basic "; base64_decode:offset 0,relative; base64_data; content:"admin:admin"; sid:2;)`)
	require.True(t, decoded.MatchPackage(buildTCPPacket(t, request)))
	require.False(t, wrong.MatchPackage(buildTCPPacket(t, request)))

	payload := compileTestRule(t, `alert tcp any any -> any any (content:"prefix "; base64_decode:bytes 8, offset 0, relative; base64_data; content:"test"; sid:3;)`)
	require.True(t, payload.MatchPackage(buildTCPPacket(t, []byte("prefix dGVzdA=="))))
}

func TestPCREQuotesBackslashesAndOptions(t *testing.T) {
	innerQuote := compileTestRule(t, `alert tcp any any -> any any (pcre:"/^["']/"; sid:1;)`)
	require.True(t, innerQuote.MatchPackage(buildTCPPacket(t, []byte(`"quoted`))))
	require.False(t, innerQuote.MatchPackage(buildTCPPacket(t, []byte("quoted"))))

	literalBackslash := compileTestRule(t, `alert tcp any any -> any any (pcre:"/^\\x41/"; sid:2;)`)
	require.True(t, literalBackslash.MatchPackage(buildTCPPacket(t, []byte(`\x41`))), "double-escaped PCRE must match a literal backslash")
	require.False(t, literalBackslash.MatchPackage(buildTCPPacket(t, []byte("A"))))

	literalPath := compileTestRule(t, `alert tcp any any -> any any (pcre:"/^C:\\Windows/"; sid:4;)`)
	require.True(t, literalPath.MatchPackage(buildTCPPacket(t, []byte(`C:\Windows`))))
	require.False(t, literalPath.MatchPackage(buildTCPPacket(t, []byte("C:XWindows"))), "\\\\W must not degrade into a non-word character class")

	rawBytes := compileTestRule(t, `alert tcp any any -> any any (pcre:"/\xc0\x00\x02\x01/B"; sid:3;)`)
	require.True(t, rawBytes.MatchPackage(buildTCPPacket(t, []byte("x"))), "pcre /B must inspect packet bytes")
}

func TestPCREHTTPBufferOptions(t *testing.T) {
	rawHeader := compileTestRule(t, `alert http any any -> any any (pcre:"/^Z-Test: 1/D"; sid:1;)`)
	normalizedHeader := compileTestRule(t, `alert http any any -> any any (pcre:"/^A-Other: 2/H"; sid:2;)`)
	payload := []byte("GET / HTTP/1.1\r\nZ-Test: 1\r\nA-Other: 2\r\n\r\n")
	require.True(t, rawHeader.MatchPackage(buildTCPPacket(t, payload)), "/D must select the raw header buffer")
	require.True(t, normalizedHeader.MatchPackage(buildTCPPacket(t, payload)), "/H must select the normalized header buffer")
}

func TestSSHProtoAndFTPBounce(t *testing.T) {
	ssh := compileTestRule(t, `alert ssh any any -> any any (ssh_proto; content:"PUTTY"; sid:1;)`)
	require.True(t, ssh.MatchPackage(buildTCPPacket(t, []byte("SSH-2.0-PUTTY\r\n"))))
	require.False(t, ssh.MatchPackage(buildTCPPacket(t, []byte("SSH-2.0-OpenSSH_9.0\r\n"))))

	bounce := compileTestRule(t, `alert tcp any any -> any any (content:"PORT"; nocase; ftpbounce; sid:2;)`)
	require.True(t, bounce.MatchPackage(buildTCPPacket(t, []byte("PORT 10,0,0,1,4,2\r\n"))))
	require.False(t, bounce.MatchPackage(buildTCPPacket(t, []byte("PORT 192,0,2,1,4,2\r\n"))), "the client source address is not a bounce")
}

func TestAppLayerProtocolAndHTTPEvent(t *testing.T) {
	notHTTP := compileTestRule(t, `alert tcp any any -> any any (app-layer-protocol:!http; content:"MARK"; sid:1;)`)
	http := compileTestRule(t, `alert tcp any any -> any any (app-layer-protocol:http; content:"POST"; sid:2;)`)
	require.True(t, notHTTP.MatchPackage(buildTCPPacket(t, []byte("MARK"))))
	require.False(t, notHTTP.MatchPackage(buildTCPPacket(t, []byte("POST /x HTTP/1.1\r\nHost: a\r\n\r\nMARK"))))
	require.True(t, http.MatchPackage(buildTCPPacket(t, []byte("POST /x HTTP/1.1\r\nHost: a\r\n\r\n"))))
	require.False(t, http.MatchPackage(buildTCPPacket(t, []byte("POST"))))

	invalidTE := compileTestRule(t, `alert http any any -> any any (app-layer-event:http.invalid_transfer_encoding_value_in_request; http.method; content:"POST"; sid:3;)`)
	require.True(t, invalidTE.MatchPackage(buildTCPPacket(t, []byte("POST /x HTTP/1.1\r\nHost: a\r\nTransfer-Encoding: bogus\r\n\r\n"))))
	require.False(t, invalidTE.MatchPackage(buildTCPPacket(t, []byte("POST /x HTTP/1.1\r\nHost: a\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"))))
	require.True(t, invalidTE.MatchPackage(buildTCPPacket(t, []byte("POST /x HTTP/1.0\r\nHost: a\r\nTransfer-Encoding: chunked\r\n\r\n"))), "HTTP/1.0 cannot use chunked framing")
}

func TestASN1Checks(t *testing.T) {
	oversize := compileTestRule(t, `alert ip any any -> any any (asn1:oversize_length 10; sid:1;)`)
	payload := []byte{0x04, 0x82, 0x01, 0x00}
	payload = append(payload, make([]byte, 256)...)
	require.True(t, oversize.MatchPackage(buildIPPacket(t, payload)))
	require.False(t, compileTestRule(t, `alert ip any any -> any any (asn1:oversize_length 300; sid:2;)`).MatchPackage(buildIPPacket(t, payload)))

	bitstring := compileTestRule(t, `alert ip any any -> any any (content:"AA"; asn1:relative_offset 0, bitstring_overflow; sid:3;)`)
	require.True(t, bitstring.MatchPackage(buildIPPacket(t, []byte{'A', 'A', 0x03, 0x01, 0xff})))
	require.False(t, bitstring.MatchPackage(buildIPPacket(t, []byte{'A', 'A', 0x03, 0x01, 0x07})))

	real := compileTestRule(t, `alert ip any any -> any any (asn1:absolute_offset 0, double_overflow; sid:4;)`)
	realPayload := []byte{0x09, 0x82, 0x01, 0x01}
	realPayload = append(realPayload, make([]byte, 257)...)
	require.True(t, real.MatchPackage(buildIPPacket(t, realPayload)))
}

func buildIPPacket(t *testing.T, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: 253,
		SrcIP:    net.ParseIP("192.0.2.1").To4(),
		DstIP:    net.ParseIP("192.0.2.2").To4(),
	}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true}, eth, ip, gopacket.Payload(payload)))
	return gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestIPHeaderStickyBuffers(t *testing.T) {
	ipv4Rule := compileTestRule(t, `alert ip any any -> any any (ipv4.hdr; content:"|c0 00 02 01|"; sid:1;)`)
	require.True(t, ipv4Rule.MatchPackage(buildTCPPacket(t, []byte("x"))))
	require.False(t, ipv4Rule.MatchPackage(buildICMPv6Packet(t, []byte("x"))))

	ipv6Rule := compileTestRule(t, `alert ip any any -> any any (ipv6.hdr; content:"|20 01 0d b8|"; sid:2;)`)
	require.True(t, ipv6Rule.MatchPackage(buildICMPv6Packet(t, []byte("x"))))
	require.False(t, ipv6Rule.MatchPackage(buildTCPPacket(t, []byte("x"))))
}

func TestTransportHeaderStickyBuffers(t *testing.T) {
	udpRule := compileTestRule(t, `alert udp any any -> any any (udp.hdr; content:"|00 35|"; sid:1;)`)
	require.True(t, udpRule.MatchPackage(buildDNSQueryPacket(t, "example.com")))

	icmp4Rule := compileTestRule(t, `alert icmp any any -> any any (icmpv4.hdr; content:"|08 00|"; sid:2;)`)
	require.True(t, icmp4Rule.MatchPackage(buildICMPv4Packet(t, []byte("ping"))))

	icmp6Rule := compileTestRule(t, `alert icmpv6 any any -> any any (icmpv6.hdr; content:"|86 00|"; sid:3;)`)
	require.True(t, icmp6Rule.MatchPackage(buildICMPv6Packet(t, []byte("ra"))))
}

func buildICMPv4Packet(t *testing.T, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolICMPv4,
		SrcIP:    net.ParseIP("192.0.2.1").To4(),
		DstIP:    net.ParseIP("192.0.2.2").To4(),
	}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(8, 0)}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true}, eth, ip, icmp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestICMPv6PayloadMatching(t *testing.T) {
	rule := compileTestRule(t, `alert icmpv6 any any -> any any (content:"|19|"; offset:12; depth:1; dsize:>10; sid:1;)`)
	payload := make([]byte, 20)
	payload[12] = 0x19
	require.True(t, rule.MatchPackage(buildICMPv6Packet(t, payload)))
	payload[12] = 0x18
	require.False(t, rule.MatchPackage(buildICMPv6Packet(t, payload)))
}

func buildICMPv6Packet(t *testing.T, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv6,
	}
	ip := &layers.IPv6{
		Version:    6,
		NextHeader: layers.IPProtocolICMPv6,
		HopLimit:   64,
		SrcIP:      net.ParseIP("2001:db8::1"),
		DstIP:      net.ParseIP("2001:db8::2"),
	}
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(134, 0)}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true}, eth, ip, icmp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func buildDNSQueryPacket(t *testing.T, name string) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP("192.0.2.1").To4(),
		DstIP:    net.ParseIP("192.0.2.2").To4(),
	}
	udp := &layers.UDP{SrcPort: 40000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	dns := &layers.DNS{
		ID:        1,
		RD:        true,
		Questions: []layers.DNSQuestion{{Name: []byte(name), Type: layers.DNSTypeA, Class: layers.DNSClassIN}},
	}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, dns))
	return gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}
