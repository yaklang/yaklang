package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

type m1Sample struct {
	name     string
	protocol string
	port     layers.TCPPort
	steps    []sessionStep
}

func m1SessionSamples(t testing.TB) []m1Sample {
	t.Helper()
	startup := pgSessionUntyped(196608, []byte("user\x00test\x00database\x00demo\x00\x00"))
	bindBody := append([]byte{2, 1, 3}, ldapSessionTLV(4, nil)...)
	bindBody = append(bindBody, 0x80, 0)
	wsReq := []byte("GET /chat HTTP/1.1\r\nHost: example.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")
	wsResp := []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	msg := make([]byte, 9)
	binary.BigEndian.PutUint32(msg[1:5], 4)
	copy(msg[5:], []byte{1, 2, 3, 4})
	headers := h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/svc.P/M", ":authority", "example.test", "content-type", "application/grpc")
	resp := h2TestHeaders(t, ":status", "200", "content-type", "application/grpc")
	trailers := h2TestHeaders(t, "grpc-status", "0")
	return []m1Sample{
		{"postgres-extended-query", "postgresql", 15432, []sessionStep{
			{0, startup},
			{1, pgSessionMsg('R', []byte{0, 0, 0, 0})},
			{1, pgSessionMsg('Z', []byte{'I'})},
			{0, pgSessionMsg('Q', append([]byte("SELECT 1"), 0))},
			{1, pgSessionMsg('C', append([]byte("SELECT 1"), 0))},
			{1, pgSessionMsg('Z', []byte{'I'})},
		}},
		{"ldap-bind-search", "ldap", 14389, []sessionStep{
			{0, ldapSessionMsg(0x60, bindBody)},
			{1, ldapSessionMsg(0x61, []byte{10, 1, 0, 4, 0, 4, 0})},
			{0, mustHexSession(t, "301c020101631704000a01020a01000201000201000101008702636e3000")},
			{1, ldapSessionMsg(0x64, append(ldapSessionTLV(4, []byte("cn=x")), 0x30, 0))},
			{1, ldapSessionMsg(0x65, []byte{10, 1, 0, 4, 0, 4, 0})},
		}},
		{"websocket-upgrade-text", "websocket", 18090, []sessionStep{
			{0, wsReq}, {1, wsResp}, {1, []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}},
		}},
		{"redis-resp2-resp3", "redis", 16379, []sessionStep{
			{0, []byte("*1\r\n$4\r\nPING\r\n")},
			{1, []byte("+PONG\r\n")},
			{1, []byte("%1\r\n+key\r\n+val\r\n")},
		}},
		{"kafka-apiversions-metadata", "kafka", 19092, []sessionStep{
			{0, kafkaRequest(18, 0, 1, "", nil)},
			{1, kafkaResponse(1, append(append(kafkaBE16(0), kafkaBE32(0)...)))},
			{0, kafkaRequest(3, 0, 2, "test", append(kafkaBE32(1), kafkaStr("foo")...))},
		}},
		{"tds-prelogin-login-batch", "tds", 11433, []sessionStep{
			{0, tdsPrelogin(2)},
			{1, tdsPreloginReply(2)},
			{0, tdsLogin7()},
			{1, tdsDone72()},
			{0, tdsSQLBatch72("SELECT 1")},
			{1, tdsQueryResult72()},
		}},
		{"amqp-publish-deliver", "amqp", 15672, []sessionStep{
			{0, amqpProto()},
			{1, amqpStart()},
			{0, amqpStartOK()},
			{0, amqpChannelOpen(1)},
			{1, amqpChannelOpenOK(1)},
			{0, amqpPublish(1, "", "rk")},
			{0, amqpHeader(1, 5)},
			{0, amqpBody(1, []byte("hello"))},
			{1, amqpDeliver(1, 1, "rk")},
			{1, amqpHeader(1, 1)},
			{1, amqpBody(1, []byte("x"))},
			{0, amqpAck(1, 1)},
		}},
		{"smb2-negotiate-create", "smb2", 1445, []sessionStep{
			{0, smb2NegotiateReq(0x0202, 0x0311)},
			{1, smb2NegotiateResp(0x0311)},
			{0, smb2SessionSetup(2, false)},
			{1, smb2SessionSetup(2, true)},
			{0, smb2TreeConnect(3, 0x11, `\\srv\share`)},
			{1, smb2TreeConnectResp(3, 0x11, 1)},
			{0, smb2Create(4, 0x11, 1, "file.txt")},
			{1, smb2CreateResp(4, 0x11, 1, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})},
		}},
		{"dcerpc-epm-srvsvc", "dcerpc", 13500, []sessionStep{
			{0, dcerpcBind(dcerpcEPM, 0, 1)},
			{1, dcerpcBindAck(1)},
			{0, dcerpcRequest(2, 0, 3, []byte{1, 2, 3, 4})},
			{1, dcerpcResponse(2, 0, []byte{9, 9})},
			{0, dcerpcBind(dcerpcSRVSVC, 1, 3)},
			{1, dcerpcBindAck(3)},
			{0, dcerpcRequest(4, 1, 15, []byte{0, 0, 0, 0})},
			{1, dcerpcResponse(4, 1, nil)},
		}},
		{"ssh-kex-newkeys", "ssh", 10022, []sessionStep{
			{0, sshIdent("SSH-2.0-OpenSSH_8.9")},
			{1, sshIdent("SSH-2.0-sshd")},
			{0, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")},
			{1, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")},
			{0, sshKexDHInit()},
			{1, sshKexDHReply()},
			{0, sshNewKeys()},
			{1, sshNewKeys()},
		}},
		{"nfsv3-lookup-read", "nfs", 12049, []sessionStep{
			{0, nfsLookupCall(1, "foo")},
			{1, nfsLookupOK(1)},
			{0, nfsGetattrCall(2)},
			{1, nfsGetattrOK(2, 42)},
			{0, nfsReadCall(3, 0, 4)},
			{1, nfsReadOK(3, []byte("abcd"), true)},
			{0, nfsWriteCall(4, 8, []byte("efgh"))},
			{1, nfsWriteOK(4, 4)},
		}},
		{"snmpv3-get-response", "snmp", 1161, []sessionStep{
			{0, snmpGet(1)},
			{1, snmpGetResponse(1, "router")},
			{0, snmpGetBulk(2, 0, 10)},
			{1, snmpGetResponse(2, "bulk")},
			{0, snmpSet(3, "name")},
			{1, snmpGetResponse(3, "name")},
			{1, snmpTrap()},
			{0, snmpInform(4)},
			{1, snmpGetResponse(4, "ack")},
		}},
		{"rdp-tpkt-negotiate-mcs", "rdp", 13389, []sessionStep{
			{0, rdpCR("eltons", rdpProtoRDP)},
			{1, rdpCC(rdpNegRsp, rdpProtoRDP)},
			{0, rdpConnectInitial("rdpdr", "cliprdr")},
			{1, rdpConnectResponse("rdpdr", "cliprdr")},
		}},
		{"dot-dns-tcp-length", "dot", 1853, []sessionStep{
			{0, dnsQuery(0x1234, "example.com", 1)},
			{1, dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34})},
			{0, dnsQuery(0x22, "ietf.org", 1)},
			{1, dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44})},
		}},
		{"doh-http-get-post", "doh", 18443, []sessionStep{
			{0, dohPOST("dns.example.test", dnsWire(dnsQuery(0x1234, "example.com", 1)))},
			{1, dohHTTPResp(200, dnsWire(dnsAResponse(0x1234, "example.com", [4]byte{93, 184, 216, 34})))},
			{0, dohGET("dns.example.test", "ietf.org", 0x22)},
			{1, dohHTTPResp(200, dnsWire(dnsAResponse(0x22, "ietf.org", [4]byte{4, 31, 198, 44})))},
		}},
		{"rtp-seq-sr-rr", "rtp", 15004, []sessionStep{
			{0, rtpPkt(0, 1, 0, 0x12345678)},
			{0, rtpPkt(0, 2, 160, 0x12345678)},
			{0, rtpPkt(0, 3, 320, 0x12345678)},
			{1, rtcpSR(0x12345678, 0x11121418, 320, 3, 48)},
			{1, rtcpRR(0xabcdef01, 0x12345678, 0, 0)},
		}},
		{"quic-v1-rfc9001-initial", "quic", 14443, []sessionStep{
			{0, rfc9001ClientInitial()},
			{1, rfc9001ServerInitial()},
		}},
		{"quic-v1-crypto-stream", "quic", 14443, []sessionStep{
			{0, quicLongPacket(0, 1, []byte{8, 3, 9, 4, 0xc8, 0xf0, 0x3e, 0x51}, nil, nil, 0, quicCryptoFrame(0, []byte("CHLO")))},
			{1, quicLongPacket(2, 1, []byte{8, 3, 9, 4, 0xc8, 0xf0, 0x3e, 0x51}, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, nil, 0, quicCryptoFrame(0, []byte("SHLO")))},
			{0, quicLongPacket(1, 1, []byte{8, 3, 9, 4, 0xc8, 0xf0, 0x3e, 0x51}, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, nil, 0, quicStreamFrame(0, 0, true, []byte("GET")))},
			{1, quicLongPacket(2, 1, []byte{8, 3, 9, 4, 0xc8, 0xf0, 0x3e, 0x51}, []byte{0xf0, 0x67, 0xa5, 0x50, 0x2a, 0x42, 0x62, 0xb5}, nil, 1, quicConnectionClose(0, "done"))},
		}},
		{"sip-invite-ack-bye", "sip", 15060, []sessionStep{
			{0, sipInvite()},
			{1, sipResp("100", "Trying", "z9hG4bK776asdhds", "314159 INVITE", "a84b4c76e66710@pc33.atlanta.example.com", "Alice <sip:alice@atlanta.example.com>;tag=1928301774", "Bob <sip:bob@biloxi.example.com>")},
			{1, sipResp("200", "OK", "z9hG4bK776asdhds", "314159 INVITE", "a84b4c76e66710@pc33.atlanta.example.com", "Alice <sip:alice@atlanta.example.com>;tag=1928301774", "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf")},
			{0, sipMsg("ACK sip:bob@biloxi.example.com SIP/2.0", [][2]string{
				{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKack"},
				{"From", "Alice <sip:alice@atlanta.example.com>;tag=1928301774"},
				{"To", "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf"},
				{"Call-ID", "a84b4c76e66710@pc33.atlanta.example.com"},
				{"CSeq", "314159 ACK"},
			}, "")},
			{0, sipMsg("BYE sip:bob@biloxi.example.com SIP/2.0", [][2]string{
				{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKbye"},
				{"From", "Alice <sip:alice@atlanta.example.com>;tag=1928301774"},
				{"To", "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf"},
				{"Call-ID", "a84b4c76e66710@pc33.atlanta.example.com"},
				{"CSeq", "314160 BYE"},
			}, "")},
			{1, sipResp("200", "OK", "z9hG4bKbye", "314160 BYE", "a84b4c76e66710@pc33.atlanta.example.com", "Alice <sip:alice@atlanta.example.com>;tag=1928301774", "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf")},
		}},
		{"mongodb-opmsg-compressed", "mongodb", 27018, []sessionStep{
			{0, mongoOpMsg(1, 0, 0, mongoKind0(mongoBSONInt32("ping", 1)))},
			{1, mongoOpMsg(2, 1, 0, mongoKind0(mongoBSONInt32("ok", 1)))},
		}},
		{"mqtt5-connect-qos", "mqtt", 18830, []sessionStep{
			{0, mqtt5Connect("dev", []byte{3, 0x22, 0, 10})},
			{1, mqtt5Connack(10)},
			{0, mqttPkt(0x32, append(append(append(mqttUTF("a/b"), 0, 1), 0), []byte("x")...))},
			{1, mqttPkt(0x40, []byte{0, 1, 0, 0})},
		}},
		{"grpc-unary-http2", "http2", 18081, []sessionStep{
			{0, append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)},
			{1, h2TestFrame(4, 0, 0, nil)},
			{0, h2TestFrame(4, 1, 0, nil)},
			{1, h2TestFrame(4, 1, 0, nil)},
			{0, h2TestFrame(1, 4, 1, headers)},
			{0, h2TestFrame(0, 1, 1, msg)},
			{1, h2TestFrame(1, 4, 1, resp)},
			{1, h2TestFrame(1, 5, 1, trailers)},
		}},
	}
}

