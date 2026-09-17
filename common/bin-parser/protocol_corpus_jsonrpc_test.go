package bin_parser

import (
	"bufio"
	"bytes"
	"encoding/json"
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

func protocolCorpusJSONValueFields(t *testing.T, value *stream_parser.JSONValue, raw json.RawMessage) {
	t.Helper()
	require.Equal(t, strings.TrimSpace(string(raw)), value.Raw)
	switch value.Kind {
	case "object":
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		require.Len(t, value.Members, len(fields))
		require.Len(t, value.Keys, len(fields))
		for key, child := range fields {
			require.NotNil(t, value.Members[key])
			protocolCorpusJSONValueFields(t, value.Members[key], child)
		}
	case "array":
		var items []json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &items))
		require.Len(t, value.Elements, len(items))
		for i, item := range items {
			protocolCorpusJSONValueFields(t, value.Elements[i], item)
		}
	case "string":
		var text string
		require.NoError(t, json.Unmarshal(raw, &text))
		require.Equal(t, text, value.StringValue)
	case "number":
		var number json.Number
		require.NoError(t, json.Unmarshal(raw, &number))
		require.Equal(t, number.String(), value.NumberValue)
	case "boolean":
		var truth bool
		require.NoError(t, json.Unmarshal(raw, &truth))
		require.Equal(t, truth, value.BoolValue)
	case "null":
		require.Equal(t, "null", value.Raw)
	default:
		t.Fatalf("unrecognized JSON value kind %q", value.Kind)
	}
}

func TestProtocolCorpusJSONRPCEveryOldAndNewRecordAndMessage(t *testing.T) {
	frameCount, bodyCount, httpCount := 0, 0, 0
	var methods []string
	for _, fixture := range []struct{ id, folder string }{{"ndpi-jsonrpc", "ndpi"}, {"gen-jsonrpc", "generated-local"}} {
		t.Run(fixture.id, func(t *testing.T) {
			frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/"+fixture.folder+"/"+fixture.id+".pcap")
			streams := map[string][]soapCorpusTCPSegment{}
			opening := map[string]uint32{}
			for i, frame := range frames {
				frameCount++
				packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
				flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
				if tcp.SYN {
					opening[flow] = tcp.Seq + 1
				}
				if len(tcp.Payload) > 0 {
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
				body := wire
				if bytes.HasPrefix(wire, []byte("POST ")) || bytes.HasPrefix(wire, []byte("HTTP/")) {
					httpCount++
					node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.http", "HTTP")
					reader := bufio.NewReader(bytes.NewReader(wire))
					var contents io.ReadCloser
					if bytes.HasPrefix(wire, []byte("HTTP/")) {
						response, err := http.ReadResponse(reader, nil)
						require.NoError(t, err)
						contents = response.Body
					} else {
						request, err := http.ReadRequest(reader)
						require.NoError(t, err)
						contents = request.Body
					}
					body, err = io.ReadAll(contents)
					require.NoError(t, err)
					require.NoError(t, contents.Close())
					protocolCorpusRequireValue(t, node, "Octets", string(body))
					remaining, err := io.ReadAll(reader)
					require.NoError(t, err)
					require.Empty(t, remaining)
				}
				bodyCount++
				node := protocolCorpusRequireBoundedRuleParse(t, body, "application-layer.jsonrpc_v2", "JSONRPC2")
				protocolCorpusRequireValue(t, node, "JSON Text", string(body))
				value := node.Cfg.GetItem("additionInfo").(map[string]any)["JSON Value"].(*stream_parser.JSONValue)
				protocolCorpusJSONValueFields(t, value, body)
				if method := value.Members["method"]; method != nil {
					methods = append(methods, method.StringValue)
				}
				for cut := 0; cut < len(body); cut++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body[:cut]), "application-layer.jsonrpc_v2", "JSONRPC2")
					require.Errorf(t, err, "message cut %d/%d", cut, len(body))
				}
			}
		})
	}
	require.Equal(t, 20, frameCount)
	require.Equal(t, 4, bodyCount)
	require.Equal(t, 3, httpCount)
	require.Contains(t, methods, "echo")
	require.Contains(t, methods, "subtract")
	require.Len(t, methods, 3)
}

