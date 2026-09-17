package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func wsmpTestHex(t *testing.T, encoded string) []byte {
	t.Helper()
	wire, err := hex.DecodeString(encoded)
	require.NoError(t, err)
	return wire
}

func wsmpTestFixtures(t *testing.T) [][]byte {
	t.Helper()
	var fixtures [][]byte
	for _, encoded := range []string{
		"0220800003616263",
		"0280020f01ac10010c0401148100058102616263",
		"02e00000018200020102",
		"03002003616263",
		"0b050f01ac10010c040114170155fe02aabb00c0000103616263",
		"0301800201fd02beef02cafe",
		"03021234567803010203",
		"0b0003123456780000",
	} {
		fixtures = append(fixtures, wsmpTestHex(t, encoded))
	}
	return fixtures
}

func wsmpTestEthernet(payload []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x88, 0xdc}, payload...)
}

type wsmpTestBitBoundaryReader struct {
	*bytes.Reader
	bits uint64
}

func (r *wsmpTestBitBoundaryReader) InputBitLength() uint64 { return r.bits }

func TestProtocolCorpusWSMPRequiresByteBoundary(t *testing.T) {
	wire := wsmpTestFixtures(t)[3]
	for extra := uint64(1); extra < 8; extra++ {
		for _, entry := range []string{"WSMP", "WSMPCarrier"} {
			for _, legacy := range []bool{false, true} {
				input := &wsmpTestBitBoundaryReader{bytes.NewReader(append(bytes.Clone(wire), 0)), uint64(len(wire))*8 + extra}
				_, err := parser.ParseBinaryWithConfig(input, "wsmp", map[string]any{"wsmpLegacy": legacy, "outScalarLegacy": legacy}, entry)
				require.ErrorContains(t, err, "wsmp: explicit byte boundary required")
				require.Equal(t, len(wire)+1, input.Len(), "unaligned boundary must not read input")
			}
		}
	}
}

func wsmpTestField(t *testing.T, root *base.Node, name string, want any, start, end uint64) *base.Node {
	t.Helper()
	protocolCorpusRequireValue(t, root, name, want)
	node := protocolCorpusFindNode(root, name)
	require.NotNil(t, node, name)
	result, err := node.Result()
	require.NoError(t, err)
	require.Same(t, node, result.Origin)
	require.Equal(t, [2]uint64{start, end}, wsmpTestSpan(t, node), name)
	return node
}

