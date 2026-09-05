package bin_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

const (
	iqiyiFullCapturePath        = "testdata/protocol-corpus/captures/ndpi/ndpi-iqiyi.pcap"
	tencentGamesFullCapturePath = "testdata/protocol-corpus/captures/ndpi/ndpi-tencent-games.pcap"
	netEaseGamesFullCapturePath = "testdata/protocol-corpus/captures/ndpi/ndpi-netease-games.pcapng"

	tencentGamesMaxCompressedJSON = 64 << 10
	tencentGamesMaxDecodedJSON    = 1 << 20
)

type protocolCorpusClassifierFullCaptureCase struct {
	capturePath string
}

var protocolCorpusClassifierFullCaptureCases = map[string]protocolCorpusClassifierFullCaptureCase{
	"ndpi-iqiyi":         {capturePath: iqiyiFullCapturePath},
	"ndpi-tencent-games": {capturePath: tencentGamesFullCapturePath},
	"ndpi-netease-games": {capturePath: netEaseGamesFullCapturePath},
}

type classifierCapturePacketReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

type classifierCapturePayload struct {
	frame        int
	transport    string
	payload      []byte
	flow         string
	direction    string
	sourcePort   uint16
	destPort     uint16
	tcpSequence  uint32
	tcpStream    int
	hasDNSLayer  bool
	hasTLSRecord bool
}

type classifierCaptureAudit struct {
	packetCount            int
	withoutApplicationData int
	payloads               []classifierCapturePayload
}

type classifierExpectedPayload struct {
	length         int
	classification string
}

func TestProtocolCorpusClassifierFullCaptureCoverage(t *testing.T) {
	caseIDs := make(map[string]struct{}, len(protocolCorpusClassifierFullCaptureCases))
	for captureID := range protocolCorpusClassifierFullCaptureCases {
		caseIDs[captureID] = struct{}{}
	}
	classifierIDs := make(map[string]struct{}, len(protocolCorpusClassifierSpecs))
	for captureID := range protocolCorpusClassifierSpecs {
		classifierIDs[captureID] = struct{}{}
	}

	require.Len(t, caseIDs, 3, "the full-capture classifier ledger must enumerate the three audited captures")
	require.Equal(t, classifierIDs, caseIDs, "every classifier capture must have a full-capture audit, and no audit may be orphaned")

	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	manifestPaths := make(map[string]string, len(protocolCorpusClassifierFullCaptureCases))
	for _, capture := range manifest.Captures {
		if _, audited := protocolCorpusClassifierFullCaptureCases[capture.ID]; !audited {
			continue
		}
		require.NotContains(t, manifestPaths, capture.ID, "duplicate classifier capture in manifest")
		manifestPaths[capture.ID] = filepath.ToSlash(filepath.Join(corpusDir, filepath.FromSlash(capture.CaptureFile)))
	}

	for captureID, fullCaptureCase := range protocolCorpusClassifierFullCaptureCases {
		manifestPath, exists := manifestPaths[captureID]
		require.Truef(t, exists, "classifier capture %q is missing from manifest", captureID)
		require.Equal(t, fullCaptureCase.capturePath, manifestPath, "classifier full-capture path must match manifest capture_file")
	}
}

func TestProtocolCorpusIQIYIClassifierFullCapture(t *testing.T) {
	audit := readClassifierFullCapture(t, classifierFullCapturePath(t, "ndpi-iqiyi"), layers.LinkTypeRaw)
	require.Equal(t, 2, audit.packetCount)
	require.Zero(t, audit.withoutApplicationData)
	require.Len(t, audit.payloads, 2)
	expected := map[int]classifierExpectedPayload{
		1: {length: 135, classification: "hit"},
		2: {length: 136, classification: "same-flow-nonmatch"},
	}

	hitFlow := ""
	for _, record := range audit.payloads {
		require.Equalf(t, "udp", record.transport, "frame %d transport", record.frame)
		if iqiyiClassifierMatches(record.payload) {
			require.Emptyf(t, hitFlow, "frame %d is an unexpected second classifier hit", record.frame)
			hitFlow = record.flow
		}
	}
	require.NotEmpty(t, hitFlow, "capture has no iQIYI classifier hit")

	counts := map[string]int{}
	for _, record := range audit.payloads {
		classification := ""
		switch {
		case iqiyiClassifierMatches(record.payload):
			classification = "hit"
		case record.flow == hitFlow:
			classification = "same-flow-nonmatch"
		default:
			t.Fatalf("frame %d UDP payload (%d bytes) is neither an iQIYI hit nor an explicit same-flow nonmatch", record.frame, len(record.payload))
		}
		requireClassifierPayloadExpectation(t, expected, record, classification)
		counts[classification]++
		t.Logf("frame=%d transport=udp bytes=%d classification=%s", record.frame, len(record.payload), classification)
	}

	require.Equal(t, map[string]int{"hit": 1, "same-flow-nonmatch": 1}, counts)
	require.Equal(t, len(audit.payloads), counts["hit"]+counts["same-flow-nonmatch"])
	require.Empty(t, expected, "expected iQIYI payload frames were not visited")
}

