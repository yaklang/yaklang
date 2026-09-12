package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const dockerAPICorpusRule = "application-layer.docker_api"
const dockerAPIDirect = "DockerContainerListRequest"
const dockerAPICarrier = "DockerContainerListRequestCarrier"

type dockerAPITestFixture struct {
	target, version string
	headers         []string
	info            map[string]any
}

func (f dockerAPITestFixture) wire() []byte {
	wire := "GET " + f.target + " " + f.version + "\r\n"
	for _, header := range f.headers {
		wire += header + "\r\n"
	}
	return []byte(wire + "\r\n")
}

// Independent literals follow v1.41 OpenAPI ContainerList and the pinned
// Moby v20.10.0 query/boolean/JSON codec, not parser-generated encodings.
// Record zero is also the exact, unchanged payload of both original captures.
func dockerAPITestFixtures() []dockerAPITestFixture {
	return []dockerAPITestFixture{
		{"/v1.41/containers/json", "HTTP/1.1", []string{"Host: localhost"}, map[string]any{
			"All": false, "Size": false, "Limit": int64(0), "All Present": false, "Size Present": false, "Limit Present": false,
			"Filters Present": false, "Filters": map[string]map[string]bool{}, "Filter Encoding": "absent-or-empty", "Host Header": "localhost", "Hostname": "localhost", "Port": "", "Host Header Present": true,
		}},
		{"/v1.41/containers/json?all=true&size=1&limit=25&filters=%7B%22status%22%3A%5B%22running%22%5D%2C%22label%22%3A%5B%22group%3Ddemo%22%5D%7D", "HTTP/1.1", []string{"Host: localhost:2375", "Content-Length: 0"}, map[string]any{
			"All": true, "Size": true, "Limit": int64(25), "All Present": true, "Size Present": true, "Limit Present": true,
			"Filters Present": true, "Filters": map[string]map[string]bool{"status": {"running": true}, "label": {"group=demo": true}}, "Filter Encoding": "string-array", "Host Header": "localhost:2375", "Hostname": "localhost", "Port": "2375", "Host Header Present": true,
		}},
		{"/v1.41/containers/json?all=NO&all=true&size=anything&limit=-1&since=older&before=newer&x=one+two&x=%2B&filters=%7B%22name%22%3A%7B%22demo%22%3Afalse%7D%7D", "HTTP/1.0", nil, map[string]any{
			"All": false, "Size": true, "Limit": int64(-1), "All Present": true, "Size Present": true, "Limit Present": true,
			"Filters Present": true, "Filters": map[string]map[string]bool{"name": {"demo": false}}, "Filter Encoding": "boolean-set", "Legacy Since": "older", "Legacy Before": "newer", "Legacy Since Present": true, "Legacy Before Present": true,
			"Unknown Query Values": map[string][]string{"x": {"one two", "+"}}, "Host Header": "", "Hostname": "", "Port": "", "Host Header Present": false,
		}},
		{"/v1.41/containers/json?all=&size=NONE&limit=&filters=null", "HTTP/1.1", []string{"hOsT: [2001:db8::1]:2375", "X-Note: one", "X-Note: two"}, map[string]any{
			"All": false, "Size": false, "Limit": int64(0), "All Present": true, "Size Present": true, "Limit Present": true,
			"Filters Present": true, "Filters": map[string]map[string]bool{}, "Filter Encoding": "boolean-set", "Host Header": "[2001:db8::1]:2375", "Hostname": "2001:db8::1", "Port": "2375", "Host Header Present": true,
		}},
	}
}

func dockerAPITestInfo(t *testing.T, node *base.Node) map[string]any {
	t.Helper()
	http := protocolCorpusFindNode(node, "HTTP")
	require.NotNil(t, http)
	return alljoynTestInfo(t, http.Cfg.GetItem(base.CfgParent).(*base.Node))
}

