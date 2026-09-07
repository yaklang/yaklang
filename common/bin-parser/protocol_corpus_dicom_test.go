package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const dicomRule = "application-layer.dicom"

// Source: nDPI tests/cfgs/default/pcap/dicom.pcap, pinned by sources.json.
// Six NULL/IP/TCP records carry four association requests, two spanning records.
// Reassembly below checks sequence numbers and uses every record, not merely
// the representative first frame. It is a fixture loader, not another UL parser.
func dicomCapturedMessages(t *testing.T) [][]byte {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-dicom.pcap")
	require.Len(t, frames, 6)
	var messages [][]byte
	for _, group := range [][]int{{0}, {1, 2}, {3, 4}, {5}} {
		var wire []byte
		var next uint32
		var port uint16
		for index, record := range group {
			frame := frames[record]
			require.Equal(t, uint32(2), binary.LittleEndian.Uint32(frame[:4]))
			ip := frame[4:]
			require.Equal(t, byte(4), ip[0]>>4)
			require.Equal(t, byte(6), ip[9])
			require.Equal(t, len(ip), int(binary.BigEndian.Uint16(ip[2:4])))
			tcp := ip[int(ip[0]&15)*4:]
			require.Equal(t, uint16(104), binary.BigEndian.Uint16(tcp[2:4]))
			sequence := binary.BigEndian.Uint32(tcp[4:8])
			if index > 0 {
				require.Equal(t, next, sequence, "missing or overlapping TCP bytes")
				require.Equal(t, port, binary.BigEndian.Uint16(tcp[:2]))
			}
			port = binary.BigEndian.Uint16(tcp[:2])
			payload := tcp[int(tcp[12]>>4)*4:]
			require.Equal(t, 56, len(frame)-len(payload))
			next = sequence + uint32(len(payload))
			wire = append(wire, payload...)
		}
		messages = append(messages, wire)
	}
	require.Equal(t, []int{683, 16509, 16509, 683}, []int{len(messages[0]), len(messages[1]), len(messages[2]), len(messages[3])})
	require.Equal(t, messages[1], messages[2], "the two large requests repeat the same UL fields")
	return messages
}

func dicomRequireParse(t *testing.T, wire []byte) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, dicomRule, "DICOM")
}

func dicomUIDValues(t *testing.T, node *base.Node, name string) []string {
	t.Helper()
	var result []string
	for _, item := range protocolCorpusNodesNamed(node, name) {
		uid := protocolCorpusFindNode(item, "UID")
		require.NotNil(t, uid)
		value, err := uid.Result()
		require.NoError(t, err)
		result = append(result, strVal(t, value))
	}
	return result
}

func TestProtocolCorpusDICOMEveryRecordAndReassembledMessage(t *testing.T) {
	for index, wire := range dicomCapturedMessages(t) {
		t.Run(fmt.Sprintf("message-%d", index+1), func(t *testing.T) {
			n := dicomRequireParse(t, wire)
			protocolCorpusRequireValue(t, n, "PDU Type", uint64(1))
			protocolCorpusRequireValue(t, n, "PDU Length", uint64(len(wire)-6))
			protocolCorpusRequireValue(t, n, "Protocol Version", uint64(1))
			called, calling := "testserver      ", "testclient      "
			if index == 3 {
				called, calling = "bogus_remote    ", "bogus_local     "
			}
			protocolCorpusRequireValue(t, n, "Called AE Title", called)
			protocolCorpusRequireValue(t, n, "Calling AE Title", calling)
			require.Equal(t, []string{"1.2.840.10008.3.1.1.1"}, dicomUIDValues(t, n, "Application Context"))
			require.Equal(t, []string{"1.2.826.0.1.3680043.9.7133.1.1"}, dicomUIDValues(t, n, "Implementation Class"))
			protocolCorpusRequireValue(t, n, "Maximum PDU Length", uint64(4194304))
			protocolCorpusRequireValue(t, n, "Implementation Version", "GODICOM_1_1")
			contexts := protocolCorpusNodesNamed(n, "Presentation Context")
			wantCount := 4
			if len(wire) > 1000 {
				wantCount = 123
			}
			require.Len(t, contexts, wantCount)
			for i, context := range contexts {
				protocolCorpusRequireValue(t, context, "Context ID", uint64(1+2*i))
				require.Equal(t, []string{"1.2.840.10008.1.2", "1.2.840.10008.1.2.1", "1.2.840.10008.1.2.2", "1.2.840.10008.1.2.1.99"}, dicomUIDValues(t, context, "Transfer Syntax"))
			}
			abstracts := dicomUIDValues(t, n, "Abstract Syntax")
			require.Len(t, abstracts, wantCount)
			if wantCount == 4 {
				require.Equal(t, []string{"1.2.840.10008.5.1.4.1.2.1.1", "1.2.840.10008.5.1.4.1.2.2.1", "1.2.840.10008.5.1.4.1.2.3.1", "1.2.840.10008.5.1.4.31"}, abstracts)
			} else {
				// Independent oracle: tshark dicom.pctx.abss.syntax from frame 3,
				// numeric UIDs in wire order, one per line including the final LF.
				// Hash pins all 123 values, not only the first/last contexts.
				digest := sha256.Sum256([]byte(strings.Join(abstracts, "\n") + "\n"))
				require.Equal(t, "3fd41814a9b0ff13b38ff021d49982f5880cc5c0a3c9db5578fcb20f22308a8e", fmt.Sprintf("%x", digest))
			}
			require.Empty(t, protocolCorpusNodesNamed(n, "Extension Data"), "all captured association fields must be parsed, not extension blobs")
		})
	}
}

