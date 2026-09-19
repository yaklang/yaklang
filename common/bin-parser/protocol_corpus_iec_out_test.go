package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

var iecOutTestModes = []struct {
	name   string
	config map[string]any
}{
	{"scalar", nil},
	{"compiled", map[string]any{"outScalarLegacy": true}},
	{"uncached", map[string]any{"outProgramLegacy": true}},
}

// The tree snapshot is shared with the DICOM differential suite because it
// checks generic Node/NodeValue contracts, including actual Go value types,
// parent/context relationships, field order and every global bit interval.
func iecOutPrimitiveRoot(t *testing.T, entry string, config map[string]any) *base.Node {
	t.Helper()
	reference, err := base.ParseRule("iec61850.yaml")
	require.NoError(t, err)
	// IECLength/IECNumber/IECBytes are reusable root types, not Package
	// entrypoints. Instantiate them through the public custom-rule API.
	document := reference.Origin.(yaml.MapSlice)
	for i := range document {
		if document[i].Key == "Package" {
			document[i].Value = yaml.MapSlice{{Key: entry, Value: entry}}
		}
	}
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	for key, value := range config {
		root.Ctx.SetItem(key, value)
	}
	root.Ctx.SetItem(base.CtxInputConfig, config)
	return root
}

func iecOutCompare(t *testing.T, wire []byte, rule, entry string, check func(*testing.T, *base.Node)) {
	t.Helper()
	var wantTree []dicomNativeTreeSnapshot
	var wantValue dicomNativeValueSnapshot
	for _, mode := range iecOutTestModes {
		t.Run(mode.name, func(t *testing.T) {
			reader := newProtocolCorpusBoundedReader(wire)
			var node *base.Node
			var err error
			if entry == "IECLength" || entry == "IECNumber" || entry == "IECBytes" {
				root := iecOutPrimitiveRoot(t, entry, mode.config)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
				err = root.ParseSubNode(base.NewBitReader(reader), entry)
				node = base.GetNodeByPath(root, "@"+entry)
			} else {
				node, err = parser.ParseBinaryWithConfig(reader, rule, mode.config, entry)
			}
			require.NoError(t, err)
			require.Zero(t, reader.Len())
			check(t, node)
			tree, value := dicomNativeSnapshot(t, node, wire)
			if wantTree == nil {
				wantTree, wantValue = tree, value
			} else {
				require.Equal(t, wantTree, tree)
				require.Equal(t, wantValue, value)
			}
			// Imported nodes must receive the diagnostic override, otherwise a
			// differential could accidentally compare two optimized paths.
			var verify func(*base.Node)
			verify = func(n *base.Node) {
				for key, value := range mode.config {
					require.Equal(t, value, n.Ctx.GetItem(key), "%s context on %s", key, n.Name)
				}
				for _, child := range n.Children {
					verify(child)
				}
			}
			verify(node)
			for repeat := 0; repeat < 2; repeat++ {
				nextTree, nextValue := dicomNativeSnapshot(t, node, wire)
				require.Equal(t, tree, nextTree)
				require.Equal(t, value, nextValue)
			}
		})
	}
}