// Scalar out nodes are structs in the public API. Their encoded extent is the
// contiguous union of their real leaf positions, not a fabricated config item.
func wsmpTestSpan(t *testing.T, node *base.Node) [2]uint64 {
	t.Helper()
	var span [2]uint64
	found := false
	var walk func(*base.Node)
	walk = func(current *base.Node) {
		if stream_parser.NodeHasResult(current) {
			position := stream_parser.GetNodeResultPos(current)
			if !found {
				span = position
				found = true
			} else {
				require.Equal(t, span[1], position[0], current.Name)
				span[1] = position[1]
			}
			return
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(node)
	require.True(t, found, node.Name)
	return span
}

func wsmpTestAll(root *base.Node, name string) []*base.Node {
	var nodes []*base.Node
	var walk func(*base.Node)
	walk = func(node *base.Node) {
		if node.Name == name && protocolCorpusNodeHasResult(node) {
			nodes = append(nodes, node)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

type wsmpTestLeaf struct {
	Name     string
	Position [2]uint64
	Value    any
}

func wsmpTestLeaves(t *testing.T, root *base.Node, offset uint64) []wsmpTestLeaf {
	t.Helper()
	var leaves []wsmpTestLeaf
	var walk func(*base.Node)
	walk = func(node *base.Node) {
		if stream_parser.NodeHasResult(node) {
			position := stream_parser.GetNodeResultPos(node)
			if position[0] < offset*8 {
				return
			}
			value, err := node.Result()
			require.NoError(t, err)
			require.Same(t, node, value.Origin)
			position[0] -= offset * 8
			position[1] -= offset * 8
			leaves = append(leaves, wsmpTestLeaf{node.Name, position, value.Value})
			return
		}
		for _, child := range node.Children {
			if protocolCorpusNodeHasResult(child) {
				require.Same(t, node, child.Cfg.GetItem(base.CfgParent), child.Name)
				// Imports have their own rule-path context, but all views must
				// retain the original shared byte buffer and bit writer.
				require.Same(t, node.Ctx.GetItem("buffer"), child.Ctx.GetItem("buffer"), child.Name)
				require.Same(t, node.Ctx.GetItem("writer"), child.Ctx.GetItem("writer"), child.Name)
			}
			walk(child)
		}
	}
	walk(root)
	return leaves
}

// Literal records and independently stated expectations exercise all supported
// versions and transport modes. None is generated from the implementation.
func TestProtocolCorpusWSMPFields(t *testing.T) {
	wires := wsmpTestFixtures(t)
	for index, wire := range wires {
		t.Run(fmt.Sprintf("record-%d", index+1), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "wsmp", "WSMP")
			switch index {
			case 0:
				wsmpTestField(t, node, "Version", uint64(2), 0, 8)
				wsmpTestField(t, node, "PSID", uint64(32), 8, 16)
				wsmpTestField(t, node, "WAVE Element ID", uint64(128), 16, 24)
				wsmpTestField(t, node, "WSM Length", uint64(3), 24, 40)
				wsmpTestField(t, node, "WSM Data", []byte("abc"), 40, 64)
			case 1:
				wsmpTestField(t, node, "PSID", uint64(130), 8, 24)
				wsmpTestField(t, node, "Encoded PSID", []byte{0x80, 2}, 8, 24)
				wsmpTestField(t, node, "Channel Number", uint64(172), 40, 48)
				wsmpTestField(t, node, "Data Rate", uint64(12), 64, 72)
				wsmpTestField(t, node, "Transmit Power", uint64(20), 88, 96)
				wsmpTestField(t, node, "WAVE Element ID", uint64(129), 96, 104)
				wsmpTestField(t, node, "WSM Length", uint64(5), 104, 120)
				supplement := protocolCorpusFindNode(node, "Supplement")
				require.NotNil(t, supplement)
				require.Equal(t, [2]uint64{120, 136}, wsmpTestSpan(t, supplement))
				require.Len(t, wsmpTestAll(supplement, "Octet"), 2)
				wsmpTestField(t, node, "WSM Data", []byte("abc"), 136, 160)
				mapped, ok := NodeToMap(protocolCorpusFindNode(node, "Legacy Extensions")).([]any)
				require.True(t, ok)
				require.Len(t, mapped, 3)
			case 2:
				wsmpTestField(t, node, "PSID", uint64(0x204081), 8, 40)
				wsmpTestField(t, node, "WAVE Element ID", uint64(130), 40, 48)
				wsmpTestField(t, node, "WSM Length", uint64(2), 48, 64)
				wsmpTestField(t, node, "WSM Data", []byte{1, 2}, 64, 80)
			case 3:
				wsmpTestField(t, node, "Subtype", uint64(0), 0, 4)
				wsmpTestField(t, node, "Network Extension Present", uint64(0), 4, 5)
				wsmpTestField(t, node, "Version", uint64(3), 5, 8)
				wsmpTestField(t, node, "TPID", uint64(0), 8, 16)
				wsmpTestField(t, node, "PSID", uint64(32), 16, 24)
				wsmpTestField(t, node, "WSM Length", uint64(3), 24, 32)
				wsmpTestField(t, node, "WSM Data", []byte("abc"), 32, 56)
			case 4:
				wsmpTestField(t, node, "Network Extension Present", uint64(1), 4, 5)
				wsmpTestField(t, node, "Element Count", uint64(5), 8, 16)
				wsmpTestField(t, node, "Channel Number", uint64(172), 32, 40)
				wsmpTestField(t, node, "Data Rate", uint64(12), 56, 64)
				wsmpTestField(t, node, "Transmit Power", uint64(20), 80, 88)
				wsmpTestField(t, node, "Channel Load", uint64(85), 104, 112)
				wsmpTestField(t, node, "Element Data", []byte{0xaa, 0xbb}, 128, 144)
				wsmpTestField(t, node, "TPID", uint64(0), 144, 152)
				wsmpTestField(t, node, "PSID", uint64(0x4081), 152, 176)
				wsmpTestField(t, node, "WSM Length", uint64(3), 176, 184)
				wsmpTestField(t, node, "WSM Data", []byte("abc"), 184, 208)
				elements := protocolCorpusFindNode(node, "Information Elements")
				require.Len(t, wsmpTestAll(elements, "Element"), 5)
				result, err := elements.Result()
				require.NoError(t, err)
				require.True(t, result.IsList())
				mapped, ok := NodeToMap(elements).([]any)
				require.True(t, ok)
				require.Len(t, mapped, 5)
			case 5:
				wsmpTestField(t, node, "TPID", uint64(1), 8, 16)
				wsmpTestField(t, node, "PSID", uint64(130), 16, 32)
				wsmpTestField(t, node, "Element Count", uint64(1), 32, 40)
				wsmpTestField(t, node, "Element ID", uint64(253), 40, 48)
				wsmpTestField(t, node, "Element Length", uint64(2), 48, 56)
				wsmpTestField(t, node, "Element Data", []byte{0xbe, 0xef}, 56, 72)
				wsmpTestField(t, node, "WSM Data", []byte{0xca, 0xfe}, 80, 96)
			case 6:
				wsmpTestField(t, node, "TPID", uint64(2), 8, 16)
				wsmpTestField(t, node, "Source ITS Port", uint64(0x1234), 16, 32)
				wsmpTestField(t, node, "Destination ITS Port", uint64(0x5678), 32, 48)
				wsmpTestField(t, node, "WSM Length", uint64(3), 48, 56)
				wsmpTestField(t, node, "WSM Data", []byte{1, 2, 3}, 56, 80)
			case 7:
				wsmpTestField(t, node, "TPID", uint64(3), 16, 24)
				wsmpTestField(t, node, "Source ITS Port", uint64(0x1234), 24, 40)
				wsmpTestField(t, node, "Destination ITS Port", uint64(0x5678), 40, 56)
				wsmpTestField(t, node, "WSM Length", uint64(0), 64, 72)
				require.Len(t, wsmpTestAll(node, "Element Count"), 2)
				require.Nil(t, protocolCorpusFindNode(node, "WSM Data"))
			}
		})
	}
}

func TestProtocolCorpusWSMPPSIDEncoding(t *testing.T) {
	for _, tc := range []struct {
		wire string
		want uint64
	}{
		{"00", 0}, {"7f", 127}, {"8000", 128}, {"bfff", 16511},
		{"c00000", 16512}, {"dfffff", 2113663}, {"e0000000", 2113664}, {"efffffff", 270549119},
	} {
		wire := append([]byte{3, 0}, wsmpTestHex(t, tc.wire)...)
		wire = append(wire, 0)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "wsmp", "WSMP")
		field := wsmpTestField(t, node, "PSID", tc.want, 16, uint64(len(wire)-1)*8)
		result, err := field.Result()
		require.NoError(t, err)
		require.IsType(t, int(0), result.Value)
	}
}

func TestProtocolCorpusWSMPTransportExtensionNamespace(t *testing.T) {
	// Transport extensions use the same counted TLV framing, not the network
	// element namespace: ID 4 here must not become a one-octet transmit power.
	wire := wsmpTestHex(t, "030120010402aabb00")
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "wsmp", "WSMP")
	wsmpTestField(t, node, "Element ID", uint64(4), 32, 40)
	wsmpTestField(t, node, "Element Length", uint64(2), 40, 48)
	wsmpTestField(t, node, "Element Data", []byte{0xaa, 0xbb}, 48, 64)
	require.Nil(t, protocolCorpusFindNode(node, "Transmit Power"))
}

func TestProtocolCorpusWSMPOriginalAndBoundaries(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-wsmp.pcap")
	require.Len(t, frames, 1)
	original := wsmpTestHex(t, "0200000000000000000000")
	require.Equal(t, wsmpTestEthernet(original), frames[0])
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original), "wsmp", "WSMP")
	require.ErrorContains(t, err, "wsmp: unsupported legacy extension identifier")
	for fixture, wire := range wsmpTestFixtures(t) {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "wsmp", "WSMP")
			require.Errorf(t, err, "fixture %d prefix %d", fixture, cut)
			if cut > 0 {
				node := protocolCorpusRequireBoundedRuleParse(t, wire[:cut], "wsmp", "WSMPCarrier")
				protocolCorpusRequireValue(t, node, "Unparsed WSMP Payload", wire[:cut])
				require.Nil(t, protocolCorpusFindNode(node, "Version"))
			}
		}
	}
	for _, tc := range []struct{ encoded, why string }{
		{"04002000", "unsupported version"},
		{"13002000", "unsupported network subtype"},
		{"03042000", "unsupported transport identifier"},
		{"0300f000", "invalid PSID encoding prefix"},
		{"030020c000", "invalid length or count encoding prefix"},
		{"0300208000", "nonminimal length or count encoding"},
		{"030020010102", "WSM length does not match"},
		{"0300200201", "WSM length does not match"},
		{"0b010f00002000", "known network element requires one value octet"},
		{"0b01fe7f002000", "information element exceeds message boundary"},
		{"0b7f002000", "extension count exceeds message boundary"},
		{"0b8401002000", "extension count exceeds the 1024-element"},
		{"022004020000800000", "legacy network element requires one value octet"},
		{"0220810000", "supplement has no terminating octet"},
		{"02208100028081", "supplement has no terminating octet"},
	} {
		wire := wsmpTestHex(t, tc.encoded)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "wsmp", "WSMP")
		require.ErrorContains(t, err, tc.why, tc.encoded)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "wsmp", "WSMPCarrier")
		protocolCorpusRequireValue(t, node, "Unparsed WSMP Payload", wire)
		require.Nil(t, protocolCorpusFindNode(node, "Version"))
	}
	for _, entry := range []string{"WSMP", "WSMPCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(wsmpTestFixtures(t)[0]), "wsmp", entry)
		require.ErrorContains(t, err, "explicit")
	}
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "wsmp", "WSMPCarrier")
	require.ErrorContains(t, err, "empty carrier")
}

