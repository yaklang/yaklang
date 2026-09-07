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

// Independently encode Linux v6.12 msg.h word positions. This does not use
// the YAML generator or a live TIPC socket. Application bytes stay opaque.
func tipcTestMessage(kind, user, header int, payload []byte) []byte {
	b := make([]byte, header+len(payload))
	word := func(index int, value uint32) { binary.BigEndian.PutUint32(b[index*4:], value) }
	w0 := uint32(2<<29 | user<<25 | (header/4)<<21 | len(b))
	if kind == 0 || kind == 2 || kind == 3 {
		w0 |= 1 << 18
	}
	if kind == 1 || kind >= 5 {
		w0 |= 1 << 19
	}
	word(0, w0)
	scope := uint32(2 << 19)
	if kind == 0 || kind == 3 {
		scope = 0
	}
	word(1, uint32(kind<<29)|scope|0x1357)
	word(2, 0x2468369c)
	word(3, 0x01002003)
	word(4, 0x10203040)
	word(5, 0x50607080)
	if kind == 1 || kind == 5 || kind == 6 {
		word(5, 0)
	}
	if header >= 32 {
		word(6, 0x01002004)
		word(7, 0x01002005)
		if kind == 1 || kind == 5 || kind == 6 {
			word(7, 0)
		}
	}
	known := 32
	if header == 24 {
		known = 24
	}
	if kind == 1 || kind == 2 || kind >= 5 {
		word(8, 0x01020304)
		word(9, 0x10203040)
		known = 40
		if kind == 1 {
			word(10, 0x1020304f)
			known = 44
		}
	}
	if kind >= 5 {
		if kind != 6 {
			word(9, 0)
		}
		word(10, 0x35a70000)
		if kind == 5 || kind == 6 {
			word(10, 0x35a70001)
		}
		known = 44
	}
	for i := known; i < header; i++ {
		b[i] = 0xa0 + byte(i-known)
	}
	copy(b[header:], payload)
	return b
}

func tipcTestEthernet(body []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x88, 0xca}, body...)
}

func tipcTestCompanions() [][]byte {
	var out [][]byte
	for _, c := range []struct{ kind, user, header int }{
		{0, 0, 24}, {3, 1, 32}, {2, 2, 40}, {1, 3, 44},
		{5, 1, 44}, {6, 2, 44}, {7, 3, 44}, {2, 1, 60},
	} {
		out = append(out, tipcTestMessage(c.kind, c.user, c.header, []byte{0xd1, byte(c.kind), 0x17, 0x80, 0xf9}))
	}
	return out
}

func tipcTestInfo(t *testing.T, n *base.Node) map[string]any {
	t.Helper()
	v := protocolCorpusFindNode(n, "Version")
	require.NotNil(t, v)
	root := v.Cfg.GetItem(base.CfgParent).(*base.Node)
	info, ok := root.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	return info
}