func TestProtocolCorpusIECOutPrimitiveEntries(t *testing.T) {
	var cases []struct {
		name, entry string
		wire        []byte
		want        any
	}
	for _, value := range []byte{0, 1, 127} {
		cases = append(cases, struct {
			name, entry string
			wire        []byte
			want        any
		}{fmt.Sprintf("length-short-%d", value), "IECLength", []byte{value}, uint8(value)})
	}
	for _, wire := range [][]byte{{0x81, 0}, {0x81, 255}, {0x82, 1, 2}, {0x83, 0, 0, 7}, {0x84, 0x7f, 0xff, 0xff, 0xff}, {0x84, 0xff, 0xff, 0xff, 0xff}} {
		var length uint64
		for _, octet := range wire[1:] {
			length = length*256 + uint64(octet)
		}
		var want any = int(length)
		if length > uint64(^uint(0)>>1) {
			want = int64(length)
		}
		cases = append(cases, struct {
			name, entry string
			wire        []byte
			want        any
		}{fmt.Sprintf("length-long-%x", wire), "IECLength", wire, want})
	}
	for width := 1; width <= 5; width++ {
		value := []byte{1, 2, 3, 4, 5}[:width]
		cases = append(cases, struct {
			name, entry string
			wire        []byte
			want        any
		}{fmt.Sprintf("number-%d", width), "IECNumber", append([]byte{0x82, byte(width)}, value...), protocolCorpusBERUnsigned(t, value)})
	}
	for _, length := range []int{0, 1, 127, 128, 258} {
		value := make([]byte, length)
		for i := range value {
			value[i] = byte(i*13 + 7)
		}
		wire := iecOutTLV(0x87, value, 0)
		var want any = value
		if length == 0 {
			want = ""
		}
		cases = append(cases, struct {
			name, entry string
			wire        []byte
			want        any
		}{fmt.Sprintf("bytes-%d", length), "IECBytes", wire, want})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iecOutCompare(t, tc.wire, "iec61850", tc.entry, func(t *testing.T, node *base.Node) {
				value, err := node.Result()
				require.NoError(t, err)
				require.Same(t, node, value.Origin)
				require.Equal(t, tc.want, value.Value)
			})
		})
	}
}

// Width 0 chooses a shortest definite length; explicit widths retain legal
// leading zeros accepted by the existing YAML. This wire builder is independent
// of both expression evaluators and is not used to generate corpus artifacts.
func iecOutTLV(tag byte, value []byte, width int) []byte {
	wire := []byte{tag}
	if width == 0 && len(value) < 128 {
		wire = append(wire, byte(len(value)))
	} else {
		if width == 0 {
			width = 2
		}
		wire = append(wire, 0x80|byte(width))
		for i := width - 1; i >= 0; i-- {
			wire = append(wire, byte(len(value)>>(i*8)))
		}
	}
	return append(wire, value...)
}

func iecOutSVFixture(width int) []byte {
	var sequence []byte
	for i := 0; i < 2; i++ {
		var fields []byte
		for index, value := range [][]byte{
			[]byte(fmt.Sprintf("station-%d", i)), []byte("dataset-a"), {0x01, byte(0x23 + i)}, {0, 0, 1, byte(2 + i)},
			{0x65, 0x32, 0x10, 0, 0x12, 0x34, 0x56, 0}, {2}, {0x12, 0x34}, bytes.Repeat([]byte{byte(0x11 + i), 0xab}, 100),
			{0, 1}, {0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		} {
			fields = append(fields, iecOutTLV(byte(0x80+index), value, width)...)
		}
		sequence = append(sequence, iecOutTLV(0x30, fields, width)...)
	}
	body := append(iecOutTLV(0x80, []byte{2}, width), iecOutTLV(0xa2, sequence, width)...)
	pdu := iecOutTLV(0x60, body, width)
	wire := []byte{0x40, 0x03, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(pdu)+8))
	return append(wire, pdu...)
}

func iecOutRequireSV(t *testing.T, wire []byte, node *base.Node) {
	t.Helper()
	fields := protocolCorpusIECFrame(t, wire, node, 0x60)
	require.Len(t, fields, 2)
	require.Equal(t, byte(0x80), fields[0].tag)
	require.Equal(t, byte(0xa2), fields[1].tag)
	count := protocolCorpusBERUnsigned(t, fields[0].value)
	protocolCorpusRequireValue(t, node, "ASDU Count", count)
	values := protocolCorpusBERFields(t, fields[1].value)
	parsed := protocolCorpusNodesNamed(node, "ASDU")
	require.Len(t, values, int(count))
	require.Len(t, parsed, int(count))
	names := []string{"SV ID", "Dataset", "Sample Counter", "Configuration Revision", "Reference Time", "Sample Synchronization", "Sample Rate", "Sample Data", "Sample Mode", "Grandmaster Identity"}
	for i, asdu := range values {
		require.Equal(t, byte(0x30), asdu.tag)
		for _, field := range protocolCorpusBERFields(t, asdu.value) {
			require.GreaterOrEqual(t, field.tag, byte(0x80))
			require.LessOrEqual(t, field.tag, byte(0x89))
			name := names[field.tag-0x80]
			switch field.tag {
			case 0x82, 0x83, 0x85, 0x86, 0x88:
				protocolCorpusRequireValue(t, parsed[i], name, protocolCorpusBERUnsigned(t, field.value))
			default:
				protocolCorpusRequireValue(t, parsed[i], name, field.value)
			}
		}
	}
}

