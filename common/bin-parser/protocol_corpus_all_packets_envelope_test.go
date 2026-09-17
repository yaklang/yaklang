package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// TestProtocolCorpusAllPacketEnvelopes validates the complete packet inventory
// at an intentionally modest semantic level. Every non-empty packet record is
// passed through a bounded bin-parser link- or network-envelope rule. The rule
// parses concrete header fields and names the remaining bounded bytes as a
// payload/trailer; it never probes L4/L7 candidates. USB usbmon records remain
// bound to their pre-existing capture contract. The one unsupported link type
// is parsed only as its real classic-pcap record container; this proves parser
// traversal without claiming support for that link format. This is envelope
// coverage, not per-packet application semantics.
func TestProtocolCorpusAllPacketEnvelopes(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	started := time.Now()

	manifestData, err := os.ReadFile(filepath.Join(corpusDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest protocolCorpusManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	type linkStats struct {
		records           int
		parsed            int
		rejected          int
		truncatedRecord   int
		explicitContainer int
	}
	stats := make(map[string]*linkStats)
	processed := 0
	parserAttempts := 0
	expected := 0
	failures := make([]string, 0)
	rejectedCaptures := make(map[string]int)

	for _, capture := range manifest.Captures {
		expected += capture.PacketCount
		captureData, err := os.ReadFile(filepath.Join(corpusDir, capture.CaptureFile))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: read capture: %v", capture.ID, err))
			continue
		}
		packetReader, err := protocolCorpusEnvelopePacketReader(captureData)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: open capture: %v", capture.ID, err))
			continue
		}
		var rawRecord []byte
		if capture.ID == "tcpdump-unsupported-linktype" {
			rawRecord, err = protocolCorpusClassicPcapSingleRecord(captureData)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: extract classic-pcap record: %v", capture.ID, err))
			}
		}

		captureRecords := 0
		for {
			frame, captureInfo, readErr := packetReader.ReadPacketData()
			if errors.Is(readErr, io.EOF) {
				break
			}
			captureRecords++
			processed++
			link := stats[capture.LinkType]
			if link == nil {
				link = &linkStats{}
				stats[capture.LinkType] = link
			}
			link.records++
			if readErr != nil {
				failures = append(failures, fmt.Sprintf("%s frame %d: read packet: %v", capture.ID, captureRecords, readErr))
				continue
			}

			outcome, usedParser, validateErr := protocolCorpusValidatePacketEnvelope(capture, captureRecords, frame, captureInfo.CaptureLength, captureInfo.Length, rawRecord)
			if usedParser {
				parserAttempts++
			}
			switch outcome {
			case protocolCorpusEnvelopeParsed:
				link.parsed++
			case protocolCorpusEnvelopeRejected:
				link.rejected++
				rejectedCaptures[capture.ID]++
			case protocolCorpusEnvelopeTruncatedRecord:
				link.truncatedRecord++
			case protocolCorpusEnvelopeExplicitContainer:
				link.explicitContainer++
			default:
				failures = append(failures, fmt.Sprintf("%s frame %d: unknown envelope outcome %q", capture.ID, captureRecords, outcome))
			}
			if validateErr != nil {
				failures = append(failures, fmt.Sprintf("%s frame %d [%s]: %v", capture.ID, captureRecords, capture.LinkType, validateErr))
			}
		}
		if captureRecords != capture.PacketCount {
			failures = append(failures, fmt.Sprintf("%s: read %d packet records, manifest declares %d", capture.ID, captureRecords, capture.PacketCount))
		}
	}

	links := make([]string, 0, len(stats))
	for link := range stats {
		links = append(links, link)
	}
	sort.Strings(links)
	for _, link := range links {
		count := stats[link]
		t.Logf("envelope link=%q records=%d parsed=%d rejected=%d truncated-record=%d explicit-container=%d", link, count.records, count.parsed, count.rejected, count.truncatedRecord, count.explicitContainer)
		dispositions := count.parsed + count.rejected + count.truncatedRecord + count.explicitContainer
		if count.records != dispositions {
			failures = append(failures, fmt.Sprintf("link %s: disposition total %d differs from %d records", link, dispositions, count.records))
		}
	}
	if processed != expected {
		failures = append(failures, fmt.Sprintf("processed %d packet records, manifest total is %d", processed, expected))
	}
	if parserAttempts != expected {
		failures = append(failures, fmt.Sprintf("%d of %d packet records entered bin-parser", parserAttempts, expected))
	}
	for id, contract := range protocolCorpusEnvelopeRejectionContracts {
		if rejectedCaptures[id] != contract.count {
			failures = append(failures, fmt.Sprintf("%s: got %d envelope rejection(s), want %d", id, rejectedCaptures[id], contract.count))
		}
	}
	for id := range rejectedCaptures {
		if _, ok := protocolCorpusEnvelopeRejectionContracts[id]; !ok {
			failures = append(failures, fmt.Sprintf("%s: unexpected envelope rejection", id))
		}
	}
	if len(failures) > 0 {
		const displayLimit = 120
		displayed := failures
		if len(displayed) > displayLimit {
			displayed = displayed[:displayLimit]
		}
		t.Fatalf("all-packet envelope validation found %d problem(s); first %d:\n%s", len(failures), len(displayed), strings.Join(displayed, "\n"))
	}
	t.Logf("validated %d packet records with %d bin-parser entries in %s; this is envelope coverage, not per-packet application semantics", processed, parserAttempts, time.Since(started).Round(time.Millisecond))
}

