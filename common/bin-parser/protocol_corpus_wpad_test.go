package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
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

const wpadCorpusRule = "application-layer.wpad"

type wpadTestFixture struct {
	target, version, host  string
	headers                []string
	body                   string
	form, authority, query string
	hostname, port         string
	accepts, agents        []string
}

func (f wpadTestFixture) wire() []byte {
	return []byte("GET " + f.target + " " + f.version + "\r\n" + strings.Join(f.headers, "\r\n") + "\r\n\r\n" + f.body)
}

// Literal source facts: the first two requests are the unchanged payloads in
// both source captures. Remaining vectors are independently constructed from
// draft-ietf-wrec-wpad-01 sections 3/4.6 and RFC 9112 section 3.2, not generated
// by the rule under test. They exercise retrieval only, never PAC execution.
func wpadTestFixtures() []wpadTestFixture {
	return []wpadTestFixture{
		{target: "/wpad.dat", version: "HTTP/1.1", host: "wpad.lab.local", headers: []string{"Host: wpad.lab.local"}, form: "origin-form", authority: "wpad.lab.local", hostname: "wpad.lab.local"},
		{target: "http://wpad/wpad.dat", version: "HTTP/1.1", host: "wpad", headers: []string{"Host: wpad"}, form: "absolute-form", authority: "wpad", hostname: "wpad"},
		{target: "/wpad.dat?site=lab%20one", version: "HTTP/1.1", host: "wpad.example", headers: []string{"hOsT:\twpad.example \t", "Accept: application/x-ns-proxy-autoconfig", "Accept: */*;q=0.1", "User-Agent: binparser-fixture/1.0", "X-Note: one", "X-Note: two"}, form: "origin-form", authority: "wpad.example", hostname: "wpad.example", query: "site=lab%20one", accepts: []string{"application/x-ns-proxy-autoconfig", "*/*;q=0.1"}, agents: []string{"binparser-fixture/1.0"}},
		{target: "HTTP://WPAD.example.:8080/wpad.dat?x=%2f", version: "HTTP/1.0", host: "other.example:80", headers: []string{"Host: other.example:80", "Accept:", "User-Agent:"}, form: "absolute-form", authority: "WPAD.example.:8080", hostname: "WPAD.example.", port: "8080", query: "x=%2f", accepts: []string{""}, agents: []string{""}},
		{target: "/wpad.dat", version: "HTTP/1.1", host: "192.0.2.10:65535", headers: []string{"Host: 192.0.2.10:65535", "Content-Length: 3"}, body: "abc", form: "origin-form", authority: "192.0.2.10:65535", hostname: "192.0.2.10", port: "65535"},
		{target: "http://[2001:db8::1]:80/wpad.dat", version: "HTTP/1.1", host: "[2001:db8::1]:80", headers: []string{"Host: [2001:db8::1]:80"}, form: "absolute-form", authority: "[2001:db8::1]:80", hostname: "2001:db8::1", port: "80"},
		{target: "/wpad.dat?", version: "HTTP/1.1", host: "[fe80::1%25en0]", headers: []string{"Host: [fe80::1%25en0]"}, form: "origin-form", authority: "[fe80::1%25en0]", hostname: "fe80::1%en0"},
		{target: "/wpad.dat", version: "HTTP/1.1", host: "wpad:0", headers: []string{"Host: wpad:0"}, form: "origin-form", authority: "wpad:0", hostname: "wpad", port: "0"},
	}
}

