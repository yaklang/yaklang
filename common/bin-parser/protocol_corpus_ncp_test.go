package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const ncpCorpusRule = "application-layer.ncp"

type ncpTestFixture struct {
	name, literal, kind string
	opaque, reply       bool
	wire                []byte
}

// Independent request encodings from Novell's NCP documentation, not the
// YAML generator or a successful heuristic dissection. Byte 5 is ignored for
// connection controls; service packets can instead use the high connection
// byte. The internet-address target is LE, unlike its BE structure length.
// https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk333.html
// https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk339.html
// https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk365.html
// https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk437.html
// https://www.novell.com/documentation/developer/ncp/ncp__enu/data/sdk443.html
func ncpTestFixtures(t *testing.T) []ncpTestFixture {
	t.Helper()
	tests := []ncpTestFixture{
		{name: "create", literal: "111100ff000000", kind: "Create Service Connection"},
		{name: "create-ignored", literal: "111100ffa5b6c7", kind: "Create Service Connection"},
		{name: "destroy-ignored", literal: "55550201345678", kind: "Destroy Service Connection"},
		{name: "server-date-time", literal: "22220101010014", kind: "Get File Server Date And Time"},
		{name: "internet-address", literal: "222202fe03801700051a78563412", kind: "Get Internet Address"},
		{name: "buffer-size", literal: "222203010100210400", kind: "Negotiate Buffer Size"},
		{name: "unknown-function", literal: "2222040101007e010203", kind: "Service Request", opaque: true},
		{name: "reply-empty", literal: "3333000100000000", kind: "Request Processed", opaque: true, reply: true},
		{name: "reply-data", literal: "33330101010000007e09060c223803", kind: "Request Processed", opaque: true, reply: true},
		{name: "acknowledgement-ignored", literal: "9999a50112c3d4e5", kind: "Request Being Processed"},
	}
	for i := range tests {
		var err error
		tests[i].wire, err = hex.DecodeString(tests[i].literal)
		require.NoError(t, err)
	}
	return tests
}

func ncpTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	node := protocolCorpusRequireBoundedRuleParse(t, wire, ncpCorpusRule, entry)
	require.Equal(t, wire, NodeToBytes(node))
	require.Equal(t, uint64(len(wire))*8, stream_parser.CalcNodeConsumedLength(node))
	return node
}

func ncpTestInfo(t *testing.T, node *base.Node) map[string]any {
	t.Helper()
	field := protocolCorpusFindNode(node, "Packet Type")
	require.NotNil(t, field)
	message, ok := field.Cfg.GetItem(base.CfgParent).(*base.Node)
	require.True(t, ok)
	info, ok := message.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	return info
}

func ncpTestField(t *testing.T, node *base.Node, name string, want any, offset, start, end int) {
	t.Helper()
	protocolCorpusRequireValue(t, node, name, want)
	field := protocolCorpusFindNode(node, name)
	require.NotNil(t, field)
	require.Equal(t, [2]uint64{uint64(offset+start) * 8, uint64(offset+end) * 8}, stream_parser.GetNodeResultPos(field), name)
	require.Equal(t, uint64(end-start)*8, stream_parser.CalcNodeConsumedLength(field), name)
	value, err := field.Result()
	require.NoError(t, err)
	require.Same(t, field, value.Origin)
}

