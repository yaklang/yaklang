package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const gssapiTestRule = "application-layer.gssapi"

// Independently constructed DER: Kerberos and NTLM OIDs identify offered
// mechanisms, not decoded mechanism messages. Token/MIC literal bodies are
// intentionally uninterpreted structural values, not working exchanges.
var gssapiTestFixtures = []struct{ name, hex, kind string }{
	{"initial-minimal", "601b06062b0601050502a011300fa00d300b06092a864886f712010202", "NegTokenInit"},
	{"initial-two-mechanisms", "602706062b0601050502a01d301ba019301706092a864886f712010202060a2b06010401823702020a", "NegTokenInit"},
	{"initial-optionals", "602e06062b0601050502a0243022a00d300b06092a864886f712010202a10403020640a2050403616263a3040402dead", "NegTokenInit"},
	{"response-optionals", "a121301fa0030a0101a10b06092a864886f712010202a2050403010203a3040402beef", "NegTokenResp"},
	{"response-completed-declaration", "a1073005a0030a0100", "NegTokenResp"},
	{"response-reject-declaration", "a1073005a0030a0102", "NegTokenResp"},
	{"response-request-mic-declaration", "a1073005a0030a0103", "NegTokenResp"},
	{"response-no-optionals", "a1023000", "NegTokenResp"},
	{"response-empty-octets", "a1063004a2020400", "NegTokenResp"},
}

func gssapiTestWire(t *testing.T, index int) []byte {
	t.Helper()
	wire, err := hex.DecodeString(gssapiTestFixtures[index].hex)
	require.NoError(t, err)
	return wire
}

func gssapiTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, gssapiTestRule, entry)
}

func gssapiTestInfo(t *testing.T, n *base.Node, key string) map[string]any {
	t.Helper()
	var found map[string]any
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if info, ok := n.Cfg.GetItem("additionInfo").(map[string]any); ok {
			if _, ok := info[key]; ok {
				found = info
				return
			}
		}
		for _, c := range n.Children {
			if found == nil {
				walk(c)
			}
		}
	}
	walk(n)
	require.NotNil(t, found, key)
	return found
}

func gssapiTestHTTP(index int, response bool) []byte {
	wire, _ := hex.DecodeString(gssapiTestFixtures[index].hex)
	encoded := base64.StdEncoding.EncodeToString(wire)
	if response {
		return []byte("HTTP/1.1 200 OK\r\nWWW-Authenticate: Negotiate " + encoded + "\r\nContent-Length: 0\r\n\r\n")
	}
	return []byte("GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate " + encoded + "\r\n\r\n")
}

