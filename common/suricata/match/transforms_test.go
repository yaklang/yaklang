package match

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func buildTestTCPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) gopacket.Packet {
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
		Ack:     200,
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

func formatHexContent(b []byte) string {
	out := "|"
	for i, v := range b {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%02x", v)
	}
	return out + "|"
}

func TestBufferTransformsAll(t *testing.T) {
	reqPayload := []byte(
		"POST /Admin/%70%61%6e%65%6c/Login%20Page?q=1 HTTP/1.1\r\n" +
			"Host: EVIL.Example.COM\r\n" +
			"X-Custom-Header: VaLuE One\r\n" +
			"User-Agent: My   Spaced\tAgent\r\n" +
			"Content-Length: 11\r\n" +
			"\r\n" +
			"hello world",
	)
	pk := buildTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 51234, 80, reqPayload)

	bodySHA1 := sha1.Sum([]byte("hello world"))
	bodySHA256 := sha256.Sum256([]byte("hello world"))
	bodyMD5 := md5.Sum([]byte("hello world"))

	cases := []struct {
		name  string
		rule  string
		match bool
	}{
		{
			name:  "to_lowercase on http.uri",
			rule:  `alert http any any -> any any (msg:"lower"; http.uri; to_lowercase; content:"/admin/"; sid:1001; rev:1;)`,
			match: true,
		},
		{
			name:  "to_lowercase rejects uppercase needle without nocase",
			rule:  `alert http any any -> any any (msg:"lower-miss"; http.uri; to_lowercase; content:"/Admin/"; sid:1002; rev:1;)`,
			match: false,
		},
		{
			name:  "to_uppercase on http.host",
			rule:  `alert http any any -> any any (msg:"upper"; http.host; to_uppercase; content:"EVIL.EXAMPLE.COM"; sid:1003; rev:1;)`,
			match: true,
		},
		{
			name:  "url_decode chained with to_lowercase",
			rule:  `alert http any any -> any any (msg:"urldec-lower"; http.uri.raw; url_decode; to_lowercase; content:"/admin/panel/login page"; sid:1004; rev:1;)`,
			match: true,
		},
		{
			name:  "header_lowercase lowercases names but keeps values",
			rule:  `alert http any any -> any any (msg:"hdr-lower"; http.header; header_lowercase; content:"x-custom-header: VaLuE One"; sid:1005; rev:1;)`,
			match: true,
		},
		{
			name:  "header_lowercase does not lowercase value",
			rule:  `alert http any any -> any any (msg:"hdr-lower-miss"; http.header; header_lowercase; content:"x-custom-header: value one"; sid:1006; rev:1;)`,
			match: false,
		},
		{
			name:  "strip_whitespace on http.user_agent",
			rule:  `alert http any any -> any any (msg:"strip-ws"; http.user_agent; strip_whitespace; content:"MySpacedAgent"; bsize:13; sid:1007; rev:1;)`,
			match: true,
		},
		{
			name:  "dotprefix chained with to_lowercase",
			rule:  `alert http any any -> any any (msg:"dotprefix-lower"; http.host; dotprefix; to_lowercase; content:".example.com"; endswith; sid:1008; rev:1;)`,
			match: true,
		},
		{
			name:  "to_sha1 on http.request_body",
			rule:  fmt.Sprintf(`alert http any any -> any any (msg:"sha1"; http.request_body; to_sha1; content:"%s"; bsize:20; sid:1009; rev:1;)`, formatHexContent(bodySHA1[:])),
			match: true,
		},
		{
			name:  "to_sha256 on http.request_body",
			rule:  fmt.Sprintf(`alert http any any -> any any (msg:"sha256"; http.request_body; to_sha256; content:"%s"; bsize:32; sid:1010; rev:1;)`, formatHexContent(bodySHA256[:])),
			match: true,
		},
		{
			name:  "to_md5 on http.request_body",
			rule:  fmt.Sprintf(`alert http any any -> any any (msg:"md5"; http.request_body; to_md5; content:"%s"; bsize:16; sid:1011; rev:1;)`, formatHexContent(bodyMD5[:])),
			match: true,
		},
		{
			name:  "http1 protocol alias",
			rule:  `alert http1 any any -> any any (msg:"http1"; http.method; content:"POST"; http.uri; to_lowercase; content:"/admin/"; sid:1012; rev:1;)`,
			match: true,
		},
		{
			name:  "pkt_data resets sticky buffer and transforms",
			rule:  `alert http any any -> any any (msg:"pkt-data"; http.uri; to_lowercase; content:"/admin/"; pkt_data; content:"EVIL.Example.COM"; sid:1013; rev:1;)`,
			match: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs, err := rule.ParseStrictRule(tc.rule)
			require.NoError(t, err)
			require.Len(t, rs, 1)
			m, err := CompileRule(rs[0])
			require.NoError(t, err)
			got, err := m.MatchStream(pk, reqPayload)
			require.NoError(t, err)
			require.Equal(t, tc.match, got)
		})
	}
}

