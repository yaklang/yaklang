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
)

const sebekCorpusRule = "application-layer.sebek"

type sebekTestFixture struct {
	version, kind                                     uint16
	magic, counter, seconds, micros, parent, pid, uid uint32
	fd, inode                                         uint32
	command                                           string
	data                                              []byte
}

// Independent field serialization from the pinned layouts documented in the
// rule; neither parser output nor GenerateBinary supplies the byte oracle.
func (f sebekTestFixture) wire() []byte {
	var b bytes.Buffer
	put := func(v any) { _ = binary.Write(&b, binary.BigEndian, v) }
	put(f.magic)
	put(f.version)
	put(f.kind)
	for _, v := range []uint32{f.counter, f.seconds, f.micros} {
		put(v)
	}
	if f.version == 3 {
		put(f.parent)
	}
	for _, v := range []uint32{f.pid, f.uid, f.fd} {
		put(v)
	}
	if f.version == 3 {
		put(f.inode)
	}
	command := make([]byte, 12)
	copy(command, f.command)
	b.Write(command)
	put(uint32(len(f.data)))
	b.Write(f.data)
	return b.Bytes()
}

func sebekTestFixtures() []sebekTestFixture {
	f := sebekTestFixture{version: 2, kind: 1, magic: 0xa1b2c3d4, counter: 0x01020304, seconds: 0x10203040, micros: 123456,
		parent: 7, pid: 8, uid: 9, fd: 10, inode: 11, command: "record-label", data: []byte{0x41, 0, 0xff}}
	v3 := f
	v3.version, v3.kind = 3, 2
	// Packed DIP, DPORT, SIP, SPORT, CALL, PROTO, exactly 15 bytes.
	v3.data = []byte{192, 0, 2, 20, 0x1f, 0x90, 198, 51, 100, 10, 0xc0, 1, 0, 3, 17}
	unknown := f
	unknown.version, unknown.kind, unknown.magic, unknown.micros = 3, 0xffff, 0, 0xffffffff
	unknown.command, unknown.data = "\x00\xfflabel\x00abcd", []byte{0xff, 0, 0x80, 1}
	empty := f
	empty.version, empty.kind, empty.data = 3, 0, nil
	v2type2 := f
	v2type2.kind, v2type2.data = 2, []byte{0x91}
	return []sebekTestFixture{f, v3, unknown, empty, v2type2}
}

func sebekTestFields(t *testing.T, node *base.Node, f sebekTestFixture, offset uint64) {
	t.Helper()
	bit := offset
	field := func(name string, value any, size uint64) {
		miopTestField(t, node, name, value, bit, bit+size*8)
		bit += size * 8
	}
	field("Magic", f.magic, 4)
	field("Version", f.version, 2)
	field("Record Type", f.kind, 2)
	field("Counter", f.counter, 4)
	field("Time Seconds", f.seconds, 4)
	field("Time Microseconds", f.micros, 4)
	if f.version == 3 {
		field("Parent Process ID", f.parent, 4)
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Parent Process ID"))
	}
	field("Process ID", f.pid, 4)
	field("User ID", f.uid, 4)
	field("File Descriptor", f.fd, 4)
	if f.version == 3 {
		field("Inode", f.inode, 4)
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Inode"))
	}
	command := make([]byte, 12)
	copy(command, f.command)
	field("Command Name", string(command), 12)
	field("Data Length", uint32(len(f.data)), 4)
	headerLength := (bit - offset) / 8
	endpoint := f.version == 3 && f.kind == 2
	if endpoint {
		require.Len(t, f.data, 15)
		field("Destination Address", f.data[:4], 4)
		field("Destination Port", binary.BigEndian.Uint16(f.data[4:6]), 2)
		field("Source Address", f.data[6:10], 4)
		field("Source Port", binary.BigEndian.Uint16(f.data[10:12]), 2)
		field("Call ID", binary.BigEndian.Uint16(f.data[12:14]), 2)
		field("IP Protocol", f.data[14], 1)
		require.Nil(t, protocolCorpusFindNode(node, "Record Data"))
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "IPv4 Endpoint Record"))
		if len(f.data) > 0 {
			field("Record Data", f.data, uint64(len(f.data)))
		} else {
			require.Nil(t, protocolCorpusFindNode(node, "Record Data"))
		}
	}
	require.Equal(t, offset+uint64(len(f.wire()))*8, bit)
	record := protocolCorpusFindNode(node, "Magic").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, record)
	require.Equal(t, "SEBEK v2/v3 bounded binary record", info["Profile"])
	require.EqualValues(t, headerLength, info["Header Bytes"])
	require.EqualValues(t, 65527, info["Maximum Record Bytes"])
	require.Equal(t, endpoint, info["IPv4 Endpoint Fields Decoded"])
	for _, key := range []string{"Configured Magic Validated", "Transport Context Validated", "Body Application Semantics Decoded", "Cross-Record Consistency Validated", "Timestamp Normalized"} {
		require.Equal(t, false, info[key], key)
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

func TestProtocolCorpusSEBEKFields(t *testing.T) {
	for index, f := range sebekTestFixtures() {
		for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
			t.Run(fmt.Sprintf("%d/%s", index, entry), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), sebekCorpusRule, entry)
				require.Equal(t, f.wire(), NodeToBytes(node))
				sebekTestFields(t, node, f, 0)
			})
		}
	}
}

func TestProtocolCorpusSEBEKCompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-sebek-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "c20b228ffec5721fe7ba63fdd324305e7d32289dba96915acc10a171efe725af", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 2)
	literals := []string{
		"a1b2c3d40002000101020304102030400001e24000000008000000090000000a7265636f72642d6c6162656c000000034100ff",
		"a1b2c3d40003000201020304102030400001e2400000000700000008000000090000000a0000000b7265636f72642d6c6162656c0000000fc00002141f90c633640ac001000311",
	}
	for index, frame := range frames {
		f := sebekTestFixtures()[index]
		literal := alljoynTestHex(t, literals[index])
		require.Equal(t, literal, f.wire())
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, literal, udp.Payload)
		require.Len(t, frame, 42+len(literal))
		for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, sebekCorpusRule, entry)
			require.Equal(t, literal, NodeToBytes(node))
			sebekTestFields(t, node, f, 0)
			// Explicit selection inside an Ethernet-sized envelope verifies the
			// actual record's global bit spans without adding port dispatch.
			root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      this.ProcessSubNode("Transport Headers")
      this.GetSubNode("Record").SetMaxLength(%d)
      this.ProcessSubNode("Record")
    Transport Headers: raw,42
    Record: "import:application-layer/sebek.yaml;node:%s"
