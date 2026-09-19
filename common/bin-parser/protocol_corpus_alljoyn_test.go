package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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

const alljoynCorpusRule = "application-layer.alljoyn"

// These literals are independently assembled from StringData/WhoHas/IsAt in
// the pinned primary implementation, not from this rule or a serializer under
// test. They cover legacy discovery records, not the AllJoyn message bus.
// https://github.com/alljoyn/core-alljoyn/blob/103b0833801f8e36e648a6d66313518356b0218d/alljoyn_core/router/ns/IpNsProtocol.cc
func alljoynTestFixtures(t testing.TB) [][]byte {
	t.Helper()
	var wires [][]byte
	for _, encoded := range []string{
		"000100008901126f72672e6578616d706c652e53656e736f72",
		"000001787b0126e3c000020a20010db8000000000000000000000010203030313132323333343435353636373738383939616162626363646465656666126f72672e6578616d706c652e53656e736f72",
		"210102ff8401126f72672e6578616d706c652e53656e736f727a010004c000020a26e320010db800000000000000000000001026e3203030313132323333343435353636373738383939616162626363646465656666126f72672e6578616d706c652e53656e736f7275010100c633641426e220010db800000000000000000000002026e2203030313132323333343435353636373738383939616162626363646465656666116f72672e6578616d706c652e436c6f636b",
	} {
		wires = append(wires, alljoynTestHex(t, encoded))
	}
	return wires
}

func alljoynTestHex(t testing.TB, encoded string) []byte {
	t.Helper()
	wire, err := hex.DecodeString(encoded)
	require.NoError(t, err)
	return wire
}

type alljoynTestLeaf struct {
	Name  string
	Span  [2]uint64
	Value any
}

func alljoynTestLeaves(t *testing.T, root *base.Node, offset uint64) []alljoynTestLeaf {
	t.Helper()
	var leaves []alljoynTestLeaf
	var visit func(*base.Node)
	visit = func(node *base.Node) {
		if stream_parser.NodeHasResult(node) {
			span := stream_parser.GetNodeResultPos(node)
			if span[0] < offset*8 {
				return
			}
			result, err := node.Result()
			require.NoError(t, err)
			require.Same(t, node, result.Origin)
			leaves = append(leaves, alljoynTestLeaf{node.Name, [2]uint64{span[0] - offset*8, span[1] - offset*8}, result.Value})
			return
		}
		for _, child := range node.Children {
			if protocolCorpusNodeHasResult(child) {
				require.Same(t, node, child.Cfg.GetItem(base.CfgParent), child.Name)
				require.Same(t, node.Ctx.GetItem("buffer"), child.Ctx.GetItem("buffer"), child.Name)
				require.Same(t, node.Ctx.GetItem("writer"), child.Ctx.GetItem("writer"), child.Name)
			}
			visit(child)
		}
	}
	visit(root)
	return leaves
}

