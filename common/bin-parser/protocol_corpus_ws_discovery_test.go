package bin_parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestProtocolCorpusWSDiscoveryEveryRecordAndFields(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-wsd-original.pcap")
	require.Equal(t, 14, len(frames))
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			offset := 42
			if index%2 == 0 {
				ipv6 := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
				require.Equal(t, uint16(664), ipv6.Length)
				require.Equal(t, layers.UDPPort(58932), udp.SrcPort)
				offset = 62
			} else {
				ipv4 := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
				require.Equal(t, uint16(684), ipv4.Length)
				require.Equal(t, layers.UDPPort(58931), udp.SrcPort)
			}
			require.Len(t, frame, offset+656)
			require.Equal(t, layers.UDPPort(3702), udp.DstPort)
			require.Equal(t, uint16(664), udp.Length)
			require.Len(t, udp.Payload, 656)
			require.Equal(t, frame[offset:], udp.Payload)
			// An independent namespace-aware standard-library decode cross-checks
			// the values actually present in this original, not a guessed reply.
			var expected struct {
				XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
				Header  struct {
					Action    string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Action"`
					MessageID string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
					To        string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing To"`
				} `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
				Body struct {
					Resolve struct {
						EPR struct {
							Address string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Address"`
						} `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing EndpointReference"`
					} `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Resolve"`
				} `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
			}
			require.NoError(t, xml.Unmarshal(udp.Payload, &expected))
			node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, "application-layer.ws_discovery", "WSDiscovery")
			protocolCorpusRequireValue(t, node, "XML Text", string(udp.Payload))
			info := node.Cfg.GetItem("additionInfo").(map[string]any)
			message := info["WS-Discovery Message"].(*stream_parser.WSDiscoveryMessage)
			require.Equal(t, "Resolve", message.Kind)
			require.Equal(t, "2005/04", message.Version)
			require.Equal(t, "http://schemas.xmlsoap.org/ws/2005/04/discovery/Resolve", message.Header.Action)
			require.Equal(t, "urn:uuid:3f42dc9a-24ce-48d1-88f9-16b96a137d71", message.Header.MessageID)
			require.Equal(t, "urn:schemas-xmlsoap-org:ws:2005:04:discovery", message.Header.To)
			require.Equal(t, expected.Header.Action, message.Header.Action)
			require.Equal(t, expected.Header.MessageID, message.Header.MessageID)
			require.Equal(t, expected.Header.To, message.Header.To)
			require.Len(t, message.Endpoints, 1)
			require.Equal(t, "urn:uuid:e3248000-80ce-11db-8000-001ba99ec956", message.Endpoints[0].EPR.Address)
			require.Equal(t, expected.Body.Resolve.EPR.Address, message.Endpoints[0].EPR.Address)
			require.Nil(t, message.Header.ReplyTo)
			require.Nil(t, message.Header.AppSequence)
			require.Empty(t, message.Header.RelatesTo)
			require.Empty(t, message.Extensions)
			require.Nil(t, message.Endpoints[0].MetadataVersion)
			require.Empty(t, message.Endpoints[0].XAddrs)
			require.False(t, message.Endpoints[0].TypesPresent)
			require.False(t, message.Endpoints[0].ScopesPresent)
			require.False(t, message.Endpoints[0].XAddrsPresent)
			require.Equal(t, message.Header.Action, info["Action"])
			require.Equal(t, message.Header.MessageID, info["Message ID"])
			// The public L2 entry must dispatch both IP families through UDP
			// port 3702 into this same structured decoder, not a raw fallback.
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			discovery := protocolCorpusFindNode(envelope, "WSDiscovery")
			require.NotNil(t, discovery)
			publicMessage := discovery.Cfg.GetItem("additionInfo").(map[string]any)["WS-Discovery Message"].(*stream_parser.WSDiscoveryMessage)
			require.Equal(t, message, publicMessage)
			for cut := 0; cut < len(udp.Payload); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload[:cut]), "application-layer.ws_discovery", "WSDiscovery")
				require.Errorf(t, err, "accepted prefix %d/%d", cut, len(udp.Payload))
			}
		})
	}
}

func TestProtocolCorpusWSDiscoveryRuleBoundariesAndConcurrentVariants(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-wsd-original.pcap")
	text := string(frames[0][62:])
	for _, config := range []map[string]any{
		{"wsDiscoveryMessageLimit": 0},
		{"wsDiscoveryMessageLimit": len(text) - 1},
		{"wsDiscoveryDepthLimit": 4},
		{"wsDiscoveryDepthLimit": 129},
	} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader([]byte(text)), "application-layer.ws_discovery", config, "WSDiscovery")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(bytes.NewReader([]byte(text)), "application-layer.ws_discovery", "WSDiscovery")
	require.ErrorContains(t, err, "explicit boundary")
	for _, invalid := range []string{
		text + text,
		strings.Replace(text, "/Resolve</wsa:Action>", "/Probe</wsa:Action>", 1),
		strings.Replace(text, "urn:uuid:e3248000-80ce-11db-8000-001ba99ec956", "", 1),
		strings.Replace(text, "2005/04/discovery", "2009/01/discovery", -1),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(invalid)), "application-layer.ws_discovery", "WSDiscovery")
		require.Error(t, err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for repeat := 0; repeat < 8; repeat++ {
				address := fmt.Sprintf("urn:example:endpoint:%d:%d", worker, repeat)
				variant := strings.Replace(text, "urn:uuid:e3248000-80ce-11db-8000-001ba99ec956", address, 1)
				reader := newProtocolCorpusBoundedReader([]byte(variant))
				node, err := parser.ParseBinary(reader, "application-layer.ws_discovery", "WSDiscovery")
				if err != nil {
					t.Errorf("variant parse: %v", err)
					return
				}
				message := node.Cfg.GetItem("additionInfo").(map[string]any)["WS-Discovery Message"].(*stream_parser.WSDiscoveryMessage)
				if reader.Len() != 0 || message.Endpoints[0].EPR.Address != address {
					t.Errorf("variant boundary or isolation mismatch")
				}
			}
		}(worker)
	}
	wg.Wait()
}
