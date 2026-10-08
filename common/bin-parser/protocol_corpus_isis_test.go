package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type isisCoreVector struct {
	name string
	hex  string
}

func isisCoreVectors() []isisCoreVector {
	// These fixed-header vectors are accepted without expert diagnostics by
	// Wireshark.  They exercise every ISO 10589 base PDU type supported here.
	return []isisCoreVector{
		{name: "L1 LAN IIH", hex: "831b01000f01000001010001000100001e001b0101000100010001"},
		{name: "L2 LAN IIH", hex: "831b01001001000002010001000100001e001b0101000100010001"},
		{name: "P2P IIH", hex: "831401001101000003010001000100001e001401"},
		{name: "L1 LSP", hex: "831b010012010000001b04b0010001000100000000000001dd1d01"},
		{name: "L2 LSP", hex: "831b010014010000001b04b0010001000100000000000001df1903"},
		{name: "L1 CSNP", hex: "83210100180100000021010001000100000000000000000000ffffffffffffffff"},
		{name: "L2 CSNP", hex: "83210100190100000021010001000100000000000000000000ffffffffffffffff"},
		{name: "L1 PSNP", hex: "831101001a010000001101000100010000"},
		{name: "L2 PSNP", hex: "831101001b010000001101000100010000"},
	}
}

func isisParse(t *testing.T, wire []byte) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(wire)
	node, err := parser.ParseBinary(reader, "isis", "ISIS")
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Zero(t, reader.Len(), "IS-IS parser left bytes unread")
	return node
}

func isisRequireCommonFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	require.GreaterOrEqual(t, len(wire), 8)
	protocolCorpusRequireValue(t, node, "Intradomain Routing Protocol Discriminator", uint64(wire[0]))
	protocolCorpusRequireValue(t, node, "Length Indicator", uint64(wire[1]))
	protocolCorpusRequireValue(t, node, "Version Protocol ID Extension", uint64(wire[2]))
	protocolCorpusRequireValue(t, node, "ID Length", uint64(wire[3]))
	protocolCorpusRequireValue(t, node, "Reserved Type Bits", uint64(wire[4]>>5))
	protocolCorpusRequireValue(t, node, "PDU Type", uint64(wire[4]&0x1f))
	protocolCorpusRequireValue(t, node, "Version", uint64(wire[5]))
	protocolCorpusRequireValue(t, node, "Reserved", uint64(wire[6]))
	protocolCorpusRequireValue(t, node, "Maximum Area Addresses", uint64(wire[7]))
}

func isisTLV(t *testing.T, typ byte, value ...byte) []byte {
	t.Helper()
	require.LessOrEqual(t, len(value), 255)
	return append([]byte{typ, byte(len(value))}, value...)
}

func isisAppendTLVBytes(t *testing.T, fixed []byte, pduLengthOffset int, fields ...[]byte) []byte {
	t.Helper()
	wire := bytes.Clone(fixed)
	for _, field := range fields {
		wire = append(wire, field...)
	}
	require.LessOrEqual(t, len(wire), 65535)
	require.LessOrEqual(t, pduLengthOffset+2, len(wire))
	binary.BigEndian.PutUint16(wire[pduLengthOffset:pduLengthOffset+2], uint16(len(wire)))
	return wire
}

func isisLANHelloCompanion(t *testing.T) []byte {
	t.Helper()
	fixed := mustHex(t, isisCoreVectors()[0].hex)
	return isisAppendTLVBytes(t, fixed, 17,
		isisTLV(t, 1, 3, 0x49, 0x00, 0x01),
		isisTLV(t, 129, 0x81, 0xcc, 0x8e),
		isisTLV(t, 132, 192, 0, 2, 1, 198, 51, 100, 2),
		isisTLV(t, 250, 0xaa, 0xbb, 0xcc),
	)
}

func isisFindTLV(t *testing.T, node *base.Node, typ byte) *base.Node {
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
	t.Fatalf("missing decoded IS-IS TLV type %d", typ)
	return nil
}

func isisFletcherSums(input []byte) (int, int) {
	c0, c1 := 0, 0
	for _, octet := range input {
		c0 = (c0 + int(octet)) % 255
		c1 = (c1 + c0) % 255
	}
	return c0, c1
}

