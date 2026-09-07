package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusEGDFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	protocolCorpusRequireValue(t, node, "PDU Type", uint64(wire[0]))
	protocolCorpusRequireValue(t, node, "Version", uint64(wire[1]))
	protocolCorpusRequireValue(t, node, "Request ID", uint64(binary.LittleEndian.Uint16(wire[2:])))
	protocolCorpusRequireValue(t, node, "Producer ID", wire[4:8])
	for name, offset := range map[string]int{"Exchange ID": 8, "Timestamp Seconds": 12, "Timestamp Nanoseconds": 16, "Status": 20, "Configuration Signature": 24, "Reserved": 28} {
		protocolCorpusRequireValue(t, node, name, uint64(binary.LittleEndian.Uint32(wire[offset:])))
	}
	if len(wire) > 32 {
		protocolCorpusRequireValue(t, node, "Exchange Data", wire[32:])
	}
}

func TestProtocolCorpusEGDEveryDatagram(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-egd.pcapng")
	require.Len(t, frames, 5)
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, layers.UDPPort(18246), udp.DstPort)
		require.Len(t, udp.Payload, 59)
		require.EqualValues(t, 0xf0d4+index, binary.LittleEndian.Uint16(udp.Payload[2:]))
		node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, "application-layer.egd", "EGD")
		protocolCorpusEGDFields(t, node, udp.Payload)
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		dispatched := protocolCorpusFindNode(envelope, "EGD")
		require.NotNil(t, dispatched)
		protocolCorpusEGDFields(t, dispatched, udp.Payload)
		for cut := 0; cut < len(udp.Payload); cut++ {
			// Data size is externally configured, not encoded in an EGD header.
			_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(udp.Payload[:cut]), "application-layer.egd", map[string]any{"egdExchangeDataLength": 27}, "EGD")
			require.Errorf(t, err, "frame %d cut %d", index+1, cut)
		}
	}
	// Independent nonzero values exercise byte order despite the many zero
	// fields in the upstream capture. Exchange payload bytes are not decoded.
	wire := make([]byte, 35)
	wire[0], wire[1] = 13, 1
	for i := 2; i < len(wire); i++ {
		wire[i] = byte(i)
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.egd", "EGD")
	protocolCorpusEGDFields(t, node, wire)
	for _, offset := range []int{0, 1} {
		bad := bytes.Clone(wire)
		bad[offset] ^= 0x80
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.egd", "EGD")
		require.ErrorContains(t, err, "unsupported data message")
	}
	_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.egd", "EGD")
	require.ErrorContains(t, err, "boundary is required")
	protocolCorpusRequireBoundedRuleParse(t, wire[:32], "application-layer.egd", "EGD")
}

func protocolCorpusTRDPFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	for name, offset := range map[string]int{"Sequence Counter": 0, "Communication ID": 8, "ETB Topology Counter": 12, "Operational Topology Counter": 16, "Dataset Length": 20} {
		protocolCorpusRequireValue(t, node, name, uint64(binary.BigEndian.Uint32(wire[offset:])))
	}
	protocolCorpusRequireValue(t, node, "Version Major", uint64(wire[4]))
	protocolCorpusRequireValue(t, node, "Version Minor", uint64(wire[5]))
	protocolCorpusRequireValue(t, node, "Message Type", string(wire[6:8]))
	header := 116
	if string(wire[6:8]) == "Pd" {
		header = 40
		protocolCorpusRequireValue(t, node, "Reserved", uint64(binary.BigEndian.Uint32(wire[24:])))
		protocolCorpusRequireValue(t, node, "Reply Communication ID", uint64(binary.BigEndian.Uint32(wire[28:])))
		protocolCorpusRequireValue(t, node, "Reply IP Address", wire[32:36])
	} else {
		reply := protocolCorpusFindNode(node, "Reply Status")
		require.NotNil(t, reply)
		value, err := reply.Result()
		require.NoError(t, err)
		require.EqualValues(t, int32(binary.BigEndian.Uint32(wire[24:])), value.Value)
		protocolCorpusRequireValue(t, node, "Session ID", wire[28:44])
		protocolCorpusRequireValue(t, node, "Reply Timeout", uint64(binary.BigEndian.Uint32(wire[44:])))
		protocolCorpusRequireValue(t, node, "Source URI", string(wire[48:80]))
		protocolCorpusRequireValue(t, node, "Destination URI", string(wire[80:112]))
	}
	protocolCorpusRequireValue(t, node, "Header CRC", uint64(crc32.ChecksumIEEE(wire[:header-4])))
	require.Equal(t, crc32.ChecksumIEEE(wire[:header-4]), binary.LittleEndian.Uint32(wire[header-4:header]))
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["Header CRC Valid"])
	if header == 40 && binary.BigEndian.Uint32(wire[8:]) == 0 {
		require.Equal(t, true, info["Noncanonical Zero Communication ID"])
	} else {
		require.NotContains(t, info, "Noncanonical Zero Communication ID")
	}
	length := int(binary.BigEndian.Uint32(wire[20:]))
	if length > 0 {
		protocolCorpusRequireValue(t, node, "Dataset", wire[header:header+length])
	}
	if header+length < len(wire) {
		protocolCorpusRequireValue(t, node, "Padding", wire[header+length:])
	}
}