func TestProtocolCorpusGSSAPIFields(t *testing.T) {
	for i, fixture := range gssapiTestFixtures {
		for _, entry := range []string{"GSSAPIToken", "GSSAPICarrier"} {
			t.Run(fixture.name+"/"+entry, func(t *testing.T) {
				wire := gssapiTestWire(t, i)
				n := gssapiTestParse(t, wire, entry)
				require.Equal(t, wire, NodeToBytes(n))
				info := gssapiTestInfo(t, n, "Token Kind")
				require.Equal(t, fixture.kind, info["Token Kind"])
				require.Equal(t, "1.3.6.1.5.5.2", info["Mechanism OID"])
				require.Equal(t, fixture.kind == "NegTokenInit", info["Outer Mechanism OID Present"])
				for _, key := range []string{"Mechanism Semantics Decoded", "MIC Verified", "Context State Validated", "Negotiation Outcome Validated"} {
					require.Equal(t, false, info[key], key)
				}
				name := "NegTokenResp"
				if fixture.kind == "NegTokenInit" {
					name = "Initial Context Token"
				}
				megacoTestTree(t, protocolCorpusFindNode(n, name), 0, uint64(len(wire))*8)
			})
		}
	}
	n := gssapiTestParse(t, gssapiTestWire(t, 0), "GSSAPIToken")
	latTestField(t, n, "Tag", "uint8", 0, 8, uint64(0x60))
	latTestField(t, n, "DER Length", "raw", 8, 16, []byte{27})
	latTestField(t, n, "OID Bytes", "raw", 32, 80, []byte{0x2b, 6, 1, 5, 5, 2})
	oid := protocolCorpusFindNode(n, "Mechanism Type 0")
	require.Equal(t, "1.2.840.113554.1.2.2", oid.Cfg.GetItem("additionInfo").(map[string]any)["OID"])
	latTestField(t, oid, "OID Bytes", "raw", 160, 232, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 1, 2, 2})
	n = gssapiTestParse(t, gssapiTestWire(t, 1), "GSSAPIToken")
	require.Equal(t, []string{"1.2.840.113554.1.2.2", "1.3.6.1.4.1.311.2.2.10"}, gssapiTestInfo(t, n, "Token Kind")["Offered Mechanisms"])
	n = gssapiTestParse(t, gssapiTestWire(t, 2), "GSSAPIToken")
	flags := gssapiTestInfo(t, n, "Token Kind")["Request Flags"].(map[string]any)
	require.Equal(t, true, flags["Mutual"])
	require.Equal(t, false, flags["Delegation"])
	require.Equal(t, 2, flags["Meaningful Bits"])
	latTestField(t, protocolCorpusFindNode(n, "Optimistic Mechanism Token"), "Value", "raw", 312, 336, []byte("abc"))
	latTestField(t, protocolCorpusFindNode(n, "Mechanism List MIC"), "Value", "raw", 368, 384, []byte{0xde, 0xad})
	n = gssapiTestParse(t, gssapiTestWire(t, 3), "GSSAPIToken")
	latTestField(t, protocolCorpusFindNode(n, "Negotiation State"), "Value", "uint8", 64, 72, uint64(1))
	require.Equal(t, "1.2.840.113554.1.2.2", gssapiTestInfo(t, n, "Token Kind")["Selected Mechanism"])
}

func gssapiTestDecodedTree(t *testing.T, fields []any, wire []byte, start, end uint64) {
	t.Helper()
	position := start
	for _, raw := range fields {
		f := raw.(map[string]any)
		a, b := f["Start Bit"].(uint64), f["End Bit"].(uint64)
		require.Equal(t, position, a)
		require.GreaterOrEqual(t, b, a)
		require.LessOrEqual(t, b, end)
		switch f["Type"] {
		case "":
			gssapiTestDecodedTree(t, f["Children"].([]any), wire, a, b)
		case "raw":
			require.Equal(t, wire[a/8:b/8], f["Value"])
		case "uint8":
			require.Equal(t, uint64(wire[a/8]), f["Value"])
		default:
			t.Fatalf("unknown decoded type %v", f["Type"])
		}
		position = b
	}
	require.Equal(t, end, position)
}

// HTTP's choice definitions include an unselected branch. Audit every actual
// Result child and wire leaf without treating that definition as parsed data.
func gssapiTestHTTPWireTree(t *testing.T, n *base.Node, start, end uint64) {
	t.Helper()
	value, err := n.Result()
	require.NoError(t, err)
	require.Same(t, n, value.Origin)
	var physical, results []*base.Node
	var nodes func(*base.Node)
	nodes = func(n *base.Node) {
		if stream_parser.NodeHasResult(n) {
			physical = append(physical, n)
			return
		}
		for _, child := range n.Children {
			require.Same(t, n, child.Cfg.GetItem(base.CfgParent))
			nodes(child)
		}
	}
	nodes(n)
	var values func(*base.NodeValue)
	values = func(v *base.NodeValue) {
		require.NotNil(t, v.Origin)
		if stream_parser.NodeHasResult(v.Origin) {
			results = append(results, v.Origin)
			return
		}
		for _, child := range v.Children() {
			values(child)
		}
	}
	values(value)
	// Result unpacks FirstLine into Method/Path/Version; compare all leaves
	// against physical nodes, not an invented one-level parent relationship.
	require.Len(t, results, len(physical))
	position := start
	for i, leaf := range physical {
		require.Same(t, leaf, results[i])
		span := stream_parser.GetNodeResultPos(leaf)
		require.Equal(t, position, span[0], leaf.Name)
		require.LessOrEqual(t, span[1], end)
		delimiter := leaf.Cfg.GetString(base.CfgDelimiter)
		if delimiter == "" {
			delimiter = leaf.Cfg.GetString(base.CfgDel)
		}
		// String value spans intentionally exclude their consumed delimiter.
		consumed := stream_parser.CalcNodeConsumedLength(leaf)
		require.Equal(t, span[1]-span[0]+uint64(len(delimiter))*8, consumed, leaf.Name)
		position += consumed
	}
	require.Equal(t, end, position, n.Name)
}

