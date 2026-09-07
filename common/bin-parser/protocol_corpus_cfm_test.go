package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Primary layout oracle: Wireshark packet-cfm.c, whose implementation records
// IEEE 802.1Q-2022 and ITU-T G.8013/Y.1731 as its source specifications.  This
// independent fixture encodes one complete opcode-1 CCM rather than extending
// the intentionally incomplete PR #5023 identification sample.
// https://gitlab.com/wireshark/wireshark/-/blob/0d289c003bfb3e40b0480860b27e35a5b4e1b279/epan/dissectors/packet-cfm.c
func cfmValidCCM(t *testing.T) []byte {
	t.Helper()
	maid := make([]byte, 48)
	copy(maid, []byte{4, 4, 'A', 'C', 'M', 'E', 2, 5, 'M', 'A', '-', '0', '1'})

	body := []byte{
		0x60, // MD level 3, version 0.
		0x01, // CCM.
		0xc5, // RDI, Traffic, reserved 0, 10-second interval.
		70,   // First TLV is at common-header offset 4+70 = 74.
		0x10, 0x20, 0x30, 0x40,
		0x12, 0x34, // MEP ID 4660 (upper reserved bits are zero).
	}
	body = append(body, maid...)
	body = append(body,
		0x01, 0x02, 0x03, 0x04, // TxFCf
		0x11, 0x22, 0x33, 0x44, // RxFCb
		0x55, 0x66, 0x77, 0x88, // TxFCb
		0, 0, 0, 0, // Y.1731 reserved
	)
	require.Len(t, body, 74)
	body = append(body,
		2, 0, 1, 2, // Port Status: psUp.
		3, 0, 3, 0xaa, 0xbb, 0xcc, // Data.
		4, 0, 1, 3, // Interface Status: isTesting.
		31, 0, 6, 0x00, 0x1b, 0x21, 7, 0xde, 0xad, // Organizational-Specific.
		250, 0, 2, 0x12, 0x34, // Unsupported TLV retained opaque.
		0, // End TLV.
	)
	require.Len(t, body, 103)
	return body
}

func cfmEthernetFrame(body []byte) []byte {
	frame := []byte{
		0x01, 0x80, 0xc2, 0x00, 0x00, 0x33,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x89, 0x02,
	}
	return append(frame, body...)
}

func cfmParse(t *testing.T, body []byte, entry string) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(body)
	node, err := parser.ParseBinary(reader, "cfm", entry)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Equal(t, body, NodeToBytes(node))
	require.Zero(t, reader.Len())
	return node
}

func cfmRequireSpan(t *testing.T, node *base.Node, name string, start, end int) {
	t.Helper()
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field, name)
	require.Equal(t, [2]uint64{uint64(start) * 8, uint64(end) * 8}, stream_parser.GetNodeResultPos(field), name)
}

func cfmFindTLV(t *testing.T, node *base.Node, typ byte) *base.Node {
	t.Helper()
	for _, candidate := range protocolCorpusNodesNamed(node, "TLV") {
		field := protocolCorpusFindNode(candidate, "TLV Type")
		if field == nil {
			continue
		}
		value, err := field.Result()
		require.NoError(t, err)
		if uintVal(t, value) == uint64(typ) {
			return candidate
		}
	}
	t.Fatalf("missing CFM TLV type %d", typ)
	return nil
}

