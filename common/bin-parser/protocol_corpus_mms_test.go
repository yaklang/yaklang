package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const mmsCorpusCapturePath = "testdata/protocol-corpus/captures/ndpi/ndpi-iec61850-mms.pcap"

type mmsCorpusPDUCase struct {
	frame         int
	payloadOffset int
	hex           string
}

var mmsCorpusPDUCases = []mmsCorpusPDUCase{
	{frame: 7, payloadOffset: 127, hex: "a826800300fa0081010a82010a830105a416800101810305e100820c03a00000000000000000e110"},
	{frame: 8, payloadOffset: 107, hex: "a92580027d0081010a820108830105a416800101810305e100820c03ee0800000400000001ed18"},
	{frame: 10, payloadOffset: 20, hex: "a02c020304a273a625a023a1211a0e41413145315130314650324c44301a0f4c4c4e30244252247263625f423032"},
	{frame: 12, payloadOffset: 20, hex: "a20c800304a273a205a003870102"},
	{frame: 13, payloadOffset: 20, hex: "a032020304a274a42ba129a0273025a023a1211a0e41413145315130314650324c44301a0f4c4c4e30244252247263625f423032"},
	{frame: 14, payloadOffset: 20, hex: "a10c020304a274a405a10380010a"},
	{frame: 15, payloadOffset: 20, hex: "8b00"},
	{frame: 16, payloadOffset: 20, hex: "8c00"},
}