func TestProtocolCorpusTencentGamesClassifierFullCapture(t *testing.T) {
	audit := readClassifierFullCapture(t, classifierFullCapturePath(t, "ndpi-tencent-games"), layers.LinkTypeRaw)
	require.Equal(t, 32, audit.packetCount)
	require.Equal(t, 22, audit.withoutApplicationData)
	require.Len(t, audit.payloads, 10)
	expected := map[int]classifierExpectedPayload{
		4:  {length: 76, classification: "branch-A"},
		6:  {length: 64, classification: "branch-A"},
		8:  {length: 117, classification: "branch-A"},
		10: {length: 133, classification: "branch-A"},
		14: {length: 150, classification: "branch-B"},
		16: {length: 125, classification: "branch-C"},
		20: {length: 338, classification: "branch-D"},
		22: {length: 433, classification: "branch-D"},
		26: {length: 458, classification: "branch-E"},
		28: {length: 2332, classification: "branch-E"},
	}

	counts := map[string]int{}
	classified := 0
	for _, record := range audit.payloads {
		require.Equalf(t, "tcp", record.transport, "frame %d transport", record.frame)
		require.Falsef(t, record.hasDNSLayer || record.hasTLSRecord, "frame %d unexpectedly decoded as known outer traffic", record.frame)

		matches := tencentGamesClassifierBranches(record.payload)
		require.Lenf(t, matches, 1, "frame %d TCP payload (%d bytes) must match exactly one Tencent Games branch; matches=%v", record.frame, len(record.payload), matches)
		branch := matches[0]
		requireClassifierPayloadExpectation(t, expected, record, "branch-"+branch)
		if branch == "D" {
			decodedBytes := requireTencentGamesZlibJSON(t, record.frame, record.payload)
			t.Logf("frame=%d transport=tcp bytes=%d classification=branch-%s decoded-json-bytes=%d", record.frame, len(record.payload), branch, decodedBytes)
		} else {
			t.Logf("frame=%d transport=tcp bytes=%d classification=branch-%s", record.frame, len(record.payload), branch)
		}
		counts[branch]++
		classified++
	}

	require.Equal(t, map[string]int{"A": 4, "B": 1, "C": 1, "D": 2, "E": 2}, counts)
	require.Equal(t, len(audit.payloads), classified)
	require.Empty(t, expected, "expected Tencent Games payload frames were not visited")
}

