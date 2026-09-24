package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func opcuaAppendU32(dst []byte, value uint32) []byte {
	var wire [4]byte
	binary.LittleEndian.PutUint32(wire[:], value)
	return append(dst, wire[:]...)
}

func opcuaAppendString(dst []byte, value string) []byte {
	dst = opcuaAppendU32(dst, uint32(len(value)))
	return append(dst, value...)
}

func opcuaTCPMessage(kind string, body []byte) []byte {
	wire := append([]byte(kind), 'F')
	wire = opcuaAppendU32(wire, uint32(8+len(body)))
	return append(wire, body...)
}

func opcuaHelloMessage() []byte {
	body := make([]byte, 0, 32+len("opc.tcp://localhost:4840"))
	for _, value := range []uint32{0, 65536, 65536, 0, 0} {
		body = opcuaAppendU32(body, value)
	}
	body = opcuaAppendString(body, "opc.tcp://localhost:4840")
	return opcuaTCPMessage("HEL", body)
}

func opcuaAckMessage() []byte {
	body := make([]byte, 0, 20)
	for _, value := range []uint32{0, 65536, 65536, 0, 0} {
		body = opcuaAppendU32(body, value)
	}
	return opcuaTCPMessage("ACK", body)
}

func opcuaErrMessage() []byte {
	body := opcuaAppendU32(nil, 0x80010000)
	body = opcuaAppendString(body, "rejected")
	return opcuaTCPMessage("ERR", body)
}

func opcuaRHEMessage() []byte {
	body := opcuaAppendString(nil, "urn:example:server")
	body = opcuaAppendString(body, "opc.tcp://example:4840")
	return opcuaTCPMessage("RHE", body)
}

func opcuaOPNMessage() []byte {
	body := opcuaAppendU32(nil, 7) // SecureChannelId
	body = opcuaAppendString(body, "http://opcfoundation.org/UA/SecurityPolicy#None")
	body = opcuaAppendU32(body, ^uint32(0)) // null SenderCertificate
	body = opcuaAppendU32(body, ^uint32(0)) // null ReceiverCertificateThumbprint
	body = opcuaAppendU32(body, 3)          // SequenceNumber
	body = opcuaAppendU32(body, 9)          // RequestId
	body = append(body, 0xde, 0xad, 0xbe, 0xef)
	return opcuaTCPMessage("OPN", body)
}

func parseOPCUATCP(t *testing.T, wire []byte) *base.NodeValue {
	t.Helper()
	node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.extended_protocols", "OPCUATCPMessage")
	require.NoError(t, err)
	value, err := node.Result()
	require.NoError(t, err)
	return value
}

func requireOPCUAFrame(t *testing.T, srcPort, dstPort layers.TCPPort, wire []byte) *base.NodeValue {
	t.Helper()
	eth := parseEthernet(t, ipv4TCPFrame(t, srcPort, dstPort, wire))
	return mustChild(t, eth, "IP", "TCP", "OPCUATCPMessage")
}

func TestOPCUATCPControlMessagesAndOpaqueOPN(t *testing.T) {
	tests := []struct {
		name      string
		wire      []byte
		src, dst  layers.TCPPort
		wantType  string
		wantField string
		wantValue string
	}{
		{name: "HEL", wire: opcuaHelloMessage(), src: 50000, dst: 4840, wantType: "HEL", wantField: "Endpoint URL", wantValue: "opc.tcp://localhost:4840"},
		{name: "ACK", wire: opcuaAckMessage(), src: 4840, dst: 50000, wantType: "ACK", wantField: "Maximum Chunk Count", wantValue: "0"},
		{name: "ERR", wire: opcuaErrMessage(), src: 4840, dst: 50000, wantType: "ERR", wantField: "Reason", wantValue: "rejected"},
		{name: "RHE", wire: opcuaRHEMessage(), src: 4840, dst: 50000, wantType: "RHE", wantField: "Server URI", wantValue: "urn:example:server"},
		{name: "OPN", wire: opcuaOPNMessage(), src: 50000, dst: 4840, wantType: "OPN", wantField: "Opaque OPN Body", wantValue: "\xde\xad\xbe\xef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseOPCUATCP(t, tt.wire)
			require.Equal(t, tt.wantType, strVal(t, parsed.Child("Message Type")))
			require.Equal(t, uint64(len(tt.wire)), uintVal(t, parsed.Child("Message Size")))
			if tt.wantField == "Opaque OPN Body" {
				require.Equal(t, []byte(tt.wantValue), bytesVal(t, parsed.Child(tt.wantField)))
				require.Equal(t, uint64(7), uintVal(t, parsed.Child("Secure Channel ID")))
				require.Equal(t, uint64(3), uintVal(t, parsed.Child("Sequence Number")))
				require.Equal(t, uint64(9), uintVal(t, parsed.Child("Request ID")))
			} else if tt.wantField == "Maximum Chunk Count" {
				require.Equal(t, uint64(0), uintVal(t, parsed.Child(tt.wantField)))
			} else if tt.wantType == "RHE" {
				require.Equal(t, "urn:example:server", strVal(t, parsed.Child("Server URI")))
				require.Equal(t, "opc.tcp://example:4840", strVal(t, parsed.Child("Endpoint URL")))
			} else {
				require.Equal(t, tt.wantValue, strVal(t, parsed.Child(tt.wantField)))
			}
			frame := requireOPCUAFrame(t, tt.src, tt.dst, tt.wire)
			require.Equal(t, tt.wantType, strVal(t, frame.Child("Message Type")))
		})
	}
}

