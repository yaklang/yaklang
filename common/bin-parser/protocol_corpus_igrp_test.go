package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Independent byte encodings, not produced by the YAML generator. These are
// constructed wire examples, not a claim of an observed router exchange.
// Cisco Doc 26825 and Wireshark v4.4.9 packet-igrp.c specify the header/vector
// layout; the request's zero checksum follows Cisco's unused-fields wording.
var igrpCorpusBodies = []string{
	"11351234000100010001bdd500010000007b0003e805dcf11702ac1000000456004c4b1234de3105c63364ffffff123456abcd807f09",
	"120012340000000000000000",
	"11a61234000000000000dc25",
}

func igrpCorpusBody(t *testing.T, index int) []byte {
	t.Helper()
	b, err := hex.DecodeString(igrpCorpusBodies[index])
	require.NoError(t, err)
	return b
}

// End-around carry after every addition provides an oracle independent of the
// rule's aggregate-and-fold checksum loop. No IPv4 pseudo-header participates.
func igrpCorpusChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data); i += 2 {
		v := uint32(data[i]) << 8
		if i+1 < len(data) {
			v |= uint32(data[i+1])
		}
		sum += v
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}

func igrpCorpusSeal(data []byte) []byte {
	data[10], data[11] = 0, 0
	binary.BigEndian.PutUint16(data[10:], igrpCorpusChecksum(data))
	return data
}

func igrpCorpusEthernet(data []byte) []byte {
	ip := []byte{0x45, 0, 0, 0, 0x12, 0x34, 0, 0, 1, 9, 0, 0, 10, 0, 0, 1, 255, 255, 255, 255}
	binary.BigEndian.PutUint16(ip[2:], uint16(20+len(data)))
	binary.BigEndian.PutUint16(ip[10:], igrpCorpusChecksum(ip))
	return append(append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}, ip...), data...)
}

func igrpCorpusRequireFields(t *testing.T, root *base.Node, wire []byte, offset int) {
	t.Helper()
	anchor := protocolCorpusFindNode(root, "Autonomous System")
	require.NotNil(t, anchor)
	n := anchor.Cfg.GetItem(base.CfgParent).(*base.Node)
	for _, f := range []struct {
		name       string
		start, end int
		value      uint64
	}{
		{"Version", 0, 4, uint64(wire[0] >> 4)}, {"Opcode", 4, 8, uint64(wire[0] & 15)}, {"Edition", 8, 16, uint64(wire[1])},
		{"Autonomous System", 16, 32, uint64(binary.BigEndian.Uint16(wire[2:]))}, {"Interior Count", 32, 48, uint64(binary.BigEndian.Uint16(wire[4:]))},
		{"System Count", 48, 64, uint64(binary.BigEndian.Uint16(wire[6:]))}, {"Exterior Count", 64, 80, uint64(binary.BigEndian.Uint16(wire[8:]))}, {"Checksum", 80, 96, uint64(binary.BigEndian.Uint16(wire[10:]))},
	} {
		protocolCorpusRequireValue(t, n, f.name, f.value)
		require.Equal(t, [2]uint64{uint64(offset*8 + f.start), uint64(offset*8 + f.end)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(n, f.name)))
	}
	position := 12
	for i, name := range []string{"Interior Routes", "System Routes", "Exterior Routes"} {
		count := int(binary.BigEndian.Uint16(wire[4+i*2:]))
		list := protocolCorpusFindNode(n, name)
		if count == 0 {
			require.Nil(t, list)
			continue
		}
		require.NotNil(t, list)
		require.Len(t, list.Children, count)
		result, err := list.Result()
		require.NoError(t, err)
		require.True(t, result.IsList())
		require.Len(t, result.Children(), count)
		require.Equal(t, count, list.Cfg.GetItem("additionInfo").(map[string]any)["Route Count"])
		for _, route := range list.Children {
			for _, f := range []struct {
				name       string
				start, end int
			}{{"Number", 0, 3}, {"Delay", 3, 6}, {"Bandwidth", 6, 9}, {"MTU", 9, 11}, {"Reliability", 11, 12}, {"Load", 12, 13}, {"Hop Count", 13, 14}} {
				var value uint64
				for _, octet := range wire[position+f.start : position+f.end] {
					value = value<<8 | uint64(octet)
				}
				protocolCorpusRequireValue(t, route, f.name, value)
				require.Equal(t, [2]uint64{uint64(offset+position+f.start) * 8, uint64(offset+position+f.end) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(route, f.name)))
			}
			info := route.Cfg.GetItem("additionInfo").(map[string]any)
			delay := int(wire[position+3])<<16 | int(wire[position+4])<<8 | int(wire[position+5])
			require.Equal(t, delay == 0xffffff, info["Unreachable"])
			if delay == 0xffffff {
				require.NotContains(t, info, "Delay Microseconds")
			} else {
				require.Equal(t, delay*10, info["Delay Microseconds"])
			}
			require.Equal(t, i == 0, info["Destination Context Required"])
			if i == 0 {
				require.NotContains(t, info, "Destination IPv4 Octets")
			} else {
				require.Equal(t, []byte{wire[position], wire[position+1], wire[position+2], 0}, info["Destination IPv4 Octets"])
			}
			position += 14
		}
	}
	require.Equal(t, len(wire), position)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, false, info["Composite Metric Computed"])
	require.Equal(t, wire[0] != 0x12 || binary.BigEndian.Uint16(wire[10:]) != 0, info["Checksum Validated"])
}