func cfmRequireCCMFields(t *testing.T, node *base.Node, offset int) {
	t.Helper()
	for _, field := range []struct {
		name       string
		value      any
		start, end int
	}{
		{"MD Level", uint64(3), 0, 1},
		{"Version", uint64(0), 0, 1},
		{"Opcode", uint64(1), 1, 2},
		{"Remote Defect Indication", uint64(1), 2, 3},
		{"Traffic", uint64(1), 2, 3},
		{"Flags Reserved", uint64(0), 2, 3},
		{"CCM Interval", uint64(5), 2, 3},
		{"First TLV Offset", uint64(70), 3, 4},
		{"Sequence Number", uint64(0x10203040), 4, 8},
		{"MEP ID Reserved", uint64(0), 8, 9},
		{"MEP ID", uint64(0x1234), 8, 10},
		{"MD Name Format", uint64(4), 10, 11},
		{"MD Name Length", uint64(4), 11, 12},
		{"MD Name", []byte("ACME"), 12, 16},
		{"Short MA Name Format", uint64(2), 16, 17},
		{"Short MA Name Length", uint64(5), 17, 18},
		{"Short MA Name", []byte("MA-01"), 18, 23},
		{"MAID Padding", make([]byte, 35), 23, 58},
		{"TxFCf", uint64(0x01020304), 58, 62},
		{"RxFCb", uint64(0x11223344), 62, 66},
		{"TxFCb", uint64(0x55667788), 66, 70},
		{"Y.1731 Reserved", []byte{0, 0, 0, 0}, 70, 74},
	} {
		protocolCorpusRequireValue(t, node, field.name, field.value)
		switch field.name {
		case "MD Level", "Version", "Remote Defect Indication", "Traffic", "Flags Reserved", "CCM Interval", "MEP ID Reserved", "MEP ID":
			// Exact bit positions are asserted separately below.
		default:
			cfmRequireSpan(t, node, field.name, offset+field.start, offset+field.end)
		}
	}
	bitOffset := uint64(offset) * 8
	require.Equal(t, [2]uint64{bitOffset, bitOffset + 3}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "MD Level")))
	require.Equal(t, [2]uint64{bitOffset + 3, bitOffset + 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Version")))
	require.Equal(t, [2]uint64{bitOffset + 16, bitOffset + 17}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Remote Defect Indication")))
	require.Equal(t, [2]uint64{bitOffset + 17, bitOffset + 18}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Traffic")))
	require.Equal(t, [2]uint64{bitOffset + 18, bitOffset + 21}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Flags Reserved")))
	require.Equal(t, [2]uint64{bitOffset + 21, bitOffset + 24}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "CCM Interval")))
	require.Equal(t, [2]uint64{bitOffset + 64, bitOffset + 67}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "MEP ID Reserved")))
	require.Equal(t, [2]uint64{bitOffset + 67, bitOffset + 80}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "MEP ID")))

	port := cfmFindTLV(t, node, 2)
	protocolCorpusRequireValue(t, port, "TLV Length", uint64(1))
	protocolCorpusRequireValue(t, port, "Port Status", uint64(2))
	cfmRequireSpan(t, port, "TLV Type", offset+74, offset+75)
	cfmRequireSpan(t, port, "TLV Length", offset+75, offset+77)
	cfmRequireSpan(t, port, "Port Status", offset+77, offset+78)

	data := cfmFindTLV(t, node, 3)
	protocolCorpusRequireValue(t, data, "TLV Length", uint64(3))
	protocolCorpusRequireValue(t, data, "Data", []byte{0xaa, 0xbb, 0xcc})
	cfmRequireSpan(t, data, "Data", offset+81, offset+84)

	status := cfmFindTLV(t, node, 4)
	protocolCorpusRequireValue(t, status, "TLV Length", uint64(1))
	protocolCorpusRequireValue(t, status, "Interface Status", uint64(3))
	cfmRequireSpan(t, status, "Interface Status", offset+87, offset+88)

	organization := cfmFindTLV(t, node, 31)
	protocolCorpusRequireValue(t, organization, "TLV Length", uint64(6))
	protocolCorpusRequireValue(t, organization, "OUI", []byte{0x00, 0x1b, 0x21})
	protocolCorpusRequireValue(t, organization, "Subtype", uint64(7))
	protocolCorpusRequireValue(t, organization, "Organization Value", []byte{0xde, 0xad})
	cfmRequireSpan(t, organization, "OUI", offset+91, offset+94)
	cfmRequireSpan(t, organization, "Subtype", offset+94, offset+95)
	cfmRequireSpan(t, organization, "Organization Value", offset+95, offset+97)

	unknown := cfmFindTLV(t, node, 250)
	protocolCorpusRequireValue(t, unknown, "TLV Length", uint64(2))
	protocolCorpusRequireValue(t, unknown, "Opaque Value", []byte{0x12, 0x34})
	cfmRequireSpan(t, unknown, "Opaque Value", offset+100, offset+102)

	end := cfmFindTLV(t, node, 0)
	protocolCorpusRequireValue(t, end, "TLV Type", uint64(0))
	cfmRequireSpan(t, end, "TLV Type", offset+102, offset+103)
}