func dockerAPITestFields(t *testing.T, node *base.Node, fixture dockerAPITestFixture, offset uint64) {
	t.Helper()
	var expected, actual []alljoynTestLeaf
	cursor := offset
	leaf := func(name, value, delimiter string) {
		expected = append(expected, alljoynTestLeaf{Name: name, Value: value, Span: [2]uint64{cursor, cursor + uint64(len(value))*8}})
		cursor += uint64(len(value)+len(delimiter)) * 8
	}
	leaf("Method", "GET", " ")
	leaf("Path", fixture.target, " ")
	leaf("Version", fixture.version, "\r\n")
	for _, line := range append(append([]string{}, fixture.headers...), "") {
		leaf("Item", line, "\r\n")
	}
	var visit func(*base.Node)
	visit = func(n *base.Node) {
		if protocolCorpusNodeHasResult(n) {
			value, err := n.Result()
			require.NoError(t, err)
			require.Same(t, n, value.Origin)
		}
		if stream_parser.NodeHasResult(n) {
			result, err := n.Result()
			require.NoError(t, err)
			require.IsType(t, "", result.Value)
			require.Equal(t, "string", n.Cfg.GetItem(base.CfgType))
			actual = append(actual, alljoynTestLeaf{Name: n.Name, Span: stream_parser.GetNodeResultPos(n), Value: result.Value})
			return
		}
		for _, child := range n.Children {
			require.Same(t, n, child.Cfg.GetItem(base.CfgParent))
			visit(child)
		}
	}
	visit(node)
	require.Equal(t, expected, actual)
	require.Equal(t, offset+uint64(len(fixture.wire()))*8, cursor)
	info := dockerAPITestInfo(t, node)
	for key, value := range map[string]any{
		"Profile": "Docker Engine v1.41 ContainerList request parameter codec", "API Version": "1.41", "Route": "/containers/json", "Operation": "ContainerList", "Method": "GET",
		"Request Target Form": "origin-form", "Body Profile": "bodyless; no Transfer-Encoding", "Response Schema Parsed": false, "Session Observed": false, "Endpoint Identity Proven": false,
		"Filter Values Evaluated": false, "Backend Acceptance Proven": false, "Filter Names Recognized": true, "Unknown Filter Names": []string{},
		"Query Duplicate Policy": "first value", "Boolean Codec": "Moby v20.10.0 BoolValue",
		"Parameter Span Coordinate System": "request-target-relative-bits",
		"API Version Target Bit Span":      [2]uint64{16, 48}, "Route Target Bit Span": [2]uint64{48, 176},
	} {
		require.Equal(t, value, info[key], key)
	}
	for key, value := range fixture.info {
		if key == "Limit" {
			// The rule VM exposes its signed integer metadata as Go int.
			require.IsType(t, int(0), info[key])
			require.EqualValues(t, value, info[key], key)
		} else {
			require.Equal(t, value, info[key], key)
		}
	}
	require.EqualValues(t, 16384, info["Maximum Request Bytes"])
	parts := strings.SplitN(fixture.target, "?", 2)
	query := ""
	if len(parts) == 2 {
		query = parts[1]
	}
	require.Equal(t, len(parts) == 2, info["Query Present"])
	require.Equal(t, query, info["Raw Query"])
	pairs, ok := info["Query Pairs"].([]map[string]any)
	require.True(t, ok, "%T", info["Query Pairs"])
	values := map[string][]string{}
	index, start := 0, len(parts[0])+1
	for _, part := range strings.Split(query, "&") {
		if part != "" {
			name, value, equal := strings.Cut(part, "=")
			decodedName, err := url.QueryUnescape(name)
			require.NoError(t, err)
			decodedValue, err := url.QueryUnescape(value)
			require.NoError(t, err)
			valueStart := start + len(name)
			if equal {
				valueStart++
			}
			require.Equal(t, map[string]any{"Name": decodedName, "Value": decodedValue, "Raw Name": name, "Raw Value": value, "Equals Present": equal,
				"Name Target Bit Span": [2]uint64{uint64(start) * 8, uint64(start+len(name)) * 8}, "Value Target Bit Span": [2]uint64{uint64(valueStart) * 8, uint64(start+len(part)) * 8}}, pairs[index])
			values[decodedName] = append(values[decodedName], decodedValue)
			index++
		}
		start += len(part) + 1
	}
	require.Len(t, pairs, index)
	require.Equal(t, index, info["Query Pair Count"])
	require.Equal(t, values, info["Query Values"])
}