func ncpTestRequireFixture(t *testing.T, node *base.Node, f ncpTestFixture, offset int) {
	t.Helper()
	wire := f.wire
	kind := binary.BigEndian.Uint16(wire)
	ncpTestField(t, node, "Packet Type", uint64(kind), offset, 0, 2)
	for i, name := range []string{"Sequence Number", "Connection Number Low", "Task Number"} {
		ncpTestField(t, node, name, uint64(wire[i+2]), offset, i+2, i+3)
	}
	info := ncpTestInfo(t, node)
	require.Equal(t, f.kind, info["Message Kind"])
	require.Equal(t, f.opaque, info["Application Data Opaque"])
	require.Equal(t, !f.opaque, info["Operation Body Validated"])
	require.Equal(t, f.reply, info["Reply Context Required"])
	require.Equal(t, false, info["Conversation State Validated"])
	if kind == 0x1111 || kind == 0x5555 || kind == 0x9999 {
		ncpTestField(t, node, "Reserved", uint64(wire[5]), offset, 5, 6)
		require.EqualValues(t, wire[3], info["Connection Number"])
	} else {
		ncpTestField(t, node, "Connection Number High", uint64(wire[5]), offset, 5, 6)
		require.EqualValues(t, int(wire[3])+int(wire[5])*256, info["Connection Number"])
	}
	switch kind {
	case 0x1111, 0x5555:
		ncpTestField(t, node, "Request Code", uint64(wire[6]), offset, 6, 7)
	case 0x2222:
		ncpTestField(t, node, "Function Code", uint64(wire[6]), offset, 6, 7)
		switch wire[6] {
		case 23:
			ncpTestField(t, node, "Subfunction Structure Length", uint64(5), offset, 7, 9)
			ncpTestField(t, node, "Subfunction Code", uint64(26), offset, 9, 10)
			ncpTestField(t, node, "Target Connection", uint64(0x12345678), offset, 10, 14)
		case 33:
			ncpTestField(t, node, "Proposed Buffer Size", uint64(1024), offset, 7, 9)
		case 20:
			require.Nil(t, protocolCorpusFindNode(node, "Function Data"))
		default:
			ncpTestField(t, node, "Function Data", wire[7:], offset, 7, len(wire))
		}
	case 0x3333, 0x9999:
		ncpTestField(t, node, "Completion Code", uint64(wire[6]), offset, 6, 7)
		ncpTestField(t, node, "Connection Status", uint64(wire[7]), offset, 7, 8)
		require.Equal(t, kind == 0x3333, info["Completion Fields Applicable"])
		if len(wire) > 8 {
			ncpTestField(t, node, "Reply Data", wire[8:], offset, 8, len(wire))
		} else {
			require.Nil(t, protocolCorpusFindNode(node, "Reply Data"))
		}
	}
}

func TestProtocolCorpusNCPFieldsAndMetadata(t *testing.T) {
	for _, f := range ncpTestFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			for _, entry := range []string{"NCP", "NCPCarrier"} {
				node := ncpTestParse(t, f.wire, entry)
				ncpTestRequireFixture(t, node, f, 0)
				require.Nil(t, protocolCorpusFindNode(node, "Unparsed NCP Payload"))
			}
		})
	}
}

