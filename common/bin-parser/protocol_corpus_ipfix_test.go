package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func protocolCorpusIPFIXMessages(t *testing.T) [][]byte {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/scapy/scapy-ipfix.pcap")
	require.Len(t, frames, 3)
	var messages [][]byte
	for _, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, layers.UDPPort(9995), udp.DstPort)
		messages = append(messages, bytes.Clone(udp.Payload))
	}
	return messages
}

func TestProtocolCorpusIPFIXEveryTemplateAndRecord(t *testing.T) {
	messages := protocolCorpusIPFIXMessages(t)
	input := bytes.Join(messages, nil)
	node := protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.ipfix", "IPFIXMessages")
	parsed := protocolCorpusNodesNamed(node, "Message")
	require.Len(t, parsed, 3)
	for i, message := range parsed {
		data := messages[i]
		for name, offset := range map[string]int{"Version": 0, "Length": 2} {
			protocolCorpusRequireValue(t, message, name, uint64(binary.BigEndian.Uint16(data[offset:])))
		}
		for name, offset := range map[string]int{"Export Time": 4, "Sequence Number": 8, "Observation Domain ID": 12} {
			protocolCorpusRequireValue(t, message, name, uint64(binary.BigEndian.Uint32(data[offset:])))
		}
		protocolCorpusRequireValue(t, message, "Set ID", uint64(binary.BigEndian.Uint16(data[16:])))
		protocolCorpusRequireValue(t, message, "Set Length", uint64(binary.BigEndian.Uint16(data[18:])))
		if i < 2 {
			id, count := binary.BigEndian.Uint16(data[20:]), binary.BigEndian.Uint16(data[22:])
			protocolCorpusRequireValue(t, message, "Template ID", uint64(id))
			protocolCorpusRequireValue(t, message, "Field Count", uint64(count))
			offset := 24
			if i == 1 {
				protocolCorpusRequireValue(t, message, "Scope Field Count", uint64(binary.BigEndian.Uint16(data[offset:])))
				offset += 2
			}
			fields := protocolCorpusNodesNamed(message, "Field Specifier")
			require.Len(t, fields, int(count))
			for _, field := range fields {
				protocolCorpusRequireValue(t, field, "Enterprise Bit", uint64(data[offset]>>7))
				protocolCorpusRequireValue(t, field, "Information Element ID", uint64(binary.BigEndian.Uint16(data[offset:])&0x7fff))
				protocolCorpusRequireValue(t, field, "Field Length", uint64(binary.BigEndian.Uint16(data[offset+2:])))
				offset += 4
			}
			if offset < len(data) {
				protocolCorpusRequireValue(t, message, "Padding", data[offset:])
			}
		}
	}
	// Decode every data field independently from the first packet's template.
	records := protocolCorpusNodesNamed(parsed[2], "Record")
	require.Len(t, records, 1)
	fields := protocolCorpusNodesNamed(records[0], "Field")
	require.Len(t, fields, 23)
	position := 20
	for i, field := range fields {
		templateOffset := 24 + 4*i
		id := binary.BigEndian.Uint16(messages[0][templateOffset:])
		size := int(binary.BigEndian.Uint16(messages[0][templateOffset+2:]))
		value := messages[2][position : position+size]
		info := field.Cfg.GetItem("additionInfo").(map[string]any)
		require.EqualValues(t, id, info["Information Element ID"])
		require.EqualValues(t, size, info["Value Length"])
		require.EqualValues(t, 0, info["Enterprise Bit"])
		require.EqualValues(t, 0, info["Enterprise Number"])
		require.Equal(t, false, info["Scope"])
		switch id {
		case 8, 12, 15, 18:
			protocolCorpusRequireValue(t, field, "Address", value)
		default:
			var integer uint64
			for _, octet := range value {
				integer = integer<<8 | uint64(octet)
			}
			protocolCorpusRequireValue(t, field, "Unsigned", integer)
		}
		position += size
	}
	require.Equal(t, len(messages[2]), position, "every captured data octet has a template field")
	// This capture's flow sequence jumps from 3791 to 3812; preserve the gap,
	// rather than inventing records that are absent from the capture.
	protocolCorpusRequireValue(t, parsed[2], "Sequence Number", uint64(3812))
}

func protocolCorpusIPFIXMessage(domain uint32, id uint16, body []byte) []byte {
	data := make([]byte, 20+len(body))
	binary.BigEndian.PutUint16(data, 10)
	binary.BigEndian.PutUint16(data[2:], uint16(len(data)))
	binary.BigEndian.PutUint32(data[12:], domain)
	binary.BigEndian.PutUint16(data[16:], id)
	binary.BigEndian.PutUint16(data[18:], uint16(len(data)-16))
	copy(data[20:], body)
	return data
}

