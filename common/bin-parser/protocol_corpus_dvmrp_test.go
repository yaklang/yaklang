package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Primary wire-format oracle: RFC 1075 sections 3 and 3.1-3.13.  The fixture
// encoder below is deliberately independent of the YAML rule.
// https://www.rfc-editor.org/rfc/rfc1075.html
//
// The field layout and the RFC's noted example typos were also cross-checked
// against this pinned Wireshark implementation:
// https://gitlab.com/wireshark/wireshark/-/blob/e3bedc57ba41dc1535f147bcb3cf97c5dc62fb75/epan/dissectors/packet-dvmrp.c
func dvmrpTestChecksum(wire []byte) uint16 {
	var sum uint32
	for i := 0; i < len(wire); i += 2 {
		sum += uint32(wire[i]) << 8
		if i+1 < len(wire) {
			sum += uint32(wire[i+1])
		}
	}
	for sum > 0xffff {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func dvmrpTestMessage(t *testing.T, subtype byte, commands ...byte) []byte {
	t.Helper()
	require.Zero(t, len(commands)%2, "RFC 1075 commands are 16-bit aligned")
	wire := append([]byte{0x13, subtype, 0, 0}, commands...)
	binary.BigEndian.PutUint16(wire[2:4], dvmrpTestChecksum(wire))
	return wire
}

func dvmrpTestEthernetIPv4(t *testing.T, wire, options []byte, fragment uint16) []byte {
	t.Helper()
	require.Zero(t, len(options)%4)
	require.LessOrEqual(t, len(options), 40)
	ip := []byte{byte(0x45 + len(options)/4), 0, 0, 0, 0, 1, 0, 0, 1, 2, 0, 0, 10, 0, 0, 1, 224, 0, 0, 4}
	ip = append(ip, options...)
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)+len(wire)))
	binary.BigEndian.PutUint16(ip[6:8], fragment)
	binary.BigEndian.PutUint16(ip[10:12], dvmrpTestChecksum(ip))
	frame := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}
	return append(append(frame, ip...), wire...)
}

func dvmrpTestEthernet(t *testing.T, wire []byte) []byte {
	t.Helper()
	return dvmrpTestEthernetIPv4(t, wire, nil, 0)
}

func dvmrpParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(wire)
	node, err := parser.ParseBinary(reader, "dvmrp", entry)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Equal(t, wire, NodeToBytes(node))
	require.Zero(t, reader.Len())
	return node
}

func dvmrpRequireSpan(t *testing.T, node *base.Node, name string, startBit, endBit uint64) {
	t.Helper()
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field, name)
	require.Equal(t, [2]uint64{startBit, endBit}, stream_parser.GetNodeResultPos(field), name)
}

func dvmrpCommand(t *testing.T, node *base.Node, kind uint64) *base.Node {
	t.Helper()
	for _, command := range protocolCorpusNodesNamed(node, "Command") {
		field := protocolCorpusFindNode(command, "Command Type")
		if field == nil {
			continue
		}
		value, err := field.Result()
		require.NoError(t, err)
		if uintVal(t, value) == kind {
			return command
		}
	}
	t.Fatalf("missing DVMRP command %d", kind)
	return nil
}

func dvmrpInfo(t *testing.T, node *base.Node, key string) any {
	t.Helper()
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, "missing DVMRP additionInfo")
	return info[key]
}

func dvmrpTestSubtypeMessages(t *testing.T) [][]byte {
	t.Helper()
	return [][]byte{
		dvmrpTestMessage(t, 1,
			2, 2, // AFI: IPv4.
			4, 2, // Metric.
			6, 16, // Infinity.
			3, 1, 255, 255, 255, 0, // Subnet mask.
			5, 0xc1, // Both defined flags plus an ignored future bit.
			7, 2, 128, 2, 251, 231, 128, 2, 236, 2, // Two routes.
		),
		dvmrpTestMessage(t, 2, 2, 2, 3, 0, 8, 2, 192, 0, 2, 0, 198, 51, 100, 0),
		dvmrpTestMessage(t, 3, 0, 0xaa, 2, 2, 5, 0xff, 9, 2,
			224, 2, 3, 1, 0, 0, 0, 20,
			224, 5, 4, 6, 0, 0, 0, 40,
		),
		dvmrpTestMessage(t, 4, 2, 2, 10, 2, 224, 7, 8, 5, 239, 1, 2, 3),
	}
}

