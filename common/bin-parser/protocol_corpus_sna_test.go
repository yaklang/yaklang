package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type snaTestField struct {
	name  string
	value any
	bits  uint64
}

func snaU8(name string, value uint8) snaTestField   { return snaTestField{name, value, 8} }
func snaU16(name string, value uint16) snaTestField { return snaTestField{name, value, 16} }
func snaRaw(name string, value []byte) snaTestField {
	return snaTestField{name, value, uint64(len(value)) * 8}
}
func snaBit(name string, value uint8, bits uint64) snaTestField {
	return snaTestField{name, value, bits}
}

// Independent MSB-first serialization of the IBM field tables; fixture bytes,
// field values, and spans are not derived from the parser or capture generator.
func snaEncode(fields []snaTestField) []byte {
	var count uint64
	for _, field := range fields {
		count += field.bits
	}
	wire := make([]byte, (count+7)/8)
	var pos uint64
	for _, field := range fields {
		var value uint64
		switch v := field.value.(type) {
		case uint8:
			value = uint64(v)
		case uint16:
			value = uint64(v)
		case []byte:
			for _, octet := range v {
				for bit := uint64(0); bit < 8; bit++ {
					wire[pos/8] |= ((octet >> (7 - bit)) & 1) << (7 - pos%8)
					pos++
				}
			}
			continue
		default:
			panic("unexpected SNA fixture type")
		}
		for bit := uint64(0); bit < field.bits; bit++ {
			wire[pos/8] |= byte((value>>(field.bits-1-bit))&1) << (7 - pos%8)
			pos++
		}
	}
	return wire
}

func snaRHFields(rh [3]byte) []snaTestField {
	fields := []snaTestField{
		snaBit("Response Indicator", rh[0]>>7, 1), snaBit("RU Category", (rh[0]>>5)&3, 2),
		snaBit("RH Reserved Bit", (rh[0]>>4)&1, 1), snaBit("Format Indicator", (rh[0]>>3)&1, 1),
		snaBit("Sense Data Included", (rh[0]>>2)&1, 1), snaBit("Begin Chain", (rh[0]>>1)&1, 1), snaBit("End Chain", rh[0]&1, 1),
	}
	names := []string{"Definite Response 1", "Length Checked Compression", "Definite Response 2", "Exception Response", "RH Second Reserved Bit", "Request Larger Window", "Queued Response", "Pacing Indicator"}
	response := rh[0]&0x80 != 0
	if response {
		names[1] = "Response Reserved Bit 1"
		names[3] = "Negative Response"
		names[5] = "Response Reserved Bit 5"
	}
	for i, name := range names {
		fields = append(fields, snaBit(name, (rh[1]>>uint(7-i))&1, 1))
	}
	if response {
		return append(fields, snaU8("Response Reserved Octet", rh[2]))
	}
	for i, name := range []string{"Begin Bracket", "End Bracket", "Change Direction", "Request Third Reserved Bit", "Code Selection", "Enciphered Data", "Padded Data", "Conditional End Bracket"} {
		fields = append(fields, snaBit(name, (rh[2]>>uint(7-i))&1, 1))
	}
	return fields
}

type snaTestFixture struct {
	name            string
	th              uint8
	rh              [3]byte
	body            []snaTestField
	control, second uint8
	llcOnly         bool
	info            map[string]any
}

func (f snaTestFixture) piuFields() []snaTestField {
	fid := f.th >> 4
	fields := []snaTestField{snaBit("Format Identification", fid, 4), snaBit("Begin BIU", (f.th>>3)&1, 1), snaBit("End BIU", (f.th>>2)&1, 1)}
	name := "TH Reserved Bit"
	if fid == 2 {
		name = "ODAI"
	}
	fields = append(fields, snaBit(name, (f.th>>1)&1, 1), snaBit("Expedited Flow", f.th&1, 1))
	if fid == 3 {
		fields = append(fields, snaBit("LU SSCP Indicator", 1, 1), snaBit("LU PU Indicator", 1, 1), snaBit("Local Address", 5, 6))
	} else {
		fields = append(fields, snaU8("TH Reserved Octet", 0))
		if fid == 2 {
			fields = append(fields, snaU8("Destination Local Address", 1), snaU8("Origin Local Address", 2))
		} else {
			fields = append(fields, snaU16("Destination Address", 0x1234), snaU16("Origin Address", 0x5678))
		}
		fields = append(fields, snaU16("Sequence Number", 0x102))
		if fid < 2 {
			length := len(snaEncode(f.body))
			if f.th&8 != 0 {
				length += 3
			}
			fields = append(fields, snaU16("Data Count", uint16(length)))
		}
	}
	if f.th&8 != 0 {
		fields = append(fields, snaRHFields(f.rh)...)
	}
	return append(fields, f.body...)
}