func TestProtocolCorpusWSMPResourceBounds(t *testing.T) {
	// The v3 length encoding supports 16383 octets, with no per-octet nodes.
	message := append([]byte{3, 0, 0x20, 0xbf, 0xff}, bytes.Repeat([]byte{0x5a}, 16383)...)
	node := protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
	wsmpTestField(t, node, "WSM Length", uint64(16383), 24, 40)
	wsmpTestField(t, node, "WSM Data", message[5:], 40, uint64(len(message))*8)
	// Two-octet count and duplicate zero-length unknown information elements.
	message = []byte{0x0b, 0x84, 0x00}
	for index := 0; index < 1024; index++ {
		message = append(message, 0xfe, 0)
	}
	message = append(message, 0, 0x20, 0)
	node = protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
	require.Len(t, wsmpTestAll(node, "Element"), 1024)
	// Maximum bounded v2 message; its length counts all remaining message data.
	message = append([]byte{2, 0x20, 0x80, 0xff, 0xfa}, bytes.Repeat([]byte{0x33}, 65530)...)
	require.Len(t, message, 65535)
	node = protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
	wsmpTestField(t, node, "WSM Length", uint64(65530), 24, 40)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(message, 0)), "wsmp", "WSMP")
	require.ErrorContains(t, err, "65535-byte implementation boundary")
	tooLong := append(bytes.Clone(message), 0)
	carrier := protocolCorpusRequireBoundedRuleParse(t, tooLong, "wsmp", "WSMPCarrier")
	protocolCorpusRequireValue(t, carrier, "Unparsed WSMP Payload", tooLong)
	require.Nil(t, protocolCorpusFindNode(carrier, "Version"))
	// Supplement termination and separate resource limits, not an endless scan.
	for _, count := range []int{256, 257} {
		message = []byte{2, 0x20, 0x81, 0, 0}
		binary.BigEndian.PutUint16(message[3:], uint16(count))
		message = append(message, bytes.Repeat([]byte{0x80}, count-1)...)
		message = append(message, 0)
		if count == 256 {
			node = protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
			require.Len(t, wsmpTestAll(node, "Octet"), count)
		} else {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(message), "wsmp", "WSMP")
			require.ErrorContains(t, err, "256-octet implementation limit")
		}
	}
	for _, count := range []int{256, 257} {
		message = []byte{2, 0x20}
		for index := 0; index < count; index++ {
			message = append(message, 15, 1, byte(index))
		}
		message = append(message, 0x80, 0, 0)
		if count == 256 {
			node = protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
			require.Len(t, wsmpTestAll(node, "Channel Number"), count)
		} else {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(message), "wsmp", "WSMP")
			require.ErrorContains(t, err, "too many legacy extension fields")
		}
	}
	for _, length := range []int{127, 128} {
		message = []byte{0x0b, 1, 0xfe}
		if length == 127 {
			message = append(message, 127)
		} else {
			message = append(message, 0x80, 0x80)
		}
		start := uint64(len(message)) * 8
		data := bytes.Repeat([]byte{0x5a}, length)
		message = append(message, data...)
		message = append(message, 0, 0x20, 0)
		node = protocolCorpusRequireBoundedRuleParse(t, message, "wsmp", "WSMP")
		wsmpTestField(t, node, "Element Length", uint64(length), 24, start)
		wsmpTestField(t, node, "Element Data", data, start, start+uint64(length)*8)
	}
}