func dvmrpRequireUintSpan(t *testing.T, node *base.Node, name string, want uint64, offset, start, end int) {
	t.Helper()
	protocolCorpusRequireValue(t, node, name, want)
	dvmrpRequireSpan(t, node, name, uint64(offset+start)*8, uint64(offset+end)*8)
}

func dvmrpRequireCommandType(t *testing.T, command *base.Node, kind uint64, offset, start int) {
	t.Helper()
	dvmrpRequireUintSpan(t, command, "Command Type", kind, offset, start, start+1)
}

// Assert every field materialized by the selected subtype, including each
// repeated address and each exact global bit span.  This is used both for the
// independent encodings and for every checked-in companion record.
func dvmrpRequireSubtypeFields(t *testing.T, node *base.Node, wire []byte, offset int) {
	t.Helper()
	subtype := uint64(wire[1])
	dvmrpRequireHeader(t, node, wire, offset, subtype)
	dvmrp := protocolCorpusFindNode(node, "Code").Cfg.GetItem(base.CfgParent).(*base.Node)

	switch subtype {
	case 1:
		afi := dvmrpCommand(t, node, 2)
		dvmrpRequireCommandType(t, afi, 2, offset, 4)
		dvmrpRequireUintSpan(t, afi, "Address Family", 2, offset, 5, 6)
		metric := dvmrpCommand(t, node, 4)
		dvmrpRequireCommandType(t, metric, 4, offset, 6)
		dvmrpRequireUintSpan(t, metric, "Metric", 2, offset, 7, 8)
		infinity := dvmrpCommand(t, node, 6)
		dvmrpRequireCommandType(t, infinity, 6, offset, 8)
		dvmrpRequireUintSpan(t, infinity, "Infinity", 16, offset, 9, 10)
		mask := dvmrpCommand(t, node, 3)
		dvmrpRequireCommandType(t, mask, 3, offset, 10)
		dvmrpRequireUintSpan(t, mask, "Count", 1, offset, 11, 12)
		dvmrpRequireUintSpan(t, mask, "Subnet Mask", 0xffffff00, offset, 12, 16)
		flags := dvmrpCommand(t, node, 5)
		dvmrpRequireCommandType(t, flags, 5, offset, 16)
		protocolCorpusRequireValue(t, flags, "Destination Unreachable", uint64(1))
		protocolCorpusRequireValue(t, flags, "Split Horizon Concealed", uint64(1))
		protocolCorpusRequireValue(t, flags, "Flags Reserved", uint64(1))
		flagStart := uint64(offset+17) * 8
		dvmrpRequireSpan(t, flags, "Destination Unreachable", flagStart, flagStart+1)
		dvmrpRequireSpan(t, flags, "Split Horizon Concealed", flagStart+1, flagStart+2)
		dvmrpRequireSpan(t, flags, "Flags Reserved", flagStart+2, flagStart+8)
		da := dvmrpCommand(t, node, 7)
		dvmrpRequireCommandType(t, da, 7, offset, 18)
		dvmrpRequireUintSpan(t, da, "Count", 2, offset, 19, 20)
		routes := protocolCorpusNodesNamed(da, "Destination Address")
		require.Len(t, routes, 2)
		for index, want := range []uint64{0x8002fbe7, 0x8002ec02} {
			value, err := routes[index].Result()
			require.NoError(t, err)
			require.Equal(t, want, uintVal(t, value))
			require.Equal(t, [2]uint64{uint64(offset+20+4*index) * 8, uint64(offset+24+4*index) * 8}, stream_parser.GetNodeResultPos(routes[index]))
		}
		require.Equal(t, 6, dvmrpInfo(t, dvmrp, "Command Count"))
	case 2:
		afi := dvmrpCommand(t, node, 2)
		dvmrpRequireCommandType(t, afi, 2, offset, 4)
		dvmrpRequireUintSpan(t, afi, "Address Family", 2, offset, 5, 6)
		mask := dvmrpCommand(t, node, 3)
		dvmrpRequireCommandType(t, mask, 3, offset, 6)
		dvmrpRequireUintSpan(t, mask, "Count", 0, offset, 7, 8)
		require.Nil(t, protocolCorpusFindNode(mask, "Subnet Mask"))
		rda := dvmrpCommand(t, node, 8)
		dvmrpRequireCommandType(t, rda, 8, offset, 8)
		dvmrpRequireUintSpan(t, rda, "Count", 2, offset, 9, 10)
		routes := protocolCorpusNodesNamed(rda, "Requested Destination Address")
		require.Len(t, routes, 2)
		for index, want := range []uint64{0xc0000200, 0xc6336400} {
			value, err := routes[index].Result()
			require.NoError(t, err)
			require.Equal(t, want, uintVal(t, value))
			require.Equal(t, [2]uint64{uint64(offset+10+4*index) * 8, uint64(offset+14+4*index) * 8}, stream_parser.GetNodeResultPos(routes[index]))
		}
		require.Equal(t, 3, dvmrpInfo(t, dvmrp, "Command Count"))
	case 3:
		null := dvmrpCommand(t, node, 0)
		dvmrpRequireCommandType(t, null, 0, offset, 4)
		dvmrpRequireUintSpan(t, null, "Ignored", 0xaa, offset, 5, 6)
		afi := dvmrpCommand(t, node, 2)
		dvmrpRequireCommandType(t, afi, 2, offset, 6)
		dvmrpRequireUintSpan(t, afi, "Address Family", 2, offset, 7, 8)
		flags := dvmrpCommand(t, node, 5)
		dvmrpRequireCommandType(t, flags, 5, offset, 8)
		protocolCorpusRequireValue(t, flags, "Destination Unreachable", uint64(1))
		protocolCorpusRequireValue(t, flags, "Split Horizon Concealed", uint64(1))
		protocolCorpusRequireValue(t, flags, "Flags Reserved", uint64(63))
		flagStart := uint64(offset+9) * 8
		dvmrpRequireSpan(t, flags, "Destination Unreachable", flagStart, flagStart+1)
		dvmrpRequireSpan(t, flags, "Split Horizon Concealed", flagStart+1, flagStart+2)
		dvmrpRequireSpan(t, flags, "Flags Reserved", flagStart+2, flagStart+8)
		nmr := dvmrpCommand(t, node, 9)
		dvmrpRequireCommandType(t, nmr, 9, offset, 10)
		dvmrpRequireUintSpan(t, nmr, "Count", 2, offset, 11, 12)
		entries := protocolCorpusNodesNamed(nmr, "Non-Membership Entry")
		require.Len(t, entries, 2)
		for index, want := range []struct {
			address, hold uint64
		}{{0xe0020301, 20}, {0xe0050406, 40}} {
			start := 12 + 8*index
			dvmrpRequireUintSpan(t, entries[index], "Address", want.address, offset, start, start+4)
			dvmrpRequireUintSpan(t, entries[index], "Hold Down Time", want.hold, offset, start+4, start+8)
		}
		require.Equal(t, 4, dvmrpInfo(t, dvmrp, "Command Count"))
	case 4:
		afi := dvmrpCommand(t, node, 2)
		dvmrpRequireCommandType(t, afi, 2, offset, 4)
		dvmrpRequireUintSpan(t, afi, "Address Family", 2, offset, 5, 6)
		cancel := dvmrpCommand(t, node, 10)
		dvmrpRequireCommandType(t, cancel, 10, offset, 6)
		dvmrpRequireUintSpan(t, cancel, "Count", 2, offset, 7, 8)
		addresses := protocolCorpusNodesNamed(cancel, "Multicast Address")
		require.Len(t, addresses, 2)
		for index, want := range []uint64{0xe0070805, 0xef010203} {
			dvmrpRequireUintSpan(t, addresses[index], "Address", want, offset, 8+4*index, 12+4*index)
		}
		require.Equal(t, 2, dvmrpInfo(t, dvmrp, "Command Count"))
	default:
		t.Fatalf("unexpected DVMRP subtype %d", subtype)
	}
}