func wpadTestCheck(t *testing.T, node *base.Node, fixture wpadTestFixture, offset uint64) {
	t.Helper()
	wire := fixture.wire()
	var expected []alljoynTestLeaf
	cursor := offset
	leaf := func(name, value, delimiter string) {
		expected = append(expected, alljoynTestLeaf{Name: name, Span: [2]uint64{cursor, cursor + uint64(len(value))*8}, Value: value})
		cursor += uint64(len(value)+len(delimiter)) * 8
	}
	leaf("Method", "GET", " ")
	leaf("Path", fixture.target, " ")
	leaf("Version", fixture.version, "\r\n")
	for _, line := range append(append([]string{}, fixture.headers...), "") {
		leaf("Item", line, "\r\n")
	}
	if fixture.body != "" {
		leaf("Octets", fixture.body, "")
	}
	var actual []alljoynTestLeaf
	var visit func(*base.Node)
	visit = func(n *base.Node) {
		if protocolCorpusNodeHasResult(n) {
			value, err := n.Result()
			require.NoError(t, err)
			require.Same(t, n, value.Origin, n.Name)
		}
		if stream_parser.NodeHasResult(n) {
			span := stream_parser.GetNodeResultPos(n)
			if span[0] < offset {
				return
			}
			value, err := n.Result()
			require.NoError(t, err)
			require.IsType(t, "", value.Value)
			require.Equal(t, "string", n.Cfg.GetItem(base.CfgType))
			actual = append(actual, alljoynTestLeaf{n.Name, span, value.Value})
			return
		}
		for _, child := range n.Children {
			require.Same(t, n, child.Cfg.GetItem(base.CfgParent), child.Name)
			visit(child)
		}
	}
	visit(node)
	require.Equal(t, expected, actual)
	require.Equal(t, offset+uint64(len(wire))*8, cursor)
	message := protocolCorpusFindNode(node, "HTTP").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, message)
	for key, want := range map[string]any{
		"Profile": "WPAD default-path HTTP retrieval request", "Request Target Form": fixture.form,
		"Configuration Authority": fixture.authority, "Configuration Path": "/wpad.dat", "Configuration Query": fixture.query,
		"Configuration Hostname": fixture.hostname, "Configuration Port": fixture.port,
		"Configuration URI": "http://" + fixture.authority + "/wpad.dat" + func() string {
			if strings.Contains(fixture.target, "?") {
				return "?" + fixture.query
			}
			return ""
		}(),
		"Scheme Source": "WPAD HTTP retrieval profile", "Host Header": fixture.host,
		"Accept Header Present": len(fixture.accepts) > 0, "User-Agent Header Present": len(fixture.agents) > 0,
		"Draft Accept Requirement Missing": len(fixture.accepts) == 0,
		"Accept Semantics Validated":       false, "Discovery Exchange Observed": false, "Proxy Use Proven": false,
		"Configuration Body Parsed": false,
	} {
		require.Equal(t, want, info[key], key)
	}
	require.EqualValues(t, 8192, info["Maximum Request Bytes"])
	require.Equal(t, append([]string{}, fixture.accepts...), info["Accept Values"])
	require.Equal(t, append([]string{}, fixture.agents...), info["User-Agent Values"])
}

func TestProtocolCorpusWPADFields(t *testing.T) {
	for index, fixture := range wpadTestFixtures() {
		for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
			t.Run(fmt.Sprintf("%d/%s", index, entry), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), wpadCorpusRule, entry)
				require.Equal(t, fixture.wire(), NodeToBytes(node))
				wpadTestCheck(t, node, fixture, 0)
			})
		}
	}
}