func TestProtocolCorpusIGRPIndependentMessages(t *testing.T) {
	for i := range igrpCorpusBodies {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			wire := igrpCorpusBody(t, i)
			if i != 1 {
				require.Zero(t, igrpCorpusChecksum(wire))
			}
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "igrp", "IGRP")
			igrpCorpusRequireFields(t, n, wire, 0)
		})
	}
}

func TestProtocolCorpusIGRPCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-igrp-valid.pcap")
	require.Len(t, frames, len(igrpCorpusBodies))
	for i, frame := range frames {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			wire := igrpCorpusBody(t, i)
			require.Equal(t, wire, frame[34:])
			require.Equal(t, byte(9), frame[23])
			require.Equal(t, byte(0x45), frame[14])
			for _, imported := range []bool{false, true} {
				input, rule, entry, offset := wire, "igrp", "IGRP", 0
				if imported {
					input, rule, entry, offset = frame, "ethernet", "Ethernet", 34
				}
				n := protocolCorpusRequireBoundedRuleParse(t, input, rule, entry)
				igrpCorpusRequireFields(t, n, wire, offset)
				require.Nil(t, protocolCorpusFindNode(n, "IGRP Payload"))
			}
		})
	}
}

func TestProtocolCorpusIGRPOriginalAndMalformed(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-igrp.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "e2d544a604d833bf4c4a577a9494c6d9da8f31e229891d56c0a597b4faf49bd0", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	require.Len(t, frames[0], 56)
	original := frames[0][34:]
	require.Equal(t, append([]byte{1}, make([]byte, 21)...), original)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(original), "igrp", "IGRP")
	require.ErrorContains(t, err, "igrp: unsupported version")
	publicOriginal := protocolCorpusRequireBoundedRuleParse(t, frames[0], "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, publicOriginal, "IGRP Payload", original)
	require.Nil(t, protocolCorpusFindNode(publicOriginal, "Autonomous System"))
	require.Equal(t, [2]uint64{34 * 8, 56 * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(publicOriginal, "IGRP Payload")))
	bad := [][]byte{original}
	for i := range igrpCorpusBodies {
		v := igrpCorpusBody(t, i)
		for cut := 0; cut < len(v); cut++ {
			bad = append(bad, bytes.Clone(v[:cut]))
		}
	}
	valid := igrpCorpusBody(t, 0)
	for _, field := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 20, 39, 53} {
		v := bytes.Clone(valid)
		v[field] ^= 1
		bad = append(bad, v)
	}
	for _, first := range []byte{0x01, 0x21, 0xf1, 0x10, 0x13, 0x1f} {
		v := bytes.Clone(valid)
		v[0] = first
		bad = append(bad, igrpCorpusSeal(v))
	}
	for _, field := range []int{4, 6, 8} {
		for _, delta := range []int{-1, 1} {
			v := bytes.Clone(valid)
			binary.BigEndian.PutUint16(v[field:], uint16(1+delta))
			bad = append(bad, igrpCorpusSeal(v))
		}
	}
	bad = append(bad, igrpCorpusSeal(append(bytes.Clone(valid), 0, 0)))
	for i, wire := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "igrp", "IGRP")
			require.Error(t, err)
			if len(wire) > 0 {
				n := protocolCorpusRequireBoundedRuleParse(t, wire, "igrp", "IGRPCarrier")
				protocolCorpusRequireValue(t, n, "IGRP Payload", wire)
				require.Nil(t, protocolCorpusFindNode(n, "Autonomous System"))
				require.Equal(t, [2]uint64{0, uint64(len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(n, "IGRP Payload")))
			}
		})
	}
	_, err = parser.ParseBinary(bytes.NewReader(valid), "igrp", "IGRP")
	require.ErrorContains(t, err, "explicit packet boundary required")
}

