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

func nattTestHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Independent RFC header construction; this does not invoke the parser or its
// rule-generation path. SPI values and Message ID deliberately are nonzero.
func nattTestIKE(next, version, flags byte, payload []byte) []byte {
	b := nattTestHex("000000001122334455667788887766554433221100202508010203040000001c")
	b[20], b[21], b[23] = next, version, flags
	binary.BigEndian.PutUint32(b[28:32], uint32(28+len(payload)))
	return append(b, payload...)
}

func nattTestFixtures() [][]byte {
	return [][]byte{
		{0xff},
		nattTestHex("1122334401020304a0a1a2a3a4a5a6a7a8a9aaabacadaeaf"),
		nattTestIKE(41, 0x20, 8, nattTestHex("280000080000402e00000014000102030405060708090a0b0c0d0e0f")),
		nattTestIKE(13, 0x10, 0, nattTestHex("0000000c1122334455667788")),
		nattTestIKE(8, 0x10, 1, bytes.Repeat([]byte{0xa5}, 16)),
		nattTestIKE(46, 0x20, 8, nattTestHex("23800018000102030405060708090a0b0c0d0e0f10111213")),
		nattTestIKE(53, 0x20, 8, nattTestHex("2300001c00010002000102030405060708090a0b0c0d0e0f10111213")),
		nattTestIKE(53, 0x20, 8, nattTestHex("0000001c00020002000102030405060708090a0b0c0d0e0f10111213")),
		nattTestIKE(41, 0x20, 8, nattTestHex("000000100304400911223344aabbccdd")),
		nattTestIKE(34, 0x20, 8, nattTestHex("0000000c0013000001020304")),
	}
}

func nattTestUDP(payload []byte, src, dst uint16) []byte {
	wire := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(wire, src)
	binary.BigEndian.PutUint16(wire[2:], dst)
	binary.BigEndian.PutUint16(wire[4:], uint16(8+len(payload)))
	return append(wire, payload...)
}

func nattTestField(t *testing.T, root *base.Node, name string, value any, start, end uint64) {
	t.Helper()
	protocolCorpusRequireValue(t, root, name, value)
	n := protocolCorpusFindNode(root, name)
	require.NotNil(t, n)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(n), name)
}

func nattTestCheck(t *testing.T, root *base.Node, payload []byte, offset uint64) {
	t.Helper()
	if len(payload) == 1 {
		nattTestField(t, root, "Keepalive", uint64(255), offset*8, (offset+1)*8)
		return
	}
	nattTestField(t, root, "Marker or SPI", uint64(binary.BigEndian.Uint32(payload)), offset*8, (offset+4)*8)
	if binary.BigEndian.Uint32(payload) != 0 {
		nattTestField(t, root, "ESP Sequence", uint64(binary.BigEndian.Uint32(payload[4:])), (offset+4)*8, (offset+8)*8)
		nattTestField(t, root, "ESP Opaque Body", payload[8:], (offset+8)*8, (offset+uint64(len(payload)))*8)
		return
	}
	spi := protocolCorpusFindNode(root, "Initiator SPI")
	require.NotNil(t, spi)
	root = spi.Cfg.GetItem(base.CfgParent).(*base.Node)
	nattTestField(t, root, "Initiator SPI", payload[4:12], (offset+4)*8, (offset+12)*8)
	nattTestField(t, root, "Responder SPI", payload[12:20], (offset+12)*8, (offset+20)*8)
	for _, f := range []struct {
		name string
		pos  int
	}{
		{"Next Payload", 20}, {"Version", 21}, {"Exchange Type", 22}, {"Flags", 23},
	} {
		nattTestField(t, root, f.name, uint64(payload[f.pos]), (offset+uint64(f.pos))*8, (offset+uint64(f.pos)+1)*8)
	}
	nattTestField(t, root, "Message ID", uint64(binary.BigEndian.Uint32(payload[24:])), (offset+24)*8, (offset+28)*8)
	nattTestField(t, root, "IKE Length", uint64(len(payload)-4), (offset+28)*8, (offset+32)*8)
	if payload[21]>>4 == 1 && payload[23]&1 != 0 {
		nattTestField(t, root, "Encrypted IKEv1 Body", payload[32:], (offset+32)*8, (offset+uint64(len(payload)))*8)
		require.Nil(t, protocolCorpusFindNode(root, "Payload Length"))
		return
	}
	list := protocolCorpusFindNode(root, "Payloads")
	if len(payload) == 32 {
		require.Nil(t, list)
		return
	}
	require.NotNil(t, list)
	value, err := list.Result()
	require.NoError(t, err)
	require.True(t, value.IsList())
	items, ok := NodeToMap(list).([]any)
	require.True(t, ok)
	fields := protocolCorpusNodesNamed(root, "Payload Length")
	position := 32
	for i, field := range fields {
		length := int(binary.BigEndian.Uint16(payload[position+2:]))
		value, err := field.Result()
		require.NoError(t, err)
		require.Equal(t, uint16(length), value.Value)
		require.Equal(t, [2]uint64{(offset + uint64(position) + 2) * 8, (offset + uint64(position) + 4) * 8}, stream_parser.GetNodeResultPos(field))
		require.Less(t, i, len(items))
		position += length
	}
	if padding := protocolCorpusFindNode(root, "ISAKMP Padding"); padding != nil {
		nattTestField(t, root, "ISAKMP Padding", payload[position:], (offset+uint64(position))*8, (offset+uint64(len(payload)))*8)
		position = len(payload)
		require.Len(t, items, len(fields)+1)
	} else {
		require.Len(t, items, len(fields))
	}
	require.Equal(t, len(payload), position)
}

