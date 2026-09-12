package bin_parser

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusHL7Fields(t *testing.T, node *base.Node, wire []byte) int {
	t.Helper()
	require.EqualValues(t, 11, wire[0])
	require.True(t, bytes.HasSuffix(wire, []byte{13, 28, 13}))
	protocolCorpusRequireValue(t, node, "Start Block", uint64(11))
	protocolCorpusRequireValue(t, node, "End Block", uint64(0x1c0d))
	lines := strings.Split(string(wire[1:len(wire)-3]), "\r")
	segments := protocolCorpusFindNode(node, "Segments")
	require.NotNil(t, segments)
	require.Len(t, segments.Children, len(lines))
	for i, line := range lines {
		require.GreaterOrEqual(t, len(line), 4)
		protocolCorpusRequireValue(t, segments.Children[i], "Segment ID", line[:3])
		protocolCorpusRequireValue(t, segments.Children[i], "Field Separator", line[3:4])
		protocolCorpusRequireValue(t, segments.Children[i], "Fields", line[4:])
		fields := strings.Split(line[4:], line[3:4])
		info := segments.Children[i].Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, fields, info["Fields"], "every field, including empty trailing fields, must survive")
	}
	header := strings.Split(lines[0][4:], lines[0][3:4])
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	for key, index := range map[string]int{"Encoding Characters": 0, "Message Type": 7, "Message Control ID": 8, "Processing ID": 9, "Version": 10} {
		require.Equal(t, header[index], info[key])
	}
	return len(lines)
}

func TestProtocolCorpusHL7EveryCapturedRecordAndReassembledMessage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-hl7.pcap")
	require.Len(t, frames, 47)
	streams := map[string][]soapCorpusTCPSegment{}
	opening := map[string]uint32{}
	dataFrames, completeFrames, splitFrames := 0, 0, 0
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
		if tcp.SYN {
			if initial, ok := opening[flow]; ok {
				require.Equal(t, initial, tcp.Seq+1, "repeated opening records must agree")
			}
			opening[flow] = tcp.Seq + 1
		}
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		if len(tcp.Payload) == 0 {
			continue
		}
		dataFrames++
		streams[flow] = append(streams[flow], soapCorpusTCPSegment{frame: index + 1, seq: tcp.Seq, payload: bytes.Clone(tcp.Payload)})
		if tcp.Payload[0] == 11 && bytes.HasSuffix(tcp.Payload, []byte{28, 13}) {
			completeFrames++
			dispatched := protocolCorpusFindNode(envelope, "HL7")
			require.NotNil(t, dispatched, "frame %d", index+1)
			require.Len(t, dispatched.Children, 1)
			protocolCorpusHL7Fields(t, dispatched.Children[0], tcp.Payload)
		} else {
			splitFrames++
			require.Nil(t, protocolCorpusFindNode(envelope, "HL7"), "fragments must not be labeled complete messages")
			protocolCorpusRequireValue(t, envelope, "Remaining Payload", tcp.Payload)
		}
	}
	require.Equal(t, 11, dataFrames)
	require.Equal(t, 9, completeFrames)
	require.Equal(t, 2, splitFrames)
	require.Len(t, streams, 6)
	require.Len(t, opening, 6)
	keys := make([]string, 0, len(streams))
	for key := range streams {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var messages [][]byte
	types := map[string]int{}
	segmentCount, repeatedRecords, ambiguousDirections, alternateRecords := 0, 0, 0, 0
	for _, key := range keys {
		segments := streams[key]
		require.Equal(t, opening[key], segments[0].seq)
		seen := map[uint32][]byte{}
		conflicting := false
		for _, segment := range segments {
			if prior, ok := seen[segment.seq]; ok {
				if bytes.Equal(prior, segment.payload) {
					repeatedRecords++
				} else {
					conflicting = true
					alternateRecords++
				}
			}
			seen[segment.seq] = segment.payload
		}
		// This independent oracle rejects gaps and inconsistent overlap bytes.
		wire, err := soapCorpusReassembleTCP(segments)
		if conflicting {
			// Original frames 10-12 reuse the same TCP range for three different
			// complete requests. Validate every alternative without silently
			// selecting a first/last-wins interpretation of an ambiguous stream.
			ambiguousDirections++
			require.ErrorContains(t, err, "TCP overlap differs in frame 11")
			require.Len(t, segments, 3)
			for i, segment := range segments {
				require.Equal(t, 10+i, segment.frame)
				require.Equal(t, segments[0].seq, segment.seq)
				require.Len(t, segment.payload, 477)
				for _, prior := range segments[:i] {
					require.NotEqual(t, prior.payload, segment.payload)
				}
				messages = append(messages, segment.payload)
			}
			continue
		}
		require.NoError(t, err)
		messages = append(messages, wire)
	}
	for index, wire := range messages {
		t.Run(fmt.Sprintf("message-%d", index+1), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.hl7", "HL7Message")
			segmentCount += protocolCorpusHL7Fields(t, node, wire)
			types[node.Cfg.GetItem("additionInfo").(map[string]any)["Message Type"].(string)]++
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.hl7", "HL7Message")
				require.Errorf(t, err, "message cut %d/%d", cut, len(wire))
			}
		})
	}
	require.Equal(t, 2, repeatedRecords)
	require.Equal(t, 1, ambiguousDirections)
	require.Equal(t, 2, alternateRecords)
	require.Len(t, messages, 8)
	require.Equal(t, map[string]int{"ADT^A01": 3, "ORM^O01": 1, "ORU^R01": 1, "ACK": 3}, types)
	require.Equal(t, 46, segmentCount)
	// A synthetic sequence of all independently validated alternatives is not
	// asserted to be the uniquely reassembled original TCP conversation.
	sequence := protocolCorpusRequireBoundedRuleParse(t, bytes.Join(messages, nil), "application-layer.hl7", "HL7Messages")
	require.Len(t, sequence.Children, len(messages))
	for i, node := range sequence.Children {
		protocolCorpusHL7Fields(t, node, messages[i])
	}
}

