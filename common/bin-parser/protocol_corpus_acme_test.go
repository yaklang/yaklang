package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const acmeCorpusRule = "application-layer.acme"
const acmeCorpusProtected = `{"alg":"ES256","nonce":"AQIDBA","url":"https://acme.example/acme/order/1","kid":"https://acme.example/acme/account/1"}`

func acmeTestBody(payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	// Independent serialized structure; the zero signature is NOT a verified
	// signature or working request. It supplies opaque signature-field bytes.
	return fmt.Sprintf(`{"protected":%q,"payload":%q,"signature":%q}`, enc([]byte(acmeCorpusProtected)), enc([]byte(payload)), enc(make([]byte, 64)))
}

func acmeTestHTTP(body string) []byte {
	return []byte(fmt.Sprintf("POST /acme/order/1 HTTP/1.1\r\nHost: acme.example\r\nUser-Agent: binparser-fixture/1\r\nContent-Type: application/jose+json\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
}

func acmeTestFields(t *testing.T, wire []byte, n *base.Node, payload string) {
	t.Helper()
	require.Equal(t, wire, NodeToBytes(n))
	protocolCorpusRequireValue(t, n, "Method", "POST")
	protocolCorpusRequireValue(t, n, "Path", "/acme/order/1")
	protocolCorpusRequireValue(t, n, "Version", "HTTP/1.1")
	protocolCorpusRequireValue(t, n, "Octets", acmeTestBody(payload))
	// HTTP and decoded values use separate coordinate systems. HTTP remains
	// a complete byte-exact tree; no base64-decoded child has a wire span.
	gssapiTestHTTPWireTree(t, protocolCorpusFindNode(n, "HTTP"), 0, uint64(len(wire))*8)
	info := gssapiTestInfo(t, n, "ACME JWS")
	require.Equal(t, true, info["HTTP Target Matches Protected Path"])
	require.Equal(t, false, info["User-Agent Requirement Missing"])
	decoded := info["ACME JWS"].(map[string]any)
	require.Equal(t, "ES256", decoded["Algorithm"])
	require.Equal(t, []byte(acmeCorpusProtected), decoded["Protected Bytes"])
	require.Equal(t, []byte(payload), decoded["Payload Bytes"])
	require.Equal(t, []byte{1, 2, 3, 4}, decoded["Nonce Bytes"])
	require.Equal(t, make([]byte, 64), decoded["Signature Bytes"])
	require.Equal(t, payload == "", decoded["POST-as-GET"])
	for _, key := range []string{"Signature Verified", "Nonce Freshness Verified", "Public Key Validated", "Account Validated", "Server URL Equality Verified", "TLS Observed", "Operation Schema Parsed", "Exchange Observed", "Decoded Values Are Wire Spans"} {
		require.Equal(t, false, decoded[key], key)
	}
}

func TestProtocolCorpusACMEFields(t *testing.T) {
	for _, payload := range []string{"", `{}`, `{"identifiers":[{"type":"dns","value":"example.test"}]}`} {
		wire := acmeTestHTTP(acmeTestBody(payload))
		for _, entry := range []string{"ACMEJWSRequest", "ACMEJWSRequestCarrier"} {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, acmeCorpusRule, entry)
			acmeTestFields(t, wire, n, payload)
		}
	}
}

func acmeTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), acmeCorpusRule, "ACMEJWSRequest")
	require.Error(t, err)
	if len(wire) == 0 {
		return
	}
	n := protocolCorpusRequireBoundedRuleParse(t, wire, acmeCorpusRule, "ACMEJWSRequestCarrier")
	protocolCorpusRequireValue(t, n, "Unparsed ACME Request", wire)
	require.Nil(t, protocolCorpusFindNode(n, "Method"))
	require.Equal(t, wire, NodeToBytes(n))
}

func TestProtocolCorpusACMEOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-acme.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "5ee4ea551eba90a260e4762da8b385e6251c27f71637cfc84cd5067a61d0b279", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		p := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		literal := []byte("POST /acme/new-nonce HTTP/1.1\r\nHost: acme.lab\r\nContent-Type: application/jose+json\r\nContent-Length: 2\r\n\r\n{}")
		require.Equal(t, literal, tcp.Payload)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), acmeCorpusRule, "ACMEJWSRequest")
		require.ErrorContains(t, err, "exactly protected, payload and signature members")
		acmeTestReject(t, tcp.Payload)
	}
}

func TestProtocolCorpusACMECompanionRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-acme-jws-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "f0a477267467242f2356fe955b0306bc23bcbea39ac58110426ae1005dfb269a", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	values := []string{"", `{}`, `{"identifiers":[{"type":"dns","value":"example.test"}]}`}
	require.Len(t, frames, len(values))
	for i, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.Equal(t, acmeTestHTTP(acmeTestBody(values[i])), tcp.Payload)
		require.EqualValues(t, 443, tcp.DstPort)
		for _, entry := range []string{"ACMEJWSRequest", "ACMEJWSRequestCarrier"} {
			n := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, acmeCorpusRule, entry)
			acmeTestFields(t, tcp.Payload, n, values[i])
		}
	}
}

func TestProtocolCorpusACMEBoundaries(t *testing.T) {
	valid := acmeTestHTTP(acmeTestBody(`{}`))
	invalid := [][]byte{
		acmeTestHTTP(`{}`), append(bytes.Clone(valid), '!'),
		bytes.Replace(valid, []byte("POST "), []byte("GET "), 1),
		bytes.Replace(valid, []byte("HTTP/1.1"), []byte("HTTP/1.0"), 1),
		bytes.Replace(valid, []byte("Host: acme.example\r\n"), nil, 1),
		bytes.Replace(valid, []byte("Host: acme.example\r\n"), []byte("Host: a\r\nHost: b\r\n"), 1),
		bytes.Replace(valid, []byte("application/jose+json"), []byte("application/json"), 1),
		bytes.Replace(valid, []byte("Content-Type:"), []byte("Content-Encoding: gzip\r\nContent-Type:"), 1),
		bytes.Replace(valid, []byte("Host: acme.example"), []byte("Host: [not-ip]"), 1),
		bytes.Replace(valid, []byte("Host: acme.example"), []byte("Host: acme.example/path"), 1),
		bytes.Replace(valid, []byte("/acme/order/1"), []byte("/acme/\\path"), 1),
		[]byte(strings.Repeat("x", 278529)),
	}
	for _, wire := range invalid {
		acmeTestReject(t, wire)
	}
	for cut := 0; cut < len(valid); cut++ {
		acmeTestReject(t, valid[:cut])
	}
	for _, wire := range [][]byte{valid, acmeTestHTTP(`{}`)} {
		root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Record:\n    import: application-layer/acme.yaml\n    node: ACMEJWSRequestCarrier\n    length: %d\n", len(wire)*8))
		reader := base.NewBitReader(bytes.NewReader(append(bytes.Clone(wire), 0x7e, 0x51)))
		require.NoError(t, root.ParseSubNode(reader, "Record"))
		require.Equal(t, wire, NodeToBytes(base.GetNodeByPath(root, "@Record")))
		suffix, err := reader.ReadBits(16)
		require.NoError(t, err)
		require.Equal(t, []byte{0x7e, 0x51}, suffix)
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
}