type protocolCorpusEnvelopeOutcome string

const (
	protocolCorpusEnvelopeParsed            protocolCorpusEnvelopeOutcome = "parsed"
	protocolCorpusEnvelopeRejected          protocolCorpusEnvelopeOutcome = "rejected"
	protocolCorpusEnvelopeTruncatedRecord   protocolCorpusEnvelopeOutcome = "truncated-record"
	protocolCorpusEnvelopeExplicitContainer protocolCorpusEnvelopeOutcome = "explicit-container"
)

type protocolCorpusEnvelopeParseSpec struct {
	entry          string
	input          []byte
	frameOffset    int
	successOutcome protocolCorpusEnvelopeOutcome
}

var protocolCorpusEnvelopeRequiredFields = map[string][]string{
	"Ethernet Envelope":                  {"Destination", "Source", "Ether Type"},
	"FDDI Envelope":                      {"Frame Control", "Destination", "Source"},
	"Token Ring Envelope":                {"Access Control", "Frame Control", "Destination", "Source First Byte", "Source Remaining Bytes"},
	"IEEE802154 Envelope":                {"Frame Control", "Sequence Number"},
	"IEEE802154 FCS Envelope":            {"Frame Control", "Sequence Number", "Frame Check Sequence"},
	"SocketCAN Envelope":                 {"Extended Flag", "Remote Flag", "Error Flag", "Identifier", "Data Length", "Flags", "Reserved", "Length Code", "Data"},
	"Truncated Ethernet Record Envelope": {"Destination Byte 1", "Destination Byte 2", "Destination Byte 3", "Destination Byte 4"},
	"Linux SLL Envelope":                 {"Packet Type", "ARPHRD Type", "Address Length", "Address", "Protocol"},
	"Linux SLL2 Envelope":                {"Protocol", "Reserved", "Interface Index", "ARPHRD Type", "Packet Type", "Address Length", "Address"},
	"USBPcap Envelope":                   {"Header Length", "IRP ID", "Status", "Function", "Information", "Bus", "Device", "Endpoint", "Transfer Type", "Data Length"},
	"PPP Envelope":                       {"Protocol High", "Protocol Low"},
	"IPv4 Envelope":                      {"Version", "Header Length", "Differentiated Services", "Total Length", "Identification", "Flags and Fragment Offset", "Time to Live", "Protocol", "Header Checksum", "Source", "Destination"},
	"IPv6 Envelope":                      {"Version", "Traffic Class", "Flow Label", "Payload Length", "Next Header", "Hop Limit", "Source", "Destination"},
	"Null IPv4 Envelope":                 {"Family Byte 1", "Family Byte 2", "Family Byte 3", "Family Byte 4", "Version", "Header Length", "Differentiated Services", "Total Length", "Identification", "Flags and Fragment Offset", "Time to Live", "Protocol", "Header Checksum", "Source", "Destination"},
	"Null IPv6 Envelope":                 {"Family Byte 1", "Family Byte 2", "Family Byte 3", "Family Byte 4", "Version", "Traffic Class", "Flow Label", "Payload Length", "Next Header", "Hop Limit", "Source", "Destination"},
	"RadioTap Envelope":                  {"Version", "Padding", "Header Length", "Present Flags"},
	"Cisco HDLC Envelope":                {"Address", "Control", "Protocol"},
	"PPI Envelope":                       {"Version", "Flags", "Header Length", "Data Link Type"},
	"Frame Relay SNAP Envelope":          {"Address", "Control", "Pad", "NLPID", "OUI", "Ether Type"},
	"USB Mon Envelope":                   {"URB ID", "Event Type", "Transfer Type", "Endpoint", "Device", "Bus", "Setup Flag", "Data Flag", "Timestamp Seconds", "Timestamp Microseconds", "Status", "URB Length", "Captured Length", "Setup", "Interval", "Start Frame", "Transfer Flags", "ISO Descriptor Count"},
	"Pcap Record Envelope":               {"Timestamp Seconds", "Timestamp Fraction", "Captured Length", "Original Length", "Payload"},
}