func TestProtocolCorpusDockerAPIFields(t *testing.T) {
	for index, fixture := range dockerAPITestFixtures() {
		for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
			t.Run(fmt.Sprintf("%d/%s", index, entry), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), dockerAPICorpusRule, entry)
				require.Equal(t, fixture.wire(), NodeToBytes(node))
				dockerAPITestFields(t, node, fixture, 0)
				t.Logf("literal %d len=%d hex=%x", index+1, len(fixture.wire()), fixture.wire())
			})
		}
	}
}

func TestProtocolCorpusDockerAPIAllOriginalRecords(t *testing.T) {
	seen, dataSeen := 0, 0
	for _, capture := range []struct{ path, sha string }{
		{"generated-local/gen-docker-api", "d67a5610fb33466f67580b472a119908a7b90376027ec0fd820dc56a5b786a76"},
		{"generated-pr5023/pr5023-gen-docker-api", "fd603adddb2d7c0a3cce560b7e17441444e8cc480c2b8cef13c4dc69953b9358"},
	} {
		path := "testdata/protocol-corpus/captures/" + capture.path + ".pcap"
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, capture.sha, fmt.Sprintf("%x", sha256.Sum256(data)))
		frames := protocolCorpusAuditPackets(t, path)
		require.Len(t, frames, 4)
		for i, frame := range frames {
			seen++
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Equal(t, frame, NodeToBytes(node))
			require.Nil(t, protocolCorpusFindNode(node, dockerAPIDirect), "HTTP production dispatch stays generic")
			if i < 3 {
				require.Len(t, frame, 54)
				require.Empty(t, tcp.Payload)
				require.Equal(t, i < 2, tcp.SYN)
				require.Equal(t, i > 0, tcp.ACK)
				require.False(t, tcp.PSH || tcp.FIN || tcp.RST || tcp.URG || tcp.ECE || tcp.CWR)
				require.Nil(t, protocolCorpusFindNode(node, "Method"))
				continue
			}
			dataSeen++
			fixture := dockerAPITestFixtures()[0]
			require.Len(t, frame, 110)
			require.Equal(t, fixture.wire(), tcp.Payload)
			require.EqualValues(t, 40100, tcp.SrcPort)
			require.EqualValues(t, 2375, tcp.DstPort)
			require.True(t, tcp.ACK && tcp.PSH)
			protocolCorpusRequireValue(t, node, "Method", "GET")
			protocolCorpusRequireValue(t, node, "Path", fixture.target)
			for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
				strict := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, dockerAPICorpusRule, entry)
				dockerAPITestFields(t, strict, fixture, 0)
			}
		}
	}
	require.Equal(t, 8, seen)
	require.Equal(t, 2, dataSeen)
}

func dockerAPITestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), dockerAPICorpusRule, dockerAPIDirect)
	require.Error(t, err, string(wire))
	if len(wire) == 0 {
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, dockerAPICorpusRule, dockerAPICarrier)
	protocolCorpusRequireValue(t, node, "Unparsed Docker Container List Request", wire)
	require.Nil(t, protocolCorpusFindNode(node, "Method"))
	require.Equal(t, wire, NodeToBytes(node))
}

