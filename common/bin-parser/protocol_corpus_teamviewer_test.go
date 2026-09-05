package bin_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const teamViewerCorpusCapture = "testdata/protocol-corpus/captures/ndpi/ndpi-teamviewer.pcap"

type teamViewerExactReader struct {
	*bytes.Reader
}

func (r *teamViewerExactReader) InputBitLength() uint64 {
	return uint64(r.Len()) * 8
}

type teamViewerTCPSegment struct {
	seq     uint32
	payload []byte
}

type teamViewerStreamRange struct {
	start int
	end   int
}

type teamViewerRecordRange struct {
	start int
	end   int
	magic uint16
}

func TestProtocolCorpusTeamViewerAllRecords(t *testing.T) {
	capture, err := os.Open(teamViewerCorpusCapture)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capture.Close()) })

	reader, err := pcapgo.NewReader(capture)
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeEthernet, reader.LinkType())

	streams := map[string][]teamViewerTCPSegment{
		"client-to-server": nil,
		"server-to-client": nil,
	}
	frameClasses := map[string]int{
		"tcp-data":                0,
		"tcp-control(no payload)": 0,
		"udp":                     0,
		"other":                   0,
	}
	packetCount := 0
	for {
		packetData, _, err := reader.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		packetCount++

		packet := gopacket.NewPacket(packetData, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errLayer := packet.ErrorLayer(); errLayer != nil {
			t.Fatalf("frame %d decode failed: %v", packetCount, errLayer.Error())
		}
		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			tcp := tcpLayer.(*layers.TCP)
			if len(tcp.Payload) == 0 {
				frameClasses["tcp-control(no payload)"]++
				continue
			}
			frameClasses["tcp-data"]++
			direction := ""
			switch {
			case tcp.DstPort == 5938:
				direction = "client-to-server"
			case tcp.SrcPort == 5938:
				direction = "server-to-client"
			default:
				t.Fatalf("frame %d has unexpected data-bearing TCP ports %d -> %d", packetCount, tcp.SrcPort, tcp.DstPort)
			}
			streams[direction] = append(streams[direction], teamViewerTCPSegment{
				seq:     tcp.Seq,
				payload: append([]byte(nil), tcp.Payload...),
			})
			continue
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			udp := udpLayer.(*layers.UDP)
			frameClasses["udp"]++
			require.GreaterOrEqualf(t, len(udp.Payload), 16, "frame %d UDP payload", packetCount)
			require.Equalf(t, uint16(0x1724), binary.BigEndian.Uint16(udp.Payload[11:13]), "frame %d nested magic", packetCount)
			bodyLength := int(binary.LittleEndian.Uint16(udp.Payload[14:16]))
			require.Equalf(t, 16+bodyLength, len(udp.Payload), "frame %d nested 1724 length", packetCount)
			node := teamViewerParseExact(t, udp.Payload, "TeamViewerDatagram")
			teamViewerRequireBytes(t, node, "Datagram Prefix", udp.Payload[:11])
			teamViewerRequireUint(t, node, "Magic", 0x1724)
			teamViewerRequireUint(t, node, "Command", uint64(udp.Payload[13]))
			teamViewerRequireUint(t, node, "Body Length 16", uint64(bodyLength))
			if bodyLength > 0 {
				teamViewerRequireBytes(t, node, "Opaque Body", udp.Payload[16:])
			}
			continue
		}
		frameClasses["other"]++
	}

	require.Equal(t, 352, packetCount)
	require.Equal(t, 161, frameClasses["tcp-data"])
	require.Equal(t, 128, frameClasses["tcp-control(no payload)"])
	require.Equal(t, 63, frameClasses["udp"])
	require.Zero(t, frameClasses["other"])
	require.Equal(t, packetCount,
		frameClasses["tcp-data"]+
			frameClasses["tcp-control(no payload)"]+
			frameClasses["udp"]+
			frameClasses["other"],
		"every TeamViewer frame must belong to exactly one explicit class",
	)

	type streamExpectation struct {
		records     int
		records1724 int
		records1130 int
	}
	expectations := map[string]streamExpectation{
		"client-to-server": {records: 74, records1724: 4, records1130: 70},
		"server-to-client": {records: 60, records1724: 0, records1130: 60},
	}
	for direction, expectation := range expectations {
		direction := direction
		expectation := expectation
		t.Run(direction, func(t *testing.T) {
			stream, segmentRanges := teamViewerReassembleTCP(t, streams[direction])
			recordRanges := teamViewerParseStream(t, stream)

			require.Len(t, recordRanges, expectation.records)
			counts := map[uint16]int{}
			for _, record := range recordRanges {
				counts[record.magic]++
			}
			require.Equal(t, expectation.records1724, counts[0x1724])
			require.Equal(t, expectation.records1130, counts[0x1130])
			teamViewerRequireSegmentationCoverage(t, segmentRanges, recordRanges)
			t.Logf("%s: %d bytes, %d records (1724=%d, 1130=%d)", direction, len(stream), len(recordRanges), counts[0x1724], counts[0x1130])
		})
	}
}

