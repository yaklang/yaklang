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
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func zigbeeTestBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}

// Independent serialization literals, not emitted by the production decoder.
// CSA 05-3474-23 Figures 2-3..2-5, 3-4..3-6 and 4-10. MAC data header uses
// short addressing, same-PAN compression and no FCS. No application decoding
// or cryptographic correctness is claimed by these envelope fixtures.
var zigbeeTestFixtures = []struct{ name, wire string }{
	{"data-unicast", "4188073412785600000800785600001e09000106000401020aaa55"},
	{"data-broadcast", "4188073412785600000800ffff00001e09080106000401020b66"},
	{"data-group", "4188073412785600000800785600001e090c341206000401020c77"},
	{"ack-data", "4188073412785600000800785600001e09020106000401020d"},
	{"ack-command", "4188073412785600000800785600001e09120e"},
	{"switch-key", "4188073412785600000800785600001e09410f0907"},
	{"transport-network", "4188073412785600000800785600001e0941100501000102030405060708090a0b0c0d0e0f0901020304050607081112131415161718"},
	{"data-fragment", "4188073412785600000800785600001e09800106000401021101030102"},
	{"ack-fragment", "4188073412785600000800785600001e0982010600040102120203aa"},
	{"source-route", "418807341278560000081c785600001e0901020304050607081112131415161718020134127856120e"},
	{"nwk-leave", "41880734127856000009007856000001090420"},
	{"nwk-protected-explicit", "4188073412785600000802785600001e092d01000000010203040506070803aabbccdd11223344"},
	{"nwk-protected-context", "4188073412785600000802785600001e092801000000010203040506070803aabbccdd11223344"},
	{"unknown-command", "4188073412785600000800785600001e09410ffffeabcd"},
	{"beacon", "00800734127856ffcf0000002084010203040506070856341209"},
	{"ack-mac", "02000c"},
}

func zigbeeTestInfo(t *testing.T, n *base.Node) map[string]any {
	t.Helper()
	var found map[string]any
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if m, ok := n.Cfg.GetItem("additionInfo").(map[string]any); ok && m["Profile"] == "Zigbee bounded MAC Beacon NWK APS fields" {
			found = m
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	require.NotNil(t, found)
	return found
}

func zigbeeTestField(t *testing.T, n *base.Node, name, typ string, start, end uint64, value any) {
	t.Helper()
	field := protocolCorpusFindNode(n, name)
	require.NotNil(t, field, name)
	require.Equal(t, typ, field.Cfg.GetItem(base.CfgType), name)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(field), name)
	protocolCorpusRequireValue(t, n, name, value)
}

// Every leaf is independently checked against original bytes; containers must
// exactly partition their parent, with no overlaps, lost padding or duplication.
func zigbeeTestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	var walk func(*base.Node, uint64, bool) uint64
	walk = func(n *base.Node, start uint64, native bool) uint64 {
		for _, child := range n.Children {
			if child.Name == "MAC Frame Control" {
				native = true
			}
		}
		length := stream_parser.CalcNodeConsumedLength(n)
		if len(n.Children) == 0 && !stream_parser.NodeHasResult(n) {
			require.Zero(t, length)
			return start
		}
		result, e := n.Result()
		require.NoError(t, e, n.Name)
		require.Same(t, n, result.Origin)
		if stream_parser.NodeHasResult(n) {
			span := stream_parser.GetNodeResultPos(n)
			require.Equal(t, [2]uint64{start, start + length}, span, n.Name)
			require.Zero(t, (span[0]-offset)%8)
			require.Zero(t, length%8)
			begin, end := (span[0]-offset)/8, (span[1]-offset)/8
			require.LessOrEqual(t, end, uint64(len(wire)))
			raw := wire[begin:end]
			switch n.Cfg.GetItem(base.CfgType) {
			case "raw":
				require.Equal(t, raw, bytesVal(t, result), n.Name)
			case "uint8", "uint16", "uint32":
				var value uint64
				for i, b := range raw {
					value |= uint64(b) << uint(i*8)
				}
				require.Equal(t, value, uintVal(t, result), n.Name)
			default:
				t.Fatalf("unexpected Zigbee leaf type %v", n.Cfg.GetItem(base.CfgType))
			}
			return span[1]
		}
		pos := start
		for index, child := range n.Children {
			require.Same(t, n, child.Cfg.GetItem(base.CfgParent))
			// TryProcess has a deliberately isolated context at the carrier's
			// import boundary; all descendants of the native frame share its context.
			if native {
				require.Same(t, n.Ctx, child.Ctx)
			}
			if n.Cfg.GetBool(stream_parser.CfgIsList) {
				require.Equal(t, index, child.Cfg.GetItem(stream_parser.CfgElementIndex))
			}
			pos = walk(child, pos, native)
		}
		require.Equal(t, start+length, pos, n.Name)
		return pos
	}
	require.Equal(t, offset+uint64(len(wire))*8, walk(n, offset, false))
}

func zigbeeTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "zigbee", entry)
	require.Equal(t, wire, NodeToBytes(n))
	zigbeeTestTree(t, n, wire, 0)
	return n
}

func TestProtocolCorpusZigbeeEveryOriginalRecord(t *testing.T) {
	const corpus = "testdata/protocol-corpus/captures/scapy/"
	for _, capture := range []struct {
		file, sha string
		count     int
		fcs       bool
	}{
		{"scapy-zigbee-join.pcap", "2b33b210d972515babb348a14454359602fa6c919247e038446c5404ec78a823", 54, false},
		{"scapy-zigbee-skke.pcap", "a48e8339fb54be229de0d449936667f22267bda8bc93982d870baa389ba9ad36", 1, true},
	} {
		t.Run(capture.file, func(t *testing.T) {
			data, e := os.ReadFile(corpus + capture.file)
			require.NoError(t, e)
			require.Equal(t, capture.sha, fmt.Sprintf("%x", sha256.Sum256(data)))
			frames := protocolCorpusAuditPackets(t, corpus+capture.file)
			require.Len(t, frames, capture.count)
			counts := map[uint16]int{}
			apsCount := 0
			for index, wire := range frames {
				t.Run(fmt.Sprintf("frame-%d", index+1), func(t *testing.T) {
					entry := "Zigbee"
					if capture.fcs {
						entry = "ZigbeeFCS"
					}
					n := zigbeeTestParse(t, wire, entry)
					info := zigbeeTestInfo(t, n)
					for _, flag := range []string{"Payload Decrypted", "Payload Authenticated", "Application Semantics Decoded", "Fragment Reassembly Performed", "Protection Level Inferred"} {
						require.Equal(t, false, info[flag])
					}
					require.Equal(t, capture.fcs, info["FCS Validated"])
					fc := binary.LittleEndian.Uint16(wire)
					kind := fc & 7
					counts[kind]++
					zigbeeTestField(t, n, "MAC Frame Control", "uint16", 0, 16, uint64(fc))
					zigbeeTestField(t, n, "MAC Sequence Number", "uint8", 16, 24, uint64(wire[2]))
					// The MAC address offset is derived from original frame-control
					// bits, never from the production tree's reported consumed length.
					p := 3
					dest, src := (fc>>10)&3, (fc>>14)&3
					if dest != 0 {
						p += 2
						if dest == 2 {
							p += 2
						} else {
							p += 8
						}
					}
					if src != 0 {
						if fc&64 == 0 {
							p += 2
						}
						if src == 2 {
							p += 2
						} else {
							p += 8
						}
					}
					switch kind {
					case 0:
						require.Equal(t, 7, p)
						require.Len(t, wire, 26)
						zigbeeTestField(t, n, "Superframe Specification", "uint16", 56, 72, uint64(binary.LittleEndian.Uint16(wire[7:9])))
						zigbeeTestField(t, n, "Beacon Protocol ID", "uint8", 88, 96, uint64(0))
						zigbeeTestField(t, n, "Beacon Stack Profile and Version", "uint8", 96, 104, uint64(0x20))
						zigbeeTestField(t, n, "Beacon Extended PAN ID", "raw", 112, 176, wire[14:22])
						zigbeeTestField(t, n, "Beacon Transmit Offset", "uint32", 176, 200, uint64(0xffffff))
						require.Equal(t, true, info["Beacon Present"])
						require.Nil(t, protocolCorpusFindNode(n, "Zigbee NWK"))
					case 1:
						nfc := binary.LittleEndian.Uint16(wire[p : p+2])
						zigbeeTestField(t, n, "NWK Frame Control", "uint16", uint64(p*8), uint64((p+2)*8), uint64(nfc))
						zigbeeTestField(t, n, "NWK Destination", "uint16", uint64((p+2)*8), uint64((p+4)*8), uint64(binary.LittleEndian.Uint16(wire[p+2:p+4])))
						zigbeeTestField(t, n, "NWK Source", "uint16", uint64((p+4)*8), uint64((p+6)*8), uint64(binary.LittleEndian.Uint16(wire[p+4:p+6])))
						p += 8
						if nfc&0x800 != 0 {
							zigbeeTestField(t, n, "NWK Destination Extended", "raw", uint64(p*8), uint64((p+8)*8), wire[p:p+8])
							p += 8
						}
						if nfc&0x1000 != 0 {
							zigbeeTestField(t, n, "NWK Source Extended", "raw", uint64(p*8), uint64((p+8)*8), wire[p:p+8])
							p += 8
						}
						require.Zero(t, nfc&0x400, "original captures have no source route")
						protected := nfc&0x200 != 0
						if !protected {
							apsCount++
							zigbeeTestField(t, n, "APS Frame Control", "uint8", uint64(p*8), uint64((p+1)*8), uint64(wire[p]))
							protected = wire[p]&0x20 != 0
							p += 2
						}
						if protected {
							control := wire[p]
							zigbeeTestField(t, n, "Auxiliary Control", "uint8", uint64(p*8), uint64((p+1)*8), uint64(control))
							require.Zero(t, control&7)
							p += 5
							if control&0x20 != 0 {
								p += 8
							}
							if (control>>3)&3 == 1 {
								p++
							}
							zigbeeTestField(t, n, "Protected Payload and MIC", "raw", uint64(p*8), uint64(len(wire)*8), wire[p:])
							require.Nil(t, protocolCorpusFindNode(n, "Message Integrity Code"))
						} else {
							require.True(t, capture.fcs)
							zigbeeTestField(t, n, "APS Command ID", "uint8", 152, 160, uint64(5))
							zigbeeTestField(t, n, "Transport Key Type", "uint8", 160, 168, uint64(1))
							zigbeeTestField(t, n, "Transport Key Material", "raw", 168, 296, wire[21:37])
							zigbeeTestField(t, n, "Transport Destination Extended", "raw", 304, 368, wire[38:46])
							zigbeeTestField(t, n, "Transport Source Extended", "raw", 368, 432, wire[46:54])
							zigbeeTestField(t, n, "Frame Check Sequence", "uint16", 432, 448, uint64(0x244f))
						}
					case 2:
						require.Len(t, wire, 5)
						require.Equal(t, true, info["Uninterpreted MAC Tail"])
						zigbeeTestField(t, n, "Uninterpreted MAC Acknowledgment Tail", "raw", 24, 40, wire[3:])
						require.Nil(t, protocolCorpusFindNode(n, "Frame Check Sequence"))
						require.Nil(t, protocolCorpusFindNode(n, "Zigbee NWK"))
					case 3:
						zigbeeTestField(t, n, "MAC Command ID", "uint8", uint64(p*8), uint64((p+1)*8), uint64(wire[p]))
						require.Equal(t, true, info["MAC Command Decoded"])
					default:
						t.Fatal("unexpected original MAC frame type")
					}
				})
			}
			if capture.fcs {
				require.Equal(t, map[uint16]int{1: 1}, counts)
				require.Equal(t, 1, apsCount)
			} else {
				require.Equal(t, map[uint16]int{0: 8, 1: 28, 2: 9, 3: 9}, counts)
				require.Equal(t, 2, apsCount)
			}
		})
	}
}

