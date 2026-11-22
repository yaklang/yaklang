package match

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func buildExtTCPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     100,
		Ack:     1,
		ACK:     true,
		PSH:     true,
		Window:  65535,
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func buildTestUDPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcPort),
		DstPort: layers.UDPPort(dstPort),
	}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

// buildDCERPCv5Bind builds a valid little-endian DCE/RPC v5 Bind PDU for a
// canonical UUID (e.g., MS08-067 SRVSVC 4b324fc8-1670-01d3-1278-5a47bf6ee188).
func buildDCERPCv5Bind(t *testing.T, ctxID uint16, uuidLE []byte, major, minor uint16) []byte {
	t.Helper()
	require.Len(t, uuidLE, 16)
	const fragLen = 72
	pdu := make([]byte, fragLen)
	pdu[0] = 5    // rpc_vers
	pdu[1] = 0    // rpc_vers_minor
	pdu[2] = 11   // PTYPE = Bind (0x0B)
	pdu[3] = 0x03 // PFC_FIRST_FRAG | PFC_LAST_FRAG
	pdu[4] = 0x10 // Little-endian
	binary.LittleEndian.PutUint16(pdu[8:10], fragLen)
	binary.LittleEndian.PutUint32(pdu[12:16], 1) // call_id
	binary.LittleEndian.PutUint16(pdu[16:18], 4280)
	binary.LittleEndian.PutUint16(pdu[18:20], 4280)
	pdu[24] = 1 // n_context_elem
	binary.LittleEndian.PutUint16(pdu[28:30], ctxID)
	pdu[30] = 1 // n_transfer_syn
	copy(pdu[32:48], uuidLE)
	binary.LittleEndian.PutUint16(pdu[48:50], major)
	binary.LittleEndian.PutUint16(pdu[50:52], minor)
	// NDR transfer syntax 8a885d04-1ceb-11c9-9fe8-08002b104860 v2.0
	ndr, _ := hex.DecodeString("045d888aeb1cc9119fe808002b104860")
	copy(pdu[52:68], ndr)
	binary.LittleEndian.PutUint32(pdu[68:72], 2)
	return pdu
}

func buildDCERPCv5Request(ctxID, opnum uint16, stub []byte) []byte {
	fragLen := 24 + len(stub)
	pdu := make([]byte, fragLen)
	pdu[0] = 5
	pdu[1] = 0
	pdu[2] = 0 // PTYPE = Request
	pdu[3] = 0x03
	pdu[4] = 0x10
	binary.LittleEndian.PutUint16(pdu[8:10], uint16(fragLen))
	binary.LittleEndian.PutUint32(pdu[12:16], 2)
	binary.LittleEndian.PutUint32(pdu[16:20], uint32(len(stub)))
	binary.LittleEndian.PutUint16(pdu[20:22], ctxID)
	binary.LittleEndian.PutUint16(pdu[22:24], opnum)
	copy(pdu[24:], stub)
	return pdu
}

func buildDCERPCv4Request(t *testing.T, uuidLE []byte, opnum uint16, stub []byte) []byte {
	t.Helper()
	require.Len(t, uuidLE, 16)
	pdu := make([]byte, 80+len(stub))
	pdu[0] = 4    // rpc_vers = 4 (connectionless UDP)
	pdu[1] = 0    // PTYPE = Request
	pdu[4] = 0x10 // Little-endian
	copy(pdu[24:40], uuidLE)
	binary.LittleEndian.PutUint32(pdu[60:64], 3) // if_vers = 3
	binary.LittleEndian.PutUint16(pdu[68:70], opnum)
	binary.LittleEndian.PutUint16(pdu[74:76], uint16(len(stub)))
	copy(pdu[80:], stub)
	return pdu
}