func TestProtocolCorpusGSSAPIHTTPFields(t *testing.T) {
	for i, f := range gssapiTestFixtures {
		for _, response := range []bool{false, true} {
			if response && f.kind == "NegTokenInit" {
				continue
			}
			for _, entry := range []string{"GSSAPIHTTP", "GSSAPIHTTPCarrier"} {
				t.Run(fmt.Sprintf("%s/%t/%s", f.name, response, entry), func(t *testing.T) {
					wire := gssapiTestHTTP(i, response)
					n := gssapiTestParse(t, wire, entry)
					require.Equal(t, wire, NodeToBytes(n))
					info := gssapiTestInfo(t, n, "Token Present")
					require.Equal(t, true, info["Token Fields Decoded"])
					decoded := info["Decoded Token"].(map[string]any)
					require.Equal(t, "base64", decoded["Source Encoding"])
					require.Equal(t, "decoded-token-relative-bits", decoded["Span Coordinate System"])
					require.Equal(t, f.kind, decoded["Token Kind"])
					token := gssapiTestWire(t, i)
					require.Equal(t, token, decoded["Decoded Bytes"])
					gssapiTestDecodedTree(t, decoded["Decoded Fields"].([]any), token, 0, uint64(len(token))*8)
					require.Nil(t, protocolCorpusFindNode(n, "Mechanism OID"), "decoded bytes must not impersonate HTTP wire leaves")
					require.Nil(t, protocolCorpusFindNode(n, "NegTokenResp"))
					if response {
						protocolCorpusRequireValue(t, n, "Status", "200")
					} else {
						protocolCorpusRequireValue(t, n, "Method", "GET")
					}
					gssapiTestHTTPWireTree(t, protocolCorpusFindNode(n, "HTTP"), 0, uint64(len(wire))*8)
				})
			}
		}
	}
	challenge := []byte("HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Negotiate\r\nContent-Length: 0\r\n\r\n")
	n := gssapiTestParse(t, challenge, "GSSAPIHTTP")
	require.Equal(t, challenge, NodeToBytes(n))
	info := gssapiTestInfo(t, n, "Token Present")
	require.Equal(t, false, info["Token Fields Decoded"])
	require.Equal(t, false, info["Token Present"])
	require.NotContains(t, info, "Decoded Token")
	lower := bytes.ReplaceAll(gssapiTestHTTP(0, false), []byte("Authorization: Negotiate"), []byte("aUtHoRiZaTiOn: nEgOtIaTe   "))
	require.Equal(t, lower, NodeToBytes(gssapiTestParse(t, lower, "GSSAPIHTTP")))
	body := bytes.Replace(gssapiTestHTTP(0, false), []byte("\r\n\r\n"), []byte("\r\nContent-Length: 3\r\n\r\nxyz"), 1)
	n = gssapiTestParse(t, body, "GSSAPIHTTP")
	require.Equal(t, body, NodeToBytes(n))
	require.Equal(t, false, gssapiTestInfo(t, n, "Token Present")["HTTP Body Semantics Decoded"])
}

func gssapiTestReject(t *testing.T, wire []byte, http bool, diagnostic string) {
	t.Helper()
	entry, carrier, raw := "GSSAPIToken", "GSSAPICarrier", "Unparsed GSS-API Token"
	if http {
		entry, carrier, raw = "GSSAPIHTTP", "GSSAPIHTTPCarrier", "Unparsed GSS-API HTTP Message"
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), gssapiTestRule, entry)
	require.ErrorContains(t, err, diagnostic)
	if len(wire) > 0 {
		n := gssapiTestParse(t, wire, carrier)
		require.Equal(t, wire, NodeToBytes(n))
		latTestField(t, n, raw, "raw", 0, uint64(len(wire))*8, wire)
		require.Nil(t, protocolCorpusFindNode(n, "Mechanism OID"))
		require.Nil(t, protocolCorpusFindNode(n, "Method"))
	}
}