func TestProtocolCorpusWSMPTransactionsAndParallel(t *testing.T) {
	fixtures := wsmpTestFixtures(t)
	for _, wire := range [][]byte{fixtures[4], append(bytes.Clone(fixtures[4]), 0), {2}, {3, 0, 0x20, 1}} {
		root, err := base.ParseRule("wsmp.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		reader := bytes.NewReader(wire)
		bitReader := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bitReader, "WSMPCarrier"))
		carrier := base.GetNodeByPath(root, "@WSMPCarrier")
		require.Equal(t, wire, NodeToBytes(carrier))
		if bytes.Equal(wire, fixtures[4]) {
			protocolCorpusRequireValue(t, carrier, "PSID", uint64(0x4081))
			require.Nil(t, protocolCorpusFindNode(carrier, "Unparsed WSMP Payload"))
		} else {
			protocolCorpusRequireValue(t, carrier, "Unparsed WSMP Payload", wire)
			require.Nil(t, protocolCorpusFindNode(carrier, "Version"))
		}
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
		_, err = bitReader.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
	var wait sync.WaitGroup
	errors := make(chan error, 24)
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			wire := fixtures[index%len(fixtures)]
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "wsmp", "WSMPCarrier")
			if err == nil && (!bytes.Equal(NodeToBytes(node), wire) || protocolCorpusFindNode(node, "Version") == nil || protocolCorpusFindNode(node, "Unparsed WSMP Payload") != nil) {
				err = fmt.Errorf("concurrent WSMP record %d lost decoded fields", index)
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

func TestProtocolCorpusWSMPPublicPaths(t *testing.T) {
	for index, wire := range wsmpTestFixtures(t) {
		direct := protocolCorpusRequireBoundedRuleParse(t, wire, "wsmp", "WSMP")
		wantLeaves := wsmpTestLeaves(t, direct, 0)
		for _, path := range []struct {
			name, rule, entry string
			wire              []byte
			offset            uint64
		}{
			{"direct", "wsmp", "WSMP", wire, 0},
			{"carrier", "wsmp", "WSMPCarrier", wire, 0},
			{"ethernet", "ethernet", "Ethernet", wsmpTestEthernet(wire), 14},
		} {
			t.Run(fmt.Sprintf("%s-record-%d", path.name, index+1), func(t *testing.T) {
				node := protocolCorpusRequireBoundedRuleParse(t, path.wire, path.rule, path.entry)
				require.Equal(t, [2]uint64{0, uint64(len(path.wire)) * 8}, wsmpTestSpan(t, node))
				require.Equal(t, wantLeaves, wsmpTestLeaves(t, node, path.offset))
				start := path.offset * 8
				version := uint64(2)
				if index >= 3 {
					version = 3
					start += 5
				}
				wsmpTestField(t, node, "Version", version, start, (path.offset+1)*8)
				require.Nil(t, protocolCorpusFindNode(node, "Unparsed WSMP Payload"))
			})
		}
	}
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-wsmp.pcap")[0]
	node := protocolCorpusRequireBoundedRuleParse(t, original, "ethernet", "Ethernet")
	wsmpTestField(t, node, "Unparsed WSMP Payload", original[14:], 112, uint64(len(original))*8)
	require.Nil(t, protocolCorpusFindNode(node, "Version"))
}

func TestProtocolCorpusWSMPCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-wsmp-valid.pcap")
	wires := wsmpTestFixtures(t)
	require.Len(t, frames, len(wires))
	for index, frame := range frames {
		require.Equal(t, wsmpTestEthernet(wires[index]), frame, "record %d", index+1)
		direct := protocolCorpusRequireBoundedRuleParse(t, wires[index], "wsmp", "WSMP")
		wantLeaves := wsmpTestLeaves(t, direct, 0)
		for _, path := range []struct {
			rule, entry string
			wire        []byte
		}{
			{"wsmp", "WSMP", frame[14:]},
			{"wsmp", "WSMPCarrier", frame[14:]},
			{"ethernet", "Ethernet", frame},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, path.wire, path.rule, path.entry)
			require.NotNil(t, protocolCorpusFindNode(node, "WSM Length"))
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed WSMP Payload"))
			require.Equal(t, [2]uint64{0, uint64(len(path.wire)) * 8}, wsmpTestSpan(t, node))
			offset := uint64(0)
			if path.rule == "ethernet" {
				offset = 14
			}
			require.Equal(t, wantLeaves, wsmpTestLeaves(t, node, offset), "record %d", index+1)
		}
	}
}