// All terminal values, their Go types and global bit extents are checked. The
// expected stream is stated from the primary layout and named constants; it
// is never decoded from the fixture or obtained from the parser under test.
func alljoynTestExpected(t *testing.T, record int) []alljoynTestLeaf {
	t.Helper()
	var out []alljoynTestLeaf
	var cursor uint64
	field := func(name string, value any, bits uint64) {
		out = append(out, alljoynTestLeaf{name, [2]uint64{cursor, cursor + bits}, value})
		cursor += bits
	}
	u8 := func(name string, value uint8, bits uint64) { field(name, value, bits) }
	u16 := func(name string, value uint16) { field(name, value, 16) }
	text := func(value string) {
		u8("Byte Length", uint8(len(value)), 8)
		if len(value) > 0 {
			field("Text", value, uint64(len(value))*8)
		}
	}
	address := func(name, encoded string) { wire := alljoynTestHex(t, encoded); field(name, wire, uint64(len(wire))*8) }
	header := func(sender, version, questions, answers, timer uint8) {
		u8("Sender Version", sender, 4)
		u8("Message Version", version, 4)
		u8("Question Count", questions, 8)
		u8("Answer Count", answers, 8)
		u8("Timer", timer, 8)
	}
	question := func(version, tcp, udp, v6, v4 uint8) {
		u8("Question Type", 2, 2)
		if version == 0 {
			u8("Question Reserved", 0, 2)
			u8("T Flag", tcp, 1)
			u8("U Flag", udp, 1)
			u8("S Flag", v6, 1)
			u8("F Flag", v4, 1)
		} else {
			u8("Question Reserved High", 0, 2)
			u8("Question Reserved Bit 3", tcp, 1)
			u8("U Compatibility Flag", udp, 1)
			u8("Question Reserved Low", v6<<1|v4, 2)
		}
		u8("Name Count", 1, 8)
		text("org.example.Sensor")
	}
	answerPrefix := func() { u8("Answer Type", 1, 2); u8("GUID Present", 1, 1); u8("Complete Flag", 1, 1) }
	guid := func() { text("00112233445566778899aabbccddeeff") }
	switch record {
	case 0:
		header(0, 0, 1, 0, 0)
		question(0, 1, 0, 0, 1)
	case 1:
		header(0, 0, 0, 1, 120)
		answerPrefix()
		u8("TCP Flag", 1, 1)
		u8("UDP Flag", 0, 1)
		u8("IPv6 Present", 1, 1)
		u8("IPv4 Present", 1, 1)
		u8("Name Count", 1, 8)
		u16("Port", 9955)
		// Official Serialize and Deserialize both place IPv4 before IPv6.
		address("IPv4 Address", "c000020a")
		address("IPv6 Address", "20010db8000000000000000000000010")
		guid()
		text("org.example.Sensor")
	case 2:
		header(2, 1, 1, 2, 255)
		question(1, 0, 1, 0, 0)
		answerPrefix()
		u8("Reliable IPv4 Present", 1, 1)
		u8("Unreliable IPv4 Present", 0, 1)
		u8("Reliable IPv6 Present", 1, 1)
		u8("Unreliable IPv6 Present", 0, 1)
		u8("Name Count", 1, 8)
		u16("Transport Mask", 4)
		address("Address", "c000020a")
		u16("Port", 9955)
		address("Address", "20010db8000000000000000000000010")
		u16("Port", 9955)
		guid()
		text("org.example.Sensor")
		answerPrefix()
		u8("Reliable IPv4 Present", 0, 1)
		u8("Unreliable IPv4 Present", 1, 1)
		u8("Reliable IPv6 Present", 0, 1)
		u8("Unreliable IPv6 Present", 1, 1)
		u8("Name Count", 1, 8)
		u16("Transport Mask", 256)
		address("Address", "c6336414")
		u16("Port", 9954)
		address("Address", "20010db8000000000000000000000020")
		u16("Port", 9954)
		guid()
		text("org.example.Clock")
	default:
		t.Fatalf("missing AllJoyn expected record %d", record)
	}
	return out
}

func alljoynTestParse(t *testing.T, wire []byte, rule, entry string) *base.Node {
	t.Helper()
	node := protocolCorpusRequireBoundedRuleParse(t, wire, rule, entry)
	require.Equal(t, wire, NodeToBytes(node))
	return node
}

func alljoynTestInfo(t *testing.T, node *base.Node) map[string]any {
	t.Helper()
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, node.Name)
	return info
}

func TestProtocolCorpusAllJoynFields(t *testing.T) {
	for record, wire := range alljoynTestFixtures(t) {
		for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier"} {
			t.Run(fmt.Sprintf("record-%d/%s", record+1, entry), func(t *testing.T) {
				node := alljoynTestParse(t, wire, alljoynCorpusRule, entry)
				require.Equal(t, alljoynTestExpected(t, record), alljoynTestLeaves(t, node, 0))
				header := protocolCorpusFindNode(node, "Sender Version")
				message := header.Cfg.GetItem(base.CfgParent).(*base.Node)
				info := alljoynTestInfo(t, message)
				require.EqualValues(t, 1454, info["Maximum Message Bytes"])
				require.Equal(t, false, info["Discovery Semantics Validated"])
				require.Equal(t, false, info["String Encoding Validated"])
				names := protocolCorpusNodesNamed(node, "Name")
				want := []string{"org.example.Sensor"}
				if record == 2 {
					want = []string{"org.example.Sensor", "org.example.Sensor", "org.example.Clock"}
				}
				require.Len(t, names, len(want))
				for i, name := range names {
					result, err := name.Result()
					require.NoError(t, err)
					require.Same(t, name, result.Origin)
					require.Equal(t, want[i], result.Value)
				}
				for _, names := range protocolCorpusNodesNamed(node, "Names") {
					result, err := names.Result()
					require.NoError(t, err)
					require.True(t, result.IsList())
					require.IsType(t, []any{}, NodeToMap(names))
				}
				for _, guid := range protocolCorpusNodesNamed(node, "GUID") {
					result, err := guid.Result()
					require.NoError(t, err)
					require.Same(t, guid, result.Origin)
					require.Equal(t, "00112233445566778899aabbccddeeff", result.Value)
				}
			})
		}
	}
}

func alljoynTestUDP(wire []byte, source bool) []byte {
	udp := make([]byte, 8+len(wire))
	binary.BigEndian.PutUint16(udp, 40101)
	binary.BigEndian.PutUint16(udp[2:], 9956)
	if source {
		binary.BigEndian.PutUint16(udp, 9956)
		binary.BigEndian.PutUint16(udp[2:], 40101)
	}
	binary.BigEndian.PutUint16(udp[4:], uint16(len(udp)))
	copy(udp[8:], wire)
	return udp
}