func TestProtocolCorpusGSSAPIPrefixesAndNegatives(t *testing.T) {
	for i, f := range gssapiTestFixtures {
		wire := gssapiTestWire(t, i)
		for cut := 0; cut < len(wire); cut++ {
			t.Run(fmt.Sprintf("%s/prefix%d", f.name, cut), func(t *testing.T) { gssapiTestReject(t, wire[:cut], false, "gssapi:") })
		}
	}
	for _, value := range []string{
		"600806062b0601050502", // Original: no negotiation token after OID.
		"a000", "a18030000000", "a181023000", "a18200023000", "a1033000ff", "a102300000",
		"a1073005a0030a0104", "a1083006a0040a020001", "a10c300aa0030a0101a0030a0101",
		"a10a3008a2020400a1020600", "a1063004a4020400", "a1073005a2030400ff",
		"600c06062b0601050502a1023000",                         // Generic framing forbidden for subsequent token.
		"600c06062b0601050502a0023000",                         // Initial mechanism list absent.
		"601006062b0601050502a0063004a0023000",                 // Empty mechanism list.
		"601806062b0601050502a00e300ca00a300806062b0601050502", // Offering SPNEGO itself.
		"a1073005a103060180", "a1083006a10406028001",           // Incomplete/nonminimal OID.
	} {
		wire, err := hex.DecodeString(value)
		require.NoError(t, err)
		gssapiTestReject(t, wire, false, "gssapi:")
	}
	for _, index := range []int{1, 9, 10, 29, 32, 33, 34} {
		bad := gssapiTestWire(t, 2)
		switch index {
		case 1:
			bad[index]++
		case 9:
			bad[index] = 0xa1
		case 10:
			bad[index]--
		case 29:
			bad[index] = 0xa0
		case 32:
			bad[index] = 8
		case 33:
			bad[index] = 8
		case 34:
			bad[index] = 0x41
		}
		gssapiTestReject(t, bad, false, "gssapi:")
	}
	badMechanism := gssapiTestWire(t, 0)
	badMechanism[8] = 3
	gssapiTestReject(t, badMechanism, false, "unsupported outer mechanism")
	// Strict complete HTTP and strict complete decoded token are independent.
	valid := gssapiTestHTTP(0, false)
	for cut := 0; cut < len(valid); cut++ {
		gssapiTestReject(t, valid[:cut], true, "")
	}
	for _, wire := range [][]byte{
		append(bytes.Clone(valid), 'x'),
		bytes.Replace(valid, []byte("Authorization:"), []byte("Proxy-Authorization:"), 1),
		bytes.Replace(valid, []byte("Negotiate "), []byte("Basic "), 1),
		bytes.Replace(valid, []byte("Negotiate "), []byte("Negotiate\t"), 1),
		bytes.Replace(valid, []byte("\r\n\r\n"), []byte("\r\nAuthorization: Negotiate aaaa\r\n\r\n"), 1),
		[]byte("GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nWWW-Authenticate: Negotiate\r\nContent-Length: 0\r\n\r\n"),
		gssapiTestHTTP(0, true),
	} {
		gssapiTestReject(t, wire, true, "")
	}
	for _, encoded := range []string{"YAgGBisGAQUFAg==", "oQI wAA==", "oQIwAA", "oQIwAB==", "oQIwAA===", "oQIwAA==,Basic eA==", "_w=="} {
		wire := []byte("GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate " + encoded + "\r\n\r\n")
		gssapiTestReject(t, wire, true, "gssapi:")
	}
}

func TestProtocolCorpusGSSAPIOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-gssapi-http.pcap"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "4d8fa4ca6f9f7ebbe710a1d92a7111f693efbdc8ad9ef5810626650ee4609e39", fmt.Sprintf("%x", sha256.Sum256(file)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(6), frame[23])
		require.Equal(t, byte(0x50), frame[46])
		require.Equal(t, len(frame)-14, int(binary.BigEndian.Uint16(frame[16:18])))
		payload := frame[54:]
		if i < 3 {
			require.Empty(t, payload)
			gssapiTestReject(t, payload, true, "")
			continue
		}
		require.Equal(t, []byte("GET / HTTP/1.1\r\nHost: lab\r\nAuthorization: Negotiate YAgGBisGAQUFAg==\r\n\r\n"), payload)
		gssapiTestReject(t, payload, true, "missing SPNEGO negotiation token after mechanism OID")
		token, err := base64.StdEncoding.DecodeString("YAgGBisGAQUFAg==")
		require.NoError(t, err)
		require.Equal(t, []byte{0x60, 8, 6, 6, 0x2b, 6, 1, 5, 5, 2}, token)
		gssapiTestReject(t, token, false, "missing SPNEGO negotiation token")
	}
}

func TestProtocolCorpusGSSAPICompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-gssapi-valid.pcap"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "79338fa63974f15261c68176aaeb482bf692db9740dac814917a34090cd22663", fmt.Sprintf("%x", sha256.Sum256(file)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 10)
	for i, frame := range frames {
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(6), frame[23])
		require.Equal(t, byte(0x50), frame[46])
		require.Equal(t, len(frame)-14, int(binary.BigEndian.Uint16(frame[16:18])))
		wire := frame[54:]
		var expected []byte
		if i < 9 {
			expected = gssapiTestHTTP(i, i >= 3)
			if i == 3 || i == 5 || i == 6 {
				expected = bytes.Replace(expected, []byte("HTTP/1.1 200 OK"), []byte("HTTP/1.1 401 Unauthorized"), 1)
			}
		} else {
			expected = []byte("HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Negotiate\r\nContent-Length: 0\r\n\r\n")
		}
		require.Equal(t, expected, wire)
		for _, entry := range []string{"GSSAPIHTTP", "GSSAPIHTTPCarrier"} {
			n := gssapiTestParse(t, wire, entry)
			require.Equal(t, wire, NodeToBytes(n))
			gssapiTestHTTPWireTree(t, protocolCorpusFindNode(n, "HTTP"), 0, uint64(len(wire))*8)
			info := gssapiTestInfo(t, n, "Token Present")
			require.Equal(t, i < 9, info["Token Fields Decoded"])
			require.Equal(t, i < 9, info["Token Present"])
			if i < 9 {
				decoded := info["Decoded Token"].(map[string]any)
				token := gssapiTestWire(t, i)
				require.Equal(t, token, decoded["Decoded Bytes"])
				require.Equal(t, gssapiTestFixtures[i].kind, decoded["Token Kind"])
				require.Equal(t, "decoded-token-relative-bits", decoded["Span Coordinate System"])
				gssapiTestDecodedTree(t, decoded["Decoded Fields"].([]any), token, 0, uint64(len(token))*8)
				parsed := gssapiTestParse(t, token, "GSSAPIToken")
				require.Equal(t, token, NodeToBytes(parsed))
			} else {
				require.NotContains(t, info, "Decoded Token")
			}
		}
	}
}

// Boundary builders are independent of the decoder; golden companions above
// are literals. Nested lengths include each inner tag and length octet.
func gssapiTestTLV(tag byte, body []byte) []byte {
	length := []byte{byte(len(body))}
	if len(body) >= 256 {
		length = []byte{0x82, byte(len(body) >> 8), byte(len(body))}
	} else if len(body) >= 128 {
		length = []byte{0x81, byte(len(body))}
	}
	return append(append([]byte{tag}, length...), body...)
}