func dicomTestItem(kind byte, value []byte) []byte {
	item := []byte{kind, 0, 0, 0}
	binary.BigEndian.PutUint16(item[2:], uint16(len(value)))
	return append(item, value...)
}

func dicomTestPDU(kind byte, value []byte) []byte {
	pdu := []byte{kind, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(pdu[2:], uint32(len(value)))
	return append(pdu, value...)
}

// These branch companions follow PS3.8 9.3.2–9.3.8, D.1, E.2 and F.1;
// they are generated tests, not represented as captured or standard example hex.
func dicomTestAssociation(kind byte, contexts, user []byte) []byte {
	body := make([]byte, 68)
	body[1] = 1
	copy(body[4:20], "DESTINATION     ")
	copy(body[20:36], "SOURCE          ")
	body = append(body, dicomTestItem(0x10, []byte("1.2.840.10008.3.1.1.1"))...)
	body = append(body, contexts...)
	body = append(body, dicomTestItem(0x50, user)...)
	return dicomTestPDU(kind, body)
}

func dicomTestUser() []byte {
	user := dicomTestItem(0x51, []byte{0, 1, 0, 0})
	user = append(user, dicomTestItem(0x52, []byte("1.2.826.0.1.3680043.10.1"))...)
	return user
}

func dicomTestContext(kind, id, result byte) []byte {
	value := []byte{id, 0, result, 0}
	if kind == 0x20 {
		value = append(value, dicomTestItem(0x30, []byte("1.2.840.10008.1.1"))...)
	}
	value = append(value, dicomTestItem(0x40, []byte("1.2.840.10008.1.2"))...)
	return dicomTestItem(kind, value)
}

func dicomTestPDV(id, control byte, fragment []byte) []byte {
	value := []byte{0, 0, 0, 0, id, control}
	binary.BigEndian.PutUint32(value, uint32(len(fragment)+2))
	return append(value, fragment...)
}

func TestProtocolCorpusDICOMCorePDUsAndStreamBoundaries(t *testing.T) {
	request := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), dicomTestUser())
	accept := dicomTestAssociation(2, dicomTestContext(0x21, 1, 0), dicomTestUser())
	pdv := append(dicomTestPDV(3, 3, []byte{8, 0, 16, 0}), dicomTestPDV(3, 2, []byte{0x10, 0, 0x20, 0})...)
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"request", request}, {"accept", accept}, {"reject", dicomTestPDU(3, []byte{0, 2, 3, 2})},
		{"data", dicomTestPDU(4, pdv)}, {"release-request", dicomTestPDU(5, []byte{1, 2, 3, 4})},
		{"release-response", dicomTestPDU(6, []byte{5, 6, 7, 8})}, {"abort", dicomTestPDU(7, []byte{0, 0, 2, 6})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := dicomRequireParse(t, tc.wire)
			protocolCorpusRequireValue(t, n, "PDU Type", uint64(tc.wire[0]))
			switch tc.wire[0] {
			case 1, 2:
				protocolCorpusRequireValue(t, n, "Context ID", uint64(1))
				require.Equal(t, []string{"1.2.840.10008.1.2"}, dicomUIDValues(t, n, "Transfer Syntax"))
				protocolCorpusRequireValue(t, n, "Maximum PDU Length", uint64(65536))
			case 3:
				protocolCorpusRequireValue(t, n, "Result", uint64(2))
				protocolCorpusRequireValue(t, n, "Source", uint64(3))
				protocolCorpusRequireValue(t, n, "Reason", uint64(2))
			case 4:
				items := protocolCorpusNodesNamed(n, "PDV")
				require.Len(t, items, 2)
				for i, item := range items {
					protocolCorpusRequireValue(t, item, "Context ID", uint64(3))
					protocolCorpusRequireValue(t, item, "Last Fragment", uint64(1))
					protocolCorpusRequireValue(t, item, "Command Fragment", uint64(1-i))
				}
				protocolCorpusRequireValue(t, items[0], "Message Fragment", []byte{8, 0, 16, 0})
				protocolCorpusRequireValue(t, items[1], "Message Fragment", []byte{0x10, 0, 0x20, 0})
			case 5, 6:
				protocolCorpusRequireValue(t, n, "Release Reserved", tc.wire[6:])
			case 7:
				protocolCorpusRequireValue(t, n, "Source", uint64(2))
				protocolCorpusRequireValue(t, n, "Reason", uint64(6))
			}
			stream := bytes.NewReader(append(bytes.Clone(tc.wire), 0xde, 0xad, 0xbe, 0xef))
			_, err := parser.ParseBinary(stream, dicomRule, "DICOM")
			require.NoError(t, err)
			require.Equal(t, 4, stream.Len(), "one UL PDU must not consume the next PDU")
			for cut := 0; cut < len(tc.wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire[:cut]), dicomRule, "DICOM")
				require.Error(t, err, "bounded prefix %d", cut)
				_, err = parser.ParseBinary(bytes.NewReader(tc.wire[:cut]), dicomRule, "DICOM")
				require.Error(t, err, "stream prefix %d", cut)
			}
		})
	}
}