func TestStripPseudoHeadersTransform(t *testing.T) {
	raw := []byte(":method: GET\r\n:authority: example.com\r\nHost: example.com\r\nUser-Agent: test\r\n")
	stripped := applyBufferTransforms(raw, []string{"strip_pseudo_headers"}, false)
	require.Equal(t, "Host: example.com\r\nUser-Agent: test\r\n", string(stripped))
}

func TestHTTPRequestAndResponseHeaderMultiBuffer(t *testing.T) {
	reqPayload := []byte(
		"GET /index.html HTTP/1.1\r\n" +
			"Host: app.example.com\r\n" +
			"X-Token: Secret-123\r\n" +
			"User-Agent: CustomBot/1.0\r\n" +
			"\r\n",
	)
	reqPkt := buildTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 51234, 80, reqPayload)

	respPayload := []byte(
		"HTTP/1.1 200 OK\r\n" +
			"Server: FireBreak/2.0\r\n" +
			"X-Powered-By: YakLang\r\n" +
			"Content-Length: 2\r\n" +
			"\r\nOK",
	)
	respPkt := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 51234, respPayload)

	// Single header line matches startswith + endswith + header_lowercase.
	r1, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"req hdr"; http.request_header; header_lowercase; content:"x-token: "; startswith; content:"-123"; endswith; sid:2001; rev:1;)`)
	require.NoError(t, err)
	m1, err := CompileRule(r1[0])
	require.NoError(t, err)
	ok, err := m1.MatchStream(reqPkt, reqPayload)
	require.NoError(t, err)
	require.True(t, ok)

	// Two contents from DIFFERENT headers must NOT match one http.request_header group.
	r2, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"req hdr cross"; http.request_header; content:"Host: "; startswith; content:"Secret-123"; sid:2002; rev:1;)`)
	require.NoError(t, err)
	m2, err := CompileRule(r2[0])
	require.NoError(t, err)
	ok, err = m2.MatchStream(reqPkt, reqPayload)
	require.NoError(t, err)
	require.False(t, ok)

	// Response header multi-buffer matching.
	r3, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"resp hdr"; http.response_header; to_lowercase; content:"x-powered-by: yaklang"; bsize:21; sid:2003; rev:1;)`)
	require.NoError(t, err)
	m3, err := CompileRule(r3[0])
	require.NoError(t, err)
	ok, err = m3.MatchStream(respPkt, respPayload)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRDPAndTLSRandomServerHello(t *testing.T) {
	// RDP TPKT/X.224 Connection Request payload.
	rdpPayload := []byte("\x03\x00\x00\x2b\x26\xe0\x00\x00\x00\x00\x00Cookie: mstshash=admin\r\n")
	rdpPkt := buildTestTCPPacket(t, "198.51.100.10", "10.0.0.10", 52000, 3389, rdpPayload)
	rRDP, err := rule.ParseStrictRule(`alert rdp any any -> any 3389 (msg:"rdp cookie"; content:"Cookie: mstshash="; sid:3001; rev:1;)`)
	require.NoError(t, err)
	mRDP, err := CompileRule(rRDP[0])
	require.NoError(t, err)
	ok, err := mRDP.MatchStream(rdpPkt, rdpPayload)
	require.NoError(t, err)
	require.True(t, ok)

	// TLS ServerHello with RFC 8446 TLS 1.1 downgrade sentinel in last 8 bytes of tls.random.
	var body []byte
	body = append(body, 0x03, 0x03) // legacy version TLS 1.2
	random32 := make([]byte, 32)
	copy(random32[0:4], []byte{0x65, 0x01, 0x02, 0x03})
	copy(random32[24:32], []byte{'D', 'O', 'W', 'N', 'G', 'R', 'D', 0x00})
	body = append(body, random32...)
	body = append(body, 0x00)       // session_id len = 0
	body = append(body, 0xc0, 0x2f) // cipher suite
	body = append(body, 0x00)       // compression

	hs := make([]byte, 4+len(body))
	hs[0] = 2 // ServerHello
	hs[1] = byte(len(body) >> 16)
	hs[2] = byte(len(body) >> 8)
	hs[3] = byte(len(body))
	copy(hs[4:], body)

	rec := make([]byte, 5+len(hs))
	rec[0] = 22 // Handshake
	rec[1] = 0x03
	rec[2] = 0x03
	binary.BigEndian.PutUint16(rec[3:5], uint16(len(hs)))
	copy(rec[5:], hs)

	tlsPkt := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 443, 52001, rec)
	rTLS, err := rule.ParseStrictRule(`alert tls any any -> any any (msg:"tls downgrade"; tls.random; content:"DOWNGRD|00|"; offset:24; depth:8; sid:3002; rev:1;)`)
	require.NoError(t, err)
	mTLS, err := CompileRule(rTLS[0])
	require.NoError(t, err)
	ok, err = mTLS.MatchStream(tlsPkt, rec)
	require.NoError(t, err)
	require.True(t, ok)

	// Also verify tls.random_time and tls.random_bytes slices.
	rTLSTime, err := rule.ParseStrictRule(`alert tls any any -> any any (msg:"tls random time"; tls.random_time; content:"|65 01 02 03|"; bsize:4; tls.random_bytes; content:"DOWNGRD|00|"; endswith; sid:3003; rev:1;)`)
	require.NoError(t, err)
	mTLSTime, err := CompileRule(rTLSTime[0])
	require.NoError(t, err)
	ok, err = mTLSTime.MatchStream(tlsPkt, rec)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestAddressRuleFastRangeLookup(t *testing.T) {
	rs, err := rule.ParseStrictRule(`alert ip [198.51.100.0/24,203.0.113.10,198.51.101.0/24,!198.51.100.5] any -> $HOME_NET any (msg:"ip blocklist"; sid:4001; rev:1;)`)
	require.NoError(t, err)
	src := rs[0].SourceAddress
	dst := rs[0].DestinationAddress

	require.True(t, src.MatchAddr(netip.MustParseAddr("198.51.100.1")))
	require.False(t, src.MatchAddr(netip.MustParseAddr("198.51.100.5")))
	require.True(t, src.MatchAddr(netip.MustParseAddr("198.51.101.254")))
	require.True(t, src.MatchAddr(netip.MustParseAddr("203.0.113.10")))
	require.False(t, src.MatchAddr(netip.MustParseAddr("203.0.113.11")))
	require.True(t, dst.MatchAddr(netip.MustParseAddr("10.0.0.80")))
	require.False(t, dst.MatchAddr(netip.MustParseAddr("8.8.8.8")))
}

func TestHTTPGzipAndDeflateBodyDecompression(t *testing.T) {
	plain := []byte("<html><body>SECRET_EXPLOIT_RESPONSE_MARKER</body></html>")

	// 1. Gzip-encoded HTTP response.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err := gw.Write(plain)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	gzResp := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n",
		gzBuf.Len(),
	)), gzBuf.Bytes()...)
	pkGz := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 52000, gzResp)

	rBody, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"gzip body"; http.response_body; content:"SECRET_EXPLOIT_RESPONSE_MARKER"; sid:5001; rev:1;)`)
	require.NoError(t, err)
	mBody, err := CompileRule(rBody[0])
	require.NoError(t, err)
	ok, err := mBody.MatchStream(pkGz, gzResp)
	require.NoError(t, err)
	require.True(t, ok)

	// Verify ParseHTTPStream reports WasDecompressed()==true after body access.
	shared := ParseHTTPStream(pkGz, gzResp)
	require.NotNil(t, shared)
	require.Contains(t, string(shared.Get(0x00)), "HTTP/1.1")
	require.Contains(t, string(shared.inner.bodyBuffer()), "SECRET_EXPLOIT_RESPONSE_MARKER")
	require.True(t, shared.WasDecompressed())

	// 2. Deflate (zlib) encoded HTTP response via file_data.
	var zlBuf bytes.Buffer
	zw := zlib.NewWriter(&zlBuf)
	_, err = zw.Write(plain)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	zlResp := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Encoding: deflate\r\nContent-Length: %d\r\n\r\n",
		zlBuf.Len(),
	)), zlBuf.Bytes()...)
	pkZl := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 52001, zlResp)

	rFD, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"deflate file_data"; file_data; content:"SECRET_EXPLOIT_RESPONSE_MARKER"; sid:5002; rev:1;)`)
	require.NoError(t, err)
	mFD, err := CompileRule(rFD[0])
	require.NoError(t, err)
	ok, err = mFD.MatchStream(pkZl, zlResp)
	require.NoError(t, err)
	require.True(t, ok)

	// 3. Corrupt gzip falls back cleanly to raw body without error.
	badResp := []byte("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 12\r\n\r\nRAW_FALLBACK")
	pkBad := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 52002, badResp)
	rRaw, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"corrupt gzip fallback"; http.response_body; content:"RAW_FALLBACK"; sid:5003; rev:1;)`)
	require.NoError(t, err)
	mRaw, err := CompileRule(rRaw[0])
	require.NoError(t, err)
	ok, err = mRaw.MatchStream(pkBad, badResp)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestSSHSoftwareByteMathAndAppLayerProtocols(t *testing.T) {
	// 1. ssh.software extraction (Erlang/OTP CVE-2025-32433 style).
	sshBanner := []byte("SSH-2.0-Erlang/5.2.0\r\n")
	pkSSH := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 22, 53000, sshBanner)
	rSSH, err := rule.ParseStrictRule(`alert ssh any any -> any any (msg:"erlang ssh"; ssh.software; content:"Erlang/"; pcre:"/^5\.2\.0/R"; sid:5101; rev:1;)`)
	require.NoError(t, err)
	mSSH, err := CompileRule(rSSH[0])
	require.NoError(t, err)
	ok, err := mSSH.MatchStream(pkSSH, sshBanner)
	require.NoError(t, err)
	require.True(t, ok)

	// 2. byte_math + isdataat:!variable (ETPro implant.js style).
	// Payload: 0xa1 + uint32_be(4) + 5 bytes => length = 4 + 6 = 10; len(payload) == 10 => !isdataat(10) is true!
	bmPayloadMatch := []byte{0xa1, 0x00, 0x00, 0x00, 0x04, 'a', 'b', 'c', 'd', 'e'}
	bmPayloadTooLong := append(append([]byte(nil), bmPayloadMatch...), 'X')
	rBM, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"byte_math"; content:"|a1|"; startswith; byte_math:bytes 4, offset 0, oper +, rvalue 6, result length, relative; isdataat:!length; sid:5102; rev:1;)`)
	require.NoError(t, err)
	mBM, err := CompileRule(rBM[0])
	require.NoError(t, err)
	ok, err = mBM.MatchStream(buildTestTCPPacket(t, "10.0.0.10", "198.51.100.10", 53001, 4444, bmPayloadMatch), bmPayloadMatch)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = mBM.MatchStream(buildTestTCPPacket(t, "10.0.0.10", "198.51.100.10", 53001, 4444, bmPayloadTooLong), bmPayloadTooLong)
	require.NoError(t, err)
	require.False(t, ok)

	// 3. app-layer-protocol:telnet and app-layer-protocol:!tls.
	telnetPayload := []byte{0xff, 0xfd, 0x18, 'r', 'u', 'n', 'C', 'm', 'd'}
	pkTelnet := buildTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 53002, 23, telnetPayload)
	rTelnet, err := rule.ParseStrictRule(`alert tcp any any -> any 23 (msg:"telnet"; app-layer-protocol:telnet; content:"runCmd"; sid:5103; rev:1;)`)
	require.NoError(t, err)
	mTelnet, err := CompileRule(rTelnet[0])
	require.NoError(t, err)
	ok, err = mTelnet.MatchStream(pkTelnet, telnetPayload)
	require.NoError(t, err)
	require.True(t, ok)

	rNotTLS, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"not tls"; app-layer-protocol:!tls; content:"runCmd"; sid:5104; rev:1;)`)
	require.NoError(t, err)
	mNotTLS, err := CompileRule(rNotTLS[0])
	require.NoError(t, err)
	ok, err = mNotTLS.MatchStream(pkTelnet, telnetPayload)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestMultiCertTLSChainMatching(t *testing.T) {
	// Build a minimal TLS 1.2 Certificate handshake containing 2 DER-prefixed certs.
	cert1 := append([]byte{0x30, 0x10}, []byte("LEAF-CERT-MARKER")...)
	cert2 := append([]byte{0x30, 0x18}, []byte("INTERMEDIATE-CA-EVIL-SIG")...)

	var certList []byte
	for _, cert := range [][]byte{cert1, cert2} {
		certList = append(certList, byte(len(cert)>>16), byte(len(cert)>>8), byte(len(cert)))
		certList = append(certList, cert...)
	}
	var body []byte
	body = append(body, byte(len(certList)>>16), byte(len(certList)>>8), byte(len(certList)))
	body = append(body, certList...)

	hs := make([]byte, 4+len(body))
	hs[0] = 11 // Certificate
	hs[1] = byte(len(body) >> 16)
	hs[2] = byte(len(body) >> 8)
	hs[3] = byte(len(body))
	copy(hs[4:], body)

	rec := make([]byte, 5+len(hs))
	rec[0] = 22
	rec[1] = 0x03
	rec[2] = 0x03
	binary.BigEndian.PutUint16(rec[3:5], uint16(len(hs)))
	copy(rec[5:], hs)

	pk := buildTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 443, 54000, rec)
	rCert, err := rule.ParseStrictRule(`alert tls any any -> any any (msg:"intermediate cert"; tls.certs; content:"INTERMEDIATE-CA-EVIL-SIG"; sid:5201; rev:1;)`)
	require.NoError(t, err)
	mCert, err := CompileRule(rCert[0])
	require.NoError(t, err)
	ok, err := mCert.MatchStream(pk, rec)
	require.NoError(t, err)
	require.True(t, ok)
}