func TestProtocolCorpusAllJoynOriginalAndCompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-alljoyn.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "e4a0af02b33e3e0bdec3711338d88b3f830a022b12fc877313f7f723d8dd836c", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	require.Equal(t, alljoynTestHex(t, "02000000000202000000000108004500002800010000401166c20a0000010a0000029ca526e400149ce900000020416c6c4a6f796e00"), frames[0])
	wire := alljoynTestHex(t, "00000020416c6c4a6f796e00")
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), alljoynCorpusRule, "AllJoynNS")
	require.ErrorContains(t, err, "trailing bytes after declared records")
	for _, p := range []struct {
		rule, entry string
		wire        []byte
		offset      uint64
	}{
		{alljoynCorpusRule, "AllJoynNSCarrier", wire, 0}, {"ethernet", "Ethernet", frames[0], 42},
	} {
		node := alljoynTestParse(t, p.wire, p.rule, p.entry)
		raw := protocolCorpusFindNode(node, "Unparsed AllJoyn NS Payload")
		require.NotNil(t, raw)
		protocolCorpusRequireValue(t, raw, raw.Name, wire)
		require.Equal(t, [2]uint64{p.offset * 8, (p.offset + 12) * 8}, stream_parser.GetNodeResultPos(raw))
		require.Nil(t, protocolCorpusFindNode(node, "Sender Version"))
	}
	frames = protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-alljoyn-valid.pcap")
	wires := alljoynTestFixtures(t)
	require.Len(t, frames, len(wires))
	for record, frame := range frames {
		require.Len(t, frame, len(wires[record])+42)
		require.Equal(t, wires[record], frame[42:])
		for _, p := range []struct {
			rule, entry string
			wire        []byte
			offset      uint64
		}{
			{alljoynCorpusRule, "AllJoynNS", frame[42:], 0}, {alljoynCorpusRule, "AllJoynNSCarrier", frame[42:], 0},
			{"user_datagram_protocol", "UDP", frame[34:], 8}, {"user_datagram_protocol", "UDP", alljoynTestUDP(wires[record], true), 8}, {"ethernet", "Ethernet", frame, 42},
		} {
			node := alljoynTestParse(t, p.wire, p.rule, p.entry)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed AllJoyn NS Payload"))
			require.Equal(t, alljoynTestExpected(t, record), alljoynTestLeaves(t, node, p.offset), "record %d %s", record+1, p.entry)
		}
	}
}

func TestProtocolCorpusAllJoynShortPrefixesAndInvalidLengths(t *testing.T) {
	for record, wire := range alljoynTestFixtures(t) {
		for cut := 0; cut < len(wire); cut++ {
			prefix := wire[:cut]
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(prefix), alljoynCorpusRule, "AllJoynNS")
			require.Errorf(t, err, "record %d prefix %d", record+1, cut)
			if cut == 0 {
				continue
			}
			node := alljoynTestParse(t, prefix, alljoynCorpusRule, "AllJoynNSCarrier")
			protocolCorpusRequireValue(t, node, "Unparsed AllJoyn NS Payload", prefix)
			require.Nil(t, protocolCorpusFindNode(node, "Sender Version"))
		}
	}
	for _, tc := range []struct{ wire, why string }{
		{"02000000", "unsupported message version"},
		{"00010000", "record counts exceed message boundary"},
		{"000001000000", "record counts exceed message boundary"},
		{"000100004000", "invalid WHO-HAS type"},
		{"0000010080000000", "invalid IS-AT type"},
		{"01010000840200", "name count exceeds record boundary"},
		{"01010000840105aa", "string length exceeds record boundary"},
		{"010001006000000405aa", "string length exceeds record boundary"},
		{"010001006001000400", "name count exceeds record boundary"},
		{"0100010041000004", ""},
		{"0000000041", "trailing bytes after declared records"},
	} {
		wire := alljoynTestHex(t, tc.wire)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), alljoynCorpusRule, "AllJoynNS")
		require.Error(t, err, tc.wire)
		if tc.why != "" {
			require.ErrorContains(t, err, tc.why)
		}
		node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNSCarrier")
		protocolCorpusRequireValue(t, node, "Unparsed AllJoyn NS Payload", wire)
		require.Nil(t, protocolCorpusFindNode(node, "Sender Version"))
	}
	for _, wire := range alljoynTestFixtures(t) {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0)), alljoynCorpusRule, "AllJoynNS")
		require.ErrorContains(t, err, "trailing bytes after declared records")
	}
}

