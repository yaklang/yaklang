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

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const miopCorpusRule = "application-layer.miop"

type miopTestFixture struct {
	little, stop       bool
	number, count      uint32
	uid, padding, body []byte
}

func (f miopTestFixture) wire() []byte {
	header := make([]byte, 20)
	copy(header, "MIOP")
	header[4] = 0x10
	var order binary.ByteOrder = binary.BigEndian
	if f.little {
		header[5] |= 1
		order = binary.LittleEndian
	}
	if f.stop {
		header[5] |= 2
	}
	order.PutUint16(header[6:], uint16(len(f.body)))
	order.PutUint32(header[8:], f.number)
	order.PutUint32(header[12:], f.count)
	order.PutUint32(header[16:], uint32(len(f.uid)))
	header = append(header, f.uid...)
	header = append(header, f.padding...)
	return append(header, f.body...)
}

// Independent wire construction from CORBA 3.2 Part 2, 11.1.4. The complete
// packet fixtures contain GIOP 1.2 Fragment PDUs, not a complete invocation:
// request ID 42, opaque fragment bytes dead, with independent GIOP byte order.
// UID uniqueness, associated requests and UIPMC dispatch are not asserted.
func miopTestFixtures(t *testing.T) []miopTestFixture {
	return []miopTestFixture{
		{stop: true, count: 1, padding: []byte{0xf1, 0xf2, 0xf3, 0xf4}, body: alljoynTestHex(t, "47494f5001020007000000060000002adead")},
		{little: true, stop: true, count: 0, uid: []byte{0xaa, 0xbb, 0xcc}, padding: []byte{0x7f}, body: alljoynTestHex(t, "47494f5001020007000000060000002adead")},
		{stop: true, count: 1, uid: []byte{1, 2, 3, 4}, body: alljoynTestHex(t, "47494f5001020107060000002a000000dead")},
		{number: 1, count: 3, uid: []byte{1, 2}, padding: []byte{0x81, 0x82}, body: []byte{0xff, 0, 0x7f}},
		{little: true, stop: true, number: 2, count: 3, padding: []byte{1, 2, 3, 4}, body: []byte{0x33}},
		{stop: true, number: 0xffffffff, count: 0, uid: bytes.Repeat([]byte{0xab}, 252)},
	}
}

func miopTestField(t *testing.T, root *base.Node, name string, want any, start, end uint64) {
	t.Helper()
	node := protocolCorpusFindNode(root, name)
	require.NotNil(t, node, name)
	value, err := node.Result()
	require.NoError(t, err)
	require.Same(t, node, value.Origin)
	require.Equal(t, want, value.Value, name)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(node), name)
	typ := map[string]string{"string": "string", "[]uint8": "raw", "uint8": "uint8", "uint16": "uint16", "uint32": "uint32"}[fmt.Sprintf("%T", want)]
	require.NotEmpty(t, typ)
	require.Equal(t, typ, node.Cfg.GetItem(base.CfgType), name)
}

