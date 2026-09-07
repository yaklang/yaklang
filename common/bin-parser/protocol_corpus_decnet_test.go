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

func decnetTestHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}

func decnetTestEnvelope(body []byte, padding ...byte) []byte {
	wire := make([]byte, 2, 2+len(body)+len(padding))
	binary.LittleEndian.PutUint16(wire, uint16(len(body)))
	wire = append(wire, body...)
	return append(wire, padding...)
}

func decnetTestEthernet(payload []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x60, 0x03}, payload...)
}

func decnetTestField(t *testing.T, root *base.Node, name string, value any, start, end uint64) *base.Node {
	t.Helper()
	protocolCorpusRequireValue(t, root, name, value)
	node := protocolCorpusFindNode(root, name)
	require.NotNil(t, node)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(node), "field %q", name)
	return node
}

func decnetTestInfo(t *testing.T, node *base.Node, name string, expected any) {
	t.Helper()
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, "field %q has no additionInfo", node.Name)
	require.Equal(t, expected, info[name], "field %q info %q", node.Name, name)
}

func decnetTestFindAll(root *base.Node, name string) []*base.Node {
	var found []*base.Node
	var walk func(*base.Node)
	walk = func(node *base.Node) {
		if node.Name == name && protocolCorpusNodeHasResult(node) {
			found = append(found, node)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return found
}

func decnetTestFindAny(root *base.Node, name string) *base.Node {
	if root.Name == name {
		return root
	}
	for _, child := range root.Children {
		if found := decnetTestFindAny(child, name); found != nil {
			return found
		}
	}
	return nil
}

func decnetTestShortPayload(t *testing.T) []byte {
	t.Helper()
	return decnetTestHex(t, "0900020104020400240100")
}

func decnetTestLongPayload(t *testing.T) []byte {
	t.Helper()
	return decnetTestHex(t, "1800060000aa00040001040000aa000400020400000000240100")
}

func decnetTestVerificationPayload(t *testing.T) []byte {
	t.Helper()
	return decnetTestHex(t, "0700820003020401aa")
}

func decnetTestL1Payload(t *testing.T) []byte {
	t.Helper()
	return decnetTestHex(t, "0c00070204000100010001040404")
}

func decnetTestEndnodePayload(t *testing.T) []byte {
	t.Helper()
	return decnetTestHex(t, "22000d020000aa0004000a04033240000000000000000000aa00040000000a000002aaaa")
}

func decnetTestRouteChecksum(segmentBytes []byte) uint16 {
	var sum uint32 = 1
	for offset := 0; offset < len(segmentBytes); offset += 2 {
		sum += uint32(binary.LittleEndian.Uint16(segmentBytes[offset : offset+2]))
		sum = (sum & 0xffff) + (sum >> 16)
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return uint16(sum)
}

func decnetTestRoutingBody(level byte, source uint16, reserved byte, segments ...[]byte) []byte {
	body := []byte{1 | level<<1, byte(source), byte(source >> 8), reserved}
	var checksumInput []byte
	for _, segment := range segments {
		body = append(body, segment...)
		checksumInput = append(checksumInput, segment...)
	}
	checksum := decnetTestRouteChecksum(checksumInput)
	return append(body, byte(checksum), byte(checksum>>8))
}

func decnetTestSegment(start uint16, words ...uint16) []byte {
	segment := []byte{byte(len(words)), byte(len(words) >> 8), byte(start), byte(start >> 8)}
	for _, word := range words {
		segment = append(segment, byte(word), byte(word>>8))
	}
	return segment
}

func decnetTestRouterHelloBody(logicalEthernets ...[]byte) []byte {
	result := []byte{
		0x0b, 2, 1, 2,
		0xaa, 0, 4, 0, 0x2a, 4,
		0xa2, 0xdc, 5, 0x77, 9, 15, 0, 0x99,
	}
	length := 0
	for _, item := range logicalEthernets {
		length += len(item)
	}
	result = append(result, byte(length))
	for _, item := range logicalEthernets {
		result = append(result, item...)
	}
	return result
}

func decnetTestLogicalEthernet(name []byte, states ...[]byte) []byte {
	if len(name) != 7 {
		panic("DECnet logical Ethernet names are seven bytes")
	}
	result := append([]byte(nil), name...)
	result = append(result, byte(len(states)*7))
	for _, state := range states {
		if len(state) != 7 {
			panic("DECnet router states are seven bytes")
		}
		result = append(result, state...)
	}
	return result
}

func TestProtocolCorpusDECnetDataFields(t *testing.T) {
	short := decnetTestShortPayload(t)
	long := decnetTestLongPayload(t)

	for _, tc := range []struct {
		name    string
		payload []byte
		check   func(*testing.T, *base.Node, uint64)
	}{
		{"short", short, func(t *testing.T, node *base.Node, offset uint64) {
			body := offset + 2
			decnetTestField(t, node, "Octet Count", uint64(9), offset*8, (offset+2)*8)
			decnetTestField(t, node, "Padding Flag", uint64(0), body*8, body*8+1)
			decnetTestField(t, node, "Future Version", uint64(0), body*8+1, body*8+2)
			decnetTestField(t, node, "Return to Sender", uint64(0), body*8+3, body*8+4)
			decnetTestField(t, node, "Return Requested", uint64(0), body*8+4, body*8+5)
			decnetTestField(t, node, "Format", uint64(2), body*8+5, body*8+8)
			destination := decnetTestField(t, node, "Destination Node", uint64(0x0401), (body+1)*8, (body+3)*8)
			source := decnetTestField(t, node, "Source Node", uint64(0x0402), (body+3)*8, (body+5)*8)
			decnetTestInfo(t, destination, "Area", 1)
			decnetTestInfo(t, destination, "Node", 1)
			decnetTestInfo(t, source, "Area", 1)
			decnetTestInfo(t, source, "Node", 2)
			decnetTestField(t, node, "Visit Count", uint64(0), (body+5)*8+2, (body+6)*8)
			decnetTestField(t, node, "NSP Payload", []byte{0x24, 1, 0}, (body+6)*8, (body+9)*8)
			decnetTestInfo(t, decnetTestFindAny(node, "Short Data Packet"), "NSP Present", true)
			decnetTestInfo(t, decnetTestFindAny(node, "Short Data Packet"), "NSP Decoded", false)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed DECnet Payload"))
		}},
		{"long", long, func(t *testing.T, node *base.Node, offset uint64) {
			body := offset + 2
			decnetTestField(t, node, "Octet Count", uint64(24), offset*8, (offset+2)*8)
			decnetTestField(t, node, "Format", uint64(6), body*8+5, body*8+8)
			decnetTestField(t, node, "Intra-Ethernet", uint64(0), body*8+2, body*8+3)
			destination := decnetTestField(t, node, "Destination ID", []byte{0xaa, 0, 4, 0, 1, 4}, (body+3)*8, (body+9)*8)
			source := decnetTestField(t, node, "Source ID", []byte{0xaa, 0, 4, 0, 2, 4}, (body+11)*8, (body+17)*8)
			decnetTestInfo(t, destination, "HIORD Valid", true)
			decnetTestInfo(t, destination, "Area", 1)
			decnetTestInfo(t, destination, "Node", 1)
			decnetTestInfo(t, source, "HIORD Valid", true)
			decnetTestInfo(t, source, "Area", 1)
			decnetTestInfo(t, source, "Node", 2)
			decnetTestField(t, node, "Next Level 2 Router", uint64(0), (body+17)*8, (body+18)*8)
			decnetTestField(t, node, "Visit Count", uint64(0), (body+18)*8, (body+19)*8)
			decnetTestField(t, node, "Service Class", uint64(0), (body+19)*8, (body+20)*8)
			decnetTestField(t, node, "Protocol Type", uint64(0), (body+20)*8, (body+21)*8)
			decnetTestField(t, node, "NSP Payload", []byte{0x24, 1, 0}, (body+21)*8, (body+24)*8)
			decnetTestInfo(t, decnetTestFindAny(node, "Long Data Packet"), "NSP Present", true)
			decnetTestInfo(t, decnetTestFindAny(node, "Long Data Packet"), "NSP Decoded", false)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []struct {
				name         string
				input        []byte
				rule, entry  string
				payloadStart uint64
			}{
				{"direct", tc.payload, "decnet", "DECnet", 0},
				{"carrier", tc.payload, "decnet", "DECnetCarrier", 0},
				{"ethernet", decnetTestEthernet(tc.payload), "ethernet", "Ethernet", 14},
			} {
				t.Run(path.name, func(t *testing.T) {
					node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
					tc.check(t, node, path.payloadStart)
				})
			}
		})
	}

	t.Run("counted message with link padding", func(t *testing.T) {
		wire := append(bytes.Clone(short), 0xde, 0xad, 0xbe, 0xef)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Octet Count", uint64(9), 0, 16)
		decnetTestField(t, node, "NSP Payload", []byte{0x24, 1, 0}, 64, 88)
		decnetTestField(t, node, "Link Padding", []byte{0xde, 0xad, 0xbe, 0xef}, 88, 120)
	})

	for _, tc := range []struct {
		name, packet string
		body         []byte
	}{
		{"short header only", "Short Data Packet", decnetTestHex(t, "020104020400")},
		{"long header only", "Long Data Packet", decnetTestHex(t, "060000aa00040001040000aa000400020400000000")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := decnetTestEnvelope(tc.body)
			for _, path := range []struct {
				name, rule, entry string
				input             []byte
			}{
				{"direct", "decnet", "DECnet", wire},
				{"carrier", "decnet", "DECnetCarrier", wire},
			} {
				t.Run(path.name, func(t *testing.T) {
					node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
					packet := decnetTestFindAny(node, tc.packet)
					require.NotNil(t, packet)
					decnetTestInfo(t, packet, "NSP Present", false)
					decnetTestInfo(t, packet, "NSP Decoded", false)
					require.Nil(t, protocolCorpusFindNode(node, "NSP Payload"))
					require.Nil(t, protocolCorpusFindNode(node, "Unparsed DECnet Payload"))
				})
			}
		})
	}
}

func TestProtocolCorpusDECnetControlFields(t *testing.T) {
	t.Run("padded verification", func(t *testing.T) {
		wire := decnetTestVerificationPayload(t)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Padding Present", uint64(1), 16, 17)
		decnetTestField(t, node, "Padding Length", uint64(2), 17, 24)
		decnetTestField(t, node, "Padding Bytes", []byte{0}, 24, 32)
		decnetTestField(t, node, "Control Type", uint64(1), 36, 39)
		decnetTestField(t, node, "Source Node", uint64(0x0402), 40, 56)
		decnetTestField(t, node, "Function Value Length", uint64(1), 56, 64)
		decnetTestField(t, node, "Function Value", []byte{0xaa}, 64, 72)
	})

	t.Run("initialization", func(t *testing.T) {
		body := decnetTestHex(t, "012a040edc050200000f00027788")
		wire := decnetTestEnvelope(body)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Control Type", uint64(0), 20, 23)
		decnetTestField(t, node, "Source Node", uint64(0x042a), 24, 40)
		decnetTestField(t, node, "Blocking Requested", uint64(1), 44, 45)
		decnetTestField(t, node, "Verification Required", uint64(1), 45, 46)
		decnetTestField(t, node, "Node Type", uint64(2), 46, 48)
		decnetTestField(t, node, "Block Size", uint64(1500), 48, 64)
		decnetTestField(t, node, "Version", uint64(2), 64, 72)
		decnetTestField(t, node, "ECO", uint64(0), 72, 80)
		decnetTestField(t, node, "User ECO", uint64(0), 80, 88)
		decnetTestField(t, node, "Hello Timer", uint64(15), 88, 104)
		decnetTestField(t, node, "Reserved Length", uint64(2), 104, 112)
		decnetTestField(t, node, "Reserved Data", []byte{0x77, 0x88}, 112, 128)
	})

	t.Run("hello and test", func(t *testing.T) {
		wire := decnetTestEnvelope(decnetTestHex(t, "052a0403aaaaaa"))
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Control Type", uint64(2), 20, 23)
		decnetTestField(t, node, "Source Node", uint64(0x042a), 24, 40)
		decnetTestField(t, node, "Test Data Length", uint64(3), 40, 48)
		decnetTestField(t, node, "Test Data", []byte{0xaa, 0xaa, 0xaa}, 48, 72)
	})

	t.Run("level 1 routing", func(t *testing.T) {
		wire := decnetTestL1Payload(t)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Control Type", uint64(3), 20, 23)
		decnetTestField(t, node, "Source Node", uint64(0x0402), 24, 40)
		decnetTestField(t, node, "Entry Count", uint64(1), 48, 64)
		decnetTestField(t, node, "Start ID", uint64(1), 64, 80)
		decnetTestField(t, node, "Cost Low", uint64(1), 80, 88)
		reserved := decnetTestFindAll(node, "Reserved")
		require.Len(t, reserved, 3)
		value, err := reserved[2].Result()
		require.NoError(t, err)
		require.Equal(t, uint64(0), uintVal(t, value))
		require.Equal(t, [2]uint64{88, 89}, stream_parser.GetNodeResultPos(reserved[2]))
		decnetTestField(t, node, "Hops", uint64(1), 89, 94)
		decnetTestField(t, node, "Cost High", uint64(0), 94, 96)
		decnetTestField(t, node, "Checksum", uint64(0x0404), 96, 112)
		infoNode := protocolCorpusFindNode(node, "Entry")
		require.NotNil(t, infoNode)
		decnetTestInfo(t, infoNode, "Cost", 1)
		decnetTestInfo(t, infoNode, "Hops", 1)
	})

	t.Run("level 2 routing multiple entries", func(t *testing.T) {
		body := decnetTestRoutingBody(4, 0x0803, 0x7e,
			decnetTestSegment(1, 0x080a, 0x94ff),
			decnetTestSegment(9, 0x7fff),
		)
		wire := decnetTestEnvelope(append([]byte{0x83, 0x55, 0xaa}, body...))
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Padding Length", uint64(3), 17, 24)
		decnetTestField(t, node, "Control Type", uint64(4), 44, 47)
		entryCounts := decnetTestFindAll(node, "Entry Count")
		require.Len(t, entryCounts, 2)
		for index, want := range []uint64{2, 1} {
			value, err := entryCounts[index].Result()
			require.NoError(t, err)
			require.Equal(t, want, uintVal(t, value))
		}
		hops := decnetTestFindAll(node, "Hops")
		require.Len(t, hops, 3)
		for index, want := range []uint64{2, 5, 31} {
			value, err := hops[index].Result()
			require.NoError(t, err)
			require.Equal(t, want, uintVal(t, value))
		}
	})

	t.Run("router hello multiple logical Ethernets", func(t *testing.T) {
		first := decnetTestLogicalEthernet(
			[]byte{1, 2, 3, 4, 5, 6, 7},
			[]byte{0xaa, 0, 4, 0, 1, 4, 0x85},
		)
		second := decnetTestLogicalEthernet([]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17})
		wire := decnetTestEnvelope(decnetTestRouterHelloBody(first, second))
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Control Type", uint64(5), 20, 23)
		decnetTestField(t, node, "Version", uint64(2), 24, 32)
		decnetTestField(t, node, "ECO", uint64(1), 32, 40)
		decnetTestField(t, node, "User ECO", uint64(2), 40, 48)
		system := decnetTestField(t, node, "System ID", []byte{0xaa, 0, 4, 0, 0x2a, 4}, 48, 96)
		decnetTestInfo(t, system, "HIORD Valid", true)
		decnetTestField(t, node, "Node Type", uint64(2), 102, 104)
		decnetTestField(t, node, "Block Size", uint64(1500), 104, 120)
		decnetTestField(t, node, "Priority", uint64(0x77), 120, 128)
		decnetTestField(t, node, "Area", uint64(9), 128, 136)
		decnetTestField(t, node, "Hello Timer", uint64(15), 136, 152)
		decnetTestField(t, node, "MPD", uint64(0x99), 152, 160)
		decnetTestField(t, node, "E-List Length", uint64(23), 160, 168)
		logicalNames := decnetTestFindAll(node, "Logical Ethernet Name")
		require.Len(t, logicalNames, 2)
		decnetTestField(t, node, "Router State List Length", uint64(7), 224, 232)
		decnetTestField(t, node, "Router ID", []byte{0xaa, 0, 4, 0, 1, 4}, 232, 280)
		decnetTestField(t, node, "Known Two-Way", uint64(1), 280, 281)
		priorities := decnetTestFindAll(node, "Priority")
		require.Len(t, priorities, 2)
		statePriority, err := priorities[1].Result()
		require.NoError(t, err)
		require.Equal(t, uint64(5), uintVal(t, statePriority))
	})

	t.Run("endnode hello", func(t *testing.T) {
		wire := decnetTestEndnodePayload(t)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "decnet", "DECnet")
		decnetTestField(t, node, "Control Type", uint64(6), 20, 23)
		decnetTestField(t, node, "Version", uint64(2), 24, 32)
		decnetTestField(t, node, "System ID", []byte{0xaa, 0, 4, 0, 0x0a, 4}, 48, 96)
		decnetTestField(t, node, "Node Type", uint64(3), 102, 104)
		decnetTestField(t, node, "Block Size", uint64(0x4032), 104, 120)
		decnetTestField(t, node, "Verification Seed", make([]byte, 8), 128, 192)
		decnetTestField(t, node, "Neighbor ID", []byte{0xaa, 0, 4, 0, 0, 0}, 192, 240)
		decnetTestField(t, node, "Hello Timer", uint64(10), 240, 256)
		decnetTestField(t, node, "MPD", uint64(0), 256, 264)
		decnetTestField(t, node, "Test Data Length", uint64(2), 264, 272)
		decnetTestField(t, node, "Test Data", []byte{0xaa, 0xaa}, 272, 288)
	})
}