func TestProtocolCorpusIGRPRequestChecksumVariants(t *testing.T) {
	request := igrpCorpusBody(t, 1)
	checked := igrpCorpusSeal(bytes.Clone(request))
	require.Equal(t, "12001234000000000000dbcb", hex.EncodeToString(checked))
	for _, v := range [][]byte{request, checked} {
		n := protocolCorpusRequireBoundedRuleParse(t, v, "igrp", "IGRP")
		igrpCorpusRequireFields(t, n, v, 0)
	}
	for _, field := range []int{1, 4, 6, 8} {
		v := bytes.Clone(request)
		v[field] = 1
		igrpCorpusSeal(v)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(v), "igrp", "IGRP")
		require.Error(t, err)
	}
	for _, checksum := range []uint16{1, 0xffff, 0xdbca} {
		v := bytes.Clone(request)
		binary.BigEndian.PutUint16(v[10:], checksum)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(v), "igrp", "IGRP")
		require.ErrorContains(t, err, "checksum mismatch")
	}
	// A mathematically valid zero checksum in an update is still validated.
	v := igrpCorpusBody(t, 2)
	binary.BigEndian.PutUint16(v[2:], 0xee59)
	igrpCorpusSeal(v)
	require.Zero(t, binary.BigEndian.Uint16(v[10:]))
	n := protocolCorpusRequireBoundedRuleParse(t, v, "igrp", "IGRP")
	igrpCorpusRequireFields(t, n, v, 0)
}

func TestProtocolCorpusIGRPRouteAndResourceBounds(t *testing.T) {
	for _, count := range []int{0, 1, 52, 104, 105} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			v := igrpCorpusBody(t, 2)
			binary.BigEndian.PutUint16(v[4:], uint16(count))
			v = append(v, bytes.Repeat(igrpCorpusBody(t, 0)[12:26], count)...)
			igrpCorpusSeal(v)
			n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(v), "igrp", "IGRP")
			if count == 105 {
				require.ErrorContains(t, err, "supported 104-entry limit")
				return
			}
			require.NoError(t, err)
			igrpCorpusRequireFields(t, n, v, 0)
			require.Equal(t, v, NodeToBytes(n))
		})
	}
	for _, counts := range [][3]uint16{{35, 35, 34}, {65535, 0, 0}, {65535, 65535, 65535}} {
		v := igrpCorpusBody(t, 2)
		total := 0
		for i, c := range counts {
			binary.BigEndian.PutUint16(v[4+i*2:], c)
			total += int(c)
		}
		if total == 104 {
			v = append(v, bytes.Repeat(igrpCorpusBody(t, 0)[12:26], total)...)
		}
		igrpCorpusSeal(v)
		n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(v), "igrp", "IGRP")
		if total > 104 {
			require.ErrorContains(t, err, "supported 104-entry limit")
		} else {
			require.NoError(t, err)
			igrpCorpusRequireFields(t, n, v, 0)
		}
	}
	for _, fill := range []byte{0, 1, 0x80, 0xff} {
		v := igrpCorpusBody(t, 0)
		for i := 12; i < len(v); i++ {
			v[i] = fill
		}
		igrpCorpusSeal(v)
		n := protocolCorpusRequireBoundedRuleParse(t, v, "igrp", "IGRP")
		igrpCorpusRequireFields(t, n, v, 0)
	}
}