func TestProtocolCorpusTeamViewerRejectsMalformedRecords(t *testing.T) {
	record1724 := []byte{0x17, 0x24, 0x09, 0x02, 0x00, 0xaa, 0xbb}
	record1130 := make([]byte, 24)
	binary.BigEndian.PutUint16(record1130[0:2], 0x1130)
	record1130[2] = 0x3c
	record1130[3] = 0
	binary.LittleEndian.PutUint32(record1130[4:8], 0)
	binary.LittleEndian.PutUint32(record1130[8:12], 7)

	teamViewerParseExact(t, record1724, "TeamViewerRecord")
	teamViewerParseExact(t, record1130, "TeamViewerRecord")

	tests := []struct {
		name  string
		entry string
		input []byte
	}{
		{name: "wrong magic", entry: "TeamViewerRecord", input: []byte{0x17, 0x25, 0, 0, 0}},
		{name: "1724 forged long body", entry: "TeamViewerRecord", input: []byte{0x17, 0x24, 0, 0xff, 0x7f}},
		{name: "1724 forged short body", entry: "TeamViewerRecord", input: []byte{0x17, 0x24, 0, 0x00, 0x00, 0xaa}},
		{name: "1724 truncated header", entry: "TeamViewerRecord", input: []byte{0x17, 0x24, 0}},
		{name: "1130 over limit", entry: "TeamViewerRecord", input: append([]byte{0x11, 0x30, 0, 0, 0x01, 0x00, 0x00, 0x01}, make([]byte, 16)...)},
		{name: "1130 truncated fixed header", entry: "TeamViewerRecord", input: record1130[:23]},
		{name: "1130 truncated body", entry: "TeamViewerRecord", input: func() []byte {
			input := append([]byte(nil), record1130...)
			binary.LittleEndian.PutUint32(input[4:8], 1)
			return input
		}()},
		{name: "datagram truncated prefix", entry: "TeamViewerDatagram", input: make([]byte, 15)},
		{name: "datagram wrong nested magic", entry: "TeamViewerDatagram", input: append(make([]byte, 11), 0x17, 0x25, 0, 0, 0)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			reader := &teamViewerExactReader{Reader: bytes.NewReader(test.input)}
			_, err := parser.ParseBinary(reader, "application-layer.teamviewer", test.entry)
			require.Error(t, err)
		})
	}
}

func teamViewerReassembleTCP(t *testing.T, segments []teamViewerTCPSegment) ([]byte, []teamViewerStreamRange) {
	t.Helper()
	require.NotEmpty(t, segments)
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].seq != segments[j].seq {
			return segments[i].seq < segments[j].seq
		}
		return len(segments[i].payload) > len(segments[j].payload)
	})

	baseSequence := segments[0].seq
	stream := make([]byte, 0)
	ranges := make([]teamViewerStreamRange, 0, len(segments))
	for _, segment := range segments {
		start := int(uint64(segment.seq) - uint64(baseSequence))
		require.LessOrEqual(t, start, len(stream), "TCP stream has a gap before sequence %d", segment.seq)
		overlap := len(stream) - start
		if overlap > 0 {
			compared := overlap
			if compared > len(segment.payload) {
				compared = len(segment.payload)
			}
			require.Equal(t, stream[start:start+compared], segment.payload[:compared], "overlapping TCP bytes differ at sequence %d", segment.seq)
		}
		if overlap >= len(segment.payload) {
			continue
		}
		appendStart := len(stream)
		stream = append(stream, segment.payload[overlap:]...)
		ranges = append(ranges, teamViewerStreamRange{start: appendStart, end: len(stream)})
	}
	return stream, ranges
}

func teamViewerParseStream(t *testing.T, stream []byte) []teamViewerRecordRange {
	t.Helper()
	records := make([]teamViewerRecordRange, 0)
	for offset := 0; offset < len(stream); {
		require.GreaterOrEqual(t, len(stream)-offset, 2, "record magic at offset %d", offset)
		magic := binary.BigEndian.Uint16(stream[offset : offset+2])
		recordLength := 0
		switch magic {
		case 0x1724:
			require.GreaterOrEqual(t, len(stream)-offset, 5, "1724 header at offset %d", offset)
			recordLength = 5 + int(binary.LittleEndian.Uint16(stream[offset+3:offset+5]))
		case 0x1130:
			require.GreaterOrEqual(t, len(stream)-offset, 24, "1130 header at offset %d", offset)
			bodyLength := binary.LittleEndian.Uint32(stream[offset+4 : offset+8])
			require.LessOrEqual(t, bodyLength, uint32(16*1024*1024), "1130 body length at offset %d", offset)
			recordLength = 24 + int(bodyLength)
		default:
			t.Fatalf("unsupported record magic %#04x at stream offset %d", magic, offset)
		}
		require.LessOrEqual(t, offset+recordLength, len(stream), "record at offset %d exceeds reassembled stream", offset)
		recordBytes := stream[offset : offset+recordLength]
		node := teamViewerParseExact(t, recordBytes, "TeamViewerRecord")
		teamViewerRequireUint(t, node, "Magic", uint64(magic))
		teamViewerRequireUint(t, node, "Command", uint64(recordBytes[2]))
		switch magic {
		case 0x1724:
			teamViewerRequireUint(t, node, "Body Length 16", uint64(recordLength-5))
			if recordLength > 5 {
				teamViewerRequireBytes(t, node, "Opaque Body", recordBytes[5:])
			}
		case 0x1130:
			teamViewerRequireUint(t, node, "Header Byte", uint64(recordBytes[3]))
			teamViewerRequireUint(t, node, "Body Length 32", uint64(recordLength-24))
			teamViewerRequireUint(t, node, "Counter", uint64(binary.LittleEndian.Uint32(recordBytes[8:12])))
			teamViewerRequireBytes(t, node, "Opaque Header", recordBytes[12:24])
			if recordLength > 24 {
				teamViewerRequireBytes(t, node, "Opaque Body", recordBytes[24:])
			}
		}
		records = append(records, teamViewerRecordRange{start: offset, end: offset + recordLength, magic: magic})
		offset += recordLength
	}
	return records
}