func TestDCERPCv5Andv4Matching(t *testing.T) {
	// MS08-067 SRVSVC UUID: 4b324fc8-1670-01d3-1278-5a47bf6ee188 in LE wire order
	srvsvcLE, err := hex.DecodeString("c84f324b7016d30112785a47bf6ee188")
	require.NoError(t, err)
	otherLE, err := hex.DecodeString("00112233445566778899aabbccddeeff")
	require.NoError(t, err)

	// 1. TCP Bind + Request with stub_data (exact ETPro MS08-067 sid:2008702)
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any 445 (msg:"MS08-067 TCP"; dcerpc.iface:4b324fc8-1670-01d3-1278-5a47bf6ee188; dcerpc.opnum:31; dcerpc.stub_data; content:"|5C|..|5C|"; sid:2008702; rev:7;)`)
	require.NoError(t, err)
	m, err := CompileRule(rules[0])
	require.NoError(t, err)

	bindPDU := buildDCERPCv5Bind(t, 0, srvsvcLE, 3, 0)
	reqPDU := buildDCERPCv5Request(0, 31, []byte("prefix\\..\\..\\evil"))
	stream := append(append([]byte(nil), bindPDU...), reqPDU...)
	pk := buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 445, stream)
	require.True(t, m.MatchPackage(pk))

	// SMB2-wrapped Bind + Request
	smb2Wrapped := append([]byte{0x00, 0x00, 0x00, 0x90, 0xfe, 'S', 'M', 'B', 0x40, 0x00}, bindPDU...)
	smb2Wrapped = append(smb2Wrapped, []byte{0x00, 0x00, 0x00, 0x60, 0xfe, 'S', 'M', 'B', 0x40, 0x00}...)
	smb2Wrapped = append(smb2Wrapped, reqPDU...)
	require.True(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 445, smb2Wrapped)))

	// Wrong opnum -> must not match
	wrongOp := append(append([]byte(nil), bindPDU...), buildDCERPCv5Request(0, 15, []byte("prefix\\..\\..\\evil"))...)
	require.False(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 445, wrongOp)))

	// Wrong iface UUID -> must not match
	wrongUUID := append(buildDCERPCv5Bind(t, 0, otherLE, 3, 0), reqPDU...)
	require.False(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 445, wrongUUID)))

	// 2. TCP Bind-only rule (exact ETPro sid:2008701)
	bindRules, err := rule.ParseStrictRule(`alert tcp any any -> any 445 (msg:"MS08-067 Bind"; content:"|0B|"; offset:2; depth:1; dcerpc.iface:4b324fc8-1670-01d3-1278-5a47bf6ee188; sid:2008701; rev:6;)`)
	require.NoError(t, err)
	bindMatcher, err := CompileRule(bindRules[0])
	require.NoError(t, err)
	require.True(t, bindMatcher.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 445, bindPDU)))

	// 3. UDP DCE/RPC v4 rule (exact ETPro sid:2008693)
	udpRules, err := rule.ParseStrictRule(`alert udp any any -> any 139 (msg:"MS08-067 UDP"; dcerpc.iface:4b324fc8-1670-01d3-1278-5a47bf6ee188; dcerpc.opnum:31; dcerpc.stub_data; content:"|00 2E 00 2E 00 2F 00 2E 00 2E 00 2F|"; sid:2008693; rev:6;)`)
	require.NoError(t, err)
	udpMatcher, err := CompileRule(udpRules[0])
	require.NoError(t, err)
	utf16Stub := []byte{0x00, 0x00, 0x00, '.', 0x00, '.', 0x00, '/', 0x00, '.', 0x00, '.', 0x00, '/'}
	v4PDU := buildDCERPCv4Request(t, srvsvcLE, 31, utf16Stub)
	require.True(t, udpMatcher.MatchPackage(buildTestUDPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 139, v4PDU)))
	require.False(t, udpMatcher.MatchPackage(buildTestUDPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 139, buildDCERPCv4Request(t, srvsvcLE, 32, utf16Stub))))
}

func TestFileMagicAndFileSizeMatching(t *testing.T) {
	// 1. 7-Zip CVE-2023-31102 (exact ETPro sid:2065690)
	rules7z, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"7z CVE-2023-31102"; file.magic; content:"7-zip archive"; startswith; file.data; content:"7z|bc af 27 1c 00 30 00 00|"; startswith; fast_pattern; pcre:"/\x30{24,}\x00\x30{24,}\x17\x06\x1a\x01/"; filesize:<10000; sid:2065690; rev:1;)`)
	require.NoError(t, err)
	m7z, err := CompileRule(rules7z[0])
	require.NoError(t, err)

	payload7z := []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x30, 0x00, 0x00}
	payload7z = append(payload7z, bytes.Repeat([]byte{0x30}, 26)...)
	payload7z = append(payload7z, 0x00)
	payload7z = append(payload7z, bytes.Repeat([]byte{0x30}, 26)...)
	payload7z = append(payload7z, 0x17, 0x06, 0x1a, 0x01)
	require.True(t, m7z.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 80, 51234, payload7z)))

	// Oversize >= 10000 bytes must fail filesize:<10000
	oversize7z := append(append([]byte(nil), payload7z...), make([]byte, 10000)...)
	matched, err := m7z.MatchStream(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 80, 51234, nil), oversize7z)
	require.NoError(t, err)
	require.False(t, matched)

	// 2. Zip-Slip over HTTP download on alert tcp (exact ETPro sid:2064945)
	rulesZip, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"Zip Slip"; file.magic; content:"Zip archive"; file.data; content:"|00 00 00 2e 2e 2f|"; fast_pattern; sid:2064945; rev:1;)`)
	require.NoError(t, err)
	mZip, err := CompileRule(rulesZip[0])
	require.NoError(t, err)

	zipBytes := append([]byte{'P', 'K', 0x03, 0x04, 0x14, 0x00, 0x00, 0x00}, []byte{0x00, 0x00, 0x00, '.', '.', '/', 'e', 't', 'c'}...)
	httpZip := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/zip\r\n\r\n"), zipBytes...)
	require.True(t, mZip.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 80, 51234, httpZip)))
	require.False(t, mZip.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 80, 51234, []byte("plain text \x00\x00\x00../"))))

	// 3. POSIX tar archive Zip-Slip (exact ETPro sid:2064946)
	rulesTar, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"Tar Zip Slip"; file.magic; content:"POSIX tar archive"; file.data; content:"|00 00 00 2e 2e 2f|"; fast_pattern; sid:2064946; rev:1;)`)
	require.NoError(t, err)
	mTar, err := CompileRule(rulesTar[0])
	require.NoError(t, err)

	tarBuf := make([]byte, 512)
	copy(tarBuf[257:263], []byte("ustar\x00"))
	copy(tarBuf[10:16], []byte{0x00, 0x00, 0x00, '.', '.', '/'})
	require.True(t, mTar.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 80, 51234, tarBuf)))
}