func TestProtocolCorpusHL7DelimitersBoundsAndMessageIsolation(t *testing.T) {
	first := []byte("\x0bMSH|^~\\&|A|B|C|D|20260905||ACK|first|P|2.5\rMSA|AA|first|\r\x1c\r")
	second := []byte("\x0bMSH*$%!?*A*B*C*D*20260905**ACK*second*P*2.5\rMSA*AA*second*\r\x1c\r")
	sequence := protocolCorpusRequireBoundedRuleParse(t, append(bytes.Clone(first), second...), "application-layer.hl7", "HL7Messages")
	require.Len(t, sequence.Children, 2)
	protocolCorpusHL7Fields(t, sequence.Children[0], first)
	protocolCorpusHL7Fields(t, sequence.Children[1], second)
	reader := newProtocolCorpusBoundedReader(append(bytes.Clone(first), second...))
	for _, wire := range [][]byte{first, second} {
		node, err := parser.ParseBinary(reader, "application-layer.hl7", "HL7Message")
		require.NoError(t, err)
		protocolCorpusHL7Fields(t, node, wire)
	}
	require.Zero(t, reader.Len())
	for _, bad := range [][]byte{
		bytes.Replace(first, []byte("MSH"), []byte("PID"), 1),
		bytes.Replace(first, []byte("^~\\&"), []byte("^^\\&"), 1),
		bytes.Replace(first, []byte("MSA|"), []byte("MSA*"), 1),
		bytes.Replace(first, []byte("first|P"), []byte("|P"), 1),
		bytes.Replace(first, []byte("MSA"), []byte("M!A"), 1),
		bytes.Replace(first, []byte("AA|"), []byte("A\x00|"), 1),
		[]byte("\x0b\x1c\r"),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.hl7", "HL7Message")
		require.Error(t, err)
	}
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(first), "application-layer.hl7", map[string]any{"hl7MessageLimit": len(first) - 1}, "HL7Message")
	require.Error(t, err)
}