func TestOPCUATCPDispatchPlanMatchesLegacy(t *testing.T) {
	validHello := opcuaHelloMessage()
	badChunk := append([]byte(nil), validHello...)
	badChunk[3] = 'C'
	badSize := append([]byte(nil), validHello...)
	binary.LittleEndian.PutUint32(badSize[4:8], uint32(len(badSize)+1))
	malformedHello := append([]byte(nil), validHello...)
	binary.LittleEndian.PutUint32(malformedHello[28:32], 0)
	works := []currentCorpusWork{
		{id: "opcua/hello-standard-port", wire: ipv4TCPFrame(t, 50000, 4840, validHello)},
		{id: "opcua/hello-custom-port", wire: ipv4TCPFrame(t, 50000, 50001, validHello)},
		{id: "opcua/hello-server-direction-rejected", wire: ipv4TCPFrame(t, 4840, 50000, validHello)},
		{id: "opcua/ack-server-direction", wire: ipv4TCPFrame(t, 4840, 50000, opcuaAckMessage())},
		{id: "opcua/err-server-direction", wire: ipv4TCPFrame(t, 4840, 50000, opcuaErrMessage())},
		{id: "opcua/rhe-server-direction", wire: ipv4TCPFrame(t, 4840, 50000, opcuaRHEMessage())},
		{id: "opcua/opn-custom-port", wire: ipv4TCPFrame(t, 50000, 50001, opcuaOPNMessage())},
		{id: "opcua/non-opcua-on-standard-port", wire: ipv4TCPFrame(t, 50000, 4840, []byte("GET / HTTP/1.1\r\nHost: example\r\n\r\n"))},
		{id: "opcua/non-final-chunk", wire: ipv4TCPFrame(t, 50000, 4840, badChunk)},
		{id: "opcua/declared-size-too-large", wire: ipv4TCPFrame(t, 50000, 4840, badSize)},
		{id: "opcua/invalid-hello-body", wire: ipv4TCPFrame(t, 50000, 4840, malformedHello)},
		{id: "opcua/invalid-hello-body-custom-port", wire: ipv4TCPFrame(t, 50000, 50001, malformedHello)},
	}
	for end := 0; end < 8; end++ {
		works = append(works, currentCorpusWork{
			id:   fmt.Sprintf("opcua/truncated-header/%d", end),
			wire: ipv4TCPFrame(t, 50000, 4840, validHello[:end]),
		})
	}
	assertDispatchEquivalence(t, works)
}