func TestProtocolCorpusDICOMCapturedEveryShortPrefix(t *testing.T) {
	for index, wire := range dicomCapturedMessages(t) {
		index, wire := index, wire
		t.Run(fmt.Sprintf("message-%d", index+1), func(t *testing.T) {
			t.Parallel()
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), dicomRule, "DICOM")
				require.Error(t, err, "capture prefix %d/%d", cut, len(wire))
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0)), dicomRule, "DICOM")
			require.ErrorContains(t, err, "PDU length differs from message boundary")
		})
	}
}

func TestProtocolCorpusDICOMPublicTCPDispatchAndFragmentFallback(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-dicom.pcap")
	for index, frame := range frames {
		t.Run(fmt.Sprintf("record-%d", index+1), func(t *testing.T) {
			// Preserve the complete captured IP/TCP record after only the NULL
			// link-type header. Reassembly is deliberately not invented here.
			n := protocolCorpusRequireBoundedRuleParse(t, frame[4:], "internet_protocol", "Internet Protocol")
			if index == 0 || index == 5 {
				dicom := protocolCorpusFindNode(n, "DICOM")
				require.NotNil(t, dicom)
				protocolCorpusRequireValue(t, dicom, "PDU Length", uint64(677))
				require.Equal(t, []string{"1.2.840.10008.3.1.1.1"}, dicomUIDValues(t, dicom, "Application Context"))
				protocolCorpusRequireValue(t, dicom, "Implementation Version", "GODICOM_1_1")
				require.Nil(t, protocolCorpusFindNode(n, "Remaining Payload"))
			} else {
				require.Nil(t, protocolCorpusFindNode(n, "DICOM"))
				protocolCorpusRequireValue(t, n, "Remaining Payload", frame[56:])
			}
		})
	}
}

func TestProtocolCorpusDICOMNestedLengthValidation(t *testing.T) {
	wire := dicomCapturedMessages(t)[0]
	var offsets []int
	// Locate every 16-bit nested length in this independently known RQ layout.
	// This only selects mutation offsets; all validation uses the delivered rule.
	var lengths func(int, int, bool)
	lengths = func(start, end int, nested bool) {
		for cursor := start; cursor < end; {
			size := int(binary.BigEndian.Uint16(wire[cursor+2 : cursor+4]))
			offsets = append(offsets, cursor+2)
			if nested {
				switch wire[cursor] {
				case 0x20:
					lengths(cursor+8, cursor+4+size, false)
				case 0x50:
					lengths(cursor+4, cursor+4+size, false)
				}
			}
			cursor += 4 + size
		}
	}
	lengths(74, len(wire), true)
	require.Len(t, offsets, 29)
	for _, offset := range offsets {
		for _, delta := range []uint16{1, 65535} {
			t.Run(fmt.Sprintf("offset-%d-delta-%d", offset, delta), func(t *testing.T) {
				bad := bytes.Clone(wire)
				binary.BigEndian.PutUint16(bad[offset:], binary.BigEndian.Uint16(bad[offset:])+delta)
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), dicomRule, "DICOM")
				require.Error(t, err)
			})
		}
	}
}