func dvmrpRequireHeader(t *testing.T, node *base.Node, wire []byte, offset int, subtype uint64) {
	t.Helper()
	protocolCorpusRequireValue(t, node, "Version", uint64(1))
	protocolCorpusRequireValue(t, node, "Type", uint64(3))
	protocolCorpusRequireValue(t, node, "Code", subtype)
	protocolCorpusRequireValue(t, node, "Checksum", uint64(binary.BigEndian.Uint16(wire[2:4])))
	bitOffset := uint64(offset) * 8
	dvmrpRequireSpan(t, node, "Version", bitOffset, bitOffset+4)
	dvmrpRequireSpan(t, node, "Type", bitOffset+4, bitOffset+8)
	dvmrpRequireSpan(t, node, "Code", bitOffset+8, bitOffset+16)
	dvmrpRequireSpan(t, node, "Checksum", bitOffset+16, bitOffset+32)
	dvmrp := protocolCorpusFindNode(node, "Version").Cfg.GetItem(base.CfgParent).(*base.Node)
	require.Equal(t, true, dvmrpInfo(t, dvmrp, "Checksum Valid"))
}

func TestProtocolCorpusDVMRPOriginalChecksumEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-dvmrp.pcap")
	require.Len(t, frames, 1)
	for index, frame := range frames {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			require.Len(t, frame, 38)
			require.Equal(t, byte(2), frame[23], "IPv4 protocol must be IGMP")
			require.Equal(t, []byte{0x13, 0x01, 0, 0}, frame[34:])
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[34:]), "dvmrp", "DVMRP")
			require.ErrorContains(t, err, "dvmrp: checksum mismatch")

			carrier := dvmrpParse(t, frame[34:], "DVMRPCarrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed DVMRP Payload", frame[34:])
			require.Nil(t, protocolCorpusFindNode(carrier, "Version"))
		})
	}
}