func miopTestOuter(t *testing.T, node *base.Node, f miopTestFixture, offset uint64) {
	t.Helper()
	field := func(name string, value any, start, end uint64) {
		miopTestField(t, node, name, value, offset+start, offset+end)
	}
	field("MIOP Magic", "MIOP", 0, 32)
	field("MIOP Major", uint8(1), 32, 36)
	field("MIOP Minor", uint8(0), 36, 40)
	field("Reserved Flags", uint8(0), 40, 46)
	bit := func(value bool) uint8 {
		if value {
			return 1
		}
		return 0
	}
	field("Stop Flag", bit(f.stop), 46, 47)
	field("Little Endian Flag", bit(f.little), 47, 48)
	field("Packet Length", uint16(len(f.body)), 48, 64)
	field("Packet Number", f.number, 64, 96)
	field("Number of Packets", f.count, 96, 128)
	field("Unique ID Length", uint32(len(f.uid)), 128, 160)
	end := uint64(160)
	if len(f.uid) > 0 {
		field("Unique ID", f.uid, end, end+uint64(len(f.uid))*8)
		end += uint64(len(f.uid)) * 8
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Unique ID"))
	}
	if len(f.padding) > 0 {
		field("Header Padding", f.padding, end, end+uint64(len(f.padding))*8)
		end += uint64(len(f.padding)) * 8
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Header Padding"))
	}
	single := f.number == 0 && f.stop && (f.count == 0 || f.count == 1)
	message := protocolCorpusFindNode(node, "MIOP Magic").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, message)
	order := "big"
	if f.little {
		order = "little"
	}
	require.Equal(t, order, info["Packet Byte Order"])
	require.EqualValues(t, end/8, info["Header Bytes"])
	require.Equal(t, single, info["Single Packet Collection"])
	require.Equal(t, f.count != 0, info["Declared Collection Count Known"])
	for _, key := range []string{"Packet Reassembly Performed", "Unique ID Uniqueness Validated", "Cross-Packet Consistency Validated", "UIPMC Profile Semantics Decoded"} {
		require.Equal(t, false, info[key], key)
	}
	for _, name := range []string{"Packet Length", "Packet Number", "Number of Packets", "Unique ID Length"} {
		require.Equal(t, order, protocolCorpusFindNode(node, name).Cfg.GetItem(base.CfgEndian), name)
	}
	if !single && len(f.body) > 0 {
		field("Packet Fragment Data", f.body, end, end+uint64(len(f.body))*8)
	}
	var walk func(*base.Node)
	walk = func(parent *base.Node) {
		for _, child := range parent.Children {
			require.Same(t, parent, child.Cfg.GetItem(base.CfgParent), child.Name)
			if protocolCorpusNodeHasResult(child) {
				value, err := child.Result()
				require.NoError(t, err)
				require.Same(t, child, value.Origin)
			}
			walk(child)
		}
	}
	walk(node)
}

func TestProtocolCorpusMIOPFields(t *testing.T) {
	for index, fixture := range miopTestFixtures(t) {
		for _, entry := range []string{"MIOP", "MIOPCarrier"} {
			t.Run(fmt.Sprintf("%d/%s", index, entry), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, fixture.wire(), miopCorpusRule, entry)
				require.Equal(t, fixture.wire(), NodeToBytes(node))
				miopTestOuter(t, node, fixture, 0)
				if index < 3 {
					giop := protocolCorpusFindNode(node, "GIOP")
					require.NotNil(t, giop)
					miopTestField(t, giop, "Magic", []byte("GIOP"), 192, 224)
					miopTestField(t, giop, "Major", uint8(1), 224, 232)
					miopTestField(t, giop, "Minor", uint8(2), 232, 240)
					miopTestField(t, giop, "Flags", uint8(index/2), 240, 248)
					miopTestField(t, giop, "Message Type", uint8(7), 248, 256)
					miopTestField(t, giop, "Message Size", uint32(6), 256, 288)
					miopTestField(t, giop, "Request ID", uint32(42), 288, 320)
					miopTestField(t, giop, "Fragment Data", []byte{0xde, 0xad}, 320, 336)
					require.Equal(t, false, alljoynTestInfo(t, giop)["Body Fields Validated"])
				} else {
					require.Nil(t, protocolCorpusFindNode(node, "GIOP"))
				}
			})
		}
	}
}

func TestProtocolCorpusMIOPOriginalEveryPacket(t *testing.T) {
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-corba.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "a4a0bfa2a212dc460da2a9ca01fa9c1fc562e7d24c7b2fd8d7424e6b85cd3289", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	count := 0
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		layer := packet.Layer(layers.LayerTypeUDP)
		if layer == nil {
			continue
		}
		udp := layer.(*layers.UDP)
		require.Equal(t, 19+count, index+1)
		require.Len(t, udp.Payload, 260)
		uid := make([]byte, 12)
		uid[0] = byte(count)
		f := miopTestFixture{little: true, stop: true, count: 1, uid: uid, body: udp.Payload[32:]}
		require.Equal(t, f.wire(), udp.Payload)
		for _, entry := range []string{"MIOP", "MIOPCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, miopCorpusRule, entry)
			require.Equal(t, udp.Payload, NodeToBytes(node))
			miopTestOuter(t, node, f, 0)
			giop := protocolCorpusFindNode(node, "GIOP")
			field := func(name string, value any, start, end uint64) {
				miopTestField(t, giop, name, value, (32+start)*8, (32+end)*8)
			}
			field("Magic", []byte("GIOP"), 0, 4)
			field("Major", uint8(1), 4, 5)
			field("Minor", uint8(2), 5, 6)
			field("Flags", uint8(1), 6, 7)
			field("Message Type", uint8(0), 7, 8)
			field("Message Size", uint32(216), 8, 12)
			field("Request ID", uint32(count), 12, 16)
			field("Response Flags", uint8(0), 16, 17)
			field("Reserved", []byte{0, 0, 0}, 17, 20)
			field("Addr Disc", uint16(1), 20, 22)
			field("Addr Pad", []byte{0, 0}, 22, 24)
			field("Profile ID", uint32(3), 24, 28)
			field("Profile Data Length", uint32(72), 28, 32)
			field("Profile Data", alljoynTestHex(t, "010100000c00000031302e39352e32382e343600703e00000100000027000000240000000101000009000000436f6e73756d65720000000000000000010000000000000000000000"), 32, 104)
			field("Op Len", uint32(20), 104, 108)
			field("Operation", "receiveReliableData\x00", 108, 128)
			field("Service Context Count", uint32(0), 128, 132)
			field("Body Padding", []byte{0, 0, 0, 0}, 132, 136)
			field("Stub Data", udp.Payload[32+136:], 136, 228)
		}
		count++
	}
	require.Equal(t, 10, count)
}

func miopTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), miopCorpusRule, "MIOP")
	require.Error(t, err)
	if len(wire) == 0 {
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, miopCorpusRule, "MIOPCarrier")
	protocolCorpusRequireValue(t, node, "Unparsed MIOP Payload", wire)
	require.Equal(t, wire, NodeToBytes(node))
	require.Nil(t, protocolCorpusFindNode(node, "MIOP Magic"))
}

func TestProtocolCorpusMIOPPrefixesAndRejects(t *testing.T) {
	fixtures := miopTestFixtures(t)
	for _, fixture := range fixtures {
		wire := fixture.wire()
		for cut := 0; cut < len(wire); cut++ {
			miopTestReject(t, wire[:cut])
		}
		miopTestReject(t, append(bytes.Clone(wire), 0))
	}
	for _, change := range []struct {
		pos   int
		value byte
	}{{0, 'X'}, {4, 0x11}, {4, 0x20}, {5, 0x82}, {6, 1}, {7, 0}, {11, 1}, {15, 2}, {19, 253}, {24, 'X'}, {31, 5}} {
		wire := fixtures[0].wire()
		wire[change.pos] = change.value
		miopTestReject(t, wire)
	}
	for _, fixture := range []miopTestFixture{
		{count: 1, padding: make([]byte, 4), body: []byte{1}},
		{stop: true, count: 3, padding: make([]byte, 4), body: []byte{1}},
		{number: 3, count: 3, padding: make([]byte, 4), body: []byte{1}},
		{stop: true, count: 1, padding: make([]byte, 4), body: alljoynTestHex(t, "47494f500102000500000000")},
		{stop: true, count: 1, padding: make([]byte, 4), body: alljoynTestHex(t, "47494f5001020002000000040000002a")},
	} {
		miopTestReject(t, fixture.wire())
	}
}

func TestProtocolCorpusMIOPResourceAndBoundary(t *testing.T) {
	fixture := miopTestFixture{little: true, stop: true, number: 1, count: 2, uid: bytes.Repeat([]byte{0x5a}, 252), body: bytes.Repeat([]byte{0xc3}, 65535)}
	wire := fixture.wire()
	require.Len(t, wire, 65807)
	for _, entry := range []string{"MIOP", "MIOPCarrier"} {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, miopCorpusRule, entry)
		miopTestOuter(t, node, fixture, 0)
		require.Equal(t, wire, NodeToBytes(node))
		_, err := parser.ParseBinary(bytes.NewReader(wire), miopCorpusRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, miopCorpusRule, entry)
		require.ErrorContains(t, err, "explicit")
		for extra := uint64(1); extra < 8; extra++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(wire), uint64(len(wire))*8 - extra}
			_, err := parser.ParseBinary(reader, miopCorpusRule, entry)
			require.ErrorContains(t, err, "explicit byte boundary")
			require.Equal(t, len(wire), reader.Len())
		}
	}
	tooLong := newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0))
	_, err := parser.ParseBinary(tooLong, miopCorpusRule, "MIOP")
	require.ErrorContains(t, err, "representable")
	require.Equal(t, 65808, tooLong.Len())
	miopTestReject(t, append(bytes.Clone(wire), 0))
}