func TestProtocolCorpusDICOMReservedAndExtensionVariants(t *testing.T) {
	request := dicomTestAssociation(1, dicomTestContext(0x20, 255, 0xa5), dicomTestUser())
	request[1], request[6], request[7] = 0xff, 0x81, 1 // reserved PDU and extra supported-version bits
	request[8], request[9], request[42], request[75] = 0xaa, 0xbb, 0xcc, 0xdd
	n := dicomRequireParse(t, request)
	protocolCorpusRequireValue(t, n, "Protocol Version", uint64(0x8101))
	protocolCorpusRequireValue(t, n, "Context ID", uint64(255))
	protocolCorpusRequireValue(t, n, "Result Reason", uint64(0xa5)) // reserved in RQ
	protocolCorpusRequireValue(t, n, "Protocol Reserved", []byte{0xaa, 0xbb})

	// User sub-items may be unordered, Maximum Length zero is explicitly valid,
	// and future user/association extensions must be retained without interpretation.
	user := dicomTestItem(0x55, []byte("VERSION_2"))
	user = append(user, dicomTestItem(0x52, []byte("1.2.3.4.5"))...)
	user = append(user, dicomTestItem(0xee, []byte("https://example.invalid/opaque"))...)
	user = append(user, dicomTestItem(0x51, []byte{0, 0, 0, 0})...)
	contexts := append(dicomTestContext(0x20, 1, 0), dicomTestItem(0x45, []byte{9, 8, 7})...)
	n = dicomRequireParse(t, dicomTestAssociation(1, contexts, user))
	protocolCorpusRequireValue(t, n, "Maximum PDU Length", uint64(0))
	protocolCorpusRequireValue(t, n, "Implementation Version", "VERSION_2")
	extensions := protocolCorpusNodesNamed(n, "Extension Data")
	require.Len(t, extensions, 2)
	protocolCorpusRequireValue(t, extensions[0], "Extension Data", []byte{9, 8, 7})
	protocolCorpusRequireValue(t, extensions[1], "Extension Data", []byte("https://example.invalid/opaque"))

	// Rejected AC contexts do not validate insignificant transfer-syntax content.
	for _, result := range []byte{1, 2, 3, 4} {
		context := dicomTestItem(0x21, []byte{3, 0x80, result, 0xff, 0xde, 0xad})
		accept := dicomTestAssociation(2, context, dicomTestUser())
		for i := 10; i < 42; i++ {
			accept[i] = 0 // AC titles are reserved, not RQ AE-title constraints
		}
		n = dicomRequireParse(t, accept)
		protocolCorpusRequireValue(t, n, "Result Reason", uint64(result))
		protocolCorpusRequireValue(t, n, "Rejected Transfer Syntax", []byte{0xde, 0xad})
	}
	for _, control := range []byte{0, 1, 2, 3, 0xff} {
		n = dicomRequireParse(t, dicomTestPDU(4, dicomTestPDV(1, control, nil)))
		protocolCorpusRequireValue(t, n, "Last Fragment", uint64(control>>1&1))
		protocolCorpusRequireValue(t, n, "Command Fragment", uint64(control&1))
		protocolCorpusRequireValue(t, n, "Control Reserved", uint64(control>>2))
	}
	n = dicomRequireParse(t, dicomTestPDU(7, []byte{0xff, 0xee, 0, 255}))
	protocolCorpusRequireValue(t, n, "Reason", uint64(255)) // user-initiated reason is insignificant
}