func TestProtocolCorpusGSSAPIResourcesAndIsolation(t *testing.T) {
	for _, count := range []int{0, 127, 128, 255, 256, 65519} {
		wire := gssapiTestTLV(0xa1, gssapiTestTLV(0x30, gssapiTestTLV(0xa2, gssapiTestTLV(4, bytes.Repeat([]byte{0xab}, count)))))
		n := gssapiTestParse(t, wire, "GSSAPIToken")
		require.Equal(t, wire, NodeToBytes(n))
		if count == 65519 {
			require.Len(t, wire, 65535)
		}
	}
	tooLong := bytes.NewReader(make([]byte, 65536))
	_, err := parser.ParseBinary(&giopCarrierTestBitReader{Reader: tooLong, bits: 65536 * 8}, gssapiTestRule, "GSSAPIToken")
	require.ErrorContains(t, err, "boundary")
	require.Equal(t, 65536, tooLong.Len())
	for _, count := range []int{64, 65} {
		// 1.2.3 is only an offered identifier; no mechanism semantics claimed.
		list := bytes.Repeat([]byte{6, 2, 0x2a, 3}, count)
		body := append([]byte{6, 6, 0x2b, 6, 1, 5, 5, 2}, gssapiTestTLV(0xa0, gssapiTestTLV(0x30, gssapiTestTLV(0xa0, gssapiTestTLV(0x30, list))))...)
		wire := gssapiTestTLV(0x60, body)
		if count == 64 {
			require.Equal(t, wire, NodeToBytes(gssapiTestParse(t, wire, "GSSAPIToken")))
		} else {
			gssapiTestReject(t, wire, false, "64-entry")
		}
	}
	for _, entry := range []string{"GSSAPIToken", "GSSAPICarrier", "GSSAPIHTTP", "GSSAPIHTTPCarrier"} {
		wire := gssapiTestWire(t, 0)
		if strings.Contains(entry, "HTTP") {
			wire = gssapiTestHTTP(0, false)
		}
		_, err := parser.ParseBinary(bytes.NewReader(wire), gssapiTestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, gssapiTestRule, entry)
		require.Error(t, err)
		for extra := uint64(1); extra < 8; extra++ {
			reader := &giopCarrierTestBitReader{Reader: bytes.NewReader(append(bytes.Clone(wire), 0)), bits: uint64(len(wire))*8 + extra}
			_, err = parser.ParseBinary(reader, gssapiTestRule, entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(wire)+1, reader.Len())
		}
	}
	for _, size := range []int{131072, 131073} {
		header := strings.TrimSuffix(string(gssapiTestHTTP(0, false)), "\r\n") + "Content-Length: 130000\r\n\r\n"
		// Fixed-width decimal length keeps the total boundary exact.
		bodyLength := size - len(header)
		header = strings.Replace(header, "130000", fmt.Sprintf("%06d", bodyLength), 1)
		wire := append([]byte(header), bytes.Repeat([]byte{'x'}, bodyLength)...)
		if size == 131072 {
			require.Equal(t, wire, NodeToBytes(gssapiTestParse(t, wire, "GSSAPIHTTP")))
		} else {
			reader := newProtocolCorpusBoundedReader(wire)
			_, err := parser.ParseBinary(reader, gssapiTestRule, "GSSAPIHTTP")
			require.ErrorContains(t, err, "131072-byte")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := gssapiTestHTTP(i%len(gssapiTestFixtures), false)
			n := gssapiTestParse(t, wire, "GSSAPIHTTPCarrier")
			require.Equal(t, wire, NodeToBytes(n))
			require.Equal(t, gssapiTestFixtures[i%len(gssapiTestFixtures)].kind, gssapiTestInfo(t, n, "Token Present")["Decoded Token"].(map[string]any)["Token Kind"])
		}(i)
	}
	wg.Wait()
}