func TestProtocolCorpusJSONRPC2NestedValuesKindsAndLimits(t *testing.T) {
	batch := []byte(`[{"jsonrpc":"2.0","method":"echo","params":[42,23,{"escaped":"a\"b\\c\u4e2d","nested":[false,null,{},[]]}],"id":9007199254740993},{"jsonrpc":"2.0","method":"notify","params":{}},{"jsonrpc":"2.0","result":null,"id":"x"},{"jsonrpc":"2.0","error":{"code":-32601,"message":"missing","data":[true,1e400]},"id":null}]`)
	node := protocolCorpusRequireBoundedRuleParse(t, batch, "application-layer.jsonrpc_v2", "JSONRPC2")
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["Batch"])
	require.Equal(t, []string{"request", "notification", "success", "error"}, info["Message Kinds"])
	protocolCorpusJSONValueFields(t, info["JSON Value"].(*stream_parser.JSONValue), batch)
	for _, port := range []layers.TCPPort{8080, 8545} {
		envelope := protocolCorpusRequireBoundedRuleParse(t, ipv4TCPFrame(t, 40002, port, batch), "ethernet", "Ethernet")
		dispatched := protocolCorpusFindNode(envelope, "JSONRPC2")
		require.NotNil(t, dispatched)
		protocolCorpusJSONValueFields(t, dispatched.Cfg.GetItem("additionInfo").(map[string]any)["JSON Value"].(*stream_parser.JSONValue), batch)
	}
	for i, bad := range []string{`[]`, `[1]`, `{}`, `{"jsonrpc":"1.0","method":"x"}`, `{"jsonrpc":"2.0","method":3}`, `{"jsonrpc":"2.0","method":"x","params":1}`, `{"jsonrpc":"2.0","method":"x","id":true}`, `{"jsonrpc":"2.0","result":true}`, `{"jsonrpc":"2.0","result":true,"error":{},"id":1}`, `{"jsonrpc":"2.0","error":{"code":1.5,"message":"bad"},"id":1}`, `{"jsonrpc":"2.0","error":null,"id":1}`, `{"jsonrpc":"2.0","method":"x","result":0}`, `{"jsonrpc":"2.0","method":"x","method":"y"}`} {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(bad)), "application-layer.jsonrpc_v2", "JSONRPC2")
			require.Error(t, err)
		})
	}
	for _, config := range []map[string]any{{"jsonrpcMessageLimit": 0}, {"jsonrpcMessageLimit": len(batch) - 1}, {"jsonrpcDepthLimit": 2}} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(batch), "application-layer.jsonrpc_v2", config, "JSONRPC2")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewReader(batch), "application-layer.jsonrpc_v2", "JSONRPC2")
	require.ErrorContains(t, err, "explicit boundary")
	// Parsing an invalid message after a valid batch cannot reuse its metadata.
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(`{"jsonrpc":"2.0"}`)), "application-layer.jsonrpc_v2", "JSONRPC2")
	require.Error(t, err)
}

func TestProtocolCorpusLegacyJSONRPCDoesNotRetainMessageState(t *testing.T) {
	first := []byte(`{"jsonrpc":"2.0","method":"first","id":1}`)
	second := []byte(`{"jsonrpc":"2.0","result":"second","id":2}`)
	reader := bytes.NewReader(append(bytes.Clone(first), second...))
	for _, wire := range [][]byte{first, second} {
		node, err := parser.ParseBinary(reader, "jsonrpc", "JSONRPC")
		require.NoError(t, err)
		protocolCorpusJSONValueFields(t, node.Cfg.GetItem("additionInfo").(map[string]any)["JSON Value"].(*stream_parser.JSONValue), wire)
	}
	require.Zero(t, reader.Len())
	for _, bad := range []string{`{"jsonrpc":"1.0","method":"x"}`, `{"jsonrpc":"2.0","method":123}`, `{"jsonrpc":"2.0","method":"x","params":1}`, `{"jsonrpc":"2.0","method":"x","id":true}`, `{"jsonrpc":"2.0","result":"x"}`, `{"jsonrpc":"2.0","method":"x","id":NaN}`, `{"jsonrpc":"2.0","method":"x","method":"y"}`} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(bad)), "jsonrpc", "JSONRPC")
		require.Error(t, err)
	}
}