func TestProtocolCorpusNATTNonceFragmentsPaddingAndUnknownLayouts(t *testing.T) {
	for _, size := range []int{0, 15, 16, 256, 257} {
		body := append([]byte{0, 0, byte((4 + size) >> 8), byte(4 + size)}, bytes.Repeat([]byte{0xa5}, size)...)
		payload := nattTestIKE(40, 0x20, 8, body)
		node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), "nat_t", "NATT")
		if size < 16 || size > 256 {
			require.ErrorContains(t, err, "nonce data must contain 16..256")
			continue
		}
		require.NoError(t, err)
		nattTestField(t, node, "Nonce Data", body[4:], 36*8, uint64(len(payload))*8)
	}
	for _, major := range []byte{0x10, 0x20} {
		for size := 1; size <= 3; size++ {
			body := append([]byte{0, 0, 0, byte(4 + size)}, bytes.Repeat([]byte{0xa5}, size)...)
			body = append(body, bytes.Repeat([]byte{0x7e}, 4-size)...)
			payload := nattTestIKE(99, major, 0, body)
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), "nat_t", "NATT")
			if major == 0x20 {
				require.ErrorContains(t, err, "bytes remain after the final")
				continue
			}
			require.NoError(t, err)
			nattTestCheck(t, node, payload, 0)
		}
	}
	for _, fragment := range []byte{1, 2} {
		body := nattTestHex("35000008112233440000000c0001000201020304")
		body[13] = fragment
		payload := nattTestIKE(43, 0x20, 8, body)
		node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), "nat_t", "NATT")
		if fragment == 2 {
			require.ErrorContains(t, err, "only first IKE fragment can have preceding payloads")
			continue
		}
		require.NoError(t, err)
		nattTestCheck(t, node, payload, 0)
	}
	// Unknown critical fields remain observable, not handled application state.
	payload := nattTestIKE(200, 0x2f, 0xff, nattTestHex("00ff0005a5"))
	node := protocolCorpusRequireBoundedRuleParse(t, payload, "nat_t", "NATT")
	field := protocolCorpusFindNode(node, "Payload Length").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := field.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, true, info["Critical"])
	require.Equal(t, "opaque", info["Body Layout"])
	require.Equal(t, false, info["Body Semantics Decoded"])
	nattTestField(t, node, "Payload Data", []byte{0xa5}, 36*8, 37*8)
	// IKEv1 notification has a DOI before protocol/SPI/type, unlike v2.
	payload = nattTestIKE(11, 0x10, 0, nattTestHex("00000011000000010304000b11223344a5"))
	node = protocolCorpusRequireBoundedRuleParse(t, payload, "nat_t", "NATT")
	nattTestField(t, node, "Domain of Interpretation", uint64(1), 36*8, 40*8)
	nattTestField(t, node, "Notification SPI", nattTestHex("11223344"), 44*8, 48*8)
	nattTestField(t, node, "Notification Data", []byte{0xa5}, 48*8, 49*8)
}