func TestProtocolCorpusCFMOriginalIsIncompleteEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-cfm.pcap")
	require.Len(t, frames, 1)
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			require.Len(t, frame, 86)
			require.Equal(t, []byte{0x89, 0x02}, frame[12:14])
			require.Equal(t, []byte{0, 1, 0, 0}, frame[14:18])
			require.Equal(t, make([]byte, 68), frame[18:])
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[14:]), "cfm", "CFM")
			require.ErrorContains(t, err, "shorter than its fixed body")

			carrier := cfmParse(t, frame[14:], "CFMCarrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed CFM Payload", frame[14:])
			require.Nil(t, protocolCorpusFindNode(carrier, "MD Level"))
			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, envelope, "Type", uint64(0x8902))
			protocolCorpusRequireValue(t, envelope, "Unparsed CFM Payload", frame[14:])
			cfmRequireSpan(t, envelope, "Unparsed CFM Payload", 14, len(frame))
		})
	}
}

func TestProtocolCorpusCFMCompleteCCMFields(t *testing.T) {
	body := cfmValidCCM(t)
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-cfm-valid.pcap")
	require.Len(t, frames, 1)
	frame := cfmEthernetFrame(body)
	require.Equal(t, frame, frames[0], "checked-in companion must match the independent encoding")
	for _, entry := range []string{"CFM", "CFMCarrier"} {
		t.Run(entry, func(t *testing.T) {
			node := cfmParse(t, body, entry)
			cfmRequireCCMFields(t, node, 0)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed CFM Payload"))
		})
	}
	require.Len(t, frame, 117)
	require.Equal(t, []byte{0x89, 0x02}, frame[12:14])
	envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, envelope, "Type", uint64(0x8902))
	cfmRequireCCMFields(t, envelope, 14)
	require.Nil(t, protocolCorpusFindNode(envelope, "Unparsed CFM Payload"))
}

func TestProtocolCorpusCFMAllShortPrefixesAndBounds(t *testing.T) {
	body := cfmValidCCM(t)
	validWithoutEndAt := map[int]bool{74: true, 78: true, 84: true, 88: true, 97: true, 102: true}
	for end := 0; end < len(body); end++ {
		node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body[:end]), "cfm", "CFM")
		if validWithoutEndAt[end] {
			require.NoError(t, err, "prefix %d ends exactly after a fixed body or declared TLV", end)
			require.Equal(t, body[:end], NodeToBytes(node))
		} else {
			require.Error(t, err, "prefix %d", end)
		}
		if end == 0 {
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "cfm", "CFMCarrier")
			require.ErrorContains(t, err, "empty carrier has no message")
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(cfmEthernetFrame(nil)), "ethernet", "Ethernet")
			require.ErrorContains(t, err, "empty carrier has no message")
			continue
		}
		carrier := cfmParse(t, body[:end], "CFMCarrier")
		if validWithoutEndAt[end] {
			protocolCorpusRequireValue(t, carrier, "MEP ID", uint64(0x1234))
			require.Nil(t, protocolCorpusFindNode(carrier, "Unparsed CFM Payload"))
		} else {
			protocolCorpusRequireValue(t, carrier, "Unparsed CFM Payload", body[:end])
			require.Nil(t, protocolCorpusFindNode(carrier, "MEP ID"), "prefix %d retained a failed candidate", end)
		}
		envelope := protocolCorpusRequireBoundedRuleParse(t, cfmEthernetFrame(body[:end]), "ethernet", "Ethernet")
		if validWithoutEndAt[end] {
			protocolCorpusRequireValue(t, envelope, "MEP ID", uint64(0x1234))
			require.Nil(t, protocolCorpusFindNode(envelope, "Unparsed CFM Payload"))
		} else {
			protocolCorpusRequireValue(t, envelope, "Unparsed CFM Payload", body[:end])
			cfmRequireSpan(t, envelope, "Unparsed CFM Payload", 14, 14+end)
		}
	}
	_, err := parser.ParseBinary(bytes.NewBuffer(body), "cfm", "CFM")
	require.ErrorContains(t, err, "explicit message boundary required")
	_, err = parser.ParseBinary(bytes.NewBuffer(body), "cfm", "CFMCarrier")
	require.ErrorContains(t, err, "explicit carrier boundary required")
	oversize := make([]byte, 65536)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(oversize), "cfm", "CFM")
	require.ErrorContains(t, err, "supported bounded size")
}

func cfmTLVCountBody(t *testing.T, count int) []byte {
	t.Helper()
	require.GreaterOrEqual(t, count, 1)
	body := bytes.Clone(cfmValidCCM(t)[:74])
	for index := 0; index < count-1; index++ {
		body = append(body, 250, 0, 0) // Unknown, zero-length, structurally complete TLV.
	}
	body = append(body, 0) // The End TLV is included in count.
	require.LessOrEqual(t, len(body), 65535)
	return body
}