func TestOPCUATCPMessageFramingAndNearMisses(t *testing.T) {
	valid := opcuaHelloMessage()
	validPlusTail := append(append([]byte(nil), valid...), 0xaa, 0xbb)
	framed := requireOPCUAFrame(t, 50000, 4840, validPlusTail)
	require.Equal(t, "HEL", strVal(t, framed.Child("Message Type")))
	require.Equal(t, []byte{0xaa, 0xbb}, bytesVal(t, mustChild(t, parseEthernet(t, ipv4TCPFrame(t, 50000, 4840, validPlusTail)), "IP", "TCP", "Remaining Payload")))

	badChunk := append([]byte(nil), valid...)
	badChunk[3] = 'C'
	badSize := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(badSize[4:8], uint32(len(badSize)-1))
	tooLarge := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(tooLarge[4:8], uint32(len(tooLarge)+1))
	truncatedBody := append([]byte(nil), valid[:len(valid)-1]...)
	binary.LittleEndian.PutUint32(truncatedBody[4:8], uint32(len(truncatedBody)))
	unknownType := append([]byte(nil), valid...)
	copy(unknownType[:3], "MSG")
	tooShortOPN := opcuaTCPMessage("OPN", make([]byte, 24))
	malformed := map[string][]byte{
		"non-final control chunk":   badChunk,
		"declared length too small": badSize,
		"declared length too large": tooLarge,
		"truncated HEL URL":         truncatedBody,
		"unsupported MSG body":      unknownType,
		"OPN without opaque body":   tooShortOPN,
	}
	for name, wire := range malformed {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.extended_protocols", "OPCUATCPMessage")
			require.Error(t, err)
		})
	}
	customPortHello := requireOPCUAFrame(t, 50000, 50001, valid)
	require.Equal(t, "HEL", strVal(t, customPortHello.Child("Message Type")), "a complete UACP signature is sufficient on a custom TCP port")

	for _, tt := range []struct {
		name string
		src  layers.TCPPort
		dst  layers.TCPPort
	}{
		{name: "HEL in server direction", src: 4840, dst: 50000},
		{name: "HEL with both ports 4840", src: 4840, dst: 4840},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eth := parseEthernet(t, ipv4TCPFrame(t, tt.src, tt.dst, valid))
			payload := mustChild(t, eth, "IP", "TCP")
			require.Nil(t, payload.Child("OPCUATCPMessage"))
		})
	}
	for _, tt := range []struct {
		name string
		wire []byte
		src  layers.TCPPort
		dst  layers.TCPPort
	}{
		{name: "ACK in client direction", wire: opcuaAckMessage(), src: 50000, dst: 4840},
		{name: "ERR in client direction", wire: opcuaErrMessage(), src: 50000, dst: 4840},
		{name: "RHE in client direction", wire: opcuaRHEMessage(), src: 50000, dst: 4840},
		{name: "non-final HEL", wire: badChunk, src: 50000, dst: 4840},
		{name: "truncated HEL", wire: truncatedBody, src: 50000, dst: 4840},
		{name: "unknown UACP type", wire: unknownType, src: 50000, dst: 4840},
	} {
		t.Run(tt.name, func(t *testing.T) {
			eth := parseEthernet(t, ipv4TCPFrame(t, tt.src, tt.dst, tt.wire))
			payload := mustChild(t, eth, "IP", "TCP")
			require.Nil(t, payload.Child("OPCUATCPMessage"))
		})
	}
	t.Run("non-OPC-UA payload on port 4840", func(t *testing.T) {
		eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 4840, []byte("GET / HTTP/1.1\r\nHost: example\r\n\r\n")))
		tcp := mustChild(t, eth, "IP", "TCP")
		require.Nil(t, tcp.Child("OPCUATCPMessage"))
	})
}

func TestOPCUATCPRejectsMalformedHelloFields(t *testing.T) {
	tests := map[string]func([]byte) []byte{
		"zero endpoint length": func(wire []byte) []byte {
			wire = append([]byte(nil), wire...)
			binary.LittleEndian.PutUint32(wire[28:32], 0)
			return wire[:32]
		},
		"oversized endpoint length": func(wire []byte) []byte {
			wire = append([]byte(nil), wire...)
			binary.LittleEndian.PutUint32(wire[28:32], 4097)
			return wire
		},
		"endpoint extends beyond declared size": func(wire []byte) []byte {
			wire = append([]byte(nil), wire...)
			binary.LittleEndian.PutUint32(wire[28:32], 4096)
			return wire
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			wire := mutate(opcuaHelloMessage())
			_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.extended_protocols", "OPCUATCPMessage")
			require.Error(t, err)
		})
	}
}

func TestOPCUATCPRejectsImplausibleHandshakeValues(t *testing.T) {
	for _, name := range []string{"HEL", "ACK"} {
		wire := opcuaHelloMessage()
		if name == "ACK" {
			wire = opcuaAckMessage()
		}
		for _, fieldOffset := range []int{12, 16} {
			bad := append([]byte(nil), wire...)
			binary.LittleEndian.PutUint32(bad[fieldOffset:fieldOffset+4], 1)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "application-layer.extended_protocols", "OPCUATCPMessage")
			require.Error(t, err, "%s with a 1-byte negotiation buffer must not be admitted", name)
			eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 50001, bad))
			tcp := mustChild(t, eth, "IP", "TCP")
			require.Nil(t, tcp.Child("OPCUATCPMessage"), "%s near-miss on custom port", name)
		}
	}

	badOPN := append([]byte(nil), opcuaOPNMessage()...)
	policyLength := binary.LittleEndian.Uint32(badOPN[12:16])
	senderLengthOffset := 16 + int(policyLength)
	binary.LittleEndian.PutUint32(badOPN[senderLengthOffset:senderLengthOffset+4], 4096)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badOPN), "application-layer.extended_protocols", "OPCUATCPMessage")
	require.Error(t, err, "a certificate length that crosses the OPN message boundary must fail")
}