func TestProtocolCorpusNATTLiteralFieldsAndPublicPaths(t *testing.T) {
	for index, payload := range nattTestFixtures() {
		for _, path := range []struct {
			rule, entry string
			wire        []byte
			offset      uint64
		}{
			{"nat_t", "NATT", payload, 0}, {"nat_t", "NATTCarrier", payload, 0},
			{"user_datagram_protocol", "UDP", nattTestUDP(payload, 40101, 4500), 8},
			{"user_datagram_protocol", "UDP", nattTestUDP(payload, 4500, 40101), 8},
		} {
			t.Run(fmt.Sprintf("%d/%s", index, path.entry), func(t *testing.T) {
				root := protocolCorpusRequireBoundedRuleParse(t, path.wire, path.rule, path.entry)
				require.Equal(t, path.wire, NodeToBytes(root))
				nattTestCheck(t, root, payload, path.offset)
				require.Nil(t, protocolCorpusFindNode(root, "Unparsed NAT-T Payload"))
			})
		}
	}
	fixtures := nattTestFixtures()
	for _, c := range []struct {
		index    int
		name     string
		value    any
		from, to uint64
	}{
		{2, "Notify Type", uint64(16430), 38, 40}, {2, "Nonce Data", fixtures[2][44:], 44, 60},
		{3, "Vendor ID", fixtures[3][36:], 36, 44}, {5, "Encrypted Payload Data", fixtures[5][36:], 36, 56},
		{6, "Fragment Number", uint64(1), 36, 38}, {6, "Total Fragments", uint64(2), 38, 40},
		{7, "Fragment Number", uint64(2), 36, 38}, {8, "Notification SPI", nattTestHex("11223344"), 40, 44},
		{8, "Notification Data", nattTestHex("aabbccdd"), 44, 48}, {9, "DH Group", uint64(19), 36, 38},
		{9, "Key Exchange Data", nattTestHex("01020304"), 40, 44},
	} {
		root := protocolCorpusRequireBoundedRuleParse(t, fixtures[c.index], "nat_t", "NATT")
		nattTestField(t, root, c.name, c.value, c.from*8, c.to*8)
	}
}

func TestProtocolCorpusNATTOriginalAndCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-nat-t.pcap")
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 78)
		payload := frame[42:]
		require.Equal(t, nattTestHex("000000000000000000000000000000000000000010100000000000000000000000000000"), payload)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(payload), "nat_t", "NATT")
		require.ErrorContains(t, err, "nat-t: IKE length must match")
		root := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(root))
		nattTestField(t, root, "Unparsed NAT-T Payload", payload, 42*8, uint64(len(frame))*8)
		require.Nil(t, protocolCorpusFindNode(root, "Initiator SPI"))
	}
	frames = protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-nat-t-valid.pcap")
	fixtures := nattTestFixtures()
	require.Len(t, frames, len(fixtures))
	for i, frame := range frames {
		require.Equal(t, fixtures[i], frame[42:])
		root := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(root))
		nattTestCheck(t, root, fixtures[i], 42)
	}
}

func nattTestNegativeFixtures() [][]byte {
	return [][]byte{{0}, {1, 2}, {0xff, 0xff, 0xff}, {1, 0, 0, 0, 0, 0, 0, 0},
		nattTestIKE(1, 0x30, 0, nil), nattTestIKE(1, 0x20, 0, nil), nattTestIKE(0, 0x20, 0, []byte{0}),
		nattTestIKE(99, 0x20, 0, nattTestHex("00000003")), nattTestIKE(99, 0x20, 0, nattTestHex("00000005")),
		nattTestIKE(99, 0x20, 0, nattTestHex("63000004")), nattTestIKE(99, 0x20, 0, nattTestHex("0000000400")),
		nattTestIKE(46, 0x20, 0, nattTestHex("00000004")), nattTestIKE(53, 0x20, 0, nattTestHex("230000090000000200")),
		nattTestIKE(53, 0x20, 0, nattTestHex("230000090002000200")), nattTestIKE(53, 0x20, 0, nattTestHex("000000090003000200")),
		nattTestIKE(41, 0x20, 0, nattTestHex("00000007000000")), nattTestIKE(41, 0x20, 0, nattTestHex("0000000800050001")),
		nattTestIKE(34, 0x20, 0, nattTestHex("0000000800010000")), nattTestIKE(8, 0x10, 1, nil),
		nattTestIKE(43, 0x20, 8, nattTestHex("29000008112233440000000800050001")),
		nattTestIKE(43, 0x20, 8, nattTestHex("35000008112233440000000c0002000201020304")),
	}
}