func TestProtocolCorpusNCPOriginalAndCompanionEveryRecord(t *testing.T) {
	const corpus = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	require.NoError(t, json.Unmarshal(readProtocolCorpusFile(t, corpus, "manifest.json"), &manifest))
	var sources protocolCorpusSourceSpec
	require.NoError(t, json.Unmarshal(readProtocolCorpusFile(t, corpus, "sources.json"), &sources))
	// Fail on an unreviewed capture instead of silently sampling only frame 1.
	want := map[string]string{
		"pr5023-gen-ncp": "18f57a680cebd36ef8b927c3b5fb3e54206a239ad10911c84741b7f0d25f68c2",
		"gen-ncp-valid":  "dd50062c82d12ef57a9c86e8a5628a6a7bf5d0d6ee74c07a3aa2589b30bf1206",
	}
	seenManifest, seenSources := map[string]bool{}, map[string]bool{}
	for _, source := range sources.Captures {
		if source.Protocol == "NCP" {
			require.Contains(t, want, source.ID)
			if source.SourceSHA256 != "" {
				require.Equal(t, want[source.ID], source.SourceSHA256)
			}
			seenSources[source.ID] = true
		}
	}
	fixtures := ncpTestFixtures(t)
	for _, capture := range manifest.Captures {
		if capture.Protocol != "NCP" {
			continue
		}
		t.Run(capture.ID, func(t *testing.T) {
			require.Contains(t, want, capture.ID)
			seenManifest[capture.ID] = true
			file := corpus + "/" + capture.CaptureFile
			data := readProtocolCorpusFile(t, ".", file)
			require.Equal(t, want[capture.ID], fmt.Sprintf("%x", sha256.Sum256(data)))
			require.Equal(t, want[capture.ID], capture.SHA256)
			frames := protocolCorpusAuditPackets(t, file)
			require.Len(t, frames, capture.PacketCount)
			if capture.ID == "pr5023-gen-ncp" {
				require.Len(t, frames, 1)
			} else {
				require.Len(t, frames, len(fixtures))
			}
			for i, frame := range frames {
				t.Run(fmt.Sprintf("frame-%d", i+1), func(t *testing.T) {
					packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
					require.Nil(t, packet.ErrorLayer())
					udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
					require.True(t, ok)
					require.True(t, udp.SrcPort == 524 || udp.DstPort == 524)
					require.Equal(t, len(udp.Payload)+8, int(udp.Length))
					require.Equal(t, len(udp.Payload)+42, len(frame))
					envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
					require.Equal(t, frame, NodeToBytes(envelope))
					if capture.ID == "pr5023-gen-ncp" {
						require.Equal(t, "11110000000000000000", hex.EncodeToString(udp.Payload))
						_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), ncpCorpusRule, "NCP")
						require.ErrorContains(t, err, "ncp: connection control must be exactly 7 bytes")
						carrier := ncpTestParse(t, udp.Payload, "NCPCarrier")
						ncpTestField(t, carrier, "Unparsed NCP Payload", udp.Payload, 0, 0, 10)
						require.Nil(t, protocolCorpusFindNode(carrier, "Packet Type"))
						ncpTestField(t, envelope, "Unparsed NCP Payload", udp.Payload, 0, 42, 52)
						require.Nil(t, protocolCorpusFindNode(envelope, "Packet Type"))
					} else {
						f := fixtures[i]
						require.Equal(t, f.wire, udp.Payload)
						ncpTestRequireFixture(t, ncpTestParse(t, udp.Payload, "NCP"), f, 0)
						ncpTestRequireFixture(t, envelope, f, 42)
						require.Nil(t, protocolCorpusFindNode(envelope, "Unparsed NCP Payload"))
					}
				})
			}
		})
	}
	require.Equal(t, map[string]bool{"pr5023-gen-ncp": true, "gen-ncp-valid": true}, seenManifest)
	require.Equal(t, seenManifest, seenSources)
}

func ncpTestUDP(payload []byte, source, destination uint16) []byte {
	wire := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(wire[0:2], source)
	binary.BigEndian.PutUint16(wire[2:4], destination)
	binary.BigEndian.PutUint16(wire[4:6], uint16(8+len(payload)))
	return append(wire, payload...)
}

func TestProtocolCorpusNCPUDPPortCarrierAndRollback(t *testing.T) {
	f := ncpTestFixtures(t)[4]
	for _, ports := range [][2]uint16{{40000, 524}, {524, 40000}, {524, 524}} {
		t.Run(fmt.Sprintf("%d-%d", ports[0], ports[1]), func(t *testing.T) {
			udp := ncpTestUDP(f.wire, ports[0], ports[1])
			node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
			require.Equal(t, udp, NodeToBytes(node))
			ncpTestRequireFixture(t, node, f, 8)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed NCP Payload"))
			require.Nil(t, protocolCorpusFindNode(node, "Remaining Payload"))
		})
	}
	for _, wire := range [][]byte{f.wire[:1], append(bytes.Clone(f.wire), 0x42), {0x77, 0x77, 0, 0, 0, 0, 0}} {
		udp := ncpTestUDP(wire, 40000, 524)
		node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
		require.Equal(t, udp, NodeToBytes(node))
		ncpTestField(t, node, "Unparsed NCP Payload", wire, 0, 8, len(udp))
		require.Nil(t, protocolCorpusFindNode(node, "Packet Type"))
		require.Nil(t, protocolCorpusFindNode(node, "Remaining Payload"))
	}
	for _, ports := range [][2]uint16{{40000, 40001}, {523, 525}} {
		node := protocolCorpusRequireBoundedRuleParse(t, ncpTestUDP(f.wire, ports[0], ports[1]), "user_datagram_protocol", "UDP")
		require.Nil(t, protocolCorpusFindNode(node, "NCPFrame"))
		require.Nil(t, protocolCorpusFindNode(node, "Unparsed NCP Payload"))
	}
}