func TestProtocolCorpusDECnetReceiveVariants(t *testing.T) {
	// Received reserved fields are retained and ignored.  The mandatory HIORD
	// portions of both long-format IDs remain canonical.
	body := decnetTestHex(t, "3e9182aa00040001047364aa00040002045a87a542deadbeef")
	node := protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(body), "decnet", "DECnet")
	decnetTestField(t, node, "Intra-Ethernet", uint64(1), 18, 19)
	decnetTestField(t, node, "Return to Sender", uint64(1), 19, 20)
	decnetTestField(t, node, "Return Requested", uint64(1), 20, 21)
	decnetTestField(t, node, "Destination Area", uint64(0x91), 24, 32)
	decnetTestField(t, node, "Destination Subarea", uint64(0x82), 32, 40)
	decnetTestField(t, node, "Next Level 2 Router", uint64(0x5a), 152, 160)
	decnetTestField(t, node, "Visit Count", uint64(0x87), 160, 168)
	decnetTestField(t, node, "Service Class", uint64(0xa5), 168, 176)
	decnetTestField(t, node, "Protocol Type", uint64(0x42), 176, 184)
	decnetTestInfo(t, protocolCorpusFindNode(node, "Destination ID"), "HIORD Valid", true)
	decnetTestInfo(t, protocolCorpusFindNode(node, "Source ID"), "HIORD Valid", true)

	short := decnetTestHex(t, "32010e2a04c5")
	node = protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(short), "decnet", "DECnet")
	reserved := decnetTestFindAll(node, "Reserved")
	require.Len(t, reserved, 2)
	for index, want := range []struct {
		value      uint64
		start, end uint64
	}{{1, 18, 19}, {3, 56, 58}} {
		value, err := reserved[index].Result()
		require.NoError(t, err)
		require.Equal(t, want.value, uintVal(t, value))
		require.Equal(t, [2]uint64{want.start, want.end}, stream_parser.GetNodeResultPos(reserved[index]))
	}
	decnetTestField(t, node, "Visit Count", uint64(5), 58, 64)

	// Control bits 4..6 and image contents are receive-side reserved data, not
	// the data-packet future-version bit or a required all-zero value.
	verification := decnetTestHex(t, "73020402ff00")
	node = protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(verification), "decnet", "DECnet")
	decnetTestField(t, node, "Reserved", uint64(7), 17, 20)
	decnetTestField(t, node, "Function Value", []byte{0xff, 0}, 48, 64)
}