func TestProtocolCorpusZigbeeIndependentLayouts(t *testing.T) {
	for _, fixture := range zigbeeTestFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			wire := zigbeeTestBytes(t, fixture.wire)
			for _, entry := range []string{"Zigbee", "ZigbeeCarrier"} {
				n := zigbeeTestParse(t, wire, entry)
				info := zigbeeTestInfo(t, n)
				require.Equal(t, false, info["Payload Decrypted"])
				switch fixture.name {
				case "data-unicast":
					zigbeeTestField(t, n, "APS Cluster ID", "uint16", 152, 168, uint64(6))
					zigbeeTestField(t, n, "APS Application Data", "raw", 200, 216, []byte{0xaa, 0x55})
				case "data-group":
					zigbeeTestField(t, n, "APS Group Address", "uint16", 144, 160, uint64(0x1234))
				case "beacon":
					zigbeeTestField(t, n, "Beacon Transmit Offset", "uint32", 176, 200, uint64(0x123456))
				case "source-route":
					require.Len(t, protocolCorpusFindNode(n, "Relays").Children, 2)
				case "switch-key":
					protocolCorpusRequireValue(t, n, "Switch Key Sequence Number", uint64(7))
				case "data-fragment", "ack-fragment":
					require.Equal(t, true, info["APS Fragment Present"])
				case "nwk-protected-context":
					require.Nil(t, protocolCorpusFindNode(n, "Message Integrity Code"))
				case "nwk-protected-explicit":
					protocolCorpusRequireValue(t, n, "Message Integrity Code", []byte{0x11, 0x22, 0x33, 0x44})
				case "unknown-command":
					require.Equal(t, false, info["APS Command Decoded"])
					protocolCorpusRequireValue(t, n, "Uninterpreted APS Command Data", []byte{0xfe, 0xab, 0xcd})
				}
			}
		})
	}
}