func TestProtocolCorpusIGRPDispatchAndReaderTransactions(t *testing.T) {
	valid := igrpCorpusBody(t, 0)
	bad := bytes.Clone(valid)
	bad[len(bad)-1] ^= 1
	for _, wire := range [][]byte{valid, igrpCorpusBody(t, 1), bad, valid[:1], valid[:len(valid)-1], append(bytes.Clone(valid), 0, 0)} {
		for _, imported := range []bool{false, true} {
			input, rule, entry := wire, "igrp.yaml", "IGRPCarrier"
			if imported {
				input, rule, entry = igrpCorpusEthernet(wire), "ethernet.yaml", "Ethernet"
			}
			root, err := base.ParseRule(rule)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
			reader := base.NewBitReader(bytes.NewReader(input))
			require.NoError(t, root.ParseSubNode(reader, entry))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			require.ErrorContains(t, reader.PopBackup(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
			parsed := base.GetNodeByPath(root, "@"+entry)
			require.NotNil(t, parsed)
			require.Equal(t, input, NodeToBytes(parsed))
			if !bytes.Equal(wire, valid) && !bytes.Equal(wire, igrpCorpusBody(t, 1)) {
				protocolCorpusRequireValue(t, parsed, "IGRP Payload", wire)
				offset := 0
				if imported {
					offset = 34
				}
				require.Equal(t, [2]uint64{uint64(offset) * 8, uint64(len(input)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(parsed, "IGRP Payload")))
			} else {
				igrpCorpusRequireFields(t, parsed, wire, len(input)-len(wire))
			}
		}
	}
	// Both the complete message and all fragment variants start after the
	// actual IPv4 header, including a 24-byte header with explicit options.
	for _, options := range []bool{false, true} {
		frame := igrpCorpusEthernet(valid)
		offset := 34
		if options {
			frame = append(append(bytes.Clone(frame[:34]), 1, 1, 0, 0), frame[34:]...)
			offset = 38
			frame[14] = 0x46
			binary.BigEndian.PutUint16(frame[16:], uint16(len(frame)-14))
		}
		for _, flags := range []uint16{0, 0x2000, 1, 0x2001} {
			binary.BigEndian.PutUint16(frame[20:], flags)
			frame[24], frame[25] = 0, 0
			binary.BigEndian.PutUint16(frame[24:], igrpCorpusChecksum(frame[14:offset]))
			n := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			if options {
				protocolCorpusRequireValue(t, n, "IP Header Options", []byte{1, 1, 0, 0})
			}
			if flags == 0 {
				igrpCorpusRequireFields(t, n, valid, offset)
			} else {
				require.Nil(t, protocolCorpusFindNode(n, "Autonomous System"))
				protocolCorpusRequireValue(t, n, "IP Fragment Data", valid)
				require.Equal(t, [2]uint64{uint64(offset) * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(n, "IP Fragment Data")))
			}
		}
	}
	// The same valid bytes on another IPv4 protocol do not select IGRP.
	other := igrpCorpusEthernet(valid)
	other[23] = 253
	other[24], other[25] = 0, 0
	binary.BigEndian.PutUint16(other[24:], igrpCorpusChecksum(other[14:34]))
	n := protocolCorpusRequireBoundedRuleParse(t, other, "ethernet", "Ethernet")
	require.Nil(t, protocolCorpusFindNode(n, "Autonomous System"))
}

func TestProtocolCorpusIGRPParallelIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for j := 0; j < 3; j++ {
				wire := igrpCorpusBody(t, (i+j)%3)
				if wire[0] == 0x11 {
					wire[1] = byte(i + j)
					igrpCorpusSeal(wire)
				}
				n := protocolCorpusRequireBoundedRuleParse(t, wire, "igrp", "IGRP")
				igrpCorpusRequireFields(t, n, wire, 0)
			}
		})
	}
}

func BenchmarkProtocolCorpusIGRP104Routes(b *testing.B) {
	header, err := hex.DecodeString(igrpCorpusBodies[2])
	if err != nil {
		b.Fatal(err)
	}
	sample, err := hex.DecodeString(igrpCorpusBodies[0])
	if err != nil {
		b.Fatal(err)
	}
	binary.BigEndian.PutUint16(header[4:], 104)
	wire := igrpCorpusSeal(append(header, bytes.Repeat(sample[12:26], 104)...))
	if _, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "igrp", "IGRP"); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(wire)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "igrp", "IGRP")
		if err != nil {
			b.Fatal(err)
		}
		list := protocolCorpusFindNode(n, "Interior Routes")
		if list == nil || len(list.Children) != 104 {
			b.Fatal("incomplete route tree")
		}
	}
}
