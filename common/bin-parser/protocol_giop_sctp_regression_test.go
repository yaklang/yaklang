package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type giopSCTPExactReader struct {
	*bytes.Reader
	bitLength uint64
}

func newGIOPSCTPExactReader(input []byte) *giopSCTPExactReader {
	return &giopSCTPExactReader{
		Reader:    bytes.NewReader(input),
		bitLength: uint64(len(input)) * 8,
	}
}

func (r *giopSCTPExactReader) InputBitLength() uint64 {
	return r.bitLength
}

func parseGIOPSCTPExact(t *testing.T, input []byte, rule, entry string) *base.Node {
	t.Helper()
	reader := newGIOPSCTPExactReader(input)
	node, err := parser.ParseBinary(reader, rule, entry)
	require.NoError(t, err)
	require.Zero(t, reader.Len(), "parser must consume the complete bounded message")
	return node
}

func rejectGIOPSCTPExact(t *testing.T, input []byte, rule, entry string) {
	t.Helper()
	_, err := parser.ParseBinary(newGIOPSCTPExactReader(input), rule, entry)
	require.Error(t, err)
}

func giopRequest(body []byte) []byte {
	message := make([]byte, 12, 12+len(body))
	copy(message[:4], "GIOP")
	message[4] = 1
	message[5] = 2
	binary.BigEndian.PutUint32(message[8:12], uint32(len(body)))
	return append(message, body...)
}

func legacyGIOPRequest() []byte {
	body := make([]byte, 0, 26)
	body = append(body, 0, 0, 0, 1)
	body = append(body, 3, 0, 0, 0)
	body = append(body, 0, 0, 0, 0)
	body = append(body, 0, 0, 0, 0)
	body = append(body, 0, 0, 0, 6)
	body = append(body, []byte("_is_a\x00")...)
	return giopRequest(body)
}

func fullGIOPRequest() []byte {
	message := legacyUnalignedGIOPRequest()
	body := append([]byte(nil), message[12:65]...)
	body = append(body, make([]byte, 7)...)
	body = append(body, message[65:]...)
	return giopRequest(body)
}

// Preserve the former positive bytes as a negative oracle: the stub starts at
// message offset 65 rather than the required GIOP 1.2 eight-byte boundary.
func legacyUnalignedGIOPRequest() []byte {
	body := append([]byte(nil), legacyGIOPRequest()[12:]...)
	body = append(body, 0, 0)
	body = append(body, 0, 0, 0, 2)
	body = append(body, 0, 0, 0, 7)
	body = append(body, 0, 0, 0, 3)
	body = append(body, 'a', 'b', 'c', 0)
	body = append(body, 0, 0, 0, 8)
	body = append(body, 0, 0, 0, 1)
	body = append(body, 'z')
	body = append(body, 0xde, 0xad, 0xbe, 0xef)
	return giopRequest(body)
}

func completeEmptyGIOPRequest() []byte {
	body := append([]byte(nil), legacyGIOPRequest()[12:]...)
	body = append(body, 0, 0, 0, 0, 0, 0) // alignment and mandatory empty context sequence
	return giopRequest(body)
}

