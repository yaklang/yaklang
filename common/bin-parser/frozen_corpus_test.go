package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// This adapter consumes answers sealed at the old head. It must not derive
// expected values from the output of the implementation being tested.
func TestFrozenCorpusSelectedInputs(t *testing.T) {
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	require.NotEmpty(t, inventories)
	require.Equal(t, "2fb090d177c3bf52277f240b2dca05ea80216485", inventories[0].SourceSHA)
	require.Len(t, inventories[0].Cases, 694)
	parses, rejections := 0, 0
	for _, answers := range inventories {
		for _, c := range answers.Cases {
			for i, e := range c.Expectations {
				if e.Kind != "selected_input_parse" && e.Kind != "selected_input_rejection" {
					continue
				}
				if e.Kind == "selected_input_parse" {
					parses++
				} else {
					rejections++
				}
				t.Run(c.ID+"/"+string(rune('a'+i)), func(t *testing.T) {
					var contract protocolCorpusParseContract
					selection := frozenSelection(t, e.Selection)
					require.NoError(t, json.Unmarshal(selection, &contract))
					info := ProtocolInfo{Name: e.Protocol, Layer: e.Layer, RuleFile: e.Rule}
					if info.Layer == "" {
						info.Layer = contract.Layer
					}
					for _, candidate := range ProtocolCatalog {
						if candidate.Name == e.Protocol {
							if info.Layer == "" {
								info.Layer = candidate.Layer
							}
							if info.RuleFile == "" {
								info.RuleFile = candidate.RuleFile
							}
						}
					}
					if contract.Layer == "" {
						contract.Layer = info.Layer
					}
					if contract.RuleFile == "" {
						contract.RuleFile = info.RuleFile
					}
					if contract.EntryNode == "" && e.Entry != "default" {
						contract.EntryNode = e.Entry
					}
					require.NotEmpty(t, contract.RuleFile, "frozen answer has no executable rule")
					require.NotNil(t, e.Frame)
					capture := protocolCorpusCapture{ID: c.ID, CaptureFile: c.Input.OriginalPath, PacketCount: c.Facts.PacketCount, LinkType: c.Facts.LinkType, RepresentativeFrame: &protocolCorpusFrame{Number: e.Frame.Number, LengthBytes: e.Frame.LengthBytes, SHA256: e.Frame.SHA256}}
					input := protocolCorpusParseInput(t, filepath.Join("..", ".."), capture, info, contract)
					node, err := protocolCorpusParseRule(newProtocolCorpusBoundedReader(input), contract)
					if e.Kind == "selected_input_rejection" {
						require.Error(t, err)
						require.Nil(t, node)
						protocolCorpusRequireExpectedFailure(t, c.ID, protocolCorpusRejectionSpec{Contract: contract, ExpectedFailureClass: protocolCorpusFailureClass(e.Expected.FailureClass), ExpectedErrorContains: e.Expected.ErrorContains}, err)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, node)
					fieldName := func(name string) string { return name }
					if (c.ID == "gen-icmpv6-mld" || c.ID == "pr5023-gen-icmpv6-mld") && e.Protocol == "ICMPv6 MLD" && !protocolCorpusHasNode(node, "Maximum Response Delay") {
						// The sealed old label called the v2 code or Report/Done's
						// unused word a response delay. Preserve its original numeric
						// assertion under the corrected label, proving literal bytes
						// and the exact old word position; never alter frozen answers.
						require.GreaterOrEqual(t, len(input), 6)
						corrected := "Unused Delay Word"
						if input[0] == 130 {
							require.GreaterOrEqual(t, len(input), 28)
							corrected = "Maximum Response Code"
						} else {
							require.Contains(t, []byte{131, 132}, input[0])
						}
						word := protocolCorpusFindNode(node, corrected)
						require.NotNil(t, word)
						require.Equal(t, [2]uint64{32, 48}, stream_parser.GetNodeResultPos(word))
						protocolCorpusRequireValue(t, node, corrected, uint64(binary.BigEndian.Uint16(input[4:6])))
						fieldName = func(name string) string {
							if name == "Maximum Response Delay" {
								return corrected
							}
							return name
						}
					}
					for _, name := range e.Expected.RequiredFields {
						require.True(t, protocolCorpusHasNode(node, fieldName(name)), "missing frozen field %q", name)
					}
					for name, raw := range e.Expected.Values {
						name = fieldName(name)
						var value any
						require.NoError(t, json.Unmarshal(raw, &value))
						switch v := value.(type) {
						case float64:
							var integer uint64
							require.NoError(t, json.Unmarshal(raw, &integer))
							protocolCorpusRequireValue(t, node, name, integer)
						case string:
							protocolCorpusRequireValue(t, node, name, v)
						case map[string]any:
							hexString, ok := v["bytes_hex"].(string)
							require.True(t, ok)
							data, err := hex.DecodeString(hexString)
							require.NoError(t, err)
							protocolCorpusRequireValue(t, node, name, data)
						default:
							t.Fatalf("unsupported frozen value for %s: %T", name, value)
						}
					}
					if c.ID == "ndpi-vxlan" {
						// The sealed old assertions above remain authoritative. New
						// header fields have a separate literal-byte oracle, independent
						// of the parser output and the default-entry choice.
						require.GreaterOrEqual(t, len(input), 8)
						values := map[string]uint64{"G": uint64(input[0] >> 7), "I": uint64(input[0] >> 3 & 1), "Reserved High": uint64(input[0] >> 4 & 7), "Reserved Low": uint64(input[0] & 7), "VNI": uint64(input[4])<<16 | uint64(input[5])<<8 | uint64(input[6])}
						for field, want := range values {
							protocolCorpusRequireValue(t, node, field, want)
						}
						for path, span := range map[string][2]uint64{"@VXLAN.G": {0, 1}, "@VXLAN.I": {4, 5}, "@VXLAN.VNI": {32, 56}} {
							field := base.GetNodeByPath(node, path)
							require.NotNil(t, field)
							require.Equal(t, span, stream_parser.GetNodeResultPos(field))
						}
					}
				})
			}
		}
	}
	require.GreaterOrEqual(t, parses, 434)
	require.GreaterOrEqual(t, rejections, 102)
}