func isisSetLSPChecksum(t *testing.T, pdu []byte) {
	t.Helper()
	require.GreaterOrEqual(t, len(pdu), 27)
	// ISO 10589 excludes the common header, PDU length, and remaining
	// lifetime.  The checksum region begins at the LSP ID (offset 12), and
	// the checksum field begins twelve octets into that region.
	pdu[24], pdu[25] = 0, 0
	region := pdu[12:]
	c0, c1 := isisFletcherSums(region)
	x := ((len(region)-12-1)*c0 - c1) % 255
	if x <= 0 {
		x += 255
	}
	y := 510 - c0 - x
	if y > 255 {
		y -= 255
	}
	pdu[24], pdu[25] = byte(x), byte(y)
	c0, c1 = isisFletcherSums(region)
	require.Zero(t, c0, "generated LSP checksum C0")
	require.Zero(t, c1, "generated LSP checksum C1")
}

func isisDot3Frame(pdu []byte, control byte) []byte {
	llc := append([]byte{0xfe, 0xfe, control}, pdu...)
	frame := []byte{
		0x01, 0x80, 0xc2, 0x00, 0x00, 0x14,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0, 0,
	}
	binary.BigEndian.PutUint16(frame[12:14], uint16(len(llc)))
	return append(frame, llc...)
}

func isisRequireLANHelloCompanionFields(t *testing.T, node *base.Node, wire []byte) {
	t.Helper()
	isisRequireCommonFields(t, node, wire)
	protocolCorpusRequireValue(t, node, "Circuit Type", uint64(1))
	protocolCorpusRequireValue(t, node, "Reserved Circuit Bits", uint64(0))
	protocolCorpusRequireValue(t, node, "Source System ID", mustHex(t, "010001000100"))
	protocolCorpusRequireValue(t, node, "Holding Timer", uint64(30))
	protocolCorpusRequireValue(t, node, "PDU Length", uint64(len(wire)))
	protocolCorpusRequireValue(t, node, "Reserved Priority Bit", uint64(0))
	protocolCorpusRequireValue(t, node, "Priority", uint64(1))
	protocolCorpusRequireValue(t, node, "LAN ID", mustHex(t, "01000100010001"))
	require.Len(t, protocolCorpusNodesNamed(node, "TLV"), 4)

	area := isisFindTLV(t, node, 1)
	protocolCorpusRequireValue(t, area, "TLV Type", uint64(1))
	protocolCorpusRequireValue(t, area, "TLV Length", uint64(4))
	protocolCorpusRequireValue(t, area, "Address Length", uint64(3))
	protocolCorpusRequireValue(t, area, "Address", mustHex(t, "490001"))

	protocols := isisFindTLV(t, node, 129)
	protocolCorpusRequireValue(t, protocols, "TLV Type", uint64(129))
	protocolCorpusRequireValue(t, protocols, "TLV Length", uint64(3))
	nlpids := protocolCorpusNodesNamed(protocols, "NLPID")
	require.Len(t, nlpids, 3)
	for index, expected := range []uint64{0x81, 0xcc, 0x8e} {
		protocolCorpusRequireValue(t, nlpids[index], "NLPID", expected)
	}

	ipv4 := isisFindTLV(t, node, 132)
	protocolCorpusRequireValue(t, ipv4, "TLV Type", uint64(132))
	protocolCorpusRequireValue(t, ipv4, "TLV Length", uint64(8))
	addresses := protocolCorpusNodesNamed(ipv4, "IPv4 Address")
	require.Len(t, addresses, 2)
	protocolCorpusRequireValue(t, addresses[0], "IPv4 Address", []byte{192, 0, 2, 1})
	protocolCorpusRequireValue(t, addresses[1], "IPv4 Address", []byte{198, 51, 100, 2})

	unknown := isisFindTLV(t, node, 250)
	protocolCorpusRequireValue(t, unknown, "TLV Type", uint64(250))
	protocolCorpusRequireValue(t, unknown, "TLV Length", uint64(3))
	protocolCorpusRequireValue(t, unknown, "Unknown Value", []byte{0xaa, 0xbb, 0xcc})
}

