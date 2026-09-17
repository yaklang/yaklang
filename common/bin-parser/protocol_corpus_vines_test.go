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

type vinesTestField struct {
	name  string
	value any
	bits  uint64
}

func vinesU8(name string, value uint8) vinesTestField   { return vinesTestField{name, value, 8} }
func vinesU16(name string, value uint16) vinesTestField { return vinesTestField{name, value, 16} }
func vinesU32(name string, value uint32) vinesTestField { return vinesTestField{name, value, 32} }
func vinesRaw(name string, value []byte) vinesTestField {
	return vinesTestField{name, value, uint64(len(value)) * 8}
}

type vinesTestFixture struct {
	name              string
	protocol, control uint8
	broadcast         bool
	body              []vinesTestField
	info              map[string]map[string]any
}

// Independent network-order field serialization; no parser or generator is
// used as the value, offset, length, or byte oracle. Checksum is deliberately
// a retained layout value, not a claim that integrity was verified.
func (f vinesTestFixture) fields() []vinesTestField {
	// Include bit fields before converting the complete body size to octets.
	var bodyBits uint64
	for _, field := range f.body {
		bodyBits += field.bits
	}
	destination, host := uint32(0x10203040), uint16(0x8001)
	if f.broadcast {
		destination, host = 0xffffffff, 0xffff
	}
	fields := []vinesTestField{
		vinesU16("Checksum", 0xffff), vinesU16("Packet Length", uint16(18+bodyBits/8)),
		{"Control High Bit", uint8(f.control >> 7), 1},
		{"Control Flags", uint8((f.control >> 4) & 7), 3},
		{"Hop Count", uint8(f.control & 15), 4},
		vinesU8("Protocol", f.protocol), vinesU32("Destination Network", destination),
		vinesU16("Destination Subnetwork", host), vinesU32("Source Network", 0x50607080), vinesU16("Source Subnetwork", 1),
	}
	return append(fields, f.body...)
}

func (f vinesTestFixture) wire() []byte {
	fields := f.fields()
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
		case uint32:
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
			panic("unexpected fixture type")
		}
		for bit := uint64(0); bit < field.bits; bit++ {
			wire[pos/8] |= byte((value>>(field.bits-1-bit))&1) << (7 - pos%8)
			pos++
		}
	}
	return wire
}

func vinesTestControl(value uint8) []vinesTestField {
	return []vinesTestField{
		{"Immediate Acknowledgment", uint8(value >> 7), 1},
		{"End of Message", uint8((value >> 6) & 1), 1},
		{"Beginning of Message", uint8((value >> 5) & 1), 1},
		{"Abort Message", uint8((value >> 4) & 1), 1},
		{"Reserved Control Bits", uint8(value & 15), 4},
	}
}