func tipcTestRequireFields(t *testing.T, node *base.Node, body []byte, offset int) {
	t.Helper()
	// The independent oracle extracts individual bits directly from the wire,
	// rather than reproducing the rule's Process order or endian conversions.
	bits := func(start, count int) uint64 {
		var n uint64
		for i := start; i < start+count; i++ {
			n = n<<1 | uint64(body[i/8]>>uint(7-i%8)&1)
		}
		return n
	}
	field := func(name string, start, count int) {
		protocolCorpusRequireValue(t, node, name, bits(start, count))
		n := protocolCorpusFindNode(node, name)
		require.Equal(t, [2]uint64{uint64(offset*8 + start), uint64(offset*8 + start + count)}, stream_parser.GetNodeResultPos(n), name)
	}
	for _, c := range []struct {
		name         string
		start, count int
	}{
		{"Version", 0, 3}, {"User", 3, 4}, {"Header Size Words", 7, 4},
		{"Non Sequential", 11, 1}, {"Destination Droppable", 12, 1}, {"SYN", 14, 1}, {"Message Size", 15, 17},
		{"Message Type", 32, 3}, {"Error Code", 35, 4}, {"Reroute Count", 39, 4}, {"Lookup Scope", 43, 2},
		{"Word One Reserved", 45, 3}, {"Broadcast ACK", 48, 16}, {"Link ACK", 64, 16}, {"Link Sequence", 80, 16},
		{"Previous Node", 96, 32}, {"Origin Port", 128, 32}, {"Destination Port", 160, 32},
	} {
		field(c.name, c.start, c.count)
	}
	kind, header := int(bits(32, 3)), int(bits(7, 4))*4
	control := "Control Flag"
	switch kind {
	case 0:
		control = "ACK Required"
	case 1, 5, 6:
		control = "Replicast"
	case 2, 3:
		control = "Source Droppable"
	}
	field(control, 13, 1)
	for _, name := range []string{"Control Flag", "ACK Required", "Replicast", "Source Droppable"} {
		if name != control {
			require.Nil(t, protocolCorpusFindNode(node, name), "overloaded bit must have only its selected interpretation")
		}
	}
	known := 24
	if header != 24 {
		field("Origin Node", 192, 32)
		field("Destination Node", 224, 32)
		known = 32
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Origin Node"))
		require.Nil(t, protocolCorpusFindNode(node, "Destination Node"))
	}
	if kind == 1 || kind == 2 || kind >= 5 {
		field("Name Type", 256, 32)
		known = 40
		if kind == 1 {
			field("Name Lower", 288, 32)
			field("Name Upper", 320, 32)
			known = 44
		} else if kind == 2 || kind == 6 {
			field("Name Instance", 288, 32)
		} else {
			field("Group Word Nine", 288, 32)
		}
	}
	if kind >= 5 {
		field("Group Broadcast Sequence", 320, 16)
		if kind == 5 || kind == 6 {
			field("Group Reserved", 336, 15)
			field("Group Broadcast ACK Required", 351, 1)
		} else {
			field("Group Unicast Reserved", 336, 16)
		}
		known = 44
	}
	for _, raw := range []struct {
		name       string
		start, end int
	}{
		{"Header Extensions", known, header}, {"Application Data", header, len(body)},
	} {
		if raw.start < raw.end {
			protocolCorpusRequireValue(t, node, raw.name, body[raw.start:raw.end])
			require.Equal(t, [2]uint64{uint64(offset+raw.start) * 8, uint64(offset+raw.end) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, raw.name)))
		} else {
			require.Nil(t, protocolCorpusFindNode(node, raw.name))
		}
	}
	info := tipcTestInfo(t, node)
	require.Equal(t, header, info["Header Bytes"])
	require.Equal(t, header == 24, info["Destination Context Required"])
	wantOrigin := bits(96, 32)
	if header != 24 {
		wantOrigin = bits(192, 32)
	}
	require.EqualValues(t, wantOrigin, info["Origin Node"])
	require.Equal(t, false, info["Application Decoded"])
	require.Equal(t, false, info["Connection State Validated"])
}

func TestProtocolCorpusTIPCUserMessageFields(t *testing.T) {
	for i, body := range tipcTestCompanions() {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			node := protocolCorpusRequireBoundedRuleParse(t, body, "tipc", "TIPC")
			tipcTestRequireFields(t, node, body, 0)
		})
	}
	// Both source-defined short/basic header shapes, unknown optional words,
	// every importance and every supported data-message type.
	for _, kind := range []int{0, 1, 2, 3, 5, 6, 7} {
		for user := 0; user < 4; user++ {
			minimum := 44
			if kind == 0 || kind == 3 {
				minimum = 32
			}
			if kind == 2 {
				minimum = 40
			}
			for h := minimum; h <= 60; h += 4 {
				body := tipcTestMessage(kind, user, h, nil)
				node := protocolCorpusRequireBoundedRuleParse(t, body, "tipc", "TIPC")
				tipcTestRequireFields(t, node, body, 0)
			}
		}
	}
	for _, kind := range []int{0, 3} {
		body := tipcTestMessage(kind, 3, 24, nil)
		tipcTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, body, "tipc", "TIPC"), body, 0)
	}
}

