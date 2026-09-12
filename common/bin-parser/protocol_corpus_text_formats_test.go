package bin_parser

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestProtocolCorpusPrometheusAndXMLRPCEveryRecordAndBody(t *testing.T) {
	frameCount, httpCount, expositionCount, xmlCount := 0, 0, 0, 0
	for _, id := range []string{"gen-prometheus", "gen-xmlrpc"} {
		t.Run(id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/"+id+".pcap")
			streams := map[string][]soapCorpusTCPSegment{}
			opening := map[string]uint32{}
			for i, frame := range frames {
				frameCount++
				envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				protocolCorpusRequireValue(t, envelope, "Source Port", uint64(tcp.SrcPort))
				protocolCorpusRequireValue(t, envelope, "Destination Port", uint64(tcp.DstPort))
				flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
				if tcp.SYN {
					opening[flow] = tcp.Seq + 1
				}
				if len(tcp.Payload) != 0 {
					streams[flow] = append(streams[flow], soapCorpusTCPSegment{frame: i + 1, seq: tcp.Seq, payload: bytes.Clone(tcp.Payload)})
				}
			}
			keys := make([]string, 0, len(streams))
			for key := range streams {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				segments := streams[key]
				require.Equal(t, opening[key], segments[0].seq)
				wire, err := soapCorpusReassembleTCP(segments)
				require.NoError(t, err)
				// Every original message also survives re-segmentation, reversed
				// arrival order and matching retransmission at every byte boundary.
				for split := 1; split < len(wire); split++ {
					reassembled, err := soapCorpusReassembleTCP([]soapCorpusTCPSegment{
						{frame: 2, seq: opening[key] + uint32(split), payload: wire[split:]},
						{frame: 1, seq: opening[key], payload: wire[:split]},
						{frame: 3, seq: opening[key], payload: wire[:split]},
					})
					require.NoError(t, err)
					require.Equal(t, wire, reassembled)
				}
				httpCount++
				node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.http", "HTTP")
				reader := bufio.NewReader(bytes.NewReader(wire))
				var bodyReader io.ReadCloser
				var contentType string
				if bytes.HasPrefix(wire, []byte("HTTP/")) {
					response, err := http.ReadResponse(reader, nil)
					require.NoError(t, err)
					require.Equal(t, "gen-prometheus", id)
					require.Equal(t, 200, response.StatusCode)
					require.Equal(t, int64(37), response.ContentLength)
					bodyReader, contentType = response.Body, response.Header.Get("Content-Type")
				} else {
					request, err := http.ReadRequest(reader)
					require.NoError(t, err)
					if id == "gen-prometheus" {
						require.Equal(t, "GET", request.Method)
						require.Equal(t, "/metrics", request.RequestURI)
					} else {
						require.Equal(t, "POST", request.Method)
						require.Equal(t, int64(85), request.ContentLength)
					}
					bodyReader, contentType = request.Body, request.Header.Get("Content-Type")
				}
				body, err := io.ReadAll(bodyReader)
				require.NoError(t, err)
				require.NoError(t, bodyReader.Close())
				trailing, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Empty(t, trailing)
				if len(body) == 0 {
					require.Equal(t, "gen-prometheus", id)
					continue // GET does not prove any exposition fields.
				}
				protocolCorpusRequireValue(t, node, "Octets", string(body))
				if id == "gen-prometheus" {
					expositionCount++
					require.Equal(t, "text/plain; version=0.0.4", contentType)
					node = protocolCorpusRequireBoundedRuleParse(t, body, "application-layer.prometheus", "PrometheusExposition")
					protocolCorpusRequireValue(t, node, "Exposition Text", string(body))
					lines := node.Cfg.GetItem("additionInfo").(map[string]any)["Lines"].([]*stream_parser.PrometheusLine)
					require.Equal(t, []*stream_parser.PrometheusLine{
						{Text: "# TYPE cpu_usage gauge", Kind: "type", Name: "cpu_usage", MetricType: "gauge"},
						{Text: "cpu_usage 0.2", Kind: "sample", Name: "cpu_usage", ValueText: "0.2", Value: 0.2},
					}, lines)
					// A line-aligned prefix is a valid, shorter document in this
					// format. HTTP Content-Length is the actual completeness proof.
					for cut := 0; cut < len(body); cut++ {
						_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(body[:cut]), "application-layer.prometheus", map[string]any{"prometheusMessageLength": len(body)}, "PrometheusExposition")
						require.Error(t, err)
					}
				} else {
					xmlCount++
					require.Equal(t, "text/xml", contentType)
					node = protocolCorpusRequireBoundedRuleParse(t, body, "application-layer.xmlrpc", "XMLRPC")
					protocolCorpusRequireValue(t, node, "XML Text", string(body))
					message := node.Cfg.GetItem("additionInfo").(map[string]any)["XML-RPC Message"].(*stream_parser.XMLRPCMessage)
					// Independent standard-library struct decoding of the actual
					// capture: it contains a method name and no params, not a result.
					var expected struct {
						XMLName xml.Name   `xml:"methodCall"`
						Method  string     `xml:"methodName"`
						Params  []struct{} `xml:"params>param"`
					}
					require.NoError(t, xml.Unmarshal(body, &expected))
					require.Equal(t, "sys.methodHelp", expected.Method)
					require.Equal(t, expected.Method, message.Method)
					require.Equal(t, "request", message.Kind)
					require.Len(t, message.Params, len(expected.Params))
					require.Empty(t, message.Params)
					require.Nil(t, message.Fault)
					for cut := 0; cut < len(body); cut++ {
						_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body[:cut]), "application-layer.xmlrpc", "XMLRPC")
						require.Error(t, err)
					}
				}
			}
		})
	}
	require.Equal(t, 9, frameCount)
	require.Equal(t, 3, httpCount)
	require.Equal(t, 1, expositionCount)
	require.Equal(t, 1, xmlCount)
}

