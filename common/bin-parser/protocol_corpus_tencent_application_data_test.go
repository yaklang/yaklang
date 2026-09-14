package bin_parser

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const tencentGamesApplicationDataRule = "application-layer.application_data"

type tencentGamesTCPSegment struct {
	frame    int
	sequence uint32
	payload  []byte
}

type tencentGamesApplicationDataRecord struct {
	start            int
	end              int
	compressedLength int
	decoded          []byte
	object           map[string]json.RawMessage
}

// TestProtocolCorpusTencentGamesFixedSampleApplicationDataFraming deliberately
// describes only branch D in the pinned nDPI capture. It does not claim that
// this record shape is a general Tencent Games protocol specification.
func TestProtocolCorpusTencentGamesFixedSampleApplicationDataFraming(t *testing.T) {
	audit := readClassifierFullCapture(t, tencentGamesFullCapturePath, layers.LinkTypeRaw)

	segments := make([]tencentGamesTCPSegment, 0, 2)
	direction := ""
	frames := make([]int, 0, 2)
	for _, payload := range audit.payloads {
		if payload.transport != "tcp" || payload.tcpStream != 2 {
			continue
		}
		if direction == "" {
			direction = payload.direction
		}
		require.Equalf(t, direction, payload.direction, "fixed TCP stream 2 frame %d application data changed direction", payload.frame)
		require.Equalf(t, []string{"D"}, tencentGamesClassifierBranches(payload.payload), "fixed TCP stream 2 frame %d no longer has the audited branch-D shape", payload.frame)
		segments = append(segments, tencentGamesTCPSegment{
			frame:    payload.frame,
			sequence: payload.tcpSequence,
			payload:  append([]byte(nil), payload.payload...),
		})
		frames = append(frames, payload.frame)
	}

	require.Equal(t, []int{20, 22}, frames, "fixed TCP stream 2 application-data frames changed")
	require.Equal(t, []uint32{3196137429, 3196137767}, []uint32{segments[0].sequence, segments[1].sequence})
	stream := tencentGamesReassembleFixedTCPDirection(t, segments)
	require.Len(t, stream, 771)

	records := tencentGamesParseFixedApplicationDataStream(t, stream)
	require.Len(t, records, 2)
	want := []struct {
		start            int
		end              int
		compressedLength int
		decodedLength    int
	}{
		{start: 0, end: 338, compressedLength: 334, decodedLength: 509},
		{start: 338, end: 771, compressedLength: 429, decodedLength: 736},
	}
	wantKeys := []string{
		"app_id", "body", "broker_vip", "cmd_id", "from_ip", "mod_id", "network_type",
		"process_id", "send_timestamp", "seq_id", "sign", "type", "version",
	}
	for index, record := range records {
		require.Equal(t, want[index].start, record.start)
		require.Equal(t, want[index].end, record.end)
		require.Equal(t, want[index].compressedLength, record.compressedLength)
		require.Len(t, record.decoded, want[index].decodedLength)
		require.Len(t, record.object, 13)
		keys := make([]string, 0, len(record.object))
		for key := range record.object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		require.Equal(t, wantKeys, keys)
	}
	t.Logf("fixed sample TCP stream=2 direction=%s frames=%v bytes=%d records=%d decoded=%d,%d", direction, frames, len(stream), len(records), len(records[0].decoded), len(records[1].decoded))
}

func TestProtocolCorpusTencentGamesFixedSampleApplicationDataRejectsMalformedRecords(t *testing.T) {
	valid := tencentGamesFixedFramePayload(t, 20)
	require.Len(t, valid, 338)

	t.Run("outer record truncated", func(t *testing.T) {
		_, err := tencentGamesParseApplicationDataRule(valid[:len(valid)-1])
		require.ErrorContains(t, err, "compressed length does not match input")
	})

	t.Run("declared compressed length mismatch", func(t *testing.T) {
		malformed := append([]byte(nil), valid...)
		binary.BigEndian.PutUint32(malformed[:4], binary.BigEndian.Uint32(malformed[:4])-1)
		_, err := tencentGamesParseApplicationDataRule(malformed)
		require.ErrorContains(t, err, "compressed length does not match input")
	})

	t.Run("compressed length exceeds bound", func(t *testing.T) {
		malformed := make([]byte, 4+tencentGamesMaxCompressedJSON+1)
		binary.BigEndian.PutUint32(malformed[:4], tencentGamesMaxCompressedJSON+1)
		_, err := tencentGamesParseApplicationDataRule(malformed)
		require.ErrorContains(t, err, "compressed length exceeds 64 KiB")
	})

	t.Run("zlib stream has no EOF", func(t *testing.T) {
		malformed := append([]byte(nil), valid[:len(valid)-1]...)
		binary.BigEndian.PutUint32(malformed[:4], uint32(len(malformed)-4))
		tencentGamesRequireApplicationDataRule(t, malformed)
		_, _, err := decodeTencentGamesFixedZlibJSON(malformed)
		require.Error(t, err)
		require.True(t, errors.Is(err, io.ErrUnexpectedEOF) || bytes.Contains([]byte(err.Error()), []byte("checksum")), "truncated zlib member failed unexpectedly: %v", err)
	})

	t.Run("zlib adler checksum mismatch", func(t *testing.T) {
		malformed := append([]byte(nil), valid...)
		malformed[len(malformed)-1] ^= 0xff
		tencentGamesRequireApplicationDataRule(t, malformed)
		_, _, err := decodeTencentGamesFixedZlibJSON(malformed)
		require.ErrorContains(t, err, "checksum")
	})

	t.Run("bytes trail the zlib member", func(t *testing.T) {
		malformed := append(append([]byte(nil), valid...), 0)
		binary.BigEndian.PutUint32(malformed[:4], uint32(len(malformed)-4))
		tencentGamesRequireApplicationDataRule(t, malformed)
		_, _, err := decodeTencentGamesFixedZlibJSON(malformed)
		require.ErrorContains(t, err, "trailing bytes")
	})

	t.Run("decoded JSON exceeds bound", func(t *testing.T) {
		malformed := tencentGamesBuildZlibRecord(t, bytes.Repeat([]byte{' '}, tencentGamesMaxDecodedJSON+1))
		require.LessOrEqual(t, len(malformed)-4, tencentGamesMaxCompressedJSON)
		tencentGamesRequireApplicationDataRule(t, malformed)
		_, _, err := decodeTencentGamesFixedZlibJSON(malformed)
		require.ErrorContains(t, err, "decoded JSON exceeds 1 MiB")
	})
}