func protocolCorpusValidateEnvelopeShape(node *base.Node, entry string) error {
	required, ok := protocolCorpusEnvelopeRequiredFields[entry]
	if !ok {
		return fmt.Errorf("envelope %q has no required direct-field contract", entry)
	}
	direct := make(map[string]*base.Node, len(node.Children))
	for _, child := range node.Children {
		if _, duplicate := direct[child.Name]; duplicate {
			return fmt.Errorf("envelope %q has duplicate direct field %q", entry, child.Name)
		}
		direct[child.Name] = child
	}

	for _, name := range required {
		field := direct[name]
		if field == nil {
			return fmt.Errorf("envelope %q is missing required direct field %q", entry, name)
		}
		if !stream_parser.NodeIsTerminal(field) || !stream_parser.NodeHasResult(field) {
			return fmt.Errorf("envelope %q required field %q is not a result-bearing terminal", entry, name)
		}
	}

	var inspect func(*base.Node, int) error
	inspect = func(current *base.Node, depth int) error {
		if stream_parser.NodeHasResult(current) {
			if depth != 1 {
				return fmt.Errorf("envelope %q has result-bearing field %q below the direct-field level", entry, current.Name)
			}
			if !stream_parser.NodeIsTerminal(current) {
				return fmt.Errorf("envelope %q has result-bearing nonterminal direct field %q", entry, current.Name)
			}
		}
		for _, child := range current.Children {
			if err := inspect(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Children {
		if err := inspect(child, 1); err != nil {
			return err
		}
	}
	return nil
}

func protocolCorpusValidatePacketEnvelope(capture protocolCorpusCapture, frameNumber int, frame []byte, captureLength int, originalLength int, rawRecord []byte) (protocolCorpusEnvelopeOutcome, bool, error) {
	if len(frame) == 0 {
		return "", false, errors.New("empty packet record")
	}

	spec, err := protocolCorpusPacketEnvelopeSpec(capture, frameNumber, frame, captureLength, originalLength, rawRecord)
	if err != nil {
		return "", false, err
	}
	if spec.frameOffset < 0 || spec.frameOffset > len(spec.input) || !bytes.Equal(spec.input[spec.frameOffset:], frame) {
		return "", false, fmt.Errorf("envelope entry %q input does not bind exactly to the captured frame at offset %d", spec.entry, spec.frameOffset)
	}
	reader := newProtocolCorpusBoundedReader(spec.input)
	node, parseErr := parser.ParseBinary(reader, "packet_envelope", spec.entry)
	if parseErr != nil {
		outcome, rejectionErr := protocolCorpusClassifyEnvelopeRejection(capture, spec.entry, fmt.Errorf("bounded input=%d bytes remaining=%d: %w", len(spec.input), reader.Len(), parseErr))
		return outcome, true, rejectionErr
	}
	if node == nil {
		return "", true, fmt.Errorf("envelope %q returned a nil parse tree", spec.entry)
	}
	value, err := node.Result()
	if err != nil {
		return "", true, fmt.Errorf("envelope %q result: %w", spec.entry, err)
	}
	if value == nil || len(value.Children()) < 2 {
		return "", true, fmt.Errorf("envelope %q did not return multiple structured fields", spec.entry)
	}
	if err := protocolCorpusValidateEnvelopeShape(node, spec.entry); err != nil {
		return "", true, err
	}
	if reader.Len() != 0 {
		return "", true, fmt.Errorf("envelope %q left %d of %d bounded bytes unread", spec.entry, reader.Len(), len(spec.input))
	}
	terminals, err := protocolCorpusTerminalCoverage(node, spec.input)
	if err != nil {
		return "", true, fmt.Errorf("envelope %q terminal coverage (%d terminals): %w", spec.entry, terminals, err)
	}
	if terminals < 2 {
		return "", true, fmt.Errorf("envelope %q produced only %d terminal field(s)", spec.entry, terminals)
	}
	if spec.successOutcome != "" {
		return spec.successOutcome, true, nil
	}
	return protocolCorpusEnvelopeParsed, true, nil
}

type protocolCorpusEnvelopeRejectionContract struct {
	entry         string
	errorContains string
	count         int
}

var protocolCorpusEnvelopeRejectionContracts = map[string]protocolCorpusEnvelopeRejectionContract{
	"tcpdump-tftp-packet-boundary": {
		entry:         "Linux SLL Envelope",
		errorContains: "packet envelope: Linux SLL packet type is outside [0,6]",
		count:         1,
	},
	"tcpdump-radiotap-header-boundary": {
		entry:         "RadioTap Envelope",
		errorContains: "packet envelope: RadioTap version is not 0",
		count:         1,
	},
}

func protocolCorpusClassifyEnvelopeRejection(capture protocolCorpusCapture, entry string, cause error) (protocolCorpusEnvelopeOutcome, error) {
	contract, ok := protocolCorpusEnvelopeRejectionContracts[capture.ID]
	if !ok {
		return "", fmt.Errorf("envelope %q has no exact rejection contract: %w", entry, cause)
	}
	if capture.EvidenceKind != "upstream-negative" || entry != contract.entry || !strings.Contains(cause.Error(), contract.errorContains) {
		return "", fmt.Errorf("envelope rejection does not match exact contract capture=%q entry=%q error-contains=%q: %w", capture.ID, contract.entry, contract.errorContains, cause)
	}
	return protocolCorpusEnvelopeRejected, nil
}

func protocolCorpusPacketEnvelopeSpec(capture protocolCorpusCapture, frameNumber int, frame []byte, captureLength int, originalLength int, rawRecord []byte) (protocolCorpusEnvelopeParseSpec, error) {
	switch capture.LinkType {
	case "Ethernet":
		if len(frame) < 14 {
			if capture.ID != "ndpi-gnutella" || frameNumber != 1 || len(frame) != 4 || captureLength != 4 || originalLength != 60 {
				return protocolCorpusEnvelopeParseSpec{}, fmt.Errorf("unregistered short Ethernet record: bytes=%d caplen=%d origlen=%d", len(frame), captureLength, originalLength)
			}
			return protocolCorpusEnvelopeParseSpec{entry: "Truncated Ethernet Record Envelope", input: frame, successOutcome: protocolCorpusEnvelopeTruncatedRecord}, nil
		}
		return protocolCorpusEnvelopeParseSpec{entry: "Ethernet Envelope", input: frame}, nil
	case "Linux SLL":
		return protocolCorpusEnvelopeParseSpec{entry: "Linux SLL Envelope", input: frame}, nil
	case "Linux SLL2":
		return protocolCorpusEnvelopeParseSpec{entry: "Linux SLL2 Envelope", input: frame}, nil
	case "USBPcap":
		return protocolCorpusEnvelopeParseSpec{entry: "USBPcap Envelope", input: frame}, nil
	case "FDDI":
		return protocolCorpusEnvelopeParseSpec{entry: "FDDI Envelope", input: frame}, nil
	case "Token Ring":
		return protocolCorpusEnvelopeParseSpec{entry: "Token Ring Envelope", input: frame}, nil
	case "IEEE 802.15.4 no FCS":
		return protocolCorpusEnvelopeParseSpec{entry: "IEEE802154 Envelope", input: frame}, nil
	case "IEEE 802.15.4":
		return protocolCorpusEnvelopeParseSpec{entry: "IEEE802154 FCS Envelope", input: frame}, nil
	case "SocketCAN":
		return protocolCorpusEnvelopeParseSpec{entry: "SocketCAN Envelope", input: frame}, nil
	case "PPP":
		return protocolCorpusEnvelopeParseSpec{entry: "PPP Envelope", input: frame}, nil
	case "RadioTap":
		return protocolCorpusEnvelopeParseSpec{entry: "RadioTap Envelope", input: frame}, nil
	case "Raw":
		return protocolCorpusIPEnvelopeSpec(frame, 0)
	case "Null":
		return protocolCorpusIPEnvelopeSpec(frame, 4)
	case "USB":
		if err := protocolCorpusValidateUSBContainerContract(capture, frameNumber, frame); err != nil {
			return protocolCorpusEnvelopeParseSpec{}, err
		}
		return protocolCorpusEnvelopeParseSpec{entry: "USB Mon Envelope", input: frame, successOutcome: protocolCorpusEnvelopeExplicitContainer}, nil
	case "UnknownLinkType":
		return protocolCorpusUnknownLinkEnvelopeSpec(capture, frame, captureLength, originalLength, rawRecord)
	default:
		return protocolCorpusEnvelopeParseSpec{}, fmt.Errorf("no packet-envelope policy for link type %q", capture.LinkType)
	}
}

func protocolCorpusIPEnvelopeSpec(frame []byte, prefix int) (protocolCorpusEnvelopeParseSpec, error) {
	if len(frame) <= prefix {
		return protocolCorpusEnvelopeParseSpec{entry: "IPv4 Envelope", input: frame}, nil
	}
	entryPrefix := ""
	if prefix == 4 {
		entryPrefix = "Null "
	}
	switch frame[prefix] >> 4 {
	case 4:
		return protocolCorpusEnvelopeParseSpec{entry: entryPrefix + "IPv4 Envelope", input: frame}, nil
	case 6:
		return protocolCorpusEnvelopeParseSpec{entry: entryPrefix + "IPv6 Envelope", input: frame}, nil
	default:
		return protocolCorpusEnvelopeParseSpec{entry: entryPrefix + "IPv4 Envelope", input: frame}, nil
	}
}

func protocolCorpusValidateUSBContainerContract(capture protocolCorpusCapture, frameNumber int, frame []byte) error {
	const usbMonHeaderLength = 64
	registered, ok := protocolCorpusCaptureParseSpecs[capture.ID]
	if !ok || capture.ID != "wireshark-usb-hid" {
		return errors.New("USB record lacks an existing capture parse contract")
	}
	contract := registered.Contract
	if contract.EntryNode != "USBHIDReport" || contract.FrameOffset != usbMonHeaderLength || !contract.OuterOnly {
		return fmt.Errorf("USB capture contract does not bind the %d-byte usbmon envelope to USBHIDReport", usbMonHeaderLength)
	}
	if len(frame) < usbMonHeaderLength {
		return fmt.Errorf("USB frame has %d bytes, needs %d-byte usbmon header", len(frame), usbMonHeaderLength)
	}
	eventType := frame[8]
	if eventType != 'S' && eventType != 'C' {
		return fmt.Errorf("USB frame has unsupported event type %#x", eventType)
	}
	transferType := frame[9]
	wantEndpoint := byte(0)
	switch transferType {
	case 1:
		wantEndpoint = 0x81
	case 2:
		wantEndpoint = 0x80
	default:
		return fmt.Errorf("USB frame transfer type is %d, want interrupt(1) or control(2)", transferType)
	}
	if frame[10] != wantEndpoint {
		return fmt.Errorf("USB frame endpoint is %#x, want %#x for transfer type %d", frame[10], wantEndpoint, transferType)
	}
	capturedLength := binary.LittleEndian.Uint32(frame[36:40])
	if int(capturedLength) != len(frame)-usbMonHeaderLength {
		return fmt.Errorf("USB frame %d captured-data length=%d payload=%d", frameNumber, capturedLength, len(frame)-usbMonHeaderLength)
	}
	return nil
}

func protocolCorpusUnknownLinkEnvelopeSpec(capture protocolCorpusCapture, frame []byte, captureLength int, originalLength int, rawRecord []byte) (protocolCorpusEnvelopeParseSpec, error) {
	switch capture.ID {
	case "ndpi-quic-length-boundary":
		return protocolCorpusIPEnvelopeSpec(frame, 0)
	case "ndpi-bgp-redist":
		return protocolCorpusEnvelopeParseSpec{entry: "Cisco HDLC Envelope", input: frame}, nil
	case "ndpi-someip-sd":
		return protocolCorpusEnvelopeParseSpec{entry: "PPI Envelope", input: frame}, nil
	case "wireshark-arp":
		return protocolCorpusEnvelopeParseSpec{entry: "Frame Relay SNAP Envelope", input: frame}, nil
	case "tcpdump-unsupported-linktype":
		if len(rawRecord) < 16 {
			return protocolCorpusEnvelopeParseSpec{}, errors.New("unsupported link record is missing its classic-pcap record envelope")
		}
		declaredCaptureLength := int(binary.LittleEndian.Uint32(rawRecord[8:12]))
		declaredOriginalLength := int(binary.LittleEndian.Uint32(rawRecord[12:16]))
		if declaredCaptureLength != captureLength || declaredOriginalLength != originalLength {
			return protocolCorpusEnvelopeParseSpec{}, fmt.Errorf("classic-pcap record lengths caplen/origlen=%d/%d differ from reader=%d/%d", declaredCaptureLength, declaredOriginalLength, captureLength, originalLength)
		}
		return protocolCorpusEnvelopeParseSpec{
			entry:          "Pcap Record Envelope",
			input:          rawRecord,
			frameOffset:    16,
			successOutcome: protocolCorpusEnvelopeExplicitContainer,
		}, nil
	default:
		return protocolCorpusEnvelopeParseSpec{}, errors.New("unregistered UnknownLinkType disposition")
	}
}

func protocolCorpusClassicPcapSingleRecord(data []byte) ([]byte, error) {
	const globalHeaderLength = 24
	const recordHeaderLength = 16
	if len(data) < globalHeaderLength+recordHeaderLength {
		return nil, fmt.Errorf("classic-pcap file has %d bytes, needs at least %d", len(data), globalHeaderLength+recordHeaderLength)
	}
	if !bytes.Equal(data[:4], []byte{0xd4, 0xc3, 0xb2, 0xa1}) {
		return nil, fmt.Errorf("classic-pcap magic is %x, want little-endian d4c3b2a1", data[:4])
	}
	record := data[globalHeaderLength:]
	capturedLength := int(binary.LittleEndian.Uint32(record[8:12]))
	originalLength := int(binary.LittleEndian.Uint32(record[12:16]))
	if originalLength < capturedLength {
		return nil, fmt.Errorf("classic-pcap record origlen=%d is below caplen=%d", originalLength, capturedLength)
	}
	if len(record) != recordHeaderLength+capturedLength {
		return nil, fmt.Errorf("classic-pcap record has %d bytes, header declares %d", len(record), recordHeaderLength+capturedLength)
	}
	return record, nil
}

func protocolCorpusEnvelopePacketReader(data []byte) (protocolCorpusPacketReader, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("capture has only %d bytes", len(data))
	}
	if bytes.Equal(data[:4], []byte{'\x0a', '\x0d', '\x0d', '\x0a'}) {
		return pcapgo.NewNgReader(bytes.NewReader(data), pcapgo.NgReaderOptions{SkipUnknownVersion: true})
	}
	reader, err := pcapgo.NewReader(bytes.NewReader(data))
	if err == nil {
		return reader, nil
	}
	normalized, ok := normalizeProtocolCorpusLegacyPcapHeader(data)
	if !ok {
		return nil, err
	}
	return pcapgo.NewReader(bytes.NewReader(normalized))
}
