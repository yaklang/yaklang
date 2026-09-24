package bin_parser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestProtocolCorpusLLMNREveryCapturedQuery(t *testing.T) {
	checked := 0
	for _, file := range []string{"gen-llmnr.pcap", "gen-llmnr-mdns.pcap"} {
		frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-local/"+file)
		for _, frame := range frames {
			packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, packet.ErrorLayer())
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			var oracle layers.DNS
			require.NoError(t, oracle.DecodeFromBytes(udp.Payload, gopacket.NilDecodeFeedback))
			rule, entry := "application-layer.nbns", "LLMNR"
			if udp.DstPort == 5353 {
				rule, entry = "application-layer.dns", "DNS"
			} else {
				require.Equal(t, layers.UDPPort(5355), udp.DstPort)
			}
			node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, rule, entry)
			for name, value := range map[string]uint64{"ID": uint64(oracle.ID), "Flags": uint64(binary.BigEndian.Uint16(udp.Payload[2:4])), "Questions": uint64(oracle.QDCount), "Answer RRs": uint64(oracle.ANCount), "Authority RRs": uint64(oracle.NSCount), "Additional RRs": uint64(oracle.ARCount)} {
				protocolCorpusRequireValue(t, protocolCorpusFindNode(node, "Header"), name, value)
			}
			questions := protocolCorpusNodesNamed(node, "Question")
			require.Len(t, questions, len(oracle.Questions))
			for i, question := range questions {
				protocolCorpusRequireValue(t, question, "Type", uint64(oracle.Questions[i].Type))
				protocolCorpusRequireValue(t, question, "Class", uint64(oracle.Questions[i].Class))
				labels := protocolCorpusNodesNamed(question, "Text")
				want := strings.Split(string(oracle.Questions[i].Name), ".")
				require.Len(t, labels, len(want))
				for j, label := range labels {
					protocolCorpusRequireValue(t, label, "Text", want[j])
				}
			}
			// Also follow the actual transport dispatch, not only a direct entry.
			protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			for cut := 0; cut < len(udp.Payload); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload[:cut]), rule, entry)
				require.Error(t, err)
			}
			checked++
		}
	}
	require.Equal(t, 3, checked)
}