func TestProtocolCorpusNetEaseGamesClassifierFullCapture(t *testing.T) {
	audit := readClassifierFullCapture(t, classifierFullCapturePath(t, "ndpi-netease-games"), layers.LinkTypeEthernet)
	require.Equal(t, 20, audit.packetCount)
	require.Equal(t, 4, audit.withoutApplicationData)
	require.Len(t, audit.payloads, 16)
	expected := map[int]classifierExpectedPayload{
		1:  {length: 45, classification: "known-outer-dns"},
		2:  {length: 45, classification: "known-outer-dns"},
		3:  {length: 157, classification: "known-outer-dns"},
		4:  {length: 157, classification: "known-outer-dns"},
		8:  {length: 517, classification: "known-outer-tls"},
		10: {length: 96, classification: "known-outer-tls"},
		11: {length: 12, classification: "branch-A"},
		12: {length: 12, classification: "branch-A-same-flow-nonmatch"},
		13: {length: 12, classification: "branch-A-same-flow-nonmatch"},
		14: {length: 30, classification: "branch-B"},
		15: {length: 30, classification: "branch-B"},
		16: {length: 82, classification: "branch-C"},
		17: {length: 290, classification: "branch-C"},
		18: {length: 34, classification: "branch-C"},
		19: {length: 97, classification: "branch-C"},
		20: {length: 40, classification: "branch-C"},
	}

	firstDirection := make(map[string]string)
	for _, record := range audit.payloads {
		if _, exists := firstDirection[record.flow]; !exists {
			firstDirection[record.flow] = record.direction
		}
	}

	aFlow := ""
	aHits := 0
	for _, record := range audit.payloads {
		fromClient := record.direction == firstDirection[record.flow]
		if record.transport == "udp" && !record.hasDNSLayer && netEaseGamesBranchA(record.payload, fromClient) {
			aFlow = record.flow
			aHits++
		}
	}
	require.Equal(t, 1, aHits, "capture must contain exactly one direction-qualified NetEase branch-A hit")
	require.NotEmpty(t, aFlow)

	counts := map[string]int{}
	classified := 0
	for _, record := range audit.payloads {
		classification := ""
		switch {
		case record.hasDNSLayer:
			require.Equalf(t, "udp", record.transport, "frame %d DNS transport", record.frame)
			require.Truef(t, record.sourcePort == 53 || record.destPort == 53, "frame %d DNS payload does not use port 53", record.frame)
			classification = "known-outer-dns"
		case record.hasTLSRecord:
			require.Equalf(t, "tcp", record.transport, "frame %d TLS transport", record.frame)
			require.Truef(t, record.sourcePort == 443 || record.destPort == 443, "frame %d TLS payload does not use port 443", record.frame)
			classification = "known-outer-tls"
		case record.transport == "udp":
			fromClient := record.direction == firstDirection[record.flow]
			matches := netEaseGamesClassifierBranches(record.payload, fromClient)
			require.LessOrEqualf(t, len(matches), 1, "frame %d UDP payload matches overlapping NetEase branches: %v", record.frame, matches)
			if len(matches) == 1 {
				classification = "branch-" + matches[0]
			} else if record.flow == aFlow {
				classification = "branch-A-same-flow-nonmatch"
			} else {
				t.Fatalf("frame %d UDP payload (%d bytes) is not a NetEase hit, an A-flow nonmatch, or known outer traffic", record.frame, len(record.payload))
			}
		default:
			t.Fatalf("frame %d %s payload (%d bytes) is not explicitly classified", record.frame, record.transport, len(record.payload))
		}

		requireClassifierPayloadExpectation(t, expected, record, classification)
		counts[classification]++
		classified++
		t.Logf("frame=%d transport=%s bytes=%d classification=%s", record.frame, record.transport, len(record.payload), classification)
	}

	require.Equal(t, map[string]int{
		"branch-A":                    1,
		"branch-A-same-flow-nonmatch": 2,
		"branch-B":                    2,
		"branch-C":                    5,
		"known-outer-dns":             4,
		"known-outer-tls":             2,
	}, counts)
	require.Equal(t, len(audit.payloads), classified)
	require.Empty(t, expected, "expected NetEase Games payload frames were not visited")
}

func requireClassifierPayloadExpectation(t *testing.T, expected map[int]classifierExpectedPayload, record classifierCapturePayload, classification string) {
	t.Helper()
	want, exists := expected[record.frame]
	require.Truef(t, exists, "frame %d has an unexpected application payload", record.frame)
	require.Equalf(t, want.length, len(record.payload), "frame %d application payload length", record.frame)
	require.Equalf(t, want.classification, classification, "frame %d application payload classification", record.frame)
	delete(expected, record.frame)
}

func classifierFullCapturePath(t *testing.T, captureID string) string {
	t.Helper()
	fullCaptureCase, exists := protocolCorpusClassifierFullCaptureCases[captureID]
	require.Truef(t, exists, "classifier capture %q has no full-capture case", captureID)
	return fullCaptureCase.capturePath
}

func iqiyiClassifierMatches(payload []byte) bool {
	return len(payload) > 120 && len(payload) < 300 && bytes.Contains(payload, []byte("PPStream"))
}

func tencentGamesClassifierBranches(payload []byte) []string {
	if len(payload) <= 50 {
		return nil
	}

	branches := make([]string, 0, 1)
	if binary.BigEndian.Uint32(payload[0:4]) == 0x3366000b && binary.BigEndian.Uint16(payload[4:6]) == 0x000b {
		branches = append(branches, "A")
	}
	if binary.BigEndian.Uint32(payload[0:4]) == 0x4366aa00 && binary.BigEndian.Uint32(payload[12:16]) == 0x10e68601 {
		branches = append(branches, "B")
	}
	if binary.BigEndian.Uint32(payload[0:4]) == 0xaa000000 && binary.BigEndian.Uint32(payload[10:14]) == 0x10e68601 {
		branches = append(branches, "C")
	}
	if binary.BigEndian.Uint16(payload[0:2]) == 0 && int(binary.BigEndian.Uint16(payload[2:4])) == len(payload)-4 && binary.BigEndian.Uint16(payload[4:6]) == 0x7801 {
		branches = append(branches, "D")
	}
	if binary.BigEndian.Uint32(payload[0:4]) == 0x4215f787 && binary.BigEndian.Uint16(payload[6:8]) == 0 {
		branches = append(branches, "E")
	}
	return branches
}

func netEaseGamesClassifierBranches(payload []byte, fromClient bool) []string {
	branches := make([]string, 0, 1)
	if netEaseGamesBranchA(payload, fromClient) {
		branches = append(branches, "A")
	}
	if len(payload) >= 30 && binary.BigEndian.Uint32(payload[0:4]) == 0xb3af8de8 {
		branches = append(branches, "B")
	}
	if len(payload) > 30 && binary.LittleEndian.Uint32(payload[0:4]) == 0x0c080807 {
		branches = append(branches, "C")
	}
	return branches
}