func (f snaTestFixture) fields() []snaTestField {
	control := f.control
	if control == 0 {
		control = 3
	}
	llc := []snaTestField{snaU8("DSAP", 4), snaU8("SSAP", 4), snaU8("LLC Control", control)}
	if control&3 != 3 {
		llc = append(llc, snaU8("LLC Extended Control", f.second))
	}
	if f.llcOnly {
		llc = append(llc, f.body...)
	} else {
		llc = append(llc, f.piuFields()...)
	}
	fields := []snaTestField{snaU16("SNA Ethernet Length", uint16(len(snaEncode(llc)))), snaU8("SNA Ethernet Padding", 0)}
	return append(fields, llc...)
}
func (f snaTestFixture) wire() []byte { return snaEncode(f.fields()) }

func snaTestFixtures() []snaTestFixture {
	return []snaTestFixture{
		{name: "fid2-request", th: 0x2c, rh: [3]byte{3, 0x80, 0xc0}, control: 4, second: 7, body: []snaTestField{snaRaw("Uninterpreted RU Bytes", []byte{1, 0, 0xff})}},
		{name: "fid2-positive", th: 0x2c, rh: [3]byte{0x83, 0x80, 0}},
		{name: "fid2-sense", th: 0x2c, rh: [3]byte{0x87, 0x90, 0}, body: []snaTestField{snaU8("Sense Category", 8), snaU8("Sense Modifier", 1), snaU16("Sense Specific Information", 0x1234), snaRaw("Sense Associated RU Bytes", []byte{0x31})}, info: map[string]any{"Sense Fields Decoded": true}},
		{name: "fid2-sdt-code", th: 0x2d, rh: [3]byte{0x6b, 0x80, 0}, body: []snaTestField{snaU8("RU Request Code", 0xa0)}, info: map[string]any{"RU Code Decoded": true}},
		{name: "fid0-chase-code", th: 0x0c, rh: [3]byte{0x4b, 0x80, 0}, body: []snaTestField{snaU8("RU Request Code", 0x84)}, info: map[string]any{"RU Code Decoded": true}},
		{name: "fid1-first-segment", th: 0x18, rh: [3]byte{3, 0x80, 0}, body: []snaTestField{snaRaw("First BIU Segment Bytes", []byte{8, 1})}},
		{name: "fid2-middle-segment", th: 0x20, body: []snaTestField{snaRaw("BIU Continuation Bytes", []byte{1, 2})}},
		{name: "fid2-last-segment", th: 0x24, body: []snaTestField{snaRaw("BIU Continuation Bytes", []byte{3, 4})}},
		{name: "fid3-request", th: 0x3c, rh: [3]byte{3, 0, 0}, body: []snaTestField{snaRaw("Uninterpreted RU Bytes", []byte{0x40, 0x40})}},
		{name: "fid2-ipr", th: 0x2c, rh: [3]byte{0x83, 1, 0}, info: map[string]any{"Isolated Pacing Unit": true}},
		{name: "fid2-ipm", th: 0x2d, rh: [3]byte{0x83, 1, 0}, body: []snaTestField{snaBit("IPM Type", 1, 2), snaBit("Reset Residual Count", 1, 1), snaBit("IPM Reserved Bits", 0, 5), {"Next Window Format", uint16(0), 1}, {"Next Window Size", uint16(16), 15}}, info: map[string]any{"Isolated Pacing Unit": true, "IPM Extension Decoded": true}},
		{name: "fid2-compression-header", th: 0x2c, rh: [3]byte{3, 0x40, 0}, body: []snaTestField{snaBit("Compression Algorithm", 1, 4), snaBit("Uncompressed Data Type", 1, 4), snaU16("Uncompressed RU Length", 4), snaRaw("Compressed RU Bytes", []byte{0xc4, 0x40})}, info: map[string]any{"Compression Header Decoded": true}},
		{name: "fid2-enciphered-opaque", th: 0x2c, rh: [3]byte{3, 0x40, 4}, body: []snaTestField{snaRaw("Enciphered RU Bytes", []byte{0x11, 0, 4, 0xc4, 0x40})}},
		{name: "llc-xid-opaque", llcOnly: true, control: 0xaf, body: []snaTestField{snaRaw("Uninterpreted LLC Information", []byte{0x81, 3, 0xfe})}},
		{name: "llc-supervisory", llcOnly: true, control: 1, second: 0x0b},
		{name: "fid2-format1-session-dependent", th: 0x2c, rh: [3]byte{0x0b, 0x80, 0}, body: []snaTestField{snaRaw("Uninterpreted RU Bytes", []byte{1, 2, 1})}},
	}
}