func TestProtocolCorpusNCPAllPrefixesAndOpaqueBoundaries(t *testing.T) {
	for _, f := range ncpTestFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			for end := 0; end < len(f.wire); end++ {
				wire := f.wire[:end]
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), ncpCorpusRule, "NCP")
				// Unknown function data and unassociated reply data have no local
				// length field. A complete shorter datagram is a valid envelope,
				// not a claim that its application operation is complete.
				valid := f.opaque && ((!f.reply && end >= 7) || (f.reply && end >= 8))
				if valid {
					require.NoError(t, err, "prefix %d", end)
					node := ncpTestParse(t, wire, "NCP")
					require.Equal(t, false, ncpTestInfo(t, node)["Operation Body Validated"])
					continue
				}
				require.Error(t, err, "prefix %d", end)
				if end == 0 {
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), ncpCorpusRule, "NCPCarrier")
					require.ErrorContains(t, err, "ncp: empty carrier has no message")
					continue
				}
				carrier := ncpTestParse(t, wire, "NCPCarrier")
				ncpTestField(t, carrier, "Unparsed NCP Payload", wire, 0, 0, end)
				require.Nil(t, protocolCorpusFindNode(carrier, "Packet Type"))
			}
		})
	}
}

func TestProtocolCorpusNCPInvalidFieldsAndKnownLengths(t *testing.T) {
	f := ncpTestFixtures(t)
	invalid := [][]byte{}
	for _, i := range []int{0, 1, 2, 3, 4, 5, 9} {
		invalid = append(invalid, append(bytes.Clone(f[i].wire), 0))
	}
	for _, mutation := range []struct {
		fixture, offset int
		value           byte
	}{
		{0, 2, 1}, {0, 3, 0}, {4, 8, 4}, {4, 8, 6}, {4, 7, 1}, {5, 7, 0}, {5, 8, 1},
	} {
		wire := bytes.Clone(f[mutation.fixture].wire)
		wire[mutation.offset] = mutation.value
		invalid = append(invalid, wire)
	}
	// Unsupported common-header variants and the unrelated TCP wrapper do not
	// silently become service requests. Burst and LIP have different layouts.
	for _, kind := range []uint16{0, 0x3e3e, 0x7777, 0xbbbb, 0x4c69, 0xffff, 0x446d} {
		wire := bytes.Clone(f[0].wire)
		binary.BigEndian.PutUint16(wire, kind)
		invalid = append(invalid, wire)
	}
	for i, wire := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), ncpCorpusRule, "NCP")
			require.ErrorContains(t, err, "ncp:")
			carrier := ncpTestParse(t, wire, "NCPCarrier")
			ncpTestField(t, carrier, "Unparsed NCP Payload", wire, 0, 0, len(wire))
			require.Nil(t, protocolCorpusFindNode(carrier, "Packet Type"))
		})
	}
	for size := 512; size <= 32768; size *= 2 {
		wire := bytes.Clone(f[5].wire)
		binary.BigEndian.PutUint16(wire[7:], uint16(size))
		node := ncpTestParse(t, wire, "NCP")
		ncpTestField(t, node, "Proposed Buffer Size", uint64(size), 0, 7, 9)
	}
	// A different subfunction is retained with its declared exact boundary.
	unknown := bytes.Clone(f[4].wire)
	unknown[9] = 0xff
	node := ncpTestParse(t, unknown, "NCP")
	ncpTestField(t, node, "Function Data", unknown[10:], 0, 10, 14)
	require.Equal(t, true, ncpTestInfo(t, node)["Application Data Opaque"])
	require.Nil(t, protocolCorpusFindNode(node, "Target Connection"))
	// Reject the original connection number independently of its bad length.
	originalSeven := []byte{0x11, 0x11, 0, 0, 0, 0, 0}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(originalSeven), ncpCorpusRule, "NCP")
	require.ErrorContains(t, err, "ncp: create requires sequence 0 and connection FF")
}