func TestProtocolCorpusIECOutSVCompleteTreeAndImports(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/mgadelha-sv/mgadelha-sv.cap")
	require.Len(t, packets, 10161)
	for _, index := range []int{0, 255, 10160} {
		frame := packets[index]
		require.Equal(t, uint16(0x88ba), binary.BigEndian.Uint16(frame[16:18]))
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("captured-%d/import-%t", index+1, imported), func(t *testing.T) {
				wire, rule, entry := frame[18:], "iec61850", "SampledValues"
				if imported {
					wire, rule, entry = frame, "ethernet", "Ethernet"
				}
				iecOutCompare(t, wire, rule, entry, func(t *testing.T, node *base.Node) {
					iecOutRequireSV(t, frame[18:], node)
				})
			})
		}
	}
	for _, width := range []int{0, 2, 3, 4} {
		t.Run(fmt.Sprintf("two-asdu-all-fields/length-width-%d", width), func(t *testing.T) {
			wire := iecOutSVFixture(width)
			iecOutCompare(t, wire, "iec61850", "SampledValues", func(t *testing.T, node *base.Node) { iecOutRequireSV(t, wire, node) })
		})
	}
}

func TestProtocolCorpusIECOutGOOSECompleteTreeAndImports(t *testing.T) {
	packets := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/iti-ics/iti-goose.pcap")
	require.Len(t, packets, 8)
	for _, index := range []int{0, 7} {
		frame := packets[index]
		for _, imported := range []bool{false, true} {
			t.Run(fmt.Sprintf("captured-%d/import-%t", index+1, imported), func(t *testing.T) {
				wire, rule, entry := frame[14:], "iec61850", "GOOSE"
				if imported {
					wire, rule, entry = frame, "ethernet", "Ethernet"
				}
				iecOutCompare(t, wire, rule, entry, func(t *testing.T, node *base.Node) {
					fields := protocolCorpusIECFrame(t, frame[14:], node, 0x61)
					require.Len(t, fields, 12)
					for i, name := range []string{"Control Block Reference", "Time Allowed To Live", "Dataset", "GOOSE ID", "Timestamp", "State Number", "Sequence Number", "Simulation", "Configuration Revision", "Needs Commissioning", "Dataset Entry Count"} {
						require.Equal(t, byte(0x80+i), fields[i].tag)
						switch i {
						case 0, 2, 3, 4:
							protocolCorpusRequireValue(t, node, name, fields[i].value)
						default:
							protocolCorpusRequireValue(t, node, name, protocolCorpusBERUnsigned(t, fields[i].value))
						}
					}
				})
			})
		}
	}
}

func TestProtocolCorpusIECOutGenerationAndResultIsolation(t *testing.T) {
	for _, mode := range iecOutTestModes {
		t.Run(mode.name, func(t *testing.T) {
			root := iecOutPrimitiveRoot(t, "IECBytes", mode.config)
			// A real structured generator input, not raw-byte parsing disguised
			// as generation. The length operator processes its octet list and
			// then its out expression supplies the Value field's byte length.
			data := map[string]any{"Tag": 0x87, "Length": []any{0x82, 0, 3}, "Value": []byte{0x12, 0x34, 0x56}}
			require.NoError(t, root.GenerateSubNode(data, "IECBytes"))
			node := base.GetNodeByPath(root, "@IECBytes")
			wire := []byte{0x87, 0x82, 0, 3, 0x12, 0x34, 0x56}
			require.Equal(t, wire, NodeToBytes(node))
			beforeTree, beforeValue := dicomNativeSnapshot(t, node, wire)
			beforeCount := len(node.Cfg.GetItem(base.CfgOptionFuns).([]base.NodeConfigFun))
			for i := 0; i < 3; i++ {
				value, err := node.Result()
				require.NoError(t, err)
				require.Equal(t, []byte{0x12, 0x34, 0x56}, value.Value)
				value.Value.([]byte)[0] = 0xff
				tree, snapshot := dicomNativeSnapshot(t, node, wire)
				require.Equal(t, beforeTree, tree)
				require.Equal(t, beforeValue, snapshot)
				require.Len(t, node.Cfg.GetItem(base.CfgOptionFuns), beforeCount)
			}
			valueNode := protocolCorpusFindNode(node, "Value")
			require.Equal(t, [2]uint64{32, 56}, stream_parser.GetNodeResultPos(valueNode))
		})
	}
}