`, len(literal), entry))
			root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
			envelope := base.GetNodeByPath(root, "@Envelope")
			require.Equal(t, frame, NodeToBytes(envelope))
			sebekTestFields(t, protocolCorpusFindNode(envelope, "Record"), f, 42*8)
		}
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
	}
}

func TestProtocolCorpusSEBEKOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-sebek.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "cb31dabb716b3fa48f2d0ced4c847a1c1454aa999128fe8d4fdc71d114334484", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	f := sebekTestFixture{version: 3, magic: 0xd0d0d0d0, command: "bash", data: []byte{0x6c, 0x73, 0x0a}}
	for _, frame := range frames {
		require.Len(t, frame, 101)
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, layers.UDPPort(40101), udp.SrcPort)
		require.Equal(t, layers.UDPPort(1101), udp.DstPort)
		require.Equal(t, f.wire(), udp.Payload)
		for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, udp.Payload, sebekCorpusRule, entry)
			require.Equal(t, udp.Payload, NodeToBytes(node))
			sebekTestFields(t, node, f, 0)
		}
		// Preserve generic Ethernet/UDP parsing; explicit protocol selection does
		// not infer record identity from a configurable default port.
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
	}
}

func sebekTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), sebekCorpusRule, "SEBEK")
	require.Error(t, err)
	if len(wire) == 0 {
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), sebekCorpusRule, "SEBEKCarrier")
		require.Error(t, err)
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, sebekCorpusRule, "SEBEKCarrier")
	miopTestField(t, node, "Unparsed SEBEK Record", wire, 0, uint64(len(wire))*8)
	require.Equal(t, wire, NodeToBytes(node))
	for _, name := range []string{"Magic", "Version", "Data Length", "IPv4 Endpoint Record", "Record Data"} {
		require.Nil(t, protocolCorpusFindNode(node, name), name)
	}
}

func TestProtocolCorpusSEBEKEveryShortPrefixAndInvalidLength(t *testing.T) {
	for _, f := range sebekTestFixtures() {
		wire := f.wire()
		for cut := 0; cut < len(wire); cut++ {
			sebekTestReject(t, wire[:cut])
		}
		sebekTestReject(t, append(bytes.Clone(wire), 0))
		lengthOffset := 44
		if f.version == 3 {
			lengthOffset = 52
		}
		for _, length := range []uint32{uint32(len(f.data)) + 1, 0xffffffff} {
			bad := bytes.Clone(wire)
			binary.BigEndian.PutUint32(bad[lengthOffset:], length)
			sebekTestReject(t, bad)
		}
		if len(f.data) > 0 {
			bad := bytes.Clone(wire)
			binary.BigEndian.PutUint32(bad[lengthOffset:], uint32(len(f.data)-1))
			sebekTestReject(t, bad)
		}
	}
	wire := sebekTestFixtures()[1].wire()
	for _, version := range []uint16{0, 1, 4, 0xffff} {
		bad := bytes.Clone(wire)
		binary.BigEndian.PutUint16(bad[4:], version)
		sebekTestReject(t, bad)
	}
	for _, size := range []int{0, 1, 14, 16, 32} {
		f := sebekTestFixtures()[1]
		f.data = bytes.Repeat([]byte{0xa5}, size)
		sebekTestReject(t, f.wire())
	}
}

func TestProtocolCorpusSEBEKResourceAndBoundary(t *testing.T) {
	for _, version := range []uint16{2, 3} {
		f := sebekTestFixtures()[0]
		f.version = version
		header := 48
		if version == 3 {
			header = 56
		}
		f.data = bytes.Repeat([]byte{0x93}, 65527-header)
		wire := f.wire()
		require.Len(t, wire, 65527)
		for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, sebekCorpusRule, entry)
			sebekTestFields(t, node, f, 0)
			require.Equal(t, wire, NodeToBytes(node))
			_, err := parser.ParseBinary(bytes.NewReader(wire), sebekCorpusRule, entry)
			require.ErrorContains(t, err, "explicit record boundary")
			_, err = parser.GenerateBinary(map[string]any{}, sebekCorpusRule, entry)
			require.ErrorContains(t, err, "explicit record boundary")
			for extra := uint64(1); extra < 8; extra++ {
				reader := &alljoynTestBitBoundaryReader{bytes.NewReader(wire), uint64(len(wire))*8 - extra}
				_, err := parser.ParseBinary(reader, sebekCorpusRule, entry)
				require.ErrorContains(t, err, "explicit byte boundary")
				require.Equal(t, len(wire), reader.Len())
			}
		}
		f.data = append(f.data, 0)
		tooLong := newProtocolCorpusBoundedReader(f.wire())
		_, err := parser.ParseBinary(tooLong, sebekCorpusRule, "SEBEK")
		require.ErrorContains(t, err, "implementation profile")
		require.Equal(t, 65528, tooLong.Len())
		sebekTestReject(t, f.wire())
	}
}

func TestProtocolCorpusSEBEKHeldReaderAndConcurrentIsolation(t *testing.T) {
	f := sebekTestFixtures()[1]
	for _, valid := range []bool{false, true} {
		for _, rollback := range []bool{false, true} {
			wire := f.wire()
			if !valid {
				wire[55]++ // Reject only after the complete header has been emitted.
			}
			input := append(append([]byte{0xa5}, wire...), 0x5a)
			reader := base.NewBitReader(bytes.NewReader(input))
			_, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.NoError(t, reader.Backup())
			root, err := base.ParseRule("application-layer/sebek.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(reader, "SEBEKCarrier"))
			node := base.GetNodeByPath(root, "@SEBEKCarrier")
			require.Equal(t, wire, NodeToBytes(node))
			if valid {
				sebekTestFields(t, node, f, 0)
			} else {
				protocolCorpusRequireValue(t, node, "Unparsed SEBEK Record", wire)
				require.Nil(t, protocolCorpusFindNode(node, "Magic"))
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
			wire := sebekTestFixtures()[index%5].wire()
			valid := index%2 == 0
			if !valid {
				wire[5] = 1
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), sebekCorpusRule, "SEBEKCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Magic") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed SEBEK Record") != nil) == valid) {
				err = fmt.Errorf("SEBEK trial state leaked at %d", index)
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

func TestProtocolCorpusSEBEKGlobalOffsetsAndPhysicalTruncation(t *testing.T) {
	for _, f := range sebekTestFixtures()[:2] {
		wire := f.wire()
		for offset := 0; offset < 8; offset++ {
			for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
				for _, valid := range []bool{true, false} {
					if !valid && entry == "SEBEK" {
						continue
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
%s    Record: "import:application-layer/sebek.yaml;node:%s"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, entry, padding))
					payload := bytes.Clone(wire)
					if !valid {
						payload[len(wire)-len(f.data)-1]++
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
					record := protocolCorpusFindNode(node, "Record")
					if valid {
						sebekTestFields(t, record, f, uint64(offset))
					} else {
						miopTestField(t, record, "Unparsed SEBEK Record", payload, uint64(offset), uint64(offset+len(wire)*8))
						require.Nil(t, protocolCorpusFindNode(record, "Magic"))
					}
					miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
					require.Equal(t, input, NodeToBytes(node))
					require.NoError(t, reader.Recovery())
					replayed, err := reader.ReadBits(uint64(len(input)) * 8)
					require.NoError(t, err)
					require.Equal(t, input, replayed)
					require.ErrorContains(t, reader.Recovery(), "no backup")
				}
			}
		}
		for _, entry := range []string{"SEBEK", "SEBEKCarrier"} {
			for cut := 0; cut < len(wire); cut++ {
				root, err := base.ParseRule("application-layer/sebek.yaml")
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
				reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
				require.NoError(t, reader.Backup())
				require.Error(t, root.ParseSubNode(reader, entry))
				require.NoError(t, reader.Recovery())
				if cut > 0 {
					replayed, err := reader.ReadBits(uint64(cut) * 8)
					require.NoError(t, err)
					require.Equal(t, wire[:cut], replayed)
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
	}
}