func TestProtocolCorpusDECnetOriginalAndMalformedBoundaries(t *testing.T) {
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-decnet.pcap")[0]
	require.Equal(t, decnetTestEthernet(decnetTestHex(t, "02000000000000000000000000000000")), original)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original[14:]), "decnet", "DECnet")
	require.ErrorContains(t, err, "decnet: unsupported data-packet format")

	valid := [][]byte{
		decnetTestShortPayload(t), decnetTestLongPayload(t), decnetTestVerificationPayload(t),
		decnetTestL1Payload(t), decnetTestEndnodePayload(t),
		decnetTestEnvelope(decnetTestHex(t, "012a040edc050200000f00027788")),
		decnetTestEnvelope(decnetTestHex(t, "052a0403aaaaaa")),
		decnetTestEnvelope(decnetTestRoutingBody(4, 0x0803, 0, decnetTestSegment(1, 0x080a, 0x94ff))),
		decnetTestEnvelope(decnetTestRouterHelloBody(decnetTestLogicalEthernet(make([]byte, 7)))),
	}
	for fixtureIndex, wire := range valid {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "decnet", "DECnet")
			require.Errorf(t, err, "fixture %d prefix %d", fixtureIndex, cut)
		}
	}

	badChecksum := bytes.Clone(decnetTestL1Payload(t))
	badChecksum[len(badChecksum)-1] ^= 1
	badDestinationHIORD := bytes.Clone(decnetTestLongPayload(t))
	badDestinationHIORD[5] ^= 1
	badSourceHIORD := bytes.Clone(decnetTestLongPayload(t))
	badSourceHIORD[13] ^= 1
	zeroSegment := decnetTestEnvelope(decnetTestHex(t, "070204000000010000000000"))
	level2Range := decnetTestEnvelope(decnetTestRoutingBody(4, 0x0402, 0, decnetTestSegment(64, 0x0401)))
	bad := []struct {
		wire []byte
		want string
	}{
		{original[14:], "unsupported data-packet format"},
		{[]byte{0, 0, 2}, "zero routing-message octet count"},
		{[]byte{2, 0, 2}, "octet count exceeds carrier boundary"},
		{decnetTestEnvelope([]byte{0x80, 2}), "zero optional-padding length"},
		{decnetTestEnvelope([]byte{0x81}), "optional padding consumes the message"},
		{decnetTestEnvelope(append([]byte{0x81}, decnetTestHex(t, "012a040edc050200000f0000")...)), "initialization message cannot be padded"},
		{decnetTestEnvelope(append([]byte{0x81}, decnetTestHex(t, "820104020400")...)), "nested short-data padding flag"},
		{decnetTestEnvelope(decnetTestHex(t, "420104020400")), "unsupported future data-packet version"},
		{decnetTestEnvelope([]byte{0x0f}), "reserved control-message type"},
		{decnetTestEnvelope(decnetTestHex(t, "0201040204")), "short data header requires six bytes"},
		{decnetTestEnvelope(append([]byte{6}, make([]byte, 19)...)), "long data header requires twenty-one bytes"},
		{badDestinationHIORD, "long destination ID does not use HIORD"},
		{badSourceHIORD, "long source ID does not use HIORD"},
		{badChecksum, "routing-message checksum mismatch"},
		{zeroSegment, "routing segment has zero entries"},
		{level2Range, "level-2 routing segment exceeds area range"},
		{decnetTestEnvelope(decnetTestHex(t, "03020402aa")), "function-value length mismatch"},
		{decnetTestEnvelope(append(decnetTestHex(t, "03020441"), make([]byte, 65)...)), "function value exceeds 64 bytes"},
		{decnetTestEnvelope(append(decnetTestHex(t, "052a0481"), make([]byte, 129)...)), "test data exceeds 128 bytes"},
	}
	for index, tc := range bad {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tc.wire), "decnet", "DECnet")
		require.ErrorContainsf(t, err, tc.want, "invalid %d", index)
		node := protocolCorpusRequireBoundedRuleParse(t, tc.wire, "decnet", "DECnetCarrier")
		protocolCorpusRequireValue(t, node, "Unparsed DECnet Payload", tc.wire)
		require.Nil(t, protocolCorpusFindNode(node, "Octet Count"))
		frame := decnetTestEthernet(tc.wire)
		imported := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		protocolCorpusRequireValue(t, imported, "Unparsed DECnet Payload", tc.wire)
		require.Equal(t, [2]uint64{14 * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(imported, "Unparsed DECnet Payload")))
	}
}