func snaTestFields(t *testing.T, node *base.Node, fields []snaTestField, offset uint64) {
	t.Helper()
	_, err := node.Result()
	require.NoError(t, err)
	var leaves []*base.Node
	var walk func(*base.Node)
	walk = func(parent *base.Node) {
		for _, child := range parent.Children {
			require.Same(t, parent, child.Cfg.GetItem(base.CfgParent), child.Name)
			if child.Cfg.GetItem(base.CfgIsTerminal) == true && stream_parser.NodeHasResult(child) {
				leaves = append(leaves, child)
			}
			walk(child)
		}
	}
	walk(node)
	require.Len(t, leaves, len(fields))
	pos := offset
	for index, field := range fields {
		leaf := leaves[index]
		require.Equal(t, field.name, leaf.Name)
		value, err := leaf.Result()
		require.NoError(t, err)
		require.Same(t, leaf, value.Origin)
		require.Equal(t, field.value, value.Value, field.name)
		typ := map[string]string{"uint8": "uint8", "uint16": "uint16", "[]uint8": "raw"}[fmt.Sprintf("%T", field.value)]
		require.Equal(t, typ, leaf.Cfg.GetItem(base.CfgType), field.name)
		require.Equal(t, [2]uint64{pos, pos + field.bits}, stream_parser.GetNodeResultPos(leaf), field.name)
		pos += field.bits
	}
	require.Equal(t, offset+uint64(len(snaEncode(fields)))*8, pos)
	// NodeToBytes intentionally returns the entire parsing context, including
	// enclosing prefixes/suffixes. Verify this field extent within that buffer.
	reader := base.NewBitReader(bytes.NewReader(NodeToBytes(node)))
	if offset > 0 {
		_, err = reader.ReadBits(offset)
		require.NoError(t, err)
	}
	actual, err := reader.ReadBits(pos - offset)
	require.NoError(t, err)
	require.Equal(t, snaEncode(fields), actual)
}