func tencentGamesReassembleFixedTCPDirection(t *testing.T, segments []tencentGamesTCPSegment) []byte {
	t.Helper()
	require.NotEmpty(t, segments)
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].sequence != segments[j].sequence {
			return segments[i].sequence < segments[j].sequence
		}
		return len(segments[i].payload) > len(segments[j].payload)
	})

	baseSequence := uint64(segments[0].sequence)
	stream := make([]byte, 0)
	for _, segment := range segments {
		require.GreaterOrEqualf(t, uint64(segment.sequence), baseSequence, "fixed sample TCP sequence wrapped before frame %d", segment.frame)
		start := int(uint64(segment.sequence) - baseSequence)
		require.LessOrEqualf(t, start, len(stream), "fixed sample TCP stream has a gap before frame %d", segment.frame)
		overlap := len(stream) - start
		if overlap > 0 {
			compared := overlap
			if compared > len(segment.payload) {
				compared = len(segment.payload)
			}
			require.Equalf(t, stream[start:start+compared], segment.payload[:compared], "fixed sample TCP overlap differs at frame %d", segment.frame)
		}
		if overlap < len(segment.payload) {
			stream = append(stream, segment.payload[overlap:]...)
		}
	}
	return stream
}

func tencentGamesParseFixedApplicationDataStream(t *testing.T, stream []byte) []tencentGamesApplicationDataRecord {
	t.Helper()
	records := make([]tencentGamesApplicationDataRecord, 0, 2)
	for offset := 0; offset < len(stream); {
		require.GreaterOrEqualf(t, len(stream)-offset, 4, "fixed sample application-data record prefix at offset %d", offset)
		compressedLength := int(binary.BigEndian.Uint32(stream[offset : offset+4]))
		require.LessOrEqualf(t, compressedLength, tencentGamesMaxCompressedJSON, "fixed sample application-data record at offset %d exceeds compressed bound", offset)
		end := offset + 4 + compressedLength
		require.LessOrEqualf(t, end, len(stream), "fixed sample application-data record at offset %d is truncated", offset)
		recordBytes := stream[offset:end]
		node := tencentGamesRequireApplicationDataRule(t, recordBytes)
		tencentGamesRequireUintField(t, node, "Compressed Length", uint64(compressedLength))
		tencentGamesRequireBytesField(t, node, "Zlib Bytes", recordBytes[4:])
		require.Equal(t, []byte{0x78, 0x01}, recordBytes[4:6], "fixed sample zlib header changed")

		decoded, object, err := decodeTencentGamesFixedZlibJSON(recordBytes)
		require.NoErrorf(t, err, "fixed sample application-data record at offset %d", offset)
		records = append(records, tencentGamesApplicationDataRecord{
			start:            offset,
			end:              end,
			compressedLength: compressedLength,
			decoded:          decoded,
			object:           object,
		})
		offset = end
	}
	return records
}

func tencentGamesParseApplicationDataRule(input []byte) (*base.Node, error) {
	reader := newProtocolCorpusBoundedReader(input)
	node, err := parser.ParseBinary(reader, tencentGamesApplicationDataRule, "LengthPrefixedZlibRecord")
	if err != nil {
		return nil, err
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("application-data rule left %d bytes unread", reader.Len())
	}
	return node, nil
}