func TestProtocolCorpusCFMTLVCountBound(t *testing.T) {
	t.Run("65535-byte-message", func(t *testing.T) {
		const valueLength = 65535 - 74 - 3 - 1
		wire := bytes.Clone(cfmValidCCM(t)[:74])
		wire = append(wire, 250, byte(valueLength>>8), byte(valueLength&0xff))
		wire = append(wire, bytes.Repeat([]byte{0xa5}, valueLength)...)
		wire = append(wire, 0)
		require.Len(t, wire, 65535)
		node := cfmParse(t, wire, "CFM")
		protocolCorpusRequireValue(t, cfmFindTLV(t, node, 250), "Opaque Value", bytes.Repeat([]byte{0xa5}, valueLength))
	})
	t.Run("1024", func(t *testing.T) {
		valid := cfmTLVCountBody(t, 1024)
		node := cfmParse(t, valid, "CFM")
		tlvs := protocolCorpusNodesNamed(node, "TLV")
		require.Len(t, tlvs, 1024)
		protocolCorpusRequireValue(t, tlvs[0], "TLV Type", uint64(250))
		protocolCorpusRequireValue(t, tlvs[len(tlvs)-1], "TLV Type", uint64(0))
		require.Equal(t, [2]uint64{uint64(len(valid)-1) * 8, uint64(len(valid)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(tlvs[len(tlvs)-1], "TLV Type")))
	})
	t.Run("1025", func(t *testing.T) {
		overLimit := cfmTLVCountBody(t, 1025)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(overLimit), "cfm", "CFM")
		require.ErrorContains(t, err, "cfm: too many TLVs")
		carrier := cfmParse(t, overLimit, "CFMCarrier")
		protocolCorpusRequireValue(t, carrier, "Unparsed CFM Payload", overLimit)
		require.Nil(t, protocolCorpusFindNode(carrier, "Sequence Number"), "failed candidate must not remain in the committed carrier tree")
	})
}

func TestProtocolCorpusCFMStrictFieldsAndTLVFailures(t *testing.T) {
	body := cfmValidCCM(t)
	for opcode := 0; opcode <= 255; opcode++ {
		if opcode == 1 {
			continue
		}
		wire := bytes.Clone(body)
		wire[1] = byte(opcode)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "cfm", "CFM")
		require.ErrorContains(t, err, "unsupported CFM opcode", "opcode %d", opcode)
	}
	for _, mutation := range []struct {
		name    string
		offset  int
		value   byte
		message string
	}{
		{"opcode", 1, 2, "unsupported CFM opcode"},
		{"zero-interval", 2, 0xc0, "zero CCM interval"},
		{"small-first-tlv", 3, 69, "precedes the CCM fixed body"},
		{"large-first-tlv", 3, 104, "exceeds the message boundary"},
		{"mep-zero-high", 8, 0, "range 1..8191"},
		{"mep-zero-low", 9, 0, "range 1..8191"},
		{"md-format", 10, 0, "reserved MD name format"},
		{"md-zero-length", 11, 0, "invalid MD name length"},
		{"short-format", 16, 0, "reserved Short MA name format"},
		{"short-zero-length", 17, 0, "string Short MA name length"},
		{"maid-padding", 23, 1, "MAID padding"},
		{"port-length", 76, 0, "Port Status TLV length"},
		{"port-value", 77, 0, "Port Status value"},
		{"data-length", 79, 0xff, "TLV length exceeds"},
		{"interface-length", 86, 0, "Interface Status TLV length"},
		{"interface-value", 87, 0, "Interface Status value"},
		{"organization-length", 90, 3, "shorter than OUI"},
		{"missing-end", 102, 250, "truncated TLV length"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			wire := bytes.Clone(body)
			wire[mutation.offset] = mutation.value
			if mutation.name == "mep-zero-high" {
				wire[9] = 0
			}
			if mutation.name == "mep-zero-low" {
				wire[8] = 0
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "cfm", "CFM")
			require.ErrorContains(t, err, mutation.message)
			carrier := cfmParse(t, wire, "CFMCarrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed CFM Payload", wire)
			require.Nil(t, protocolCorpusFindNode(carrier, "Sequence Number"))
		})
	}

	badMACName := bytes.Clone(body)
	badMACName[10], badMACName[11] = 3, 4
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badMACName), "cfm", "CFM")
	require.ErrorContains(t, err, "MAC-based MD name must be eight")
	for _, invalid := range []struct {
		format  byte
		message string
	}{
		{1, "PVID Short MA name"},
		{3, "integer Short MA name"},
		{4, "VPN ID Short MA name"},
		{32, "ICC Short MA name"},
		{33, "ICC and CC Short MA name"},
	} {
		wire := bytes.Clone(body)
		wire[16] = invalid.format
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "cfm", "CFM")
		require.ErrorContains(t, err, invalid.message, "format %d", invalid.format)
	}

	shortTLVHeader := append(bytes.Clone(body[:102]), 250)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(shortTLVHeader), "cfm", "CFM")
	require.ErrorContains(t, err, "truncated TLV length")
}