func vinesTestFixtures() []vinesTestFixture {
	short := []vinesTestField{vinesU16("Source Port", 0x1234), vinesU16("Destination Port", 0x5678), vinesU8("IPC Packet Type", 0), vinesU8("Datagram Padding", 0), vinesRaw("IPC Data", []byte{0x61, 0, 0xff})}
	long := []vinesTestField{vinesU16("Source Port", 0x1234), vinesU16("Destination Port", 0x5678), vinesU8("IPC Packet Type", 1)}
	long = append(long, vinesTestControl(0x20)...)
	long = append(long, vinesU16("Local Connection ID", 0x102), vinesU16("Remote Connection ID", 0x304), vinesU16("Sequence Number", 5), vinesU16("Acknowledgment Number", 6), vinesU16("Message Length", 9), vinesRaw("IPC Data", []byte{1, 2, 3}))
	ipcError := append([]vinesTestField(nil), long[:len(long)-1]...)
	ipcError[2] = vinesU8("IPC Packet Type", 2)
	ipcError[len(ipcError)-1] = vinesU16("Error Code", 157)
	spp := append([]vinesTestField(nil), long[:len(long)-1]...)
	spp[2] = vinesU8("SPP Packet Type", 5)
	spp[len(spp)-1] = vinesU16("Window", 0x1234)
	arp0 := []vinesTestField{vinesU8("ARP Version", 0), vinesU8("ARP Packet Type", 3), vinesU32("Assigned Network", 0x12345678), vinesU16("Assigned Subnetwork", 0x8002)}
	arp1 := append([]vinesTestField(nil), arp0...)
	arp1[0] = vinesU8("ARP Version", 1)
	arp1 = append(arp1, vinesU32("ARP Sequence Number", 0x1020304), vinesU16("Interface Metric", 10))
	arpQuery := []vinesTestField{vinesU8("ARP Version", 1), vinesU8("ARP Packet Type", 0), vinesRaw("Inactive Address Bytes", make([]byte, 6)), vinesU32("ARP Sequence Number", 0), vinesU16("Interface Metric", 0)}
	rtp := []vinesTestField{vinesU8("RTP First Octet", 2), vinesU8("Node Type", 2), vinesU8("Controller Type", 0), vinesU8("Machine Type", 0), vinesU32("Route Network", 0x10203040), vinesU16("Neighbor Metric", 5), vinesU32("Route Network", 0x10203040), vinesU16("Neighbor Metric", 0xffff)}
	srtp := []vinesTestField{vinesU8("RTP First Octet", 0), vinesU8("RTP Version Low", 1), vinesU8("RTP Operation", 1), vinesU8("Node Type", 2), vinesU8("Compatibility Flags", 1), vinesU8("SRTP Reserved", 0)}
	srtpRequest := append(append([]vinesTestField(nil), srtp...), vinesU8("Requested Information", 1))
	srtpUpdate := append([]vinesTestField(nil), srtp...)
	srtpUpdate[2] = vinesU8("RTP Operation", 2)
	srtpUpdate = append(srtpUpdate, vinesU8("Information Type", 2), vinesU8("Update Control", 0x60), vinesU16("Packet ID", 0x1234), vinesU16("Data Offset", 0), vinesU32("Router Sequence Number", 0x12345678), vinesU16("Router Metric", 5), vinesU32("Route Network", 0x50607080), vinesU16("Neighbor Metric", 6), vinesU32("Route Sequence Number", 0x1020304), vinesU8("Network Flags", 0), vinesU8("Route Reserved", 0))
	srtpInit := append([]vinesTestField(nil), srtp...)
	srtpInit[2] = vinesU8("RTP Operation", 4)
	quote := (vinesTestFixture{protocol: 0xee, control: 15}).wire()
	icp := []vinesTestField{vinesU16("ICP Packet Type", 1), vinesU16("Notification Metric", 5), vinesRaw("Quoted Packet Bytes", quote)}
	fixtures := []vinesTestFixture{
		{name: "ipc-short", protocol: 1, control: 15, body: short, info: map[string]map[string]any{"IPC": {"Header Bytes": 6, "Known Packet Type": true, "Message Length Validated": false}}},
		{name: "ipc-fragment", protocol: 1, control: 0x2f, body: long, info: map[string]map[string]any{"IPC": {"Header Bytes": 16, "Known Packet Type": true, "Message Length Validated": false}}},
		{name: "ipc-error", protocol: 1, control: 0x1e, body: ipcError},
		{name: "spp-ack", protocol: 2, control: 0x4d, body: spp, info: map[string]map[string]any{"SPP": {"Header Bytes": 16, "Known Packet Type": true}}},
		{name: "arp-assignment", protocol: 4, control: 15, body: arp0, info: map[string]map[string]any{"ARP": {"Body Layout Decoded": true, "Legacy Opcode Active": true, "Legacy Opcode": 3}}},
		{name: "sarp-assignment", protocol: 4, control: 15, body: arp1, info: map[string]map[string]any{"ARP": {"Body Layout Decoded": true, "Legacy Opcode Active": false}}},
		{name: "sarp-query", protocol: 4, control: 0x7f, broadcast: true, body: arpQuery},
		{name: "rtp-update", protocol: 5, control: 0x3f, broadcast: true, body: rtp, info: map[string]map[string]any{"RTP": {"Sequenced": false, "Operation": 2, "Body Layout Decoded": true, "Routing State Validated": false}}},
		{name: "srtp-request", protocol: 5, control: 15, body: srtpRequest, info: map[string]map[string]any{"RTP": {"Sequenced": true, "Operation": 1, "Body Layout Decoded": true}}},
		{name: "srtp-update", protocol: 5, control: 15, body: srtpUpdate},
		{name: "srtp-reinitialize", protocol: 5, control: 15, body: srtpInit},
		{name: "icp-metric", protocol: 6, control: 15, body: icp, info: map[string]map[string]any{"ICP": {"Known Packet Type": true, "Quoted Packet Decoded": false, "Quote Extent Validated": false}}},
	}
	return fixtures
}

