package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusHiSLIPFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	protocolCorpusRequireValue(t, node, "Prologue", "HS")
	protocolCorpusRequireValue(t, node, "Message Type", uint64(wire[2]))
	protocolCorpusRequireValue(t, node, "Control Code", uint64(wire[3]))
	parameter := binary.BigEndian.Uint32(wire[4:])
	protocolCorpusRequireValue(t, node, "Parameter", uint64(parameter))
	protocolCorpusRequireValue(t, node, "Payload Length", uint64(len(wire)-16))
	message := protocolCorpusFindNode(node, "HiSLIP")
	if message == nil {
		message = node
	}
	info, _ := message.Cfg.GetItem("additionInfo").(map[string]any)
	switch wire[2] {
	case 0:
		require.EqualValues(t, parameter>>16, info["Protocol Version"])
		require.EqualValues(t, parameter&0xffff, info["Vendor ID"])
	case 1:
		require.EqualValues(t, parameter>>16, info["Protocol Version"])
		require.EqualValues(t, parameter&0xffff, info["Session ID"])
	case 17:
		require.EqualValues(t, parameter&0xffff, info["Session ID"])
	case 18:
		require.EqualValues(t, parameter&0xffff, info["Vendor ID"])
	case 4:
		key := "Message ID"
		if wire[3] == 1 {
			key = "Lock Timeout"
		}
		require.EqualValues(t, parameter, info[key])
	case 6, 7, 10, 12, 13, 14, 21:
		require.EqualValues(t, parameter, info["Message ID"])
	case 20, 22:
		require.EqualValues(t, wire[3], info["Status Byte"])
	case 25:
		require.EqualValues(t, parameter, info["Client Count"])
	}
	if wire[2] == 15 || wire[2] == 16 {
		require.Len(t, wire, 24)
		protocolCorpusRequireValue(t, node, "Maximum Message Size", binary.BigEndian.Uint64(wire[16:]))
	} else if len(wire) > 16 {
		protocolCorpusRequireValue(t, node, "Payload", wire[16:])
	}
}

func TestProtocolCorpusHiSLIPEveryCapturedMessage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-hislip.pcap")
	require.Len(t, frames, 184)
	nextSequence := map[string]uint32{}
	kinds := map[byte]int{}
	var messages [][]byte
	probes := 0
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
		if tcp.SYN {
			nextSequence[flow] = tcp.Seq + 1
		}
		if len(tcp.Payload) == 0 {
			require.Nil(t, protocolCorpusFindNode(envelope, "HiSLIP"), "transport-only frames have no HiSLIP message")
			continue
		}
		expected, ok := nextSequence[flow]
		require.True(t, ok, "every direction must have an observed opening sequence")
		if len(tcp.Payload) == 1 && tcp.Seq == expected-1 && tcp.ACK && !tcp.SYN && !tcp.FIN && !tcp.RST {
			// These pinned zero-byte keepalive probes do not add stream bytes.
			require.Equal(t, []byte{0}, tcp.Payload)
			require.Nil(t, protocolCorpusFindNode(envelope, "HiSLIP"), "a TCP probe is not a HiSLIP message")
			protocolCorpusRequireValue(t, envelope, "Remaining Payload", []byte{0})
			protocolCorpusRequireValue(t, envelope, "Frame Trailer", make([]byte, 5))
			probes++
			continue
		}
		require.Equal(t, expected, tcp.Seq, "frame %d has an unaccounted stream gap or retransmission", index+1)
		nextSequence[flow] += uint32(len(tcp.Payload))
		wire := tcp.Payload
		require.GreaterOrEqual(t, len(wire), 16)
		require.Equal(t, len(wire), 16+int(binary.BigEndian.Uint64(wire[8:])), "pinned capture has one complete PDU per data segment")
		messages = append(messages, bytes.Clone(wire))
		kinds[wire[2]]++
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.hislip", "HiSLIP")
			protocolCorpusHiSLIPFields(t, node, wire)
			dispatched := protocolCorpusFindNode(envelope, "HiSLIP")
			require.NotNil(t, dispatched, "complete messages must also decode through the TCP dispatcher")
			require.Len(t, dispatched.Children, 1)
			protocolCorpusHiSLIPFields(t, dispatched.Children[0], wire)
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.hislip", "HiSLIP")
				require.Errorf(t, err, "cut %d", cut)
			}
		})
	}
	joined := protocolCorpusRequireBoundedRuleParse(t, bytes.Join(messages, nil), "application-layer.hislip", "HiSLIPMessages")
	require.Len(t, joined.Children, len(messages))
	for index, node := range joined.Children {
		protocolCorpusHiSLIPFields(t, node, messages[index])
	}
	require.Len(t, messages, 85)
	require.Equal(t, 15, probes)
	require.Len(t, nextSequence, 8)
	require.Equal(t, map[byte]int{0: 2, 1: 2, 4: 9, 5: 9, 7: 24, 12: 1, 13: 5, 14: 5, 15: 2, 16: 2, 17: 2, 18: 2, 21: 7, 22: 7, 24: 3, 25: 3}, kinds)
	t.Logf("validated %d HiSLIP messages and %d sequence-checked TCP probes across %d directions; message kinds %v", len(messages), probes, len(nextSequence), kinds)
}