func TestProtocolCorpusGSSAPIImportedHeldReader(t *testing.T) {
	for _, http := range []bool{false, true} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{false, true} {
				wire, entry, rawName := gssapiTestWire(t, 2), "GSSAPICarrier", "Unparsed GSS-API Token"
				if http {
					wire, entry, rawName = gssapiTestHTTP(2, false), "GSSAPIHTTPCarrier", "Unparsed GSS-API HTTP Message"
				}
				if !valid {
					wire = append(wire, 0xff)
				}
				t.Run(fmt.Sprintf("http%t/offset%d/valid%t", http, offset, valid), func(t *testing.T) {
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					}
					require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
					require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/gssapi.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("gssapi-caller", "preserved")
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Envelope"))
					n := base.GetNodeByPath(root, "@Envelope")
					message := protocolCorpusFindNode(n, "Message")
					if !valid {
						latTestField(t, message, rawName, "raw", offset, offset+uint64(len(wire))*8, wire)
						require.Nil(t, protocolCorpusFindNode(message, "Mechanism OID"))
						require.Nil(t, protocolCorpusFindNode(message, "HTTP"))
					} else if http {
						decoded := gssapiTestInfo(t, message, "Token Present")["Decoded Token"].(map[string]any)
						token := gssapiTestWire(t, 2)
						gssapiTestDecodedTree(t, decoded["Decoded Fields"].([]any), token, 0, uint64(len(token))*8)
						method := protocolCorpusFindNode(message, "Method")
						require.Equal(t, offset, stream_parser.GetNodeResultPos(method)[0])
						gssapiTestHTTPWireTree(t, protocolCorpusFindNode(message, "HTTP"), offset, offset+uint64(len(wire))*8)
					} else {
						latTestField(t, message, "Tag", "uint8", offset, offset+8, uint64(0x60))
						megacoTestTree(t, protocolCorpusFindNode(message, "Initial Context Token"), offset, offset+uint64(len(wire))*8)
					}
					protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
					require.Equal(t, "preserved", root.Ctx.GetItem("gssapi-caller"))
					require.Equal(t, packed.Bytes(), NodeToBytes(n))
					require.NoError(t, r.Recovery())
					got, err := r.ReadBits(uint64(packed.Len()) * 8)
					require.NoError(t, err)
					require.Equal(t, packed.Bytes(), got)
					require.ErrorContains(t, r.PopBackup(), "no backup")
				})
			}
		}
	}
}

func TestProtocolCorpusGSSAPIImportedNonByteAndPhysicalBoundaries(t *testing.T) {
	for _, http := range []bool{false, true} {
		wire, entry := gssapiTestWire(t, 0), "GSSAPICarrier"
		if http {
			wire, entry = gssapiTestHTTP(0, false), "GSSAPIHTTPCarrier"
		}
		for offset := uint64(0); offset < 8; offset++ {
			for extra := uint64(1); extra < 8; extra++ {
				var packed, before bytes.Buffer
				w, expected := base.NewBitWriter(&packed), base.NewBitWriter(&before)
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					require.NoError(t, expected.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, w.WriteBits([]byte{0x55}, extra))
				require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
				if pad := (8 - (offset+extra)%8) % 8; pad > 0 {
					require.NoError(t, w.WriteBits([]byte{0}, pad))
				}
				root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
    Prefix: uint8,%dbit
    Message:
      import: application-layer/gssapi.yaml
      node: %s
      length: %d
    Sentinel: uint8
`, offset, offset, entry, uint64(len(wire))*8+extra))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.ErrorContains(t, root.ParseSubNode(r, "Envelope"), "explicit byte boundary")
				require.Equal(t, offset, root.Ctx.GetUint64("pointer"))
				require.Equal(t, before.Bytes(), root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
				require.Equal(t, expected.Snapshot(), root.Ctx.GetItem("writer").(*base.BitWriter).Snapshot())
				got, err := r.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, got)
				_, err = r.ReadBits(extra)
				require.NoError(t, err)
				got, err = r.ReadBits(8)
				require.NoError(t, err)
				require.Equal(t, []byte{0xd3}, got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
		for _, cut := range []int{0, 1, len(wire) / 2, len(wire) - 1} {
			root, err := base.ParseRule("application-layer/gssapi.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			r := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.NoError(t, r.Backup())
			require.Error(t, root.ParseSubNode(r, entry))
			require.NoError(t, r.Recovery())
			if cut > 0 {
				got, err := r.ReadBits(uint64(cut) * 8)
				require.NoError(t, err)
				require.Equal(t, wire[:cut], got)
			}
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
}