func TestProtocolCorpusMMSFullCapture(t *testing.T) {
	captureData := readProtocolCorpusFile(t, ".", mmsCorpusCapturePath)
	captureDigest := sha256.Sum256(captureData)
	require.Equal(t, "f4950e357733e51877a8804bcadb0ec87b7397f4a74a4e4b92fcfa3310dc03e0", hex.EncodeToString(captureDigest[:]))
	reader, err := pcapgo.NewReader(bytes.NewReader(captureData))
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	expectedPayloadLengths := []int{
		0, 0, 0, 22, 0, 22, 167, 146, 0, 66, 0,
		34, 72, 34, 22, 22, 25, 25, 0, 0, 0, 0,
	}
	pduByFrame := make(map[int]mmsCorpusPDUCase, len(mmsCorpusPDUCases))
	for _, pduCase := range mmsCorpusPDUCases {
		pduByFrame[pduCase.frame] = pduCase
	}

	packetCount := 0
	directions := map[string]int{}
	parsed := make(map[int]*base.NodeValue, len(mmsCorpusPDUCases))
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		packetCount++

		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errorLayer := packet.ErrorLayer(); errorLayer != nil {
			t.Fatalf("frame %d decode failed: %v", packetCount, errorLayer.Error())
		}
		ipv4, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		require.Truef(t, ok, "frame %d must contain IPv4", packetCount)
		tcp, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.Truef(t, ok, "frame %d must contain TCP", packetCount)
		require.Equalf(t, expectedPayloadLengths[packetCount-1], len(tcp.Payload), "frame %d TCP payload length", packetCount)

		switch {
		case ipv4.SrcIP.String() == "172.16.0.101" && uint16(tcp.SrcPort) == 1345 && ipv4.DstIP.String() == "172.16.202.5" && uint16(tcp.DstPort) == 102:
			directions["client-to-server"]++
		case ipv4.SrcIP.String() == "172.16.202.5" && uint16(tcp.SrcPort) == 102 && ipv4.DstIP.String() == "172.16.0.101" && uint16(tcp.DstPort) == 1345:
			directions["server-to-client"]++
		default:
			t.Fatalf("frame %d is outside the pinned MMS flow: %s:%d -> %s:%d", packetCount, ipv4.SrcIP, tcp.SrcPort, ipv4.DstIP, tcp.DstPort)
		}

		pduCase, isMMS := pduByFrame[packetCount]
		if !isMMS {
			continue
		}
		expectedPDU := mmsCorpusDecodeHex(t, pduCase.hex)
		require.LessOrEqualf(t, pduCase.payloadOffset, len(tcp.Payload), "frame %d MMS offset", packetCount)
		require.Equalf(t, expectedPDU, tcp.Payload[pduCase.payloadOffset:], "frame %d MMS PDU bytes changed", packetCount)
		_, parsed[packetCount] = mmsCorpusParseExact(t, packetCount, expectedPDU)
	}

	require.Equal(t, 22, packetCount)
	require.Equal(t, map[string]int{"client-to-server": 11, "server-to-client": 11}, directions)
	require.Len(t, parsed, 8, "the fixed capture must contain exactly eight audited MMS PDUs")

	request := mustChild(t, parsed[7], "Initiate Request")
	require.Equal(t, uint64(0xa8), uintVal(t, parsed[7].Child("PDU Tag")))
	require.Equal(t, uint64(38), uintVal(t, parsed[7].Child("PDU Length")))
	require.Equal(t, []byte{0x00, 0xfa, 0x00}, bytesVal(t, request.Child("Local Detail Calling")))
	require.Equal(t, uint64(10), uintVal(t, request.Child("Proposed Calling Limit")))
	require.Equal(t, uint64(10), uintVal(t, request.Child("Proposed Called Limit")))
	require.Equal(t, uint64(5), uintVal(t, request.Child("Proposed Nesting Level")))
	requestDetail := mustChild(t, request, "Initiate Request Detail")
	require.Equal(t, uint64(1), uintVal(t, requestDetail.Child("Proposed Version")))
	require.Equal(t, uint64(5), uintVal(t, requestDetail.Child("Parameter CBB Unused Bits")))
	require.Equal(t, []byte{0xe1, 0x00}, bytesVal(t, requestDetail.Child("Parameter CBB Bits")))
	require.Equal(t, uint64(3), uintVal(t, requestDetail.Child("Services Unused Bits")))
	require.Equal(t, []byte{0xa0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe1, 0x10}, bytesVal(t, requestDetail.Child("Services Supported Bits")))

	response := mustChild(t, parsed[8], "Initiate Response")
	require.Equal(t, uint64(0xa9), uintVal(t, parsed[8].Child("PDU Tag")))
	require.Equal(t, []byte{0x7d, 0x00}, bytesVal(t, response.Child("Local Detail Called")))
	require.Equal(t, uint64(10), uintVal(t, response.Child("Negotiated Calling Limit")))
	require.Equal(t, uint64(8), uintVal(t, response.Child("Negotiated Called Limit")))
	require.Equal(t, uint64(5), uintVal(t, response.Child("Negotiated Nesting Level")))
	responseDetail := mustChild(t, response, "Initiate Response Detail")
	require.Equal(t, uint64(1), uintVal(t, responseDetail.Child("Negotiated Version")))
	require.Equal(t, []byte{0xe1, 0x00}, bytesVal(t, responseDetail.Child("Parameter CBB Bits")))
	require.Equal(t, []byte{0xee, 0x08, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x01, 0xed, 0x18}, bytesVal(t, responseDetail.Child("Services Supported Bits")))

	getAttributes := mustChild(t, parsed[10], "Confirmed Request")
	require.Equal(t, uint64(0x04a273), uintVal(t, getAttributes.Child("Invoke ID")))
	require.Equal(t, uint64(0xa6), uintVal(t, getAttributes.Child("Service Tag")))
	getAttributesName := mustChild(t, getAttributes, "Get Variable Access Attributes", "Object Name", "Domain Specific")
	require.Equal(t, "AA1E1Q01FP2LD0", strVal(t, getAttributesName.Child("Domain ID")))
	require.Equal(t, "LLN0$BR$rcb_B02", strVal(t, getAttributesName.Child("Item ID")))

	confirmedError := mustChild(t, parsed[12], "Confirmed Error")
	require.Equal(t, uint64(0x04a273), uintVal(t, confirmedError.Child("Invoke ID")))
	require.Equal(t, uint64(2), uintVal(t, mustChild(t, confirmedError, "Service Error", "Error Class", "Access Error")))

	readRequest := mustChild(t, parsed[13], "Confirmed Request")
	require.Equal(t, uint64(0x04a274), uintVal(t, readRequest.Child("Invoke ID")))
	require.Equal(t, uint64(0xa4), uintVal(t, readRequest.Child("Service Tag")))
	readName := mustChild(t, readRequest, "Read Request", "Variable Access Specification", "List Of Variable", "Variable Specification", "Domain Specific")
	require.Equal(t, "AA1E1Q01FP2LD0", strVal(t, readName.Child("Domain ID")))
	require.Equal(t, "LLN0$BR$rcb_B02", strVal(t, readName.Child("Item ID")))

	readResponse := mustChild(t, parsed[14], "Confirmed Response")
	require.Equal(t, uint64(0x04a274), uintVal(t, readResponse.Child("Invoke ID")))
	require.Equal(t, uint64(0xa4), uintVal(t, readResponse.Child("Service Tag")))
	require.Equal(t, uint64(10), uintVal(t, mustChild(t, readResponse, "Read Response", "Access Results", "Data Access Error")))

	require.Equal(t, uint64(0x8b), uintVal(t, parsed[15].Child("PDU Tag")))
	require.Equal(t, uint64(0), uintVal(t, parsed[15].Child("PDU Length")))
	require.Equal(t, uint64(0x8c), uintVal(t, parsed[16].Child("PDU Tag")))
	require.Equal(t, uint64(0), uintVal(t, parsed[16].Child("PDU Length")))
}