func TestProtocolCorpusDockerAPIAllCompanionRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-validated/gen-docker-api-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "b5b8adcbe5c5cf33181b7e5672ef6ad8ffc9dd67a0185c764678f457f68c68eb", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	fixtures := dockerAPITestFixtures()
	require.Len(t, frames, len(fixtures))
	for index, frame := range frames {
		fixture := fixtures[index]
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.EqualValues(t, 40100, tcp.SrcPort)
		require.EqualValues(t, 2375, tcp.DstPort)
		require.Equal(t, fixture.wire(), tcp.Payload)
		require.Len(t, frame, 54+len(tcp.Payload))
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
		protocolCorpusRequireValue(t, node, "Method", "GET")
		protocolCorpusRequireValue(t, node, "Path", fixture.target)
		for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
			strict := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, dockerAPICorpusRule, entry)
			dockerAPITestFields(t, strict, fixture, 0)
			require.Equal(t, fixture.wire(), NodeToBytes(strict))
			// Opt-in import after the actual 54-byte Ethernet/IP/TCP prefix;
			// production dispatch remains generic HTTP, without service claims.
			root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Frame:
    operator: |
      this.ProcessSubNode("Prefix")
      this.GetSubNode("Request").SetMaxLength(%d)
      this.ProcessSubNode("Request")
    Prefix: raw,54
    Request: "import:application-layer/docker_api.yaml;node:%s"