func TestProtocolCorpusAllJoynVersionsFlagsAndEndpointCombinations(t *testing.T) {
	// Sender version is capability metadata, independent of the wire layout.
	for sender := byte(0); sender < 16; sender++ {
		for version := byte(0); version < 2; version++ {
			wire := []byte{sender<<4 | version, 0, 0, 255}
			node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
			protocolCorpusRequireValue(t, node, "Sender Version", uint64(sender))
			protocolCorpusRequireValue(t, node, "Message Version", uint64(version))
			require.Nil(t, protocolCorpusFindNode(node, "Questions"))
			require.Nil(t, protocolCorpusFindNode(node, "Answers"))
		}
	}
	for version := byte(0); version < 2; version++ {
		for flags := byte(0); flags < 64; flags++ {
			wire := []byte{version, 1, 0, 0, 0x80 | flags, 0}
			node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
			if version == 0 {
				protocolCorpusRequireValue(t, node, "Question Reserved", uint64(flags>>4))
				for bit, name := range []string{"F Flag", "S Flag", "U Flag", "T Flag"} {
					protocolCorpusRequireValue(t, node, name, uint64((flags>>bit)&1))
				}
				require.Nil(t, protocolCorpusFindNode(node, "U Compatibility Flag"))
			} else {
				protocolCorpusRequireValue(t, node, "Question Reserved High", uint64(flags>>4))
				protocolCorpusRequireValue(t, node, "Question Reserved Bit 3", uint64((flags>>3)&1))
				protocolCorpusRequireValue(t, node, "U Compatibility Flag", uint64((flags>>2)&1))
				protocolCorpusRequireValue(t, node, "Question Reserved Low", uint64(flags&3))
				for _, name := range []string{"T Flag", "U Flag", "S Flag", "F Flag"} {
					require.Nil(t, protocolCorpusFindNode(node, name))
				}
			}
			question := protocolCorpusFindNode(node, "Question")
			require.Equal(t, version == 0, alljoynTestInfo(t, question)["Legacy Flags Active"])
			require.Equal(t, version == 1, alljoynTestInfo(t, question)["U Compatibility Flag Active"])
		}
	}
	v4 := alljoynTestHex(t, "c000020a")
	v6 := alljoynTestHex(t, "20010db8000000000000000000000010")
	for flags := byte(0); flags < 16; flags++ {
		wire := []byte{0, 0, 1, 120, 0x40 | flags, 0, 0x26, 0xe3}
		if flags&1 != 0 {
			wire = append(wire, v4...)
		}
		if flags&2 != 0 {
			wire = append(wire, v6...)
		}
		node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
		if flags&1 != 0 {
			protocolCorpusRequireValue(t, node, "IPv4 Address", v4)
		} else {
			require.Nil(t, protocolCorpusFindNode(node, "IPv4 Address"))
		}
		if flags&2 != 0 {
			protocolCorpusRequireValue(t, node, "IPv6 Address", v6)
		} else {
			require.Nil(t, protocolCorpusFindNode(node, "IPv6 Address"))
		}
	}
	for flags := byte(0); flags < 16; flags++ {
		wire := []byte{0x21, 0, 1, 120, 0x40 | flags, 0, 0, 4}
		for i := 0; i < 4; i++ {
			if flags&(8>>i) == 0 {
				continue
			}
			address := v4
			if i >= 2 {
				address = v6
			}
			wire = append(wire, address...)
			wire = append(wire, 0x26, byte(0xe3+i))
		}
		node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
		cursor := uint64(8 * 8)
		for i, name := range []string{"Reliable IPv4", "Unreliable IPv4", "Reliable IPv6", "Unreliable IPv6"} {
			endpoint := protocolCorpusFindNode(node, name)
			if flags&(8>>i) == 0 {
				require.Nil(t, endpoint)
				continue
			}
			require.NotNil(t, endpoint)
			address := v4
			if i >= 2 {
				address = v6
			}
			protocolCorpusRequireValue(t, endpoint, "Address", address)
			protocolCorpusRequireValue(t, endpoint, "Port", uint64(9955+i))
			require.Equal(t, [2]uint64{cursor, cursor + uint64(len(address))*8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(endpoint, "Address")))
			cursor += uint64(len(address)+2) * 8
		}
		require.Equal(t, uint64(len(wire))*8, cursor)
	}
	// Layout decoding preserves unknown and multi-bit masks; daemon policy is
	// separately exposed and does not silently reject codec-compatible input.
	for _, mask := range []uint16{0, 1, 4, 256, 260, 0x8000, 0xffff} {
		wire := []byte{1, 0, 1, 0, 0x40, 0, byte(mask >> 8), byte(mask)}
		node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
		protocolCorpusRequireValue(t, node, "Transport Mask", uint64(mask))
		answer := protocolCorpusFindNode(node, "Answer")
		require.Equal(t, mask != 0 && mask&(mask-1) == 0, alljoynTestInfo(t, answer)["Transport Mask Has Single Bit"])
	}
}