func netEaseGamesBranchA(payload []byte, fromClient bool) bool {
	return len(payload) == 12 && fromClient && payload[0] == 0x01 &&
		binary.LittleEndian.Uint16(payload[2:4]) == 0x01d0 &&
		binary.LittleEndian.Uint32(payload[8:12]) == 0x01010100
}

func readClassifierFullCapture(t *testing.T, path string, expectedLinkType layers.LinkType) classifierCaptureAudit {
	t.Helper()
	capture, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capture.Close()) })

	var magic [4]byte
	_, err = io.ReadFull(capture, magic[:])
	require.NoError(t, err)
	_, err = capture.Seek(0, io.SeekStart)
	require.NoError(t, err)

	var reader classifierCapturePacketReader
	if bytes.Equal(magic[:], []byte{'\x0a', '\x0d', '\x0d', '\x0a'}) {
		ngReader, openErr := pcapgo.NewNgReader(capture, pcapgo.NgReaderOptions{SkipUnknownVersion: true})
		require.NoError(t, openErr)
		reader = ngReader
	} else {
		pcapReader, openErr := pcapgo.NewReader(capture)
		require.NoError(t, openErr)
		reader = pcapReader
	}
	require.Equal(t, expectedLinkType, reader.LinkType())

	audit := classifierCaptureAudit{}
	tcpStreams := make(map[string]int)
	for {
		frame, _, readErr := reader.ReadPacketData()
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
		audit.packetCount++

		packet := gopacket.NewPacket(frame, reader.LinkType(), gopacket.DecodeOptions{Lazy: false, NoCopy: true})
		if errorLayer := packet.ErrorLayer(); errorLayer != nil {
			t.Fatalf("frame %d decode failed: %v", audit.packetCount, errorLayer.Error())
		}
		networkLayer := packet.NetworkLayer()
		require.NotNilf(t, networkLayer, "frame %d has no network layer", audit.packetCount)

		record := classifierCapturePayload{
			frame:       audit.packetCount,
			tcpStream:   -1,
			hasDNSLayer: packet.Layer(layers.LayerTypeDNS) != nil,
		}
		switch transport := packet.TransportLayer().(type) {
		case *layers.TCP:
			record.transport = "tcp"
			record.payload = append([]byte(nil), transport.Payload...)
			record.sourcePort = uint16(transport.SrcPort)
			record.destPort = uint16(transport.DstPort)
			record.tcpSequence = transport.Seq
		case *layers.UDP:
			record.transport = "udp"
			record.payload = append([]byte(nil), transport.Payload...)
			record.sourcePort = uint16(transport.SrcPort)
			record.destPort = uint16(transport.DstPort)
		default:
			t.Fatalf("frame %d has unsupported or missing transport layer %T", audit.packetCount, packet.TransportLayer())
		}
		record.flow, record.direction = classifierCaptureFlow(networkLayer.NetworkFlow(), record.sourcePort, record.destPort)
		if record.transport == "tcp" {
			stream, exists := tcpStreams[record.flow]
			if !exists {
				stream = len(tcpStreams)
				tcpStreams[record.flow] = stream
			}
			record.tcpStream = stream
		}

		if len(record.payload) == 0 {
			audit.withoutApplicationData++
			continue
		}
		record.hasTLSRecord = record.transport == "tcp" &&
			(record.sourcePort == 443 || record.destPort == 443) &&
			classifierCaptureIsTLSHandshakeRecord(record.payload)
		audit.payloads = append(audit.payloads, record)
	}

	require.Equal(t, audit.packetCount, audit.withoutApplicationData+len(audit.payloads), "every capture frame must be accounted for")
	return audit
}

func classifierCaptureIsTLSHandshakeRecord(payload []byte) bool {
	if len(payload) < 9 || payload[0] != byte(layers.TLSHandshake) || payload[1] != 0x03 {
		return false
	}
	recordLength := int(binary.BigEndian.Uint16(payload[3:5]))
	if recordLength != len(payload)-5 {
		return false
	}
	handshakeLength := int(payload[6])<<16 | int(payload[7])<<8 | int(payload[8])
	return handshakeLength == recordLength-4
}

func classifierCaptureFlow(network gopacket.Flow, sourcePort, destPort uint16) (flow, direction string) {
	source := fmt.Sprintf("%s:%d", network.Src(), sourcePort)
	destination := fmt.Sprintf("%s:%d", network.Dst(), destPort)
	direction = source + ">" + destination
	endpoints := []string{source, destination}
	sort.Strings(endpoints)
	flow = endpoints[0] + "|" + endpoints[1]
	return flow, direction
}