func TestProtocolCorpusDICOMInvalidValuesAndRequiredItems(t *testing.T) {
	request := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), dicomTestUser())
	badByte := func(offset int, value byte) []byte {
		bad := bytes.Clone(request)
		bad[offset] = value
		return bad
	}
	badUID := func(uid string) []byte {
		body := append(bytes.Clone(request[6:74]), dicomTestItem(0x10, []byte(uid))...)
		body = append(body, request[99:]...)
		return dicomTestPDU(1, body)
	}
	blankTitle := bytes.Clone(request)
	copy(blankTitle[10:26], strings.Repeat(" ", 16))
	missingApplication := dicomTestPDU(1, append(bytes.Clone(request[6:74]), request[99:]...))
	duplicateApplication := dicomTestPDU(1, append(append(bytes.Clone(request[6:99]), request[74:99]...), request[99:]...))
	duplicateMaximum := append(dicomTestUser(), dicomTestItem(0x51, []byte{0, 0, 0, 0})...)
	badMaximum := append(dicomTestItem(0x51, []byte{0, 0, 0}), dicomTestItem(0x52, []byte("1.2.3"))...)
	wrongSyntaxOrder := append([]byte{1, 0, 0, 0}, dicomTestItem(0x40, []byte("1.2.3"))...)
	wrongSyntaxOrder = append(wrongSyntaxOrder, dicomTestItem(0x30, []byte("1.2.4"))...)
	for _, tc := range []struct {
		name, reason string
		wire         []byte
	}{
		{"pdu-type-zero", "unsupported PDU type", badByte(0, 0)},
		{"pdu-type-eight", "unsupported PDU type", badByte(0, 8)},
		{"version", "protocol version one", badByte(7, 2)},
		{"AE-character", "invalid AE title", badByte(10, 0)},
		{"blank-AE", "empty AE title", blankTitle},
		{"missing-application", "required association items", missingApplication},
		{"duplicate-application", "required association items", duplicateApplication},
		{"wrong-context-direction", "differs from association direction", badByte(99, 0x21)},
		{"even-context", "ID must be odd", badByte(103, 2)},
		{"no-context", "required association items", dicomTestAssociation(1, nil, dicomTestUser())},
		{"no-user-fields", "required user information", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), nil)},
		{"duplicate-maximum", "required user information", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), duplicateMaximum)},
		{"maximum-length", "must contain four bytes", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), badMaximum)},
		{"long-version", "implementation version length", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), append(dicomTestUser(), dicomTestItem(0x55, []byte(strings.Repeat("V", 17)))...))},
		{"non-ASCII-version", "invalid implementation version character", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), append(dicomTestUser(), dicomTestItem(0x55, []byte{0x80})...))},
		{"control-version", "invalid implementation version character", dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), append(dicomTestUser(), dicomTestItem(0x55, []byte{'V', 0})...))},
		{"duplicate-context", "duplicate presentation context ID", dicomTestAssociation(1, append(dicomTestContext(0x20, 1, 0), dicomTestContext(0x20, 1, 0)...), dicomTestUser())},
		{"missing-transfer", "requires abstract and transfer", dicomTestAssociation(1, dicomTestItem(0x20, append([]byte{1, 0, 0, 0}, dicomTestItem(0x30, []byte("1.2.3"))...)), dicomTestUser())},
		{"missing-abstract", "requires abstract and transfer", dicomTestAssociation(1, dicomTestItem(0x20, append([]byte{1, 0, 0, 0}, dicomTestItem(0x40, []byte("1.2.3"))...)), dicomTestUser())},
		{"syntax-order", "syntax item types out of order", dicomTestAssociation(1, dicomTestItem(0x20, wrongSyntaxOrder), dicomTestUser())},
		{"accepted-context-missing-transfer", "requires one transfer syntax", dicomTestAssociation(2, dicomTestItem(0x21, []byte{1, 0, 0, 0}), dicomTestUser())},
		{"accepted-context-result", "invalid presentation context result", dicomTestAssociation(2, dicomTestContext(0x21, 1, 5), dicomTestUser())},
		{"empty-uid", "UID length", badUID("")},
		{"long-uid", "UID length", badUID(strings.Repeat("1", 65))},
		{"invalid-uid-character", "invalid UID character", badUID("1.2x3")},
		{"uid-empty-component", "empty UID component", badUID("1..2")},
		{"uid-trailing-dot", "empty UID component", badUID("1.2.")},
		{"uid-leading-zero", "leading zero", badUID("1.02.3")},
		{"empty-PDATA", "missing presentation data", dicomTestPDU(4, nil)},
		{"invalid-PDV-length", "invalid PDV length", dicomTestPDU(4, []byte{0, 0, 0, 1, 1, 0})},
		{"odd-fragment-length", "even byte length", dicomTestPDU(4, dicomTestPDV(1, 2, []byte{1}))},
		{"even-PDV-context", "ID must be odd", dicomTestPDU(4, dicomTestPDV(2, 2, nil))},
		{"different-PDV-context", "same context", dicomTestPDU(4, append(dicomTestPDV(1, 3, nil), dicomTestPDV(3, 2, nil)...))},
		{"reject-result", "rejection result", dicomTestPDU(3, []byte{0, 0, 1, 1})},
		{"reject-source", "rejection source", dicomTestPDU(3, []byte{0, 1, 0, 1})},
		{"reject-user-reason", "service-user rejection reason", dicomTestPDU(3, []byte{0, 1, 1, 6})},
		{"reject-provider-reason", "provider rejection reason", dicomTestPDU(3, []byte{0, 1, 3, 0})},
		{"abort-source", "abort source", dicomTestPDU(7, []byte{0, 0, 1, 0})},
		{"abort-reason", "abort reason", dicomTestPDU(7, []byte{0, 0, 2, 3})},
		{"release-length", "control PDU length", dicomTestPDU(5, []byte{0, 0, 0})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire), dicomRule, "DICOM")
			require.ErrorContains(t, err, "dicom: ")
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestProtocolCorpusDICOMResourceLimits(t *testing.T) {
	for _, tc := range []struct {
		kind   byte
		length uint32
		reason string
	}{{4, 16777217, "16 MiB"}, {1, 1048577, "1 MiB"}, {1, 67, "fixed fields truncated"}} {
		header := dicomTestPDU(tc.kind, nil)
		binary.BigEndian.PutUint32(header[2:], tc.length)
		// Open reader: reject the announced resource demand before reading a body.
		_, err := parser.ParseBinary(bytes.NewReader(header), dicomRule, "DICOM")
		require.ErrorContains(t, err, tc.reason)
	}
	for _, count := range []int{4096, 4097} {
		t.Run(fmt.Sprintf("PDVs-%d", count), func(t *testing.T) {
			wire := dicomTestPDU(4, bytes.Repeat(dicomTestPDV(1, 0, nil), count))
			if count == 4096 {
				n := dicomRequireParse(t, wire)
				require.Len(t, protocolCorpusNodesNamed(n, "PDV"), count)
			} else {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), dicomRule, "DICOM")
				require.ErrorContains(t, err, "PDV count exceeds 4096")
			}
		})
		t.Run(fmt.Sprintf("items-%d", count), func(t *testing.T) {
			// Baseline: application + context + abstract + transfer + user-info +
			// maximum + implementation = seven items, all included in the limit.
			user := append(dicomTestUser(), bytes.Repeat(dicomTestItem(0xee, nil), count-7)...)
			wire := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), user)
			if count == 4096 {
				n := dicomRequireParse(t, wire)
				require.Len(t, protocolCorpusNodesNamed(n, "User Item"), count-5)
			} else {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), dicomRule, "DICOM")
				require.ErrorContains(t, err, "item count exceeds 4096")
			}
		})
	}
}