func TestProtocolSessionM1PCAP(t *testing.T) {
	dir := filepath.Join("testdata", "protocol-sessions")
	manifestPath := filepath.Join(dir, "m1-manifest.json")
	type row struct {
		File     string `json:"file"`
		Protocol string `json:"protocol"`
		SHA256   string `json:"sha256"`
		Bytes    int    `json:"bytes"`
		Port     int    `json:"decode_as_port"`
		Kind     string `json:"kind"`
		Source   string `json:"source"`
	}
	doc := struct {
		Schema    int    `json:"schema_version"`
		Generator string `json:"generator"`
		Reproduce string `json:"reproduce"`
		Samples   []row  `json:"samples"`
	}{
		Schema:    1,
		Generator: "protocol_session_samples_test.go:TestProtocolSessionM1PCAP",
		Reproduce: "YAK_UPDATE_SESSION_PCAP=1 go test ./common/pcapx/pcaputil -run ^TestProtocolSessionM1PCAP$ -count=1",
	}
	for _, sample := range m1SessionSamples(t) {
		golden := sessionTestPCAP(t, sample.steps, sample.port, 0, false, false)
		sum := sha256.Sum256(golden)
		path := filepath.Join(dir, sample.name+".pcap")
		if os.Getenv("YAK_UPDATE_SESSION_PCAP") == "1" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			require.NoError(t, os.WriteFile(path, golden, 0644))
		}
		stored, err := os.ReadFile(path)
		require.NoError(t, err, sample.name)
		require.Equal(t, hex.EncodeToString(golden), hex.EncodeToString(stored), sample.name)
		events, stats, err := binReplay(t, stored, 1)
		require.NoError(t, err, sample.name)
		require.Zero(t, stats.Malformed, sample.name)
		found := false
		for _, e := range events {
			if e.Protocol == sample.protocol || sample.protocol == "http2" && e.Protocol == "grpc" {
				found = true
			}
		}
		require.True(t, found, "%s: no %s events in %d", sample.name, sample.protocol, len(events))
		doc.Samples = append(doc.Samples, row{
			File: sample.name + ".pcap", Protocol: sample.protocol,
			SHA256: hex.EncodeToString(sum[:]), Bytes: len(golden),
			Port: int(sample.port), Kind: "deterministic-generated-not-real-capture",
			Source: "constructed Ethernet+IPv4+TCP from protocol_session_samples_test.go",
		})
	}
	if os.Getenv("YAK_UPDATE_SESSION_PCAP") == "1" {
		raw, err := json.MarshalIndent(doc, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(manifestPath, append(raw, '\n'), 0644))
	}
	stored, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.Contains(t, string(stored), doc.Samples[0].SHA256)
}