func TestGIOPRequestRequiredTailAndStrictTruncation(t *testing.T) {
	t.Run("legacy operation-only request is incomplete", func(t *testing.T) {
		rejectGIOPSCTPExact(t, legacyGIOPRequest(), "application-layer.iiop", "GIOP")
	})
	t.Run("legacy unaligned stub is incomplete", func(t *testing.T) {
		rejectGIOPSCTPExact(t, legacyUnalignedGIOPRequest(), "application-layer.iiop", "GIOP")
	})
	t.Run("empty body with mandatory context count", func(t *testing.T) {
		node := parseGIOPSCTPExact(t, completeEmptyGIOPRequest(), "application-layer.iiop", "GIOP")
		value, err := node.Result()
		require.NoError(t, err)
		request := mustChild(t, value, "GIOPRequest")
		require.Equal(t, "_is_a\x00", strVal(t, request.Child("Operation")))
		require.Equal(t, uint64(0), uintVal(t, request.Child("Service Context Count")))
		require.Empty(t, bytesVal(t, request.Child("Stub Data")))
	})

	t.Run("aligned context and stub", func(t *testing.T) {
		node := parseGIOPSCTPExact(t, fullGIOPRequest(), "application-layer.iiop", "GIOP")
		value, err := node.Result()
		require.NoError(t, err)
		request := mustChild(t, value, "GIOPRequest")
		require.Equal(t, uint64(2), uintVal(t, request.Child("Service Context Count")))
		contexts := request.Child("Service Contexts").Children()
		require.Len(t, contexts, 2)
		require.Equal(t, uint64(7), uintVal(t, contexts[0].Child("Context ID")))
		require.Equal(t, []byte("abc"), bytesVal(t, contexts[0].Child("Context Data")))
		require.Equal(t, []byte{0}, bytesVal(t, contexts[0].Child("Context Padding")))
		require.Equal(t, uint64(8), uintVal(t, contexts[1].Child("Context ID")))
		require.Equal(t, []byte("z"), bytesVal(t, contexts[1].Child("Context Data")))
		require.Nil(t, contexts[1].Child("Context Padding"))
		require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, bytesVal(t, request.Child("Stub Data")))
	})

	t.Run("partial operation padding", func(t *testing.T) {
		message := legacyGIOPRequest()
		message = append(message, 0)
		binary.BigEndian.PutUint32(message[8:12], 27)
		rejectGIOPSCTPExact(t, message, "application-layer.iiop", "GIOP")
	})

	t.Run("missing context count", func(t *testing.T) {
		message := append(legacyGIOPRequest(), 0, 0)
		binary.BigEndian.PutUint32(message[8:12], 28)
		rejectGIOPSCTPExact(t, message, "application-layer.iiop", "GIOP")
	})

	t.Run("truncated context data", func(t *testing.T) {
		message := fullGIOPRequest()
		rejectGIOPSCTPExact(t, message[:12+42], "application-layer.iiop", "GIOP")
	})

	t.Run("excessive context count", func(t *testing.T) {
		message := fullGIOPRequest()
		binary.BigEndian.PutUint32(message[40:44], 1025)
		rejectGIOPSCTPExact(t, message, "application-layer.iiop", "GIOP")
	})

	t.Run("context length exceeds message", func(t *testing.T) {
		message := fullGIOPRequest()
		binary.BigEndian.PutUint32(message[48:52], 100)
		rejectGIOPSCTPExact(t, message, "application-layer.iiop", "GIOP")
	})
}

func sctpPacket(chunks ...[]byte) []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], 16384)
	binary.BigEndian.PutUint16(packet[2:4], 2944)
	binary.BigEndian.PutUint32(packet[4:8], 0x00016f0a)
	binary.BigEndian.PutUint32(packet[8:12], 0x6db01882)
	for _, chunk := range chunks {
		packet = append(packet, chunk...)
	}
	return packet
}

func sctpDataChunk(payload []byte) []byte {
	length := 16 + len(payload)
	chunk := make([]byte, length)
	chunk[1] = 3
	binary.BigEndian.PutUint16(chunk[2:4], uint16(length))
	binary.BigEndian.PutUint32(chunk[4:8], 0x28024345)
	binary.BigEndian.PutUint16(chunk[8:10], 2)
	binary.BigEndian.PutUint16(chunk[10:12], 3)
	binary.BigEndian.PutUint32(chunk[12:16], 7)
	copy(chunk[16:], payload)
	return append(chunk, make([]byte, (4-length%4)%4)...)
}