func TestProtocolCorpusDVMRPValidMinimalRequestBytesAndFields(t *testing.T) {
	wire := dvmrpTestMessage(t, 2, 8, 0) // RFC 1075 3.12.3: request all routes.
	require.Equal(t, []byte{0x13, 0x02, 0xe4, 0xfd, 0x08, 0x00}, wire)
	frame := dvmrpTestEthernet(t, wire)
	want, err := hex.DecodeString("02000000000202000000000108004500001a000100000102cfdc0a000001e00000041302e4fd0800")
	require.NoError(t, err)
	require.Equal(t, want, frame)

	for _, entry := range []string{"DVMRP", "DVMRPCarrier"} {
		t.Run(entry, func(t *testing.T) {
			node := dvmrpParse(t, wire, entry)
			dvmrpRequireHeader(t, node, wire, 0, 2)
			command := dvmrpCommand(t, node, 8)
			protocolCorpusRequireValue(t, command, "Count", uint64(0))
			dvmrpRequireSpan(t, command, "Command Type", 32, 40)
			dvmrpRequireSpan(t, command, "Count", 40, 48)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed DVMRP Payload"))
			dvmrp := protocolCorpusFindNode(node, "Version").Cfg.GetItem(base.CfgParent).(*base.Node)
			require.Equal(t, 1, dvmrpInfo(t, dvmrp, "Command Count"))
		})
	}
}

func TestProtocolCorpusDVMRPValidatedCompanionEveryRecord(t *testing.T) {
	wires := dvmrpTestSubtypeMessages(t)
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-dvmrp-valid.pcap")
	require.Len(t, frames, len(wires))
	for index, wire := range wires {
		t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
			want := dvmrpTestEthernet(t, wire)
			require.Equal(t, want, frames[index], "checked-in companion must match the independent RFC encoding")
			node := dvmrpParse(t, frames[index][34:], "DVMRP")
			dvmrpRequireSubtypeFields(t, node, wire, 0)
			envelope := protocolCorpusRequireBoundedRuleParse(t, frames[index], "ethernet", "Ethernet")
			code := protocolCorpusFindNode(envelope, "Code")
			require.NotNil(t, code)
			dvmrp := code.Cfg.GetItem(base.CfgParent).(*base.Node)
			dvmrpRequireSubtypeFields(t, dvmrp, wire, 34)
			require.Nil(t, protocolCorpusFindNode(envelope, "Unparsed DVMRP Payload"))
		})
	}
}