func TestProtocolCorpusAllJoynDuplicatesAndStringOctets(t *testing.T) {
	// Duplicate questions, answers and names are ordered wire items, not maps
	// keyed by their text. The codec permits empty strings and opaque octets.
	wire := alljoynTestHex(t, "010202008402016101618402016101614000000440000004")
	node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
	require.Len(t, protocolCorpusNodesNamed(node, "Question"), 2)
	require.Len(t, protocolCorpusNodesNamed(node, "Answer"), 2)
	names := protocolCorpusNodesNamed(node, "Name")
	require.Len(t, names, 4)
	for _, name := range names {
		result, err := name.Result()
		require.NoError(t, err)
		require.Equal(t, "a", result.Value)
	}
	for _, list := range protocolCorpusNodesNamed(node, "Names") {
		// NodeToMap deliberately retains the encoded child structure, while
		// Name.Result applies the scalar out expression checked above.
		require.Equal(t, []any{
			map[string]any{"Byte Length": uint8(1), "Text": "a"},
			map[string]any{"Byte Length": uint8(1), "Text": "a"},
		}, NodeToMap(list))
	}
	wire = alljoynTestHex(t, "010001006002000400000300ff61")
	node = alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
	protocolCorpusRequireValue(t, node, "GUID", "")
	names = protocolCorpusNodesNamed(node, "Name")
	require.Len(t, names, 2)
	for index, want := range []string{"", "\x00\xffa"} {
		result, err := names[index].Result()
		require.NoError(t, err)
		require.Equal(t, want, result.Value)
	}
	require.Equal(t, [2]uint64{72, 80}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(names[0], "Byte Length")))
	// UTF-8 uses its encoded byte length, not the number of Unicode characters.
	wire = alljoynTestHex(t, "01010000840102c3a9")
	node = alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
	protocolCorpusRequireValue(t, node, "Name", "é")
	protocolCorpusRequireValue(t, node, "Byte Length", uint64(2))
}

func TestProtocolCorpusAllJoynResourceProfile(t *testing.T) {
	// Exactly NS_MESSAGE_MAX: one question containing five 255-byte strings
	// and one 167-byte string. Appending one byte to the final string yields
	// another codec-shaped datagram, rejected solely by the implementation cap.
	wire := []byte{1, 1, 0, 0, 0x84, 6}
	for i := 0; i < 5; i++ {
		wire = append(wire, 255)
		wire = append(wire, bytes.Repeat([]byte{byte('a' + i)}, 255)...)
	}
	wire = append(wire, 167)
	wire = append(wire, bytes.Repeat([]byte{'z'}, 167)...)
	require.Len(t, wire, 1454)
	node := alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
	names := protocolCorpusNodesNamed(node, "Name")
	require.Len(t, names, 6)
	for index, name := range names {
		result, err := name.Result()
		require.NoError(t, err)
		want := 255
		if index == 5 {
			want = 167
		}
		require.Len(t, result.Value.(string), want)
	}
	tooLong := bytes.Clone(wire)
	tooLong[1286] = 168
	tooLong = append(tooLong, 'z')
	reader := newProtocolCorpusBoundedReader(tooLong)
	_, err := parser.ParseBinary(reader, alljoynCorpusRule, "AllJoynNS")
	require.ErrorContains(t, err, "1454-byte implementation profile")
	require.Equal(t, len(tooLong), reader.Len(), "over-limit direct entry must not consume input")
	node = alljoynTestParse(t, tooLong, alljoynCorpusRule, "AllJoynNSCarrier")
	protocolCorpusRequireValue(t, node, "Unparsed AllJoyn NS Payload", tooLong)
	// Exercise all uint8 count maxima independently and together. These are
	// codec boundary vectors, not claims of useful daemon advertisements.
	for _, tc := range []struct{ q, a, n int }{{255, 0, 0}, {0, 255, 0}, {255, 235, 0}, {1, 0, 255}} {
		wire = []byte{1, byte(tc.q), byte(tc.a), 0}
		for i := 0; i < tc.q; i++ {
			wire = append(wire, 0x84, byte(tc.n))
			wire = append(wire, make([]byte, tc.n)...)
		}
		for i := 0; i < tc.a; i++ {
			wire = append(wire, 0x40, 0, 0, 4)
		}
		node = alljoynTestParse(t, wire, alljoynCorpusRule, "AllJoynNS")
		require.Len(t, protocolCorpusNodesNamed(node, "Question"), tc.q)
		require.Len(t, protocolCorpusNodesNamed(node, "Answer"), tc.a)
		require.Len(t, protocolCorpusNodesNamed(node, "Name"), tc.q*tc.n)
	}
}