func TestProtocolCorpusNATTTruncationChainAndTransactions(t *testing.T) {
	fixtures := nattTestFixtures()
	for _, legacy := range []bool{true, false} {
		config := map[string]any{"nattPayloadsLegacy": legacy}
		for _, payload := range fixtures[2:] {
			for size := 0; size < len(payload); size++ {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(payload[:size]), "nat_t", config, "NATT")
				require.Error(t, err, "prefix %d/%d legacy %t", size, len(payload), legacy)
			}
		}
		for _, payload := range nattTestNegativeFixtures() {
			_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(payload), "nat_t", config, "NATT")
			require.Error(t, err)
			root, err := base.ParseRule("nat_t.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(payload))*8)
			root.Ctx.SetItem("nattPayloadsLegacy", legacy)
			root.Ctx.SetItem(base.CtxInputConfig, config)
			reader := bytes.NewReader(payload)
			br := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(br, "NATTCarrier"))
			carrier := base.GetNodeByPath(root, "@NATTCarrier")
			require.Equal(t, payload, NodeToBytes(carrier))
			protocolCorpusRequireValue(t, carrier, "Unparsed NAT-T Payload", payload)
			require.Nil(t, protocolCorpusFindNode(carrier, "Marker or SPI"))
			require.Nil(t, protocolCorpusFindNode(carrier, "Payload Length"))
			require.Zero(t, reader.Len())
			require.ErrorContains(t, br.Recovery(), "no backup")
			require.ErrorContains(t, br.PopBackup(), "no backup")
			_, err = br.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for _, entry := range []string{"NATT", "NATTCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(fixtures[2]), "nat_t", entry)
		require.ErrorContains(t, err, "explicit")
	}
}

func nattTestManyPayloads(count int) []byte {
	body := bytes.Repeat([]byte{99, 0, 0, 4}, count)
	if count > 0 {
		body[len(body)-4] = 0
	}
	return nattTestIKE(99, 0x20, 8, body)
}

func TestProtocolCorpusNATTResourceBounds(t *testing.T) {
	for _, count := range []int{1, 64, 4096} {
		payload := nattTestManyPayloads(count)
		root := protocolCorpusRequireBoundedRuleParse(t, payload, "nat_t", "NATT")
		require.Len(t, protocolCorpusNodesNamed(root, "Payload Length"), count)
		nattTestCheck(t, root, payload, 0)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nattTestManyPayloads(4097)), "nat_t", "NATT")
	require.ErrorContains(t, err, "payload count exceeds 4096")
	maximum := append(nattTestHex("1122334400000000"), bytes.Repeat([]byte{0xa5}, 65527-8)...)
	root := protocolCorpusRequireBoundedRuleParse(t, maximum, "nat_t", "NATT")
	protocolCorpusRequireValue(t, root, "ESP Sequence", uint64(0))
	protocolCorpusRequireValue(t, root, "ESP Opaque Body", maximum[8:])
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(append(maximum, 0)), "nat_t", "NATT")
	require.ErrorContains(t, err, "outside 1..65527")
}

func TestProtocolCorpusNATTConcurrentIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		i := i
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			payload := nattTestManyPayloads(i + 1)
			binary.BigEndian.PutUint32(payload[24:], uint32(i))
			root := protocolCorpusRequireBoundedRuleParse(t, payload, "nat_t", "NATT")
			protocolCorpusRequireValue(t, root, "Message ID", uint64(i))
			require.Len(t, protocolCorpusNodesNamed(root, "Payload Length"), i+1)
		})
	}
}