func TestProtocolCorpusCFMNonzeroAndLengthVariants(t *testing.T) {
	body := cfmValidCCM(t)
	for interval := byte(1); interval <= 7; interval++ {
		wire := bytes.Clone(body)
		wire[0] = byte(interval << 5) // Every MD level, still version zero.
		wire[2] = interval            // RDI/Traffic zero, reserved zero.
		wire[4], wire[5], wire[6], wire[7] = interval, interval+1, interval+2, interval+3
		wire[77] = 1 + interval%2
		wire[87] = interval
		node := cfmParse(t, wire, "CFM")
		protocolCorpusRequireValue(t, node, "MD Level", uint64(interval))
		protocolCorpusRequireValue(t, node, "CCM Interval", uint64(interval))
		protocolCorpusRequireValue(t, node, "Sequence Number", uint64(binary.BigEndian.Uint32(wire[4:8])))
		protocolCorpusRequireValue(t, node, "Port Status", uint64(wire[77]))
		protocolCorpusRequireValue(t, node, "Interface Status", uint64(interval))
	}
	for version := byte(0); version <= 31; version++ {
		wire := bytes.Clone(body)
		wire[0] = 0xa0 | version // MD level 5 and every encoded version.
		node := cfmParse(t, wire, "CFM")
		protocolCorpusRequireValue(t, node, "MD Level", uint64(5))
		protocolCorpusRequireValue(t, node, "Version", uint64(version))
	}

	// Clause 11.2 receiver compatibility: a future version is processed using
	// known v0 fields, while undefined/reserved bits are exposed and ignored.
	receiverVariant := bytes.Clone(body)
	receiverVariant[0] = 0x7f // MD level 3, future version 31.
	receiverVariant[2] = 0xbd // RDI, all reserved bits, interval 5.
	receiverVariant[8] = 0xf2 // MEP reserved bits 7, identifier still 0x1234.
	copy(receiverVariant[70:74], []byte{1, 2, 3, 4})
	node := cfmParse(t, receiverVariant, "CFM")
	protocolCorpusRequireValue(t, node, "Version", uint64(31))
	protocolCorpusRequireValue(t, node, "Flags Reserved", uint64(7))
	protocolCorpusRequireValue(t, node, "MEP ID Reserved", uint64(7))
	protocolCorpusRequireValue(t, node, "MEP ID", uint64(0x1234))
	protocolCorpusRequireValue(t, node, "Y.1731 Reserved", []byte{1, 2, 3, 4})

	// A known TLV may be longer than the v0 definition on reception.  Decode
	// its known prefix and retain all extension octets explicitly.
	overlongPort := append(bytes.Clone(body[:78]), append([]byte{0xee}, body[78:]...)...)
	overlongPort[76] = 2
	node = cfmParse(t, overlongPort, "CFM")
	port := cfmFindTLV(t, node, 2)
	protocolCorpusRequireValue(t, port, "Port Status", uint64(2))
	protocolCorpusRequireValue(t, port, "TLV Extension", []byte{0xee})

	withExtension := append(bytes.Clone(body[:74]), append([]byte{0x9a, 0xbc}, body[74:]...)...)
	withExtension[3] = 72
	node = cfmParse(t, withExtension, "CFM")
	protocolCorpusRequireValue(t, node, "Header Extension", []byte{0x9a, 0xbc})
	cfmRequireSpan(t, node, "Header Extension", 74, 76)

	zeroUnknown := append(bytes.Clone(body[:102]), 250, 0, 0, 0)
	node = cfmParse(t, zeroUnknown, "CFM")
	zeroTLV := cfmFindTLV(t, node, 250)
	protocolCorpusRequireValue(t, zeroTLV, "TLV Length", uint64(2))
	require.Len(t, protocolCorpusNodesNamed(node, "TLV"), 7)
	zeroCandidates := 0
	for _, candidate := range protocolCorpusNodesNamed(node, "TLV") {
		length := protocolCorpusFindNode(candidate, "TLV Length")
		if length == nil {
			continue
		}
		value, err := length.Result()
		require.NoError(t, err)
		if uintVal(t, value) == 0 {
			zeroCandidates++
			require.Nil(t, protocolCorpusFindNode(candidate, "Opaque Value"))
		}
	}
	require.Equal(t, 1, zeroCandidates)

	withPadding := append(bytes.Clone(body), 0xa5, 0x5a, 0)
	node = cfmParse(t, withPadding, "CFM")
	protocolCorpusRequireValue(t, node, "Link Padding", []byte{0xa5, 0x5a, 0})
	cfmRequireSpan(t, node, "Link Padding", 103, 106)

	// MD-name format 1 omits its length.  Exercise the fixed-size PVID Short
	// MA format without relying on the string-name shape above.
	maidNone := make([]byte, 48)
	copy(maidNone, []byte{1, 1, 2, 0x01, 0x23})
	maidVariant := bytes.Clone(body)
	copy(maidVariant[10:58], maidNone)
	node = cfmParse(t, maidVariant, "CFM")
	protocolCorpusRequireValue(t, node, "MD Name Format", uint64(1))
	require.Nil(t, protocolCorpusFindNode(node, "MD Name Length"))
	protocolCorpusRequireValue(t, node, "Short MA Name Format", uint64(1))
	protocolCorpusRequireValue(t, node, "Short MA Name", []byte{1, 0x23})

	for _, shortName := range []struct {
		format byte
		value  []byte
	}{
		{3, []byte{0x12, 0x34}},
		{4, []byte{0x00, 0x1b, 0x21, 1, 2, 3, 4}},
		{32, []byte("ABCDEFGHIJKLM")},
		{33, []byte("USABCDEFGHIJKLM")},
	} {
		maid := make([]byte, 48)
		maid[0], maid[1], maid[2] = 1, shortName.format, byte(len(shortName.value))
		copy(maid[3:], shortName.value)
		wire := bytes.Clone(body)
		copy(wire[10:58], maid)
		node = cfmParse(t, wire, "CFM")
		protocolCorpusRequireValue(t, node, "Short MA Name Format", uint64(shortName.format))
		protocolCorpusRequireValue(t, node, "Short MA Name Length", uint64(len(shortName.value)))
		protocolCorpusRequireValue(t, node, "Short MA Name", shortName.value)
	}
}