func TestProtocolCorpusTextFormatRuleVariantsAndLimits(t *testing.T) {
	xmlBody := []byte(`<methodCall><methodName>read</methodName><params><param><value><array><data><value><int>42</int></value><value><struct><member><name>label</name><value><string>中&amp;文</string></value></member></struct></value></data></array></value></param></params></methodCall>`)
	node := protocolCorpusRequireBoundedRuleParse(t, xmlBody, "application-layer.xmlrpc", "XMLRPC")
	message := node.Cfg.GetItem("additionInfo").(map[string]any)["XML-RPC Message"].(*stream_parser.XMLRPCMessage)
	require.Len(t, message.Params, 1)
	require.Equal(t, int32(42), message.Params[0].Elements[0].IntValue)
	require.Equal(t, "中&文", message.Params[0].Elements[1].Members[0].Value.Text)
	promBody := []byte("# HELP temp Temperature\n# TYPE temp gauge\n" + `temp{zone="a\nb"} -12.25 123` + "\n")
	node = protocolCorpusRequireBoundedRuleParse(t, promBody, "application-layer.prometheus", "PrometheusExposition")
	lines := node.Cfg.GetItem("additionInfo").(map[string]any)["Lines"].([]*stream_parser.PrometheusLine)
	require.Len(t, lines, 3)
	require.Equal(t, "Temperature", lines[0].Help)
	require.Equal(t, "a\nb", lines[2].Labels[0].Value)
	require.Equal(t, int64(123), lines[2].Timestamp)
	for _, fixture := range []struct {
		rule, entry string
		body        []byte
		bad         string
		configs     []map[string]any
	}{
		{"application-layer.xmlrpc", "XMLRPC", xmlBody, `<methodCall/>`, []map[string]any{{"xmlrpcMessageLimit": 0}, {"xmlrpcMessageLimit": len(xmlBody) - 1}, {"xmlrpcDepthLimit": 3}}},
		{"application-layer.prometheus", "PrometheusExposition", promBody, "a 1\na 2\n", []map[string]any{{"prometheusMessageLimit": 0}, {"prometheusMessageLimit": len(promBody) - 1}, {"prometheusLineLimit": 3}, {"prometheusMessageLength": len(promBody) + 1}}},
	} {
		t.Run(fixture.entry, func(t *testing.T) {
			_, err := parser.ParseBinary(bytes.NewReader(fixture.body), fixture.rule, fixture.entry)
			require.ErrorContains(t, err, "explicit boundary")
			for i, config := range fixture.configs {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(fixture.body), fixture.rule, config, fixture.entry)
				require.Error(t, err, fmt.Sprint(i))
			}
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(fixture.bad)), fixture.rule, fixture.entry)
			require.Error(t, err)
			protocolCorpusRequireBoundedRuleParse(t, fixture.body, fixture.rule, fixture.entry)
		})
	}
	// Parsing a second exposition starts new per-document HELP/TYPE/series sets.
	for i := 0; i < 2; i++ {
		protocolCorpusRequireBoundedRuleParse(t, promBody, "application-layer.prometheus", "PrometheusExposition")
	}
	// Whole-document XML ignores trailing XML whitespace, but never another root.
	protocolCorpusRequireBoundedRuleParse(t, append(bytes.Clone(xmlBody), []byte("\n\t")...), "application-layer.xmlrpc", "XMLRPC")
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(strings.Repeat(string(xmlBody), 2))), "application-layer.xmlrpc", "XMLRPC")
	require.Error(t, err)
}