func TestProtocolCorpusISISExistingCaptureEveryRecordAndField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-isis.pcap")
	require.Len(t, frames, 1)
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			require.Len(t, frame, 44)
			require.Equal(t, uint16(30), binary.BigEndian.Uint16(frame[12:14]))
			require.Equal(t, []byte{0xfe, 0xfe, 3}, frame[14:17])
			wire := frame[17:]

			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, envelope, "Destination", frame[:6])
			protocolCorpusRequireValue(t, envelope, "Source", frame[6:12])
			protocolCorpusRequireValue(t, envelope, "Type", uint64(30))
			require.Equal(t, 3, protocolCorpusLLCFields(t, envelope, frame[14:]))
			envelopeISIS := protocolCorpusFindNode(envelope, "ISIS")
			require.NotNil(t, envelopeISIS, "FE/FE UI LLC did not dispatch to IS-IS")
			isisRequireCommonFields(t, envelopeISIS, wire)

			node := isisParse(t, wire)
			isisRequireCommonFields(t, node, wire)
			protocolCorpusRequireValue(t, node, "Circuit Type", uint64(1))
			protocolCorpusRequireValue(t, node, "Reserved Circuit Bits", uint64(0))
			protocolCorpusRequireValue(t, node, "Source System ID", mustHex(t, "010001000100"))
			protocolCorpusRequireValue(t, node, "Holding Timer", uint64(30))
			protocolCorpusRequireValue(t, node, "PDU Length", uint64(27))
			protocolCorpusRequireValue(t, node, "Reserved Priority Bit", uint64(0))
			protocolCorpusRequireValue(t, node, "Priority", uint64(1))
			protocolCorpusRequireValue(t, node, "LAN ID", mustHex(t, "01000100010001"))
			require.Nil(t, protocolCorpusFindNode(node, "TLVs"))
			require.Nil(t, protocolCorpusFindNode(node, "Area Addresses"))

			// This original record is a complete no-TLV syntax edge, not proof
			// that a sender supplied the Area Address needed for a LAN adjacency.
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "isis", "ISIS")
				require.Errorf(t, err, "IS-IS accepted capture prefix %d/%d", cut, len(wire))
			}
		})
	}
}

func TestProtocolCorpusISISCompanionEveryRecordAndField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-isis-valid.pcap")
	require.Len(t, frames, 1)
	wire := isisLANHelloCompanion(t)
	expectedFrame := isisDot3Frame(wire, 3)
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			require.Equal(t, expectedFrame, frame, "generated IS-IS companion bytes changed")
			require.Len(t, frame, 70)
			require.Equal(t, uint16(56), binary.BigEndian.Uint16(frame[12:14]))
			require.Equal(t, []byte{0xfe, 0xfe, 3}, frame[14:17])

			envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, envelope, "Destination", frame[:6])
			protocolCorpusRequireValue(t, envelope, "Source", frame[6:12])
			protocolCorpusRequireValue(t, envelope, "Type", uint64(56))
			require.Equal(t, 3, protocolCorpusLLCFields(t, envelope, frame[14:]))
			envelopeISIS := protocolCorpusFindNode(envelope, "ISIS")
			require.NotNil(t, envelopeISIS, "FE/FE UI LLC did not dispatch to IS-IS")
			isisRequireLANHelloCompanionFields(t, envelopeISIS, frame[17:])

			node := isisParse(t, frame[17:])
			isisRequireLANHelloCompanionFields(t, node, frame[17:])
		})
	}
}