func TestProtocolCorpusNCPImportsHeldReaderAndCallerConfig(t *testing.T) {
	valid := ncpTestFixtures(t)[4]
	for _, sample := range []struct {
		name  string
		wire  []byte
		valid bool
	}{
		{"valid", valid.wire, true}, {"late-invalid", append(bytes.Clone(valid.wire), 0x87), false}, {"short", valid.wire[:5], false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			// Three prefixed bytes establish nonzero global field spans. The
			// imported carrier is bounded independently of the held next message.
			var doc yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf("Package:\n  Test:\n    Prefix: raw,3\n    Message:\n      import: application-layer/ncp.yaml\n      node: NCPCarrier\n      length: %d\n", len(sample.wire)*8)), &doc))
			root, err := base.NewNodeTree(doc)
			require.NoError(t, err)
			held := []byte{0xde, 0xad, 0xbe, 0xef}
			input := append([]byte{0x12, 0x34, 0x56}, sample.wire...)
			reader := bytes.NewReader(append(bytes.Clone(input), held...))
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "Test"))
			node := base.GetNodeByPath(root, "@Test")
			require.Equal(t, input, NodeToBytes(node))
			if sample.valid {
				ncpTestRequireFixture(t, node, valid, 3)
			} else {
				ncpTestField(t, node, "Unparsed NCP Payload", sample.wire, 3, 0, len(sample.wire))
				require.Nil(t, protocolCorpusFindNode(node, "Packet Type"))
			}
			require.Equal(t, len(held), reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			value, err := bitReader.ReadBits(32)
			require.NoError(t, err)
			require.Equal(t, held, value)
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		})
	}
	for _, entry := range []string{"NCP", "NCPCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid.wire), ncpCorpusRule, entry)
		require.ErrorContains(t, err, "ncp: explicit")
		root, err := base.ParseRule("application-layer/ncp.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid.wire))*8-1)
		require.ErrorContains(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(valid.wire)), entry), "requires byte alignment")
	}
	reply := ncpTestFixtures(t)[8]
	config := map[string]any{"ncp_request_function": 20, "ncp_request_sequence": 1, "unrelated": "held"}
	node := protocolCorpusRequireBoundedRuleParseWithConfig(t, reply.wire, ncpCorpusRule, "NCP", config)
	ncpTestRequireFixture(t, node, reply, 0)
	require.Equal(t, map[string]any{"ncp_request_function": 20, "ncp_request_sequence": 1, "unrelated": "held"}, config)
	require.Nil(t, protocolCorpusFindNode(node, "Year"), "caller config cannot establish request/reply association")
}

func TestProtocolCorpusNCPResourceBoundsAndIsolation(t *testing.T) {
	header := ncpTestFixtures(t)[6].wire[:7]
	maximum := append(bytes.Clone(header), bytes.Repeat([]byte{0xa5}, 65527-len(header))...)
	node := ncpTestParse(t, maximum, "NCP")
	ncpTestField(t, node, "Function Data", maximum[7:], 0, 7, len(maximum))
	message := protocolCorpusFindNode(node, "Packet Type").Cfg.GetItem(base.CfgParent).(*base.Node)
	materialized := 0
	for _, field := range message.Children {
		if stream_parser.NodeHasResult(field) {
			materialized++
		}
	}
	require.Equal(t, 7, materialized, "maximum opaque payload remains one bounded leaf, not per-byte nodes")
	over := append(bytes.Clone(maximum), 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(over), ncpCorpusRule, "NCP")
	require.ErrorContains(t, err, "ncp: datagram size outside 7..65527 bytes")
	carrier := ncpTestParse(t, over, "NCPCarrier")
	ncpTestField(t, carrier, "Unparsed NCP Payload", over, 0, 0, len(over))
	for worker := 0; worker < 12; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Clone(header)
			wire[2], wire[3], wire[5] = byte(worker), byte(worker+1), byte(worker+2)
			wire = append(wire, byte(worker+3))
			node := ncpTestParse(t, wire, "NCP")
			ncpTestField(t, node, "Sequence Number", uint64(worker), 0, 2, 3)
			ncpTestField(t, node, "Function Data", wire[7:], 0, 7, 8)
			require.EqualValues(t, worker+1+(worker+2)*256, ncpTestInfo(t, node)["Connection Number"])
		})
	}
}