func zigbeeTestRaw(t *testing.T, wire []byte, entry string) {
	t.Helper()
	n := zigbeeTestParse(t, wire, entry)
	zigbeeTestField(t, n, "Unparsed Zigbee Frame", "raw", 0, uint64(len(wire))*8, wire)
	require.Nil(t, protocolCorpusFindNode(n, "MAC Frame Control"))
}

func TestProtocolCorpusZigbeeShortPrefixesAndInvalidFlags(t *testing.T) {
	// Strict fixed-layout commands have no valid shorter prefix. Data/unknown
	// payload truncation cannot be inferred without a length field: not asserted.
	for _, index := range []int{3, 4, 5, 6, 8, 9, 10} {
		wire := zigbeeTestBytes(t, zigbeeTestFixtures[index].wire)
		for cut := 0; cut < len(wire); cut++ {
			_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "zigbee", "Zigbee")
			require.Error(t, e, "%s prefix %d", zigbeeTestFixtures[index].name, cut)
			if cut > 0 {
				zigbeeTestRaw(t, wire[:cut], "ZigbeeCarrier")
			}
		}
	}
	data := zigbeeTestBytes(t, zigbeeTestFixtures[0].wire)
	for _, tc := range []struct {
		index int
		value byte
	}{{0, 0x49}, {1, 0xa8}, {1, 0x84}, {9, 0x0c}, {9, 0x0a}, {9, 0x88}, {10, 0x40}, {10, 1}, {17, 3}, {17, 4}, {17, 0x10}, {17, 0x48}} {
		wire := bytes.Clone(data)
		wire[tc.index] = tc.value
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "zigbee", "Zigbee")
		require.Error(t, e, "mutation %+v", tc)
		zigbeeTestRaw(t, wire, "ZigbeeCarrier")
	}
	// Literal FCS oracle: the original unmodified frame succeeds; changing any
	// byte in that exact record must fail CRC and preserve the whole fallback.
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/scapy/scapy-zigbee-skke.pcap")
	for i := range frames[0] {
		wire := bytes.Clone(frames[0])
		wire[i] ^= 1
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "zigbee", "ZigbeeFCS")
		require.ErrorContains(t, e, "frame check sequence")
		zigbeeTestRaw(t, wire, "ZigbeeFCSCarrier")
	}
}