func TestISISCorePDUFixedHeaders(t *testing.T) {
	for _, vector := range isisCoreVectors() {
		vector := vector
		t.Run(vector.name, func(t *testing.T) {
			wire := mustHex(t, vector.hex)
			node := isisParse(t, wire)
			isisRequireCommonFields(t, node, wire)
			protocolCorpusRequireValue(t, node, "PDU Length", uint64(len(wire)))
			switch typ := wire[4] & 0x1f; typ {
			case 15, 16:
				protocolCorpusRequireValue(t, node, "Circuit Type", uint64(wire[8]&3))
				protocolCorpusRequireValue(t, node, "Reserved Circuit Bits", uint64(wire[8]>>2))
				protocolCorpusRequireValue(t, node, "Source System ID", wire[9:15])
				protocolCorpusRequireValue(t, node, "Holding Timer", uint64(binary.BigEndian.Uint16(wire[15:17])))
				protocolCorpusRequireValue(t, node, "Reserved Priority Bit", uint64(wire[19]>>7))
				protocolCorpusRequireValue(t, node, "Priority", uint64(wire[19]&0x7f))
				protocolCorpusRequireValue(t, node, "LAN ID", wire[20:27])
			case 17:
				protocolCorpusRequireValue(t, node, "Circuit Type", uint64(wire[8]&3))
				protocolCorpusRequireValue(t, node, "Reserved Circuit Bits", uint64(wire[8]>>2))
				protocolCorpusRequireValue(t, node, "Source System ID", wire[9:15])
				protocolCorpusRequireValue(t, node, "Holding Timer", uint64(binary.BigEndian.Uint16(wire[15:17])))
				protocolCorpusRequireValue(t, node, "Local Circuit ID", uint64(wire[19]))
			case 18, 20:
				protocolCorpusRequireValue(t, node, "Remaining Lifetime", uint64(binary.BigEndian.Uint16(wire[10:12])))
				protocolCorpusRequireValue(t, node, "LSP ID", wire[12:20])
				protocolCorpusRequireValue(t, node, "Sequence Number", uint64(binary.BigEndian.Uint32(wire[20:24])))
				protocolCorpusRequireValue(t, node, "Checksum", uint64(binary.BigEndian.Uint16(wire[24:26])))
				protocolCorpusRequireValue(t, node, "Partition Repair", uint64(wire[26]>>7))
				protocolCorpusRequireValue(t, node, "Attached Error", uint64(wire[26]>>6&1))
				protocolCorpusRequireValue(t, node, "Attached Expense", uint64(wire[26]>>5&1))
				protocolCorpusRequireValue(t, node, "Attached Delay", uint64(wire[26]>>4&1))
				protocolCorpusRequireValue(t, node, "Attached Default", uint64(wire[26]>>3&1))
				protocolCorpusRequireValue(t, node, "Overload", uint64(wire[26]>>2&1))
				protocolCorpusRequireValue(t, node, "IS Type", uint64(wire[26]&3))
			case 24, 25:
				protocolCorpusRequireValue(t, node, "Source Node ID", wire[10:17])
				protocolCorpusRequireValue(t, node, "Start LSP ID", wire[17:25])
				protocolCorpusRequireValue(t, node, "End LSP ID", wire[25:33])
			case 26, 27:
				protocolCorpusRequireValue(t, node, "Source Node ID", wire[10:17])
			default:
				t.Fatalf("uncovered PDU type %d", typ)
			}
			require.Nil(t, protocolCorpusFindNode(node, "TLVs"))
		})
	}
}

func TestISISLANHelloCompanionAndLLCDispatch(t *testing.T) {
	wire := isisLANHelloCompanion(t)
	node := isisParse(t, wire)
	isisRequireLANHelloCompanionFields(t, node, wire)

	for _, control := range []byte{3, 0x13} {
		frame := isisDot3Frame(wire, control)
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, 3, protocolCorpusLLCFields(t, envelope, frame[14:]))
		require.NotNil(t, protocolCorpusFindNode(envelope, "ISIS"), "FE/FE UI control 0x%02x did not dispatch", control)
		protocolCorpusRequireValue(t, envelope, "Address", mustHex(t, "490001"))
	}

	for _, header := range [][]byte{{0xff, 0xfe, 3}, {0xfe, 0xff, 3}, {0xfe, 0xfe, 0x0f}} {
		llc := append(bytes.Clone(header), wire...)
		notISIS := protocolCorpusRequireBoundedRuleParse(t, llc, "llc", "LLC")
		require.Nil(t, protocolCorpusFindNode(notISIS, "ISIS"), "non-FE/FE/UI carrier dispatched to IS-IS")
	}
}

func TestISISTypedTLVsAndLSPChecksumRecipe(t *testing.T) {
	fixed := mustHex(t, isisCoreVectors()[3].hex)
	wire := isisAppendTLVBytes(t, fixed, 8,
		isisTLV(t, 6, 1, 2, 3, 4, 5, 6, 10, 11, 12, 13, 14, 15),
		isisTLV(t, 8, 0, 1, 2),
		isisTLV(t, 10, 1, 'a', 'b', 'c'),
		isisTLV(t, 14, 0x05, 0xdc),
		isisTLV(t, 137, 'r', 'o', 'u', 't', 'e', 'r'),
		isisTLV(t, 232, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1),
		isisTLV(t, 250, 0xde, 0xad, 0xbe, 0xef),
	)
	isisSetLSPChecksum(t, wire)
	node := isisParse(t, wire)
	protocolCorpusRequireValue(t, node, "PDU Length", uint64(len(wire)))
	protocolCorpusRequireValue(t, node, "Checksum", uint64(binary.BigEndian.Uint16(wire[24:26])))
	require.Len(t, protocolCorpusNodesNamed(node, "TLV"), 7)

	neighbors := isisFindTLV(t, node, 6)
	systemIDs := protocolCorpusNodesNamed(neighbors, "System ID")
	require.Len(t, systemIDs, 2)
	protocolCorpusRequireValue(t, systemIDs[0], "System ID", []byte{1, 2, 3, 4, 5, 6})
	protocolCorpusRequireValue(t, systemIDs[1], "System ID", []byte{10, 11, 12, 13, 14, 15})
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 8), "Padding", []byte{0, 1, 2})

	authentication := isisFindTLV(t, node, 10)
	protocolCorpusRequireValue(t, authentication, "Authentication Type", uint64(1))
	protocolCorpusRequireValue(t, authentication, "Authentication Value", []byte("abc"))
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 14), "Buffer Size", uint64(1500))
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 137), "Hostname Bytes", []byte("router"))
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 232), "IPv6 Address", mustHex(t, "20010db8000000000000000000000001"))
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 250), "Unknown Value", mustHex(t, "deadbeef"))
}