`, len(tcp.Payload), entry))
			root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Frame"))
			envelope := base.GetNodeByPath(root, "@Frame")
			dockerAPITestFields(t, protocolCorpusFindNode(envelope, "Request"), fixture, 54*8)
			require.Equal(t, frame, NodeToBytes(envelope))
		}
	}
}

func TestProtocolCorpusDockerAPIShortPrefixesAndBounds(t *testing.T) {
	for _, fixture := range dockerAPITestFixtures() {
		wire := fixture.wire()
		for cut := 0; cut < len(wire); cut++ {
			dockerAPITestReject(t, wire[:cut])
		}
		for _, suffix := range [][]byte{{0}, {'\r', '\n'}, wire} {
			dockerAPITestReject(t, append(bytes.Clone(wire), suffix...))
		}
	}
	for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
		_, err := parser.ParseBinary(bytes.NewBuffer(dockerAPITestFixtures()[0].wire()), dockerAPICorpusRule, entry)
		require.ErrorContains(t, err, "explicit")
		node, err := parser.GenerateBinary(map[string]any{}, dockerAPICorpusRule, entry)
		require.Nil(t, node)
		require.ErrorContains(t, err, "explicit")
		for extra := uint64(1); extra < 8; extra++ {
			wire := append(dockerAPITestFixtures()[0].wire(), 0)
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(wire), uint64(len(wire)-1)*8 + extra}
			_, err := parser.ParseBinary(reader, dockerAPICorpusRule, entry)
			require.ErrorContains(t, err, "explicit byte boundary")
			require.Equal(t, len(wire), reader.Len())
		}
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), dockerAPICorpusRule, dockerAPICarrier)
	require.ErrorContains(t, err, "empty carrier")
}

func TestProtocolCorpusDockerAPIRejectsAndCompatibility(t *testing.T) {
	for _, query := range []string{"limit=1.5", "limit=abc", "limit=+1", "limit=9223372036854775808", "limit=-9223372036854775809", "filters=%5B%5D", "filters=true", "filters=%7B%22status%22%3A1%7D", "filters=%7B%22status%22%3A%5B1%5D%7D", "filters=%7B%22status%22%3A%7B%22running%22%3A1%7D%7D", "filters=%7B", "filters=null+null", "x=%", "x=%0z", "x=#fragment", "x=;", "x=[a]", "x=\\"} {
		fixture := dockerAPITestFixtures()[0]
		fixture.target += "?" + query
		dockerAPITestReject(t, fixture.wire())
	}
	for _, target := range []string{"*", "/containers/json", "/v1.40/containers/json", "/v1.42/containers/json", "/v1.41/containers/json/", "/v1.41/containers/abc/json", "/v1.41/%63ontainers/json", "http://localhost/v1.41/containers/json"} {
		fixture := dockerAPITestFixtures()[0]
		fixture.target = target
		dockerAPITestReject(t, fixture.wire())
	}
	for _, header := range []string{"", "Host:", "Host: a\r\nHost: a", "Host: a/b", "Host: a?b", "Host: a@b", "Host: a#b", "Host: [hostname]", "Host: a:65536", "Host: a:", "Host: a b", "Host: 2001:db8::1"} {
		dockerAPITestReject(t, []byte("GET /v1.41/containers/json HTTP/1.1\r\n"+header+"\r\n\r\n"))
	}
	for _, wire := range []string{
		"POST /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n[]",
		"GET /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1\r\n\r\nx",
		"GET /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		"GET /v1.41/containers/json HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1\r\nContent-Length: 0\r\n\r\nx",
	} {
		dockerAPITestReject(t, []byte(wire))
	}
	for _, test := range []struct {
		query string
		want  map[string]any
	}{
		{"", map[string]any{"Query Present": true, "Query Pair Count": 0}},
		{"&&all&&size=+FaLsE+&limit=%2B0001&", map[string]any{"All": false, "All Present": true, "Size": false, "Limit": int64(1), "Query Pair Count": 3}},
		{"limit=9223372036854775807", map[string]any{"Limit": int64(9223372036854775807)}},
		{"limit=-9223372036854775808", map[string]any{"Limit": int64(-9223372036854775808)}},
		{"limit=2&limit=bad&filters=&filters=bad", map[string]any{"Limit": int64(2), "Filters Present": true, "Filter Encoding": "absent-or-empty"}},
		{"filters=%7B%22name%22%3A%5B%22a%22%2C%22a%22%5D%7D", map[string]any{"Filters": map[string]map[string]bool{"name": {"a": true}}}},
		{"filters=%7B%22name%22%3Anull%7D", map[string]any{"Filters": map[string]map[string]bool{"name": nil}}},
		{"filters=%7B%22future%22%3A%5B%22value%22%5D%7D", map[string]any{"Filter Names Recognized": false, "Unknown Filter Names": []string{"future"}, "Filters": map[string]map[string]bool{"future": {"value": true}}}},
		{"filters=%7B%22status%22%3A%5B%22not-a-state%22%5D%7D", map[string]any{"Filter Names Recognized": true, "Filter Values Evaluated": false}},
		{"filters=%7B%22name%22%3A%7B%22a%22%3Atrue%7D%2C%22name%22%3A%7B%22b%22%3Afalse%7D%7D", map[string]any{"Filters": map[string]map[string]bool{"name": {"b": false}}}},
	} {
		fixture := dockerAPITestFixtures()[0]
		fixture.target += "?" + test.query
		node := protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), dockerAPICorpusRule, dockerAPIDirect)
		info := dockerAPITestInfo(t, node)
		for key, value := range test.want {
			if key == "Limit" {
				require.IsType(t, int(0), info[key])
				require.EqualValues(t, value, info[key], "%s: %s", test.query, key)
			} else {
				require.Equal(t, value, info[key], "%s: %s", test.query, key)
			}
		}
		require.Equal(t, false, info["Backend Acceptance Proven"])
	}
}

func TestProtocolCorpusDockerAPIResourceProfile(t *testing.T) {
	fixture := dockerAPITestFixtures()[0]
	fixture.target += "?" + strings.TrimSuffix(strings.Repeat("x=1&", 256), "&")
	node := protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), dockerAPICorpusRule, dockerAPIDirect)
	require.Equal(t, 256, dockerAPITestInfo(t, node)["Query Pair Count"])
	fixture.target += "&x=1"
	dockerAPITestReject(t, fixture.wire())
	fixture = dockerAPITestFixtures()[0]
	fixture.target += "?x=" + strings.Repeat("a", 8192-len(fixture.target)-3)
	require.Len(t, fixture.target, 8192)
	protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), dockerAPICorpusRule, dockerAPIDirect)
	fixture.target += "a"
	dockerAPITestReject(t, fixture.wire())
	fixture = dockerAPITestFixtures()[0]
	fixture.headers = append(fixture.headers, "X-Pad: ")
	fixture.headers[1] += strings.Repeat("x", 16384-len(fixture.wire()))
	wire := fixture.wire()
	require.Len(t, wire, 16384)
	protocolCorpusRequireBoundedRuleParse(t, wire, dockerAPICorpusRule, dockerAPIDirect)
	reader := newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0))
	_, err := parser.ParseBinary(reader, dockerAPICorpusRule, dockerAPIDirect)
	require.ErrorContains(t, err, "16384-byte implementation profile")
	require.Equal(t, 16385, reader.Len(), "size guard precedes all HTTP reads")
	dockerAPITestReject(t, append(bytes.Clone(wire), 0))
}

func TestProtocolCorpusDockerAPIImportsAndTransactions(t *testing.T) {
	fixture := dockerAPITestFixtures()[1]
	for offset := 0; offset < 8; offset++ {
		for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
			prefix, pad := "", ""
			if offset != 0 {
				prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
				pad = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
			}
			wire := fixture.wire()
			root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Request").SetMaxLength(%d)
      this.ProcessSubNode("Request")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Request: "import:application-layer/docker_api.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, entry, pad))
			var fields []snaTestField
			if offset != 0 {
				fields = append(fields, snaBit("Prefix", uint8(1<<offset-1), uint64(offset)))
			}
			fields = append(fields, snaRaw("Request", wire), snaU8("Suffix", 0x5a))
			if offset != 0 {
				fields = append(fields, snaBit("Padding", 0, uint64(8-offset)))
			}
			input := snaEncode(fields)
			root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
			reader := base.NewBitReader(bytes.NewReader(input))
			require.NoError(t, reader.Backup())
			require.NoError(t, root.ParseSubNode(reader, "Envelope"))
			node := base.GetNodeByPath(root, "@Envelope")
			dockerAPITestFields(t, protocolCorpusFindNode(node, "Request"), fixture, uint64(offset))
			miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
			require.Equal(t, input, NodeToBytes(node))
			require.NoError(t, reader.Recovery())
			replayed, err := reader.ReadBits(uint64(len(input)) * 8)
			require.NoError(t, err)
			require.Equal(t, input, replayed)
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
	for _, valid := range []bool{true, false} {
		for _, rollback := range []bool{false, true} {
			wire := fixture.wire()
			if !valid {
				wire = bytes.Replace(wire, []byte("limit=25"), []byte("limit=no"), 1)
			}
			reader := base.NewBitReader(bytes.NewReader(append(append([]byte{0xa5}, wire...), 0x5a)))
			prefix, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0xa5}, prefix)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("application-layer/docker_api.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, dockerAPICarrier))
			node := base.GetNodeByPath(root, "@"+dockerAPICarrier)
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				dockerAPITestFields(t, node, fixture, 0)
			} else {
				protocolCorpusRequireValue(t, node, "Unparsed Docker Container List Request", wire)
				require.Nil(t, protocolCorpusFindNode(node, "Method"))
			}
			if rollback {
				require.NoError(t, reader.Recovery())
				replay, err := reader.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, replay)
			} else {
				require.NoError(t, reader.PopBackup())
			}
			suffix, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0x5a}, suffix)
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
}

func TestProtocolCorpusDockerAPIPhysicalTruncationAndConcurrentIsolation(t *testing.T) {
	wire := dockerAPITestFixtures()[1].wire()
	for _, entry := range []string{dockerAPIDirect, dockerAPICarrier} {
		for _, cut := range []int{1, 20, len(wire) - 1} {
			root, err := base.ParseRule("application-layer/docker_api.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery())
			replay, err := reader.ReadBits(uint64(cut) * 8)
			require.NoError(t, err)
			require.Equal(t, wire[:cut], replay)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	var wait sync.WaitGroup
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			wire := dockerAPITestFixtures()[i%4].wire()
			valid := i%3 != 0
			if !valid {
				wire = append(wire, 0)
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), dockerAPICorpusRule, dockerAPICarrier)
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Method") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed Docker Container List Request") != nil) == valid) {
				err = fmt.Errorf("Docker API context %d leaked trial state", i)
			}
			errors <- err
		}(i)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