func protocolCorpusIPFIXParseWithState(t *testing.T, data []byte, state map[int]any) (*base.Node, error) {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(data)
	node, err := parser.ParseBinaryWithConfig(reader, "application-layer.ipfix", map[string]any{"ipfixTemplates": state}, "IPFIX")
	if err == nil {
		require.Zero(t, reader.Len())
		covered, coverageErr := protocolCorpusTerminalCoverage(node, data)
		require.NoError(t, coverageErr)
		require.Positive(t, covered)
	}
	return node, err
}

func TestProtocolCorpusIPFIXSessionIsolationAndTransactionalTemplates(t *testing.T) {
	messages := protocolCorpusIPFIXMessages(t)
	state := map[int]any{}
	for _, message := range messages {
		_, err := protocolCorpusIPFIXParseWithState(t, message, state)
		require.NoError(t, err)
	}
	require.Len(t, state, 1)
	original := state[0].(map[int]any)
	require.Len(t, original, 2)
	for _, data := range [][]byte{messages[2], func() []byte { b := bytes.Clone(messages[2]); binary.BigEndian.PutUint32(b[12:], 42); return b }()} {
		selected := state
		if binary.BigEndian.Uint32(data[12:]) == 0 {
			selected = map[int]any{}
		}
		_, err := protocolCorpusIPFIXParseWithState(t, data, selected)
		require.ErrorContains(t, err, "missing template for data set")
	}
	// New template 600 is well-formed, but a following reserved set makes the
	// whole message invalid. It must not escape into the session's state.
	bad := protocolCorpusIPFIXMessage(0, 2, mustHex(t, "0258000100080004"))
	bad = append(bad, 0, 4, 0, 4)
	binary.BigEndian.PutUint16(bad[2:], uint16(len(bad)))
	_, err := protocolCorpusIPFIXParseWithState(t, bad, state)
	require.ErrorContains(t, err, "reserved set identifier")
	require.Equal(t, original, state[0])
	require.NotContains(t, state[0], 600)
	for _, message := range messages {
		for cut := 0; cut < len(message); cut++ {
			_, err := protocolCorpusIPFIXParseWithState(t, message[:cut], state)
			require.Error(t, err)
		}
	}
	require.Equal(t, original, state[0])
}

func TestProtocolCorpusIPFIXEnterpriseVariableLengthAndScope(t *testing.T) {
	// Option template: an IPv6 scope, a variable-length enterprise IE whose
	// number overlaps standard octetDeltaCount, and a reduced-size integer.
	template := protocolCorpusIPFIXMessage(7, 3, mustHex(t, "010100030001001b00108001ffff00007ed900070002"))
	address := mustHex(t, "20010db8000000000000000000000001")
	require.Len(t, address, 16)
	var body []byte
	for _, value := range [][]byte{nil, {0xaa, 0xbb}, bytes.Repeat([]byte{0x5a}, 260)} {
		body = append(body, address...)
		if len(value) < 255 {
			body = append(body, byte(len(value)))
		} else {
			body = append(body, 255, byte(len(value)>>8), byte(len(value)))
		}
		body = append(body, value...)
		body = append(body, 0x12, 0x34)
	}
	body = append(body, 0) // less than the minimum record length
	data := protocolCorpusIPFIXMessage(7, 257, body)
	require.Len(t, body, 322)
	node := protocolCorpusRequireBoundedRuleParse(t, append(bytes.Clone(template), data...), "application-layer.ipfix", "IPFIXMessages")
	records := protocolCorpusNodesNamed(node, "Record")
	require.Len(t, records, 3)
	for i, record := range records {
		fields := protocolCorpusNodesNamed(record, "Field")
		require.Len(t, fields, 3)
		protocolCorpusRequireValue(t, fields[0], "Address", address)
		require.Equal(t, true, fields[0].Cfg.GetItem("additionInfo").(map[string]any)["Scope"])
		info := fields[1].Cfg.GetItem("additionInfo").(map[string]any)
		require.EqualValues(t, 1, info["Enterprise Bit"])
		require.EqualValues(t, 32473, info["Enterprise Number"])
		if i == 0 {
			require.Nil(t, protocolCorpusFindNode(fields[1], "Octets"))
		} else if i == 1 {
			protocolCorpusRequireValue(t, fields[1], "Octets", []byte{0xaa, 0xbb})
		} else {
			protocolCorpusRequireValue(t, fields[1], "Extended Length", uint64(260))
			protocolCorpusRequireValue(t, fields[1], "Octets", bytes.Repeat([]byte{0x5a}, 260))
		}
		protocolCorpusRequireValue(t, fields[2], "Unsigned", uint64(0x1234))
	}
	protocolCorpusRequireValue(t, node, "Padding", []byte{0})
	bad := bytes.Clone(data)
	bad[len(bad)-1] = 1
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(template), bad...)), "application-layer.ipfix", "IPFIXMessages")
	require.ErrorContains(t, err, "padding must be zero")
}