func TestISISSNPEntriesOptionalChecksumAndP2PState(t *testing.T) {
	csnpFixed := mustHex(t, isisCoreVectors()[5].hex)
	entryValue := mustHex(t, "04b0010203040506070801020304abcd")
	csnp := isisAppendTLVBytes(t, csnpFixed, 8,
		isisTLV(t, 9, entryValue...),
		// RFC 3358 defines zero as a correct optional-checksum value.
		isisTLV(t, 12, 0, 0),
	)
	node := isisParse(t, csnp)
	protocolCorpusRequireValue(t, node, "PDU Length", uint64(len(csnp)))
	entries := protocolCorpusNodesNamed(isisFindTLV(t, node, 9), "LSP Entry")
	require.Len(t, entries, 1)
	protocolCorpusRequireValue(t, entries[0], "Remaining Lifetime", uint64(1200))
	protocolCorpusRequireValue(t, entries[0], "LSP ID", mustHex(t, "0102030405060708"))
	protocolCorpusRequireValue(t, entries[0], "Sequence Number", uint64(0x01020304))
	protocolCorpusRequireValue(t, entries[0], "Checksum", uint64(0xabcd))
	protocolCorpusRequireValue(t, isisFindTLV(t, node, 12), "Checksum Value", uint64(0))

	p2pFixed := mustHex(t, isisCoreVectors()[2].hex)
	// Exercise the standard nonzero encodings that retain the six-octet ID:
	// ID Length 6, ignored reserved bits, and Maximum Area Addresses 2.
	p2pFixed[3], p2pFixed[4], p2pFixed[6], p2pFixed[7], p2pFixed[8] = 6, 0xb1, 0x7e, 2, 0x83
	p2p := isisAppendTLVBytes(t, p2pFixed, 17,
		isisTLV(t, 240,
			0,
			0x11, 0x22, 0x33, 0x44,
			1, 2, 3, 4, 5, 6,
			0x55, 0x66, 0x77, 0x88,
		),
	)
	node = isisParse(t, p2p)
	isisRequireCommonFields(t, node, p2p)
	protocolCorpusRequireValue(t, node, "Reserved Circuit Bits", uint64(0x20))
	protocolCorpusRequireValue(t, node, "Circuit Type", uint64(3))
	state := isisFindTLV(t, node, 240)
	protocolCorpusRequireValue(t, state, "Adjacency State", uint64(0))
	protocolCorpusRequireValue(t, state, "Extended Local Circuit ID", uint64(0x11223344))
	protocolCorpusRequireValue(t, state, "Neighbor System ID", []byte{1, 2, 3, 4, 5, 6})
	protocolCorpusRequireValue(t, state, "Neighbor Extended Local Circuit ID", uint64(0x55667788))
}