func TestOPCUATCPRealCaptureAutoAdmission(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	var capture *protocolCorpusCapture
	for i := range manifest.Captures {
		if manifest.Captures[i].ID == "ndpi-opcua" {
			capture = &manifest.Captures[i]
			break
		}
	}
	require.NotNil(t, capture)
	captureData := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
	assertProtocolCorpusHash(t, capture.CaptureFile, captureData, capture.SHA256)
	_, linkType, frame := inspectProtocolCorpusCapture(t, captureData, capture.RepresentativeFrame)
	require.Equal(t, "Null", linkType)
	require.Greater(t, len(frame), 4)

	// The upstream packet is loopback/null encapsulated IPv4. Parse the captured
	// IP/TCP bytes directly so port-direction detection is exercised against an
	// unmodified HEL packet, independently decoded by Wireshark in the corpus.
	ipNode, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[4:]), "internet_protocol", "Internet Protocol")
	require.NoError(t, err)
	ipValue, err := ipNode.Result()
	require.NoError(t, err)
	opcua := mustChild(t, ipValue, "TCP", "OPCUATCPMessage")
	require.Equal(t, "HEL", strVal(t, opcua.Child("Message Type")))
	require.Equal(t, "opc.tcp://localhost:4840", strVal(t, opcua.Child("Endpoint URL")))
}

func TestOPCUATCPSignedWiresharkCapture(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	var capture *protocolCorpusCapture
	for i := range manifest.Captures {
		if manifest.Captures[i].ID == "ws-opcua-signed" {
			capture = &manifest.Captures[i]
			break
		}
	}
	require.NotNil(t, capture)
	require.Equal(t, "upstream-positive", capture.EvidenceKind)
	require.Equal(t, "OPC UA signed", capture.Protocol)
	require.Equal(t, "test/captures/opcua-signed.pcapng", capture.UpstreamPath)
	require.Equal(t, "a2aaa4a74040dd079de4358131864725f01634bf53477f1a23485f6a0acfc7d2", capture.SHA256)
	captureData := readProtocolCorpusFile(t, corpusDir, capture.CaptureFile)
	assertProtocolCorpusHash(t, capture.CaptureFile, captureData, capture.SHA256)
	packetCount, linkType, helloFrame := inspectProtocolCorpusCapture(t, captureData, capture.RepresentativeFrame)
	require.Equal(t, 98, packetCount)
	require.Equal(t, "Ethernet", linkType)
	assertProtocolCorpusHash(t, "ws-opcua-signed representative frame", helloFrame, capture.RepresentativeFrame.SHA256)
	hello := mustChild(t, parseEthernet(t, helloFrame), "IP", "TCP", "OPCUATCPMessage")
	require.Equal(t, "HEL", strVal(t, hello.Child("Message Type")))
	require.Equal(t, uint64(68), uintVal(t, hello.Child("Message Size")))
	_, _, ackFrame := inspectProtocolCorpusCapture(t, captureData, &protocolCorpusFrame{Number: 6})
	ack := mustChild(t, parseEthernet(t, ackFrame), "IP", "TCP", "OPCUATCPMessage")
	require.Equal(t, "ACK", strVal(t, ack.Child("Message Type")))

	for _, frameNumber := range []int{8, 9} {
		_, _, frame := inspectProtocolCorpusCapture(t, captureData, &protocolCorpusFrame{Number: frameNumber})
		opn := mustChild(t, parseEthernet(t, frame), "IP", "TCP", "OPCUATCPMessage")
		require.Equal(t, "OPN", strVal(t, opn.Child("Message Type")), "captured frame %d", frameNumber)
		require.NotEmpty(t, strVal(t, opn.Child("Security Policy URI")))
		require.NotEmpty(t, bytesVal(t, opn.Child("Opaque OPN Body")))
	}

	// This corpus also contains real signed MSG traffic. The bounded slice
	// intentionally leaves that message/session grammar out of its claim.
	_, _, msgFrame := inspectProtocolCorpusCapture(t, captureData, &protocolCorpusFrame{Number: 10})
	msgTCP := mustChild(t, parseEthernet(t, msgFrame), "IP", "TCP")
	require.Nil(t, msgTCP.Child("OPCUATCPMessage"))
}