func snaTestInfo(t *testing.T, node *base.Node, f snaTestFixture, ethernet bool) {
	t.Helper()
	if ethernet {
		first := protocolCorpusFindNode(node, "SNA Ethernet Length")
		require.NotNil(t, first)
		info := alljoynTestInfo(t, first.Cfg.GetItem(base.CfgParent).(*base.Node))
		require.Equal(t, "SNA Ethernet LLC bounded field layout", info["Profile"])
		require.Equal(t, !f.llcOnly, info["PIU Decoded"])
		require.Equal(t, false, info["XID Body Decoded"])
		require.Equal(t, false, info["Session State Validated"])
		if f.control == 4 {
			require.EqualValues(t, 2, info["LLC Send Sequence"])
			require.EqualValues(t, 3, info["LLC Receive Sequence"])
			require.Equal(t, true, info["LLC Poll Final"])
		}
	}
	if f.llcOnly {
		require.Nil(t, protocolCorpusFindNode(node, "Format Identification"))
		return
	}
	first := protocolCorpusFindNode(node, "Format Identification")
	require.NotNil(t, first)
	info := alljoynTestInfo(t, first.Cfg.GetItem(base.CfgParent).(*base.Node))
	require.Equal(t, "SNA FID0 FID1 FID2 FID3 bounded PIU fields", info["Profile"])
	header := map[uint8]int{0: 10, 1: 10, 2: 6, 3: 2}[f.th>>4]
	require.EqualValues(t, header, info["TH Bytes"])
	require.Equal(t, f.th&8 != 0, info["RH Present"])
	require.Equal(t, f.th&12 == 12, info["Whole BIU"])
	for _, key := range []string{"Sense Fields Decoded", "Isolated Pacing Unit", "IPM Extension Decoded", "Compression Header Decoded", "RU Code Decoded", "Application Body Decoded", "Reassembled", "Decompressed", "Session State Validated"} {
		want := any(false)
		if v, ok := f.info[key]; ok {
			want = v
		}
		require.Equal(t, want, info[key], key)
	}
}

func TestProtocolCorpusSNAFields(t *testing.T) {
	for _, f := range snaTestFixtures() {
		t.Run(f.name, func(t *testing.T) {
			t.Logf("companion %s %d %x", f.name, len(f.wire()), f.wire())
			for _, entry := range []string{"SNAEthernet", "SNAEthernetCarrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "sna", entry)
				require.Equal(t, f.wire(), NodeToBytes(node))
				snaTestFields(t, node, f.fields(), 0)
				snaTestInfo(t, node, f, true)
			}
			if !f.llcOnly {
				node := protocolCorpusRequireBoundedRuleParse(t, snaEncode(f.piuFields()), "sna", "SNAPIU")
				require.Equal(t, snaEncode(f.piuFields()), NodeToBytes(node))
				snaTestFields(t, node, f.piuFields(), 0)
				snaTestInfo(t, node, f, false)
			}
		})
	}
}

func snaTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "sna", "SNAEthernet")
	require.Error(t, err)
	if len(wire) == 0 {
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "sna", "SNAEthernetCarrier")
		require.Error(t, err)
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "sna", "SNAEthernetCarrier")
	miopTestField(t, node, "Unparsed SNA Ethernet Packet", wire, 0, uint64(len(wire))*8)
	require.Equal(t, wire, NodeToBytes(node))
	for _, name := range []string{"SNA Ethernet Length", "DSAP", "PIU", "Format Identification", "Request Response Header"} {
		require.Nil(t, protocolCorpusFindNode(node, name), name)
	}
}

func TestProtocolCorpusSNAOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-sna.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "6cfec3ff48e2727eb5f7a82a627b257d9492a8fe6db9c482edd7b185b63c1ced", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 30)
		require.Equal(t, []byte{0x80, 0xd5}, frame[12:14])
		wire := append([]byte{0x2c}, make([]byte, 15)...)
		require.Equal(t, wire, frame[14:])
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "sna", "SNAEthernet")
		require.ErrorContains(t, err, "encapsulated length")
		snaTestReject(t, wire)
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
		miopTestField(t, node, "Unparsed SNA Ethernet Packet", wire, 112, 240)
	}
}

func TestProtocolCorpusSNACompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-sna-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "47b5862b5e95e205a25b7244173564f1e02abf76983ee86d4ac859e9f6ade7e8", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	fixtures := snaTestFixtures()
	require.Len(t, frames, len(fixtures))
	for index, frame := range frames {
		f := fixtures[index]
		t.Run(f.name, func(t *testing.T) {
			require.Len(t, frame, 14+len(f.wire()))
			require.Equal(t, []byte{0x80, 0xd5}, frame[12:14])
			require.Equal(t, f.wire(), frame[14:])
			for _, entry := range []string{"SNAEthernet", "SNAEthernetCarrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, frame[14:], "sna", entry)
				snaTestFields(t, node, f.fields(), 0)
				snaTestInfo(t, node, f, true)
				require.Equal(t, frame[14:], NodeToBytes(node))
			}
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			first := protocolCorpusFindNode(node, "SNA Ethernet Length")
			require.NotNil(t, first)
			packet := first.Cfg.GetItem(base.CfgParent).(*base.Node)
			snaTestFields(t, packet, f.fields(), 112)
			snaTestInfo(t, packet, f, true)
			require.Equal(t, frame, NodeToBytes(node))
		})
	}
}