func TestProtocolCorpusDICOMParallelRuntimeIsolation(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
			t.Parallel()
			for iteration := 0; iteration < 8; iteration++ {
				kind := byte(1 + iteration%2)
				contextType := byte(0x20 + iteration%2)
				id := byte(worker*2 + 1)
				wire := dicomTestAssociation(kind, dicomTestContext(contextType, id, 0), dicomTestUser())
				called := fmt.Sprintf("NODE_%02d_%02d      ", worker, iteration)
				copy(wire[10:26], called)
				n := dicomRequireParse(t, wire)
				protocolCorpusRequireValue(t, n, "PDU Type", uint64(kind))
				protocolCorpusRequireValue(t, n, "Context ID", uint64(id))
				protocolCorpusRequireValue(t, n, "Called AE Title", called)
				abstracts := dicomUIDValues(t, n, "Abstract Syntax")
				if kind == 1 {
					require.Equal(t, []string{"1.2.840.10008.1.1"}, abstracts)
				} else {
					require.Empty(t, abstracts)
				}
				// An intervening rejected parse cannot change the next root's
				// PDU direction, counters, or retained per-input field values.
				wire[7] = 0
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), dicomRule, "DICOM")
				require.ErrorContains(t, err, "protocol version one")
			}
		})
	}
}