func vinesTestFields(t *testing.T, node *base.Node, f vinesTestFixture, offset uint64) {
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
	fields := f.fields()
	require.Len(t, leaves, len(fields))
	pos := offset
	for index, field := range fields {
		leaf := leaves[index]
		require.Equal(t, field.name, leaf.Name)
		value, err := leaf.Result()
		require.NoError(t, err)
		require.Same(t, leaf, value.Origin)
		require.Equal(t, field.value, value.Value, field.name)
		typ := map[string]string{"uint8": "uint8", "uint16": "uint16", "uint32": "uint32", "[]uint8": "raw"}[fmt.Sprintf("%T", field.value)]
		require.Equal(t, typ, leaf.Cfg.GetItem(base.CfgType), field.name)
		require.Equal(t, [2]uint64{pos, pos + field.bits}, stream_parser.GetNodeResultPos(leaf), field.name)
		pos += field.bits
	}
	require.Equal(t, offset+uint64(len(f.wire()))*8, pos)
	packet := leaves[0].Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, packet)
	require.Equal(t, "VINES VIP bounded field layout", info["Profile"])
	require.EqualValues(t, 1500, info["Maximum Packet Bytes"])
	require.Equal(t, f.broadcast, info["Broadcast Destination"])
	require.EqualValues(t, (f.control>>4)&7, info["Control Flags Raw"])
	interpretation := "unicast"
	if f.broadcast {
		interpretation = "broadcast class"
	}
	require.Equal(t, interpretation, info["Control Flags Interpretation"])
	require.Equal(t, f.protocol == 1 || f.protocol == 2 || f.protocol == 4 || f.protocol == 5 || f.protocol == 6, info["Upper Header Decoded"])
	for _, key := range []string{"Checksum Validated", "Application Body Decoded", "Reassembled", "Session State Validated"} {
		require.Equal(t, false, info[key], key)
	}
	for name, expectations := range f.info {
		info := alljoynTestInfo(t, protocolCorpusFindNode(node, name))
		for key, want := range expectations {
			require.EqualValues(t, want, info[key], name+"/"+key)
		}
	}
}

func TestProtocolCorpusVINESFields(t *testing.T) {
	for _, f := range vinesTestFixtures() {
		t.Run(f.name, func(t *testing.T) {
			t.Logf("companion %s %d %x", f.name, len(f.wire()), f.wire())
			for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "vines", entry)
				require.Equal(t, f.wire(), NodeToBytes(node))
				vinesTestFields(t, node, f, 0)
			}
		})
	}
}

func vinesTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "vines", "VINESVIP")
	require.Error(t, err)
	if len(wire) == 0 {
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "vines", "VINESCarrier")
		require.Error(t, err)
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "vines", "VINESCarrier")
	miopTestField(t, node, "Unparsed VINES Packet", wire, 0, uint64(len(wire))*8)
	require.Equal(t, wire, NodeToBytes(node))
	for _, name := range []string{"Checksum", "Protocol", "IPC", "SPP", "ARP", "RTP", "ICP"} {
		require.Nil(t, protocolCorpusFindNode(node, name), name)
	}
}

func TestProtocolCorpusVINESOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-vines.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "cd8a23eaafbb50af4bd960ce0135b762923ee78c56a1178a2686946155704438", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 30)
		require.Equal(t, []byte{0x0b, 0xad}, frame[12:14])
		require.Equal(t, make([]byte, 16), frame[14:])
		vinesTestReject(t, frame[14:])
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
	}
}

func TestProtocolCorpusVINESCompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-vines-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "5e28f5e857531f4f63e432922d30a0d5a8f438b6ad02f716f9084e893a649489", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	fixtures := vinesTestFixtures()
	require.Len(t, frames, len(fixtures))
	for index, frame := range frames {
		f := fixtures[index]
		t.Run(f.name, func(t *testing.T) {
			require.Len(t, frame, 14+len(f.wire()))
			require.Equal(t, []byte{0x0b, 0xad}, frame[12:14])
			require.Equal(t, f.wire(), frame[14:])
			for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, frame[14:], "vines", entry)
				vinesTestFields(t, node, f, 0)
				require.Equal(t, frame[14:], NodeToBytes(node))
			}
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			checksum := protocolCorpusFindNode(node, "Checksum")
			require.NotNil(t, checksum)
			packet := checksum.Cfg.GetItem(base.CfgParent).(*base.Node)
			vinesTestFields(t, packet, f, 14*8)
			require.Equal(t, frame, NodeToBytes(node))
		})
	}
}