func tencentGamesRequireApplicationDataRule(t *testing.T, input []byte) *base.Node {
	t.Helper()
	node, err := tencentGamesParseApplicationDataRule(input)
	require.NoError(t, err)
	result, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, result)
	terminals, firstBit, lastBit := protocolCorpusConsumedRange(node, uint64(len(input))*8)
	require.Equal(t, 2, terminals)
	require.Zero(t, firstBit)
	require.Equal(t, uint64(len(input))*8, lastBit)
	coveredTerminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
	require.NoError(t, coverageErr)
	require.Equal(t, 2, coveredTerminals)
	return node
}

func decodeTencentGamesFixedZlibJSON(record []byte) ([]byte, map[string]json.RawMessage, error) {
	if len(record) < 4 {
		return nil, nil, fmt.Errorf("application-data record is truncated")
	}
	declaredLength := int(binary.BigEndian.Uint32(record[:4]))
	if declaredLength > tencentGamesMaxCompressedJSON {
		return nil, nil, fmt.Errorf("compressed length exceeds 64 KiB")
	}
	if declaredLength != len(record)-4 {
		return nil, nil, fmt.Errorf("compressed length does not match record")
	}

	compressed := bytes.NewReader(record[4:])
	zlibReader, err := zlib.NewReader(compressed)
	if err != nil {
		return nil, nil, fmt.Errorf("zlib header: %w", err)
	}
	decoded, readErr := io.ReadAll(io.LimitReader(zlibReader, tencentGamesMaxDecodedJSON+1))
	if readErr != nil {
		_ = zlibReader.Close()
		return nil, nil, fmt.Errorf("zlib body: %w", readErr)
	}
	if len(decoded) > tencentGamesMaxDecodedJSON {
		_ = zlibReader.Close()
		return nil, nil, fmt.Errorf("decoded JSON exceeds 1 MiB")
	}
	var probe [1]byte
	n, eofErr := zlibReader.Read(probe[:])
	if n != 0 || !errors.Is(eofErr, io.EOF) {
		_ = zlibReader.Close()
		return nil, nil, fmt.Errorf("zlib stream did not end at EOF: bytes=%d err=%w", n, eofErr)
	}
	if closeErr := zlibReader.Close(); closeErr != nil {
		return nil, nil, fmt.Errorf("zlib close: %w", closeErr)
	}
	if compressed.Len() != 0 {
		return nil, nil, fmt.Errorf("zlib member has %d trailing bytes", compressed.Len())
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &object); err != nil {
		return nil, nil, fmt.Errorf("decoded body is not a JSON object: %w", err)
	}
	if object == nil {
		return nil, nil, fmt.Errorf("decoded body is not a JSON object")
	}
	return decoded, object, nil
}

func requireTencentGamesZlibJSON(t *testing.T, frame int, payload []byte) int {
	t.Helper()
	decoded, object, err := decodeTencentGamesFixedZlibJSON(payload)
	require.NoErrorf(t, err, "fixed capture frame %d branch-D zlib JSON", frame)
	require.Lenf(t, object, 13, "fixed capture frame %d branch-D JSON key count", frame)
	return len(decoded)
}

func tencentGamesFixedFramePayload(t *testing.T, frame int) []byte {
	t.Helper()
	audit := readClassifierFullCapture(t, tencentGamesFullCapturePath, layers.LinkTypeRaw)
	for _, payload := range audit.payloads {
		if payload.frame == frame {
			return append([]byte(nil), payload.payload...)
		}
	}
	t.Fatalf("fixed Tencent Games capture has no application payload at frame %d", frame)
	return nil
}

func tencentGamesBuildZlibRecord(t *testing.T, decoded []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(decoded)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	record := make([]byte, 4+compressed.Len())
	binary.BigEndian.PutUint32(record[:4], uint32(compressed.Len()))
	copy(record[4:], compressed.Bytes())
	return record
}

func tencentGamesFindResultNode(node *base.Node, name string) *base.Node {
	if node.Name == name {
		if _, err := node.Result(); err == nil {
			return node
		}
	}
	for _, child := range node.Children {
		if found := tencentGamesFindResultNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

func tencentGamesRequireUintField(t *testing.T, node *base.Node, name string, expected uint64) {
	t.Helper()
	field := tencentGamesFindResultNode(node, name)
	require.NotNil(t, field, "missing field %q", name)
	result, err := field.Result()
	require.NoError(t, err)
	actual, ok := base.InterfaceToUint64(result.Value)
	require.True(t, ok, "field %q has non-numeric type %T", name, result.Value)
	require.Equal(t, expected, actual, "field %q", name)
}

func tencentGamesRequireBytesField(t *testing.T, node *base.Node, name string, expected []byte) {
	t.Helper()
	field := tencentGamesFindResultNode(node, name)
	require.NotNil(t, field, "missing field %q", name)
	result, err := field.Result()
	require.NoError(t, err)
	actual, ok := result.Value.([]byte)
	if !ok {
		if text, textOK := result.Value.(string); textOK {
			actual = []byte(text)
			ok = true
		}
	}
	require.True(t, ok, "field %q has non-byte type %T", name, result.Value)
	require.Equal(t, expected, actual, "field %q", name)
}