func TestProtocolCorpusDVMRPIPv4DispatchOptionsFragmentsAndIGMP(t *testing.T) {
	valid := dvmrpTestMessage(t, 2, 8, 0)
	options := []byte{1, 1, 0, 0} // two NOPs, EOL, and one alignment octet.
	for _, sample := range []struct {
		name     string
		frame    []byte
		decoded  bool
		fragment bool
		offset   int
	}{
		{"plain", dvmrpTestEthernet(t, valid), true, false, 34},
		{"ihl-options", dvmrpTestEthernetIPv4(t, valid, options, 0), true, false, 38},
		{"more-fragments", dvmrpTestEthernetIPv4(t, valid, options, 0x2000), false, true, 38},
		{"noninitial-fragment", dvmrpTestEthernetIPv4(t, valid, options, 1), false, true, 38},
	} {
		t.Run(sample.name, func(t *testing.T) {
			root, err := base.ParseRule("ethernet.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(sample.frame))*8)
			reader := bytes.NewReader(sample.frame)
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "Ethernet"))
			parsed := base.GetNodeByPath(root, "@Ethernet")
			require.NotNil(t, parsed)
			require.Equal(t, sample.frame, NodeToBytes(parsed))
			if len(options) > 0 && sample.offset == 38 {
				protocolCorpusRequireValue(t, parsed, "IP Header Options", options)
				dvmrpRequireSpan(t, parsed, "IP Header Options", 34*8, 38*8)
			}
			if sample.decoded {
				code := protocolCorpusFindNode(parsed, "Code")
				require.NotNil(t, code)
				dvmrp := code.Cfg.GetItem(base.CfgParent).(*base.Node)
				dvmrpRequireHeader(t, dvmrp, valid, sample.offset, 2)
				require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed DVMRP Payload"))
				require.Nil(t, protocolCorpusFindNode(parsed, "IP Fragment Data"))
			} else {
				require.Nil(t, protocolCorpusFindNode(parsed, "Code"))
				require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed DVMRP Payload"))
				protocolCorpusRequireValue(t, parsed, "IP Fragment Data", valid)
				dvmrpRequireSpan(t, parsed, "IP Fragment Data", uint64(sample.offset)*8, uint64(len(sample.frame))*8)
			}
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		})
	}

	// IPv4 protocol 2 remains ordinary IGMP unless the complete bounded
	// payload starts with DVMRP's 0x13 message byte.
	igmp := []byte{0x16, 0, 0, 0, 224, 0, 0, 1}
	binary.BigEndian.PutUint16(igmp[2:4], dvmrpTestChecksum(igmp))
	frame := dvmrpTestEthernet(t, igmp)
	parsed := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, parsed, "Max Resp Time", uint64(0))
	protocolCorpusRequireValue(t, parsed, "Group Address", []byte{224, 0, 0, 1})
	require.Nil(t, protocolCorpusFindNode(parsed, "Code"))
	require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed DVMRP Payload"))

	// The retained malformed identification fixture reaches the DVMRP carrier,
	// which must preserve all four payload octets after its failed checksum.
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-dvmrp.pcap")[0]
	parsed = protocolCorpusRequireBoundedRuleParse(t, original, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, parsed, "Unparsed DVMRP Payload", original[34:])
	dvmrpRequireSpan(t, parsed, "Unparsed DVMRP Payload", 34*8, uint64(len(original))*8)
	require.Nil(t, protocolCorpusFindNode(parsed, "Code"))
}

func TestProtocolCorpusDVMRPAllV1SubtypesAndCommands(t *testing.T) {
	messages := dvmrpTestSubtypeMessages(t)

	for _, sample := range []struct {
		name    string
		wire    []byte
		subtype uint64
		primary uint64
	}{
		{"response", messages[0], 1, 7},
		{"request", messages[1], 2, 8},
		{"non-membership", messages[2], 3, 9},
		{"cancellation", messages[3], 4, 10},
	} {
		t.Run(sample.name, func(t *testing.T) {
			node := dvmrpParse(t, sample.wire, "DVMRP")
			dvmrpRequireSubtypeFields(t, node, sample.wire, 0)
			require.NotNil(t, dvmrpCommand(t, node, sample.primary))
			for end := 0; end < len(sample.wire); end++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(sample.wire[:end]), "dvmrp", "DVMRP")
				require.Error(t, err, "prefix %d of %d", end, len(sample.wire))
				if end == 0 {
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "dvmrp", "DVMRPCarrier")
					require.ErrorContains(t, err, "dvmrp: empty carrier has no message")
					continue
				}
				carrier := dvmrpParse(t, sample.wire[:end], "DVMRPCarrier")
				protocolCorpusRequireValue(t, carrier, "Unparsed DVMRP Payload", sample.wire[:end])
				require.Nil(t, protocolCorpusFindNode(carrier, "Version"))
			}
		})
	}
}