func TestProtocolCorpusVINESPrefixesAndBoundaries(t *testing.T) {
	for _, f := range vinesTestFixtures() {
		wire := f.wire()
		for cut := 0; cut < len(wire); cut++ {
			vinesTestReject(t, wire[:cut])
		}
		vinesTestReject(t, append(bytes.Clone(wire), 0))
		for _, length := range []uint16{0, 17, uint16(len(wire) - 1), uint16(len(wire) + 1), 0xffff} {
			bad := bytes.Clone(wire)
			binary.BigEndian.PutUint16(bad[2:], length)
			vinesTestReject(t, bad)
		}
	}
	// These mutations also update VIP length, exercising lower-layer bounds,
	// not merely the outer exact-length check.
	for _, index := range []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11} {
		wire := vinesTestFixtures()[index].wire()
		minimum := map[int]int{0: 6, 1: 16, 3: 16, 4: 8, 5: 14, 6: 14, 7: 4, 8: 7, 9: 18, 10: 6, 11: 4}[index]
		bad := bytes.Clone(wire[:18+minimum-1])
		binary.BigEndian.PutUint16(bad[2:], uint16(len(bad)))
		vinesTestReject(t, bad)
	}
	for _, index := range []int{4, 5, 6, 7, 8, 9, 10} {
		bad := append(bytes.Clone(vinesTestFixtures()[index].wire()), 0)
		binary.BigEndian.PutUint16(bad[2:], uint16(len(bad)))
		vinesTestReject(t, bad)
	}
}

func TestProtocolCorpusVINESOpaqueAndCompatibility(t *testing.T) {
	fixtures := []vinesTestFixture{
		{name: "unknown-vip", protocol: 0xee, control: 0xff, body: []vinesTestField{vinesRaw("Uninterpreted VIP Body", []byte{0xff, 0, 1})}},
		{name: "empty-unknown-vip", protocol: 0xee},
		{name: "unknown-arp", protocol: 4, body: []vinesTestField{vinesU8("ARP Version", 2), vinesU8("ARP Packet Type", 0xfe), vinesRaw("Uninterpreted ARP Body", []byte{0xa5})}, info: map[string]map[string]any{"ARP": {"Body Layout Decoded": false}}},
		{name: "legacy-arp-query", protocol: 4, body: []vinesTestField{vinesU8("ARP Version", 0), vinesU8("ARP Packet Type", 0), vinesRaw("Uninterpreted ARP Body", []byte{1, 2})}},
		{name: "rtp-redirect-opaque", protocol: 5, body: []vinesTestField{vinesU8("RTP First Octet", 3), vinesU8("Node Type", 2), vinesU8("Controller Type", 0), vinesU8("Machine Type", 0), vinesRaw("Uninterpreted RTP Body", []byte{0xde, 0xad})}, info: map[string]map[string]any{"RTP": {"Body Layout Decoded": false}}},
		{name: "unknown-icp", protocol: 6, body: []vinesTestField{vinesU16("ICP Packet Type", 0xfe), vinesU16("Uninterpreted Notification Value", 0xffff), vinesRaw("Quoted Packet Bytes", []byte{1})}, info: map[string]map[string]any{"ICP": {"Known Packet Type": false, "Quote Extent Validated": false}}},
	}
	padding := vinesTestFixtures()[0]
	padding.body = append([]vinesTestField(nil), padding.body...)
	padding.body[3] = vinesU8("Datagram Padding", 0xa5)
	fixtures = append(fixtures, padding)
	for _, f := range fixtures {
		for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "vines", entry)
			vinesTestFields(t, node, f, 0)
			require.Equal(t, f.wire(), NodeToBytes(node))
		}
	}
	// Integrity is explicitly not inferred from a checksum value.
	for _, checksum := range []uint16{0, 1, 0x1234, 0xffff} {
		wire := vinesTestFixtures()[0].wire()
		binary.BigEndian.PutUint16(wire, checksum)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "vines", "VINESVIP")
		miopTestField(t, node, "Checksum", checksum, 0, 16)
		require.Equal(t, wire, NodeToBytes(node))
	}
}