func TestProtocolCorpusTIPCOriginalAndInvalidBoundaries(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-tipc.pcap"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "997b848b0a44cc4892ae910d3c073f8aa383153eb6fc2d6ad550c93fa6c829cc", fmt.Sprintf("%x", sha256.Sum256(file)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	original := append([]byte{0x80, 1}, make([]byte, 22)...)
	require.Equal(t, tipcTestEthernet(original), frames[0])
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(original), "tipc", "TIPC")
	require.ErrorContains(t, err, "tipc: unsupported version")
	invalid := [][]byte{original}
	for _, body := range tipcTestCompanions() {
		for end := 0; end < len(body); end++ {
			invalid = append(invalid, bytes.Clone(body[:end]))
		}
		for _, delta := range []int{-1, 1} {
			wire := bytes.Clone(body)
			v := binary.BigEndian.Uint32(wire[:4])
			binary.BigEndian.PutUint32(wire[:4], v&^0x1ffff|uint32(len(body)+delta))
			invalid = append(invalid, wire)
		}
		invalid = append(invalid, append(bytes.Clone(body), 0xa5))
	}
	baseBody := tipcTestMessage(2, 1, 40, nil)
	for version := 0; version < 8; version++ {
		if version == 2 {
			continue
		}
		b := bytes.Clone(baseBody)
		b[0] = b[0]&31 | byte(version<<5)
		invalid = append(invalid, b)
	}
	for user := 4; user < 16; user++ {
		b := bytes.Clone(baseBody)
		b[0] = b[0]&0xe1 | byte(user<<1)
		invalid = append(invalid, b)
	}
	for _, kind := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		for _, header := range []int{0, 4, 20, 24, 28, 32, 36, 40} {
			if kind != 4 && ((kind == 0 || kind == 3) && (header == 24 || header >= 32) || kind == 2 && header >= 40) {
				continue
			}
			b := tipcTestMessage(2, 1, 60, nil)
			w := binary.BigEndian.Uint32(b[:4])
			binary.BigEndian.PutUint32(b[:4], w&^(15<<21)|uint32(header/4)<<21)
			b[4] = b[4]&31 | byte(kind<<5)
			invalid = append(invalid, b)
		}
	}
	for i, body := range invalid {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body), "tipc", "TIPC")
		require.Error(t, err, "invalid %d %x", i, body)
	}
	for _, entry := range []string{"TIPC", "TIPCCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(baseBody), "tipc", entry)
		require.ErrorContains(t, err, "explicit")
	}
}

func TestProtocolCorpusTIPCReservedAndFlagBits(t *testing.T) {
	for _, kind := range []int{0, 1, 2, 3, 5, 6, 7} {
		body := tipcTestMessage(kind, 2, 60, []byte{0x71})
		for _, bit := range []int{11, 12, 13, 14, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47} {
			wire := bytes.Clone(body)
			wire[bit/8] ^= 1 << uint(7-bit%8)
			tipcTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "tipc", "TIPC"), wire, 0)
		}
		if kind >= 5 {
			for bit := 288; bit < 352; bit++ {
				wire := bytes.Clone(body)
				wire[bit/8] ^= 1 << uint(7-bit%8)
				tipcTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "tipc", "TIPC"), wire, 0)
			}
		}
	}
}

func TestProtocolCorpusTIPCResourceLimits(t *testing.T) {
	for _, h := range []int{24, 32, 40, 44, 60} {
		for _, size := range []int{0, 1, 65999, 66000, 66001} {
			kind := 0
			if h == 40 {
				kind = 2
			}
			if h >= 44 {
				kind = 6
			}
			body := tipcTestMessage(kind, 3, h, bytes.Repeat([]byte{0xa7}, size))
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body), "tipc", "TIPC")
			if size > 66000 {
				require.ErrorContains(t, err, "tipc:")
				continue
			}
			require.NoError(t, err)
			tipcTestRequireFields(t, node, body, 0)
			require.Equal(t, body, NodeToBytes(node))
		}
	}
}