func TestProtocolCorpusMMSRejectsEveryTruncatedCapturePDU(t *testing.T) {
	for _, pduCase := range mmsCorpusPDUCases {
		pduCase := pduCase
		valid := mmsCorpusDecodeHex(t, pduCase.hex)
		t.Run("frame "+strconv.Itoa(pduCase.frame), func(t *testing.T) {
			for cut := 0; cut < len(valid); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid[:cut]), "application-layer.mms", "MMSPDU")
				require.Errorf(t, err, "frame %d truncated at byte %d/%d was accepted", pduCase.frame, cut, len(valid))
			}
		})
	}
}

func TestProtocolCorpusMMSRejectsMalformedCapturePDUs(t *testing.T) {
	pdus := make(map[int][]byte, len(mmsCorpusPDUCases))
	for _, pduCase := range mmsCorpusPDUCases {
		pdus[pduCase.frame] = mmsCorpusDecodeHex(t, pduCase.hex)
	}
	mutate := func(frame, offset int, value byte) []byte {
		result := append([]byte(nil), pdus[frame]...)
		result[offset] = value
		return result
	}
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "unsupported PDU tag", input: mutate(7, 0, 0xa3)},
		{name: "long-form root length", input: mutate(7, 1, 0x81)},
		{name: "initiate-request local-detail tag", input: mutate(7, 2, 0x81)},
		{name: "initiate-request calling-limit length", input: mutate(7, 8, 0x02)},
		{name: "initiate-request detail tag", input: mutate(7, 16, 0xa5)},
		{name: "initiate-request detail length", input: mutate(7, 17, 0x15)},
		{name: "initiate-request parameter-CBB length", input: mutate(7, 22, 0x04)},
		{name: "initiate-request services length", input: mutate(7, 27, 0x0b)},
		{name: "initiate-response local-detail tag", input: mutate(8, 2, 0x81)},
		{name: "initiate-response detail length", input: mutate(8, 16, 0x15)},
		{name: "get-attributes service length", input: mutate(10, 8, 0x24)},
		{name: "get-attributes item length", input: mutate(10, 30, 0x0e)},
		{name: "confirmed-error error-class length", input: mutate(12, 10, 0x02)},
		{name: "confirmed-error access tag", input: mutate(12, 11, 0x86)},
		{name: "read-request list length", input: mutate(13, 14, 0x24)},
		{name: "read-request variable tag", input: mutate(13, 15, 0xa1)},
		{name: "read-response access-results length", input: mutate(14, 10, 0x02)},
		{name: "read-response failure length", input: mutate(14, 12, 0x02)},
		{name: "conclude-request non-empty body", input: []byte{0x8b, 0x01, 0x00}},
		{name: "conclude-response non-empty body", input: []byte{0x8c, 0x01, 0x00}},
	}
	for frame, pdu := range pdus {
		tests = append(tests,
			struct {
				name  string
				input []byte
			}{name: "frame " + strconv.Itoa(frame) + " trailing byte", input: append(append([]byte(nil), pdu...), 0)},
		)
		if pdu[1] > 0 {
			tests = append(tests,
				struct {
					name  string
					input []byte
				}{name: "frame " + strconv.Itoa(frame) + " short root length", input: mutate(frame, 1, pdu[1]-1)},
				struct {
					name  string
					input []byte
				}{name: "frame " + strconv.Itoa(frame) + " long root length", input: mutate(frame, 1, pdu[1]+1)},
			)
		}
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(test.input), "application-layer.mms", "MMSPDU")
			require.Error(t, err)
		})
	}
}