func iecOutEthernet(payload []byte, etherType uint16, tags int) ([]byte, int) {
	frame := []byte{1, 12, 205, 4, 0, 1, 2, 0, 0, 0, 0, 7}
	put := func(value uint16) { frame = append(frame, byte(value>>8), byte(value)) }
	switch tags {
	case 1:
		put(0x8100)
		put(0xa123)
	case 2:
		put(0x88a8)
		put(0x6123)
		put(0x8100)
		put(0xa234)
	case 3:
		put(0x88a8)
		put(0x6123)
	}
	put(etherType)
	start := len(frame)
	return append(frame, payload...), start
}

func TestProtocolCorpusIECOutCarrierBoundaries(t *testing.T) {
	sv := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/mgadelha-sv/mgadelha-sv.cap")[0][18:]
	goose := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/iti-ics/iti-goose.pcap")[0][14:]
	for _, protocol := range []struct {
		entry     string
		etherType uint16
		wire      []byte
	}{{"SampledValues", 0x88ba, sv}, {"GOOSE", 0x88b8, goose}} {
		t.Run(protocol.entry, func(t *testing.T) {
			// The length header excludes optional link padding. All shorter
			// complete-PDU prefixes must reject at the direct entry, with the
			// exact same errors and reader position on both evaluation paths.
			pduEnd := int(binary.BigEndian.Uint16(protocol.wire[2:4]))
			for cut := 0; cut < pduEnd; cut++ {
				var wantError string
				var wantRemaining int
				for modeIndex, mode := range iecOutTestModes[:2] {
					reader := newProtocolCorpusBoundedReader(protocol.wire[:cut])
					node, err := parser.ParseBinaryWithConfig(reader, "iec61850", mode.config, protocol.entry)
					require.Error(t, err, "cut=%d mode=%s", cut, mode.name)
					require.Nil(t, node)
					if modeIndex == 0 {
						wantError, wantRemaining = err.Error(), reader.Len()
					} else {
						require.Equal(t, wantError, err.Error(), "cut=%d", cut)
						require.Equal(t, wantRemaining, reader.Len(), "cut=%d", cut)
					}
				}
			}
			badTag := bytes.Clone(protocol.wire[:pduEnd])
			badTag[8] ^= 0xff
			badLength := bytes.Clone(protocol.wire[:pduEnd])
			binary.BigEndian.PutUint16(badLength[2:4], uint16(pduEnd+1))
			for tags := 0; tags < 4; tags++ {
				// Exercise every short payload on both untagged and QinQ
				// carriers. Single-tag routes share these carrier rules and have
				// independent empty/middle/final prefix and full-message checks.
				var invalid [][]byte
				if tags == 0 || tags == 2 {
					for cut := 0; cut < pduEnd; cut++ {
						invalid = append(invalid, protocol.wire[:cut])
					}
				} else {
					invalid = append(invalid, protocol.wire[:0], protocol.wire[:pduEnd/2], protocol.wire[:pduEnd-1])
				}
				invalid = append(invalid, badTag, badLength)
				for _, payload := range invalid {
					frame, start := iecOutEthernet(payload, protocol.etherType, tags)
					for _, mode := range iecOutTestModes[:2] {
						reader := newProtocolCorpusBoundedReader(frame)
						node, err := parser.ParseBinaryWithConfig(reader, "ethernet", mode.config, "Ethernet")
						if len(payload) == 0 {
							require.ErrorContains(t, err, "iec61850: empty carrier has no message")
							require.Nil(t, node)
							continue
						}
						require.NoError(t, err, "tags=%d payload=%d mode=%s", tags, len(payload), mode.name)
						require.Zero(t, reader.Len())
						require.Equal(t, frame, NodeToBytes(node))
						field := protocolCorpusFindNode(node, "Unparsed IEC Payload")
						require.NotNil(t, field)
						require.Equal(t, [2]uint64{uint64(start) * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(field))
						value, err := field.Result()
						require.NoError(t, err)
						require.Equal(t, len(payload), len(value.Value.([]byte)))
						require.True(t, bytes.Equal(payload, value.Value.([]byte)))
						require.Nil(t, protocolCorpusFindNode(node, "APPID"), "failed speculative fields must not remain attached")
					}
				}
				frame, _ := iecOutEthernet(protocol.wire, protocol.etherType, tags)
				iecOutCompare(t, frame, "ethernet", "Ethernet", func(t *testing.T, node *base.Node) {
					require.Nil(t, protocolCorpusFindNode(node, "Unparsed IEC Payload"))
					protocolCorpusRequireValue(t, node, "APPID", uint64(binary.BigEndian.Uint16(protocol.wire[:2])))
				})
				// Keep the actual BitReader to prove that nested successful and
				// recovered carrier transactions leave no replay/backup behind.
				for _, payload := range [][]byte{protocol.wire, append(bytes.Clone(protocol.wire), 0xa5, 0x5a), badTag, badLength, protocol.wire[:1]} {
					wire, _ := iecOutEthernet(payload, protocol.etherType, tags)
					root, err := base.ParseRule("ethernet.yaml")
					require.NoError(t, err)
					root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
					reader := bytes.NewReader(wire)
					bitReader := base.NewBitReader(reader)
					require.NoError(t, root.ParseSubNode(bitReader, "Ethernet"))
					require.Zero(t, reader.Len())
					require.ErrorContains(t, bitReader.Recovery(), "no backup")
					require.ErrorContains(t, bitReader.PopBackup(), "no backup")
					_, err = bitReader.ReadBits(8)
					require.ErrorIs(t, err, io.EOF)
				}
			}
		})
	}
}

func TestProtocolCorpusIECOutEditedSourceFallback(t *testing.T) {
	for _, mode := range iecOutTestModes {
		t.Run(mode.name, func(t *testing.T) {
			root := iecOutPrimitiveRoot(t, "IECNumber", mode.config)
			wire := []byte{0x82, 2, 0x12, 0x34}
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), "IECNumber"))
			node := base.GetNodeByPath(root, "@IECNumber")
			original := node.Cfg.GetString("out")
			prefix := "calls = 0\nif node.Ctx.Has(\"scalar-out-calls\") { calls = node.Ctx.GetItem(\"scalar-out-calls\") }\nnode.Ctx.SetItem(\"scalar-out-calls\", calls+1)\n"
			for _, fail := range []bool{false, true, false} {
				source := prefix + original
				if fail {
					source = prefix + "panic(\"scalar-custom-source\")\n"
				}
				node.Cfg.SetItem("out", source)
				node.Ctx.SetItem("scalar-out-calls", 0)
				value, err := node.Result()
				if fail {
					require.ErrorContains(t, err, "scalar-custom-source")
				} else {
					require.NoError(t, err)
					require.Equal(t, uint64(0x1234), value.Value)
				}
				require.Equal(t, 1, node.Ctx.GetItem("scalar-out-calls"), "source edits must execute once, not be ignored or retried")
				require.Equal(t, source, node.Cfg.GetString("out"))
			}
			node.Cfg.SetItem("out", original)
			value, err := node.Result()
			require.NoError(t, err)
			require.Equal(t, uint64(0x1234), value.Value)
		})
	}
}