// Preserve the complete observable tree, including inactive template fields,
// scalar types, list shape, provenance, metadata and global bit positions.
// The explicit legacy switch is the original YAML path in the same build.
func nattTestTreeDetails(root *base.Node) []any {
	var result []any
	var walk func(*base.Node)
	walk = func(node *base.Node) {
		result = append(result, node.Name, node.Origin)
		for _, key := range []string{"additionInfo", stream_parser.CfgElementIndex, "endian", "parser", "out", "input", stream_parser.CfgStopValue, "delimiter", stream_parser.CfgDelimiterOptional, stream_parser.CfgExceptionPlan, stream_parser.CfgRefType} {
			result = append(result, key, node.Cfg.Has(key), node.Cfg.GetItem(key))
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return result
}

func TestProtocolCorpusNATTNativeLegacyDifferential(t *testing.T) {
	fixtures := nattTestFixtures()
	fixtures = append(fixtures,
		nattTestManyPayloads(1), nattTestManyPayloads(64),
		nattTestIKE(200, 0x2f, 0xff, nattTestHex("00ff0005a5")),
		nattTestIKE(11, 0x10, 0, nattTestHex("00000011000000010304000b11223344a5")),
		nattTestIKE(43, 0x20, 8, nattTestHex("35000008112233440000000c0001000201020304")),
		nattTestIKE(41, 0x20, 0, nattTestHex("000000080000402e")),
		nattTestIKE(13, 0x10, 0, nattTestHex("00000004")),
		nattTestIKE(10, 0x10, 0, nattTestHex("00000004")),
	)
	for size := 1; size <= 3; size++ {
		body := append([]byte{0, 0, 0, byte(4 + size)}, bytes.Repeat([]byte{0xa5}, size)...)
		body = append(body, bytes.Repeat([]byte{0x7e}, 4-size)...)
		fixtures = append(fixtures, nattTestIKE(99, 0x10, 0, body))
	}
	for index, payload := range fixtures {
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/import-%t", index, imported), func(t *testing.T) {
				wire, rule, entry := payload, "nat_t", "NATT"
				if imported {
					wire = append(nattTestHex("0200000000020200000000010800450000001234000040110000c0000201c0000202"), nattTestUDP(payload, 40101, 4500)...)
					binary.BigEndian.PutUint16(wire[16:18], uint16(len(wire)-14))
					rule, entry = "ethernet", "Ethernet"
				}
				legacy := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, map[string]any{"nattPayloadsLegacy": true})
				native := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, map[string]any{"nattPayloadsLegacy": false})
				oldTree, oldValue := dicomNativeSnapshot(t, legacy, wire)
				newTree, newValue := dicomNativeSnapshot(t, native, wire)
				require.Equal(t, oldTree, newTree)
				require.Equal(t, oldValue, newValue)
				require.Equal(t, nattTestTreeDetails(legacy), nattTestTreeDetails(native))
				require.Equal(t, NodeToMap(legacy), NodeToMap(native))
				if list := protocolCorpusFindNode(native, "Payloads"); list != nil {
					oldList := protocolCorpusFindNode(legacy, "Payloads")
					for _, key := range []string{"natt_ike_major", "natt_payload_type", "natt_payload_index"} {
						require.Equal(t, oldList.Ctx.GetItem(key), list.Ctx.GetItem(key), key)
					}
				}
			})
		}
	}
}

func BenchmarkNATTParseAndResult(b *testing.B) {
	for _, count := range []int{1, 64, 512} {
		for _, legacy := range []bool{true, false} {
			b.Run(fmt.Sprintf("%d/legacy-%t", count, legacy), func(b *testing.B) {
				payload := nattTestManyPayloads(count)
				config := map[string]any{"nattPayloadsLegacy": legacy}
				parse := func() {
					root, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(payload), "nat_t", config, "NATT")
					if err != nil {
						b.Fatal(err)
					}
					if _, err = root.Result(); err != nil {
						b.Fatal(err)
					}
					if !bytes.Equal(payload, NodeToBytes(root)) {
						b.Fatal("wire changed")
					}
				}
				parse()
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					parse()
				}
			})
		}
	}
}