func TestISISStrictBoundariesAndMalformedTLVs(t *testing.T) {
	for _, vector := range isisCoreVectors() {
		wire := mustHex(t, vector.hex)
		t.Run(vector.name+" prefixes", func(t *testing.T) {
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "isis", "ISIS")
				require.Errorf(t, err, "%s accepted truncated prefix %d/%d", vector.name, cut, len(wire))
			}
		})
	}

	valid := isisLANHelloCompanion(t)
	_, err := parser.ParseBinary(bytes.NewReader(valid), "isis", "ISIS")
	require.ErrorContains(t, err, "isis: PDU boundary is required")
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(valid), 0)), "isis", "ISIS")
	require.ErrorContains(t, err, "isis: PDU length does not match input")

	for _, tc := range []struct {
		name       string
		offset     int
		value      byte
		diagnostic string
	}{
		{name: "discriminator", offset: 0, value: 0x82, diagnostic: "invalid intradomain routing protocol discriminator"},
		{name: "length indicator", offset: 1, value: 26, diagnostic: "length indicator does not match"},
		{name: "protocol extension", offset: 2, value: 2, diagnostic: "unsupported version/protocol ID extension"},
		{name: "unsupported ID length", offset: 3, value: 9, diagnostic: "only six-octet system IDs"},
		{name: "PDU type", offset: 4, value: 19, diagnostic: "unsupported PDU type"},
		{name: "PDU version", offset: 5, value: 2, diagnostic: "unsupported PDU version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Clone(valid)
			bad[tc.offset] = tc.value
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "isis", "ISIS")
			require.ErrorContains(t, err, tc.diagnostic)
		})
	}

	for _, declared := range []uint16{0, 26, uint16(len(valid) - 1), uint16(len(valid) + 1), 65535} {
		bad := bytes.Clone(valid)
		binary.BigEndian.PutUint16(bad[17:19], declared)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "isis", "ISIS")
		require.ErrorContains(t, err, "PDU length does not match input")
	}
	zeroCircuit := bytes.Clone(valid)
	zeroCircuit[8] &= 0xfc
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(zeroCircuit), "isis", "ISIS")
	require.ErrorContains(t, err, "invalid zero circuit type")

	fixed := mustHex(t, isisCoreVectors()[0].hex)
	malformed := []struct {
		name       string
		field      []byte
		diagnostic string
	}{
		{name: "TLV header", field: []byte{250}, diagnostic: "truncated TLV header"},
		{name: "TLV value", field: []byte{250, 4, 1, 2}, diagnostic: "TLV length exceeds PDU boundary"},
		{name: "area address", field: isisTLV(t, 1, 3, 0x49), diagnostic: "area address exceeds TLV boundary"},
		{name: "empty area address", field: isisTLV(t, 1, 0), diagnostic: "empty area address"},
		{name: "IS neighbors", field: isisTLV(t, 6, 1, 2, 3, 4, 5, 6, 7), diagnostic: "not a multiple of six"},
		{name: "LSP entries", field: isisTLV(t, 9, make([]byte, 15)...), diagnostic: "not a multiple of sixteen"},
		{name: "authentication", field: isisTLV(t, 10), diagnostic: "authentication TLV has no type"},
		{name: "optional checksum", field: isisTLV(t, 12, 1), diagnostic: "optional checksum TLV length must be two"},
		{name: "buffer size", field: isisTLV(t, 14, 1), diagnostic: "LSP buffer size TLV length must be two"},
		{name: "IPv4 addresses", field: isisTLV(t, 132, 192, 0, 2), diagnostic: "not a multiple of four"},
		{name: "hostname", field: isisTLV(t, 137), diagnostic: "dynamic hostname TLV is empty"},
		{name: "IPv6 addresses", field: isisTLV(t, 232, make([]byte, 15)...), diagnostic: "not a multiple of sixteen"},
		{name: "P2P state", field: isisTLV(t, 240, 0, 1), diagnostic: "invalid point-to-point adjacency state TLV length"},
	}
	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			bad := isisAppendTLVBytes(t, fixed, 17, tc.field)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "isis", "ISIS")
			require.ErrorContains(t, err, tc.diagnostic)
		})
	}

	// Zero-length Padding and an unknown zero-length TLV both have complete
	// framing and therefore remain valid, fully consumed structural values.
	zeroLength := isisAppendTLVBytes(t, fixed, 17, isisTLV(t, 8), isisTLV(t, 250))
	node := isisParse(t, zeroLength)
	require.Len(t, protocolCorpusNodesNamed(node, "TLV"), 2)
	padding := isisFindTLV(t, node, 8)
	protocolCorpusRequireValue(t, padding, "TLV Length", uint64(0))
	require.Nil(t, protocolCorpusFindNode(padding, "Padding"))
	unknown := isisFindTLV(t, node, 250)
	protocolCorpusRequireValue(t, unknown, "TLV Length", uint64(0))
	require.Nil(t, protocolCorpusFindNode(unknown, "Unknown Value"))
}

func TestISISParallelCorePDUParsing(t *testing.T) {
	for _, vector := range isisCoreVectors() {
		vector := vector
		t.Run(vector.name, func(t *testing.T) {
			t.Parallel()
			wire := mustHex(t, vector.hex)
			for iteration := 0; iteration < 8; iteration++ {
				node := isisParse(t, wire)
				isisRequireCommonFields(t, node, wire)
			}
		})
	}
}