func TestHTTP2WindowUpdateMatching(t *testing.T) {
	// Exact ETPro Nginx CVE-2026-123007 rule (sid:2072837)
	rules, err := rule.ParseStrictRule(`alert http2 any any -> any any (msg:"HTTP/2 Near-Max WINDOW_UPDATE"; http2.frametype:WINDOWUPDATE; http2.window:>2147483000; sid:2072837; rev:1;)`)
	require.NoError(t, err)
	m, err := CompileRule(rules[0])
	require.NoError(t, err)

	buildH2 := func(winInc uint32) []byte {
		out := append([]byte(nil), []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")...)
		// Empty SETTINGS frame (len=0, type=4, flags=0, stream=0)
		out = append(out, 0, 0, 0, 4, 0, 0, 0, 0, 0)
		// WINDOW_UPDATE frame (len=4, type=8, flags=0, stream=0)
		out = append(out, 0, 0, 4, 8, 0, 0, 0, 0, 0)
		var inc [4]byte
		binary.BigEndian.PutUint32(inc[:], winInc)
		out = append(out, inc[:]...)
		return out
	}

	require.True(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 443, buildH2(2147483647))))
	require.False(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 443, buildH2(65535))))
	require.False(t, m.MatchPackage(buildExtTCPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 443, []byte("GET / HTTP/1.1\r\nHost: a\r\n\r\n"))))
}

func TestSNMPVersionMatching(t *testing.T) {
	// Exact ETPro Cisco IOS SNMP RCE CVE-2025-20352 rule (sid:2069171)
	rules, err := rule.ParseStrictRule(`alert snmp any any -> any 161 (msg:"Cisco IOS SNMP RCE"; snmp.version:<=2; content:"|06 0D 2B 06 01 04 01 09 09 85 10 01 04 02 01|"; pcre:"/^.{0,256}(?:\x24\x28|\x3b|\x7c|\x60)/R"; sid:2069171; rev:1;)`)
	require.NoError(t, err)
	m, err := CompileRule(rules[0])
	require.NoError(t, err)

	oid := []byte{0x06, 0x0d, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x09, 0x09, 0x85, 0x10, 0x01, 0x04, 0x02, 0x01}
	buildSNMP := func(wireVer byte, nextTag byte) []byte {
		body := []byte{0x02, 0x01, wireVer, nextTag, 0x06, 'p', 'u', 'b', 'l', 'i', 'c'}
		body = append(body, oid...)
		body = append(body, []byte(";id")...)
		return append([]byte{0x30, byte(len(body))}, body...)
	}

	// SNMPv1 (wire 0 -> version 1) & SNMPv2c (wire 1 -> version 2) match <=2
	require.True(t, m.MatchPackage(buildTestUDPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 161, buildSNMP(0, 0x04))))
	require.True(t, m.MatchPackage(buildTestUDPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 161, buildSNMP(1, 0x04))))
	// SNMPv3 (wire 3 -> version 3) must NOT match <=2
	require.False(t, m.MatchPackage(buildTestUDPPacket(t, "198.51.100.5", "10.0.0.10", 51234, 161, buildSNMP(3, 0x30))))
}

func TestHTTPRequestAuthUnrecognizedEvent(t *testing.T) {
	// Exact ETPro PigButcher kit headers rule (sid:2055539, stateless variant)
	rules, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"PigButcher Auth"; app-layer-event:http.request_auth_unrecognized; http.header_names; to_lowercase; content:"|0d 0a|x-channel-code"; fast_pattern; content:"|0d 0a|x-app-code"; sid:2055539; rev:1;)`)
	require.NoError(t, err)
	m, err := CompileRule(rules[0])
	require.NoError(t, err)

	badAuth := []byte("GET /api/info HTTP/1.1\r\nHost: evil.example\r\nAuthorization: raw-pigbutcher-token\r\nX-Channel-Code: 1\r\nX-App-Code: 2\r\n\r\n")
	require.True(t, m.MatchPackage(buildExtTCPPacket(t, "10.0.0.5", "198.51.100.9", 51234, 80, badAuth)))

	goodAuth := []byte("GET /api/info HTTP/1.1\r\nHost: evil.example\r\nAuthorization: Bearer eyJhbGciOiJIUzI1NiJ9\r\nX-Channel-Code: 1\r\nX-App-Code: 2\r\n\r\n")
	require.False(t, m.MatchPackage(buildExtTCPPacket(t, "10.0.0.5", "198.51.100.9", 51234, 80, goodAuth)))
}