func TestProtocolCorpusTRDPEveryMessage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-trdp.pcapng")
	require.Len(t, frames, 14)
	var messages [][]byte
	nextSequence := map[string]uint32{}
	tcpMessages, udpMessages := 0, 0
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		var config map[string]any
		if index == 12 || index == 13 {
			// The original PD records have a noncanonical zero communication ID.
			// Keep their bytes, require strict rejection, and mark compatibility.
			config = map[string]any{"trdpAllowZeroCommunicationID": true}
		}
		envelope := protocolCorpusRequireBoundedRuleParseWithConfig(t, frame, "ethernet", "Ethernet", config)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		var wire []byte
		if tcpLayer != nil {
			tcp := tcpLayer.(*layers.TCP)
			flow := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
			if tcp.SYN {
				nextSequence[flow] = tcp.Seq + 1
			}
			if len(tcp.Payload) == 0 {
				continue
			}
			wire = tcp.Payload
			expected, ok := nextSequence[flow]
			require.True(t, ok, "each data direction must have an observed opening sequence")
			require.Equal(t, expected, tcp.Seq)
			nextSequence[flow] += uint32(len(wire))
			tcpMessages++
		} else {
			udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			wire = udp.Payload
			udpMessages++
		}
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			length := int(binary.BigEndian.Uint32(wire[20:]))
			header := 116
			if string(wire[6:8]) == "Pd" {
				header = 40
				require.EqualValues(t, 0, binary.BigEndian.Uint32(wire[8:]))
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.trdp", "TRDPMessage")
				require.ErrorContains(t, err, "zero PD communication ID")
			}
			require.Equal(t, (header+length+3)&^3, len(wire), "one complete message per captured data segment/datagram")
			node := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.trdp", "TRDPMessage", config)
			protocolCorpusTRDPFields(t, node, wire)
			dispatched := protocolCorpusFindNode(envelope, "TRDP")
			require.NotNil(t, dispatched)
			if tcpLayer != nil {
				require.Len(t, dispatched.Children, 1)
				dispatched = dispatched.Children[0]
			}
			protocolCorpusTRDPFields(t, dispatched, wire)
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.trdp", config, "TRDPMessage")
				require.Errorf(t, err, "cut %d", cut)
			}
		})
		messages = append(messages, bytes.Clone(wire))
	}
	require.Len(t, messages, 6)
	require.Equal(t, 2, tcpMessages)
	require.Equal(t, 4, udpMessages)
	require.Len(t, nextSequence, 2)
	require.Equal(t, "Mr", string(messages[0][6:8]))
	require.Equal(t, "Mp", string(messages[1][6:8]))
	require.Equal(t, messages[0][28:44], messages[1][28:44], "reply retains request session identity")
	require.Equal(t, messages[2][28:44], messages[3][28:44])
	joined := protocolCorpusRequireBoundedRuleParseWithConfig(t, bytes.Join(messages, nil), "application-layer.trdp", "TRDPMessages", map[string]any{"trdpAllowZeroCommunicationID": true})
	require.Len(t, joined.Children, len(messages))
	for i, node := range joined.Children {
		protocolCorpusTRDPFields(t, node, messages[i])
	}
	for offset := 0; offset < 116; offset++ {
		bad := bytes.Clone(messages[0])
		bad[offset] ^= 1
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.trdp", "TRDPMessage")
		require.Errorf(t, err, "modified header byte %d must not pass CRC/value checks", offset)
	}
	badPadding := bytes.Clone(messages[0])
	badPadding[len(badPadding)-1] = 1
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badPadding), "application-layer.trdp", "TRDPMessage")
	require.ErrorContains(t, err, "nonzero message padding")
	// A changed dataset does not affect a header-only CRC.
	changed := bytes.Clone(messages[0])
	changed[116] ^= 1
	protocolCorpusRequireBoundedRuleParse(t, changed, "application-layer.trdp", "TRDPMessage")
	for size := 0; size <= 4; size++ {
		variant := append(bytes.Clone(messages[0][:116]), bytes.Repeat([]byte{0x7b}, size)...)
		variant = append(variant, make([]byte, (4-size%4)%4)...)
		binary.BigEndian.PutUint32(variant[0:], 0xabcdef12)
		binary.BigEndian.PutUint32(variant[12:], 0x12345678)
		binary.BigEndian.PutUint32(variant[16:], 0x23456789)
		binary.BigEndian.PutUint32(variant[20:], uint32(size))
		binary.BigEndian.PutUint32(variant[24:], 0xfffffffd)
		binary.LittleEndian.PutUint32(variant[112:], crc32.ChecksumIEEE(variant[:112]))
		node := protocolCorpusRequireBoundedRuleParse(t, variant, "application-layer.trdp", "TRDPMessage")
		protocolCorpusTRDPFields(t, node, variant)
	}
	for _, original := range messages[4:] {
		variant := bytes.Clone(original)
		binary.BigEndian.PutUint32(variant[8:], 1001)
		binary.LittleEndian.PutUint32(variant[36:], crc32.ChecksumIEEE(variant[:36]))
		node := protocolCorpusRequireBoundedRuleParse(t, variant, "application-layer.trdp", "TRDPMessage")
		protocolCorpusTRDPFields(t, node, variant)
	}
}
