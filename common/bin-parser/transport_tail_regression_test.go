package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type transportTailExactReader struct {
	*bytes.Reader
}

func newTransportTailExactReader(input []byte) *transportTailExactReader {
	return &transportTailExactReader{Reader: bytes.NewReader(input)}
}

func (r *transportTailExactReader) InputBitLength() uint64 {
	return uint64(r.Len()) * 8
}

func tcpSegment(payload []byte) []byte {
	segment := make([]byte, 20, 20+len(payload))
	binary.BigEndian.PutUint16(segment[0:2], 40000)
	binary.BigEndian.PutUint16(segment[2:4], 40001)
	segment[12] = 5 << 4
	return append(segment, payload...)
}

func udpDatagram(payload []byte) []byte {
	datagram := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(datagram[0:2], 40000)
	binary.BigEndian.PutUint16(datagram[2:4], 40001)
	binary.BigEndian.PutUint16(datagram[4:6], uint16(8+len(payload)))
	return append(datagram, payload...)
}

func TestTransportUnboundedEmptyPayloadDoesNotCreateHugeTail(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		rule  string
		entry string
	}{
		{name: "tcp", input: tcpSegment(nil), rule: "transmission_control_protocol", entry: "TCP"},
		{name: "udp", input: udpDatagram(nil), rule: "user_datagram_protocol", entry: "UDP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.ParseBinary(bytes.NewReader(tt.input), tt.rule, tt.entry)
			require.NoError(t, err)
			require.NotNil(t, node)
		})
	}
}

func TestTransportBoundedUnknownPayloadIsPreserved(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	tests := []struct {
		name     string
		input    []byte
		rule     string
		entry    string
		tailPath string
	}{
		{name: "tcp", input: tcpSegment(payload), rule: "transmission_control_protocol", entry: "TCP", tailPath: "@TCP.Payload.Remaining Payload"},
		{name: "udp", input: udpDatagram(payload), rule: "user_datagram_protocol", entry: "UDP", tailPath: "@UDP.Payload.Remaining Payload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTransportTailExactReader(tt.input)
			node, err := parser.ParseBinary(reader, tt.rule, tt.entry)
			require.NoError(t, err)
			require.Zero(t, reader.Len())

			tail := base.GetNodeByPath(node, tt.tailPath)
			require.NotNil(t, tail)
			value, err := tail.Result()
			require.NoError(t, err)
			require.Equal(t, payload, value.Value)
		})
	}
}

func TestTransportBoundedRecognizedPayloadTailIsPreserved(t *testing.T) {
	tailBytes := []byte{0xde, 0xad, 0xbe, 0xef}

	tcpPayload := append([]byte("SSH-2.0-test\r\n"), tailBytes...)
	tcpInput := tcpSegment(tcpPayload)
	binary.BigEndian.PutUint16(tcpInput[2:4], 22)

	dnsHeader := make([]byte, 12)
	udpInput := udpDatagram(append(dnsHeader, tailBytes...))
	binary.BigEndian.PutUint16(udpInput[2:4], 53)

	tests := []struct {
		name       string
		input      []byte
		rule       string
		entry      string
		parsedPath string
		tailPath   string
	}{
		{name: "tcp", input: tcpInput, rule: "transmission_control_protocol", entry: "TCP", parsedPath: "@TCP.Payload.SSH", tailPath: "@TCP.Payload.Remaining Payload"},
		{name: "udp", input: udpInput, rule: "user_datagram_protocol", entry: "UDP", parsedPath: "@UDP.Payload.DNS", tailPath: "@UDP.Payload.Remaining Payload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTransportTailExactReader(tt.input)
			node, err := parser.ParseBinary(reader, tt.rule, tt.entry)
			require.NoError(t, err)
			require.Zero(t, reader.Len())
			require.NotNil(t, base.GetNodeByPath(node, tt.parsedPath))

			tail := base.GetNodeByPath(node, tt.tailPath)
			require.NotNil(t, tail)
			value, err := tail.Result()
			require.NoError(t, err)
			require.Equal(t, tailBytes, value.Value)
		})
	}
}

func TestEthernetFrameTrailerRequiresExplicitCaptureBoundary(t *testing.T) {
	// A short IPv4/TCP packet can carry padding or a capture-retained FCS.
	// Neither is part of the TCP payload, and opaque nonzero bytes must survive.
	header := make([]byte, 34)
	binary.BigEndian.PutUint16(header[12:], 0x0800)
	header[14] = 0x45
	header[23] = 6
	binary.BigEndian.PutUint16(header[16:], 40)
	frame := append(header, tcpSegment(nil)...)
	trailer := []byte{0, 0, 0xde, 0xad, 0xbe, 0xef}
	withTrailer := append(bytes.Clone(frame), trailer...)
	node := protocolCorpusRequireBoundedRuleParse(t, withTrailer, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, node, "Frame Trailer", trailer)
	require.Nil(t, protocolCorpusFindNode(node, "Remaining Payload"))

	stream := bytes.NewReader(append(bytes.Clone(frame), []byte("next frame")...))
	parsed, err := parser.ParseBinary(stream, "ethernet", "Ethernet")
	require.NoError(t, err)
	require.Equal(t, len("next frame"), stream.Len())
	require.Nil(t, protocolCorpusFindNode(parsed, "Frame Trailer"))
}