type alljoynTestBitBoundaryReader struct {
	*bytes.Reader
	bits uint64
}

func (r *alljoynTestBitBoundaryReader) InputBitLength() uint64 { return r.bits }

func TestProtocolCorpusAllJoynBoundaryAndTransactions(t *testing.T) {
	valid := alljoynTestFixtures(t)[2]
	for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), alljoynCorpusRule, entry)
		require.ErrorContains(t, err, "explicit")
		for extra := uint64(1); extra < 8; extra++ {
			reader := &alljoynTestBitBoundaryReader{bytes.NewReader(append(bytes.Clone(valid), 0)), uint64(len(valid))*8 + extra}
			_, err = parser.ParseBinary(reader, alljoynCorpusRule, entry)
			require.ErrorContains(t, err, "explicit byte boundary required")
			require.Equal(t, len(valid)+1, reader.Len())
		}
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), alljoynCorpusRule, "AllJoynNSCarrier")
	require.ErrorContains(t, err, "empty carrier")
	for _, wire := range [][]byte{valid, append(bytes.Clone(valid), 0), {1}, {1, 1, 0, 0, 0x84, 1, 3, 'a'}} {
		root, err := base.ParseRule("application-layer/alljoyn.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		reader := bytes.NewReader(wire)
		bitReader := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bitReader, "AllJoynNSCarrier"))
		node := base.GetNodeByPath(root, "@AllJoynNSCarrier")
		require.Equal(t, wire, NodeToBytes(node))
		if bytes.Equal(wire, valid) {
			require.NotNil(t, protocolCorpusFindNode(node, "Sender Version"))
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed AllJoyn NS Payload"))
		} else {
			protocolCorpusRequireValue(t, node, "Unparsed AllJoyn NS Payload", wire)
			require.Nil(t, protocolCorpusFindNode(node, "Sender Version"))
		}
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bitReader.Recovery(), "no backup")
		require.ErrorContains(t, bitReader.PopBackup(), "no backup")
		_, err = bitReader.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
}

func TestProtocolCorpusAllJoynStructuredGenerationUnsupported(t *testing.T) {
	input := map[string]any{
		"Sender Version": uint8(0), "Message Version": uint8(0),
		"Question Count": uint8(1), "Answer Count": uint8(0), "Timer": uint8(0),
		"Questions": []any{map[string]any{"Question Type": uint8(2), "T Flag": uint8(1), "F Flag": uint8(1), "Name Count": uint8(1), "Names": []any{map[string]any{"Byte Length": uint8(18), "Text": "org.example.Sensor"}}}},
	}
	for _, tc := range []struct{ entry, why string }{
		{"AllJoynNS", "alljoyn-ns: explicit message boundary required"},
		{"AllJoynNSCarrier", "alljoyn-ns: explicit carrier boundary required"},
	} {
		node, err := parser.GenerateBinary(input, alljoynCorpusRule, tc.entry)
		require.Nil(t, node)
		require.ErrorContains(t, err, tc.why)
	}
}