func teamViewerRequireSegmentationCoverage(t *testing.T, segments []teamViewerStreamRange, records []teamViewerRecordRange) {
	t.Helper()
	multipleStartsInOneSegment := false
	for _, segment := range segments {
		starts := 0
		for _, record := range records {
			if record.start >= segment.start && record.start < segment.end {
				starts++
			}
		}
		if starts >= 2 {
			multipleStartsInOneSegment = true
			break
		}
	}
	require.True(t, multipleStartsInOneSegment, "capture must exercise multiple record starts in one TCP segment")

	crossesSegmentBoundary := false
	for _, segment := range segments[1:] {
		boundary := segment.start
		for _, record := range records {
			if record.start < boundary && boundary < record.end {
				crossesSegmentBoundary = true
				break
			}
		}
		if crossesSegmentBoundary {
			break
		}
	}
	require.True(t, crossesSegmentBoundary, "capture must exercise a record split across TCP segments")
}

func teamViewerParseExact(t *testing.T, input []byte, entry string) *base.Node {
	t.Helper()
	reader := &teamViewerExactReader{Reader: bytes.NewReader(input)}
	node, err := parser.ParseBinary(reader, "application-layer.teamviewer", entry)
	require.NoError(t, err)
	result, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, reader.Len())

	terminals, firstBit, lastBit := teamViewerConsumedRange(node)
	require.Positive(t, terminals)
	require.Zero(t, firstBit)
	require.Equal(t, uint64(len(input))*8, lastBit, fmt.Sprintf("%s did not consume the complete bounded input", entry))
	coveredTerminals, coverageErr := protocolCorpusTerminalCoverage(node, input)
	require.NoError(t, coverageErr, "%s terminal fields do not cover the complete bounded input", entry)
	require.Positive(t, coveredTerminals)
	require.NotNil(t, teamViewerFindResultNode(node, "Magic"))
	require.NotNil(t, teamViewerFindResultNode(node, "Command"))
	return node
}

func teamViewerConsumedRange(node *base.Node) (terminals int, firstBit, lastBit uint64) {
	first := true
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if stream_parser.NodeHasResult(current) {
			position := stream_parser.GetNodeResultPos(current)
			terminals++
			if first || position[0] < firstBit {
				firstBit = position[0]
			}
			if first || position[1] > lastBit {
				lastBit = position[1]
			}
			first = false
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	return terminals, firstBit, lastBit
}

func teamViewerFindResultNode(node *base.Node, name string) *base.Node {
	if node.Name == name && stream_parser.NodeHasResult(node) {
		return node
	}
	for _, child := range node.Children {
		if found := teamViewerFindResultNode(child, name); found != nil {
			return found
		}
	}
	return nil
}

func teamViewerRequireUint(t *testing.T, node *base.Node, name string, expected uint64) {
	t.Helper()
	field := teamViewerFindResultNode(node, name)
	require.NotNil(t, field, "missing field %q", name)
	value, err := field.Result()
	require.NoError(t, err)
	actual, ok := base.InterfaceToUint64(value.Value)
	require.True(t, ok, "field %q has non-numeric type %T", name, value.Value)
	require.Equal(t, expected, actual, "field %q", name)
}

func teamViewerRequireBytes(t *testing.T, node *base.Node, name string, expected []byte) {
	t.Helper()
	field := teamViewerFindResultNode(node, name)
	require.NotNil(t, field, "missing field %q", name)
	value, err := field.Result()
	require.NoError(t, err)
	var actual []byte
	switch typed := value.Value.(type) {
	case []byte:
		actual = typed
	case string:
		actual = []byte(typed)
	default:
		t.Fatalf("field %q has non-byte type %T", name, value.Value)
	}
	require.Equal(t, expected, actual, "field %q", name)
}