func TestProtocolCorpusHiSLIPBoundsAndVariants(t *testing.T) {
	makeMessage := func(kind, control byte, parameter uint32, data []byte) []byte {
		wire := make([]byte, 16, 16+len(data))
		copy(wire, "HS")
		wire[2], wire[3] = kind, control
		binary.BigEndian.PutUint32(wire[4:], parameter)
		binary.BigEndian.PutUint64(wire[8:], uint64(len(data)))
		return append(wire, data...)
	}
	valid := [][]byte{
		makeMessage(0, 0, 0x01011234, []byte("device2")),
		makeMessage(1, 1, 0x0101abcd, nil),
		makeMessage(4, 1, 123456, []byte("shared-key")),
		makeMessage(6, 1, 0x12345678, []byte("part one")),
		makeMessage(7, 0, 0x12345678, []byte("part two")),
		makeMessage(25, 1, 7, nil),
		makeMessage(15, 0, 0, []byte{0, 0, 0, 1, 0, 0, 0, 0}),
	}
	for _, wire := range valid {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.hislip", "HiSLIP")
		protocolCorpusHiSLIPFields(t, node, wire)
	}
	for _, wire := range [][]byte{
		makeMessage(0, 0, 0x02000000, nil),
		makeMessage(4, 0, 1, []byte("unexpected")),
		makeMessage(15, 0, 0, []byte{1, 2}),
		makeMessage(17, 0, 0xffffabcd, nil),
		makeMessage(21, 2, 0, nil),
		makeMessage(24, 0, 1, nil),
		makeMessage(28, 0, 0, nil),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.hislip", "HiSLIP")
		require.Errorf(t, err, "type=%d control=%d parameter=%08x length=%d", wire[2], wire[3], binary.BigEndian.Uint32(wire[4:]), len(wire)-16)
	}
	for _, length := range []uint64{1 << 32, 1 << 63, ^uint64(0)} {
		tooLong := makeMessage(7, 0, 1, []byte("data"))
		binary.BigEndian.PutUint64(tooLong[8:], length)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tooLong), "application-layer.hislip", "HiSLIP")
		require.ErrorContains(t, err, "configured limit")
	}
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(valid[0]), "application-layer.hislip", map[string]any{"hislipPayloadLimit": 3}, "HiSLIP")
	require.ErrorContains(t, err, "configured limit")
}

func TestProtocolCorpusHiSLIPCallerLimitReachesNestedDispatcher(t *testing.T) {
	wire := append([]byte{'H', 'S', 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4}, []byte("inst")...)
	frame := ipv4TCPFrame(t, 41000, 4880, wire)
	for _, limit := range []int{3, 4, 0, 5} {
		config := map[string]any{"hislipPayloadLimit": limit}
		node := protocolCorpusRequireBoundedRuleParseWithConfig(t, frame, "ethernet", "Ethernet", config)
		message := protocolCorpusFindNode(node, "HiSLIP")
		if limit >= 4 {
			require.NotNil(t, message)
			require.Len(t, message.Children, 1)
			protocolCorpusHiSLIPFields(t, message.Children[0], wire)
		} else {
			require.Nil(t, message)
			protocolCorpusRequireValue(t, node, "Remaining Payload", wire)
		}
	}
	// A limit from an earlier call cannot change another caller's defaults.
	node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
	require.NotNil(t, protocolCorpusFindNode(node, "HiSLIP"))
}