func mmsCorpusParseExact(t *testing.T, frame int, input []byte) (*base.Node, *base.NodeValue) {
	t.Helper()
	bounded := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinary(bounded, "application-layer.mms", "MMSPDU")
	require.NoErrorf(t, err, "frame %d MMS parse", frame)
	require.Zero(t, bounded.Len(), "frame %d MMS parser left bytes unread", frame)
	terminals, firstBit, lastBit := protocolCorpusConsumedRange(node, uint64(len(input))*8)
	require.Positivef(t, terminals, "frame %d MMS produced no terminal fields", frame)
	require.Zero(t, firstBit, "frame %d MMS first terminal bit", frame)
	require.Equalf(t, uint64(len(input))*8, lastBit, "frame %d MMS last terminal bit", frame)
	coveredTerminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
	require.NoErrorf(t, coverageErr, "frame %d MMS terminal coverage", frame)
	require.Positivef(t, coveredTerminals, "frame %d MMS produced no covered terminal fields", frame)
	value, err := node.Result()
	require.NoErrorf(t, err, "frame %d MMS structured result", frame)
	require.NotNilf(t, value, "frame %d MMS structured result is nil", frame)
	return node, value
}

func mmsCorpusDecodeHex(t *testing.T, encoded string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(encoded)
	require.NoError(t, err)
	return decoded
}

func TestProtocolCorpusMMSRejectsMalformedInitiateRequests(t *testing.T) {
	valid := []byte{
		0xa8, 0x26,
		0x80, 0x03, 0x00, 0xfa, 0x00,
		0x81, 0x01, 0x0a,
		0x82, 0x01, 0x0a,
		0x83, 0x01, 0x05,
		0xa4, 0x16,
		0x80, 0x01, 0x01,
		0x81, 0x03, 0x05, 0xe1, 0x00,
		0x82, 0x0c, 0x03, 0xa0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe1, 0x10,
	}

	node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid), "application-layer.mms", "MMSInitiateRequest")
	require.NoError(t, err)
	terminals, firstBit, lastBit := protocolCorpusConsumedRange(node, uint64(len(valid))*8)
	require.Positive(t, terminals)
	require.Zero(t, firstBit)
	require.Equal(t, uint64(len(valid))*8, lastBit)

	mutate := func(offset int, value byte) []byte {
		result := append([]byte(nil), valid...)
		result[offset] = value
		return result
	}
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "truncated PDU", input: valid[:len(valid)-1]},
		{name: "trailing byte", input: append(append([]byte(nil), valid...), 0)},
		{name: "wrong PDU tag", input: mutate(0, 0xa9)},
		{name: "forged PDU length long", input: mutate(1, 0x27)},
		{name: "forged PDU length short", input: mutate(1, 0x25)},
		{name: "wrong local-detail tag", input: mutate(2, 0x81)},
		{name: "oversized local-detail", input: mutate(3, 0x05)},
		{name: "wrong calling-limit length", input: mutate(8, 0x02)},
		{name: "wrong detail tag", input: mutate(16, 0xa5)},
		{name: "forged detail length", input: mutate(17, 0x15)},
		{name: "wrong parameter-CBB tag", input: mutate(21, 0x82)},
		{name: "wrong parameter-CBB length", input: mutate(22, 0x04)},
		{name: "wrong services tag", input: mutate(26, 0x83)},
		{name: "wrong services length", input: mutate(27, 0x0b)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(test.input), "application-layer.mms", "MMSInitiateRequest")
			require.Error(t, err)
		})
	}
}