func TestProtocolCorpusWPADAllOriginalRecords(t *testing.T) {
	framesSeen, requestsSeen := 0, 0
	for _, capture := range []struct {
		path, sha string
		fixture   int
	}{
		{"generated-local/gen-wpad", "6da36e545a14c500a6466f7470ab446e552dc332fa6b0677136782719583135d", 0},
		{"generated-local/gen-wpad-proxy", "52bf26c678f4d3dd5cd4d323fcac49c9471ca6bbd26518022e87f71a3b83ef07", 1},
		{"generated-pr5023/pr5023-gen-wpad", "0a6ddbfe9debbe20d80c25e010045802b2a168ae31ad6f3269bb437d5fc3616a", 0},
		{"generated-pr5023/pr5023-gen-wpad-proxy", "58a87873b6b1fd04d1112cfed3c9dd78dba7e0ddbb4ec5f6c611463984ad0ba6", 1},
	} {
		t.Run(capture.path, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/" + capture.path + ".pcap"
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, capture.sha, fmt.Sprintf("%x", sha256.Sum256(data)))
			frames := protocolCorpusAuditPackets(t, path)
			require.Len(t, frames, 4)
			for index, frame := range frames {
				framesSeen++
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
				require.Equal(t, frame, NodeToBytes(node))
				if index < 3 {
					require.Empty(t, tcp.Payload)
					require.Equal(t, index < 2, tcp.SYN)
					require.Equal(t, index > 0, tcp.ACK)
					require.False(t, tcp.FIN || tcp.RST || tcp.PSH || tcp.URG || tcp.ECE || tcp.CWR)
					require.Nil(t, protocolCorpusFindNode(node, "Method"))
					continue
				}
				requestsSeen++
				fixture := wpadTestFixtures()[capture.fixture]
				require.Equal(t, fixture.wire(), tcp.Payload)
				// Ethernet/TCP keeps its generic HTTP contract; WPAD is opt-in.
				protocolCorpusRequireValue(t, node, "Method", "GET")
				protocolCorpusRequireValue(t, node, "Path", fixture.target)
				require.Nil(t, protocolCorpusFindNode(node, "WPADRequest"))
				for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
					request := protocolCorpusRequireBoundedRuleParse(t, tcp.Payload, wpadCorpusRule, entry)
					wpadTestCheck(t, request, fixture, 0)
				}
			}
		})
	}
	require.Equal(t, 16, framesSeen)
	require.Equal(t, 4, requestsSeen)
}

func TestProtocolCorpusWPADShortPrefixesAndBoundary(t *testing.T) {
	for index, fixture := range wpadTestFixtures() {
		wire := fixture.wire()
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), wpadCorpusRule, "WPADRequest")
			require.Errorf(t, err, "fixture %d prefix %d", index, cut)
			if cut == 0 {
				continue
			}
			node := protocolCorpusRequireBoundedRuleParse(t, wire[:cut], wpadCorpusRule, "WPADRequestCarrier")
			protocolCorpusRequireValue(t, node, "Unparsed WPAD Request", wire[:cut])
			require.Nil(t, protocolCorpusFindNode(node, "Method"))
		}
		for _, suffix := range [][]byte{{0}, {'\r', '\n'}, wire} {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), suffix...)), wpadCorpusRule, "WPADRequest")
			require.ErrorContains(t, err, "unconsumed bytes")
		}
	}
}