func TestProtocolCorpusSNAPrefixesAndBoundaries(t *testing.T) {
	for _, f := range snaTestFixtures() {
		wire := f.wire()
		for cut := 0; cut < len(wire); cut++ {
			snaTestReject(t, wire[:cut])
		}
		snaTestReject(t, append(bytes.Clone(wire), 0))
		for _, length := range []uint16{0, 2, uint16(len(wire) - 4), uint16(len(wire) - 2), 0xffff} {
			bad := bytes.Clone(wire)
			binary.BigEndian.PutUint16(bad, length)
			snaTestReject(t, bad)
		}
		if f.llcOnly {
			continue
		}
		piu := snaEncode(f.piuFields())
		header := map[uint8]int{0: 10, 1: 10, 2: 6, 3: 2}[f.th>>4]
		minimum := header
		if f.th&8 != 0 {
			minimum += 3
		}
		for cut := 0; cut < minimum; cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(piu[:cut]), "sna", "SNAPIU")
			require.Error(t, err)
		}
	}
	// Recompute the Ethernet bound so these reach inner structural failures.
	baseWire := snaTestFixtures()[1].wire()
	for _, fid := range []byte{0x40, 0x50, 0x80, 0xf0} {
		bad := bytes.Clone(baseWire)
		bad[6] = fid | 12
		snaTestReject(t, bad)
	}
	for _, sap := range []byte{0, 6, 0xaa, 0xfe} {
		bad := bytes.Clone(baseWire)
		bad[3] = sap
		snaTestReject(t, bad)
	}
	for _, index := range []int{2, 10, 11} {
		wire := snaTestFixtures()[index].wire()
		for _, bodyLength := range []int{1, 2} {
			bad := bytes.Clone(wire[:6+6+3+bodyLength])
			binary.BigEndian.PutUint16(bad, uint16(len(bad)-3))
			snaTestReject(t, bad)
		}
	}
	f := snaTestFixtures()[4]
	wire := f.wire()
	for _, dcf := range []uint16{0, 3, 5, 0xffff} {
		bad := bytes.Clone(wire)
		binary.BigEndian.PutUint16(bad[14:], dcf)
		snaTestReject(t, bad)
	}
	bad := append(bytes.Clone(snaTestFixtures()[14].wire()), 0)
	binary.BigEndian.PutUint16(bad, uint16(len(bad)-3))
	snaTestReject(t, bad)
	// FID2 has no RU length field: a shorter bounded raw RU is valid, not a
	// truncated header. This test prevents conflating those separate cases.
	f = snaTestFixtures()[0]
	for n := 0; n < 3; n++ {
		short := f
		short.body = nil
		if n > 0 {
			short.body = []snaTestField{snaRaw("Uninterpreted RU Bytes", []byte{1, 0, 0xff}[:n])}
		}
		node := protocolCorpusRequireBoundedRuleParse(t, short.wire(), "sna", "SNAEthernet")
		snaTestFields(t, node, short.fields(), 0)
	}
}