func TestProtocolCorpusCFMCarrierTransactions(t *testing.T) {
	body := cfmValidCCM(t)
	invalid := bytes.Clone(body)
	invalid[102] = 250 // Fail after every complete fixed field and value TLV.
	for _, sample := range []struct {
		name    string
		payload []byte
		valid   bool
	}{
		{"complete", body, true},
		{"complete-with-padding", append(bytes.Clone(body), 0xa5, 0x5a), true},
		{"late-invalid", invalid, false},
		{"short", body[:73], false},
		{"exact-no-end", body[:102], true},
		{"truncated-final-value", body[:101], false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			root, err := base.ParseRule("cfm.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(sample.payload))*8)
			reader := bytes.NewReader(sample.payload)
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "CFMCarrier"))
			parsed := base.GetNodeByPath(root, "@CFMCarrier")
			require.NotNil(t, parsed)
			require.Equal(t, sample.payload, NodeToBytes(parsed))
			if sample.valid {
				protocolCorpusRequireValue(t, parsed, "MEP ID", uint64(0x1234))
				require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed CFM Payload"))
			} else {
				protocolCorpusRequireValue(t, parsed, "Unparsed CFM Payload", sample.payload)
				require.Nil(t, protocolCorpusFindNode(parsed, "MEP ID"))
			}
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

func TestProtocolCorpusCFMConcurrentIsolation(t *testing.T) {
	body := cfmValidCCM(t)
	for index := 0; index < 12; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			t.Parallel()
			invalid := bytes.Clone(body)
			invalid[23+index%35] = byte(index + 1)
			carrier := cfmParse(t, invalid, "CFMCarrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed CFM Payload", invalid)
			valid := cfmParse(t, body, "CFM")
			cfmRequireCCMFields(t, valid, 0)
		})
	}
}