func BenchmarkWSMPParseAndResult(b *testing.B) {
	wire, err := hex.DecodeString("0b050f01ac10010c040114170155fe02aabb00c0000103616263")
	if err != nil {
		b.Fatal(err)
	}
	for _, entry := range []string{"WSMP", "WSMPCarrier"} {
		for _, mode := range []struct {
			name                      string
			legacyParse, legacyScalar bool
		}{
			{"Native", false, false},
			{"LegacyParseNativeScalar", true, false},
			{"Legacy", true, true},
		} {
			b.Run(entry+"/"+mode.name, func(b *testing.B) {
				config := map[string]any{"wsmpLegacy": mode.legacyParse, "outScalarLegacy": mode.legacyScalar}
				warm, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "wsmp", config, entry)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := warm.Result(); err != nil {
					b.Fatal(err)
				}
				if !bytes.Equal(wire, NodeToBytes(warm)) {
					b.Fatal("WSMP warmup changed encoded bytes")
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "wsmp", config, entry)
					if err != nil {
						b.Fatal(err)
					}
					if _, err := node.Result(); err != nil {
						b.Fatal(err)
					}
					if !bytes.Equal(wire, NodeToBytes(node)) {
						b.Fatal("WSMP result changed encoded bytes")
					}
				}
			})
		}
	}
}

func TestProtocolCorpusWSMPNativePublicDifferential(t *testing.T) {
	for index, wire := range wsmpTestFixtures(t) {
		for _, path := range []struct {
			rule, entry string
			wire        []byte
			offset      uint64
		}{
			{"wsmp", "WSMP", wire, 0}, {"wsmp", "WSMPCarrier", wire, 0},
			{"ethernet", "Ethernet", wsmpTestEthernet(wire), 14},
		} {
			t.Run(fmt.Sprintf("record-%d/%s", index+1, path.entry), func(t *testing.T) {
				parse := func(legacy bool) *base.Node {
					node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(path.wire), path.rule, map[string]any{"wsmpLegacy": legacy, "outScalarLegacy": legacy}, path.entry)
					require.NoError(t, err)
					require.Equal(t, path.wire, NodeToBytes(node))
					require.Equal(t, [2]uint64{0, uint64(len(path.wire)) * 8}, wsmpTestSpan(t, node))
					return node
				}
				fast, legacy := parse(false), parse(true)
				require.Equal(t, wsmpTestLeaves(t, legacy, path.offset), wsmpTestLeaves(t, fast, path.offset))
				require.Equal(t, NodeToMap(legacy), NodeToMap(fast))
			})
		}
	}
}