func TestProtocolCorpusSNACompatibilityAndOpaqueBodies(t *testing.T) {
	for _, index := range []int{9, 10} {
		f := snaTestFixtures()[index]
		// IBM identifies isolated pacing using only RRI, DR1, DR2 and PI;
		// its receiver ignores other RH bits, including SDI and RTI.
		f.rh = [3]byte{0xff, 0x5f, 0xff}
		node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "sna", "SNAEthernet")
		snaTestFields(t, node, f.fields(), 0)
		snaTestInfo(t, node, f, true)
		require.Nil(t, protocolCorpusFindNode(node, "Sense Data"))
	}
	for _, fid := range []uint8{0, 1, 2, 3} {
		f := snaTestFixtures()[0]
		f.th = fid<<4 | 0x0e
		// Reserved and retired bits are preserved; no inferred address policy.
		wire := f.wire()
		if fid != 3 {
			wire[8] = 0xa5
		}
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "sna", "SNAEthernet")
		require.Equal(t, wire, NodeToBytes(node))
	}
	f := snaTestFixtures()[11]
	f.body = []snaTestField{snaBit("Compression Algorithm", 15, 4), snaBit("Uncompressed Data Type", 2, 4), snaRaw("Uninterpreted Compression Bytes", []byte{1, 2, 3})}
	f.info = nil
	node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "sna", "SNAEthernet")
	snaTestFields(t, node, f.fields(), 0)
	snaTestInfo(t, node, f, true)
	// A first response segment may contain only part of a sense field (for
	// example segmented RSP(BIND)); full-body decoding waits for reassembly.
	f = snaTestFixtures()[5]
	f.rh = [3]byte{0xef, 0x90, 0}
	node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), "sna", "SNAEthernet")
	snaTestFields(t, node, f.fields(), 0)
	snaTestInfo(t, node, f, true)
	for _, sap := range []byte{4, 5, 8, 9, 12, 13, 64, 65, 200, 201} {
		wire := snaTestFixtures()[1].wire()
		wire[3] = sap
		wire[4] = 5
		wire[2] = 0xa5
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "sna", "SNAEthernet")
		require.Equal(t, wire, NodeToBytes(node))
		packet := protocolCorpusFindNode(node, "SNA Ethernet Length").Cfg.GetItem(base.CfgParent).(*base.Node)
		info := alljoynTestInfo(t, packet)
		require.EqualValues(t, sap&0xfe, info["Destination SAP"])
		require.Equal(t, sap&1 != 0, info["Destination Group"])
		require.Equal(t, true, info["LLC Response"])
	}
}

func TestProtocolCorpusSNAResourcesAndEntryBoundaries(t *testing.T) {
	f := snaTestFixtures()[0]
	f.body = []snaTestField{snaRaw("Uninterpreted RU Bytes", bytes.Repeat([]byte{0xa5}, 1484))}
	require.Len(t, f.wire(), 1500)
	for _, entry := range []string{"SNAEthernet", "SNAEthernetCarrier", "SNAPIU"} {
		wire := f.wire()
		if entry == "SNAPIU" {
			piu := f
			piu.body = []snaTestField{snaRaw("Uninterpreted RU Bytes", bytes.Repeat([]byte{0xa5}, 1491))}
			wire = snaEncode(piu.piuFields())
			require.Len(t, wire, 1500)
		}
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "sna", entry)
		require.Equal(t, wire, NodeToBytes(node))
		_, err := parser.ParseBinary(bytes.NewReader(wire), "sna", entry)
		require.ErrorContains(t, err, "explicit packet boundary")
		_, err = parser.GenerateBinary(map[string]any{}, "sna", entry)
		require.ErrorContains(t, err, "explicit packet boundary")
		for bits := uint64(1); bits < 8; bits++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(wire), uint64(len(wire))*8 - bits}
			_, err := parser.ParseBinary(reader, "sna", entry)
			require.ErrorContains(t, err, "explicit byte boundary")
			require.Equal(t, len(wire), reader.Len())
		}
		if entry == "SNAPIU" {
			reader := newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0))
			_, err := parser.ParseBinary(reader, "sna", entry)
			require.ErrorContains(t, err, "implementation profile")
			require.Equal(t, 1501, reader.Len())
		}
	}
	f.body = []snaTestField{snaRaw("Uninterpreted RU Bytes", bytes.Repeat([]byte{0xa5}, 1485))}
	reader := newProtocolCorpusBoundedReader(f.wire())
	_, err := parser.ParseBinary(reader, "sna", "SNAEthernet")
	require.ErrorContains(t, err, "implementation profile")
	require.Equal(t, 1501, reader.Len())
	snaTestReject(t, f.wire())
}