func TestProtocolCorpusDVMRPRejectsInvalidV1Structures(t *testing.T) {
	validRequest := dvmrpTestMessage(t, 2, 8, 0)
	v3Signature := dvmrpTestMessage(t, 1, 0, 0, 255, 3)
	for _, sample := range []struct {
		name string
		wire []byte
		err  string
	}{
		{"wrong-version", dvmrpTestMessage(t, 2, 8, 0), "dvmrp: unsupported version"},
		{"wrong-type", dvmrpTestMessage(t, 2, 8, 0), "dvmrp: invalid IGMP DVMRP type"},
		{"unknown-subtype", dvmrpTestMessage(t, 0, 8, 0), "dvmrp: unsupported v1 subtype"},
		{"unsupported-command", dvmrpTestMessage(t, 2, 1, 0), "dvmrp: unsupported v1 command"},
		{"v3-signature", v3Signature, "dvmrp: unsupported v1 command"},
		{"unsupported-address-family", dvmrpTestMessage(t, 2, 2, 3, 8, 0), "dvmrp: unsupported address family"},
		{"subnet-count", dvmrpTestMessage(t, 2, 3, 2, 8, 0), "dvmrp: subnet-mask count must be zero or one"},
		{"subnet-high-octet", dvmrpTestMessage(t, 2, 3, 1, 127, 255, 255, 0, 8, 0), "dvmrp: invalid subnet mask"},
		{"subnet-all-ones", dvmrpTestMessage(t, 2, 3, 1, 255, 255, 255, 255, 8, 0), "dvmrp: invalid subnet mask"},
		{"zero-metric", dvmrpTestMessage(t, 1, 4, 0, 7, 1, 192, 0, 2, 0), "dvmrp: metric must be nonzero"},
		{"zero-infinity", dvmrpTestMessage(t, 1, 4, 1, 6, 0, 7, 1, 192, 0, 2, 0), "dvmrp: infinity must be nonzero"},
		{"infinity-below-metric", dvmrpTestMessage(t, 1, 4, 17, 6, 16, 7, 1, 192, 0, 2, 0), "dvmrp: infinity is less than the current metric"},
		{"destination-zero", dvmrpTestMessage(t, 1, 4, 1, 7, 0), "dvmrp: destination-address count must be nonzero"},
		{"destination-without-metric", dvmrpTestMessage(t, 1, 7, 1, 192, 0, 2, 0), "dvmrp: destination command has no current metric"},
		{"metric-above-default-infinity", dvmrpTestMessage(t, 1, 4, 17, 7, 1, 192, 0, 2, 0), "dvmrp: current metric exceeds infinity"},
		{"response-rda", dvmrpTestMessage(t, 1, 8, 0), "dvmrp: command is incompatible with the message subtype"},
		{"request-da", dvmrpTestMessage(t, 2, 7, 1, 192, 0, 2, 0), "dvmrp: command is incompatible with the message subtype"},
		{"nmr-zero", dvmrpTestMessage(t, 3, 9, 0), "dvmrp: non-membership count must be nonzero"},
		{"nmr-unicast", dvmrpTestMessage(t, 3, 9, 1, 192, 0, 2, 1, 0, 0, 0, 20), "dvmrp: non-membership address is not multicast"},
		{"cancel-zero", dvmrpTestMessage(t, 4, 10, 0), "dvmrp: cancellation count must be nonzero"},
		{"checksum", append([]byte(nil), validRequest...), "dvmrp: checksum mismatch"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			wire := bytes.Clone(sample.wire)
			switch sample.name {
			case "wrong-version":
				wire[0] = 0x23
				binary.BigEndian.PutUint16(wire[2:4], 0)
				binary.BigEndian.PutUint16(wire[2:4], dvmrpTestChecksum(wire))
			case "wrong-type":
				wire[0] = 0x12
				binary.BigEndian.PutUint16(wire[2:4], 0)
				binary.BigEndian.PutUint16(wire[2:4], dvmrpTestChecksum(wire))
			case "checksum":
				wire[3] ^= 1
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "dvmrp", "DVMRP")
			require.ErrorContains(t, err, sample.err)
		})
	}

	odd := append(bytes.Clone(validRequest), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(odd), "dvmrp", "DVMRP")
	require.ErrorContains(t, err, "dvmrp: command stream is not 16-bit aligned")
	for _, short := range [][]byte{nil, {0x13}, {0x13, 2}, {0x13, 2, 0}} {
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(short), "dvmrp", "DVMRP")
		require.ErrorContains(t, err, "dvmrp: message is shorter than the fixed header")
	}
}