func TestProtocolCorpusWPADRejectsAndResourceProfile(t *testing.T) {
	for _, wire := range []string{
		"GET /wpad.dat HTTP/1.1\r\n\r\n", "GET /wpad.dat HTTP/1.1\r\nHost:\r\n\r\n",
		"GET /wpad.dat HTTP/1.1\r\nHost: wpad\r\nhost: wpad\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n",
		"POST /wpad.dat HTTP/1.1\r\nHost: wpad\r\n\r\n",
		"GET /other.pac HTTP/1.1\r\nHost: wpad\r\n\r\n",
	} {
		wpadTestReject(t, []byte(wire))
	}
	for _, target := range []string{"*", "wpad:80", "https://wpad/wpad.dat", "http://wpad", "http:///wpad.dat", "http://u@wpad/wpad.dat", "http://wpad?x/wpad.dat", "http://wpad?x=/wpad.dat", "/wpad.dat#x", "/wpad.dat?x=#a", "/wpad.dat?x=%", "/wpad.dat?x=%0z", "/wpad.dat?x=\\"} {
		wpadTestReject(t, []byte("GET "+target+" HTTP/1.1\r\nHost: wpad\r\n\r\n"))
	}
	for _, host := range []string{"a b", "a\tb", "[wpad]", "[192.0.2.1]", "2001:db8::1", "a:65536", "a:", "a:ab", "a:80:80", "a/b", "a@b", "a?x", "a#x", "a\\b"} {
		wpadTestReject(t, []byte("GET /wpad.dat HTTP/1.1\r\nHost: "+host+"\r\n\r\n"))
	}
	baseWire := "GET /wpad.dat HTTP/1.1\r\nHost: wpad\r\nX-Pad: "
	maximum := []byte(baseWire + strings.Repeat("x", 8192-len(baseWire)-4) + "\r\n\r\n")
	require.Len(t, maximum, 8192)
	protocolCorpusRequireBoundedRuleParse(t, maximum, wpadCorpusRule, "WPADRequest")
	wpadTestReject(t, append(bytes.Clone(maximum), 0))
	tooLong := newProtocolCorpusBoundedReader(append(bytes.Clone(maximum), 0))
	_, err := parser.ParseBinary(tooLong, wpadCorpusRule, "WPADRequest")
	require.ErrorContains(t, err, "8192-byte implementation profile")
	require.Equal(t, 8193, tooLong.Len(), "resource guard must precede HTTP reads")
	for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
		_, err := parser.ParseBinary(bytes.NewBuffer(wpadTestFixtures()[0].wire()), wpadCorpusRule, entry)
		require.ErrorContains(t, err, "explicit")
		node, err := parser.GenerateBinary(map[string]any{}, wpadCorpusRule, entry)
		require.Nil(t, node)
		require.ErrorContains(t, err, "explicit")
	}
}

func TestProtocolCorpusWPADBitBoundariesAndNestedImports(t *testing.T) {
	fixture := wpadTestFixtures()[1]
	wire := fixture.wire()
	for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
		for extra := uint64(1); extra < 8; extra++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(append(bytes.Clone(wire), 0)), uint64(len(wire))*8 + extra}
			_, err := parser.ParseBinary(reader, wpadCorpusRule, entry)
			require.ErrorContains(t, err, "explicit byte boundary required")
			require.Equal(t, len(wire)+1, reader.Len())
		}
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), wpadCorpusRule, "WPADRequestCarrier")
	require.ErrorContains(t, err, "empty carrier")
	for offset := 0; offset < 8; offset++ {
		for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
			for _, rollback := range []bool{false, true} {
				t.Run(fmt.Sprintf("bits-%d/%s/recover-%t", offset, entry, rollback), func(t *testing.T) {
					prefix := ""
					padding := ""
					if offset > 0 {
						prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
						padding = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
					}
					root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Request").SetMaxLength(%d)
      this.ProcessSubNode("Request")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Request: "import:application-layer/wpad.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, entry, padding))
					totalBits := offset + len(wire)*8 + 8
					input := make([]byte, (totalBits+7)/8)
					for bit := 0; bit < offset; bit++ {
						input[bit/8] |= 1 << (7 - bit%8)
					}
					for index, octet := range append(bytes.Clone(wire), 0x5a) {
						for bit := 0; bit < 8; bit++ {
							pos := offset + index*8 + bit
							input[pos/8] |= ((octet >> (7 - bit)) & 1) << (7 - pos%8)
						}
					}
					root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
					reader := base.NewBitReader(bytes.NewReader(input))
					require.NoError(t, reader.Backup())
					require.NoError(t, root.ParseSubNode(reader, "Envelope"))
					node := base.GetNodeByPath(root, "@Envelope")
					request := protocolCorpusFindNode(node, "Request")
					require.NotNil(t, request)
					wpadTestCheck(t, request, fixture, uint64(offset))
					// NodeToBytes exposes the owning context's complete buffer, not
					// a slice of the imported subnode. The leaves above prove every
					// request byte and delimiter at its shifted global position.
					require.Equal(t, input, NodeToBytes(node))
					suffixValue, err := protocolCorpusFindNode(node, "Suffix").Result()
					require.NoError(t, err)
					require.Equal(t, uint8(0x5a), suffixValue.Value)
					require.Equal(t, [2]uint64{uint64(offset + len(wire)*8), uint64(offset + (len(wire)+1)*8)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Suffix")))
					if rollback {
						require.NoError(t, reader.Recovery())
						replayed, err := reader.ReadBits(uint64(len(input)) * 8)
						require.NoError(t, err)
						require.Equal(t, input, replayed)
					} else {
						require.NoError(t, reader.PopBackup())
					}
					require.ErrorContains(t, reader.Recovery(), "no backup")
				})
			}
		}
	}
}

func wpadTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), wpadCorpusRule, "WPADRequest")
	require.Error(t, err, string(wire))
	node := protocolCorpusRequireBoundedRuleParse(t, wire, wpadCorpusRule, "WPADRequestCarrier")
	protocolCorpusRequireValue(t, node, "Unparsed WPAD Request", wire)
	require.Nil(t, protocolCorpusFindNode(node, "Method"))
	require.Equal(t, wire, NodeToBytes(node))
}

func TestProtocolCorpusWPADHeldReaderTransactions(t *testing.T) {
	fixture := wpadTestFixtures()[2]
	for _, valid := range []bool{true, false} {
		for _, rollback := range []bool{false, true} {
			wire := fixture.wire()
			if !valid {
				wire = append(wire, 0)
			}
			input := append(append([]byte{0xa5}, wire...), 0x5a)
			reader := base.NewBitReader(bytes.NewReader(input))
			prefix, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0xa5}, prefix)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("application-layer/wpad.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "WPADRequestCarrier"))
			node := base.GetNodeByPath(root, "@WPADRequestCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				wpadTestCheck(t, node, fixture, 0)
			} else {
				protocolCorpusRequireValue(t, node, "Unparsed WPAD Request", wire)
			}
			if rollback {
				require.NoError(t, reader.Recovery())
				replayed, err := reader.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, replayed)
			} else {
				require.NoError(t, reader.PopBackup())
			}
			suffix, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0x5a}, suffix)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
}

func TestProtocolCorpusWPADConcurrentIsolation(t *testing.T) {
	var wait sync.WaitGroup
	errors := make(chan error, 16)
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			fixtures := wpadTestFixtures()
			wire := fixtures[index%len(fixtures)].wire()
			valid := index%2 == 0
			if !valid {
				wire = append(wire, 0)
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), wpadCorpusRule, "WPADRequestCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Method") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed WPAD Request") != nil) == valid) {
				err = fmt.Errorf("WPAD concurrent request %d leaked trial state", index)
			}
			errors <- err
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestProtocolCorpusWPADHTTPConfigAndPhysicalTruncation(t *testing.T) {
	bodyWire := wpadTestFixtures()[4].wire()
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(bodyWire), wpadCorpusRule, map[string]any{"httpBodyLimit": 2}, "WPADRequest")
	require.ErrorContains(t, err, "body exceeds configured limit")
	node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(bodyWire), wpadCorpusRule, map[string]any{"httpBodyLimit": 2}, "WPADRequestCarrier")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, node, "Unparsed WPAD Request", bodyWire)
	require.Nil(t, protocolCorpusFindNode(node, "Method"))
	// The explicit HTTP config must cross both import levels, but not become
	// a new default in a later invocation of either entry.
	for _, entry := range []string{"WPADRequest", "WPADRequestCarrier"} {
		protocolCorpusRequireBoundedRuleParse(t, bodyWire, wpadCorpusRule, entry)
		for _, cut := range []int{1, 17, len(bodyWire) - 1} {
			root, err := base.ParseRule("application-layer/wpad.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(bodyWire))*8)
			reader := base.NewBitReader(bytes.NewReader(bodyWire[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery(), "failed inner probe must retain the outer backup")
			replayed, err := reader.ReadBits(uint64(cut) * 8)
			require.NoError(t, err)
			require.Equal(t, bodyWire[:cut], replayed)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
}