func TestProtocolCorpusMIOPHeldReaderAndConcurrentIsolation(t *testing.T) {
	fixture := miopTestFixtures(t)[1]
	for _, valid := range []bool{false, true} {
		for _, rollback := range []bool{false, true} {
			wire := fixture.wire()
			if !valid {
				wire[19] = 0xff
			}
			input := append(append([]byte{0xa5}, wire...), 0x5a)
			reader := base.NewBitReader(bytes.NewReader(input))
			_, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("application-layer/miop.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "MIOPCarrier"))
			node := base.GetNodeByPath(root, "@MIOPCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				miopTestOuter(t, node, fixture, 0)
			} else {
				protocolCorpusRequireValue(t, node, "Unparsed MIOP Payload", wire)
			}
			if rollback {
				require.NoError(t, reader.Recovery())
				got, err := reader.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, got)
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
	var wait sync.WaitGroup
	errors := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			wire := fixture.wire()
			valid := index%2 == 0
			if !valid {
				wire[19] = 0xff
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), miopCorpusRule, "MIOPCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "MIOP Magic") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed MIOP Payload") != nil) == valid) {
				err = fmt.Errorf("MIOP trial state leaked at %d", index)
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

func TestProtocolCorpusMIOPGlobalOffsetsAndPhysicalTruncation(t *testing.T) {
	fixture := miopTestFixtures(t)[1] // Unknown count, 3-byte UID, packet/GIOP orders differ.
	wire := fixture.wire()
	for offset := 0; offset < 8; offset++ {
		for _, entry := range []string{"MIOP", "MIOPCarrier"} {
			for _, valid := range []bool{true, false} {
				if !valid && entry == "MIOP" {
					continue // Direct rejects are exercised by every shorter prefix.
				}
				t.Run(fmt.Sprintf("bits-%d/%s/valid-%t", offset, entry, valid), func(t *testing.T) {
					prefix, padding := "", ""
					if offset > 0 {
						prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
						padding = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
					}
					root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Packet").SetMaxLength(%d)
      this.ProcessSubNode("Packet")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Packet: "import:application-layer/miop.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, entry, padding))
					payload := bytes.Clone(wire)
					if !valid {
						payload[19] = 0xff // Fail after fields have been emitted, before UID allocation.
					}
					input := make([]byte, (offset+len(wire)*8+8+7)/8)
					for bit := 0; bit < offset; bit++ {
						input[bit/8] |= 1 << (7 - bit%8)
					}
					for index, octet := range append(payload, 0x5a) {
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
					packet := protocolCorpusFindNode(node, "Packet")
					require.NotNil(t, packet)
					if valid {
						miopTestOuter(t, packet, fixture, uint64(offset))
						miopTestField(t, packet, "Request ID", uint32(42), uint64(offset)+288, uint64(offset)+320)
						miopTestField(t, packet, "Fragment Data", []byte{0xde, 0xad}, uint64(offset)+320, uint64(offset)+336)
					} else {
						miopTestField(t, packet, "Unparsed MIOP Payload", payload, uint64(offset), uint64(offset+len(wire)*8))
						require.Nil(t, protocolCorpusFindNode(packet, "MIOP Magic"))
					}
					miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
					require.Equal(t, input, NodeToBytes(node))
					// Import save does not commit an enclosing reader transaction.
					require.NoError(t, reader.Recovery())
					replayed, err := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, err)
					require.Equal(t, input, replayed)
					require.ErrorContains(t, reader.Recovery(), "no backup")
				})
			}
		}
	}
	for _, entry := range []string{"MIOP", "MIOPCarrier"} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), miopCorpusRule, entry)
		require.Error(t, err)
		for _, cut := range []int{1, 19, 23, 35, len(wire) - 1} {
			root, err := base.ParseRule("application-layer/miop.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.NoError(t, reader.Backup())
			require.Error(t, root.ParseSubNode(reader, entry))
			require.NoError(t, reader.Recovery())
			replayed, err := reader.ReadBits(uint64(cut) * 8)
			require.NoError(t, err)
			require.Equal(t, wire[:cut], replayed)
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
}