func TestProtocolCorpusVINESResourcesAndEntryBoundary(t *testing.T) {
	f := vinesTestFixture{protocol: 0xee, body: []vinesTestField{vinesRaw("Uninterpreted VIP Body", bytes.Repeat([]byte{0x95}, 1482))}}
	for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
		node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "vines", entry)
		vinesTestFields(t, node, f, 0)
		require.Equal(t, f.wire(), NodeToBytes(node))
		_, err := parser.ParseBinary(bytes.NewReader(f.wire()), "vines", entry)
		require.ErrorContains(t, err, "explicit packet boundary")
		_, err = parser.GenerateBinary(map[string]any{}, "vines", entry)
		require.ErrorContains(t, err, "explicit packet boundary")
		for bits := uint64(1); bits < 8; bits++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(f.wire()), uint64(len(f.wire()))*8 - bits}
			_, err := parser.ParseBinary(reader, "vines", entry)
			require.ErrorContains(t, err, "explicit byte boundary")
			require.Equal(t, len(f.wire()), reader.Len())
		}
	}
	f.body[0] = vinesRaw("Uninterpreted VIP Body", bytes.Repeat([]byte{0x95}, 1483))
	reader := newProtocolCorpusBoundedReader(f.wire())
	_, err := parser.ParseBinary(reader, "vines", "VINESVIP")
	require.ErrorContains(t, err, "implementation profile")
	require.Equal(t, 1501, reader.Len())
	vinesTestReject(t, f.wire())
	// Maximum complete legacy routing list; repeated networks remain separate.
	routes := vinesTestFixtures()[7]
	routes.body = routes.body[:4]
	for i := 0; i < 246; i++ {
		routes.body = append(routes.body, vinesU32("Route Network", uint32(i%3)), vinesU16("Neighbor Metric", uint16(i)))
	}
	node := protocolCorpusRequireBoundedRuleParse(t, routes.wire(), "vines", "VINESVIP")
	vinesTestFields(t, node, routes, 0)
	require.Equal(t, routes.wire(), NodeToBytes(node))
}

func TestProtocolCorpusVINESImportOffsetsAndRollback(t *testing.T) {
	for _, f := range vinesTestFixtures() {
		for offset := 0; offset < 8; offset++ {
			for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
				for _, valid := range []bool{true, false} {
					if !valid && entry == "VINESVIP" {
						continue
					}
					wire := f.wire()
					if !valid {
						wire[3]++
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
%s    Record: "import:vines.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, entry, padding))
					input := make([]byte, (offset+len(wire)*8+8+7)/8)
					for bit := 0; bit < offset; bit++ {
						input[bit/8] |= 1 << (7 - bit%8)
					}
					for index, octet := range append(bytes.Clone(wire), 0x5a) {
						for bit := 0; bit < 8; bit++ {
							pos := offset + index*8 + bit
							input[pos/8] |= ((octet >> (7 - bit)) & 1) << (7 - pos%8)
						}
					}
					root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
					reader := base.NewBitReader(bytes.NewReader(input))
					require.NoError(t, reader.Backup())
					require.NoError(t, root.ParseSubNode(reader, "Envelope"))
					node := base.GetNodeByPath(root, "@Envelope")
					record := protocolCorpusFindNode(node, "Record")
					if valid {
						vinesTestFields(t, record, f, uint64(offset))
					} else {
						miopTestField(t, record, "Unparsed VINES Packet", wire, uint64(offset), uint64(offset+len(wire)*8))
						require.Nil(t, protocolCorpusFindNode(record, "Checksum"))
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
}

func TestProtocolCorpusVINESHeldReaderAndIsolation(t *testing.T) {
	f := vinesTestFixtures()[1]
	for _, valid := range []bool{true, false} {
		for _, rollback := range []bool{true, false} {
			wire := f.wire()
			if !valid {
				wire[5] = 4
				wire[18] = 1
				wire[19] = 3
			} // Late known ARP length failure after VIP fields.
			reader := base.NewBitReader(bytes.NewReader(append(append([]byte{0xa5}, wire...), 0x5a)))
			_, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("vines.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "VINESCarrier"))
			node := base.GetNodeByPath(root, "@VINESCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				vinesTestFields(t, node, f, 0)
			} else {
				require.Nil(t, protocolCorpusFindNode(node, "Checksum"))
				protocolCorpusRequireValue(t, node, "Unparsed VINES Packet", wire)
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
	for _, entry := range []string{"VINESVIP", "VINESCarrier"} {
		for cut := 0; cut < len(f.wire()); cut++ {
			root, err := base.ParseRule("vines.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(f.wire()))*8)
			reader := base.NewBitReader(bytes.NewReader(f.wire()[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery())
			if cut > 0 {
				replay, err := reader.ReadBits(uint64(cut) * 8)
				require.NoError(t, err)
				require.Equal(t, f.wire()[:cut], replay)
			}
		}
	}
	var wait sync.WaitGroup
	errors := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			wire := vinesTestFixtures()[index].wire()
			valid := index%2 == 0
			if !valid {
				wire[3]++
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "vines", "VINESCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Checksum") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed VINES Packet") != nil) == valid) {
				err = fmt.Errorf("VINES context leaked at %d", index)
			}
			errors <- err
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