func TestProtocolCorpusTIPCCarrierTransactions(t *testing.T) {
	good := tipcTestCompanions()[2]
	badVersion := bytes.Clone(good)
	badVersion[0] |= 0x80
	badType := bytes.Clone(good)
	badType[4] = 4 << 5
	bundled := bytes.Clone(good)
	bundled[0] = bundled[0]&0xe1 | 6<<1
	fragment := bytes.Clone(good)
	fragment[0] = fragment[0]&0xe1 | 12<<1
	for i, c := range []struct {
		body    []byte
		parsed  bool
		trailer []byte
	}{
		{good, true, nil}, {append(bytes.Clone(good), 0xa1, 0xb2), true, []byte{0xa1, 0xb2}},
		{good[:len(good)-1], false, nil}, {[]byte{0x45}, false, nil}, {badVersion, false, nil},
		{badType, false, nil}, {bundled, false, nil}, {fragment, false, nil},
	} {
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/imported-%t", i, imported), func(t *testing.T) {
				wire, rule, entry, offset := c.body, "tipc.yaml", "TIPCCarrier", 0
				if imported {
					wire, rule, entry, offset = tipcTestEthernet(c.body), "ethernet.yaml", "Ethernet", 14
				}
				root, err := base.ParseRule(rule)
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire)*8))
				reader := base.NewBitReader(bytes.NewReader(wire))
				require.NoError(t, root.ParseSubNode(reader, entry))
				node := base.GetNodeByPath(root, "@"+entry)
				require.NotNil(t, node)
				require.Equal(t, wire, NodeToBytes(node))
				if c.parsed {
					tipcTestRequireFields(t, node, good, offset)
					require.Nil(t, protocolCorpusFindNode(node, "Unparsed TIPC Payload"))
					if len(c.trailer) > 0 {
						protocolCorpusRequireValue(t, node, "TIPC Link Trailer", c.trailer)
						require.Equal(t, [2]uint64{uint64(offset+len(good)) * 8, uint64(len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "TIPC Link Trailer")))
					}
				} else {
					protocolCorpusRequireValue(t, node, "Unparsed TIPC Payload", c.body)
					require.Nil(t, protocolCorpusFindNode(node, "Version"))
					require.Equal(t, [2]uint64{uint64(offset) * 8, uint64(len(wire) * 8)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Unparsed TIPC Payload")))
				}
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

func TestProtocolCorpusTIPCParallelIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			body := tipcTestMessage(6, i%4, 44, []byte{byte(i), 0xa5})
			for repeat := 0; repeat < 3; repeat++ {
				tipcTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, body, "tipc", "TIPC"), body, 0)
			}
		})
	}
}

// These source-derived literal encodings are separately pinned in the recipe;
// checks against both the word builder and all captured records detect drift.
func TestProtocolCorpusTIPCCompanionEveryRecord(t *testing.T) {
	literals := []string{
		"40c4001d000013572468369c010020031020304050607080d1001780f9",
		"43040025600013572468369c0100200310203040506070800100200401002005d1031780f9",
		"4544002d401013572468369c01002003102030405060708001002004010020050102030410203040d1021780f9",
		"47680031201013572468369c010020031020304000000000010020040000000001020304102030401020304fd1011780f9",
		"43680031a01013572468369c0100200310203040000000000100200400000000010203040000000035a70001d1051780f9",
		"45680031c01013572468369c0100200310203040000000000100200400000000010203041020304035a70001d1061780f9",
		"47680031e01013572468369c0100200310203040506070800100200401002005010203040000000035a70000d1071780f9",
		"43e40041401013572468369c01002003102030405060708001002004010020050102030410203040a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3d1021780f9",
	}
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-tipc-valid.pcap")
	require.Len(t, frames, 8)
	for i, body := range tipcTestCompanions() {
		literal, err := hex.DecodeString(literals[i])
		require.NoError(t, err)
		require.Equal(t, literal, body)
		require.Equal(t, tipcTestEthernet(body), frames[i])
		for _, imported := range []bool{false, true} {
			wire, rule, entry, offset := body, "tipc", "TIPC", 0
			if imported {
				wire, rule, entry, offset = frames[i], "ethernet", "Ethernet", 14
			}
			node := protocolCorpusRequireBoundedRuleParse(t, wire, rule, entry)
			tipcTestRequireFields(t, node, body, offset)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed TIPC Payload"))
		}
	}
}

func TestProtocolCorpusTIPCPublicTruncationAndIsolation(t *testing.T) {
	for _, body := range tipcTestCompanions() {
		for end := 1; end < len(body); end++ {
			wire := tipcTestEthernet(body[:end])
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, node, "Unparsed TIPC Payload", body[:end])
			require.Nil(t, protocolCorpusFindNode(node, "Version"))
			require.Equal(t, [2]uint64{112, uint64(len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Unparsed TIPC Payload")))
		}
	}
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-tipc.pcap")[0]
	node := protocolCorpusRequireBoundedRuleParse(t, original, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, node, "Unparsed TIPC Payload", original[14:])
	require.Nil(t, protocolCorpusFindNode(node, "Version"))
	wire := tipcTestEthernet(tipcTestCompanions()[0])
	wire[13] = 0xcb
	node = protocolCorpusRequireBoundedRuleParse(t, wire, "ethernet", "Ethernet")
	require.Nil(t, protocolCorpusFindNode(node, "Message Type"))
	require.Nil(t, protocolCorpusFindNode(node, "Header Size Words"))
}