func TestProtocolCorpusDECnetCarrierTransactions(t *testing.T) {
	for _, wire := range [][]byte{
		decnetTestShortPayload(t),
		decnetTestVerificationPayload(t),
		decnetTestHex(t, "0200000000000000000000000000"),
		{0, 0, 2},
	} {
		root, err := base.ParseRule("decnet.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		reader := bytes.NewReader(wire)
		bitReader := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bitReader, "DECnetCarrier"))
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
		_, err = bitReader.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "decnet", "DECnetCarrier")
	require.ErrorContains(t, err, "decnet: empty carrier has no message")
	for _, entry := range []string{"DECnet", "DECnetCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(decnetTestShortPayload(t)), "decnet", entry)
		require.ErrorContains(t, err, "explicit")
	}
}

func TestProtocolCorpusDECnetResourceBounds(t *testing.T) {
	maxShortBody := append(decnetTestHex(t, "020104020400"), make([]byte, 65535-6)...)
	protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(maxShortBody), "decnet", "DECnet")
	overEnvelope := append(decnetTestEnvelope(maxShortBody), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(overEnvelope), "decnet", "DECnet")
	require.ErrorContains(t, err, "envelope exceeds the supported 65537-byte boundary")

	maxPaddingBody := append([]byte{0xff}, make([]byte, 126)...)
	maxPaddingBody = append(maxPaddingBody, decnetTestHex(t, "020104020400")...)
	protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(maxPaddingBody), "decnet", "DECnet")

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"initialization image 64", append(decnetTestHex(t, "012a040edc050200000f0040"), make([]byte, 64)...)},
		{"verification image 64", append(decnetTestHex(t, "03020440"), make([]byte, 64)...)},
		{"test image 128", append(decnetTestHex(t, "052a0480"), make([]byte, 128)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(tc.body), "decnet", "DECnet")
		})
	}

	overInitialization := append(decnetTestHex(t, "012a040edc050200000f0041"), make([]byte, 65)...)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(decnetTestEnvelope(overInitialization)), "decnet", "DECnet")
	require.ErrorContains(t, err, "initialization reserved image exceeds 64 bytes")

	state := []byte{0xaa, 0, 4, 0, 1, 4, 1}
	states := make([][]byte, 28)
	for i := range states {
		states[i] = bytes.Clone(state)
		states[i][4] = byte(i + 1)
	}
	logical := [][]byte{decnetTestLogicalEthernet(make([]byte, 7), states...)}
	for i := 0; i < 5; i++ {
		logical = append(logical, decnetTestLogicalEthernet(bytes.Repeat([]byte{byte(i + 1)}, 7)))
	}
	routerBody := decnetTestRouterHelloBody(logical...)
	require.Equal(t, 18+1+244, len(routerBody))
	protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(routerBody), "decnet", "DECnet")
	overRouter := bytes.Clone(routerBody)
	overRouter[18] = 245
	overRouter = append(overRouter, 0)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(decnetTestEnvelope(overRouter)), "decnet", "DECnet")
	require.ErrorContains(t, err, "invalid router E-list length")

	endnode := decnetTestHex(t, "0d020000aa0004000a04033240000000000000000000aa00040000000a000080")
	endnode = append(endnode, bytes.Repeat([]byte{0xaa}, 128)...)
	protocolCorpusRequireBoundedRuleParse(t, decnetTestEnvelope(endnode), "decnet", "DECnet")
	overEndnode := decnetTestHex(t, "0d020000aa0004000a04033240000000000000000000aa00040000000a000081")
	overEndnode = append(overEndnode, bytes.Repeat([]byte{0xaa}, 129)...)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(decnetTestEnvelope(overEndnode)), "decnet", "DECnet")
	require.ErrorContains(t, err, "endnode test data exceeds 128 bytes")

	entries := make([]uint16, 1024)
	for i := range entries {
		entries[i] = uint16(i & 0x7fff)
	}
	maxRouting := decnetTestEnvelope(decnetTestRoutingBody(3, 0x0402, 0, decnetTestSegment(0, entries...)))
	node := protocolCorpusRequireBoundedRuleParse(t, maxRouting, "decnet", "DECnet")
	segments := protocolCorpusFindNode(node, "Segments")
	require.NotNil(t, segments)
	decnetTestInfo(t, segments, "Entry Count", 1024)

	tooMany := make([]uint16, 1025)
	badRouting := decnetTestEnvelope(decnetTestRoutingBody(3, 0x0402, 0, decnetTestSegment(0, tooMany...)))
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(badRouting), "decnet", "DECnet")
	require.ErrorContains(t, err, "too many routing entries")
}

func TestProtocolCorpusDECnetCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-decnet-valid.pcap")
	expected := [][]byte{
		decnetTestShortPayload(t), decnetTestLongPayload(t), decnetTestVerificationPayload(t),
		decnetTestL1Payload(t), decnetTestEndnodePayload(t),
	}
	require.Len(t, frames, len(expected))
	for index, frame := range frames {
		require.GreaterOrEqualf(t, len(frame), 14, "frame %d", index+1)
		require.Equalf(t, expected[index], frame[14:], "frame %d payload", index+1)
		for _, path := range []struct {
			input       []byte
			rule, entry string
		}{
			{frame[14:], "decnet", "DECnet"},
			{frame[14:], "decnet", "DECnetCarrier"},
			{frame, "ethernet", "Ethernet"},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, path.input, path.rule, path.entry)
			require.Nilf(t, protocolCorpusFindNode(node, "Unparsed DECnet Payload"), "frame %d", index+1)
			protocolCorpusRequireValue(t, node, "Octet Count", uint64(len(expected[index])-2))
		}
	}
}

func TestProtocolCorpusDECnetParallelIsolation(t *testing.T) {
	fixtures := [][]byte{
		decnetTestShortPayload(t), decnetTestLongPayload(t), decnetTestVerificationPayload(t),
		decnetTestL1Payload(t), decnetTestEndnodePayload(t),
	}
	for worker := 0; worker < 10; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for repeat := 0; repeat < 3; repeat++ {
				wire := fixtures[(worker+repeat)%len(fixtures)]
				node := protocolCorpusRequireBoundedRuleParse(t, decnetTestEthernet(wire), "ethernet", "Ethernet")
				protocolCorpusRequireValue(t, node, "Octet Count", uint64(len(wire)-2))
				require.Nil(t, protocolCorpusFindNode(node, "Unparsed DECnet Payload"))
			}
		})
	}
}