func TestProtocolCorpusSNAImportOffsetsAndRollback(t *testing.T) {
	for _, f := range snaTestFixtures() {
		for offset := 0; offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				wire := f.wire()
				if !valid {
					wire[1]++
				}
				prefix, padding := "", ""
				if offset > 0 {
					prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
					padding = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
				}
				root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Record").SetMaxLength(%d)
      this.ProcessSubNode("Record")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Record: "import:sna.yaml;node:SNAEthernetCarrier"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, padding))
				fields := []snaTestField{}
				if offset > 0 {
					fields = append(fields, snaBit("Prefix", uint8((1<<offset)-1), uint64(offset)))
				}
				fields = append(fields, snaRaw("Record", wire), snaU8("Suffix", 0x5a))
				if offset > 0 {
					fields = append(fields, snaBit("Padding", 0, uint64(8-offset)))
				}
				input := snaEncode(fields)
				root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
				reader := base.NewBitReader(bytes.NewReader(input))
				require.NoError(t, reader.Backup())
				require.NoError(t, root.ParseSubNode(reader, "Envelope"))
				node := base.GetNodeByPath(root, "@Envelope")
				record := protocolCorpusFindNode(node, "Record")
				if valid {
					snaTestFields(t, record, f.fields(), uint64(offset))
					snaTestInfo(t, record, f, true)
				} else {
					miopTestField(t, record, "Unparsed SNA Ethernet Packet", wire, uint64(offset), uint64(offset+len(wire)*8))
					require.Nil(t, protocolCorpusFindNode(record, "SNA Ethernet Length"))
				}
				miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
				require.Equal(t, input, NodeToBytes(node))
				require.NoError(t, reader.Recovery())
				replay, err := reader.ReadBits(uint64(len(input)) * 8)
				require.NoError(t, err)
				require.Equal(t, input, replay)
				require.ErrorContains(t, reader.Recovery(), "no backup")
			}
		}
	}
}

func TestProtocolCorpusSNAHeldReaderAndIsolation(t *testing.T) {
	f := snaTestFixtures()[2]
	for _, valid := range []bool{true, false} {
		for _, rollback := range []bool{true, false} {
			wire := f.wire()
			if !valid {
				wire = bytes.Clone(wire[:16])
				binary.BigEndian.PutUint16(wire, uint16(len(wire)-3))
			} // Late truncated sense, after TH/RH.
			reader := base.NewBitReader(bytes.NewReader(append(append([]byte{0xa5}, wire...), 0x5a)))
			_, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("sna.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "SNAEthernetCarrier"))
			node := base.GetNodeByPath(root, "@SNAEthernetCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				snaTestFields(t, node, f.fields(), 0)
			} else {
				require.Nil(t, protocolCorpusFindNode(node, "Format Identification"))
				protocolCorpusRequireValue(t, node, "Unparsed SNA Ethernet Packet", wire)
			}
			if rollback {
				require.NoError(t, reader.Recovery())
				replay, err := reader.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, replay)
			} else {
				require.NoError(t, reader.PopBackup())
			}
			suffix, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0x5a}, suffix)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for _, entry := range []string{"SNAEthernet", "SNAEthernetCarrier", "SNAPIU"} {
		wire := f.wire()
		if entry == "SNAPIU" {
			wire = snaEncode(f.piuFields())
		}
		for cut := 0; cut < len(wire); cut++ {
			root, err := base.ParseRule("sna.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery())
			if cut > 0 {
				replay, err := reader.ReadBits(uint64(cut) * 8)
				require.NoError(t, err)
				require.Equal(t, wire[:cut], replay)
			}
		}
	}
	var wait sync.WaitGroup
	errors := make(chan error, 16)
	for index, f := range snaTestFixtures() {
		wait.Add(1)
		go func(index int, f snaTestFixture) {
			defer wait.Done()
			wire := f.wire()
			valid := index%2 == 0
			if !valid {
				wire[1]++
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "sna", "SNAEthernetCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "SNA Ethernet Length") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed SNA Ethernet Packet") != nil) == valid) {
				err = fmt.Errorf("SNA context leaked at %d", index)
			}
			errors <- err
		}(index, f)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