func frozenSelection(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	for _, name := range []string{"TrimSuffix", "StartMagic", "StartAfter", "Base64After", "Prefix", "Contains"} {
		value, ok := fields[name]
		if !ok || len(value) == 0 || value[0] != '{' {
			continue
		}
		var encoded struct {
			Hex string `json:"bytes_hex"`
		}
		require.NoError(t, json.Unmarshal(value, &encoded))
		data, err := hex.DecodeString(encoded.Hex)
		require.NoError(t, err)
		fields[name], err = json.Marshal(data)
		require.NoError(t, err)
	}
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	return data
}

func TestFrozenCorpusClassificationAndStructure(t *testing.T) {
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	classifiers, structures := 0, 0
	for _, answers := range inventories {
		for _, c := range answers.Cases {
			for _, e := range c.Expectations {
				if e.Kind != "classification_only" && e.Kind != "capture_structure" {
					continue
				}
				if e.Kind == "classification_only" {
					classifiers++
				} else {
					structures++
				}
				t.Run(c.ID, func(t *testing.T) {
					capture := protocolCorpusCapture{ID: c.ID, CaptureFile: c.Input.OriginalPath, PacketCount: c.Facts.PacketCount, LinkType: c.Facts.LinkType}
					if e.Kind == "capture_structure" {
						raw := readProtocolCorpusFile(t, filepath.Join("..", ".."), c.Input.OriginalPath)
						count, link, _ := inspectProtocolCorpusCapture(t, raw, nil)
						require.Equal(t, c.Facts.PacketCount, count)
						require.Equal(t, c.Facts.LinkType, link)
						if e.Expected.FirstLayerError != "" {
							_, err := protocolCorpusFirstLayer(nil, link)
							require.ErrorContains(t, err, e.Expected.FirstLayerError)
						} else {
							require.Zero(t, count)
						}
						return
					}
					var spec protocolCorpusClassifierSpec
					require.NoError(t, json.Unmarshal(frozenSelection(t, e.PayloadConstraints), &spec))
					require.NotNil(t, e.Frame)
					capture.RepresentativeFrame = &protocolCorpusFrame{Number: e.Frame.Number, LengthBytes: e.Frame.LengthBytes, SHA256: e.Frame.SHA256}
					input := protocolCorpusParseInput(t, filepath.Join("..", ".."), capture, ProtocolInfo{Layer: spec.Layer}, protocolCorpusParseContract{Layer: spec.Layer, UseFullFrame: spec.UseFullFrame})
					if spec.ExactBytes > 0 {
						require.Len(t, input, spec.ExactBytes)
					}
					if spec.MinimumBytes > 0 {
						require.GreaterOrEqual(t, len(input), spec.MinimumBytes)
					}
					if spec.MaximumBytes > 0 {
						require.LessOrEqual(t, len(input), spec.MaximumBytes)
					}
					require.True(t, bytes.HasPrefix(input, spec.Prefix))
					require.True(t, bytes.Contains(input, spec.Contains))
					for offset, value := range spec.AtOffset {
						require.GreaterOrEqual(t, offset, 0)
						require.GreaterOrEqual(t, len(input), offset+len(value))
						require.Equal(t, value, input[offset:offset+len(value)])
					}
				})
			}
		}
	}
	require.GreaterOrEqual(t, classifiers, 14)
	require.GreaterOrEqual(t, structures, 2)
}