func TestProtocolCorpusDVMRPHeaderOnlyReceiverBoundary(t *testing.T) {
	// RFC 1075 explicitly defines errors for individual commands but does not
	// make the tagged stream nonempty.  Accept the checksum-valid bounded
	// header as receive syntax; the retained PR fixture is still rejected for
	// its independently invalid zero checksum and is not a semantic positive.
	wire := dvmrpTestMessage(t, 1)
	require.Equal(t, []byte{0x13, 0x01, 0xec, 0xfe}, wire)
	node := dvmrpParse(t, wire, "DVMRP")
	dvmrpRequireHeader(t, node, wire, 0, 1)
	dvmrp := protocolCorpusFindNode(node, "Version").Cfg.GetItem(base.CfgParent).(*base.Node)
	require.Equal(t, 0, dvmrpInfo(t, dvmrp, "Command Count"))
	require.Nil(t, protocolCorpusFindNode(node, "Command"))
}

func TestProtocolCorpusDVMRPResourceBoundary(t *testing.T) {
	commands := make([]byte, 0, 508)
	for i := 0; i < 253; i++ {
		commands = append(commands, 0, byte(i))
	}
	commands = append(commands, 8, 0)
	wire := dvmrpTestMessage(t, 2, commands...)
	require.Len(t, wire, 512)
	node := dvmrpParse(t, wire, "DVMRP")
	require.Len(t, protocolCorpusNodesNamed(node, "Command"), 254)
	dvmrp := protocolCorpusFindNode(node, "Version").Cfg.GetItem(base.CfgParent).(*base.Node)
	require.Equal(t, 254, dvmrpInfo(t, dvmrp, "Command Count"))

	over := append(bytes.Clone(wire), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(over), "dvmrp", "DVMRP")
	require.ErrorContains(t, err, "dvmrp: message exceeds the 512-byte RFC 1075 limit")
}

func TestProtocolCorpusDVMRPCarrierTransactions(t *testing.T) {
	valid := dvmrpTestMessage(t, 2, 8, 0)
	validWithNull := dvmrpTestMessage(t, 2, 0, 0xab, 8, 0)
	invalidChecksum := bytes.Clone(valid)
	invalidChecksum[2] ^= 1
	lateInvalid := dvmrpTestMessage(t, 3, 2, 2, 9, 1, 192, 0, 2, 1, 0, 0, 0, 20)
	for _, sample := range []struct {
		name    string
		wire    []byte
		decoded bool
	}{
		{"valid", valid, true},
		{"valid-with-null", validWithNull, true},
		{"bad-checksum", invalidChecksum, false},
		{"late-invalid", lateInvalid, false},
		{"truncated", valid[:5], false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			root, err := base.ParseRule("dvmrp.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(sample.wire))*8)
			reader := bytes.NewReader(sample.wire)
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "DVMRPCarrier"))
			parsed := base.GetNodeByPath(root, "@DVMRPCarrier")
			require.NotNil(t, parsed)
			require.Equal(t, sample.wire, NodeToBytes(parsed))
			if sample.decoded {
				protocolCorpusRequireValue(t, parsed, "Version", uint64(1))
				require.Nil(t, protocolCorpusFindNode(parsed, "Unparsed DVMRP Payload"))
			} else {
				protocolCorpusRequireValue(t, parsed, "Unparsed DVMRP Payload", sample.wire)
				require.Nil(t, protocolCorpusFindNode(parsed, "Version"))
			}
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		})
	}

	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "dvmrp", "DVMRPCarrier")
	require.ErrorContains(t, err, "dvmrp: empty carrier has no message")
}

func TestProtocolCorpusDVMRPConcurrentIsolation(t *testing.T) {
	valid := [][]byte{
		dvmrpTestMessage(t, 1, 4, 2, 6, 16, 7, 1, 128, 2, 251, 231),
		dvmrpTestMessage(t, 2, 8, 0),
		dvmrpTestMessage(t, 3, 9, 1, 224, 2, 3, 1, 0, 0, 0, 20),
		dvmrpTestMessage(t, 4, 10, 1, 224, 7, 8, 5),
	}
	for index := 0; index < 12; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(valid[index%len(valid)])
			invalid := bytes.Clone(wire)
			invalid[2] ^= byte(index + 1)
			carrier := dvmrpParse(t, invalid, "DVMRPCarrier")
			protocolCorpusRequireValue(t, carrier, "Unparsed DVMRP Payload", invalid)
			node := dvmrpParse(t, wire, "DVMRP")
			dvmrpRequireHeader(t, node, wire, 0, uint64(wire[1]))
		})
	}
}