func TestProtocolCorpusZigbeeImportedOffsetsAndFallback(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{true, false} {
			t.Run(fmt.Sprintf("offset-%d/%t", offset, valid), func(t *testing.T) {
				wire := zigbeeTestBytes(t, zigbeeTestFixtures[5].wire)
				if !valid {
					wire = append(wire, 0xff)
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				source := fmt.Sprintf("endian: big\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Frame\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Frame\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Frame: \"import:zigbee.yaml;node:ZigbeeCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, e := base.NewNodeTree(doc)
				require.NoError(t, e)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("caller-marker", "held")
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				frame := protocolCorpusFindNode(n, "Frame")
				zigbeeTestTree(t, frame, wire, offset)
				if valid {
					zigbeeTestInfo(t, frame)
				} else {
					zigbeeTestField(t, frame, "Unparsed Zigbee Frame", "raw", offset, offset+uint64(len(wire))*8, wire)
					require.Nil(t, protocolCorpusFindNode(frame, "MAC Frame Control"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.Equal(t, "held", root.Ctx.GetItem("caller-marker"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
				_, e = reader.ReadBits(8)
				require.ErrorIs(t, e, io.EOF)
			})
		}
	}
}

func TestProtocolCorpusZigbeePhysicalBoundsAndIsolation(t *testing.T) {
	wire := zigbeeTestBytes(t, zigbeeTestFixtures[5].wire)
	for _, entry := range []string{"Zigbee", "ZigbeeCarrier"} {
		_, e := parser.ParseBinary(bytes.NewReader(wire), "zigbee", entry)
		require.ErrorContains(t, e, "explicit")
		_, e = parser.GenerateBinary(map[string]any{}, "zigbee", entry)
		require.Error(t, e)
		for cut := 0; cut < len(wire); cut++ {
			root, e := base.ParseRule("zigbee.yaml")
			require.NoError(t, e)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.Error(t, root.ParseSubNode(reader, entry))
			require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@"+entry), "MAC Frame Control"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
	maximum := append(zigbeeTestBytes(t, zigbeeTestFixtures[0].wire[:50]), bytes.Repeat([]byte{0xaa}, 100)...)
	require.Len(t, maximum, 125)
	zigbeeTestParse(t, maximum, "Zigbee")
	over := append(bytes.Clone(maximum), 0)
	_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(over), "zigbee", "Zigbee")
	require.Error(t, e)
	zigbeeTestRaw(t, over, "ZigbeeCarrier")
	for worker := 0; worker < 8; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			input := bytes.Clone(wire)
			input[2] = byte(worker)
			cfg := map[string]any{"caller-marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, input, "zigbee", "Zigbee", cfg)
			protocolCorpusRequireValue(t, n, "MAC Sequence Number", uint64(worker))
			info := zigbeeTestInfo(t, n)
			require.Equal(t, false, info["Payload Decrypted"])
			info["Payload Decrypted"] = true
			require.Equal(t, map[string]any{"caller-marker": worker}, cfg)
		})
	}
}