func TestSCTPChunkLengthsAndPadding(t *testing.T) {
	t.Run("data padding followed by cookie ack", func(t *testing.T) {
		cookieAck := []byte{11, 0, 0, 4}
		node := parseGIOPSCTPExact(t, sctpPacket(sctpDataChunk([]byte("A")), cookieAck), "sctp", "SCTP")
		value, err := node.Result()
		require.NoError(t, err)
		chunks := value.Child("Chunks").Children()
		require.Len(t, chunks, 2)
		require.Equal(t, uint64(17), uintVal(t, chunks[0].Child("Length")))
		require.Equal(t, "A", strVal(t, chunks[0].Child("User Data")))
		require.Len(t, strVal(t, chunks[0].Child("Padding")), 3)
		require.Equal(t, uint64(11), uintVal(t, chunks[1].Child("Type")))
	})

	t.Run("sack block counts", func(t *testing.T) {
		sack := make([]byte, 24)
		sack[0] = 3
		binary.BigEndian.PutUint16(sack[2:4], 24)
		binary.BigEndian.PutUint32(sack[4:8], 10)
		binary.BigEndian.PutUint32(sack[8:12], 4096)
		binary.BigEndian.PutUint16(sack[12:14], 1)
		binary.BigEndian.PutUint16(sack[14:16], 1)
		binary.BigEndian.PutUint16(sack[16:18], 2)
		binary.BigEndian.PutUint16(sack[18:20], 4)
		binary.BigEndian.PutUint32(sack[20:24], 9)
		node := parseGIOPSCTPExact(t, sctpPacket(sack), "sctp", "SCTP")
		value, err := node.Result()
		require.NoError(t, err)
		chunk := value.Child("Chunks").Children()[0]
		require.Equal(t, []byte{0, 2, 0, 4}, bytesVal(t, chunk.Child("Gap Ack Block Data")))
		require.Equal(t, []byte{0, 0, 0, 9}, bytesVal(t, chunk.Child("Duplicate TSN Data")))
	})

	t.Run("unknown chunk remains bounded", func(t *testing.T) {
		unknown := []byte{99, 1, 0, 5, 0xaa, 0, 0, 0}
		node := parseGIOPSCTPExact(t, sctpPacket(unknown), "sctp", "SCTP")
		value, err := node.Result()
		require.NoError(t, err)
		chunk := value.Child("Chunks").Children()[0]
		require.Equal(t, uint64(99), uintVal(t, chunk.Child("Type")))
		require.Equal(t, []byte{0xaa}, bytesVal(t, chunk.Child("Chunk Data")))
		require.Equal(t, []byte{0, 0, 0}, bytesVal(t, chunk.Child("Padding")))
	})

	validCookieAck := []byte{11, 0, 0, 4}
	malformed := map[string][]byte{
		"missing chunk":               sctpPacket(),
		"zero chunk length":           sctpPacket([]byte{0, 0, 0, 0}),
		"short data fixed header":     sctpPacket([]byte{0, 0, 0, 4}),
		"truncated data fixed header": sctpPacket([]byte{0, 0, 0, 16, 0, 0, 0, 1}),
		"overlong data":               sctpPacket([]byte{0, 0, 0, 20, 0, 0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 7}),
		"missing data padding":        sctpPacket(sctpDataChunk([]byte("A"))[:17]),
		"trailing partial header":     sctpPacket(validCookieAck, []byte{1}),
		"short unknown chunk":         sctpPacket([]byte{99, 0, 0, 3}),
		"overlong unknown chunk":      sctpPacket([]byte{99, 0, 0, 8}),
		"bad cookie ack length":       sctpPacket([]byte{11, 0, 0, 8, 0, 0, 0, 0}),
	}

	badSACK := make([]byte, 16)
	badSACK[0] = 3
	binary.BigEndian.PutUint16(badSACK[2:4], 16)
	binary.BigEndian.PutUint16(badSACK[12:14], 1)
	malformed["sack count length mismatch"] = sctpPacket(badSACK)

	for name, input := range malformed {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			rejectGIOPSCTPExact(t, input, "sctp", "SCTP")
		})
	}
}