func TestProtocolCorpusAllJoynConcurrentIsolation(t *testing.T) {
	wires := alljoynTestFixtures(t)
	var wait sync.WaitGroup
	errors := make(chan error, 24)
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			wire := bytes.Clone(wires[index%len(wires)])
			valid := index%2 == 0
			if !valid {
				wire = append(wire, 0)
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), alljoynCorpusRule, "AllJoynNSCarrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Sender Version") != nil) != valid || (protocolCorpusFindNode(node, "Unparsed AllJoyn NS Payload") != nil) == valid) {
				err = fmt.Errorf("AllJoyn concurrent record %d lost fields or rollback isolation", index)
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

func TestProtocolCorpusAllJoynHeldReaderOuterTransactions(t *testing.T) {
	valid := alljoynTestFixtures(t)[2]
	for _, legacy := range []bool{false, true} {
		for pending := 0; pending < 8; pending++ {
			for _, wire := range [][]byte{valid, append(bytes.Clone(valid), 0), {1}, {1, 1, 0, 0, 0x84, 1, 3, 'a'}} {
				for _, rollback := range []bool{false, true} {
					// Shift the datagram behind an independently consumed bit prefix,
					// keeping a suffix outside its explicit parse boundary. Recovery
					// must neither duplicate prefix bytes nor consume that suffix.
					totalBits := pending + len(wire)*8 + 8
					input := make([]byte, (totalBits+7)/8)
					set := func(index int, bit byte) { input[index/8] |= bit << (7 - index%8) }
					for i := 0; i < pending; i++ {
						set(i, 1)
					}
					for i, value := range append(bytes.Clone(wire), 0x5a) {
						for bit := 0; bit < 8; bit++ {
							set(pending+i*8+bit, (value>>uint(7-bit))&1)
						}
					}
					reader := base.NewBitReader(bytes.NewReader(input))
					if pending > 0 {
						_, err := reader.ReadBits(uint64(pending))
						require.NoError(t, err)
					}
					require.NoError(t, reader.Backup())
					root, err := base.ParseRule("application-layer/alljoyn.yaml")
					require.NoError(t, err)
					root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
					root.Ctx.SetItem("alljoynNSLegacyLengths", legacy)
					root.Ctx.SetItem(base.CtxInputConfig, map[string]any{"alljoynNSLegacyLengths": legacy})
					require.NoError(t, root.ParseSubNode(reader, "AllJoynNSCarrier"))
					node := base.GetNodeByPath(root, "@AllJoynNSCarrier")
					require.Equal(t, wire, NodeToBytes(node), "pending=%d rollback=%t", pending, rollback)
					if bytes.Equal(wire, valid) {
						require.NotNil(t, protocolCorpusFindNode(node, "Sender Version"))
					} else {
						protocolCorpusRequireValue(t, node, "Unparsed AllJoyn NS Payload", wire)
						require.Nil(t, protocolCorpusFindNode(node, "Sender Version"))
					}
					if rollback {
						require.NoError(t, reader.Recovery())
						replayed, err := reader.ReadBits(uint64(len(wire)) * 8)
						require.NoError(t, err)
						require.Equal(t, wire, replayed)
					} else {
						require.NoError(t, reader.PopBackup())
					}
					suffix, err := reader.ReadBits(8)
					require.NoError(t, err)
					require.Equal(t, []byte{0x5a}, suffix)
					if padding := len(input)*8 - totalBits; padding > 0 {
						_, err = reader.ReadBits(uint64(padding))
						require.NoError(t, err)
					}
					require.ErrorContains(t, reader.Recovery(), "no backup")
					require.ErrorContains(t, reader.PopBackup(), "no backup")
					_, err = reader.ReadBits(1)
					require.ErrorIs(t, err, io.EOF)
				}
			}
		}
	}
}

func alljoynTestCompareTrees(t *testing.T, fast, legacy *base.Node) {
	t.Helper()
	require.Equal(t, legacy.Name, fast.Name)
	require.Equal(t, legacy.Origin, fast.Origin, fast.Name)
	// Runtime closure identity is intentionally excluded. Compare all rule,
	// wire-state and metadata keys relevant to these nodes, including skipped
	// alternative fields and the original list templates.
	for _, key := range []string{
		base.CfgType, base.CfgOperator, "out", "input", base.CfgLength, base.CfgIsList, base.CfgImport,
		stream_parser.CfgRefType, base.CfgDelimiter, stream_parser.CfgDelimiterOptional, base.CfgDel,
		stream_parser.CfgLengthFromField, stream_parser.CfgLengthForStartField, stream_parser.CfgLengthForField,
		stream_parser.CfgStopValue, stream_parser.CfgExceptionPlan, base.CfgNodeResult, stream_parser.CfgConsumedBits,
		stream_parser.CfgElementIndex, stream_parser.CfgLengthCacheMap, "additionInfo", base.CfgInList,
		base.CfgIsTerminal, base.CfgEndian, "parser", stream_parser.CfgUnit, base.CfgLastNode, stream_parser.CfgIsTempRoot, "package-child",
	} {
		require.Equal(t, legacy.Cfg.Has(key), fast.Cfg.Has(key), "%s config %s presence", fast.Name, key)
		require.Equal(t, legacy.Cfg.GetItem(key), fast.Cfg.GetItem(key), "%s config %s", fast.Name, key)
	}
	require.Equal(t, legacy.Cfg.Has("template"), fast.Cfg.Has("template"))
	if fast.Cfg.Has("template") {
		ft, lt := fast.Cfg.GetItem("template").(*base.Node), legacy.Cfg.GetItem("template").(*base.Node)
		require.Same(t, fast, ft.Cfg.GetItem(base.CfgParent))
		require.Same(t, legacy, lt.Cfg.GetItem(base.CfgParent))
		alljoynTestCompareTrees(t, ft, lt)
	}
	require.Len(t, fast.Children, len(legacy.Children), fast.Name)
	for i, fc := range fast.Children {
		lc := legacy.Children[i]
		require.Same(t, fast, fc.Cfg.GetItem(base.CfgParent), fc.Name)
		require.Same(t, legacy, lc.Cfg.GetItem(base.CfgParent), lc.Name)
		require.Equal(t, fast.Ctx == fc.Ctx, legacy.Ctx == lc.Ctx, fc.Name)
		alljoynTestCompareTrees(t, fc, lc)
	}
	if protocolCorpusNodeHasResult(fast) {
		fv, fe := fast.Result()
		lv, le := legacy.Result()
		require.NoError(t, fe)
		require.NoError(t, le)
		require.Same(t, fast, fv.Origin)
		require.Same(t, legacy, lv.Origin)
		require.Equal(t, lv.IsList(), fv.IsList())
		require.Equal(t, lv.IsStruct(), fv.IsStruct())
		if fv.IsValue() {
			require.Equal(t, lv.Value, fv.Value, fast.Name)
		}
	}
}

func alljoynTestHighCountWire() []byte {
	wire := []byte{1, 255, 235, 0}
	for i := 0; i < 255; i++ {
		wire = append(wire, 0x84, 0)
	}
	for i := 0; i < 235; i++ {
		wire = append(wire, 0x40, 0, 0, 4)
	}
	return wire
}

func TestProtocolCorpusAllJoynLengthTrackingDifferential(t *testing.T) {
	wires := alljoynTestFixtures(t)
	wires = append(wires, alljoynTestHighCountWire(), alljoynTestHex(t, "010202008402016101618402016101614000000440000004"), alljoynTestHex(t, "010001006002000400000300ff61"))
	// A single record with 255 names exercises the third length-tracking
	// loop independently of the Questions and Answers list counts.
	wires = append(wires, append([]byte{1, 1, 0, 0, 0x84, 255}, make([]byte, 255)...))
	for _, original := range alljoynTestFixtures(t) {
		for cut := 1; cut < len(original); cut++ {
			wires = append(wires, bytes.Clone(original[:cut]))
		}
		wires = append(wires, append(bytes.Clone(original), 0))
	}
	for index, wire := range wires {
		for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier"} {
			parse := func(legacy bool) (*base.Node, error) {
				return parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), alljoynCorpusRule, map[string]any{"alljoynNSLegacyLengths": legacy}, entry)
			}
			fast, fe := parse(false)
			legacy, le := parse(true)
			if le != nil {
				require.Error(t, fe, "wire %d %s", index, entry)
				require.Equal(t, protocolCorpusFailureDiagnostic(le), protocolCorpusFailureDiagnostic(fe), "wire %d %s failure cause", index, entry)
				continue
			}
			require.NoError(t, fe, "wire %d %s", index, entry)
			require.Equal(t, wire, NodeToBytes(fast))
			require.Equal(t, wire, NodeToBytes(legacy))
			require.Equal(t, alljoynTestLeaves(t, legacy, 0), alljoynTestLeaves(t, fast, 0))
			require.Equal(t, NodeToMap(legacy), NodeToMap(fast))
			alljoynTestCompareTrees(t, fast, legacy)
		}
	}
	// Explicit caller settings must survive the UDP and imported carrier path.
	for _, wire := range alljoynTestFixtures(t) {
		udp := alljoynTestUDP(wire, false)
		fast := protocolCorpusRequireBoundedRuleParseWithConfig(t, udp, "user_datagram_protocol", "UDP", map[string]any{"alljoynNSLegacyLengths": false})
		legacy := protocolCorpusRequireBoundedRuleParseWithConfig(t, udp, "user_datagram_protocol", "UDP", map[string]any{"alljoynNSLegacyLengths": true})
		alljoynTestCompareTrees(t, fast, legacy)
		require.Equal(t, alljoynTestLeaves(t, legacy, 8), alljoynTestLeaves(t, fast, 8))
	}
}

func BenchmarkAllJoynNSLengthTracking(b *testing.B) {
	for _, fixture := range []struct {
		name string
		wire []byte
	}{
		{"ordinary-184B", alljoynTestFixtures(b)[2]},
		{"counts-1454B", alljoynTestHighCountWire()},
	} {
		for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier"} {
			for _, legacy := range []bool{false, true} {
				mode := "Remaining"
				if legacy {
					mode = "LegacyLengths"
				}
				b.Run(fixture.name+"/"+entry+"/"+mode, func(b *testing.B) {
					config := map[string]any{"alljoynNSLegacyLengths": legacy}
					parse := func() {
						node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(fixture.wire), alljoynCorpusRule, config, entry)
						if err != nil {
							b.Fatal(err)
						}
						if _, err = node.Result(); err != nil {
							b.Fatal(err)
						}
						if !bytes.Equal(fixture.wire, NodeToBytes(node)) {
							b.Fatal("AllJoyn length mode changed bytes")
						}
					}
					parse() // Full Parse + Result + bytes warmup, including scalar out.
					b.ReportAllocs()
					b.SetBytes(int64(len(fixture.wire)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						parse()
					}
				})
			}
		}
	}
}
