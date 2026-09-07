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

// This oracle follows the document's wire offsets independently of the rule.
// It checks every observed value and every document/element terminator.
func protocolCorpusCheckBSON(t *testing.T, document *base.Node, wire []byte) {
	t.Helper()
	require.GreaterOrEqual(t, len(wire), 5)
	require.Equal(t, len(wire), int(binary.LittleEndian.Uint32(wire)))
	protocolCorpusRequireValue(t, document, "Size", uint64(len(wire)))
	elements := protocolCorpusFindNode(document, "Elements")
	require.NotNil(t, elements)
	offset := 4
	for index, element := range elements.Children {
		require.Less(t, offset, len(wire))
		kind := wire[offset]
		offset++
		protocolCorpusRequireValue(t, element, "Type", uint64(kind))
		if kind == 0 {
			require.Equal(t, len(wire), offset)
			require.Equal(t, len(elements.Children)-1, index)
			return
		}
		nameEnd := bytes.IndexByte(wire[offset:], 0)
		require.GreaterOrEqual(t, nameEnd, 0)
		protocolCorpusRequireValue(t, element, "Name", string(wire[offset:offset+nameEnd]))
		offset += nameEnd + 1
		switch kind {
		case 0x10:
			require.LessOrEqual(t, offset+4, len(wire))
			protocolCorpusRequireValue(t, element, "Int32", uint64(binary.LittleEndian.Uint32(wire[offset:])))
			offset += 4
		case 0x02:
			require.LessOrEqual(t, offset+4, len(wire))
			length := int(binary.LittleEndian.Uint32(wire[offset:]))
			protocolCorpusRequireValue(t, element, "StrLen", uint64(length))
			offset += 4
			require.Positive(t, length)
			require.LessOrEqual(t, offset+length, len(wire))
			require.Zero(t, wire[offset+length-1])
			protocolCorpusRequireValue(t, element, "Str", string(wire[offset:offset+length]))
			offset += length
		case 0x03, 0x04:
			require.LessOrEqual(t, offset+4, len(wire))
			length := int(binary.LittleEndian.Uint32(wire[offset:]))
			require.GreaterOrEqual(t, length, 5)
			require.LessOrEqual(t, offset+length, len(wire))
			inner := protocolCorpusFindNode(element, "BSONDoc")
			require.NotNil(t, inner)
			protocolCorpusCheckBSON(t, inner, wire[offset:offset+length])
			offset += length
		default:
			t.Fatalf("observed BSON oracle needs an explicit case for type 0x%02x", kind)
		}
	}
	t.Fatal("no terminal document marker")
}

func TestProtocolCorpusMongoDBEveryCapturedMessage(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-mongodb.pcap")
	require.Len(t, frames, 27)
	messages := 0
	for frameIndex, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if len(tcp.Payload) == 0 {
			continue
		}
		messages++
		t.Run(fmt.Sprintf("frame-%d", frameIndex+1), func(t *testing.T) {
			wire := tcp.Payload
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "mongodb", "MongoDB")
			for name, offset := range map[string]int{"Message Length": 0, "Request ID": 4, "Response To": 8, "Op Code": 12, "Flags": 16} {
				protocolCorpusRequireValue(t, node, name, uint64(binary.LittleEndian.Uint32(wire[offset:])))
			}
			require.EqualValues(t, 2004, binary.LittleEndian.Uint32(wire[12:]))
			nameEnd := bytes.IndexByte(wire[20:], 0)
			require.GreaterOrEqual(t, nameEnd, 0)
			protocolCorpusRequireValue(t, node, "Collection", string(wire[20:20+nameEnd]))
			offset := 21 + nameEnd
			protocolCorpusRequireValue(t, node, "Skip", uint64(binary.LittleEndian.Uint32(wire[offset:])))
			protocolCorpusRequireValue(t, node, "Return", uint64(binary.LittleEndian.Uint32(wire[offset+4:])))
			document := protocolCorpusFindNode(node, "BSONDoc")
			require.NotNil(t, document)
			protocolCorpusCheckBSON(t, document, wire[offset+8:])
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "mongodb", "MongoDB")
				require.Errorf(t, err, "cut %d", cut)
			}
		})
	}
	require.Equal(t, 5, messages)
}

func protocolCorpusMongoMessage(document []byte) []byte {
	message := make([]byte, 21, 21+len(document))
	binary.LittleEndian.PutUint32(message, uint32(21+len(document)))
	binary.LittleEndian.PutUint32(message[12:], 2013)
	return append(message, document...)
}

func TestProtocolCorpusMongoDBDocumentFraming(t *testing.T) {
	valid := []string{
		"0500000000",                 // Empty document still consumes its terminal zero.
		"0d000000036400050000000000", // Nested empty document.
		"0f0000001070696e67000100000000",
	}
	for _, encoded := range valid {
		message := protocolCorpusMongoMessage(mustHex(t, encoded))
		node := protocolCorpusRequireBoundedRuleParse(t, message, "mongodb", "MongoDB")
		protocolCorpusCheckBSON(t, protocolCorpusFindNode(node, "BSONDoc"), message[21:])
		reader := newProtocolCorpusBoundedReader(append(bytes.Clone(message), message...))
		_, err := parser.ParseBinary(reader, "mongodb", "MongoDB")
		require.NoError(t, err)
		require.Equal(t, len(message), reader.Len(), "leave the second message intact")
		_, err = parser.ParseBinary(reader, "mongodb", "MongoDB")
		require.NoError(t, err)
		require.Zero(t, reader.Len())
	}
	for _, encoded := range []string{
		"04000000",                   // Too short for the terminal marker.
		"0500000001",                 // Missing marker, incomplete element.
		"060000000000",               // Premature marker.
		"0800000008780002",           // Invalid boolean and missing marker.
		"090000000878000200",         // Invalid boolean with a marker.
		"0800000070780000",           // Unsupported element type.
		"0c0000000278000000000000",   // Zero-length BSON string.
		"0d000000027800010000004100", // String lacks its own NUL.
		"0c0000000364000600000000",   // Child length crosses the parent end.
		"0c0000000364000500000001",   // Parent document lacks its own marker.
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(protocolCorpusMongoMessage(mustHex(t, encoded))), "mongodb", "MongoDB")
		require.Errorf(t, err, "%s", encoded)
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[16] = 1 }, // Unsupported checksum is never silently skipped.
		func(b []byte) { b[16] = 4 }, // Unknown required flag.
		func(b []byte) { b[20] = 1 }, // Different section format.
	} {
		message := protocolCorpusMongoMessage(mustHex(t, valid[0]))
		mutate(message)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(message), "mongodb", "MongoDB")
		require.Error(t, err)
	}
}
